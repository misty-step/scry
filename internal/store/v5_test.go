package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

const tlsMaterial = "A TLS client trusts a server certificate only when the certificate chains to a root authority the client already trusts.\n" +
	"The client also checks that the certificate names the host it meant to reach.\n" +
	"Encryption alone does not prove who is on the other end of the connection."

func claimKind(t *testing.T, s *Store, kind string) *Job {
	t.Helper()
	j, err := s.ClaimJob(context.Background(), time.Minute, 100, 1_000_000)
	if err != nil || j == nil {
		t.Fatalf("claim %s: %v %v", kind, j, err)
	}
	if j.Kind != kind {
		t.Fatalf("claimed %s job, want %s", j.Kind, kind)
	}
	return j
}

func complete(t *testing.T, s *Store, j *Job, result GenerationResult) {
	t.Helper()
	cost := int64(50)
	if result.Model == "" {
		result.Model = "authored-test-fixture"
	}
	if result.PromptVersion == "" {
		result.PromptVersion = "fixture-v5"
	}
	if err := s.CompleteJob(context.Background(), j.ID, j.LeaseToken, result, &cost); err != nil {
		t.Fatalf("complete %s: %v", j.Kind, err)
	}
}

func tlsPlan() *PlanContent {
	return &PlanContent{Goal: "How a TLS client trusts a server", Concepts: []PlannedConcept{
		{Key: "chain", Name: "Certificate chain of trust", Summary: "A certificate is trusted when it chains to a root the client already trusts.",
			Note: &NoteContent{Level: "standard", Title: "Chains of trust", Basis: "source",
				Body:     "A server certificate is only as good as the authority behind it. The client follows the chain upward until it reaches a root it already trusts; if it never does, the certificate is not trusted.",
				Evidence: []string{"chains to a root authority the client already trusts"}}},
		{Key: "hostname", Name: "Hostname verification", Summary: "The certificate must name the host the client meant to reach.", Requires: []string{"chain"}, ConfusedWith: []string{"chain"},
			Note: &NoteContent{Level: "standard", Title: "Right certificate, right host", Basis: "source",
				Body:     "A valid chain is not enough: the certificate must also name the host the client asked for, otherwise a trusted certificate for another site could impersonate this one.",
				Evidence: []string{"names the host it meant to reach"}}},
	}}
}

