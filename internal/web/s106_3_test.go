package web_test

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"scry/internal/deck"
)

// upload posts Create's "Import a deck" form like a browser: a multipart
// body with the CSRF token, the file and the optional idea title.
func (b *browser) upload(filename string, data []byte, idea string) (int, string) {
	b.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("csrf", b.csrf)
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write(data)
	_ = mw.WriteField("idea", idea)
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, b.w.srv.URL+"/learn/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return b.do(req)
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "decks", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// S106-3: Create shows "Import a deck" after a rule, linked from Library
// and from Settings → Your data, which offers Markdown, CSV and JSON.
func TestS106_3_ImportSectionAndDownloadsAreLinked(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("imp@example.com")
	b.addSet("nato-alphabet")
	has(t, b.mustGet("/learn"), `id="import"`, "Import a deck", "Anki .apkg, CSV or Markdown · up to 20 MiB",
		`enctype="multipart/form-data"`, `type="file"`, "Idea title", ">Import</button>")
	has(t, b.mustGet("/library"), `href="/learn#import">Create or import cards`)
	has(t, b.mustGet("/settings"), "Download Markdown", "Download CSV", "Download JSON",
		`href="/settings/export?format=md"`, `href="/settings/export?format=csv"`, `href="/settings/export?format=json"`,
		`href="/learn#import">Import a deck`)
}

// S106-3: an Anki package (basic and cloze notes), a CSV with a header and
// card-block Markdown each go straight into the library at no allowance
// cost, and Library opens on the imported idea with a plain count.
func TestS106_3_ImportApkgCSVMarkdownIntoLibrary(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("imp@example.com")
	us := w.lib("imp@example.com")
	before, err := us.ReservedMicros(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		file, idea, flash string
		want              []string
	}{
		{"sample.apkg", "", "Imported 4 cards.", nil},
		{"sample.csv", "", "Imported 5 cards.", []string{"Spanish basics"}},
		{"sample.md", "Memory", "Imported 3 cards.", []string{"Who first measured the forgetting curve?"}},
	} {
		status, body := b.upload(tc.file, fixture(t, tc.file), tc.idea)
		if status != http.StatusOK || !strings.HasPrefix(b.path, "/library/ideas/") {
			t.Fatalf("%s: status %d at %s\n%s", tc.file, status, b.path, body)
		}
		has(t, body, tc.flash)
		has(t, body, tc.want...)
	}
	if w.model.judgeCalls() != 0 || len(w.model.drafts) != 0 {
		t.Fatal("an import called the model")
	}
	after, _ := us.ReservedMicros(ctx)
	if after != before {
		t.Fatalf("an import reserved allowance: %d -> %d", before, after)
	}
	cards, err := us.DeckCards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 12 {
		t.Fatalf("library has %d cards, want 12", len(cards))
	}
	var exact int
	for _, c := range cards {
		if c.Exact {
			exact++
		}
		if strings.Contains(c.Question, "<") || strings.Contains(c.Answer, "<img") {
			t.Fatalf("HTML kept in %+v", c)
		}
	}
	if exact != 1 {
		t.Fatalf("%d exact cards, want 1", exact)
	}
}

// S106-3: unstructured Markdown goes to the drafting agent as a draft
// outline, never straight into the library.
func TestS106_3_UnstructuredMarkdownGoesToDrafting(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("imp@example.com")
	status, body := b.upload("notes.md", fixture(t, "notes.md"), "")
	w.app.WaitDrafts()
	if status != http.StatusOK || !strings.HasPrefix(b.path, "/learn/") {
		t.Fatalf("status %d at %s", status, b.path)
	}
	has(t, body, "Sent to drafting")
	if len(w.model.drafts) != 1 || !strings.Contains(w.model.drafts[0].Query, "hippocampus") {
		t.Fatalf("drafts = %+v", w.model.drafts)
	}
	cards, _ := w.lib("imp@example.com").DeckCards(ctx)
	if len(cards) != 0 {
		t.Fatalf("%d cards entered the library before approval", len(cards))
	}
	has(t, b.mustGet(b.path), "Approve all")
}

// S106-3: a file Scry can't read, an unknown kind, no file, or one over
// 20 MiB is refused in words on Create, and nothing is added.
func TestS106_3_ImportRefusedInWords(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("imp@example.com")
	b.mustGet("/learn")
	status, body := b.upload("deck.apkg", []byte("not a zip"), "")
	if status != http.StatusOK {
		t.Fatalf("bad apkg: %d", status)
	}
	has(t, body, "Import a deck", `class="field-error"`)
	status, body = b.upload("deck.pdf", []byte("%PDF"), "")
	if status != http.StatusOK || !strings.Contains(body, "field-error") {
		t.Fatalf("pdf: %d", status)
	}
	status, _ = b.upload("bad.csv", []byte("only-one-column\n"), "")
	if status != http.StatusOK {
		t.Fatalf("bad csv: %d", status)
	}
	// Over 20 MiB: refused before it is read, back on Create in words.
	big := bytes.Repeat([]byte("q,a\n"), (deck.MaxBytes/4)+(1<<19))
	status, body = b.upload("big.csv", big, "")
	if status != http.StatusOK || b.path != "/learn" {
		t.Fatalf("big file: %d at %s", status, b.path)
	}
	has(t, body, "larger than 20 MiB")
	if cards, _ := w.lib("imp@example.com").DeckCards(ctx); len(cards) != 0 {
		t.Fatalf("a refused import added %d cards", len(cards))
	}
	// Without the CSRF token nothing is imported.
	b.csrf = "wrong"
	if status, _ := b.upload("sample.csv", fixture(t, "sample.csv"), ""); status != http.StatusForbidden {
		t.Fatalf("no csrf: %d", status)
	}
}

// S106-3: Settings → Your data downloads everything as Markdown, CSV or
// JSON, and a Markdown or CSV export imports back with every idea,
// question, answer and exact flag intact.
func TestS106_3_ExportRoundTrip(t *testing.T) {
	w := newWorld(t)
	a := w.browser()
	a.signUp("a@example.com")
	a.addSet("nato-alphabet")
	a.mustGet("/learn")
	a.upload("sample.csv", fixture(t, "sample.csv"), "")
	w.addExactText("a@example.com", "Sonnet 18", "Shall I compare thee to a summer's day?\nThou art more lovely and more temperate:")
	want, err := w.lib("a@example.com").DeckCards(ctx)
	if err != nil {
		t.Fatal(err)
	}

	get := func(b *browser, path string) (string, http.Header) {
		t.Helper()
		resp, err := b.c.Get(w.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			t.Fatalf("%s = %d", path, resp.StatusCode)
		}
		return string(raw), resp.Header
	}
	md, h := get(a, "/settings/export?format=md")
	if !strings.HasPrefix(h.Get("Content-Disposition"), "attachment") || !strings.HasPrefix(md, deck.ExportHeading) {
		t.Fatalf("markdown export: %q %q", h.Get("Content-Disposition"), md[:40])
	}
	csvText, _ := get(a, "/settings/export?format=csv")
	if !strings.HasPrefix(csvText, "idea,question,answer,exact") {
		t.Fatalf("csv export starts %q", csvText[:30])
	}
	js, h := get(a, "/settings/export?format=json")
	if !strings.Contains(h.Get("Content-Type"), "json") || !strings.Contains(js, `"items"`) {
		t.Fatal("json export")
	}

	for _, f := range []struct{ name, body string }{{"scry-export.md", md}, {"scry-export.csv", csvText}} {
		email := "rt-" + strings.TrimPrefix(filepath.Ext(f.name), ".") + "@example.com"
		b := w.browser()
		b.signUp(email)
		b.mustGet("/learn")
		if status, body := b.upload(f.name, []byte(f.body), ""); status != 200 || !strings.Contains(body, "Imported") {
			t.Fatalf("%s re-import: %d at %s", f.name, status, b.path)
		}
		got, _ := w.lib(email).DeckCards(ctx)
		if len(got) != len(want) {
			t.Fatalf("%s: %d cards back, want %d", f.name, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s card %d: got %+v want %+v", f.name, i, got[i], want[i])
			}
		}
	}
}

// S106-3: the downloads work on any plan state, an ended plan included.
func TestS106_3_ExportOnAnyPlanState(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUpNoCard("ended@example.com")
	b.addSet("nato-alphabet")
	w.advance(9 * 24 * time.Hour) // the 7-day no-card trial is over
	if status, _ := b.get("/study"); b.path != "/billing" {
		t.Fatalf("study after the trial: %d at %s", status, b.path)
	}
	for _, f := range []string{"md", "csv", "json"} {
		resp, err := b.c.Get(w.srv.URL + "/settings/export?format=" + f)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(raw), "Alfa") {
			t.Fatalf("%s export on an ended plan: %d", f, resp.StatusCode)
		}
	}
}
