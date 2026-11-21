package speech

import (
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Slice 36: the default speech model is microsoft/mai-transcribe-2, heard
// on OpenRouter's dedicated speech-to-text route.
func TestSlice36_DefaultSpeechModelIsMaiTranscribe2(t *testing.T) {
	if DefaultOpenRouterModel != "microsoft/mai-transcribe-2" {
		t.Fatalf("default speech model %q", DefaultOpenRouterModel)
	}
	o := OpenRouter{APIKey: routerKey}
	if o.ModelName() != DefaultOpenRouterModel || o.Route() != RouteTranscriptions {
		t.Fatalf("default adapter: model %q route %q", o.ModelName(), o.Route())
	}
	if ModelOf(o) != DefaultOpenRouterModel || RouteOf(o) != RouteTranscriptions {
		t.Fatalf("ModelOf %q RouteOf %q", ModelOf(o), RouteOf(o))
	}
	if g := (OpenRouter{Model: "google/gemini-3.1-flash-lite"}); RouteOf(g) != RouteChat || ModelOf(&g) != "google/gemini-3.1-flash-lite" || RouteOf(&g) != RouteChat {
		t.Fatalf("chat model: %q %q", RouteOf(g), ModelOf(&g))
	}
	var nilRouter *OpenRouter
	for _, tc := range []struct {
		tr           Transcriber
		model, route string
	}{
		{Fake{}, "test-speech", "test"},
		{OpenAI{}, "gpt-4o-mini-transcribe", RouteTranscriptions},
		{OpenAI{Model: "gpt-4o-transcribe"}, "gpt-4o-transcribe", RouteTranscriptions},
		{nil, "", ""},
		{nilRouter, "", ""},
	} {
		if m, r := ModelOf(tc.tr), RouteOf(tc.tr); m != tc.model || r != tc.route {
			t.Errorf("%#v: model %q route %q; want %q %q", tc.tr, m, r, tc.model, tc.route)
		}
	}
}

// UsesTranscriptionsRoute splits dedicated speech-to-text slugs from
// audio-input chat models.
func TestSlice36_UsesTranscriptionsRoute(t *testing.T) {
	for model, want := range map[string]bool{
		"microsoft/mai-transcribe-2":           true,
		"microsoft/mai-transcribe-1.5":         true,
		"  Microsoft/MAI-Transcribe-2 ":        true,
		"openai/whisper-large-v3-turbo":        true,
		"openai/whisper-1":                     true,
		"openai/gpt-4o-mini-transcribe":        true,
		"openai/gpt-4o-transcribe":             true,
		"openai/gpt-transcribe":                true,
		"deepgram/nova-3":                      true,
		"x-ai/grok-stt-2.0":                    true,
		"mistralai/voxtral-mini-transcribe":    true,
		"mistralai/voxtral-small-24b-2507-stt": true,
		"nvidia/nemotron-3.5-transcribe":       true,
		"google/gemini-3.5-transcribe":         true,
		"google/gemini-2.5-flash":              false,
		"google/gemini-3.1-flash-lite":         false,
		"google/gemini-4-hydrogen":             false,
		"openai/gpt-4o-audio-preview":          false,
		"openai/gpt-6-luna":                    false,
		"anthropic/claude-sonnet-5.5":          false,
		"whisper-1":                            false, // not an OpenRouter slug
		"":                                     false,
		"deepgram/":                            false,
		"vendor/firststep-model":               false, // "stt" only as a word part
	} {
		if got := UsesTranscriptionsRoute(model); got != want {
			t.Errorf("UsesTranscriptionsRoute(%q) = %v, want %v", model, got, want)
		}
	}
}

// sttServer is a stand-in for POST /api/v1/audio/transcriptions that
// checks the request and answers with body.
func sttServer(t *testing.T, check func(r *http.Request, model, filename, ctype string, data []byte), status int, body string) (OpenRouter, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/audio/transcriptions" {
			t.Errorf("endpoint %s %s", r.Method, r.URL.Path)
		}
		mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mt != "multipart/form-data" || params["boundary"] == "" {
			t.Errorf("content type %q: %v", r.Header.Get("Content-Type"), err)
		}
		var model, filename, ctype string
		var data []byte
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("multipart: %v", err)
				break
			}
			b, _ := io.ReadAll(p)
			switch p.FormName() {
			case "model":
				model = string(b)
			case "file":
				filename, ctype, data = p.FileName(), p.Header.Get("Content-Type"), b
			default:
				t.Errorf("unexpected form field %q", p.FormName())
			}
		}
		if check != nil {
			check(r, model, filename, ctype, data)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return OpenRouter{APIKey: routerKey, BaseURL: srv.URL, HTTP: srv.Client()}, &calls
}