// textPack captures the TLS material and runs plan and questions, returning
// the source and the two concept IDs (chain, hostname).
func textPack(t *testing.T, s *Store) (Source, string, string) {
	t.Helper()
	ctx := context.Background()
	src, err := s.Capture(ctx, CaptureInput{Text: tlsMaterial, Mode: "text"}, "capture-tls")
	if err != nil {
		t.Fatal(err)
	}
	complete(t, s, claimKind(t, s, "plan"), GenerationResult{Plan: tlsPlan()})
	questions := claimKind(t, s, "questions")
	jc, err := s.JobContext(ctx, questions.ID)
	if err != nil || len(jc.Concepts) != 2 || jc.Concepts[0].Note == nil {
		t.Fatalf("questions context: %+v %v", jc, err)
	}
	chain, hostname := jc.Concepts[0].ID, jc.Concepts[1].ID
	if jc.Concepts[0].Name != "Certificate chain of trust" || len(jc.Concepts[1].Requires) != 1 {
		t.Fatalf("questions context lost prerequisite order or relations: %+v", jc.Concepts)
	}
	complete(t, s, questions, GenerationResult{Quizzes: []GeneratedQuiz{
		{Kind: "choice", Level: "recognize", Concept: chain, Prompt: "When does a TLS client trust a server certificate's issuer?", Answer: "When it chains to a root the client already trusts",
			Choices:        []string{"When it chains to a root the client already trusts", "When the connection is encrypted", "When the certificate names the host"},
			ChoiceConcepts: []string{"", "", hostname}, Explanation: "Trust comes from the chain to a known root, not from encryption or the name alone.",
			Basis: "source", Evidence: "chains to a root authority the client already trusts"},
		{Kind: "recall", Level: "recall", AnswerForm: "flexible", Concept: hostname, Prompt: "Besides the chain of trust, what must a TLS certificate match?", Answer: "The hostname",
			Explanation: "The certificate must name the host the client meant to reach.", Basis: "source", Evidence: "names the host it meant to reach"},
	}})
	src, err = s.Source(ctx, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	return src, chain, hostname
}

// US-005: the capture mode is the learner's explicit choice and decides where
// the material may go. Only a topic is researched on the web; pasted text is
// planned privately; links are read; photos are transcribed.
func TestCaptureModesUS005(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 64)...)
	for _, tc := range []struct {
		in        CaptureInput
		kind, job string
		web       bool
	}{
		{CaptureInput{Text: "How HTTPS works", Mode: "topic"}, "topic", "research", true},
		{CaptureInput{Text: "My private notes: short", Mode: "text"}, "source", "plan", false},
		{CaptureInput{Text: " https://example.com/article#section ", Mode: "link"}, "source", "research", false},
		{CaptureInput{Text: "Page 12", Mode: "photo", Image: jpeg, ImageMIME: "image/jpeg"}, "source", "transcribe", false},
	} {
		src, err := s.Capture(ctx, tc.in, "capture-"+tc.in.Mode)
		if err != nil {
			t.Fatalf("%s: %v", tc.in.Mode, err)
		}
		if src.Kind != tc.kind || src.Mode != tc.in.Mode || src.Web != tc.web || src.Job == nil || src.Job.Kind != tc.job || src.GoalID == "" {
			t.Fatalf("%s capture = %+v (job %+v)", tc.in.Mode, src, src.Job)
		}
		replay, err := s.Capture(ctx, tc.in, "capture-"+tc.in.Mode)
		if err != nil || replay.ID != src.ID || len(replay.Jobs) != 1 {
			t.Fatalf("%s capture replay duplicated work: %+v %v", tc.in.Mode, replay, err)
		}
	}
	link, err := s.Source(ctx, mustSourceByMode(t, s, "link"))
	if err != nil || link.Text != "https://example.com/article" {
		t.Fatalf("link was not normalized: %q %v", link.Text, err)
	}
	image, err := s.CaptureImage(ctx, mustSourceByMode(t, s, "photo"))
	if err != nil || image.MIME != "image/jpeg" || len(image.Bytes) != len(jpeg) {
		t.Fatalf("photo not stored: %+v %v", image, err)
	}
	for _, bad := range []CaptureInput{
		{Text: "No mode chosen"},
		{Text: "javascript:alert(1)", Mode: "link"},
		{Text: "caption", Mode: "photo", Image: []byte("not an image")},
		{Text: "caption", Mode: "photo", Image: jpeg, ImageMIME: "image/png"},
		{Text: strings.Repeat("x", maxTopicBytes+1), Mode: "topic"},
	} {
		if _, err := s.Capture(ctx, bad, newID()); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid capture %+v accepted: %v", bad.Mode, err)
		}
	}
	// The only enqueue path refuses web research for material the learner did
	// not capture as a Topic or a Link.
	private := []string{mustSourceByMode(t, s, "text"), mustSourceByMode(t, s, "photo")}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, id := range private {
		if err = enqueue(ctx, tx, id, 1, "research", nil, 1); !errors.Is(err, ErrInvalid) {
			t.Fatalf("research enqueued for private material %s: %v", id, err)
		}
	}
}

