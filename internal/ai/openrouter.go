package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// OpenRouter's OpenAI-compatible chat completions route. Drafting streams
// it; explanations call it once. The key is OPENROUTER_API_KEY, the same
// key Jev grades with, so production needs no separate Anthropic key.

const defaultRouterBase = "https://openrouter.ai"

func (c *Claude) routerRequest(ctx context.Context, payload []byte) (*http.Request, error) {
	base := c.BaseURL
	if base == "" {
		base = defaultRouterBase
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setRouterHeaders(req, c.secret)
	return req, nil
}

// setRouterHeaders sets the auth and OpenRouter attribution headers.
func setRouterHeaders(req *http.Request, key string) {
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://scry.mistystep.io")
	req.Header.Set("X-Title", "Scry")
}

// routerUsage is OpenRouter's usage object. Cost is the provider's exact
// charge in dollars; it is recorded only when present and sane, never
// inferred.
type routerUsage struct {
	Prompt     int64    `json:"prompt_tokens"`
	Completion int64    `json:"completion_tokens"`
	Cost       *float64 `json:"cost"`
}

func (u *routerUsage) apply(use *Usage) {
	if u == nil {
		return
	}
	use.InputTokens, use.OutputTokens = u.Prompt, u.Completion
	if c := u.Cost; c != nil && !math.IsNaN(*c) && !math.IsInf(*c, 0) && *c >= 0 && *c <= 100 {
		usd := *c
		micros := int64(math.Ceil(usd * 1e6))
		use.CostUSD, use.CostMicros = &usd, &micros
	}
}

// routerError is the error object OpenRouter puts in a body or a stream
// chunk. Its message is never surfaced: it may echo the request.
type routerError struct {
	Code any `json:"code"`
}

func (c *Claude) draftOpenRouter(ctx context.Context, user string, sink DraftSink) (DraftResult, Usage, error) {
	payload, _ := json.Marshal(map[string]any{
		"model": c.DraftModel, "max_tokens": 16000, "stream": true,
		"stream_options": map[string]any{"include_usage": true},
		"messages":       []map[string]string{{"role": "system", "content": draftSystem}, {"role": "user", "content": user}},
	})
	req, err := c.routerRequest(ctx, payload)
	if err != nil {
		return DraftResult{}, Usage{}, errors.New("invalid drafting endpoint")
	}
	resp, err := c.client(5 * time.Minute).Do(req)
	if err != nil {
		return DraftResult{}, Usage{}, errors.New("draft provider unreachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return DraftResult{}, Usage{}, fmt.Errorf("draft provider status %d", resp.StatusCode)
	}
	p := &draftLines{sink: sink}
	// G2: use.Model stays empty until the provider reports usage. App.runDraft
	// settles a reservation only when use.Model is set; on any failure with
	// no usage reported (a truncated stream, a stream error, a refusal or a
	// sink error before the usage chunk) the outcome is unknown, so the
	// reservation must stay held. The model name is tracked separately.
	use := Usage{}
	model := c.DraftModel
	reported := false
	stopped := false
	var eventData []string
	process := func() error {
		if len(eventData) == 0 {
			return nil
		}
		raw := strings.TrimSpace(strings.Join(eventData, "\n"))
		eventData = nil
		if raw == "[DONE]" {
			stopped = true
			return nil
		}
		if stopped {
			return nil // nothing after [DONE] counts
		}
		var ev struct {
			Model   string       `json:"model"`
			Error   *routerError `json:"error"`
			Usage   *routerUsage `json:"usage"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			return errors.New("invalid draft stream")
		}
		if ev.Model != "" {
			model = ev.Model
		}
		if ev.Usage != nil {
			// Reported usage is recorded even on a chunk that also fails, so
			// a billed failure settles at its reported cost.
			ev.Usage.apply(&use)
			reported = true
		}
		if reported {
			use.Model = redactModel(model, c.secret)
		}
		if ev.Error != nil {
			return errors.New("draft provider stream failed")
		}
		if len(ev.Choices) == 0 {
			return nil
		}
		ch := ev.Choices[0]
		if ch.FinishReason != nil {
			switch *ch.FinishReason {
			case "content_filter":
				return errors.New("draft provider declined")
			case "error":
				return errors.New("draft provider stream failed")
			}
		}
		if ch.Delta.Content != "" {
			return p.feed(ch.Delta.Content)
		}
		return nil
	}
	scan := bufio.NewScanner(io.LimitReader(resp.Body, 4<<20))
	scan.Buffer(make([]byte, 4096), 1<<20)
	for scan.Scan() {
		line := strings.TrimRight(scan.Text(), "\r")
		switch {
		case line == "":
			if err := process(); err != nil {
				return p.res, use, err
			}
		case strings.HasPrefix(line, ":"):
			// An SSE comment, such as ": OPENROUTER PROCESSING" keepalives.
		case strings.HasPrefix(line, "data:"):
			eventData = append(eventData, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scan.Err(); err != nil {
		return p.res, use, errors.New("draft stream interrupted")
	}
	if err := process(); err != nil {
		return p.res, use, err
	}
	if !stopped {
		return p.res, use, errors.New("draft stream ended before completion")
	}
	res, err := p.finish()
	if err == nil && !reported {
		// A clean completion whose usage chunk upstream omitted: the outcome
		// is known, so it settles (at the tokens reported, none).
		use.Model = redactModel(model, c.secret)
	}
	return res, use, err
}

func (c *Claude) explainOpenRouter(ctx context.Context, model, user string) (string, Usage, error) {
	payload, _ := json.Marshal(map[string]any{
		"model": model, "max_tokens": ExplainMaxTokens,
		"messages": []map[string]string{{"role": "system", "content": explainSystem}, {"role": "user", "content": user}},
	})
	req, err := c.routerRequest(ctx, payload)
	if err != nil {
		return "", Usage{}, errors.New("invalid explanation endpoint")
	}
	resp, err := c.client(ExplainTimeout).Do(req)
	if err != nil {
		return "", Usage{}, errors.New("explanation provider unreachable")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", Usage{}, errors.New("explanation response unreadable")
	}
	var out struct {
		Model   string       `json:"model"`
		Error   *routerError `json:"error"`
		Usage   *routerUsage `json:"usage"`
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", Usage{}, fmt.Errorf("explanation provider status %d", resp.StatusCode)
	}
	// G2, as in draftOpenRouter: use.Model is set only when the provider
	// reported usage, or on a clean success; a failure with no usage
	// reported leaves it empty (outcome unknown).
	if out.Model != "" {
		model = out.Model
	}
	use := Usage{}
	if out.Usage != nil {
		out.Usage.apply(&use)
		use.Model = redactModel(model, c.secret)
	}
	if resp.StatusCode != http.StatusOK || out.Error != nil {
		return "", use, fmt.Errorf("explanation provider status %d", resp.StatusCode)
	}
	if len(out.Choices) == 0 {
		return "", use, errors.New("explanation was empty")
	}
	switch out.Choices[0].FinishReason {
	case "content_filter":
		return "", use, errors.New("explanation provider declined")
	case "error":
		return "", use, errors.New("explanation provider failed")
	}
	text := ""
	if out.Choices[0].Message.Content != nil {
		text = *out.Choices[0].Message.Content
	}
	s := ClampExplanation(text)
	if s == "" {
		return "", use, errors.New("explanation was empty")
	}
	if use.Model == "" {
		use.Model = redactModel(model, c.secret)
	}
	return s, use, nil
}
