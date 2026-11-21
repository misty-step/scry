package speech

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	routerKey = "sk-or-v1-ROUTERstandin"
	// JWT-shaped stand-ins: header {"alg":"HS256"}, and one without the eyJ prefix.
	jwtKey       = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJzdGFuZGluIn0.c3RhbmRpbnNpZw"
	jwtNoPrefix  = "e30.eyJzdWIiOiJ4In0.c2ln" // e30 is base64url for {}
	jwtUnsigned  = "eyJhbGciOiJub25lIn0.eyJzdWIiOiJ4In0."
	notJWTDotted = "sk-proj.abc.def!" // dots, but not base64url segments
)

func routerServer(t *testing.T, h http.HandlerFunc) OpenRouter {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return OpenRouter{APIKey: routerKey, BaseURL: srv.URL, HTTP: srv.Client()}
}

// Slice 20: speech goes to OpenRouter's chat completions route as an
// input_audio block, base64-encoded, with a strict verbatim prompt.
func TestSlice20_OpenRouterTranscribeRequest(t *testing.T) {
	audio := []byte{0x1a, 0x45, 0xdf, 0xa3, 0xff, 0x00, 'w', 'e', 'b', 'm'}
	o := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/chat/completions" {
			t.Errorf("endpoint %s %s", r.Method, r.URL.Path)
		}
		for k, want := range map[string]string{"Authorization": "Bearer " + routerKey, "Content-Type": "application/json", "HTTP-Referer": "https://scry.mistystep.io", "X-Title": "Scry"} {
			if r.Header.Get(k) != want {
				t.Errorf("header %s = %q", k, r.Header.Get(k))
			}
		}
		var b struct {
			Model       string   `json:"model"`
			Temperature *float64 `json:"temperature"`
			MaxTokens   int      `json:"max_tokens"`
			Messages    []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			t.Fatal(err)
		}
		if b.Model != DefaultOpenRouterModel || b.Temperature == nil || *b.Temperature != 0 || b.MaxTokens != 512 || len(b.Messages) != 2 {
			t.Errorf("request %+v", b)
		}
		var system string
		_ = json.Unmarshal(b.Messages[0].Content, &system)
		if b.Messages[0].Role != "system" || !strings.Contains(system, "verbatim") || !strings.Contains(system, "empty string") || !strings.Contains(system, "no quotation marks") {
			t.Errorf("system prompt %q", system)
		}
		var parts []struct {
			Type       string `json:"type"`
			Text       string `json:"text"`
			InputAudio struct {
				Data   string `json:"data"`
				Format string `json:"format"`
			} `json:"input_audio"`
		}
		if err := json.Unmarshal(b.Messages[1].Content, &parts); err != nil || b.Messages[1].Role != "user" || len(parts) != 2 {
			t.Fatalf("user content %s %v", b.Messages[1].Content, err)
		}
		if parts[1].Type != "input_audio" || parts[1].InputAudio.Format != "webm" || parts[1].InputAudio.Data != base64.StdEncoding.EncodeToString(audio) {
			t.Errorf("audio block %+v", parts[1])
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"google/gemini-2.5-flash","choices":[{"message":{"role":"assistant","content":"  The mitochondria.\n"},"finish_reason":"stop"}],"usage":{"prompt_tokens":40,"completion_tokens":4,"cost":0.00001}}`)
	})
	if o.Name() != "openrouter" {
		t.Fatal(o.Name())
	}
	text, err := o.Transcribe(context.Background(), Audio{Data: audio, MIME: "audio/webm;codecs=opus", Seconds: 1.2})
	if err != nil || text != "The mitochondria." {
		t.Fatalf("%q %v", text, err)
	}
}

func TestSlice20_OpenRouterAudioFormat(t *testing.T) {
	for mime, want := range map[string]string{
		"audio/wav": "wav", "audio/x-wav": "wav", "audio/wave": "wav",
		"audio/mpeg": "mp3", "audio/mp3": "mp3",
		"audio/ogg": "ogg", "audio/ogg;codecs=opus": "ogg",
		"audio/mp4": "mp4", "audio/mp4;codecs=mp4a.40.2": "mp4", "audio/x-m4a": "m4a", "audio/m4a": "m4a",
		"audio/flac": "flac", "audio/aac": "aac",
		"audio/webm": "webm", "audio/webm;codecs=opus": "webm", "": "webm", "application/octet-stream": "webm", "AUDIO/WAV": "wav",
	} {
		if got := AudioFormat(mime); got != want {
			t.Errorf("AudioFormat(%q) = %q, want %q", mime, got, want)
		}
	}
}

// Empty audio is nothing heard: no call, no error, no panic.
func TestSlice20_EmptyAudioIsNothingHeard(t *testing.T) {
	var calls atomic.Int32
	h := func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }
	o := routerServer(t, h)
	for _, a := range []Audio{{}, {Data: []byte{}, MIME: "audio/webm"}, {Data: nil, Seconds: 3}} {
		if text, err := o.Transcribe(context.Background(), a); err != nil || text != "" {
			t.Fatalf("%q %v", text, err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(h))
	defer srv.Close()
	if text, err := (OpenAI{APIKey: "oai-standin", BaseURL: srv.URL}).Transcribe(context.Background(), Audio{}); err != nil || text != "" {
		t.Fatalf("%q %v", text, err)
	}
	if calls.Load() != 0 {
		t.Fatal("empty audio reached a provider")
	}
}

func TestSlice20_OpenRouterTranscriptCleanup(t *testing.T) {
	for content, want := range map[string]string{
		`"The mitochondria."`:            "The mitochondria.",
		`“Paris”`:                        "Paris",
		`""`:                             "",
		``:                               "",
		`He said "no" twice`:             `He said "no" twice`,
		`"one" and "two"`:                `"one" and "two"`,
		"  plain words with spaces  \n ": "plain words with spaces",
	} {
		o := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
			b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
			w.Write(b)
		})
		if got, err := o.Transcribe(context.Background(), Audio{Data: []byte("x"), MIME: "audio/wav"}); err != nil || got != want {
			t.Errorf("content %q: got %q (%v), want %q", content, got, err, want)
		}
	}
}

// Every failure is an error that never carries the key, even when the
// provider echoes it; redirects are never followed.
func TestSlice20_OpenRouterTranscribeFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"http error echoing key", `{"error":{"code":401,"message":"No auth credentials found for ` + routerKey + `"}}`, 401, "speech provider: status 401 No auth credentials found for [redacted]"},
		{"error object on 200", `{"error":{"code":502,"message":"upstream"}}`, 200, "speech provider: status 200 upstream"},
		{"not json", `<html>bad gateway</html>`, 502, "speech provider: status 502"},
		{"no choices", `{"choices":[]}`, 200, "no transcript"},
		{"content filter", `{"choices":[{"message":{"content":""},"finish_reason":"content_filter"}]}`, 200, "speech provider declined"},
		{"finish reason error with partial text", `{"choices":[{"message":{"content":"The capital of"},"finish_reason":"error"}]}`, 200, "speech provider failed before finishing the transcript"},
		{"finish reason error with no text", `{"choices":[{"message":{"content":null},"finish_reason":"error"}]}`, 200, "speech provider failed before finishing the transcript"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			_, err := o.Transcribe(context.Background(), Audio{Data: []byte("x"), MIME: "audio/ogg"})
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), routerKey) {
				t.Fatalf("err %v, want %q", err, tc.want)
			}
		})
	}
	t.Run("redirect not followed", func(t *testing.T) {
		var leaked atomic.Int32
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
		defer other.Close()
		o := routerServer(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) })
		if _, err := o.Transcribe(context.Background(), Audio{Data: []byte("x")}); err == nil || leaked.Load() != 0 {
			t.Fatalf("err %v leaked %d", err, leaked.Load())
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
		defer srv.Close()
		if _, err := (OpenAI{APIKey: "oai-standin", BaseURL: srv.URL}).Transcribe(context.Background(), Audio{Data: []byte("x")}); err == nil || leaked.Load() != 0 {
			t.Fatalf("openai err %v leaked %d", err, leaked.Load())
		}
	})
	t.Run("unreachable with key in URL is redacted", func(t *testing.T) {
		o := OpenRouter{APIKey: routerKey, BaseURL: "http://127.0.0.1:1/" + routerKey}
		if _, err := o.Transcribe(context.Background(), Audio{Data: []byte("x")}); err == nil || strings.Contains(err.Error(), routerKey) {
			t.Fatalf("err %v", err)
		}
	})
	t.Run("oversized response is bounded", func(t *testing.T) {
		o := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"`+strings.Repeat("a", 2<<20)+`"}}]}`)
		})
		if _, err := o.Transcribe(context.Background(), Audio{Data: []byte("x")}); err == nil {
			t.Fatal("unbounded read")
		}
	})
}