func mustSourceByMode(t *testing.T, s *Store, mode string) string {
	t.Helper()
	var id string
	if err := s.db.QueryRow("SELECT id FROM sources WHERE mode=?", mode).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// US-006: a capture becomes concepts with notes and questions linked to them;
// a new concept is introduced by its note before its first question, and the
// concept page shows the note, questions, and relations.
func TestConceptChainAndIntroUS006(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	src, chain, hostname := textPack(t, s)
	if src.Status != "ready" || len(src.Quizzes) != 2 || len(src.Jobs) != 2 {
		t.Fatalf("chain did not finish: status=%s quizzes=%d jobs=%d", src.Status, len(src.Quizzes), len(src.Jobs))
	}
	for _, q := range src.Quizzes {
		if q.ConceptID == "" {
			t.Fatalf("question %q is not linked to a concept", q.Prompt)
		}
	}
	state, err := s.Review(ctx)
	if err != nil || state.Current != nil || state.Intro == nil || state.Intro.Concept.ID != chain || state.Intro.Note == nil {
		t.Fatalf("first new concept was not introduced by its note: %+v %v", state, err)
	}
	if again, err := s.Review(ctx); err != nil || again.Intro == nil || again.Intro.Concept.ID != chain {
		t.Fatalf("an unacknowledged intro did not persist across reads: %+v %v", again, err)
	}
	*now = now.Add(time.Second)
	state, err = s.AcknowledgeIntro(ctx, chain, "intro-chain", false)
	if err != nil {
		t.Fatal(err)
	}
	// US-011: hostname requires chain, so it waits while chain has no unaided
	// success; chain's own question comes right after its note.
	if state.Intro != nil || state.Current == nil || state.Current.Concept == nil || state.Current.Concept.ID != chain {
		t.Fatalf("prerequisite question did not follow its intro: %+v", state)
	}
	if state.CurrentConcept == nil || state.CurrentConcept.ID != chain || state.CurrentConcept.Status != "new" {
		t.Fatalf("current concept chip state missing: %+v", state.CurrentConcept)
	}
	if replay, err := s.AcknowledgeIntro(ctx, chain, "intro-chain", false); err != nil || replay.Current == nil || replay.Current.ID != state.Current.ID {
		t.Fatalf("intro acknowledgment replay changed the stream: %+v %v", replay, err)
	}
	*now = now.Add(time.Second)
	answered, err := s.Submit(ctx, state.Current.ID, "chain-answer", "When it chains to a root the client already trusts", false)
	if err != nil || answered.Outcome != "correct" {
		t.Fatalf("chain answer: %+v %v", answered, err)
	}
	if state, err = s.Next(ctx, answered.ID); err != nil || state.Intro == nil || state.Intro.Concept.ID != hostname {
		t.Fatalf("a met prerequisite did not unlock the dependent concept's intro: %+v %v", state, err)
	}
	state, err = s.AcknowledgeIntro(ctx, hostname, "intro-hostname", false)
	if err != nil || state.Current == nil || state.Current.Concept == nil || state.Current.Concept.ID != hostname {
		t.Fatalf("dependent concept's question did not follow its intro: %+v %v", state, err)
	}
	view, err := s.ConceptPage(ctx, hostname)
	if err != nil {
		t.Fatal(err)
	}
	if view.Notes["standard"] == nil || len(view.Questions) != 1 || len(view.Requires) != 1 || view.Requires[0].ID != chain || len(view.ConfusedWith) != 1 {
		t.Fatalf("concept page missing note, questions, or relations: %+v", view)
	}
	chainView, err := s.ConceptPage(ctx, chain)
	if err != nil || len(chainView.RequiredBy) != 1 || len(chainView.ConfusedWith) != 1 {
		t.Fatalf("reverse relations missing: %+v %v", chainView, err)
	}
	m, err := s.Map(ctx, "")
	if err != nil || len(m.Goals) != 1 || m.Goals[0].Goal.Title != "How a TLS client trusts a server" || len(m.Goals[0].Concepts) != 2 || len(m.Goals[0].Edges) != 1 {
		t.Fatalf("map did not show the goal constellation: %+v %v", m, err)
	}
	hits, err := s.Map(ctx, "hostnam")
	if err != nil || len(hits.Hits) == 0 {
		t.Fatalf("prefix search found nothing: %+v %v", hits.Hits, err)
	}
	for _, h := range hits.Hits {
		if h.Kind == "question" && h.Snippet != "" {
			t.Fatalf("question search leaked answer-bearing text: %+v", h)
		}
	}
}

// Provenance fences hold at publication: material-based notes quote the
// material, general knowledge claims no evidence, and an invalid plan leaves
// no partial rows behind its recorded failure.
func TestPublicationProvenanceFences(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	if _, err := s.Capture(ctx, CaptureInput{Text: tlsMaterial, Mode: "text"}, "fence-text"); err != nil {
		t.Fatal(err)
	}
	plan := tlsPlan()
	plan.Concepts[0].Note.Evidence = []string{"a sentence that is not in the material"}
	j := claimKind(t, s, "plan")
	cost := int64(10)
	if err := s.CompleteJob(ctx, j.ID, j.LeaseToken, GenerationResult{Plan: plan, Model: "m", PromptVersion: "p"}, &cost); !errors.Is(err, ErrInvalid) {
		t.Fatalf("an invented quotation was published: %v", err)
	}
	var concepts, notes, queued int
	if err := s.db.QueryRow("SELECT (SELECT count(*) FROM concepts),(SELECT count(*) FROM notes),(SELECT count(*) FROM jobs WHERE status='queued')").Scan(&concepts, &notes, &queued); err != nil {
		t.Fatal(err)
	}
	if concepts != 0 || notes != 0 || queued != 0 {
		t.Fatalf("a rejected plan left rows behind: concepts=%d notes=%d queued=%d", concepts, notes, queued)
	}
	topic, err := s.Capture(ctx, CaptureInput{Text: "How HTTPS works", Mode: "topic"}, "fence-topic")
	if err != nil {
		t.Fatal(err)
	}
	complete(t, s, claimKind(t, s, "research"), GenerationResult{Note: "Web search was unavailable; general knowledge only."})
	topicPlan := &PlanContent{Goal: "How HTTPS works", Concepts: []PlannedConcept{{Key: "tls", Name: "TLS", Summary: "The protocol HTTPS runs over.",
		Note: &NoteContent{Level: "standard", Title: "TLS", Basis: "topic", Body: "TLS wraps HTTP in an encrypted, authenticated channel so neither eavesdroppers nor impostors can read or alter it.", Evidence: []string{"quoted"}}}}}
	j = claimKind(t, s, "plan")
	if err = s.CompleteJob(ctx, j.ID, j.LeaseToken, GenerationResult{Plan: topicPlan, Model: "m", PromptVersion: "p"}, &cost); !errors.Is(err, ErrInvalid) {
		t.Fatalf("general knowledge claimed evidence: %v", err)
	}
	webPlan := *topicPlan
	webPlan.Concepts = []PlannedConcept{topicPlan.Concepts[0]}
	webNote := *topicPlan.Concepts[0].Note
	webNote.Basis, webNote.Evidence = "web", []string{"encrypted"}
	webPlan.Concepts[0].Note = &webNote
	if _, err = s.RetrySource(ctx, topic.ID, "retry-topic-plan"); err != nil {
		t.Fatal(err)
	}
	j = claimKind(t, s, "plan")
	if err = s.CompleteJob(ctx, j.ID, j.LeaseToken, GenerationResult{Plan: &webPlan, Model: "m", PromptVersion: "p"}, &cost); !errors.Is(err, ErrInvalid) {
		t.Fatalf("web basis without stored web results was published: %v", err)
	}
}

// Web-grounded notes quote and cite a stored search result; citations are
// normalized from the stored document, not the model's claimed title or URL.
func TestWebBasisCitesStoredResults(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	src, err := s.Capture(ctx, CaptureInput{Text: "How HTTPS works", Mode: "topic"}, "web-topic")
	if err != nil {
		t.Fatal(err)
	}
	complete(t, s, claimKind(t, s, "research"), GenerationResult{Documents: []DocumentContent{{Kind: "search_result", URL: "https://example.org/tls", Title: "TLS explained", Text: "TLS encrypts traffic and authenticates the server with certificates.", Provider: "exa"}}})
	planJob := claimKind(t, s, "plan")
	jc, err := s.JobContext(ctx, planJob.ID)
	if err != nil || len(jc.Documents) != 1 {
		t.Fatalf("plan did not receive the stored result: %+v %v", jc, err)
	}
	doc := jc.Documents[0]
	complete(t, s, planJob, GenerationResult{Plan: &PlanContent{Goal: "How HTTPS works", Concepts: []PlannedConcept{{Key: "tls", Name: "TLS authentication", Summary: "Certificates let TLS authenticate the server.",
		Note: &NoteContent{Level: "standard", Title: "TLS", Basis: "web", Body: "TLS does two jobs at once: it encrypts the traffic and it authenticates the server using certificates.",
			Evidence: []string{"authenticates the server with certificates"}, Citations: []Citation{{DocumentID: doc.ID, Title: "spoofed", URL: "https://evil.example"}}}}}}})
	view, err := s.Map(ctx, "")
	if err != nil || len(view.Goals) != 1 {
		t.Fatalf("goal missing: %+v %v", view, err)
	}
	page, err := s.ConceptPage(ctx, view.Goals[0].Concepts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	note := page.Notes["standard"]
	if note == nil || len(note.Citations) != 1 || note.Citations[0].Title != "TLS explained" || note.Citations[0].URL != "https://example.org/tls" {
		t.Fatalf("citation was not normalized from the stored document: %+v", note)
	}
	if len(page.Documents) != 1 || page.Documents[0].Text != "" {
		t.Fatalf("concept page should list cited documents without their text: %+v", page.Documents)
	}
	if src.Web != true {
		t.Fatal("topic capture did not record web consent")
	}
}

func answerCurrent(t *testing.T, s *Store, op, answer string) Presentation {
	t.Helper()
	state, err := s.Review(context.Background())
	if err != nil || state.Current == nil {
		t.Fatalf("no current question: %+v %v", state, err)
	}
	p, err := s.Submit(context.Background(), state.Current.ID, op, answer, false)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// US-007: one tap contests an automatic miss. The original event stays; the
// override recomputes the schedule from the pre-review card as if graded
// correct, and only while that review is still the latest transition.
func TestOverrideAutomaticGradeUS007(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, GeneratedQuiz{Kind: "recall", Prompt: "Which protocol secures HTTPS?", Answer: "TLS", Explanation: "HTTPS runs HTTP over TLS.", Basis: "topic"})
	missed := answerCurrent(t, s, "miss", "the TLS protocol")
	if !missed.Graded || missed.Outcome != "wrong" || missed.Rating != 1 || missed.Authority != "exact" {
		t.Fatalf("exact short mismatch should be an automatic miss: %+v", missed)
	}
	var before string
	if err := s.db.QueryRow("SELECT card FROM schedules").Scan(&before); err != nil {
		t.Fatal(err)
	}
	overridden, err := s.OverrideGrade(ctx, missed.ID, "override", true)
	if err != nil || overridden.Override != "correct" || overridden.DueAt <= missed.DueAt {
		t.Fatalf("override did not count the answer correct: %+v %v", overridden, err)
	}
	replay, err := s.OverrideGrade(ctx, missed.ID, "override", true)
	if err != nil || !reflect.DeepEqual(replay, overridden) {
		t.Fatal("override retry changed its result")
	}
	if _, err = s.OverrideGrade(ctx, missed.ID, "override-again", true); !errors.Is(err, ErrConflict) {
		t.Fatalf("a second override was accepted: %v", err)
	}
	var events, overrides, version int
	var outcome string
	if err = s.db.QueryRow("SELECT (SELECT count(*) FROM review_events),(SELECT outcome FROM review_events),(SELECT count(*) FROM grade_overrides),(SELECT version FROM schedules)").Scan(&events, &outcome, &overrides, &version); err != nil {
		t.Fatal(err)
	}
	if events != 1 || outcome != "wrong" || overrides != 1 || version != 3 {
		t.Fatalf("override rewrote history or schedule versions: events=%d outcome=%s overrides=%d version=%d", events, outcome, overrides, version)
	}
	history, err := s.History(ctx, 10)
	if err != nil || len(history) != 1 || history[0].Override != "correct" || history[0].Authority != "exact" {
		t.Fatalf("history does not show the correction: %+v %v", history, err)
	}
	// A later review closes the window: correcting then belongs to History.
	if _, err = s.Next(ctx, missed.ID); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(30 * 24 * time.Hour)
	later := answerCurrent(t, s, "later-miss", "SSL")
	if _, err = s.Next(ctx, later.ID); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Hour)
	if _, err = s.OverrideGrade(ctx, later.ID, "late-override", true); err != nil {
		t.Fatalf("the latest review could not be corrected: %v", err)
	}
}

// US-008: a flexible recall answer that does not match exactly is checked by
// meaning (short-v1): accept grades correct, a confident reject grades a miss,
// and anything else asks the learner, all under the policy that decided.
func TestShortAnswerStagingAndFinalizationUS008(t *testing.T) {
	for _, tc := range []struct {
		name, verdict string
		probability   float64
		outcome       string
		graded        bool
		self          bool
	}{
		{"accept", "accept", 0.95, "correct", true, false},
		{"reject", "reject", 0.97, "wrong", true, false},
		{"unsure", "unsure", 0.90, "ungraded", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newTestStore(t)
			ctx := context.Background()
			publishFixture(t, s, GeneratedQuiz{Kind: "recall", AnswerForm: "flexible", Prompt: "Which protocol secures HTTPS?", Answer: "TLS", Explanation: "HTTPS runs HTTP over TLS.", Basis: "topic"})
			pending := answerCurrent(t, s, "short-"+tc.name, "the TLS protocol")
			if !pending.Pending || pending.Quiz.Answer != "" {
				t.Fatalf("flexible mismatch was not staged for a meaning check: %+v", pending)
			}
			var policy string
			if err := s.db.QueryRow("SELECT policy_version FROM semantic_assessments WHERE id=?", pending.AssessmentID).Scan(&policy); err != nil || policy != "short-v1" {
				t.Fatalf("staged policy = %q %v", policy, err)
			}
			lease, err := s.BeginAssessmentTransmission(ctx, pending.AssessmentID, "typesafe/jev-1.13", []byte(`{}`), testSpending)
			if err != nil || !lease.Send {
				t.Fatalf("lease: %+v %v", lease, err)
			}
			result := AssessmentResult{ResponseModel: "typesafe/jev-1.13", ResponseJSON: []byte(`{}`), Short: shortJudgments(tc.verdict, tc.probability)}
			final, err := s.FinalizeAssessment(ctx, pending.AssessmentID, lease.Token, result)
			if err != nil {
				t.Fatal(err)
			}
			if final.Graded != tc.graded || final.Outcome != tc.outcome || final.SelfCheck != tc.self {
				t.Fatalf("finalized %+v", final)
			}
			if tc.graded {
				var grading string
				if err = s.db.QueryRow("SELECT grading FROM review_events WHERE rating>0").Scan(&grading); err != nil || grading != "short-v1" {
					t.Fatalf("event grading = %q %v", grading, err)
				}
			}
		})
	}
}

