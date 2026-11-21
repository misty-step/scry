package evals

import (
	"bytes"
	"context"
	"encoding/json"
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