// The multipart upload: the same OPENROUTER_API_KEY and attribution
// headers as every other OpenRouter call, the model field, and the file
// named with the recording's own extension and audio type, for every
// format a browser recorder or Scry's WAV conversion produces.
func TestSlice36_TranscriptionsMultipartUpload(t *testing.T) {
	audio := []byte("RIFF\x24\x00\x00\x00WAVEfmt fake-audio-bytes\x00\xff")
	for _, tc := range []struct{ mime, file, ctype string }{
		{"audio/wav", "audio.wav", "audio/wav"},
		{"audio/webm;codecs=opus", "audio.webm", "audio/webm"},
		{"audio/mpeg", "audio.mp3", "audio/mpeg"},
		{"audio/ogg;codecs=opus", "audio.ogg", "audio/ogg"},
		{"audio/x-m4a", "audio.m4a", "audio/mp4"},
		{"audio/mp4", "audio.mp4", "audio/mp4"},
		{"", "audio.webm", "audio/webm"},
	} {
		t.Run(tc.file+"/"+tc.mime, func(t *testing.T) {
			o, calls := sttServer(t, func(r *http.Request, model, filename, ctype string, data []byte) {
				for k, want := range map[string]string{"Authorization": "Bearer " + routerKey, "HTTP-Referer": "https://scry.mistystep.io", "X-Title": "Scry"} {
					if r.Header.Get(k) != want {
						t.Errorf("header %s = %q", k, r.Header.Get(k))
					}
				}
				if model != DefaultOpenRouterModel || filename != tc.file || ctype != tc.ctype || string(data) != string(audio) {
					t.Errorf("upload model %q file %q type %q, %d bytes", model, filename, ctype, len(data))
				}
			}, 200, `{"text":"  The mitochondria.\n","usage":{"cost":0.0001385,"seconds":5}}`)
			text, cost, err := o.TranscribeMetered(context.Background(), Audio{Data: audio, MIME: tc.mime, Seconds: 5})
			if err != nil || text != "The mitochondria." || cost == nil || *cost != 139 || calls.Load() != 1 {
				t.Fatalf("%q cost %v err %v calls %d", text, cost, err, calls.Load())
			}
			if text, err := o.Transcribe(context.Background(), Audio{Data: audio, MIME: tc.mime}); err != nil || text != "The mitochondria." {
				t.Fatalf("Transcribe: %q %v", text, err)
			}
		})
	}
}