func shortJudgments(verdict string, p float64) learning.ShortJudgments {
	return learning.ShortJudgments{Verdict: verdict, Probabilities: map[string]float64{verdict: p}, Identity: 0.02, Injection: 0.01}
}

// US-012: choosing a distractor that stands for another concept records a
// confusion; the second confusion between the same pair schedules one
// contrast job, and further confusions do not schedule more.
func TestConfusionsScheduleContrastUS012(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	_, chain, hostname := textPack(t, s)
	for _, id := range []string{chain, hostname} {
		if _, err := s.AcknowledgeIntro(ctx, id, "intro-"+id, false); err != nil {
			t.Fatal(err)
		}
	}
	confuse := func(op string) {
		t.Helper()
		for i := 0; i < 4; i++ {
			state, err := s.Review(ctx)
			if err != nil || state.Current == nil {
				t.Fatalf("no current question: %+v %v", state, err)
			}
			p := *state.Current
			if p.Quiz.Kind == "choice" {
				if _, err = s.Submit(ctx, p.ID, op, "When the certificate names the host", false); err != nil {
					t.Fatal(err)
				}
				if _, err = s.Next(ctx, p.ID); err != nil {
					t.Fatal(err)
				}
				return
			}
			if _, err = s.Submit(ctx, p.ID, op+"-skip", "", true); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Next(ctx, p.ID); err != nil {
				t.Fatal(err)
			}
			*now = now.Add(20 * time.Minute)
		}
		t.Fatal("the choice question never came up")
	}
	confuse("confuse-1")
	var contrasts int
	if err := s.db.QueryRow("SELECT count(*) FROM jobs WHERE kind='contrast'").Scan(&contrasts); err != nil || contrasts != 0 {
		t.Fatalf("one confusion scheduled a contrast: %d %v", contrasts, err)
	}
	*now = now.Add(24 * time.Hour)
	confuse("confuse-2")
	*now = now.Add(24 * time.Hour)
	confuse("confuse-3")
	var confusions int
	var payload string
	if err := s.db.QueryRow("SELECT (SELECT count(*) FROM evidence WHERE kind='confusion'),(SELECT count(*) FROM jobs WHERE kind='contrast'),(SELECT payload FROM jobs WHERE kind='contrast')").Scan(&confusions, &contrasts, &payload); err != nil {
		t.Fatal(err)
	}
	var pair struct {
		Concepts []string `json:"concepts"`
	}
	if confusions != 3 || contrasts != 1 || json.Unmarshal([]byte(payload), &pair) != nil || len(pair.Concepts) != 2 || pair.Concepts[0] != chain || pair.Concepts[1] != hostname {
		t.Fatalf("confusions=%d contrasts=%d payload=%s", confusions, contrasts, payload)
	}
	// The contrast question links both concepts, so answer-secrecy gates
	// cover the one it contrasts with as well as the one it assesses.
	contrast := claimKind(t, s, "contrast")
	complete(t, s, contrast, GenerationResult{Quizzes: []GeneratedQuiz{{Kind: "recall", Level: "recall", AnswerForm: "flexible", Concept: chain, Also: []string{hostname},
		Prompt: "Which check proves the server is the host you asked for, not just trusted by someone?", Answer: "Hostname verification",
		Explanation: "A trusted chain alone can belong to another site; the name check ties it to this host.", Basis: "source", Evidence: "names the host it meant to reach"}}})
	var quizID string
	if err := s.db.QueryRow("SELECT quiz_id FROM concept_quizzes WHERE role='contrasts'").Scan(&quizID); err != nil {
		t.Fatal(err)
	}
	linked, err := s.QuizConcepts(context.Background(), quizID)
	if err != nil || len(linked) != 2 || !reflect.DeepEqual(map[string]bool{linked[0]: true, linked[1]: true}, map[string]bool{chain: true, hostname: true}) {
		t.Fatalf("contrast question links %v, want both %s and %s: %v", linked, chain, hostname, err)
	}
}

