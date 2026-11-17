package web_test

import (
	"bytes"
	"context"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"scry/internal/ai"
	"scry/internal/alert"
	"scry/internal/app"
	"scry/internal/auth"
	"scry/internal/billing"
	"scry/internal/speech"
	"scry/internal/store"
	"scry/internal/web"
)

// spyModel is the test-mode model with a notebook: it records what it was
// asked, and a test can script its next judgment or make it fail.
type spyModel struct {
	mu     sync.Mutex
	fake   ai.Fake
	judged []ai.JudgeInput
	drafts []ai.DraftInput
	script []ai.Judgment // judgments to return before falling back to the fake
	err    error
	panics bool
}

func (m *spyModel) Name() string { return "spy" }

func (m *spyModel) Judge(ctx context.Context, in ai.JudgeInput) (ai.Judgment, ai.Usage, error) {
	m.mu.Lock()
	m.judged = append(m.judged, in)
	err, panics := m.err, m.panics
	var scripted *ai.Judgment
	if len(m.script) > 0 {
		j := m.script[0]
		m.script = m.script[1:]
		scripted = &j
	}
	m.mu.Unlock()
	if panics {
		panic("the model adapter exploded")
	}
	if err != nil {
		return ai.Judgment{}, ai.Usage{}, err
	}
	j, use, ferr := m.fake.Judge(ctx, in)
	if scripted != nil {
		return *scripted, use, nil
	}
	return j, use, ferr
}

func (m *spyModel) Draft(ctx context.Context, in ai.DraftInput, sink ai.DraftSink) (ai.DraftResult, ai.Usage, error) {
	m.mu.Lock()
	m.drafts = append(m.drafts, in)
	err := m.err
	m.mu.Unlock()
	if err != nil {
		return ai.DraftResult{}, ai.Usage{}, err
	}
	return m.fake.Draft(ctx, in, sink)
}

func (m *spyModel) judgeCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.judged)
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// world is one running Scry with test-mode adapters and a clock the test
// controls.
type world struct {
	t      *testing.T
	app    *app.App
	store  *store.Store
	srv    *httptest.Server
	model  *spyModel
	alerts *alert.Recorder
	logs   *syncBuffer
	mu     sync.Mutex
	clock  time.Time
}

func newWorld(t *testing.T) *world {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "scry.db"))
	if err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, store: st, model: &spyModel{}, alerts: &alert.Recorder{}, logs: &syncBuffer{},
		clock: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)}
	cfg := app.DefaultConfig()
	cfg.TestMode = true
	log := slog.New(slog.NewJSONHandler(w.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	w.app = &app.App{
		Store: st, Auth: &auth.Local{DB: st.DB(), Cost: bcrypt.MinCost}, Pay: billing.Fake{}, Model: w.model,
		Speech: speech.Fake{}, Alert: w.alerts, Log: log, Now: w.now, Cfg: cfg,
	}
	w.srv = httptest.NewServer(web.New(w.app, log))
	t.Cleanup(func() {
		w.srv.Close()
		w.app.WaitDrafts()
		st.Close()
	})
	return w
}

func (w *world) now() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.clock
}

func (w *world) advance(d time.Duration) {
	w.mu.Lock()
	w.clock = w.clock.Add(d)
	w.mu.Unlock()
}

func (w *world) user(email string) store.User {
	w.t.Helper()
	u, err := w.store.UserByEmail(context.Background(), email)
	if err != nil {
		w.t.Fatalf("user %s: %v", email, err)
	}
	return u
}

func (w *world) lib(email string) *store.UserStore { return w.store.For(w.user(email).ID) }

// browser is one person's browser: cookies, and the CSRF token of the
// last page it saw.
type browser struct {
	t    *testing.T
	w    *world
	c    *http.Client
	csrf string
	path string // where the last request ended up
}

func (w *world) browser() *browser {
	jar, _ := cookiejar.New(nil)
	return &browser{t: w.t, w: w, c: &http.Client{Jar: jar}}
}

var csrfRE = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

func (b *browser) do(req *http.Request) (int, string) {
	b.t.Helper()
	resp, err := b.c.Do(req)
	if err != nil {
		b.t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)
	if m := csrfRE.FindStringSubmatch(body); m != nil {
		b.csrf = m[1]
	}
	b.path = resp.Request.URL.Path
	// Entities are decoded so tests can look for the words a person reads.
	return resp.StatusCode, html.UnescapeString(body)
}