func TestSlice20_JWTShapedKeys(t *testing.T) {
	for _, k := range []string{jwtKey, jwtNoPrefix, jwtUnsigned, "  " + jwtKey + "\n"} {
		if !LooksLikeJWT(k) {
			t.Errorf("%q should look like a JWT", k)
		}
	}
	for _, k := range []string{routerKey, "sk-proj-abc_DEF-123", "oai-standin", notJWTDotted, "a.b", "a.b.c.d", ".a.b", "abc.def.ghi", ""} {
		if LooksLikeJWT(k) {
			t.Errorf("%q should not look like a JWT", k)
		}
	}
	for _, name := range []string{"OPENROUTER_API_KEY", "OPENAI_API_KEY"} {
		err := ValidateAPIKey(name, jwtKey)
		if err == nil || !strings.Contains(err.Error(), name+" is JWT-shaped") || strings.Contains(err.Error(), jwtKey) || strings.Contains(err.Error(), "eyJ") {
			t.Fatalf("%s: %v", name, err)
		}
		if err := ValidateAPIKey(name, ""); err == nil || !strings.Contains(err.Error(), name+" is missing") {
			t.Fatal(err)
		}
		if err := ValidateAPIKey(name, "sk-real-looking"); err != nil {
			t.Fatal(err)
		}
	}
}