// US-010: pausing a goal stops its new material; resuming brings it back.
func TestGoalPauseStopsNewMaterialUS010(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	textPack(t, s)
	m, err := s.Map(ctx, "")
	if err != nil || len(m.Goals) != 1 {
		t.Fatal(err)
	}
	goal := m.Goals[0].Goal.ID
	if err = s.UpdateGoal(ctx, goal, "pause"); err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil || state.Current != nil || state.Intro != nil {
		t.Fatalf("a paused goal introduced new material: %+v %v", state, err)
	}
	if err = s.UpdateGoal(ctx, goal, "resume"); err != nil {
		t.Fatal(err)
	}
	if state, err = s.Review(ctx); err != nil || state.Intro == nil {
		t.Fatalf("a resumed goal did not introduce its material: %+v %v", state, err)
	}
	if err = s.UpdateGoal(ctx, goal, "delete"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown goal action accepted: %v", err)
	}
}

// Practice focus serves the chosen concept first, even before its
// prerequisites are met; a concept not yet introduced shows its note first.
func TestPracticeFocusAndOnDemandRequests(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	_, chain, hostname := textPack(t, s)
	if err := s.PracticeConcept(ctx, hostname, "focus-host"); err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil || state.Intro == nil || state.Intro.Concept.ID != hostname {
		t.Fatalf("practice focus on a new concept did not introduce it: %+v %v", state, err)
	}
	state, err = s.AcknowledgeIntro(ctx, hostname, "focus-intro", false)
	if err != nil || state.Current == nil || state.Current.Concept == nil || state.Current.Concept.ID != hostname {
		t.Fatalf("practice focus did not serve its concept: %+v %v", state, err)
	}
	if err = s.RequestNote(ctx, chain, "simpler", "note-simpler"); err != nil {
		t.Fatal(err)
	}
	if err = s.RequestNote(ctx, chain, "simpler", "note-simpler"); err != nil {
		t.Fatalf("request replay failed: %v", err)
	}
	if err = s.RequestQuestions(ctx, chain, "more-questions"); !errors.Is(err, ErrConflict) {
		t.Fatalf("a second live job on the same material was accepted: %v", err)
	}
	page, err := s.ConceptPage(ctx, chain)
	if err != nil || len(page.PendingLevels) != 1 || page.PendingLevels[0] != "simpler" {
		t.Fatalf("pending note level not shown: %+v %v", page.PendingLevels, err)
	}
	j := claimKind(t, s, "note")
	jc, err := s.JobContext(ctx, j.ID)
	if err != nil || jc.Level != "simpler" || len(jc.Concepts) != 1 || jc.Concepts[0].ID != chain {
		t.Fatalf("note context: %+v %v", jc, err)
	}
	complete(t, s, j, GenerationResult{StudyNote: &NoteContent{Level: "simpler", Title: "Who vouches for whom", Basis: "source",
		Body: "Think of a chain of people vouching for each other: you trust the stranger because someone you already trust vouched for them.", Evidence: []string{"chains to a root authority"}}})
	page, err = s.ConceptPage(ctx, chain)
	if err != nil || page.Notes["simpler"] == nil || page.Notes["standard"] == nil {
		t.Fatalf("simpler note not published beside the standard note: %+v %v", page.Notes, err)
	}
	if err = s.ArchiveConcept(ctx, hostname); err != nil {
		t.Fatal(err)
	}
	var archived int
	if err = s.db.QueryRow("SELECT count(*) FROM quizzes q JOIN concept_quizzes cq ON cq.quiz_id=q.id WHERE cq.concept_id=? AND q.archived=1", hostname).Scan(&archived); err != nil || archived != 1 {
		t.Fatalf("archiving a concept kept its questions active: %d %v", archived, err)
	}
}

