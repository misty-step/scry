package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

func authoredSemantic() GeneratedQuiz {
	return GeneratedQuiz{
		Kind: "recall", Grading: "semantic", Prompt: "How do HTTP validators support safe cache reuse?",
		Answer:      "They identify a stored representation and let the server confirm whether it changed.",
		Explanation: "A validator identifies a representation; a conditional request lets the origin confirm whether it is still current.",
		Basis:       "topic",
		Rubric: &Rubric{
			Required: []RubricIdea{
				{Text: "A validator identifies a stored representation", Cue: "Think about how a prior representation is recognized."},
				{Text: "The server confirms whether that representation changed"},
			},
			Contradictions: []RubricClaim{{Text: "A validator forces every response body to be downloaded", Feedback: "An unchanged response can be reused without downloading its body again."}},
		},
	}
}

func stageSemantic(t *testing.T, s *Store, operation, answer string) Presentation {
	t.Helper()
	state, err := s.Review(context.Background())
	if err != nil || state.Current == nil {
		t.Fatalf("open semantic review: %+v %v", state, err)
	}
	p, err := s.Submit(context.Background(), state.Current.ID, operation, answer, false)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Pending || p.Graded || p.AssessmentID == "" || p.Answer != answer || p.Quiz.Answer != "" {
		t.Fatalf("semantic answer was not staged safely: %+v", p)
	}
	return p
}

