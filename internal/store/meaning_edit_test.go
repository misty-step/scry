package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

// generatedMeaning is what ordinary generation now publishes for conceptual
// recall: a semantic version whose rubric has required ideas only.
func generatedMeaning() GeneratedQuiz {
	return GeneratedQuiz{
		Kind: "recall", Grading: "semantic", Prompt: "Why can a browser reuse a cached page after a conditional request?",
		Answer:      "The server confirms the stored copy is still current, so the body is not sent again.",
		Explanation: "A 304 response confirms the stored representation is current, so the client reuses it.",
		Basis:       "topic",
		Rubric:      &Rubric{Required: []RubricIdea{{Text: "The server confirms the stored copy is still current"}, {Text: "The body is not sent again"}}},
	}
}

// learnerEdit is the only shape the Library form can submit: no grading mode
// and no rubric. The store settles grading itself.
func learnerEdit(q GeneratedQuiz) GeneratedQuiz {
	q.Grading, q.Rubric = "", nil
	return q
}

func TestLearnerEditNeverRetainsARubricForChangedWording(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		mutate func(*GeneratedQuiz)
		keep   bool
	}{
		{"explanation only", func(q *GeneratedQuiz) {
			q.Explanation = "A 304 means the stored copy can be reused without downloading it again."
		}, true},
		{"surrounding whitespace only", func(q *GeneratedQuiz) { q.Prompt = "  " + q.Prompt + "\n" }, true},
		{"variant added", func(q *GeneratedQuiz) { q.Variants = []string{"It is confirmed current, so no body is resent."} }, true},
		{"prompt changed", func(q *GeneratedQuiz) { q.Prompt = "What does a 304 response let a browser do?" }, false},
		{"expected answer changed", func(q *GeneratedQuiz) { q.Answer = "The server says the copy is current." }, false},
		{"changed to choice", func(q *GeneratedQuiz) {
			q.Kind = "choice"
			q.Choices = []string{q.Answer, "The server always resends the body.", "The browser never asks the server."}
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newTestStore(t)
			src := publishFixture(t, s, generatedMeaning())
			before := src.Quizzes[0]
			edit := learnerEdit(generatedMeaning())
			tc.mutate(&edit)
			edited, err := s.EditQuiz(ctx, before.ID, before.Version, edit)
			if err != nil {
				t.Fatal(err)
			}
			if tc.keep {
				if edited.Grading != "semantic" || edited.Rubric == nil || len(edited.Rubric.Required) != 2 || edited.Rubric.Required[1].Text != "The body is not sent again" {
					t.Fatalf("unchanged wording lost its rubric: %+v", edited)
				}
			} else if edited.Grading != "" || edited.Rubric != nil {
				t.Fatalf("changed wording silently kept a stale rubric: %+v", edited)
			}
			// History: the original version is immutable and still semantic.
			var content string
			if err = s.db.QueryRowContext(ctx, "SELECT content FROM quiz_versions WHERE quiz_id=? AND version=1", before.ID).Scan(&content); err != nil {
				t.Fatal(err)
			}
			if original, err := s.Quiz(ctx, before.ID); err != nil || original.Version != 2 || content == "" {
				t.Fatalf("version history: %+v %v", original, err)
			}
			if !containsAll(content, `"grading":"semantic"`, "The body is not sent again") {
				t.Fatalf("original version was rewritten: %s", content)
			}
		})
	}
}

// A generated rubric is supported by one quotation. Pointing the same wording
// at a different quotation must not keep ideas the new evidence never stated.
func TestLearnerEditOfQuotedEvidenceDropsRubric(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	text := "Regular sleep may improve recall in some adults.\nRegular sleep can improve memory retention, according to this note.\nThese two sentences are a saved source, not a topic."
	src, err := s.Capture(ctx, text, newID())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := s.ClaimJob(ctx, time.Minute, 100, 10_000)
	if err != nil || claim == nil {
		t.Fatalf("claim: %+v %v", claim, err)
	}
	original := GeneratedQuiz{
		Kind: "recall", Grading: "semantic", Basis: "source",
		Prompt:      "According to your note, what may regular sleep do for some adults?",
		Answer:      "It may improve recall in some adults.",
		Explanation: "The note is hedged: sleep may improve recall, and only in some adults.",
		Evidence:    "Regular sleep may improve recall in some adults.",
		Rubric:      &Rubric{Required: []RubricIdea{{Text: "Regular sleep may improve recall in some adults"}}},
	}
	cost := int64(1)
	if err = s.CompleteJob(ctx, claim.ID, claim.LeaseToken, GenerationResult{Quizzes: []GeneratedQuiz{original}, Model: "authored-test-fixture", PromptVersion: "fixture-v1"}, &cost); err != nil {
		t.Fatal(err)
	}
	published, err := s.Source(ctx, src.ID)
	if err != nil || len(published.Quizzes) != 1 || published.Quizzes[0].Grading != "semantic" {
		t.Fatalf("publish: %+v %v", published, err)
	}
	same := learnerEdit(original)
	same.Explanation = "Only some adults, and only may: the note does not promise better recall."
	kept, err := s.EditQuiz(ctx, published.Quizzes[0].ID, 1, same)
	if err != nil || kept.Grading != "semantic" || kept.Rubric == nil {
		t.Fatalf("unchanged quotation lost its rubric: %+v %v", kept, err)
	}
	moved := learnerEdit(original)
	moved.Evidence = "Regular sleep can improve memory retention, according to this note."
	edited, err := s.EditQuiz(ctx, kept.ID, kept.Version, moved)
	if err != nil || edited.Grading != "" || edited.Rubric != nil {
		t.Fatalf("changed quotation kept an unsupported rubric: %+v %v", edited, err)
	}
}

func TestLearnerEditOfExactQuizStaysExact(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	src := publishFixture(t, s, GeneratedQuiz{Kind: "recall", Prompt: "Which header carries a strong validator?", Answer: "ETag", Explanation: "ETag carries an opaque validator for one representation.", Basis: "topic"})
	edit := GeneratedQuiz{Kind: "recall", Prompt: "Which response header carries an opaque validator?", Answer: "ETag", Explanation: "ETag carries an opaque validator for one representation.", Basis: "topic"}
	edited, err := s.EditQuiz(ctx, src.Quizzes[0].ID, 1, edit)
	if err != nil || edited.Grading != "" || edited.Rubric != nil {
		t.Fatalf("exact quiz changed grading on edit: %+v %v", edited, err)
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