// A JWT-shaped key is refused inside Transcribe as well as at boot, with
// no network call and no echo of the key.
func TestSlice20_TranscribeRefusesJWTShapedKeys(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	for _, tr := range []Transcriber{
		OpenRouter{APIKey: jwtKey, BaseURL: srv.URL},
		OpenAI{APIKey: jwtKey, BaseURL: srv.URL},
	} {
		_, err := tr.Transcribe(context.Background(), Audio{Data: []byte("x"), MIME: "audio/wav"})
		if err == nil || !strings.Contains(err.Error(), "JWT-shaped") || strings.Contains(err.Error(), jwtKey) {
			t.Fatalf("%s: %v", tr.Name(), err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("a JWT-shaped key reached a provider")
	}
}

// Slice 20: OpenRouter is preferred whenever its key is set; OpenAI is the
// direct fallback only without it; a JWT-shaped key on the chosen route is
// refused loudly.
func TestSlice20_NewTranscriberPrecedence(t *testing.T) {
	tr, err := NewTranscriber(routerKey, "oai-standin", "")
	if o, ok := tr.(OpenRouter); err != nil || !ok || o.APIKey != routerKey || o.Model != "" || tr.Name() != "openrouter" {
		t.Fatalf("%#v %v", tr, err)
	}
	tr, err = NewTranscriber(routerKey, jwtKey, "google/gemini-2.5-pro")
	if o, ok := tr.(OpenRouter); err != nil || !ok || o.Model != "google/gemini-2.5-pro" {
		t.Fatalf("an unused fallback key must not block OpenRouter: %#v %v", tr, err)
	}
	tr, err = NewTranscriber("", "oai-standin", "gpt-4o-transcribe")
	if o, ok := tr.(OpenAI); err != nil || !ok || o.APIKey != "oai-standin" || o.Model != "gpt-4o-transcribe" || tr.Name() != "openai" {
		t.Fatalf("%#v %v", tr, err)
	}
	for _, tc := range []struct{ router, openai, model, want string }{
		{jwtKey, "oai-standin", "", "OPENROUTER_API_KEY is JWT-shaped"},
		{"", jwtKey, "", "OPENAI_API_KEY is JWT-shaped"},
		{"", "", "", "set OPENROUTER_API_KEY or OPENAI_API_KEY"},
		{routerKey, "", "gpt-4o-mini-transcribe", "not an OpenRouter model slug"},
	} {
		tr, err := NewTranscriber(tc.router, tc.openai, tc.model)
		if err == nil || tr != nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "eyJ") {
			t.Fatalf("%+v: %v %v", tc, tr, err)
		}
	}
}

// The direct OpenAI fallback bounds and redacts its error messages.
func TestSlice20_OpenAIErrorRedactedAndBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprintf(w, `{"error":{"message":"Incorrect API key provided: oai-SECRETstandin %s"}}`, strings.Repeat("x", 1000))
	}))
	defer srv.Close()
	_, err := OpenAI{APIKey: "oai-SECRETstandin", BaseURL: srv.URL}.Transcribe(context.Background(), Audio{Data: []byte("x")})
	if err == nil || strings.Contains(err.Error(), "oai-SECRETstandin") || !strings.Contains(err.Error(), "[redacted]") || len(err.Error()) > 300 {
		t.Fatalf("%v", err)
	}
}

// SCRY_SPEECH_MODEL on OpenRouter goes through the same strict slug check
// as SCRY_DRAFT_MODEL and SCRY_EXPLAIN_MODEL (safeerr.IsRouterSlug):
// whitespace inside, a leading or trailing slash, or an empty segment is
// refused at boot, never sent to the provider.
func TestSlice20_NewTranscriberRejectsMalformedSlugs(t *testing.T) {
	for _, m := range []string{
		"google/gemini 2.5-flash", "google/\tgemini", "google /gemini", "google/gemini\u00a0flash",
		"/google/gemini-2.5-flash", "google/gemini-2.5-flash/", "/gemini", "google/", "google//gemini", "/", "gemini-2.5-flash",
	} {
		tr, err := NewTranscriber(routerKey, "", m)
		if err == nil || tr != nil || !strings.Contains(err.Error(), "not an OpenRouter model slug") || strings.Contains(err.Error(), routerKey) {
			t.Errorf("SCRY_SPEECH_MODEL=%q: %v %v", m, tr, err)
		}
	}
	for _, m := range []string{"google/gemini-2.5-flash", "openai/gpt-4o-audio-preview", "  google/gemini-2.5-pro  "} {
		if tr, err := NewTranscriber(routerKey, "", m); err != nil || tr.(OpenRouter).Model != strings.TrimSpace(m) {
			t.Errorf("SCRY_SPEECH_MODEL=%q: %v %v", m, tr, err)
		}
	}
	// The direct OpenAI fallback takes plain OpenAI model names.
	if tr, err := NewTranscriber("", "oai-standin", "gpt-4o-mini-transcribe"); err != nil || tr.Name() != "openai" {
		t.Fatalf("%v %v", tr, err)
	}
}