// An override from a stale result page moves the schedule; a newer unanswered
// occurrence of the same question must not be stranded at the old version.
func TestOverrideRetiresStrandedOccurrence(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, GeneratedQuiz{Kind: "recall", Prompt: "Which protocol secures HTTPS?", Answer: "TLS", Explanation: "HTTPS runs HTTP over TLS.", Basis: "topic"})
	missed := answerCurrent(t, s, "first-miss", "SSL")
	if _, err := s.Next(ctx, missed.ID); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(10 * time.Minute)
	again, err := s.Review(ctx)
	if err != nil || again.Current == nil || again.Current.ID == missed.ID || again.Current.Quiz.ID != missed.Quiz.ID {
		t.Fatalf("the missed question was not presented again: %+v %v", again, err)
	}
	if _, err = s.OverrideGrade(ctx, missed.ID, "stale-override", true); err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Current != nil && state.Current.ID == again.Current.ID {
		if _, err = s.Submit(ctx, state.Current.ID, "stranded-answer", "TLS", false); err != nil {
			t.Fatalf("the re-presented occurrence can no longer be answered: %v", err)
		}
	}
}

// Output that arrives after its concept was archived is refused, so an
// archived concept never returns as an intro the learner cannot dismiss.
func TestLateQuestionsForArchivedConceptAreRefused(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	_, chain, _ := textPack(t, s)
	if err := s.RequestQuestions(ctx, chain, "late-questions"); err != nil {
		t.Fatal(err)
	}
	j := claimKind(t, s, "questions")
	if err := s.ArchiveConcept(ctx, chain); err != nil {
		t.Fatal(err)
	}
	cost := int64(10)
	late := GenerationResult{Model: "m", PromptVersion: "p", Quizzes: []GeneratedQuiz{{Kind: "recall", Level: "recall", AnswerForm: "exact", Concept: chain,
		Prompt: "What must a certificate chain end at?", Answer: "A trusted root", Explanation: "Trust comes from a root the client already trusts.", Basis: "source", Evidence: "chains to a root authority the client already trusts"}}}
	if err := s.CompleteJob(ctx, j.ID, j.LeaseToken, late, &cost); !errors.Is(err, ErrInvalid) {
		t.Fatalf("late questions were published for an archived concept: %v", err)
	}
	var live int
	if err := s.db.QueryRow("SELECT count(*) FROM quizzes q JOIN concept_quizzes cq ON cq.quiz_id=q.id WHERE cq.concept_id=? AND q.archived=0", chain).Scan(&live); err != nil || live != 0 {
		t.Fatalf("archived concept has live questions: %d %v", live, err)
	}
}

// The store hides distractor tags until the answer is graded: the correct
// choice is the one untagged, so the tags alone would reveal it.
func TestUngradedChoiceHidesDistractorConcepts(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	_, chain, hostname := textPack(t, s)
	for _, id := range []string{chain, hostname} {
		if _, err := s.AcknowledgeIntro(ctx, id, "tags-intro-"+id, false); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		state, err := s.Review(ctx)
		if err != nil || state.Current == nil {
			t.Fatalf("no question: %+v %v", state, err)
		}
		current, err := s.Current(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if state.Current.Quiz.Kind == "choice" {
			if len(state.Current.Quiz.ChoiceConcepts) != 0 || len(current.Quiz.ChoiceConcepts) != 0 {
				t.Fatalf("ungraded choice exposed its distractor tags: %v %v", state.Current.Quiz.ChoiceConcepts, current.Quiz.ChoiceConcepts)
			}
			return
		}
		if _, err = s.Submit(ctx, state.Current.ID, "tags-skip-"+state.Current.ID, "", true); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Next(ctx, state.Current.ID); err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("the choice question never came up")
}
