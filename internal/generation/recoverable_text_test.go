package generation

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/misty-step/scry/internal/store"
)

func TestRecoverablePrivateTextValidationPreservesGrounding(t *testing.T) {
	job := &store.Job{
		Kind:       "plan",
		SourceKind: "source",
		SourceMode: "text",
		SourceText: "Teach me why HTTP caches go stale and how to diagnose them.",
	}
	note := &store.NoteContent{
		Title: "Stale HTTP caches",
		Body:  "A cached response can outlive the freshness period intended by an origin or intermediary. Diagnosis compares cache directives, response age, validators, and the behavior of each cache in the request path. A common confusion is to treat every old response as a browser-only problem, even when a shared cache served it.",
		Basis: "topic",
	}
	plan := store.PlanContent{
		Goal: "Understand stale HTTP caches",
		Concepts: []store.PlannedConcept{{
			Key:     "stale-cache",
			Name:    "Stale HTTP caches",
			Summary: "Understand why cached responses can remain stale.",
			Note:    note,
		}},
	}
	encode := func() string {
		t.Helper()
		data, err := json.Marshal(plan)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	result, err := validateV5Output(job, store.JobContext{}, encode())
	if err != nil || result.Plan == nil || result.Plan.Concepts[0].Note.Basis != "topic" ||
		len(result.Plan.Concepts[0].Note.Evidence) != 0 || len(result.Plan.Concepts[0].Note.Citations) != 0 {
		t.Fatalf("broad private-text plan was not retained as general knowledge: %+v %v", result, err)
	}

	note.Basis = "source"
	note.Evidence = []string{"HTTP caches always stay fresh automatically."}
	result, err = validateV5Output(job, store.JobContext{}, encode())
	if err == nil || !strings.Contains(err.Error(), "source evidence is not an exact quotation") || result.Plan != nil {
		t.Fatalf("failed claimed quotation was relabeled as general knowledge: %+v %v", result, err)
	}
}
