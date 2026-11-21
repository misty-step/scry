package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// All tests here are offline: no key, no network beyond httptest.

func mustLoad(t *testing.T) *Datasets {
	t.Helper()
	d, err := LoadDatasets()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestGradingDatasetMix(t *testing.T) {
	d := mustLoad(t)
	if len(d.Grading) != 25 {
		t.Fatalf("grading cases = %d, want 25", len(d.Grading))
	}
	want := map[string]int{"exact": 5, "paraphrase": 5, "typo": 3, "adversarial_overlap": 5, "incomplete": 4, "negation_injection": 3}
	got := map[string]int{}
	for _, c := range d.Grading {
		got[c.Category]++
		ok := map[string][]string{"exact": {"correct"}, "paraphrase": {"correct"}, "typo": {"correct", "close"}, "adversarial_overlap": {"incorrect"}, "incomplete": {"close"}, "negation_injection": {"incorrect"}}[c.Category]
		if !oneOf(c.Label, ok...) {
			t.Errorf("%s: category %s has label %s", c.ID, c.Category, c.Label)
		}
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("category %s = %d, want %d", k, got[k], n)
		}
	}
}

func TestExplainerDataset(t *testing.T) {
	d := mustLoad(t)
	if len(d.Explainer) != 15 {
		t.Fatalf("explainer cases = %d, want 15", len(d.Explainer))
	}
	domains := map[string]int{}
	for _, c := range d.Explainer {
		domains[c.Domain]++
		// The expected answer plus the misconception must cover every required concept, or the rubric is unfair.
		if r := ScoreExplanation(c, c.Expected+" "+c.KeyMisconception); r.ConceptsCovered != r.ConceptsTotal {
			t.Errorf("%s: expected answer and misconception miss concepts %v", c.ID, r.MissingConcepts)
		}
	}
	for _, dom := range []string{"STEM", "history", "language", "code", "finance"} {
		if domains[dom] != 3 {
			t.Errorf("domain %s = %d, want 3", dom, domains[dom])
		}
	}
}

func TestDraftingDataset(t *testing.T) {
	d := mustLoad(t)
	if len(d.Drafting) != 10 {
		t.Fatalf("drafting cases = %d, want 10", len(d.Drafting))
	}
	cats := map[string]int{}
	books := ""
	for _, c := range d.Drafting {
		cats[c.Category]++
		if (c.Category == "book") != (c.ExpectedKind == "book") {
			t.Errorf("%s: category %s with expected kind %s", c.ID, c.Category, c.ExpectedKind)
		}
		if c.Category == "book" {
			books += c.Query + "\n"
		}
	}
	if cats["note"] != 5 || cats["topic"] != 3 || cats["book"] != 2 {
		t.Errorf("categories = %v, want 5 note, 3 topic, 2 book", cats)
	}
	for _, b := range []string{"Thinking in Systems by Donella Meadows", "The Art of Doing Science and Engineering by Richard Hamming"} {
		if !strings.Contains(books, b) {
			t.Errorf("missing book %q", b)
		}
	}
}

func TestSpeechFixtures(t *testing.T) {
	d := mustLoad(t)
	if len(d.Speech) != 12 {
		t.Fatalf("clips = %d, want 12", len(d.Speech))
	}
	grades := map[string]int{}
	total := 0
	for i, c := range d.Speech {
		grades[c.ExpectedGrade]++
		total += len(c.Audio)
		if c.File != fmt.Sprintf("clip-%02d.mp3", i+1) {
			t.Errorf("clip %d is %s", i+1, c.File)
		}
		// An MP3 starts with an ID3 tag or a frame sync.
		if !(bytes.HasPrefix(c.Audio, []byte("ID3")) || (c.Audio[0] == 0xFF && c.Audio[1]&0xE0 == 0xE0)) {
			t.Errorf("%s is not an MP3", c.File)
		}
		for _, term := range c.CriticalTerms {
			if !TermPresent(term, c.SpokenText) {
				t.Errorf("%s: term %q is not in its own spoken text", c.ID, term)
			}
		}
		if WER(c.SpokenText, c.SpokenText) != 0 {
			t.Errorf("%s: self WER not 0", c.ID)
		}
	}
	if grades["correct"] != 8 || grades["incorrect"] != 4 {
		t.Errorf("grades = %v, want 8 correct and 4 incorrect", grades)
	}
	if total > 300<<10 {
		t.Errorf("audio fixtures total %d bytes, want < 300 KiB", total)
	}
}

func TestWER(t *testing.T) {
	cases := []struct {
		ref, hyp string
		want     float64
	}{
		{"the cat sat", "the cat sat", 0},
		{"The cat sat.", "the CAT, sat!", 0},
		{"the cat sat", "the cat sat down", 1.0 / 3},
		{"the cat sat", "the dog sat", 1.0 / 3},
		{"the cat sat", "cat", 2.0 / 3},
		{"About 113 grams, or 8 tablespoons.", "about 113 grams or eight tablespoons", 0},
		{"Gödel's theorem", "Godels theorem", 0},
		{"can't stop", "cant stop", 0},
		{"multiversion concurrency control", "Multi-Version Concurrency Control", 0},
		{"", "", 0},
		{"", "noise", 1},
		{"a b", "", 1},
	}
	for _, c := range cases {
		if got := WER(c.ref, c.hyp); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("WER(%q, %q) = %v, want %v", c.ref, c.hyp, got, c.want)
		}
	}
}

func TestTermPresent(t *testing.T) {
	cases := []struct {
		term, hyp string
		want      bool
	}{
		{"MVCC", "PostgreSQL uses M.V.C.C. for this", true},
		{"MVCC", "PostgreSQL uses MVC", false},
		{"ATP synthase", "mitochondrial atp synthase driven by", true},
		{"40", "from 40 to 140 degrees", true},
		{"40", "from 140 degrees", false},
		{"Gödel", "godel's theorem", true}, // "godels" holds the term (compact match, 4+ characters)
		{"Gödel", "Godel said", true},
		{"8 tablespoons", "or eight tablespoons", true},
		{"n log n", "it's O(n log n)", true},
	}
	for _, c := range cases {
		if got := TermPresent(c.term, c.hyp); got != c.want {
			t.Errorf("TermPresent(%q, %q) = %v, want %v", c.term, c.hyp, got, c.want)
		}
	}
}

func TestPercentile(t *testing.T) {
	xs := []float64{5, 1, 4, 2, 3, 10, 9, 8, 7, 6}
	if p := Percentile(xs, .5); p != 5 {
		t.Errorf("p50 = %v", p)
	}
	if p := Percentile(xs, .95); p != 10 {
		t.Errorf("p95 = %v", p)
	}
	if Percentile(nil, .5) != 0 || xs[0] != 5 {
		t.Error("empty input or input mutated")
	}
}

