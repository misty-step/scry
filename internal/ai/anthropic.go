package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"scry/internal/safeerr"
	"strings"
	"time"
)

// Claude is the real model adapter, on the Anthropic API. The key comes
// from the environment (ANTHROPIC_API_KEY).
//
// Drafting alone uses this generative adapter; Jev makes study decisions.
type Claude struct {
	secret     string
	BaseURL    string
	HTTP       *http.Client
	DraftModel string
}

// The drafting model can be changed from the environment.
const (
	DefaultDraftModel = "claude-opus-5-5"
)

// NewClaude builds the adapter. Extra options let tests point it at a
// stand-in server.
func NewClaude(apiKey, draftModel string) *Claude {
	if draftModel == "" {
		draftModel = DefaultDraftModel
	}
	return &Claude{secret: apiKey, DraftModel: draftModel}
}

func (c *Claude) Name() string { return "anthropic" }

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

const draftSystem = `You write study material for Scry, a spaced-repetition app. The learner asks for a topic, a question, or a book's title; you reply with a short outline of ideas, each with a few questions that test it. The learner will answer these from memory weeks later, typing or speaking, and a model will judge each answer.

If the request is a book's title (with or without its author), write the book's core ideas: the arguments and concepts a careful reader would want to keep, not plot trivia or chapter summaries. Otherwise treat it as a topic or question and write the ideas that matter most for understanding it.

Shape the material to this learner, described in the learner tags: pitch it to what they say about themselves, do not repeat ideas they already have (listed by title), and link to those ideas where the connection is real.

Write 4 to 7 ideas. For each idea:
- title: a short name for the idea, at most 48 characters, no colon.
- summary: one sentence that states the idea itself, at most 140 characters.
- links: titles of 1 to 3 related ideas, from this outline or from the learner's library, copied exactly.
- items: 2 to 4 questions. Each question stands alone (name its subject; it will be seen out of context), has one defensible answer, and is not yes/no. Each answer is short and canonical, with the essential part first: what a knowledgeable person would say aloud.

Only include what you are confident is correct. If you do not recognise the book or the topic, or the request is not something that can be learned (gibberish, a request for personal or dangerous information), say so instead of guessing.

Reply with JSON Lines only: one JSON object per line, no prose, no code fences.
Line 1: {"kind":"book" or "topic","title":"the book's title or the topic, tidied"}
Then one line per idea: {"idea":{"title":"…","summary":"…","links":["…"],"items":[{"q":"…","a":"…"}]}}
Last line: {"done":true,"note":"one plain sentence on how you shaped this to the learner, or empty"}
If there is nothing useful to write, reply with the single line {"nothing":"one plain sentence saying why, with a suggestion to narrow the topic"}.`

