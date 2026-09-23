package generation

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
	"unicode/utf8"
)

type completionEnvelope struct {
	Model   string `json:"model"`
	Choices []struct {
		FinishReason string          `json:"finish_reason"`
		Error        json.RawMessage `json:"error"`
		Message      struct {
			Role      string          `json:"role"`
			Content   json.RawMessage `json:"content"`
			Refusal   json.RawMessage `json:"refusal"`
			ToolCalls json.RawMessage `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		Cost json.RawMessage `json:"cost"`
	} `json:"usage"`
	Error json.RawMessage `json:"error"`
}

type completion struct {
	content string
	model   string
	cost    *int64
}

func (w *Worker) call(ctx context.Context, payload []byte) (completion, *generationFailure) {
	response := completion{}
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, w.cfg.Endpoint, bytes.NewReader(payload))
	if err != nil {
		zero := int64(0)
		response.cost = &zero
		return response, &generationFailure{message: "Preparation could not start. Check the connection settings and retry."}
	}
	if requestCtx.Err() != nil {
		zero := int64(0)
		response.cost = &zero
		return response, &generationFailure{message: "Preparation stopped before anything was sent. Your material is saved and can be retried."}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if w.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+w.cfg.APIKey)
	}
	if w.openRouter {
		req.Header.Set("X-OpenRouter-Title", "Scry")
	}
	// The durable claim is the attempt boundary. No idempotency header is
	// invented: the provider does not promise exactly-once billing for this API.
	res, err := w.client.Do(req)
	if err != nil {
		if res != nil && res.Body != nil {
			res.Body.Close()
		}
		return response, &generationFailure{message: "The request failed or timed out after it may have been sent. Cost is unknown, so it will not retry automatically. Check your receipt before retrying."}
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return response, &generationFailure{message: "The response was interrupted or too large to read. Cost is unknown and nothing was published. Check your receipt before retrying."}
	}
	var envelope completionEnvelope
	jsonValid := utf8.Valid(body) && checkJSON(body, 24) == nil
	readable := false
	// Extract usage independently of malformed choices/model fields. A valid
	// paid receipt does not become unknown just because its content is rejected.
	if jsonValid {
		var receipt struct {
			Usage struct {
				Cost json.RawMessage `json:"cost"`
			} `json:"usage"`
		}
		if json.Unmarshal(body, &receipt) == nil {
			response.cost = costMicros(receipt.Usage.Cost)
		}
		readable = json.Unmarshal(body, &envelope) == nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := "Preparation was rejected. Check your connection settings before retrying."
		switch res.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			message = "Access was denied. Check your private key and account access before retrying."
		case http.StatusPaymentRequired:
			message = "The account has insufficient credit. Check billing and your receipt before retrying."
		case http.StatusTooManyRequests:
			message = "Too many requests were sent at once. Your material and reported cost are saved; retry later."
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			message = "The preparation service did not accept the request. Check your connection settings before retrying."
		default:
			if res.StatusCode >= 300 && res.StatusCode < 400 {
				message = "The preparation service tried to change destinations. No material was sent to the new address. Check your connection settings."
			} else if res.StatusCode >= 500 {
				message = "The preparation service is temporarily unavailable. Your material and reported cost are saved; retry later."
			}
		}
		// A lost/unknown-cost response is never silently billed again. An
		// explicitly free transient rejection may use Store's bounded backoff.
		retry := (res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500) && response.cost != nil && *response.cost == 0
		return response, &generationFailure{message: message, retry: retry}
	}
	if !readable {
		return response, &generationFailure{message: "The response could not be read. Reported cost was saved; any missing cost remains unknown. Nothing was published. Check your receipt before retrying."}
	}
	if nonNull(envelope.Error) || len(envelope.Choices) != 1 {
		return response, &generationFailure{message: "The response was incomplete or unclear. Reported cost was saved and nothing was published. Check your receipt before retrying."}
	}
	choice := envelope.Choices[0]
	if nonNull(choice.Error) || choice.FinishReason != "stop" || (choice.Message.Role != "" && choice.Message.Role != "assistant") || nonEmpty(choice.Message.Refusal) || nonEmpty(choice.Message.ToolCalls) {
		return response, &generationFailure{message: "The response was refused or cut short. Reported cost was saved and nothing was published. Clarify or split your material before retrying."}
	}
	if json.Unmarshal(choice.Message.Content, &response.content) != nil || response.content == "" || len(response.content) > maxContentBytes || !utf8.ValidString(response.content) {
		return response, &generationFailure{message: "The response was missing, too large, or could not be read. Reported cost was saved and nothing was published. Split your material before retrying."}
	}
	if len(envelope.Model) > 200 || strings.ContainsAny(envelope.Model, "\r\n\x00") {
		return response, &generationFailure{message: "The response was missing its source details. Reported cost was saved and nothing was published."}
	}
	response.model = envelope.Model
	return response, nil
}

func nonNull(raw json.RawMessage) bool {
	return len(raw) != 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func nonEmpty(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return nonNull(trimmed) && !bytes.Equal(trimmed, []byte(`""`)) && !bytes.Equal(trimmed, []byte("[]"))
}

// costMicros accepts only an explicit nonnegative USD number, rounds UP to the
// next micro-dollar, and never substitutes token estimates or missing=zero.
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

// checkJSON rejects duplicate members, excessive nesting and extra top-level
// values. encoding/json otherwise quietly accepts the last duplicate key.
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
		return fmt.Errorf("extra or invalid JSON after value")
	}
	return nil
}