func TestExplainerRubric(t *testing.T) {
	c := ExplainerCase{ID: "x", Question: "Why does ice float?", Expected: "Ice is less dense than water because hydrogen bonds hold an open lattice.", LearnerAnswer: "Because of trapped air bubbles.", Verdict: "missed", RequiredConcepts: []string{"dens", "hydrogen bond|lattice"}}
	good := "Trapped air isn't the reason: ice floats because hydrogen bonds lock its molecules into an open lattice, making it less dense than liquid water."
	r := ScoreExplanation(c, good)
	if r.Score != 100 || !r.VerdictRespected || !r.AddressesLearner || r.ConceptsCovered != 2 {
		t.Fatalf("good explanation scored %+v", r)
	}
	if r := ScoreExplanation(c, "You're right! "+good); r.VerdictRespected || r.Score != 70 {
		t.Errorf("verdict reversal not caught: %+v", r)
	}
	if r := ScoreExplanation(c, "You're right that air matters a little, but "+good); !r.VerdictRespected {
		t.Errorf("a partial concession is not a verdict change: %+v", r)
	}
	if r := ScoreExplanation(c, "You're not right: "+good); !r.VerdictRespected {
		t.Errorf("negated claim flagged: %+v", r)
	}
	if r := ScoreExplanation(c, "Sure! Here is why: "+good); r.NoPreamble {
		t.Error("preamble not caught")
	}
	if r := ScoreExplanation(c, "**Density**: "+good); r.NoMarkdown {
		t.Error("markdown not caught")
	}
	if r := ScoreExplanation(c, "- air: no\n- "+good); r.NoBullets {
		t.Error("bullets not caught")
	}
	long := strings.Repeat("word ", 50) + good
	if r := ScoreExplanation(c, long); r.Within40Words || !r.Within80Words || r.Within280Chars {
		t.Errorf("length bands wrong: %+v", r)
	}
	if r := ScoreExplanation(c, "Ice floats because of its structure."); r.ConceptsCovered != 0 || r.AddressesLearner {
		t.Errorf("coverage wrong: %+v", r)
	}
	if r := ScoreExplanation(c, ""); r.Score != 0 {
		t.Error("empty text must score 0")
	}
}

func draftFixture(lines ...string) string { return strings.Join(lines, "\n") }

const (
	headOK = `{"kind":"topic","title":"Raft consensus"}`
	doneOK = `{"done":true,"note":"Pitched at an on-call engineer."}`
)

func idea(title, link string) string {
	return fmt.Sprintf(`{"idea":{"title":%q,"summary":"Raft elects one leader per term.","links":[%q],"items":[{"q":"How does Raft avoid split votes in leader elections?","a":"Randomized election timeouts."},{"q":"When is a Raft log entry committed?","a":"Once a majority of servers store it."}]}}`, title, link)
}

func TestCheckDraft(t *testing.T) {
	c := DraftingCase{ExpectedKind: "topic", Learner: DraftLearner{Knows: []string{"Paxos basics", "Quorum intersection"}}}
	valid := draftFixture(headOK, idea("Leader election", "Quorum intersection"), idea("Log replication", "Leader election"), idea("Commit rule", "Paxos basics"), idea("Terms as clocks", "Commit rule"), doneOK)
	d := CheckDraft(c, valid)
	if !d.SchemaValid || d.QualityScore != 100 || d.IdeaCount != 4 || d.LibraryLinks != 2 || !d.KindMatches {
		t.Fatalf("valid draft: %+v", d)
	}
	if d := CheckDraft(c, "```json\n"+valid+"\n```"); d.SchemaValid || d.MalformedLines != 3 { // two fences, and the head is no longer first
		t.Errorf("code fences: %+v", d.MalformedLines)
	}
	if d := CheckDraft(c, draftFixture(headOK, idea("Leader election", "x"), idea("b", "x"), idea("c", "x"), doneOK)); d.SchemaValid || d.IdeasInRange {
		t.Error("3 ideas accepted")
	}
	if d := CheckDraft(c, strings.Replace(valid, `"Leader election","summary"`, `"Raft: leader election","summary"`, 1)); d.SchemaValid {
		t.Error("colon in title accepted")
	}
	if d := CheckDraft(c, strings.TrimSuffix(valid, "\n"+doneOK)); d.SchemaValid || d.DoneLast {
		t.Error("missing done line accepted")
	}
	if d := CheckDraft(c, strings.Replace(valid, `"links":["Paxos basics"]`, `"links":["paxos basics"]`, 1)); d.LinkValidRate == 1 || len(d.MiscopiedLinks) != 1 {
		t.Errorf("miscopied link not caught: %+v", d.MiscopiedLinks)
	}
	if d := CheckDraft(c, strings.Replace(valid, `"title":"Commit rule"`, `"title":"Paxos basic"`, 1)); d.NonDuplicateRate == 1 {
		t.Error("duplicate of learner.knows not caught")
	}
	if d := CheckDraft(c, `{"nothing":"Not a learnable topic."}`); d.SchemaValid || d.Nothing == "" {
		t.Error("nothing reply accepted")
	}
	if !YesNoQuestion("Is Raft a consensus algorithm?") || YesNoQuestion("What does Raft elect?") {
		t.Error("yes/no detection")
	}
	if StandaloneQuestion("What does it elect?") || StandaloneQuestion("According to the passage, what is committed?") || !StandaloneQuestion("What does Raft elect?") {
		t.Error("standalone detection")
	}
}

func TestDraftingInterleave(t *testing.T) {
	d := mustLoad(t)
	var got []string
	for _, c := range interleave(d.Drafting) {
		got = append(got, c.Category)
	}
	want := "note topic book note topic book note topic note note"
	if strings.Join(got, " ") != want {
		t.Errorf("interleave = %v", got)
	}
	if len(interleave(d.Drafting)) != len(d.Drafting) {
		t.Error("interleave lost cases")
	}
}

func TestBlindShuffleDeterministicAndSealed(t *testing.T) {
	models := []string{"a/one", "b/two", "c/three", "d/four", "e/five"}
	first := BlindAssign("seed", "explainer", "e01", models)
	again := BlindAssign("seed", "explainer", "e01", []string{"e/five", "d/four", "c/three", "b/two", "a/one"})
	for k, v := range first {
		if again[k] != v {
			t.Fatalf("not deterministic / leaks input order: %v vs %v", first, again)
		}
	}
	seen := map[string]bool{}
	for _, l := range BlindLabels(5) {
		seen[first[l]] = true
	}
	if len(first) != 5 || len(seen) != 5 {
		t.Fatalf("not a bijection: %v", first)
	}
	distinct := map[string]bool{}
	for i := 1; i <= 15; i++ {
		a := BlindAssign("seed", "explainer", fmt.Sprintf("e%02d", i), models)
		distinct[a["Model A"]+a["Model B"]+a["Model C"]] = true
	}
	if len(distinct) < 5 {
		t.Errorf("shuffle barely varies across cases: %d distinct", len(distinct))
	}
	if other := BlindAssign("other", "explainer", "e01", models); fmt.Sprint(other) == fmt.Sprint(first) && fmt.Sprint(BlindAssign("other2", "explainer", "e01", models)) == fmt.Sprint(first) {
		t.Error("seed has no effect")
	}
	set, key := BuildBlind("seed", "explainer", "e01", nil, models, map[string]BlindOutput{"a/one": {Status: "ok", Output: "text-from-a"}})
	for _, o := range set.Outputs {
		if strings.Contains(fmt.Sprint(o), "/") {
			t.Errorf("blind output leaks a slug: %v", o)
		}
		if o.Output == "text-from-a" && key[o.ModelLabel] != "a/one" {
			t.Errorf("key does not trace %s back to a/one", o.ModelLabel)
		}
	}
}

