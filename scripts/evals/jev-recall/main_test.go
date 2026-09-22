package main

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/semantic"
	"github.com/misty-step/scry/internal/store"
)

const committedEvalDir = "../../../" + defaultEvalDir

func loadCommitted(t *testing.T) ([]WorkItem, []RawRecord) {
	t.Helper()
	items, corpus, _, err := loadEvaluation(committedEvalDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateEvaluation(items, corpus); err != nil {
		t.Fatal(err)
	}
	records, err := loadRawOptional(filepath.Join(committedEvalDir, "raw.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 {
		t.Fatal("committed raw.jsonl is empty")
	}
	return items, records
}

// The committed raw records must bind to the committed corpus and gold, and
// any record that is stale, foreign, edited, or duplicated must be refused
// before a table is computed from it.
func TestBindRecordsAcceptsCommittedRawAndRefusesForeignRecords(t *testing.T) {
	items, records := loadCommitted(t)
	if err := bindRecords(records, items); err != nil {
		t.Fatalf("committed raw.jsonl does not bind: %v", err)
	}
	if err := bindRecords(nil, items); err != nil {
		t.Fatalf("empty raw must bind: %v", err)
	}
	mutate := func(name string, change func(r *RawRecord), want string) {
		t.Helper()
		record := records[0]
		record.Request.Questions = copyQuestions(records[0].Request.Questions)
		change(&record)
		err := bindRecords([]RawRecord{record}, items)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: want error containing %q, got %v", name, want, err)
		}
	}
	mutate("unknown response", func(r *RawRecord) { r.ResponseID = "resp-not-in-corpus" }, "not in the loaded corpus")
	mutate("wrong concept", func(r *RawRecord) { r.ConceptID = "other-concept" }, "identity does not match")
	mutate("wrong bucket", func(r *RawRecord) { r.Bucket = "exact" }, "identity does not match")
	mutate("wrong split", func(r *RawRecord) { r.Split = map[string]string{"tune": "holdout", "holdout": "tune"}[r.Split] }, "identity does not match")
	mutate("edited learner answer", func(r *RawRecord) { r.LearnerAnswer += " extra" }, "identity does not match")
	mutate("edited gold action", func(r *RawRecord) { r.GoldAction = "ungraded_not_judgeable" }, "identity does not match")
	mutate("edited request", func(r *RawRecord) { r.Request.State.ExpectedAnswer += "!" }, "request differs")
	mutate("edited request question", func(r *RawRecord) { delete(r.Request.Questions, "relation") }, "request differs")
	mutate("answers do not fit request", func(r *RawRecord) {
		if r.Answers != nil {
			r.Answers = map[string]DecisionAnswer{"relation": r.Answers["relation"]}
		}
	}, "answers do not fit")
	dup := []RawRecord{records[0], records[0]}
	if err := bindRecords(dup, items); err == nil || !strings.Contains(err.Error(), "duplicate run") {
		t.Fatalf("duplicate (run, response) must be refused, got %v", err)
	}
}

func copyQuestions(in map[string]DecisionQuestion) map[string]DecisionQuestion {
	out := make(map[string]DecisionQuestion, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// The eval's request builder must produce byte-for-byte the request the
// shipped product builds for the same recall state, otherwise the corpus
// measures a different judge than the one learners meet.
func TestBuildRecallRequestMatchesShippedBuilder(t *testing.T) {
	items, _ := loadCommitted(t)
	checked := 0
	for _, item := range items {
		if item.Question.Grading != "semantic" || item.Question.Rubric == nil {
			continue
		}
		evalRequest, err := json.Marshal(buildRecallRequest(item.Question, item.Response.Text))
		if err != nil {
			t.Fatal(err)
		}
		rubric := store.Rubric{}
		for _, required := range item.Question.Rubric.Required {
			rubric.Required = append(rubric.Required, store.RubricIdea{Text: required.Text})
		}
		for _, claim := range item.Question.Rubric.Contradictions {
			rubric.Contradictions = append(rubric.Contradictions, store.RubricClaim{Text: claim.Text})
		}
		shipped, err := json.Marshal(semantic.BuildRecallRequest(requestModel, semantic.RecallState{
			Prompt:         item.Question.Prompt,
			ExpectedAnswer: item.Question.ExpectedAnswer,
			LearnerAnswer:  item.Response.Text,
			Variants:       item.Question.Variants,
			Rubric:         rubric,
		}))
		if err != nil {
			t.Fatal(err)
		}
		if string(evalRequest) != string(shipped) {
			t.Fatalf("request for %s differs from the shipped builder\neval:    %s\nshipped: %s", item.Response.ID, evalRequest, shipped)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no semantic responses compared")
	}
}

// A request that leaves the process and returns no billable outcome must be
// recorded as unknown spend, never as a free failure; a refusal that provably
// completed before any model work stays known.
func TestCallDecisionAPIMarksLostOutcomesAsUnknownSpend(t *testing.T) {
	items, _ := loadCommitted(t)
	var request DecisionRequest
	for _, item := range items {
		if item.Question.Grading == "semantic" {
			request = buildRecallRequest(item.Question, item.Response.Text)
			break
		}
	}
	serve := func(handler http.HandlerFunc) callResult {
		server := httptest.NewServer(handler)
		defer server.Close()
		client := &http.Client{Timeout: 300 * time.Millisecond, Transport: rewriteTo(server.URL)}
		return callDecisionAPI(client, "test-key", request)
	}
	timeout := serve(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(600 * time.Millisecond)
	})
	if !timeout.CostUnknown || timeout.Error == "" {
		t.Fatalf("timeout must be unknown spend: %+v", timeout)
	}
	truncated := serve(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x","model":"m","answers":{`))
	})
	if !truncated.CostUnknown || truncated.Error == "" {
		t.Fatalf("undecodable 200 must be unknown spend: %+v", truncated)
	}
	refused := serve(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	})
	if refused.CostUnknown || refused.Error == "" || refused.Status != http.StatusBadRequest {
		t.Fatalf("a completed 4xx refusal is a known outcome: %+v", refused)
	}
}

// rewriteTo sends every request to the test server regardless of the fixed
// production endpoint constant.
type rewriteTransport struct{ base string }

func rewriteTo(base string) http.RoundTripper { return rewriteTransport{base: base} }

func (t rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(t.base, "http://")
	return http.DefaultTransport.RoundTrip(clone)
}

// Concurrent workers must not all pass the ceiling check on stale totals:
// the reservation taken before each send bounds in-flight spend so the
// ceiling holds even when every response costs the full reservation.
func TestRunJobsReservationBoundsConcurrentSpend(t *testing.T) {
	const (
		ceiling     = 1.0
		reservation = 0.25
		total       = 40
	)
	items, _ := loadCommitted(t)
	jobs := make([]runJob, 0, total)
	for i := range total {
		jobs = append(jobs, runJob{Item: items[i%len(items)], RunID: "t", Sequence: i + 1})
	}
	var (
		mu       sync.Mutex
		inFlight int
		peak     int
	)
	var launched atomic.Int32
	var out bytes.Buffer
	totals, err := runJobs(jobs, runBudget{MaxSpend: ceiling, Reservation: reservation, Concurrency: 8}, &syncedBuffer{Buffer: &out},
		func(job runJob) RawRecord {
			launched.Add(1)
			mu.Lock()
			inFlight++
			peak = max(peak, inFlight)
			mu.Unlock()
			time.Sleep(5 * time.Millisecond)
			mu.Lock()
			inFlight--
			mu.Unlock()
			// Every call costs the whole reservation, the worst honest case.
			return RawRecord{ResponseID: job.Item.Response.ID, CostUSD: reservation, Transmissions: []Transmission{{Attempt: 1}}}
		})
	if err != nil {
		t.Fatalf("spend exactly at the ceiling is not an overrun: %v", err)
	}
	if totals.Written >= total {
		t.Fatal("expected the run to stop short of the selection")
	}
	if totals.Spent > ceiling {
		t.Fatalf("ceiling overrun: spent=%v", totals.Spent)
	}
	if got, want := int(launched.Load()), int(ceiling/reservation); got != want {
		t.Fatalf("launched %d requests; want exactly %d under the ceiling", got, want)
	}
	if peak > int(ceiling/reservation) {
		t.Fatalf("in-flight requests %d exceed what the ceiling can cover", peak)
	}
	if got := bytes.Count(out.Bytes(), []byte("\n")); got != totals.Written {
		t.Fatalf("wrote %d lines, totals say %d", got, totals.Written)
	}
}

// A lost outcome keeps its reservation as spend and stops the run so unknown
// provider charges never hide behind a $0 failure.
func TestRunJobsRetainsUnknownSpendAndHalts(t *testing.T) {
	items, _ := loadCommitted(t)
	jobs := make([]runJob, 0, 10)
	for i := range 10 {
		jobs = append(jobs, runJob{Item: items[i], RunID: "t", Sequence: i + 1})
	}
	var calls atomic.Int32
	var out bytes.Buffer
	totals, err := runJobs(jobs, runBudget{MaxSpend: 1, Reservation: 0.01, Concurrency: 1}, &syncedBuffer{Buffer: &out},
		func(job runJob) RawRecord {
			n := calls.Add(1)
			if n == 2 {
				return RawRecord{ResponseID: job.Item.Response.ID, CostUnknown: true, Error: "context deadline exceeded", Transmissions: []Transmission{{Attempt: 1}}}
			}
			return RawRecord{ResponseID: job.Item.Response.ID, CostUSD: 0.001, Transmissions: []Transmission{{Attempt: 1}}}
		})
	if err == nil || !strings.Contains(err.Error(), "unknown spend retained") {
		t.Fatalf("want unknown-spend halt, got %v", err)
	}
	if totals.Unknown != 1 || totals.Written != 2 {
		t.Fatalf("want 1 unknown record after 2 writes, got %+v", totals)
	}
	if want := 0.001 + 0.01; math.Abs(totals.Spent-want) > 1e-12 {
		t.Fatalf("spent %v, want measured cost plus retained reservation %v", totals.Spent, want)
	}
	if calls.Load() != 2 {
		t.Fatalf("run continued after the lost outcome: %d calls", calls.Load())
	}
	var last RawRecord
	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if err := json.Unmarshal(lines[len(lines)-1], &last); err != nil {
		t.Fatal(err)
	}
	if !last.CostUnknown {
		t.Fatal("the unknown-spend record must be persisted with cost_unknown set")
	}
}

type syncedBuffer struct{ *bytes.Buffer }

func (syncedBuffer) Sync() error { return nil }