// SCRY_SPEECH_MODEL picks the route: another dedicated model is uploaded
// to the transcriptions route under its own name; a chat-audio model never
// reaches it.
func TestSlice36_SpeechModelOverrideRoutes(t *testing.T) {
	o, calls := sttServer(t, func(_ *http.Request, model, _, _ string, _ []byte) {
		if model != "openai/whisper-large-v3-turbo" {
			t.Errorf("model %q", model)
		}
	}, 200, `{"text":"heard"}`)
	o.Model = "openai/whisper-large-v3-turbo"
	if text, err := o.Transcribe(context.Background(), Audio{Data: []byte("x"), MIME: "audio/wav"}); err != nil || text != "heard" || calls.Load() != 1 {
		t.Fatalf("%q %v %d", text, err, calls.Load())
	}

	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"heard by chat"},"finish_reason":"stop"}],"usage":{"cost":0.00002}}`)
	}))
	defer srv.Close()
	chat := OpenRouter{APIKey: routerKey, Model: "google/gemini-3.1-flash-lite", BaseURL: srv.URL}
	text, cost, err := chat.TranscribeMetered(context.Background(), Audio{Data: []byte("x"), MIME: "audio/wav"})
	if err != nil || text != "heard by chat" || cost == nil || *cost != 20 || len(paths) != 1 || paths[0] != "/api/v1/chat/completions" {
		t.Fatalf("%q %v %v %v", text, cost, err, paths)
	}
	tr, err := NewTranscriber(routerKey, "", "")
	if err != nil || RouteOf(tr) != RouteTranscriptions || ModelOf(tr) != DefaultOpenRouterModel {
		t.Fatalf("NewTranscriber default: %#v %v", tr, err)
	}
	tr, err = NewTranscriber(routerKey, "", "google/gemini-4-hydrogen")
	if err != nil || RouteOf(tr) != RouteChat {
		t.Fatalf("NewTranscriber chat override: %#v %v", tr, err)
	}
}

// Every failure on the transcriptions route is an error that never
// carries the key, even when the provider echoes it; an empty transcript
// is nothing heard; empty audio never reaches the provider.
func TestSlice36_TranscriptionsFailuresAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"401 echoing key", `{"error":{"code":401,"message":"No auth credentials found for ` + routerKey + `"}}`, 401, "speech provider: status 401 No auth credentials found for [redacted]"},
		{"402 out of credit", `{"error":{"code":402,"message":"Insufficient credits"}}`, 402, "speech provider: status 402 Insufficient credits"},
		{"429", `{"error":{"code":429,"message":"rate limited"}}`, 429, "speech provider: status 429 rate limited"},
		{"error object on 200", `{"error":{"code":502,"message":"upstream"}}`, 200, "speech provider: status 200 upstream"},
		{"5xx not json", `<html>bad gateway</html>`, 502, "speech provider: status 502"},
		{"200 without text", `{"usage":{"cost":0.0001}}`, 200, "no transcript"},
		{"200 text null", `{"text":null}`, 200, "no transcript"},
		{"long message bounded", `{"error":{"message":"` + strings.Repeat("y", 5000) + `"}}`, 400, "speech provider: status 400 yyyy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, _ := sttServer(t, nil, tc.status, tc.body)
			_, _, err := o.TranscribeMetered(context.Background(), Audio{Data: []byte("x"), MIME: "audio/wav"})
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), routerKey) || len(err.Error()) > 300 {
				t.Fatalf("err %v, want %q", err, tc.want)
			}
		})
	}
	t.Run("a failed call still reports its cost", func(t *testing.T) {
		o, _ := sttServer(t, nil, 500, `{"error":{"message":"boom"},"usage":{"cost":0.0002}}`)
		_, cost, err := o.TranscribeMetered(context.Background(), Audio{Data: []byte("x"), MIME: "audio/wav"})
		if err == nil || cost == nil || *cost != 200 {
			t.Fatalf("cost %v err %v", cost, err)
		}
	})
	t.Run("empty transcript is nothing heard", func(t *testing.T) {
		for _, body := range []string{`{"text":""}`, `{"text":"   \n"}`} {
			o, _ := sttServer(t, nil, 200, body)
			if text, err := o.Transcribe(context.Background(), Audio{Data: []byte("x"), MIME: "audio/wav"}); err != nil || text != "" {
				t.Fatalf("%s: %q %v", body, text, err)
			}
		}
	})
	t.Run("insane cost is not believed", func(t *testing.T) {
		for _, body := range []string{`{"text":"a","usage":{"cost":-1}}`, `{"text":"a","usage":{"cost":1e9}}`, `{"text":"a","usage":{}}`, `{"text":"a"}`} {
			o, _ := sttServer(t, nil, 200, body)
			if _, cost, err := o.TranscribeMetered(context.Background(), Audio{Data: []byte("x"), MIME: "audio/wav"}); err != nil || cost != nil {
				t.Fatalf("%s: cost %v err %v", body, cost, err)
			}
		}
	})
	t.Run("empty audio makes no call", func(t *testing.T) {
		o, calls := sttServer(t, nil, 200, `{"text":"never"}`)
		for _, a := range []Audio{{}, {Data: []byte{}, MIME: "audio/wav"}} {
			if text, cost, err := o.TranscribeMetered(context.Background(), a); err != nil || text != "" || cost != nil {
				t.Fatalf("%q %v %v", text, cost, err)
			}
		}
		if calls.Load() != 0 {
			t.Fatal("empty audio reached the provider")
		}
	})
	t.Run("JWT-shaped key refused before any call", func(t *testing.T) {
		o, calls := sttServer(t, nil, 200, `{"text":"never"}`)
		o.APIKey = jwtKey
		if _, err := o.Transcribe(context.Background(), Audio{Data: []byte("x")}); err == nil || strings.Contains(err.Error(), jwtKey) || calls.Load() != 0 {
			t.Fatalf("%v calls %d", err, calls.Load())
		}
	})
	t.Run("redirect not followed", func(t *testing.T) {
		var leaked atomic.Int32
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
		defer other.Close()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
		}))
		defer srv.Close()
		o := OpenRouter{APIKey: routerKey, BaseURL: srv.URL}
		if _, err := o.Transcribe(context.Background(), Audio{Data: []byte("x"), MIME: "audio/wav"}); err == nil || leaked.Load() != 0 {
			t.Fatalf("err %v leaked %d", err, leaked.Load())
		}
	})
	t.Run("unreachable with key in URL is redacted", func(t *testing.T) {
		o := OpenRouter{APIKey: routerKey, BaseURL: "http://127.0.0.1:1/" + routerKey}
		if _, err := o.Transcribe(context.Background(), Audio{Data: []byte("x")}); err == nil || strings.Contains(err.Error(), routerKey) {
			t.Fatalf("err %v", err)
		}
	})
	t.Run("oversized response is bounded", func(t *testing.T) {
		o, _ := sttServer(t, nil, 200, `{"text":"`+strings.Repeat("a", 2<<20)+`"}`)
		if _, err := o.Transcribe(context.Background(), Audio{Data: []byte("x")}); err == nil {
			t.Fatal("unbounded read")
		}
	})
}