func TestBudgetGuard(t *testing.T) {
	b := &Budget{Max: 1.0}
	t1, err := b.Reserve(0.4)
	if err != nil {
		t.Fatal(err)
	}
	t2, err := b.Reserve(0.4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Reserve(0.4); err != ErrBudget {
		t.Fatal("in-flight reservations must count against the cap")
	}
	c := 0.1
	t1.Settle(&c)
	t2.Settle(nil) // unknown cost is charged its estimate
	s := b.State()
	if math.Abs(s.ChargedUSD-0.5) > 1e-9 || math.Abs(s.ExactUSD-0.1) > 1e-9 || s.UnknownCostCalls != 1 || s.RefusedForBudget != 1 {
		t.Fatalf("state %+v", s)
	}
	if _, err := b.Reserve(0.6); err == nil {
		t.Fatal("0.5 + 0.6 > 1.0 must be refused")
	}
	if _, err := b.Reserve(0.5); err != nil {
		t.Fatal("0.5 + 0.5 = cap is allowed")
	}
}

func TestGateDecision(t *testing.T) {
	f := func(x float64) *float64 { return &x }
	probs := func(c, a, i, n float64) map[string]float64 {
		return map[string]float64{"correct": c, "almost correct": a, "incorrect": i, "none_of_these": n}
	}
	cases := []struct {
		choice string
		conf   *float64
		probs  map[string]float64
		want   string
	}{
		{"correct", f(0.9), probs(0.9, 0.05, 0.05, 0), "correct"},
		{"almost correct", nil, nil, "close"},
		{"incorrect", f(0.4), nil, "abstain"},
		{"correct", nil, probs(0.7, 0.3, 0, 0), "abstain"},
		{"correct", nil, probs(0.8, 0.0, 0.0, 0.1), "abstain"}, // sums to 0.9
		{"none_of_these", nil, probs(0, 0, 0, 1), "abstain"},
		{"correct", nil, map[string]float64{"correct": 1}, "abstain"},
	}
	for _, c := range cases {
		if got := gateDecision(c.choice, c.conf, c.probs); got != c.want {
			t.Errorf("gate(%s, %v, %v) = %s, want %s", c.choice, c.conf, c.probs, got, c.want)
		}
	}
}

// The direct-audio track mirrors Jev's grading contract; fail if it drifts.
func TestJevMirrorsMatchProduction(t *testing.T) {
	src, err := os.ReadFile("../internal/ai/jev.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	if !strings.Contains(s, "const jevInstructions = "+strconv.Quote(jevInstructionsMirror)) {
		t.Error("jevInstructionsMirror differs from internal/ai/jev.go")
	}
	for name, v := range map[string]float64{"minJevConfidence": minJevConfidenceMirror, "minJevProbability": minJevProbabilityMirror, "minJevMargin": minJevMarginMirror} {
		m := regexp.MustCompile(`const ` + name + ` = ([0-9.]+)`).FindStringSubmatch(s)
		if m == nil || m[1] != strconv.FormatFloat(v, 'f', -1, 64) {
			t.Errorf("%s mirror %v differs from source %v", name, v, m)
		}
	}
	for _, c := range jevCriteriaMirror {
		if !strings.Contains(s, "{"+strconv.Quote(c.ID)+", ") {
			t.Errorf("criterion %q not in jev.go", c.ID)
		}
	}
}

// fakeRouter is a stand-in for OpenRouter's routes. It checks the bearer
// key and, on the audio Decisions route, echoes it in an error to prove
// the bench redacts.
type fakeRouter struct {
	key   string
	mu    sync.Mutex
	calls map[string]int
}

const fakeDraft = `{"kind":"topic","title":"Raft consensus"}
{"idea":{"title":"Leader election","summary":"Raft elects one leader per term.","links":["Quorum intersection"],"items":[{"q":"How does Raft avoid split votes in leader elections?","a":"Randomized election timeouts."},{"q":"Who appends client commands in Raft?","a":"The leader."}]}}
{"idea":{"title":"Log replication","summary":"The leader replicates its log.","links":["Leader election"],"items":[{"q":"Where does a Raft leader send new log entries?","a":"To its followers."},{"q":"What does a Raft follower store?","a":"The leader's log entries."}]}}
{"idea":{"title":"Commit rule","summary":"An entry commits on a majority.","links":["Paxos basics"],"items":[{"q":"When is a Raft entry committed?","a":"When a majority stores it."},{"q":"What fraction of servers must store a Raft entry to commit it?","a":"A majority."}]}}
{"idea":{"title":"Terms as clocks","summary":"Terms order leadership.","links":["Commit rule"],"items":[{"q":"What does a Raft server do on seeing a higher term?","a":"Steps down to follower."},{"q":"What acts as Raft's logical clock?","a":"The term number."}]}}
{"done":true,"note":"Pitched at an on-call engineer."}
`

func (f *fakeRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls[r.URL.Path]++
	f.mu.Unlock()
	if r.URL.Path == "/api/v1/models" {
		fmt.Fprint(w, `{"data":[{"id":"t/judge","pricing":{"prompt":"0.0000001","completion":"0"}},{"id":"t/writer","pricing":{"prompt":"0.000001","completion":"0.000005"}},{"id":"t/stt","pricing":{"prompt":"0.00001","completion":"0"}},{"id":"t/probe","pricing":{"prompt":"0.1","completion":"0"}},{"id":"t/opt","pricing":{"prompt":"0.000001","completion":"0.000005"}}]}`)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+f.key {
		w.WriteHeader(401)
		return
	}
	body, _ := io.ReadAll(r.Body)
	switch r.URL.Path {
	case "/api/alpha/decisions":
		if bytes.Contains(body, []byte("input_audio")) {
			w.WriteHeader(400)
			fmt.Fprintf(w, `{"error":{"code":400,"message":"audio parts are not accepted (debug key=%s)"}}`, f.key)
			return
		}
		choice := "correct"
		if bytes.Contains(body, []byte("Ignore all previous")) {
			choice = "incorrect"
		}
		fmt.Fprintf(w, `{"model":"t/judge-2026","provider":"Fake","answers":{"judgment":{"type":"choice","choice":%q,"confidence":0.9,"probabilities":{"correct":%v,"almost correct":0.05,"incorrect":%v,"none_of_these":0}}},"usage":{"input_tokens":120,"output_tokens":0,"cost":0.00001}}`,
			choice, map[bool]float64{true: 0.9, false: 0.05}[choice == "correct"], map[bool]float64{true: 0.9, false: 0.05}[choice == "incorrect"])
	case "/api/v1/chat/completions":
		switch {
		case bytes.Contains(body, []byte(`"stream":true`)):
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, ": OPENROUTER PROCESSING\n\n")
			for i := 0; i < len(fakeDraft); i += 70 {
				chunk, _ := json.Marshal(fakeDraft[i:min(i+70, len(fakeDraft))])
				fmt.Fprintf(w, "data: {\"model\":\"t/writer-2026\",\"provider\":\"Fake\",\"choices\":[{\"delta\":{\"content\":%s}}]}\n\n", chunk)
			}
			fmt.Fprint(w, "data: {\"model\":\"t/writer-2026\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":900,\"completion_tokens\":700,\"cost\":0.004}}\n\ndata: [DONE]\n\n")
		case bytes.Contains(body, []byte("input_audio")):
			fmt.Fprint(w, `{"model":"t/writer-2026","choices":[{"message":{"content":"PostgreSQL uses MVCC, multiversion concurrency control."},"finish_reason":"stop"}],"usage":{"prompt_tokens":200,"completion_tokens":12,"cost":0.0002}}`)
		default:
			fmt.Fprint(w, `{"model":"t/writer-2026","choices":[{"message":{"content":"Trapped air isn't why: ice is less dense because hydrogen bonds hold an open lattice."},"finish_reason":"stop"}],"usage":{"prompt_tokens":150,"completion_tokens":20,"cost":0.0003}}`)
		}
	case "/api/v1/audio/transcriptions":
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":{"code":400,"message":"multipart not supported"}}`)
			return
		}
		cost := 0.00002
		if bytes.Contains(body, []byte(`"t/probe"`)) {
			cost = 0.02
		}
		fmt.Fprintf(w, `{"text":"PostgreSQL uses MVCC multi version concurrency control","usage":{"cost":%v,"seconds":5.8}}`, cost)
	default:
		w.WriteHeader(404)
	}
}