func (c *Claude) Draft(ctx context.Context, in DraftInput, sink DraftSink) (retD DraftResult, retU Usage, retErr error) {
	defer func() { retErr = safeerr.Redact(retErr, c.secret) }()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var user strings.Builder
	user.WriteString("<learner>\n")
	if in.Learner.About != "" {
		user.WriteString("About them, in their words: " + in.Learner.About + "\n")
	}
	if in.Learner.Level != "" {
		user.WriteString(in.Learner.Level + "\n")
	}
	if len(in.Learner.Knows) > 0 {
		user.WriteString("Ideas already in their library:\n")
		for _, k := range in.Learner.Knows {
			user.WriteString("- " + k + "\n")
		}
	}
	user.WriteString("</learner>\n<request>" + in.Query + "</request>")

	// Bound input and output before calling; automatic fallbacks could multiply cost.
	if len(user.String())+len(draftSystem) > 32000 {
		return DraftResult{}, Usage{}, fmt.Errorf("draft input exceeds budget")
	}
	payload, _ := json.Marshal(map[string]any{"model": c.DraftModel, "max_tokens": 16000, "stream": true, "system": draftSystem, "messages": []map[string]string{{"role": "user", "content": user.String()}}})
	base := c.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return DraftResult{}, Usage{}, errors.New("invalid drafting endpoint")
	}
	req.Header.Set("x-api-key", c.secret)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 5 * time.Minute}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return DraftResult{}, Usage{}, errors.New("draft provider unreachable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return DraftResult{}, Usage{}, fmt.Errorf("draft provider status %d", response.StatusCode)
	}
	var buf strings.Builder
	var res DraftResult
	var done bool
	use := Usage{}
	stopped := false
	handle := func(line string) error {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			return nil
		}
		var obj struct {
			Kind    string `json:"kind"`
			Title   string `json:"title"`
			Nothing string `json:"nothing"`
			Done    bool   `json:"done"`
			Note    string `json:"note"`
			Idea    *struct {
				Title   string   `json:"title"`
				Summary string   `json:"summary"`
				Links   []string `json:"links"`
				Items   []struct {
					Q string `json:"q"`
					A string `json:"a"`
				} `json:"items"`
			} `json:"idea"`
		}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			return nil // a malformed line is skipped, not fatal
		}
		switch {
		case obj.Nothing != "":
			res.Note, done = oneLine(obj.Nothing), true
		case obj.Done:
			res.Note, done = oneLine(obj.Note), true
		case obj.Idea != nil:
			idea := DraftIdea{Title: oneLine(obj.Idea.Title), Summary: oneLine(obj.Idea.Summary), Links: obj.Idea.Links}
			for _, it := range obj.Idea.Items {
				if q, a := oneLine(it.Q), oneLine(it.A); q != "" && a != "" {
					idea.Items = append(idea.Items, DraftItem{q, a})
				}
			}
			if idea.Title == "" || len(idea.Items) == 0 {
				return nil
			}
			res.Ideas++
			return sink.Idea(idea)
		case obj.Title != "" && sink.Head != nil:
			kind := "topic"
			if obj.Kind == "book" {
				kind = "book"
			}
			return sink.Head(DraftHead{Kind: kind, Title: oneLine(obj.Title)})
		}
		return nil
	}
	scan := bufio.NewScanner(io.LimitReader(response.Body, 4<<20))
	scan.Buffer(make([]byte, 4096), 1<<20)
	var eventData []string
	process := func() error {
		if len(eventData) == 0 {
			return nil
		}
		raw := strings.Join(eventData, "\n")
		eventData = nil
		var ev struct {
			Type    string
			Message struct {
				Model string
				Usage struct {
					Input  int64 `json:"input_tokens"`
					Output int64 `json:"output_tokens"`
				}
			}
			Delta struct {
				Type, Text string
				StopReason string `json:"stop_reason"`
			}
			Usage struct {
				Output int64 `json:"output_tokens"`
			}
		}
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			return errors.New("invalid draft stream")
		}
		switch ev.Type {
		case "message_start":
			use = Usage{Model: ev.Message.Model, InputTokens: ev.Message.Usage.Input, OutputTokens: ev.Message.Usage.Output}
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" {
				buf.WriteString(ev.Delta.Text)
				text := buf.String()
				if idx := strings.LastIndexByte(text, '\n'); idx >= 0 {
					for _, line := range strings.Split(text[:idx], "\n") {
						if err := handle(line); err != nil {
							return err
						}
					}
					buf.Reset()
					buf.WriteString(text[idx+1:])
				}
			}
		case "message_delta":
			use.OutputTokens = ev.Usage.Output
			if ev.Delta.StopReason == "refusal" {
				return errors.New("draft provider declined")
			}
		case "message_stop":
			stopped = true
		case "error":
			return errors.New("draft provider stream failed")
		}
		return nil
	}
	for scan.Scan() {
		line := scan.Text()
		if line == "" {
			if err := process(); err != nil {
				return res, use, err
			}
		} else if strings.HasPrefix(line, "data:") {
			eventData = append(eventData, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scan.Err(); err != nil {
		return res, use, errors.New("draft stream interrupted")
	}
	if err := process(); err != nil {
		return res, use, err
	}
	if !stopped {
		return res, use, errors.New("draft stream ended before completion")
	}
	if err := handle(buf.String()); err != nil {
		return res, use, err
	}
	if res.Ideas == 0 && res.Note == "" {
		res.Note = "Scry couldn't find anything useful on that. Try a narrower topic, a question, or a book's title."
	}
	_ = done
	return res, use, nil
}
