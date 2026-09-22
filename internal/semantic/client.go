// Package semantic owns bounded Jev assessment requests and orchestration. It
// never holds a SQL transaction while calling a model.
package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrUnavailable = errors.New("semantic assessor unavailable")
	ErrRejected    = errors.New("semantic assessment rejected")
	ErrMalformed   = errors.New("malformed semantic response")
	// ErrNotConfigured is the one provable no-send failure: nothing left the
	// process, so no provider spend can exist. It also satisfies ErrUnavailable.
	ErrNotConfigured = fmt.Errorf("%w: endpoint is not configured", ErrUnavailable)
)

const (
	DefaultModel    = "typesafe/jev-1.13"
	requestTimeout  = 8 * time.Second
	maxResponseSize = 256 << 10
)

type Question struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type Request struct {
	Model     string              `json:"model"`
	State     any                 `json:"state"`
	Questions map[string]Question `json:"questions"`
}

type Answer struct {
	Type          string             `json:"type,omitempty"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	CostMicros   *int64
}

type Response struct {
	Model     string
	Answers   map[string]Answer
	Usage     Usage
	Raw       json.RawMessage
	LatencyMS int64
	RequestID string
}

type Client interface {
	Decide(ctx context.Context, req Request) (Response, error)
}

type Config struct {
	Endpoint   string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

type client struct {
	endpoint string
	apiKey   string
	model    string
	http     *http.Client
}

func NewClient(cfg Config) Client {
	model := cfg.Model
	if model == "" {
		model = DefaultModel
	}
	httpClient := http.Client{}
	if cfg.HTTPClient != nil {
		httpClient = *cfg.HTTPClient
	}
	httpClient.Timeout = requestTimeout
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return errors.New("semantic redirects are not permitted")
	}
	return &client{endpoint: strings.TrimSpace(cfg.Endpoint), apiKey: cfg.APIKey, model: model, http: &httpClient}
}

type responseEnvelope struct {
	ID      string            `json:"id"`
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   struct {
		InputTokens  int             `json:"input_tokens"`
		OutputTokens int             `json:"output_tokens"`
		Cost         json.RawMessage `json:"cost"`
	} `json:"usage"`
}

func (c *client) Decide(ctx context.Context, request Request) (Response, error) {
	started := time.Now()
	response := Response{}
	if c.endpoint == "" {
		return response, ErrNotConfigured
	}
	if request.Model == "" {
		request.Model = c.model
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return response, fmt.Errorf("%w: encode request", ErrMalformed)
	}
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return response, fmt.Errorf("%w: invalid endpoint", ErrRejected)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("X-OpenRouter-Title", "Scry")
	httpResponse, err := c.http.Do(httpRequest)
	response.LatencyMS = elapsedMillis(started)
	if err != nil {
		if httpResponse != nil && httpResponse.Body != nil {
			httpResponse.Body.Close()
		}
		if httpResponse != nil && httpResponse.StatusCode >= 300 && httpResponse.StatusCode < 400 {
			return response, fmt.Errorf("%w: redirect refused", ErrRejected)
		}
		if requestCtx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return response, fmt.Errorf("%w: request timed out or was canceled", ErrUnavailable)
		}
		return response, fmt.Errorf("%w: network request failed", ErrUnavailable)
	}
	defer httpResponse.Body.Close()
	response.RequestID = httpResponse.Header.Get("X-Request-ID")
	body, readErr := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseSize+1))
	if len(body) <= maxResponseSize && utf8.Valid(body) && json.Valid(body) {
		response.Raw = append(json.RawMessage(nil), body...)
	}
	if readErr != nil || len(body) > maxResponseSize {
		return response, fmt.Errorf("%w: response exceeded the 256 KiB limit or was interrupted", ErrMalformed)
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		switch {
		case httpResponse.StatusCode == http.StatusTooManyRequests,
			httpResponse.StatusCode == 529,
			httpResponse.StatusCode >= 500:
			return response, fmt.Errorf("%w: HTTP %d", ErrUnavailable, httpResponse.StatusCode)
		case httpResponse.StatusCode >= 400 && httpResponse.StatusCode < 500:
			return response, fmt.Errorf("%w: HTTP %d", ErrRejected, httpResponse.StatusCode)
		default:
			return response, fmt.Errorf("%w: unexpected HTTP %d", ErrRejected, httpResponse.StatusCode)
		}
	}
	if !utf8.Valid(body) || checkJSON(body, 24) != nil {
		return response, fmt.Errorf("%w: response is not strict JSON", ErrMalformed)
	}
	var envelope responseEnvelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err = decoder.Decode(&envelope); err != nil {
		return response, fmt.Errorf("%w: invalid response envelope", ErrMalformed)
	}
	if envelope.ID != "" {
		response.RequestID = envelope.ID
	}
	if !validLabel(envelope.Model, 200) || len(envelope.Answers) == 0 || envelope.Usage.InputTokens < 0 || envelope.Usage.OutputTokens < 0 {
		return response, fmt.Errorf("%w: missing or invalid response fields", ErrMalformed)
	}
	for key, answer := range envelope.Answers {
		if !validLabel(key, 200) || !validAnswer(answer) {
			return response, fmt.Errorf("%w: invalid answer %q", ErrMalformed, key)
		}
	}
	response.Model = envelope.Model
	response.Answers = envelope.Answers
	response.Usage = Usage{InputTokens: envelope.Usage.InputTokens, OutputTokens: envelope.Usage.OutputTokens, CostMicros: costMicros(envelope.Usage.Cost)}
	response.Raw = append(json.RawMessage(nil), body...)
	return response, nil
}

func validAnswer(answer Answer) bool {
	if answer.Type != "" && !validLabel(answer.Type, 200) {
		return false
	}
	if answer.Type != "" && answer.Type != "noul" && answer.Type != "choice" && answer.Type != "score" {
		return false
	}
	if answer.Choice != "" && !validLabel(answer.Choice, 200) {
		return false
	}
	if answer.Noul != nil && !probability(*answer.Noul) {
		return false
	}
	if answer.Confidence != nil && !probability(*answer.Confidence) {
		return false
	}
	for label, value := range answer.Probabilities {
		if !validLabel(label, 200) || !probability(value) {
			return false
		}
	}
	return answer.Noul != nil || answer.Choice != "" || answer.Score != nil
}

func probability(value float64) bool { return value >= 0 && value <= 1 }

func validLabel(value string, max int) bool {
	return value != "" && len(value) <= max && utf8.ValidString(value) && !strings.ContainsAny(value, "\r\n\x00")
}

func elapsedMillis(started time.Time) int64 {
	elapsed := time.Since(started).Milliseconds()
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

// costMicros accepts only a nonnegative JSON number, rounds upward to the next
// micro-dollar, and keeps missing or malformed cost unknown.
func costMicros(raw json.RawMessage) *int64 {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > 64 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&number) != nil || number.String() == "" || strings.HasPrefix(number.String(), "-") || raw[0] == '"' {
		return nil
	}
	if index := strings.IndexAny(number.String(), "eE"); index >= 0 {
		exponent, err := strconv.Atoi(number.String()[index+1:])
		if err != nil || exponent < -30 || exponent > 30 {
			return nil
		}
	}
	rate, ok := new(big.Rat).SetString(number.String())
	if !ok || rate.Sign() < 0 {
		return nil
	}
	rate.Mul(rate, big.NewRat(1_000_000, 1))
	micros, remainder := new(big.Int), new(big.Int)
	micros.QuoRem(rate.Num(), rate.Denom(), remainder)
	if remainder.Sign() > 0 {
		micros.Add(micros, big.NewInt(1))
	}
	if !micros.IsInt64() {
		return nil
	}
	value := micros.Int64()
	return &value
}

// checkJSON rejects duplicate members, excessive nesting, and extra top-level
// values; encoding/json otherwise silently accepts the last duplicate member.
func checkJSON(data []byte, maxDepth int) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var visit func(int) error
	visit = func(depth int) error {
		if depth > maxDepth {
			return errors.New("JSON nesting exceeds limit")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, composite := token.(json.Delim)
		if !composite {
			return nil
		}
		switch delimiter {
		case '{':
			keys := make(map[string]bool)
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || keys[name] {
					return errors.New("duplicate or invalid JSON member")
				}
				keys[name] = true
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
		default:
			return errors.New("unexpected JSON delimiter")
		}
		closeToken, err := decoder.Token()
		if err != nil || (delimiter == '{' && closeToken != json.Delim('}')) || (delimiter == '[' && closeToken != json.Delim(']')) {
			return errors.New("unterminated JSON composite")
		}
		return nil
	}
	if err := visit(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("extra or invalid JSON after value")
	}
	return nil
}
