package generation

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/misty-step/scry/internal/store"
)

// A previously observed model response included its answer in the prompt.
// The v5 questions path must reject it before any critic or publication.
func TestLeakedLiveControlFailsV5QuestionGate(t *testing.T) {
	raw, err := os.ReadFile("testdata/critic-leaked-control.json")
	if err != nil {
		t.Fatal(err)
	}
	var draft quizDraft
	if err := json.Unmarshal(raw, &draft); err != nil {
		t.Fatal(err)
	}
	draft.Kind = "recall"
	if issue := validateQuiz(draft, &store.Job{SourceKind: "topic", SourceText: "mitochondria"}); issue != "answer_leakage_or_vague_prompt" {
		t.Fatalf("answer-bearing prompt escaped the shared v5 quiz validator: %q", issue)
	}
}
