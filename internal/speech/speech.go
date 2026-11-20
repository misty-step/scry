// Package speech turns a spoken answer into text behind a small interface.
package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
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

// OpenAI is the real adapter: the OpenAI audio transcription API. The key
// comes from the environment (OPENAI_API_KEY).
type OpenAI struct {
	APIKey  string
	Model   string // defaults to gpt-4o-mini-transcribe
	BaseURL string
	HTTP    *http.Client
}

func (o OpenAI) Name() string { return "openai" }

func (o OpenAI) Transcribe(ctx context.Context, a Audio) (result string, retErr error) {
	defer func() { retErr = safeerr.Redact(retErr, o.APIKey) }()
	base, model := o.BaseURL, o.Model
	if base == "" {
		base = "https://api.openai.com"
	}
	if model == "" {
		model = "gpt-4o-mini-transcribe"
	}
	client := o.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
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
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := client.Do(req)
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
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("speech provider: status %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("speech provider: status %d %s", resp.StatusCode, out.Error.Message)
	}
	return strings.TrimSpace(out.Text), nil
}
