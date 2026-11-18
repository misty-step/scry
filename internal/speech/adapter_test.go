package speech

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestG11_SpeechMultipartUpload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" || r.Header.Get("Authorization") != "Bearer local-test-key" {
			t.Error("wrong endpoint/auth")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		defer r.MultipartForm.RemoveAll()
		if r.FormValue("model") != "gpt-4o-mini-transcribe" || r.FormValue("response_format") != "json" {
			t.Error("wrong fields")
		}
		f, h, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		if string(b) != "test audio" || h.Filename != "answer.wav" {
			t.Error("wrong audio")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"text":" heard words "}`))
	}))
	defer srv.Close()
	o := OpenAI{APIKey: "local-test-key", BaseURL: srv.URL, HTTP: srv.Client()}
	text, err := o.Transcribe(context.Background(), Audio{Data: []byte("test audio"), MIME: "audio/wav"})
	if err != nil || text != "heard words" {
		t.Fatalf("%q %v", text, err)
	}
}
