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

	"github.com/misty-step/scry/internal/store"
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

func (w *Worker) generate(ctx context.Context, job *store.Job) (store.GenerationResult, *int64, *generationFailure) {
	if job.FoundationTarget != nil {
		return w.generateFoundation(ctx, job)
	}
	zero := int64(0)
	plan, err := planTask(job.SourceText, job.SourceKind)
	if err != nil {
		return store.GenerationResult{}, &zero, &generationFailure{message: err.Error()}
	}
	repair := job.Attempts == 2 && strings.HasPrefix(job.Error, repairMarker)
	request, err := makeRequest(w.cfg.Model, job, plan, repair, w.openRouter)
	if err != nil {
		return store.GenerationResult{}, &zero, &generationFailure{message: "Generation request exceeds its safe size limit. Split the saved material into smaller captures, then retry."}
	}
	response, failure := w.call(ctx, request)
	if failure != nil {
		return store.GenerationResult{}, response.cost, failure
	}
	result, issues, parseErr := validateOutput(response.content, job, plan)
	if parseErr != nil {
		// Syntax errors, truncation, refusals, and adversarial envelopes do not earn
		// a second paid request. In particular, never recover fenced substrings.
		return store.GenerationResult{}, response.cost, &generationFailure{message: "The provider returned malformed or unsupported quiz JSON. No quizzes were published; usage was retained. Retry explicitly after checking the model's structured-output support."}
	}
	if len(result.Quizzes) == 0 {
		if job.Attempts == 1 && len(issues) > 0 && len(issues) <= 4 && response.cost != nil {
			// One earned quality repair is a NEW durable attempt with its own spend
			// reservation. Store preserves the safe code-only error on that claim.
			return store.GenerationResult{}, response.cost, &generationFailure{
				message: repairMarker + strings.Join(issues, ", ") + ". No usable quizzes passed; one separately reserved repair may run.",
				retry:   true,
			}
		}
		message := "No usable quizzes passed the source, task-coverage, or answerability checks. The source and paid usage are saved. Clarify the learning task or provide an authoritative excerpt before retrying."
		if len(issues) > 0 {
			message += " Checks: " + strings.Join(issues[:min(4, len(issues))], ", ") + "."
		}
		return store.GenerationResult{}, response.cost, &generationFailure{message: message}
	}
	result.Model = response.model
	if result.Model == "" {
		result.Model = w.cfg.Model
	} else if result.Model != w.cfg.Model {
		result.Note += " Requested model: " + w.cfg.Model + "."
	}
	result.PromptVersion = promptVersion
	if repair {
		result.PromptVersion += "-repair1"
	}
	return result, response.cost, nil
}

func (w *Worker) call(ctx context.Context, payload []byte) (completion, *generationFailure) {
	response := completion{}
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, w.cfg.Endpoint, bytes.NewReader(payload))
	if err != nil {
		zero := int64(0)
		response.cost = &zero
		return response, &generationFailure{message: "Generation request configuration is invalid. Update the endpoint and retry."}
	}
	if requestCtx.Err() != nil {
		zero := int64(0)
		response.cost = &zero
		return response, &generationFailure{message: "Generation stopped before transmission; the source is saved and can be retried."}
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
		return response, &generationFailure{message: "The generation request failed or timed out after transmission may have begun. Spend is unknown; no automatic paid retry was scheduled. Check the provider receipt before retrying this saved source."}
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return response, &generationFailure{message: "The provider response was interrupted or exceeded the 1 MiB limit. Spend is unknown; no quizzes were published. Check the provider receipt before retrying."}
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
		message := "The provider rejected generation. Check provider configuration and retry this saved source."
		switch res.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			message = "The provider rejected authentication or model access. Update the private API key or model permissions, then retry this saved source."
		case http.StatusPaymentRequired:
			message = "The provider has insufficient credit. Check provider billing and the recorded attempt before retrying this saved source."
		case http.StatusTooManyRequests:
			message = "The provider rate-limited generation. The source and reported usage are saved; retry after the provider limit clears."
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			message = "The provider does not accept this model or structured-output request. Choose a compatible model and chat-completions endpoint, then retry."
		default:
			if res.StatusCode >= 300 && res.StatusCode < 400 {
				message = "The provider attempted a redirect, which was not followed to protect source data and credentials. Configure the final HTTPS chat-completions endpoint and retry."
			} else if res.StatusCode >= 500 {
				message = "The provider is temporarily unavailable. The source and reported usage are saved; retry when the provider recovers."
			}
		}
		// A lost/unknown-cost response is never silently billed again. An
		// explicitly free transient rejection may use Store's bounded backoff.
		retry := (res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500) && response.cost != nil && *response.cost == 0
		return response, &generationFailure{message: message, retry: retry}
	}
	if !readable {
		return response, &generationFailure{message: "The provider returned an unreadable response. Reported usage was retained; any missing price remains unknown. No quizzes were published. Check provider health and the receipt before retrying."}
	}
	if nonNull(envelope.Error) || len(envelope.Choices) != 1 {
		return response, &generationFailure{message: "The provider returned an error or an ambiguous completion envelope. Usage was retained; no quizzes were published. Check provider compatibility before retrying."}
	}
	choice := envelope.Choices[0]
	if nonNull(choice.Error) || choice.FinishReason != "stop" || (choice.Message.Role != "" && choice.Message.Role != "assistant") || nonEmpty(choice.Message.Refusal) || nonEmpty(choice.Message.ToolCalls) {
		return response, &generationFailure{message: "The provider refused, truncated, or did not finish an ordinary text completion. Usage was retained; no quizzes were published. Clarify or split the source before retrying."}
	}
	if json.Unmarshal(choice.Message.Content, &response.content) != nil || response.content == "" || len(response.content) > maxContentBytes || !utf8.ValidString(response.content) {
		return response, &generationFailure{message: "The provider returned missing, oversized, or unsupported completion content. Usage was retained; no quizzes were published. Check structured-output support or split the source before retrying."}
	}
	if len(envelope.Model) > 200 || strings.ContainsAny(envelope.Model, "\r\n\x00") {
		return response, &generationFailure{message: "The provider returned invalid model provenance. Usage was retained; no quizzes were published."}
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
