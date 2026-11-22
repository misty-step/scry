package speech

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Slice 36 pass 2 (critic finding 3): dedicated speech-to-text models are
// sent to /api/v1/audio/transcriptions, including elevenlabs/scribe-v2,
// which the first cut sent to chat completions.
func TestSlice36_DedicatedSTTModelsUseTranscriptions(t *testing.T) {
	for model, want := range map[string]bool{
		"elevenlabs/scribe-v2":             true,
		"elevenlabs/scribe-v1":             true,
		"elevenlabs/anything-new":          true,
		"microsoft/mai-transcribe-2":       true,
		"openai/whisper-large-v3":          true,
		"openai/gpt-4o-mini-transcribe":    true,
		"deepgram/nova-3":                  true,
		"assemblyai/universal-2":           true,
		"mistralai/voxtral-small-24b-2507": true,
		"mistralai/voxtral-mini":           true,
		"vendor/fast-stt":                  true,
		"vendor/stt-v2":                    true,
		"vendor/model-stt-large":           true,
		"vendor/whisper-distil":            true,
		"vendor/scribe-pro":                true,
		"google/gemini-3.1-flash-lite":     false,
		"google/gemini-2.5-flash":          false,
		"mistralai/mistral-large":          false,
		"openai/gpt-4o-audio-preview":      false,
		"vendor/firststep-model":           false,
	} {
		if got := UsesTranscriptionsRoute(model); got != want {
			t.Errorf("UsesTranscriptionsRoute(%q) = %v, want %v", model, got, want)
		}
	}
	// And the request really goes there.
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		fmt.Fprint(w, `{"text":"four"}`)
	}))
	defer srv.Close()
	o := OpenRouter{APIKey: routerKey, Model: "elevenlabs/scribe-v2", BaseURL: srv.URL, HTTP: srv.Client()}
	if text, err := o.Transcribe(context.Background(), Audio{Data: []byte("RIFFwav"), MIME: "audio/wav"}); err != nil || text != "four" ||
		path != "/api/v1/audio/transcriptions" || RouteOf(o) != RouteTranscriptions {
		t.Fatalf("scribe-v2: %q %v at %s (route %s)", text, err, path, RouteOf(o))
	}
}

// Slice 36 pass 2 (critic finding 3): SCRY_SPEECH_ROUTE forces a route
// either way; "" and "auto" keep the name rule; anything else is refused.
func TestSlice36_SpeechRouteOverride(t *testing.T) {
	for _, tc := range []struct {
		env, model, want string
	}{
		{"", "google/gemini-3.1-flash-lite", RouteChat},
		{"auto", "elevenlabs/scribe-v2", RouteTranscriptions},
		{" AUTO ", "google/gemini-3.1-flash-lite", RouteChat},
		{"transcriptions", "google/gemini-3.1-flash-lite", RouteTranscriptions},
		{"chat", "microsoft/mai-transcribe-2", RouteChat},
		{"Chat", "", RouteChat},
	} {
		tr, err := NewTranscriberWithRoute(routerKey, "", tc.model, tc.env)
		if err != nil {
			t.Fatalf("%q: %v", tc.env, err)
		}
		if got := RouteOf(tr); got != tc.want {
			t.Errorf("SCRY_SPEECH_ROUTE=%q, model %q: route %s, want %s", tc.env, tc.model, got, tc.want)
		}
		// The adapter calls the route it reports.
		var path string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path = r.URL.Path
			if strings.HasSuffix(r.URL.Path, "/audio/transcriptions") {
				fmt.Fprint(w, `{"text":"four"}`)
				return
			}
			fmt.Fprint(w, `{"choices":[{"message":{"content":"four"},"finish_reason":"stop"}]}`)
		}))
		o := tr.(OpenRouter)
		o.BaseURL, o.HTTP = srv.URL, srv.Client()
		if text, err := o.Transcribe(context.Background(), Audio{Data: []byte("RIFFwav"), MIME: "audio/wav"}); err != nil || text != "four" || path != "/api/v1/"+tc.want {
			t.Errorf("SCRY_SPEECH_ROUTE=%q: %q %v at %s", tc.env, text, err, path)
		}
		srv.Close()
	}
	for _, bad := range []string{"multipart", "chat/completions", "audio/transcriptions", "both", "0"} {
		if _, err := ParseSpeechRoute(bad); err == nil || !strings.Contains(err.Error(), "SCRY_SPEECH_ROUTE") {
			t.Errorf("ParseSpeechRoute(%q) = %v; want a loud error", bad, err)
		}
		if _, err := NewTranscriberWithRoute(routerKey, "", "", bad); err == nil {
			t.Errorf("NewTranscriberWithRoute accepted SCRY_SPEECH_ROUTE=%q", bad)
		}
	}
	if _, err := NewTranscriberWithRoute("", "oai-standin", "", "chat"); err == nil || !strings.Contains(err.Error(), "needs OPENROUTER_API_KEY") {
		t.Fatalf("chat on the direct OpenAI route: %v", err)
	}
	if tr, err := NewTranscriberWithRoute("", "oai-standin", "", "transcriptions"); err != nil || RouteOf(tr) != RouteTranscriptions {
		t.Fatalf("transcriptions on the direct OpenAI route: %v", err)
	}
}

// Slice 36 pass 2 (critic finding 4): the key is redacted before a
// provider's message is bounded, on both OpenRouter routes and the direct
// OpenAI one. A key placed right across bounded's 200-rune cut leaves no
// fragment of itself, however long a prefix.
func TestSlice36_KeyAcrossTheBoundIsRedactedFirst(t *testing.T) {
	for _, offset := range []int{150, 190, 195, 199, 200} {
		for _, route := range []string{"transcriptions", "chat", "openai"} {
			key := routerKey
			if route == "openai" {
				key = "oai-SECRETstandinKEY0123456789"
			}
			msg := strings.Repeat("x", offset) + key + " trailing words"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(w, `{"error":{"message":%q}}`, msg)
			}))
			var err error
			audio := Audio{Data: []byte("RIFFwav"), MIME: "audio/wav"}
			switch route {
			case "openai":
				_, err = OpenAI{APIKey: key, BaseURL: srv.URL, HTTP: srv.Client()}.Transcribe(context.Background(), audio)
			default:
				o := OpenRouter{APIKey: key, Model: "microsoft/mai-transcribe-2", RouteOverride: route, BaseURL: srv.URL, HTTP: srv.Client()}
				_, err = o.Transcribe(context.Background(), audio)
			}
			srv.Close()
			if err == nil {
				t.Fatalf("%s offset %d: no error", route, offset)
			}
			got := err.Error()
			// No prefix of the key of four or more characters survives.
			for n := 4; n <= len(key); n++ {
				if strings.Contains(got, key[:n]) {
					t.Fatalf("%s offset %d: %d-character key fragment leaked: %q", route, offset, n, got)
				}
			}
			if offset+len("[redacted]") <= 200 && !strings.Contains(got, "[redacted]") {
				t.Fatalf("%s offset %d: key not redacted inside the bound: %q", route, offset, got)
			}
		}
	}
}