func (b *browser) get(path string) (int, string) {
	b.t.Helper()
	req, _ := http.NewRequest(http.MethodGet, b.w.srv.URL+path, nil)
	return b.do(req)
}

func (b *browser) post(path string, kv ...string) (int, string) {
	b.t.Helper()
	form := url.Values{"csrf": {b.csrf}}
	for i := 0; i+1 < len(kv); i += 2 {
		form.Set(kv[i], kv[i+1])
	}
	req, _ := http.NewRequest(http.MethodPost, b.w.srv.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return b.do(req)
}

// mustGet fetches a page and fails unless it is a 200.
func (b *browser) mustGet(path string) string {
	b.t.Helper()
	status, body := b.get(path)
	if status != http.StatusOK {
		b.t.Fatalf("GET %s = %d (ended at %s)", path, status, b.path)
	}
	return body
}

const goodCard = "4242 4242 4242 4242"

// signUp creates an account and starts its trial with a working card.
func (b *browser) signUp(email string) {
	b.t.Helper()
	b.get("/signup")
	if status, _ := b.post("/signup", "email", email, "password", "correct horse"); status != 200 || b.path != "/signup/card" {
		b.t.Fatalf("sign-up for %s: status %d at %s", email, status, b.path)
	}
	if status, body := b.post("/signup/card", "number", goodCard, "expiry", "12/30", "cvc", "123"); status != 200 || b.path != "/shelf" {
		b.t.Fatalf("card step: status %d at %s\n%s", status, b.path, body)
	}
}

func (b *browser) addSet(slug string) {
	b.t.Helper()
	b.get("/shelf")
	if status, _ := b.post("/shelf/add", "slug", slug); status != 200 {
		b.t.Fatalf("add %s: %d", slug, status)
	}
}

var (
	itemRE   = regexp.MustCompile(`name="item_id" value="(\d+)"`)
	promptRE = regexp.MustCompile(`(?s)<h1 class="prompt[^"]*"[^>]*>(.*?)</h1>`)
	countRE  = regexp.MustCompile(`<span class="margin-count">(\d+) / (\d+)</span>`)
)

// current reads the item waiting for an answer off the study page.
func (b *browser) current(body string) (store.Item, bool) {
	b.t.Helper()
	m := itemRE.FindStringSubmatch(body)
	if m == nil {
		return store.Item{}, false
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	u, _, err := b.w.app.SessionUser(context.Background(), b.cookie())
	if err != nil {
		b.t.Fatal(err)
	}
	it, err := b.w.store.For(u.ID).Item(context.Background(), id)
	if err != nil {
		b.t.Fatal(err)
	}
	return it, true
}

func (b *browser) cookie() string {
	u, _ := url.Parse(b.w.srv.URL)
	for _, c := range b.c.Jar.Cookies(u) {
		if c.Name == "scry_session" {
			return c.Value
		}
	}
	return ""
}

// answer submits an answer to the current item and returns the page that
// follows.
func (b *browser) answer(item store.Item, text string, kv ...string) string {
	b.t.Helper()
	args := append([]string{"item_id", strconv.FormatInt(item.ID, 10), "answer", text}, kv...)
	_, body := b.post("/study/answer", args...)
	return body
}

// next presses Enter on a judgment.
func (b *browser) next() string {
	b.t.Helper()
	_, body := b.post("/study/next")
	return body
}

// finish works through the open session, answering every item with the
// answer given by pick, and returns the page shown at the end.
func (b *browser) finish(pick func(store.Item) string) string {
	b.t.Helper()
	body := b.mustGet("/study")
	for i := 0; i < 200; i++ {
		it, ok := b.current(body)
		if !ok {
			if strings.Contains(body, "data-next") {
				body = b.next()
				continue
			}
			return body
		}
		body = b.answer(it, pick(it))
	}
	b.t.Fatal("session never ended")
	return ""
}

func has(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("page is missing %q", want)
		}
	}
}

func hasNot(t *testing.T, body string, nots ...string) {
	t.Helper()
	for _, not := range nots {
		if strings.Contains(body, not) {
			t.Errorf("page should not contain %q", not)
		}
	}
}
