package generation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/misty-step/scry/internal/store"
)

func TestExaSearchAndContents(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.Header.Get("x-api-key") != "test-key" {
			t.Error("missing authenticated POST")
		}
		switch r.URL.Path {
		case "/search":
			var request struct {
				Query      string `json:"query"`
				Type       string `json:"type"`
				NumResults int    `json:"numResults"`
				Contents   struct {
					Text struct {
						MaxCharacters int `json:"maxCharacters"`
					} `json:"text"`
				} `json:"contents"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if len([]rune(request.Query)) != 400 || request.Type != "auto" || request.NumResults != 6 || request.Contents.Text.MaxCharacters != 4000 {
				t.Errorf("search shape: %+v", request)
			}
			fmt.Fprint(w, `{"results":[{"url":"https://example.test/a","title":"Title\u0001","publishedDate":"2026","text":"Useful\u0000 excerpt"},{"text":" \t "}],"costDollars":{"total":0.0000011}}`)
		case "/contents":
			var request struct {
				URLs []string `json:"urls"`
				Text struct {
					MaxCharacters int `json:"maxCharacters"`
				} `json:"text"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if len(request.URLs) != 1 || request.URLs[0] != "https://example.test/a" || request.Text.MaxCharacters != 30000 {
				t.Errorf("contents shape: %+v", request)
			}
			fmt.Fprint(w, `{"results":[{"url":"https://example.test/a","text":"Page text"}]}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := newExaClient(server.URL, "test-key", nil)
	if err != nil {
		t.Fatal(err)
	}
	docs, cost, err := client.research(context.Background(), "topic", strings.Repeat("x", 450))
	if err != nil || cost == nil || *cost != 2 || len(docs) != 1 || docs[0].Kind != "search_result" || docs[0].Provider != "exa" || docs[0].Text != "Useful excerpt" || docs[0].Title != "Title" {
		t.Fatalf("search: %+v, %v, %v", docs, cost, err)
	}
	docs, cost, err = client.research(context.Background(), "link", "https://example.test/a")
	if err != nil || cost != nil || len(docs) != 1 || docs[0].Kind != "page" {
		t.Fatalf("contents: %+v, %v, %v", docs, cost, err)
	}
	if calls != 2 {
		t.Fatalf("expected one paid transmission per research attempt, got %d", calls)
	}
}

func TestExaMissingKeyAndUnsafeResponses(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			http.Redirect(w, r, "/other", http.StatusFound)
		case 2:
			w.Write([]byte(strings.Repeat("x", exaResponseLimit+1)))
		}
	}))
	defer server.Close()
	client, _ := newExaClient(server.URL, "", nil)
	docs, cost, err := client.research(context.Background(), "topic", "learning")
	if err != nil || len(docs) != 0 || cost == nil || *cost != 0 || calls != 0 {
		t.Fatalf("missing key sent or charged: %+v %v %v", docs, cost, err)
	}
	client, _ = newExaClient(server.URL, "test", nil)
	if _, _, err = client.research(context.Background(), "topic", "learning"); err == nil || calls != 1 {
		t.Fatal("redirect should not be followed")
	}
	if _, _, err = client.research(context.Background(), "topic", "learning"); err == nil || calls != 2 {
		t.Fatal("oversized response accepted")
	}
	for _, endpoint := range []string{"http://example.com", "https://user:secret@example.com", "https://example.com/?key=secret"} {
		if ValidateExaEndpoint(endpoint) == nil {
			t.Errorf("unsafe endpoint accepted: %s", endpoint)
		}
	}
}

func TestResearchWithoutKeyDistinguishesTopicAndUnreadableLink(t *testing.T) {
	cfg := localConfig("http://127.0.0.1:1")
	worker := New(nil, cfg)
	topic := &store.Job{Kind: "research", SourceMode: "topic", SourceText: "cell energy"}
	result, cost, failure := worker.generateV5(context.Background(), topic, store.JobContext{})
	if failure != nil || cost == nil || *cost != 0 || len(result.Documents) != 0 || !strings.Contains(result.Note, "unavailable") {
		t.Fatalf("topic should continue honestly without web access: %+v %v %+v", result, cost, failure)
	}
	link := &store.Job{Kind: "research", SourceMode: "link", SourceText: "https://example.test/page"}
	_, cost, failure = worker.generateV5(context.Background(), link, store.JobContext{})
	if failure == nil || cost == nil || *cost != 0 || failure.retry || !strings.Contains(failure.message, "Paste its text") {
		t.Fatalf("unreadable link was incorrectly completed: cost=%v failure=%+v", cost, failure)
	}
}

// US-005: only an explicitly captured Topic is searched and only a chosen Link
// is read. My text, a photo, or unlabeled legacy material never reaches web
// research, even when it is configured.
func TestResearchNeverSendsNonWebMaterialUS005(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	cfg := localConfig("http://127.0.0.1:1")
	cfg.ExaEndpoint, cfg.ExaAPIKey = server.URL, "test-key"
	worker := New(nil, cfg)
	for _, mode := range []string{"text", "photo", "", "unknown"} {
		job := &store.Job{Kind: "research", SourceMode: mode, SourceKind: "source", SourceText: "PRIVATE pasted notes"}
		_, cost, failure := worker.generateV5(context.Background(), job, store.JobContext{})
		if failure == nil || failure.retry || cost == nil || *cost != 0 {
			t.Fatalf("mode %q research was not refused at zero cost: cost=%v failure=%+v", mode, cost, failure)
		}
	}
	if calls != 0 {
		t.Fatalf("non-web material reached web research %d times", calls)
	}
}

func TestExaErrorRetainsReportedCostAndDropsUnsafeLinks(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"costDollars":{"total":0.0000011}}`)
			return
		}
		fmt.Fprint(w, `{"results":[{"url":"javascript:alert(1)","text":"Unsafe document"},{"url":"https://example.test/safe","text":"Safe excerpt"}],"costDollars":{"total":0}}`)
	}))
	defer server.Close()
	client, err := newExaClient(server.URL, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, cost, err := client.research(context.Background(), "topic", "learning")
	if err == nil || cost == nil || *cost != 2 {
		t.Fatalf("error response cost lost: cost=%v err=%v", cost, err)
	}
	docs, cost, err := client.research(context.Background(), "topic", "learning")
	if err != nil || cost == nil || *cost != 0 || len(docs) != 1 || docs[0].URL != "https://example.test/safe" {
		t.Fatalf("unsafe result was published: %+v %v %v", docs, cost, err)
	}
}