func beginSemantic(t *testing.T, s *Store, id string) Assessment {
	t.Helper()
	a, err := s.BeginAssessmentTransmission(context.Background(), id, "typesafe/jev-1.13", []byte(`{"model":"typesafe/jev-1.13","state":{},"questions":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func semanticResult(ideas []float64, relation string, relationProbability, contradiction, injection float64) AssessmentResult {
	cost := int64(19)
	return AssessmentResult{
		ResponseModel: "typesafe/jev-1.13-20260917",
		ResponseJSON:  []byte(`{"model":"typesafe/jev-1.13-20260917","answers":{"recorded":true}}`),
		Judgments: learning.SemanticJudgments{
			Ideas: ideas, Contradictions: []float64{contradiction}, Relation: relation,
			RelationProbabilities: map[string]float64{relation: relationProbability}, Injection: injection,
		},
		InputTokens: 100, OutputTokens: 20, CostMicros: &cost, LatencyMS: 42,
	}
}

func TestSemanticAssessmentStagesFinalizesAndReplaysIdempotently(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredSemantic())
	var beforeCard string
	if err := s.db.QueryRowContext(ctx, "SELECT card FROM schedules LIMIT 1").Scan(&beforeCard); err != nil {
		t.Fatal(err)
	}
	p := stageSemantic(t, s, "semantic-correct", "Validators name the cached representation so the server can say whether it changed.")
	var events int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM review_events").Scan(&events); err != nil || events != 0 {
		t.Fatalf("staging wrote a review event: %d %v", events, err)
	}
	a := beginSemantic(t, s, p.AssessmentID)
	if a.Transmissions != 1 || a.Answer != p.Answer || a.PolicyVersion != learning.SemanticPolicyVersion {
		t.Fatalf("bad durable transmission: %+v", a)
	}
	finalized, err := s.FinalizeAssessment(ctx, p.AssessmentID, semanticResult([]float64{0.96, 0.93}, "equivalent", 0.94, 0.02, 0.01))
	if err != nil {
		t.Fatal(err)
	}
	if !finalized.Graded || finalized.Outcome != "correct" || finalized.Rating != 3 || finalized.AssessmentStatus != "judged" {
		t.Fatalf("semantic correct was not finalized: %+v", finalized)
	}
	var grading, afterCard string
	if err = s.db.QueryRowContext(ctx, "SELECT grading FROM review_events WHERE presentation_id=?", p.ID).Scan(&grading); err != nil || grading != learning.SemanticPolicyVersion {
		t.Fatalf("missing grading provenance: %q %v", grading, err)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT card FROM schedules LIMIT 1").Scan(&afterCard); err != nil || afterCard == beforeCard {
		t.Fatalf("correct semantic grade did not advance schedule: %v", err)
	}
	replayed, err := s.Submit(ctx, p.ID, "semantic-correct", p.Answer, false)
	if err != nil || !replayed.Graded || replayed.ReviewID != finalized.ReviewID || replayed.AssessmentID != p.AssessmentID {
		t.Fatalf("judged operation replay changed state: %+v %v", replayed, err)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM review_events").Scan(&events); err != nil || events != 1 {
		t.Fatalf("replay duplicated review: %d %v", events, err)
	}
}

func TestSemanticPendingConflictsAndTransmissionLimitFailsSameOperation(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredSemantic())
	p := stageSemantic(t, s, "semantic-pending", "Caching validators can help with reuse.")
	if _, err := s.Submit(ctx, p.ID, "concurrent-other-operation", "Another answer", false); !errors.Is(err, ErrConflict) {
		t.Fatalf("concurrent pending submit was accepted: %v", err)
	}
	first := beginSemantic(t, s, p.AssessmentID)
	second := beginSemantic(t, s, p.AssessmentID)
	if first.Transmissions != 1 || second.Transmissions != 2 {
		t.Fatalf("transmission accounting changed: %d then %d", first.Transmissions, second.Transmissions)
	}
	replayed, err := s.Submit(ctx, p.ID, "semantic-pending", p.Answer, false)
	if err != nil || replayed.Pending || replayed.AssessmentStatus != "failed" || replayed.Graded || replayed.Answer != p.Answer {
		t.Fatalf("bounded replay did not fail safely: %+v %v", replayed, err)
	}
}

func TestSemanticFailurePreservesAnswerAndAllowsNewOperation(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredSemantic())
	p := stageSemantic(t, s, "semantic-failed", "A saved validator helps ask if data changed.")
	beginSemantic(t, s, p.AssessmentID)
	failed, err := s.FailAssessment(ctx, p.AssessmentID, AssessmentResult{Error: "unavailable", LatencyMS: 8000})
	if err != nil {
		t.Fatal(err)
	}
	if failed.Graded || failed.Pending || failed.AssessmentStatus != "failed" || failed.Answer != p.Answer || failed.Draft != p.Answer {
		t.Fatalf("failure lost or graded the answer: %+v", failed)
	}
	same, err := s.Submit(ctx, p.ID, "semantic-failed", p.Answer, false)
	if err != nil || same.AssessmentID != p.AssessmentID || same.AssessmentStatus != "failed" {
		t.Fatalf("failed operation replay changed assessment: %+v %v", same, err)
	}
	retry, err := s.Submit(ctx, p.ID, "semantic-failed-retry", p.Answer, false)
	if err != nil || !retry.Pending || retry.AssessmentID == p.AssessmentID {
		t.Fatalf("new operation did not stage explicit retry: %+v %v", retry, err)
	}
	oldReplay, err := s.Submit(ctx, p.ID, "semantic-failed", p.Answer, false)
	if err != nil || oldReplay.Pending || oldReplay.AssessmentID != p.AssessmentID || oldReplay.AssessmentStatus != "failed" || oldReplay.Answer != p.Answer {
		t.Fatalf("old failed replay inherited newer assessment: %+v %v", oldReplay, err)
	}
	var retryTransmissions int
	if err = s.db.QueryRowContext(ctx, "SELECT transmissions FROM semantic_assessments WHERE id=?", retry.AssessmentID).Scan(&retryTransmissions); err != nil || retryTransmissions != 0 {
		t.Fatalf("old replay resumed the newer assessment: transmissions=%d err=%v", retryTransmissions, err)
	}
}

func TestSemanticFinalizeSupersededChangesOnlyAssessment(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	src := publishFixture(t, s, authoredSemantic())
	p := stageSemantic(t, s, "semantic-stale", "Validators let a server check a cached copy.")
	beginSemantic(t, s, p.AssessmentID)
	updated := authoredSemantic()
	updated.Prompt = "Edited future semantic prompt?"
	if _, err := s.EditQuiz(ctx, src.Quizzes[0].ID, 1, updated); err != nil {
		t.Fatal(err)
	}
	finalized, err := s.FinalizeAssessment(ctx, p.AssessmentID, semanticResult([]float64{0.99, 0.99}, "equivalent", 0.99, 0.01, 0.01))
	if err != nil {
		t.Fatal(err)
	}
	if finalized.AssessmentStatus != "superseded" || finalized.Graded {
		t.Fatalf("stale finalization changed presentation: %+v", finalized)
	}
	var status string
	var events int
	if err = s.db.QueryRowContext(ctx, "SELECT status FROM semantic_assessments WHERE id=?", p.AssessmentID).Scan(&status); err != nil || status != "superseded" {
		t.Fatalf("assessment not superseded: %q %v", status, err)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM review_events").Scan(&events); err != nil || events != 0 {
		t.Fatalf("superseded assessment wrote event: %d %v", events, err)
	}
}

func TestSemanticCueFencesAssistanceAndLaterCorrectIsWarm(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredSemantic())
	p := stageSemantic(t, s, "semantic-incomplete", "The server can tell whether it changed.")
	var scheduleVersion int
	if err := s.db.QueryRowContext(ctx, "SELECT version FROM schedules LIMIT 1").Scan(&scheduleVersion); err != nil {
		t.Fatal(err)
	}
	beginSemantic(t, s, p.AssessmentID)
	incomplete, err := s.FinalizeAssessment(ctx, p.AssessmentID, semanticResult([]float64{0.10, 0.94}, "partial", 0.91, 0.02, 0.01))
	if err != nil {
		t.Fatal(err)
	}
	if incomplete.Graded || !incomplete.Assisted || incomplete.AssessmentDecision != "incomplete" || incomplete.AssessmentDetail == "" {
		t.Fatalf("cue was not assistance-fenced: %+v", incomplete)
	}
	warm, err := s.Submit(ctx, p.ID, "semantic-after-cue", authoredSemantic().Answer, false)
	if err != nil {
		t.Fatal(err)
	}
	if !warm.Graded || !warm.Assisted || warm.Outcome != "warm_correct" || warm.Rating != 0 {
		t.Fatalf("post-cue exact answer manufactured cold success: %+v", warm)
	}
	var afterVersion int
	if err = s.db.QueryRowContext(ctx, "SELECT version FROM schedules LIMIT 1").Scan(&afterVersion); err != nil || afterVersion != scheduleVersion {
		t.Fatalf("warm completion changed FSRS version: %d -> %d (%v)", scheduleVersion, afterVersion, err)
	}
	if _, err = s.Next(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil || state.Current != nil || state.NextDueAt != now.Add(24*time.Hour).UnixMilli() {
		t.Fatalf("warm availability fence failed: %+v %v", state, err)
	}
}

func TestEditQuizValidatesSemanticRubricAndDeterministicTaskBoundary(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	src := publishFixture(t, s, GeneratedQuiz{
		Kind: "recall", Prompt: "Original exact recall?", Answer: "Original answer", Explanation: "Original explanation.", Basis: "topic",
	})
	base := authoredSemantic()
	cases := []struct {
		name   string
		mutate func(*GeneratedQuiz)
	}{
		{"exact carries rubric", func(q *GeneratedQuiz) { q.Grading = "exact" }},
		{"semantic choice", func(q *GeneratedQuiz) { q.Kind = "choice"; q.Choices = []string{q.Answer, "Other"} }},
		{"numeric answer", func(q *GeneratedQuiz) { q.Answer = "1234" }},
		{"mixed single token", func(q *GeneratedQuiz) { q.Answer = "HTTP/2" }},
		{"missing required ideas", func(q *GeneratedQuiz) { q.Rubric.Required = nil }},
		{"cue leaks answer", func(q *GeneratedQuiz) { q.Rubric.Required[0].Cue = q.Answer }},
		{"cue leaks idea", func(q *GeneratedQuiz) { q.Rubric.Required[0].Cue = "Review: " + q.Rubric.Required[0].Text }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := base
			rubric := *base.Rubric
			rubric.Required = append([]RubricIdea(nil), base.Rubric.Required...)
			rubric.Contradictions = append([]RubricClaim(nil), base.Rubric.Contradictions...)
			q.Rubric = &rubric
			tc.mutate(&q)
			if _, err := s.EditQuiz(ctx, src.Quizzes[0].ID, 1, q); !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid semantic quiz accepted: %v", err)
			}
		})
	}
	if edited, err := s.EditQuiz(ctx, src.Quizzes[0].ID, 1, base); err != nil || edited.Grading != "semantic" || edited.Rubric == nil {
		t.Fatalf("valid semantic rubric rejected: %+v %v", edited, err)
	}
}

func TestSchemaV3ToV4MigrationAndAssessmentExport(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "populated-v3.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, schemaV1); err == nil {
		_, err = db.ExecContext(ctx, schemaV2)
	}
	if err == nil {
		_, err = db.ExecContext(ctx, schemaV3)
	}
	if err != nil {
		t.Fatal(err)
	}
	card, err := marshal(learning.NewCard(time.UnixMilli(1_700_000_000_000)))
	if err != nil {
		t.Fatal(err)
	}
	quizJSON := `{"kind":"recall","prompt":"Old v3 prompt","answer":"Old answer","explanation":"Old explanation","basis":"topic","choices":null,"variants":null}`
	statements := []string{
		`INSERT INTO sources VALUES('src','old topic','topic',1,0,1700000000000)`,
		`INSERT INTO source_revisions VALUES('src',1,'old topic','topic',1700000000000)`,
		`INSERT INTO jobs VALUES('job','src',1,'complete','','old-model','old-prompt',1,1700000000000,1700000000000,1700000000000,'',0,1,'{}')`,
		`INSERT INTO quizzes VALUES('quiz','src',1,0,1700000000000,'job',0)`,
		`INSERT INTO quiz_versions VALUES('quiz',1,'` + quizJSON + `','old-model','old-prompt',1700000000000)`,
		`INSERT INTO schedules VALUES('quiz',1,'` + card + `',1700000000000,'` + learning.Algorithm + `')`,
		`INSERT INTO concepts VALUES('concept','Old concept','Preserved',1700000000000)`,
		`INSERT INTO concept_quizzes VALUES('concept','quiz',1700000000000)`,
	}
	for _, statement := range statements {
		if _, err = db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	readonly, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := readonly.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = CheckSchema(ctx, tx, 3); err != nil {
		t.Fatal(err)
	}
	if err = errors.Join(tx.Rollback(), readonly.Close()); err != nil {
		t.Fatal(err)
	}
	afterCheck, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(before, afterCheck) {
		t.Fatalf("v3 check mutated database: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version, sources, concepts int
	if err = s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err == nil {
		err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM sources").Scan(&sources)
	}
	if err == nil {
		err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM concepts").Scan(&concepts)
	}
	if err != nil || version != 4 || sources != 1 || concepts != 1 {
		t.Fatalf("migration did not preserve v3 rows: version=%d sources=%d concepts=%d err=%v", version, sources, concepts, err)
	}

	// Add a semantic assessment in the migrated store and prove both new export
	// sections and the review-event grading column are portable.
	semantic := authoredSemantic()
	if _, err = s.EditQuiz(ctx, "quiz", 1, semantic); err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil || state.Current == nil {
		t.Fatalf("review migrated quiz: %+v %v", state, err)
	}
	staged, err := s.Submit(ctx, state.Current.ID, "migrated-semantic", "Validators support cache checks.", false)
	if err != nil || !staged.Pending {
		t.Fatalf("stage on migrated DB: %+v %v", staged, err)
	}
	exported, err := s.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err = json.Unmarshal(exported, &document); err != nil {
		t.Fatal(err)
	}
	var assessments []map[string]any
	var contentAssessments []map[string]any
	if err = json.Unmarshal(document["semantic_assessments"], &assessments); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(document["content_assessments"], &contentAssessments); err != nil {
		t.Fatal(err)
	}
	if len(assessments) != 1 || len(contentAssessments) != 0 || assessments[0]["request_json"] == nil {
		t.Fatalf("new assessment sections missing from export: semantic=%v content=%v", assessments, contentAssessments)
	}
}
