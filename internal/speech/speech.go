// Package speech turns a spoken answer into text behind a small interface.
package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"reflect"
	"scry/internal/safeerr"
	"strings"
	"time"
)

// Audio is a recording of the learner speaking.
type Audio struct {
	Data    []byte
	MIME    string  // for example audio/webm
	Seconds float64 // length reported by the recorder
}

// Transcriber hears an answer.
type Transcriber interface {
	Name() string
	// Transcribe returns the words heard. Empty text means nothing was
	// caught; that is not an error.
	Transcribe(ctx context.Context, a Audio) (text string, err error)
}

// Fake is the test-mode adapter. It needs no keys and cannot hear, so it
// follows a script: a recording whose bytes begin "say:" is heard as the
// text after the prefix (the tests use this); any other recording is heard
// as Phrase, revealed a few words at a time as the recording grows, which
// is what makes live text appear in the demo. Very short recordings are
// not caught at all.
type Fake struct {
	Phrase string
}

func (f Fake) Name() string { return "test" }

// ModelName is the test adapter's model label.
func (f Fake) ModelName() string { return "test-speech" }

func (f Fake) Transcribe(_ context.Context, a Audio) (string, error) {
	if s, ok := strings.CutPrefix(string(a.Data), "say:"); ok {
		return strings.TrimSpace(s), nil
	}
	if len(a.Data) < 2000 && a.Seconds < 0.6 {
		return "", nil
	}
	phrase := f.Phrase
	if phrase == "" {
		phrase = "this is the test microphone"
	}
	words := strings.Fields(phrase)
	n := int(a.Seconds*2.5) + 1
	if a.Seconds == 0 {
		n = len(a.Data)/6000 + 1
	}
	if n > len(words) {
		n = len(words)
	}
	return strings.Join(words[:n], " "), nil
}

// OpenAI is the optional direct adapter: the OpenAI audio transcription
// API. The key comes from the environment (OPENAI_API_KEY) and is used only
// when OPENROUTER_API_KEY is unset (see NewTranscriber).
type OpenAI struct {
	APIKey  string
	Model   string // defaults to gpt-4o-mini-transcribe
	BaseURL string
	HTTP    *http.Client
}

func (o OpenAI) Name() string { return "openai" }

// ModelName is the model this adapter calls.
func (o OpenAI) ModelName() string {
	if m := strings.TrimSpace(o.Model); m != "" {
		return m
	}
	return "gpt-4o-mini-transcribe"
}

// ModelOf names the model a transcriber calls, or "" when none is wired.
func ModelOf(t Transcriber) string {
	if t == nil {
		return ""
	}
	if rv := reflect.ValueOf(t); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return ""
	}
	if m, ok := t.(interface{ ModelName() string }); ok {
		return m.ModelName()
	}
	return "unknown"
}

// RouteOf names the provider route a transcriber sends recordings on:
// RouteTranscriptions or RouteChat on OpenRouter, "audio/transcriptions"
// for the direct OpenAI adapter, "test" for the fake, "" when none is wired.
func RouteOf(t Transcriber) string {
	if ModelOf(t) == "" {
		return ""
	}
	switch v := t.(type) {
	case OpenRouter:
		return v.Route()
	case *OpenRouter:
		return v.Route()
	case OpenAI, *OpenAI:
		return RouteTranscriptions
	case Fake, *Fake:
		return "test"
	}
	return "unknown"
}

func (o OpenAI) Transcribe(ctx context.Context, a Audio) (result string, retErr error) {
	defer func() { retErr = safeerr.Redact(retErr, o.APIKey) }()
	if err := ValidateAPIKey("OPENAI_API_KEY", o.APIKey); err != nil {
		return "", err
	}
	if len(a.Data) == 0 {
		return "", nil // nothing recorded is nothing heard, not an error
	}
	base, model := o.BaseURL, o.Model
	if base == "" {
		base = "https://api.openai.com"
	}
	if model == "" {
		model = "gpt-4o-mini-transcribe"
	}
	ext := "webm"
	switch {
	case strings.Contains(a.MIME, "mp4"), strings.Contains(a.MIME, "m4a"):
		ext = "mp4"
	case strings.Contains(a.MIME, "ogg"):
		ext = "ogg"
	case strings.Contains(a.MIME, "wav"):
		ext = "wav"
	case strings.Contains(a.MIME, "mpeg"):
		ext = "mp3"
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("model", model)
	_ = mw.WriteField("response_format", "json")
	part, err := mw.CreateFormFile("file", "answer."+ext)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(a.Data); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/audio/transcriptions", &body)
	if err != nil {
		return "", errors.New("invalid speech endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := noRedirect(o.HTTP).Do(req)
	if err != nil {
		return "", fmt.Errorf("speech provider unreachable: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		Text  string `json:"text"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxSpeechResponse)).Decode(&out); err != nil {
		return "", fmt.Errorf("speech provider: status %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("speech provider: status %d %s", resp.StatusCode, bounded(out.Error.Message))
	}
	return strings.TrimSpace(out.Text), nil
}

// maxSpeechResponse bounds what is read from a speech provider.
const maxSpeechResponse = 1 << 20

// noRedirect copies the given client (or a 20-second default) and stops
// it following redirects, so a key is never replayed to another origin.
func noRedirect(c *http.Client) *http.Client {
	client := http.Client{Timeout: 20 * time.Second}
	if c != nil {
		client = *c
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client
}

// bounded keeps a provider's error message to one short line.
func bounded(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200]) + "…"
	}
	return s
}