func readSummary(t *testing.T, dir string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "eval-summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func path(m any, keys ...any) any {
	for _, k := range keys {
		switch k := k.(type) {
		case string:
			m = m.(map[string]any)[k]
		case int:
			m = m.([]any)[k]
		}
	}
	return m
}

// End to end through the production adapters against a stand-in server.
func TestRunAllSurfacesOffline(t *testing.T) {
	const key = "sk-or-v1-offline-test-key-0123456789"
	f := &fakeRouter{key: key, calls: map[string]int{}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	out, ev := t.TempDir(), t.TempDir()
	r, err := NewRunner(Config{Surfaces: Surfaces, OutDir: out, EvidenceDir: ev, MaxSpend: 1, Limit: 2, Key: key, Base: srv.URL, Log: io.Discard, SkipAccount: true,
		Models: map[string][]string{"grading": {"t/judge"}, "explainer": {"t/writer", "t/writer2"}, "drafting": {"t/writer"}, "drafting-optional": {"t/opt", "t/unpriced"}, "speech-chat": {"t/writer"}, "speech-stt": {"t/stt"}, "speech-stt-probe": {"t/probe"}, "speech-audio-grade": {"t/judge"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := readSummary(t, out)
	if got := path(s, "results", "grading", "models", 0, "accuracy_pct"); got != 100.0 {
		t.Errorf("grading accuracy = %v", got)
	}
	if got := path(s, "results", "explainer", "models", 0, "verdict_respected_pct"); got != 100.0 {
		t.Errorf("explainer verdict respected = %v", got)
	}
	exp, _ := os.ReadFile(filepath.Join(out, "explainer-raw.jsonl"))
	if !bytes.Contains(exp, []byte(`"case_id":"e01","domain":"STEM","verdict":"missed","model":"t/writer","status":"ok"`)) || !bytes.Contains(exp, []byte(`"score":100`)) {
		t.Errorf("explainer e01 record not scored 100: %s", exp)
	}
	if got := path(s, "results", "drafting", "models", 0, "schema_valid_pct"); got != 100.0 {
		t.Errorf("drafting schema valid = %v", got)
	}
	if got := path(s, "results", "drafting", "models", 0, "usd_per_100_drafts"); math.Abs(got.(float64)-0.4) > 1e-9 {
		t.Errorf("$/100 drafts = %v", got)
	}
	if got := path(s, "results", "speech", "track_4b_format_probe", "chosen"); got != "json" {
		t.Errorf("format = %v", got)
	}
	if got := path(s, "results", "speech", "track_4c_direct_audio_grade", "models", 0, "errors"); got != 2.0 {
		t.Errorf("4C errors = %v", got)
	}
	if got := path(s, "results", "speech", "track_4d_today_transcript_then_jev", "models", 0, "cases"); got != 2.0 {
		t.Errorf("4D cases = %v", got)
	}
	notes := fmt.Sprint(s["notes"])
	if !strings.Contains(notes, "t/probe") || !strings.Contains(notes, "not run on the other clips") {
		t.Errorf("probe gate note missing: %s", notes)
	}
	if f.calls["/api/v1/audio/transcriptions"] != 2+2+1 { // format probe x2, t/stt x2 clips, t/probe x1
		t.Errorf("STT calls = %d", f.calls["/api/v1/audio/transcriptions"])
	}
	// Exact spend equals the sum of the fake costs.
	if !strings.Contains(notes, "drafting optional t/opt run") || !strings.Contains(notes, "drafting optional t/unpriced not run") {
		t.Errorf("optional drafting notes: %s", notes)
	}
	if got := path(s, "results", "drafting", "models", 1, "model"); got != "t/opt" {
		t.Errorf("optional model row = %v", got)
	}
	want := 2*0.00001 + 4*0.0003 + 2*2*0.004 + 2*0.0002 + 3*0.00002 + 0.02 + 2*0.00001 // the 400 multipart probe reports no cost
	if got := path(s, "spend", "exact_usage_cost_usd").(float64); math.Abs(got-want) > 1e-9 {
		t.Errorf("exact spend = %v, want %v", got, want)
	}
	for _, name := range EvidenceNames {
		b, err := os.ReadFile(filepath.Join(ev, name))
		if err != nil || len(b) == 0 {
			t.Errorf("evidence %s missing or empty: %v", name, err)
		}
		if bytes.Contains(b, []byte(key)) {
			t.Errorf("key leaked into %s", name)
		}
	}
	raw, _ := os.ReadFile(filepath.Join(out, "speech-raw.jsonl"))
	if !bytes.Contains(raw, []byte("[redacted]")) {
		t.Error("echoed key was not redacted in the 4C error")
	}
	var blind struct{ Sets []BlindSet }
	b, _ := os.ReadFile(filepath.Join(out, "blind-samples.json"))
	if err := json.Unmarshal(b, &blind); err != nil || len(blind.Sets) != 4 {
		t.Fatalf("blind sets = %d, %v", len(blind.Sets), err)
	}
	if bytes.Contains(b, []byte("t/writer")) {
		t.Error("blind samples leak a model slug")
	}
}

func TestRunStopsAtSpendCap(t *testing.T) {
	const key = "sk-or-v1-offline-test-key-cap"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/models", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":[]}`) })
	var calls int
	mux.HandleFunc("/api/alpha/decisions", func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"model":"t/judge","answers":{"judgment":{"type":"choice","choice":"correct"}},"usage":{"input_tokens":1,"output_tokens":0,"cost":0.3}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	out := t.TempDir()
	r, err := NewRunner(Config{Surfaces: []string{"grading"}, OutDir: out, MaxSpend: 1, Key: key, Base: srv.URL, Log: io.Discard, SkipAccount: true, Models: map[string][]string{"grading": {"t/judge"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Call 1 reserves the floor and costs 0.3; call 2 reserves 2x0.3 (0.3+0.6 <= 1) and costs 0.3;
	// call 3 would need 0.6+0.6 > 1 and is refused, as is every later call.
	if calls != 2 {
		t.Fatalf("dispatched %d calls, want 2", calls)
	}
	s := readSummary(t, out)
	if got := path(s, "spend", "refused_for_budget"); got != 23.0 {
		t.Errorf("refused = %v", got)
	}
	if got := path(s, "results", "grading", "models", 0, "skipped_for_budget"); got != 23.0 {
		t.Errorf("skipped = %v", got)
	}
	if got := path(s, "total_spend_usd").(float64); got > 1 {
		t.Errorf("spend %v over cap", got)
	}
}

func TestNewRunnerNeedsKey(t *testing.T) {
	if _, err := NewRunner(Config{Surfaces: []string{"grading"}, OutDir: t.TempDir(), MaxSpend: 1}); err == nil {
		t.Fatal("a live run without OPENROUTER_API_KEY must fail")
	}
	if _, err := NewRunner(Config{Surfaces: []string{"nope"}, OutDir: t.TempDir(), MaxSpend: 1, Key: "k"}); err == nil {
		t.Fatal("unknown surface accepted")
	}
}

func TestAppendMergesAndCapsCumulatively(t *testing.T) {
	const key = "sk-or-v1-offline-test-key-append"
	f := &fakeRouter{key: key, calls: map[string]int{}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	out := t.TempDir()
	cfg := func(surface string, maxSpend float64, appendRun bool) Config {
		return Config{Surfaces: []string{surface}, OutDir: out, MaxSpend: maxSpend, Limit: 2, Key: key, Base: srv.URL, Log: io.Discard, SkipAccount: true, Append: appendRun,
			Models: map[string][]string{"grading": {"t/judge"}, "explainer": {"t/writer"}}}
	}
	run := func(c Config) {
		t.Helper()
		r, err := NewRunner(c)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	run(cfg("grading", 1, false))        // spends 2 x 0.00001
	run(cfg("explainer", 0.00002, true)) // cap already reached by run 1: both refused
	s := readSummary(t, out)
	if got := path(s, "spend", "refused_for_budget"); got != 2.0 {
		t.Fatalf("refused = %v; the cap must count the earlier spend", got)
	}
	if path(s, "results", "grading") == nil || path(s, "results", "explainer", "models", 0, "skipped_for_budget") != 2.0 {
		t.Fatalf("results not merged: %v", s["results"])
	}
	run(cfg("explainer", 1, true)) // re-run replaces the explainer result
	s = readSummary(t, out)
	if got := path(s, "results", "explainer", "models", 0, "ok"); got != 2.0 {
		t.Errorf("explainer ok = %v", got)
	}
	if got := len(s["commands"].([]any)); got != 3 {
		t.Errorf("commands = %d", got)
	}
	if got := path(s, "spend", "exact_usage_cost_usd").(float64); math.Abs(got-(2*0.00001+2*0.0003)) > 1e-12 {
		t.Errorf("cumulative spend = %v", got)
	}
	raw, _ := os.ReadFile(filepath.Join(out, "explainer-raw.jsonl"))
	if n := bytes.Count(raw, []byte("\n")); n != 4 {
		t.Errorf("explainer raw lines = %d, want 4 (2 skipped + 2 ok, appended)", n)
	}
	var blind struct{ Sets []BlindSet }
	b, _ := os.ReadFile(filepath.Join(out, "blind-samples.json"))
	if json.Unmarshal(b, &blind) != nil || len(blind.Sets) != 2 {
		t.Errorf("blind sets after re-run = %d, want 2 (replaced, not duplicated)", len(blind.Sets))
	}
	if _, err := NewRunner(Config{Surfaces: []string{"grading"}, OutDir: out, MaxSpend: 1, Key: key, Append: true, Seed: "other"}); err == nil {
		t.Error("appending with a different blind seed must fail")
	}
}

func TestInterruptStillWritesSummary(t *testing.T) {
	const key = "sk-or-v1-offline-test-key-interrupt"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/models", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":[]}`) })
	mux.HandleFunc("/api/alpha/decisions", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"model":"t/judge","answers":{"judgment":{"type":"choice","choice":"correct"}},"usage":{"input_tokens":1,"output_tokens":0,"cost":0.001}}`)
		cancel() // the operator interrupts after the first call
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	out := t.TempDir()
	r, err := NewRunner(Config{Surfaces: []string{"grading", "explainer"}, OutDir: out, MaxSpend: 1, Limit: 3, Key: key, Base: srv.URL, Log: io.Discard, SkipAccount: true, Models: map[string][]string{"grading": {"t/judge"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Run(ctx); err != nil {
		t.Fatal(err)
	}
	s := readSummary(t, out)
	notes := fmt.Sprint(s["notes"])
	if !strings.Contains(notes, "run interrupted before surface explainer") || !strings.Contains(notes, "recorded as skipped") {
		t.Errorf("notes = %s", notes)
	}
	if got := path(s, "results", "grading", "models", 0, "skipped_for_budget"); got != 2.0 {
		t.Errorf("skipped after interrupt = %v", got)
	}
}

// CodeRabbit (PR #497): a negative, NaN or infinite usage.cost is unknown,
// so it is charged its reservation and can never lower recorded spend.
func TestMalformedCostIsUnknown(t *testing.T) {
	for _, body := range []string{`{"usage":{"cost":-0.5}}`, `{"usage":{"cost":-1e-9}}`} {
		if u := parseUsage([]byte(body)); u.Cost != nil {
			t.Errorf("parseUsage(%s).Cost = %v, want nil", body, *u.Cost)
		}
	}
	if u := parseUsage([]byte(`{"usage":{"cost":0}}`)); u.Cost == nil || *u.Cost != 0 {
		t.Errorf("a zero cost is exact, got %v", u.Cost)
	}
	if u := parseUsage([]byte(`{"usage":{"cost":0.0012}}`)); u.Cost == nil || *u.Cost != 0.0012 {
		t.Errorf("a positive cost is exact, got %v", u.Cost)
	}
	for _, c := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if validCost(&c) != nil {
			t.Errorf("validCost(%v) != nil", c)
		}
	}
	// The breaker itself: a malformed exact cost is charged the estimate.
	b := &Budget{Max: 1}
	tk, _ := b.Reserve(0.25)
	neg := -10.0
	tk.Settle(&neg)
	if st := b.State(); st.ChargedUSD != 0.25 || st.ExactUSD != 0 || st.UnknownCostCalls != 1 {
		t.Errorf("after a negative cost: %+v", st)
	}

	// End to end: a provider reporting a negative cost cannot reopen the cap.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/models", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":[]}`) })
	var calls int
	mux.HandleFunc("/api/alpha/decisions", func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"model":"t/judge","answers":{"judgment":{"type":"choice","choice":"correct","confidence":0.9,"probabilities":{"correct":0.9,"almost correct":0.05,"incorrect":0.05,"none_of_these":0}}},"usage":{"input_tokens":1,"output_tokens":0,"cost":-5}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	out := t.TempDir()
	r, err := NewRunner(Config{Surfaces: []string{"grading"}, OutDir: out, MaxSpend: 0.0012, Key: "sk-or-v1-offline-neg", Base: srv.URL, Log: io.Discard, SkipAccount: true, Models: map[string][]string{"grading": {"t/judge"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Each call reserves the 0.0005 grading floor and, its cost unknown,
	// is charged that: 0.0005, 0.001, then 0.0015 > 0.0012 is refused.
	if calls != 2 {
		t.Fatalf("dispatched %d calls, want 2 (a negative cost must not lower spend)", calls)
	}
	s := readSummary(t, out)
	if got := path(s, "spend", "exact_usage_cost_usd").(float64); got != 0 {
		t.Errorf("exact spend = %v, want 0", got)
	}
	if got := path(s, "spend", "guard_charged_usd").(float64); math.Abs(got-0.001) > 1e-12 {
		t.Errorf("charged = %v, want 0.001", got)
	}
}

// grading server that counts calls, for the raw-log tests.
func countingJudge(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/models", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":[]}`) })
	calls := new(int)
	var mu sync.Mutex
	mux.HandleFunc("/api/alpha/decisions", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*calls++
		mu.Unlock()
		fmt.Fprint(w, `{"model":"t/judge","answers":{"judgment":{"type":"choice","choice":"correct","confidence":0.9,"probabilities":{"correct":0.9,"almost correct":0.05,"incorrect":0.05,"none_of_these":0}}},"usage":{"input_tokens":1,"output_tokens":0,"cost":0.00001}}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, calls
}

// CodeRabbit (PR #497): a raw log that cannot be opened stops dispatch and
// fails the run.
func TestRawLogOpenFailureStopsDispatch(t *testing.T) {
	srv, calls := countingJudge(t)
	out := t.TempDir()
	// A directory where the raw log should go makes the open fail.
	if err := os.Mkdir(filepath.Join(out, "grading-raw.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := NewRunner(Config{Surfaces: []string{"grading", "explainer"}, OutDir: out, MaxSpend: 1, Limit: 4, Key: "sk-or-v1-offline-open", Base: srv.URL, Log: io.Discard, SkipAccount: true, Models: map[string][]string{"grading": {"t/judge"}, "explainer": {"t/writer"}}})
	if err != nil {
		t.Fatal(err)
	}
	err = r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "open grading raw log") {
		t.Fatalf("Run error = %v, want the raw-log open failure", err)
	}
	if *calls != 1 {
		t.Errorf("dispatched %d paid calls, want 1 (none after recording failed)", *calls)
	}
	s := readSummary(t, out)
	if got := path(s, "spend", "settled_calls"); got != 1.0 {
		t.Errorf("settled calls = %v, want 1", got)
	}
	if notes := fmt.Sprint(s["notes"]); !strings.Contains(notes, "raw log recording failed") {
		t.Errorf("notes = %s", notes)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "explainer-raw.jsonl")); bytes.Contains(b, []byte(`"status":"ok"`)) {
		t.Error("explainer dispatched after grading's raw log failed")
	}
}

// CodeRabbit (PR #497): a write failure is kept (before a later close
// failure on the same file), stops dispatch and is returned from Run.
func TestRawLogWriteFailureStopsDispatch(t *testing.T) {
	srv, calls := countingJudge(t)
	out := t.TempDir()
	r, err := NewRunner(Config{Surfaces: []string{"grading"}, OutDir: out, MaxSpend: 1, Limit: 5, Key: "sk-or-v1-offline-write", Base: srv.URL, Log: io.Discard, SkipAccount: true, Models: map[string][]string{"grading": {"t/judge"}}})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(out, "grading-raw.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close() // writes and the final close both fail
	r.files["grading"] = f
	err = r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "write grading raw log") {
		t.Fatalf("Run error = %v, want the first (write) failure", err)
	}
	if *calls != 1 {
		t.Errorf("dispatched %d paid calls, want 1", *calls)
	}
	s := readSummary(t, out)
	if got := path(s, "results", "grading", "models", 0, "skipped_for_budget"); got != 4.0 {
		t.Errorf("not-dispatched = %v, want 4", got)
	}
}

// CodeRabbit (PR #497): --append keeps earlier blind artifacts for a
// surface that produced nothing this time (interrupt or budget), and a
// surface that did run merges per model instead of dropping earlier ones.
func TestAppendPreservesBlindForUnrunSurfaces(t *testing.T) {
	const key = "sk-or-v1-offline-test-key-blind"
	f := &fakeRouter{key: key, calls: map[string]int{}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	out := t.TempDir()
	run := func(ctx context.Context, surfaces []string, limit int, maxSpend float64, models map[string][]string, appendRun bool) {
		t.Helper()
		r, err := NewRunner(Config{Surfaces: surfaces, OutDir: out, MaxSpend: maxSpend, Limit: limit, Key: key, Base: srv.URL, Log: io.Discard, SkipAccount: true, Append: appendRun, Models: models})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	type blindFile struct{ Sets []BlindSet }
	read := func() (blindFile, BlindKey) {
		t.Helper()
		var b blindFile
		var k BlindKey
		raw, _ := os.ReadFile(filepath.Join(out, "blind-samples.json"))
		if err := json.Unmarshal(raw, &b); err != nil {
			t.Fatal(err)
		}
		raw, _ = os.ReadFile(filepath.Join(out, "blind-key.json"))
		if err := json.Unmarshal(raw, &k); err != nil {
			t.Fatal(err)
		}
		return b, k
	}
	okOutputs := func(b blindFile) int {
		n := 0
		for _, s := range b.Sets {
			for _, o := range s.Outputs {
				if o.Status == "ok" {
					n++
				}
			}
		}
		return n
	}
	// 1. explainer on 2 cases with t/writer.
	run(context.Background(), []string{"explainer"}, 2, 1, map[string][]string{"explainer": {"t/writer"}}, false)
	b0, k0 := read()
	if len(b0.Sets) != 2 || okOutputs(b0) != 2 {
		t.Fatalf("baseline blind = %+v", b0)
	}

	// 2. --append grading,explainer, interrupted during grading: explainer
	// never runs, so its blind sets and key must survive unchanged.
	ctx, cancel := context.WithCancel(context.Background())
	mux := http.NewServeMux()
	mux.Handle("/", f)
	mux.HandleFunc("/api/alpha/decisions", func(w http.ResponseWriter, r *http.Request) { f.ServeHTTP(w, r); cancel() })
	isrv := httptest.NewServer(mux)
	defer isrv.Close()
	r, err := NewRunner(Config{Surfaces: []string{"grading", "explainer"}, OutDir: out, MaxSpend: 1, Limit: 2, Key: key, Base: isrv.URL, Log: io.Discard, SkipAccount: true, Append: true, Models: map[string][]string{"grading": {"t/judge"}, "explainer": {"t/writer"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Run(ctx); err != nil {
		t.Fatal(err)
	}
	b1, k1 := read()
	if len(b1.Sets) != 2 || okOutputs(b1) != 2 || !jsonEqual(k1.Sets, k0.Sets) {
		t.Fatalf("interrupt dropped earlier blind artifacts: sets=%d ok=%d key=%v", len(b1.Sets), okOutputs(b1), k1.Sets)
	}
	s := readSummary(t, out)
	if got := path(s, "results", "explainer", "models", 0, "ok"); got != 2.0 {
		t.Errorf("explainer result after interrupt = %v, want the earlier ok=2", got)
	}

	// 3. --append explainer with the cap already spent: every call refused,
	// so nothing replaces the earlier explainer artifacts.
	spent := path(readSummary(t, out), "spend", "guard_charged_usd").(float64)
	run(context.Background(), []string{"explainer"}, 2, spent, map[string][]string{"explainer": {"t/writer"}}, true)
	b2, k2 := read()
	if len(b2.Sets) != 2 || okOutputs(b2) != 2 || !jsonEqual(k2.Sets, k0.Sets) {
		t.Fatalf("budget refusal dropped earlier blind artifacts: sets=%d ok=%d", len(b2.Sets), okOutputs(b2))
	}
	s = readSummary(t, out)
	if got := path(s, "results", "explainer", "models", 0, "ok"); got != 2.0 {
		t.Errorf("explainer result after refusal = %v, want the earlier ok=2", got)
	}

	// 4. --append explainer with a new model on 1 case: e01 now compares
	// both models, e02 keeps t/writer, and both rows are in the summary.
	run(context.Background(), []string{"explainer"}, 1, 1, map[string][]string{"explainer": {"t/writer2"}}, true)
	b3, k3 := read()
	if len(b3.Sets) != 2 || okOutputs(b3) != 3 {
		t.Fatalf("merged blind: sets=%d ok=%d", len(b3.Sets), okOutputs(b3))
	}
	e01 := k3.Sets["explainer/e01"]
	if len(e01) != 2 || !jsonEqual(e01, BlindAssign("scry-eval-v1", "explainer", "e01", []string{"t/writer", "t/writer2"})) {
		t.Errorf("e01 key = %v", e01)
	}
	if !jsonEqual(k3.Sets["explainer/e02"], k0.Sets["explainer/e02"]) {
		t.Errorf("e02 key changed: %v", k3.Sets["explainer/e02"])
	}
	s = readSummary(t, out)
	rows := path(s, "results", "explainer", "models").([]any)
	if len(rows) != 2 || path(rows, 0, "model") != "t/writer" || path(rows, 0, "ok") != 2.0 || path(rows, 1, "model") != "t/writer2" || path(rows, 1, "ok") != 1.0 {
		t.Errorf("explainer rows = %v", rows)
	}
	if got := fmt.Sprint(path(s, "results", "explainer", "rows_from_earlier_invocations")); got != "[t/writer]" {
		t.Errorf("rows_from_earlier_invocations = %s", got)
	}
	if got := path(s, "results", "explainer", "earlier_dataset", "cases"); got != 2.0 {
		t.Errorf("earlier_dataset cases = %v", got)
	}
	raw, _ := os.ReadFile(filepath.Join(out, "blind-samples.json"))
	if bytes.Contains(raw, []byte("t/writer")) {
		t.Error("merged blind samples leak a model slug")
	}
}

// Cold Critic (PR #497): the breaker admits calls against estimates, so a
// provider settling above the reservation can overshoot the cap. A $0.0005
// reservation under a $0.001 cap settles at $0.002: the run must record the
// overshoot, dispatch nothing after it, still write the summary and raw
// log, and return ErrSpendCapExceeded so the CLI exits non-zero.
func TestRealizedOvershootFailsRun(t *testing.T) {
	const key = "sk-or-v1-offline-test-key-overshoot"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/models", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":[]}`) })
	var mu sync.Mutex
	var calls int
	mux.HandleFunc("/api/alpha/decisions", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		fmt.Fprint(w, `{"model":"t/judge","answers":{"judgment":{"type":"choice","choice":"correct","confidence":0.9,"probabilities":{"correct":0.9,"almost correct":0.05,"incorrect":0.05,"none_of_these":0}}},"usage":{"input_tokens":1,"output_tokens":0,"cost":0.002}}`)
	})
	mux.HandleFunc("/api/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		fmt.Fprint(w, `{"model":"t/writer","choices":[{"message":{"content":"x"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"cost":0.0001}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	out := t.TempDir()
	r, err := NewRunner(Config{Surfaces: []string{"grading", "explainer"}, OutDir: out, MaxSpend: 0.001, Limit: 3, Key: key, Base: srv.URL, Log: io.Discard, SkipAccount: true,
		Models: map[string][]string{"grading": {"t/judge"}, "explainer": {"t/writer"}}})
	if err != nil {
		t.Fatal(err)
	}
	// The first call's reservation is the $0.0005 grading floor (no catalog price).
	if est := r.estimate("grading", "t/judge", 0); est != 0.0005 {
		t.Fatalf("first reservation = %v, want 0.0005", est)
	}
	err = r.Run(context.Background())
	if !errors.Is(err, ErrSpendCapExceeded) {
		t.Fatalf("Run error = %v, want ErrSpendCapExceeded", err)
	}
	if calls != 1 {
		t.Errorf("dispatched %d calls, want 1 (none after the overshoot settled)", calls)
	}
	s := readSummary(t, out)
	if got := path(s, "spend", "cap_exceeded"); got != true {
		t.Errorf("cap_exceeded = %v", got)
	}
	if got := path(s, "spend", "overshoot_usd").(float64); math.Abs(got-0.001) > 1e-12 || got <= 0 {
		t.Errorf("overshoot_usd = %v, want 0.001", got)
	}
	if got := path(s, "spend", "exact_usage_cost_usd").(float64); math.Abs(got-0.002) > 1e-12 {
		t.Errorf("exact spend = %v", got)
	}
	if got := path(s, "spend", "refused_for_budget"); got != 5.0 { // 2 grading + 3 explainer
		t.Errorf("refused = %v, want 5", got)
	}
	if notes := fmt.Sprint(s["notes"]); !strings.Contains(notes, "realized spend exceeded the cap") {
		t.Errorf("notes = %s", notes)
	}
	raw, err := os.ReadFile(filepath.Join(out, "grading-raw.jsonl"))
	if err != nil || bytes.Count(raw, []byte("\n")) != 3 || bytes.Count(raw, []byte(`"predicted":"skipped"`)) != 2 || !bytes.Contains(raw, []byte(`"cost_usd":0.002`)) {
		t.Errorf("grading raw log: %v lines=%d\n%s", err, bytes.Count(raw, []byte("\n")), raw)
	}
	if bytes.Contains(raw, []byte(key)) {
		t.Error("key leaked into raw log")
	}

	// Under the cap, the same contract returns nil and reports no overshoot.
	b := &Budget{Max: 0.001}
	tk, _ := b.Reserve(0.0005)
	c := 0.0009
	tk.Settle(&c)
	if st := b.State(); st.CapExceeded || st.OvershootUSD != 0 {
		t.Errorf("under-cap state = %+v", st)
	}
}

// Cold Critic (PR #497): --append re-running a model on fewer cases than
// before must not leave that model's earlier blind outputs beside its new
// summary row. Run 1: t/writer and t/writer2 on 2 cases. Run 2 (--append):
// t/writer alone on 1 case. t/writer's row now covers 1 case, so its blind
// outputs must too; t/writer2 was not re-run and keeps both.
func TestAppendSameModelFewerCasesBlindMatchesSummary(t *testing.T) {
	const key = "sk-or-v1-offline-test-key-samemodel"
	f := &fakeRouter{key: key, calls: map[string]int{}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	out := t.TempDir()
	run := func(limit int, models []string, appendRun bool) {
		t.Helper()
		r, err := NewRunner(Config{Surfaces: []string{"explainer"}, OutDir: out, MaxSpend: 1, Limit: limit, Key: key, Base: srv.URL, Log: io.Discard, SkipAccount: true, Append: appendRun,
			Models: map[string][]string{"explainer": models}})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	// blindScope is model -> case IDs with a blind output from a dispatched call.
	blindScope := func() map[string][]string {
		t.Helper()
		var b struct{ Sets []BlindSet }
		var k BlindKey
		raw, _ := os.ReadFile(filepath.Join(out, "blind-samples.json"))
		if err := json.Unmarshal(raw, &b); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte("t/writer")) {
			t.Error("blind samples leak a model slug")
		}
		raw, _ = os.ReadFile(filepath.Join(out, "blind-key.json"))
		if err := json.Unmarshal(raw, &k); err != nil {
			t.Fatal(err)
		}
		scope := map[string][]string{}
		for _, s := range b.Sets {
			assign := k.Sets[s.Surface+"/"+s.CaseID]
			if len(assign) != len(s.Outputs) {
				t.Errorf("%s: %d outputs but %d key entries", s.CaseID, len(s.Outputs), len(assign))
			}
			for _, o := range s.Outputs {
				m := assign[o.ModelLabel]
				if m == "" {
					t.Errorf("%s %s has no key entry", s.CaseID, o.ModelLabel)
				}
				if blindRan(o.Status) {
					scope[m] = append(scope[m], s.CaseID)
				}
			}
		}
		return scope
	}
	// summaryScope is model -> dispatched calls in its summary row.
	summaryScope := func() map[string]float64 {
		t.Helper()
		calls := map[string]float64{}
		for _, row := range path(readSummary(t, out), "results", "explainer", "models").([]any) {
			calls[path(row, "model").(string)] = path(row, "calls").(float64)
		}
		return calls
	}

	run(2, []string{"t/writer", "t/writer2"}, false)
	if got := fmt.Sprint(blindScope()); got != "map[t/writer:[e01 e02] t/writer2:[e01 e02]]" {
		t.Fatalf("baseline blind scope = %s", got)
	}

	run(1, []string{"t/writer"}, true)
	scope, rows := blindScope(), summaryScope()
	if got := fmt.Sprint(scope); got != "map[t/writer:[e01] t/writer2:[e01 e02]]" {
		t.Errorf("blind scope after same-model --append = %s, want t/writer on e01 only", got)
	}
	for m, calls := range rows {
		if float64(len(scope[m])) != calls {
			t.Errorf("%s: summary row has %v calls but %d blind outputs %v", m, calls, len(scope[m]), scope[m])
		}
	}
	if rows["t/writer"] != 1 || rows["t/writer2"] != 2 {
		t.Errorf("summary rows = %v", rows)
	}
	s := readSummary(t, out)
	if got := fmt.Sprint(path(s, "results", "explainer", "rows_from_earlier_invocations")); got != "[t/writer2]" {
		t.Errorf("rows_from_earlier_invocations = %s", got)
	}
	if notes := fmt.Sprint(s["notes"]); !strings.Contains(notes, "explainer model t/writer was re-run on fewer cases than before; its 1 earlier blind output(s)") {
		t.Errorf("drop note missing: %s", notes)
	}
	// e02 now holds t/writer2 alone, re-laid out over its remaining model.
	var k BlindKey
	raw, _ := os.ReadFile(filepath.Join(out, "blind-key.json"))
	_ = json.Unmarshal(raw, &k)
	if !jsonEqual(k.Sets["explainer/e02"], BlindAssign("scry-eval-v1", "explainer", "e02", []string{"t/writer2"})) {
		t.Errorf("e02 key = %v", k.Sets["explainer/e02"])
	}

	// Re-running both models on 1 case drops e02 entirely: no model is
	// left on it, so only e01 remains in the blind file.
	run(1, []string{"t/writer", "t/writer2"}, true)
	scope, rows = blindScope(), summaryScope()
	if got := fmt.Sprint(scope); got != "map[t/writer:[e01] t/writer2:[e01]]" {
		t.Errorf("blind scope after re-running both on 1 case = %s", got)
	}
	for m, calls := range rows {
		if float64(len(scope[m])) != calls {
			t.Errorf("%s: summary row has %v calls but %d blind outputs", m, calls, len(scope[m]))
		}
	}
	var b struct{ Sets []BlindSet }
	raw, _ = os.ReadFile(filepath.Join(out, "blind-samples.json"))
	if json.Unmarshal(raw, &b) != nil || len(b.Sets) != 1 || b.Sets[0].CaseID != "e01" {
		t.Errorf("blind sets = %+v, want e01 only", b.Sets)
	}
}

// Cold Critic (PR #497): --append over earlier blind outputs whose
// blind-key.json is missing, unreadable, malformed or incomplete must fail
// before any provider request (catalog included) is dispatched, so the run
// spends nothing and leaves the earlier artifacts untouched.
func TestAppendRejectsMissingOrIncompletePriorBlindKey(t *testing.T) {
	const key = "sk-or-v1-offline-test-key-blindkey"
	f := &fakeRouter{key: key, calls: map[string]int{}}
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		f.ServeHTTP(w, r)
	}))
	defer srv.Close()
	out := t.TempDir()
	cfg := func(appendRun bool) Config {
		return Config{Surfaces: []string{"explainer"}, OutDir: out, MaxSpend: 1, Limit: 2, Key: key, Base: srv.URL, Log: io.Discard, SkipAccount: true, Append: appendRun,
			Models: map[string][]string{"explainer": {"t/writer", "t/writer2"}}}
	}
	r, err := NewRunner(cfg(false))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	keyPath, samplesPath := filepath.Join(out, "blind-key.json"), filepath.Join(out, "blind-samples.json")
	goodKey, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	samples, err := os.ReadFile(samplesPath)
	if err != nil {
		t.Fatal(err)
	}
	summary, _ := os.ReadFile(filepath.Join(out, "eval-summary.json"))
	var blind struct{ Sets []BlindSet }
	if err := json.Unmarshal(samples, &blind); err != nil || len(blind.Sets) != 2 || len(blind.Sets[0].Outputs) != 2 {
		t.Fatalf("baseline blind sets = %+v, %v", blind.Sets, err)
	}
	var bk BlindKey
	if err := json.Unmarshal(goodKey, &bk); err != nil || len(bk.Sets) != 2 {
		t.Fatalf("baseline blind key = %+v, %v", bk, err)
	}
	// Key with one case/label entry removed.
	victim := blind.Sets[1].Surface + "/" + blind.Sets[1].CaseID
	delete(bk.Sets[victim], blind.Sets[1].Outputs[0].ModelLabel)
	incomplete, _ := json.Marshal(bk)

	cases := []struct {
		name  string
		setup func(t *testing.T)
		want  string
	}{
		{"key missing", func(t *testing.T) {
			if err := os.Remove(keyPath); err != nil {
				t.Fatal(err)
			}
		}, "blind-key.json is missing"},
		{"key missing one case/label", func(t *testing.T) {
			if err := os.WriteFile(keyPath, incomplete, 0o644); err != nil {
				t.Fatal(err)
			}
		}, "lacks 1 of 4 blind label(s)"},
		{"key malformed", func(t *testing.T) {
			if err := os.WriteFile(keyPath, []byte("{not json"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, "blind-key.json is malformed"},
		{"key unreadable", func(t *testing.T) {
			// A directory in its place: exists, but cannot be read as a file.
			if err := os.Remove(keyPath); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(keyPath, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Remove(keyPath) })
		}, "blind-key.json is unreadable"},
		{"samples malformed", func(t *testing.T) {
			if err := os.WriteFile(keyPath, goodKey, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(samplesPath, []byte("[truncated"), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.WriteFile(samplesPath, samples, 0o644) })
		}, "blind-samples.json is malformed"},
		{"samples symlink target missing", func(t *testing.T) {
			backup := samplesPath + ".backup"
			if err := os.Rename(samplesPath, backup); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Rename(backup, samplesPath); err != nil {
					t.Error(err)
				}
			})
			if err := os.Symlink(samplesPath+".missing", samplesPath); err != nil {
				t.Skipf("cannot create a symlink: %v", err)
			}
		}, "blind-samples.json is unreadable"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.setup(t)
			requests.Store(0)
			r, err := NewRunner(cfg(true))
			if err == nil {
				err = r.Run(context.Background())
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Run err = %v, want it to contain %q", err, c.want)
			}
			if !strings.Contains(err.Error(), "use a fresh --out directory") {
				t.Errorf("error does not tell the operator how to recover: %v", err)
			}
			if n := requests.Load(); n != 0 {
				t.Fatalf("%d provider request(s) dispatched before the preflight rejected --append; want 0", n)
			}
			if now, _ := os.ReadFile(filepath.Join(out, "eval-summary.json")); !bytes.Equal(now, summary) {
				t.Error("a rejected --append rewrote eval-summary.json")
			}
		})
	}

	// Run re-checks on its own: a key that disappears after NewRunner
	// accepted the directory still stops the run before any request.
	if err := os.WriteFile(keyPath, goodKey, 0o644); err != nil {
		t.Fatal(err)
	}
	r, err = NewRunner(cfg(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	requests.Store(0)
	if err := r.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "blind-key.json is missing") {
		t.Fatalf("Run after the key vanished: err = %v", err)
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("Run dispatched %d request(s) after the key vanished; want 0", n)
	}

	// Control: with the intact key restored the same --append dispatches.
	if err := os.WriteFile(keyPath, goodKey, 0o644); err != nil {
		t.Fatal(err)
	}
	requests.Store(0)
	r, err = NewRunner(cfg(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("--append with an intact blind key: %v", err)
	}
	if requests.Load() == 0 {
		t.Error("--append with an intact blind key dispatched nothing")
	}
}
