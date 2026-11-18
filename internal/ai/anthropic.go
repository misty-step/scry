package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"scry/internal/safeerr"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// Claude is the real model adapter, on the Anthropic API. The key comes
// from the environment (ANTHROPIC_API_KEY).
//
// Drafting alone uses this generative adapter; Jev makes study decisions.
type Claude struct {
	secret     string
	client     anthropic.Client
	DraftModel string
}

// The drafting model can be changed from the environment.
const (
	DefaultDraftModel = "claude-opus-5-5"
)

// NewClaude builds the adapter. Extra options let tests point it at a
// stand-in server.
func NewClaude(apiKey, draftModel string, opts ...option.RequestOption) *Claude {
	if draftModel == "" {
		draftModel = DefaultDraftModel
	}
	opts = append([]option.RequestOption{option.WithAPIKey(apiKey), option.WithMaxRetries(0)}, opts...)
	return &Claude{secret: apiKey, client: anthropic.NewClient(opts...), DraftModel: draftModel}
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
	stream := c.client.Beta.Messages.NewStreaming(ctx, anthropic.BetaMessageNewParams{
		Model:     c.DraftModel,
		MaxTokens: 16000,
		System:    []anthropic.BetaTextBlockParam{{Text: draftSystem}},
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(user.String()))},
	})
	var (
		msg  anthropic.BetaMessage
		buf  strings.Builder
		res  DraftResult
		done bool
	)
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
	for stream.Next() {
		event := stream.Current()
		if err := msg.Accumulate(event); err != nil {
			return res, Usage{}, err
		}
		if delta, ok := event.AsAny().(anthropic.BetaRawContentBlockDeltaEvent); ok {
			if t, ok := delta.Delta.AsAny().(anthropic.BetaTextDelta); ok {
				buf.WriteString(t.Text)
				// Hand over every complete line; keep the unfinished tail.
				text := buf.String()
				if i := strings.LastIndexByte(text, '\n'); i >= 0 {
					sc := bufio.NewScanner(strings.NewReader(text[:i]))
					sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
					for sc.Scan() {
						if err := handle(sc.Text()); err != nil {
							return res, Usage{}, err
						}
					}
					buf.Reset()
					buf.WriteString(text[i+1:])
				}
			}
		}
	}
	use := Usage{Model: c.DraftModel, InputTokens: msg.Usage.InputTokens, OutputTokens: msg.Usage.OutputTokens}
	if err := stream.Err(); err != nil {
		return res, use, describe(err)
	}
	if err := handle(buf.String()); err != nil {
		return res, use, err
	}
	if msg.StopReason == anthropic.BetaStopReasonRefusal {
		return DraftResult{Note: "Scry can't write material on that. Try a different topic."}, use, nil
	}
	if res.Ideas == 0 && res.Note == "" {
		res.Note = "Scry couldn't find anything useful on that. Try a narrower topic, a question, or a book's title."
	}
	_ = done
	return res, use, nil
}

// describe turns an SDK error into one that says whether retrying helps.
func describe(err error) error {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == 401 || apiErr.StatusCode == 403:
			return fmt.Errorf("model provider rejected the API key (status %d)", apiErr.StatusCode)
		case apiErr.StatusCode == 429:
			return fmt.Errorf("model provider is rate limiting (status 429): %w", err)
		case apiErr.StatusCode >= 500:
			return fmt.Errorf("model provider is failing (status %d): %w", apiErr.StatusCode, err)
		default:
			return fmt.Errorf("model provider refused the request (status %d): %w", apiErr.StatusCode, err)
		}
	}
	return fmt.Errorf("model provider unreachable: %w", err)
}
