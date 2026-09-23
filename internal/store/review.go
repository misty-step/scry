package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

// Warm completion consumes this exact content/schedule version for one day.
// This is practice availability, not an FSRS transition. A newer review or
// explicit schedule reset wins because its schedule version no longer matches.
const quizAvailableAtSQL = `max(sc.due_at,COALESCE((SELECT max(e.reviewed_at)+86400000
 FROM review_events e JOIN presentations consumed ON consumed.id=e.presentation_id
 WHERE consumed.quiz_id=q.id AND consumed.content_version=q.version
 AND e.schedule_version_after=sc.version AND e.rating=0 AND e.assisted=1
 AND substr(e.outcome,1,5)='warm_'),0))`

const availabilityCountsSQL = `SELECT count(*),COALESCE(sum(available_at<=?),0),
 COALESCE(min(CASE WHEN available_at>? THEN available_at END),0)
 FROM (SELECT ` + quizAvailableAtSQL + ` AS available_at FROM quizzes q
 JOIN sources src ON src.id=q.source_id JOIN schedules sc ON sc.quiz_id=q.id
 WHERE q.archived=0 AND src.archived=0)`

const (
	recentlySeenWindow = 10 * 60 * 1000
	remedialWindow     = 24 * 60 * 60 * 1000
	impliedWindow      = 14 * 24 * 60 * 60 * 1000
	focusWindow        = 30 * 60 * 1000
)

// Review may establish the one current occurrence, but never clears feedback,
// grades an answer, or advances a schedule. A preview has no submission token.
func (s *Store) Review(ctx context.Context) (ReviewState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReviewState{}, err
	}
	defer tx.Rollback()
	state, err := reviewState(ctx, tx, s.now(), true)
	if err != nil {
		return ReviewState{}, err
	}
	if err = tx.Commit(); err != nil {
		return ReviewState{}, err
	}
	return state, nil
}

// Current reads only an already-established occurrence. Library/help guards use
// it without turning browsing into a new review or an assistance event.
func (s *Store) Current(ctx context.Context) (*Presentation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id sql.NullString
	if err = tx.QueryRowContext(ctx, "SELECT current_id FROM review_session WHERE singleton=1").Scan(&id); err != nil {
		return nil, err
	}
	if !id.Valid {
		return nil, tx.Commit()
	}
	p, err := presentation(ctx, tx, id.String)
	if err != nil {
		return nil, err
	}
	hideAnswer(&p)
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) Next(ctx context.Context, presentationID string) (ReviewState, error) {
	if presentationID == "" {
		return ReviewState{}, fmt.Errorf("%w: missing presentation ID", ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReviewState{}, err
	}
	defer tx.Rollback()
	// The predicate is the entire advancement authority. Repeated/stale Next
	// cannot consume a new prompt, an ungraded response, or another held result.
	advance, err := tx.ExecContext(ctx, `UPDATE review_session SET current_id=NULL WHERE singleton=1 AND current_id=?
	 AND EXISTS(SELECT 1 FROM presentations WHERE id=? AND graded=1)`, presentationID, presentationID)
	if err != nil {
		return ReviewState{}, err
	}
	advanced, err := advance.RowsAffected()
	if err != nil {
		return ReviewState{}, err
	}
	state, err := reviewState(ctx, tx, s.now(), advanced == 1)
	if err != nil {
		return ReviewState{}, err
	}
	if err = tx.Commit(); err != nil {
		return ReviewState{}, err
	}
	return state, nil
}

func reviewState(ctx context.Context, tx *sql.Tx, now int64, allowNew bool) (ReviewState, error) {
	var state ReviewState
	var current sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT current_id FROM review_session WHERE singleton=1").Scan(&current); err != nil {
		return state, err
	}
	if current.Valid {
		p, err := presentation(ctx, tx, current.String)
		if err != nil {
			return state, err
		}
		state.Current = &p
	} else if allowNew {
		next, err := selectNext(ctx, tx, now, "")
		if err != nil {
			return state, err
		}
		if next.intro != nil {
			state.Intro = next.intro
		} else if next.quizID != "" {
			p, err := present(ctx, tx, next.quizID, now)
			if err != nil {
				return state, err
			}
			state.Current = &p
		}
	}
	if err := tx.QueryRowContext(ctx, availabilityCountsSQL, now, now).Scan(&state.Total, &state.Due, &state.NextDueAt); err != nil {
		return state, err
	}
	if state.Current != nil {
		next, err := selectNext(ctx, tx, now, state.Current.Quiz.ID)
		if err != nil {
			return state, err
		}
		if next.quizID != "" && next.intro == nil {
			q, err := quiz(ctx, tx, next.quizID)
			if err != nil {
				return state, err
			}
			state.Preview = &Presentation{Quiz: q, DueAt: q.DueAt, Concept: conceptRef(ctx, tx, q.ConceptID)}
			hideAnswer(state.Preview)
		}
		hideAnswer(state.Current)
	}
	conceptID := ""
	if state.Current != nil && state.Current.Concept != nil {
		conceptID = state.Current.Concept.ID
	} else if state.Intro != nil {
		conceptID = state.Intro.Concept.ID
	}
	if conceptID != "" {
		briefs, err := conceptBriefs(ctx, tx, []string{conceptID}, now)
		if err != nil {
			return state, err
		}
		if len(briefs) == 1 {
			state.CurrentConcept = &briefs[0]
		}
	}
	preparing, err := preparingList(ctx, tx, now)
	if err != nil {
		return state, err
	}
	state.Preparing = preparing
	return state, nil
}

// present establishes a new current occurrence of one question.
func present(ctx context.Context, tx *sql.Tx, quizID string, now int64) (Presentation, error) {
	q, err := quiz(ctx, tx, quizID)
	if err != nil {
		return Presentation{}, err
	}
	var scheduleVersion int
	if err = tx.QueryRowContext(ctx, "SELECT version FROM schedules WHERE quiz_id=?", quizID).Scan(&scheduleVersion); err != nil {
		return Presentation{}, err
	}
	p := Presentation{ID: newID(), Quiz: q, DueAt: q.DueAt, Concept: conceptRef(ctx, tx, q.ConceptID)}
	snapshot, err := marshal(q)
	if err != nil {
		return Presentation{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO presentations(id,quiz_id,content_version,schedule_version,snapshot,created_at,due_at)
	 VALUES(?,?,?,?,?,?,?)`, p.ID, q.ID, q.Version, scheduleVersion, snapshot, now, q.DueAt)
	if err != nil {
		return Presentation{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE review_session SET current_id=? WHERE singleton=1", p.ID); err != nil {
		return Presentation{}, err
	}
	return p, nil
}

func hideAnswer(p *Presentation) {
	if !p.Graded && !p.SelfCheck {
		p.Quiz.Answer = ""
		p.Quiz.Explanation = ""
		p.Quiz.Evidence = ""
		p.Quiz.Variants = nil
		p.Quiz.Rubric = nil
		p.Quiz.Citations = nil
		// Distractor tags reveal the answer: only the correct choice is untagged.
		p.Quiz.ChoiceConcepts = nil
	}
}

func conceptRef(ctx context.Context, tx *sql.Tx, id string) *ConceptRef {
	if id == "" {
		return nil
	}
	var name string
	if err := tx.QueryRowContext(ctx, "SELECT name FROM concepts WHERE id=? AND origin<>'foundation'", id).Scan(&name); err != nil {
		return nil
	}
	return &ConceptRef{ID: id, Name: name}
}

type nextChoice struct {
	quizID string
	reason string
	intro  *ConceptIntro
}

func stringSet(ctx context.Context, tx *sql.Tx, query string, args ...any) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[string]bool{}
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			return nil, err
		}
		set[value] = true
	}
	return set, rows.Err()
}

func timeByConcept(ctx context.Context, tx *sql.Tx, query string, args ...any) (map[string]int64, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]int64{}
	for rows.Next() {
		var id string
		var at int64
		if err = rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		values[id] = at
	}
	return values, rows.Err()
}

// unaidedSQL selects unaided successes (after learner overrides) per concept.
const unaidedSQL = `SELECT cq.concept_id,max(e.reviewed_at) FROM review_events e JOIN presentations p ON p.id=e.presentation_id
 JOIN concept_quizzes cq ON cq.quiz_id=p.quiz_id AND cq.role='assesses' LEFT JOIN grade_overrides o ON o.review_id=e.id
 WHERE (e.rating=3 AND e.assisted=0 AND COALESCE(o.direction,'')<>'missed') OR o.direction='correct' GROUP BY cq.concept_id`

// selectNext gathers the learner's state and asks the pure selection policy
// for the next question. A new concept with a note is introduced first.
func selectNext(ctx context.Context, tx *sql.Tx, now int64, exclude string) (nextChoice, error) {
	in := learning.SelectionInput{Now: now, Ready: map[string]bool{}, LevelUnlocked: map[string]bool{}, Deferred: map[string]bool{}, Remedial: map[string]bool{}, UnmetPrereqs: map[string]int{}}
	recent, err := stringSet(ctx, tx, "SELECT DISTINCT quiz_id FROM presentations WHERE created_at>=?", now-recentlySeenWindow)
	if err != nil {
		return nextChoice{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT q.id,q.created_at,q.rowid,sc.card,`+quizAvailableAtSQL+`,v.content,
	 COALESCE(cq.concept_id,''),COALESCE(g.id,''),COALESCE(g.focus,0),COALESCE(g.status,'active'),COALESCE(g.created_at,0),
	 EXISTS(SELECT 1 FROM concept_quizzes x WHERE x.quiz_id=q.id AND x.role='contrasts')
	 FROM quizzes q JOIN sources src ON src.id=q.source_id JOIN schedules sc ON sc.quiz_id=q.id
	 JOIN quiz_versions v ON v.quiz_id=q.id AND v.version=q.version
	 LEFT JOIN concept_quizzes cq ON cq.quiz_id=q.id AND cq.role='assesses'
	 LEFT JOIN goals g ON g.source_id=q.source_id
	 WHERE q.archived=0 AND src.archived=0 AND q.id<>?
	 AND NOT EXISTS(SELECT 1 FROM concepts c WHERE c.id=cq.concept_id AND c.status<>'active')`, exclude)
	if err != nil {
		return nextChoice{}, err
	}
	byID := map[string]learning.Candidate{}
	for rows.Next() {
		var c learning.Candidate
		var cardJSON, content, goalStatus string
		if err = rows.Scan(&c.QuizID, &c.CreatedAt, &c.Order, &cardJSON, &c.AvailableAt, &content, &c.ConceptID, &c.GoalID, &c.GoalFocus, &goalStatus, &c.GoalCreated, &c.Contrast); err != nil {
			rows.Close()
			return nextChoice{}, err
		}
		var card learning.Card
		if err = json.Unmarshal([]byte(cardJSON), &card); err != nil {
			rows.Close()
			return nextChoice{}, err
		}
		var generated GeneratedQuiz
		if err = json.Unmarshal([]byte(content), &generated); err != nil {
			rows.Close()
			return nextChoice{}, err
		}
		c.Level, c.New, c.Learning = generated.Level, learning.IsNew(card), learning.IsLearning(card)
		c.Retrievability = learning.Retrievability(card, time.UnixMilli(now))
		c.GoalPaused, c.RecentlySeen = goalStatus == "paused", recent[c.QuizID]
		in.Candidates = append(in.Candidates, c)
		byID[c.QuizID] = c
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nextChoice{}, err
	}
	rows.Close()
	if len(in.Candidates) == 0 {
		return nextChoice{}, nil
	}
	// Level ladder: only each concept's lowest remaining new level is open.
	lowest := map[string]int{}
	for _, c := range in.Candidates {
		if c.New && c.ConceptID != "" {
			rank := learning.LevelRank(c.Level)
			if current, ok := lowest[c.ConceptID]; !ok || rank < current {
				lowest[c.ConceptID] = rank
			}
		}
	}
	for _, c := range in.Candidates {
		if c.ConceptID == "" || (c.New && learning.LevelRank(c.Level) == lowest[c.ConceptID]) {
			in.LevelUnlocked[c.QuizID] = true
		}
	}
	// Prerequisite readiness, implied credit, and remediation.
	prereqs := map[string][]string{}
	relationRows, err := tx.QueryContext(ctx, `SELECT r.from_id,r.to_id FROM concept_relations r JOIN concepts c ON c.id=r.to_id
	 WHERE r.kind='requires' AND r.retired_at=0 AND c.status='active' AND c.origin<>'foundation'`)
	if err != nil {
		return nextChoice{}, err
	}
	for relationRows.Next() {
		var from, to string
		if err = relationRows.Scan(&from, &to); err != nil {
			relationRows.Close()
			return nextChoice{}, err
		}
		prereqs[from] = append(prereqs[from], to)
	}
	if err = relationRows.Err(); err != nil {
		relationRows.Close()
		return nextChoice{}, err
	}
	relationRows.Close()
	unaided, err := timeByConcept(ctx, tx, unaidedSQL)
	if err != nil {
		return nextChoice{}, err
	}
	known, err := stringSet(ctx, tx, "SELECT DISTINCT concept_id FROM evidence WHERE kind='know'")
	if err != nil {
		return nextChoice{}, err
	}
	withQuestions := map[string]bool{}
	for _, c := range in.Candidates {
		if c.ConceptID != "" {
			withQuestions[c.ConceptID] = true
		}
	}
	for concept, required := range prereqs {
		unmet := 0
		for _, p := range required {
			if withQuestions[p] && unaided[p] == 0 && !known[p] {
				unmet++
			}
		}
		in.UnmetPrereqs[concept] = unmet
	}
	for concept := range withQuestions {
		in.Ready[concept] = in.UnmetPrereqs[concept] == 0
	}
	lastReview, err := timeByConcept(ctx, tx, `SELECT cq.concept_id,max(e.reviewed_at) FROM review_events e JOIN presentations p ON p.id=e.presentation_id
	 JOIN concept_quizzes cq ON cq.quiz_id=p.quiz_id AND cq.role='assesses' WHERE e.rating>0 GROUP BY cq.concept_id`)
	if err != nil {
		return nextChoice{}, err
	}
	for concept, required := range prereqs {
		if at := unaided[concept]; at >= now-impliedWindow {
			for _, p := range required {
				if at > lastReview[p] {
					in.Deferred[p] = true
				}
			}
		}
	}
	misses, err := timeByConcept(ctx, tx, `SELECT cq.concept_id,max(e.reviewed_at) FROM review_events e JOIN presentations p ON p.id=e.presentation_id
	 JOIN concept_quizzes cq ON cq.quiz_id=p.quiz_id AND cq.role='assesses' LEFT JOIN grade_overrides o ON o.review_id=e.id
	 WHERE e.rating=1 AND e.assisted=0 AND e.reviewed_at>=? AND COALESCE(o.direction,'')<>'correct' GROUP BY cq.concept_id`, now-remedialWindow)
	if err != nil {
		return nextChoice{}, err
	}
	lastShown, err := timeByConcept(ctx, tx, `SELECT cq.concept_id,max(p.created_at) FROM presentations p
	 JOIN concept_quizzes cq ON cq.quiz_id=p.quiz_id AND cq.role='assesses' GROUP BY cq.concept_id`)
	if err != nil {
		return nextChoice{}, err
	}
	for concept, missedAt := range misses {
		for _, p := range prereqs[concept] {
			if lastShown[p] >= missedAt {
				continue
			}
			for _, c := range in.Candidates {
				if c.ConceptID == p && !c.New {
					in.Remedial[c.QuizID] = true
				}
			}
		}
	}
	// Introduction and pacing.
	introduced, err := timeByConcept(ctx, tx, `SELECT concept_id,min(at) FROM (
	 SELECT concept_id,created_at AS at FROM evidence WHERE kind IN ('read','know') AND concept_id<>''
	 UNION ALL SELECT cq.concept_id,p.created_at FROM presentations p JOIN concept_quizzes cq ON cq.quiz_id=p.quiz_id AND cq.role='assesses')
	 GROUP BY concept_id`)
	if err != nil {
		return nextChoice{}, err
	}
	in.Introduced = map[string]bool{}
	started := 0
	for concept, at := range introduced {
		in.Introduced[concept] = true
		if at >= now-remedialWindow {
			started++
		}
	}
	var pace string
	if err = tx.QueryRowContext(ctx, "SELECT pace FROM preferences WHERE singleton=1").Scan(&pace); err != nil {
		return nextChoice{}, err
	}
	in.NewBudgetLeft = learning.NewConceptBudget(pace) - started
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT cq.concept_id FROM presentations p JOIN concept_quizzes cq ON cq.quiz_id=p.quiz_id AND cq.role='assesses'
	 ORDER BY p.created_at DESC,p.rowid DESC LIMIT 1),'')`).Scan(&in.LastConcept); err != nil {
		return nextChoice{}, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT concept_id FROM evidence WHERE kind IN ('read','know') AND json_extract(detail,'$.context')='intro'
	 AND created_at>(SELECT COALESCE(max(created_at),0) FROM presentations) ORDER BY created_at DESC,rowid DESC LIMIT 1),'')`).Scan(&in.LastIntro); err != nil {
		return nextChoice{}, err
	}
	var focusUntil int64
	if err = tx.QueryRowContext(ctx, "SELECT focus_concept,focus_until FROM review_session WHERE singleton=1").Scan(&in.Focus, &focusUntil); err != nil {
		return nextChoice{}, err
	}
	if focusUntil <= now {
		in.Focus = ""
	}
	chosen := learning.Select(in)
	if chosen.QuizID == "" {
		return nextChoice{}, nil
	}
	c := byID[chosen.QuizID]
	if c.New && c.ConceptID != "" && !in.Introduced[c.ConceptID] {
		note, err := currentNote(ctx, tx, c.ConceptID)
		if err != nil {
			return nextChoice{}, err
		}
		if note != nil {
			var name, summary string
			if err = tx.QueryRowContext(ctx, "SELECT name,description FROM concepts WHERE id=?", c.ConceptID).Scan(&name, &summary); err != nil {
				return nextChoice{}, err
			}
			return nextChoice{intro: &ConceptIntro{Concept: ConceptRef{ID: c.ConceptID, Name: name}, Summary: summary, Note: note}}, nil
		}
	}
	return nextChoice{quizID: chosen.QuizID, reason: chosen.Reason}, nil
}

func authorityOf(grading, outcome string, graded bool) string {
	if !graded {
		return ""
	}
	if outcome == "revealed" {
		return "reveal"
	}
	if strings.HasPrefix(outcome, "warm_") {
		return ""
	}
	switch grading {
	case "exact-v1":
		return "exact"
	case learning.SemanticPolicyVersion, learning.ShortPolicyVersion:
		return "jev"
	case learning.LearnerPolicyVersion:
		return "learner"
	}
	return ""
}

func presentation(ctx context.Context, tx *sql.Tx, id string) (Presentation, error) {
	var p Presentation
	var snapshot, quizID, grading string
	err := tx.QueryRowContext(ctx, `SELECT p.id,p.snapshot,p.answer,p.outcome,p.assisted,p.graded,p.rating,p.due_at,p.reviewed_at,p.review_id,
	 EXISTS(SELECT 1 FROM corrections c WHERE c.review_id=p.review_id),
	 COALESCE((SELECT direction FROM grade_overrides o WHERE o.review_id=p.review_id AND p.review_id<>''),''),
	 COALESCE((SELECT grading FROM review_events e WHERE e.id=p.review_id AND p.review_id<>''),''),p.quiz_id
	 FROM presentations p WHERE p.id=?`, id).
		Scan(&p.ID, &snapshot, &p.Answer, &p.Outcome, &p.Assisted, &p.Graded, &p.Rating, &p.DueAt, &p.ReviewedAt, &p.ReviewID, &p.Disputed, &p.Override, &grading, &quizID)
	if err != nil {
		return p, notFound(err, "presentation")
	}
	if err = json.Unmarshal([]byte(snapshot), &p.Quiz); err != nil {
		return p, fmt.Errorf("decode presented quiz: %w", err)
	}
	conceptID := p.Quiz.ConceptID
	if conceptID == "" {
		if err = tx.QueryRowContext(ctx, "SELECT COALESCE((SELECT concept_id FROM concept_quizzes WHERE quiz_id=? AND role='assesses'),'')", quizID).Scan(&conceptID); err != nil {
			return p, err
		}
	}
	p.Concept = conceptRef(ctx, tx, conceptID)
	p.Authority = authorityOf(grading, p.Outcome, p.Graded)
	var assessmentID, assessmentOperationID, assessmentStatus, assessmentDecision, assessmentDetail string
	var assessmentApplied bool
	err = tx.QueryRowContext(ctx, `SELECT id,operation_id,status,decision,applied,detail FROM semantic_assessments WHERE presentation_id=?
	 ORDER BY created_at DESC,rowid DESC LIMIT 1`, p.ID).Scan(&assessmentID, &assessmentOperationID, &assessmentStatus, &assessmentDecision, &assessmentApplied, &assessmentDetail)
	if err == nil {
		p.AssessmentID, p.AssessmentOperationID, p.AssessmentStatus = assessmentID, assessmentOperationID, assessmentStatus
		if assessmentApplied {
			p.AssessmentDecision, p.AssessmentDetail = assessmentDecision, assessmentDetail
		}
		p.Pending = assessmentStatus == "pending"
	} else if !errors.Is(err, sql.ErrNoRows) {
		return p, err
	}
	// Self-check: the saved answer could not be graded automatically, so the
	// key is shown and the learner grades. Derived from durable state, which
	// also covers legacy close/ungraded occurrences.
	if !p.Graded && !p.Pending && p.Answer != "" {
		switch {
		case p.AssessmentStatus == "failed":
			p.SelfCheck, p.SelfCheckReason = true, "failed"
		case (p.AssessmentStatus == "judged" && p.AssessmentDecision == "") || p.AssessmentStatus == "superseded":
			p.SelfCheck, p.SelfCheckReason = true, "unsure"
		case p.Outcome == "selfcheck" || p.Outcome == "close" || p.Outcome == "ungraded":
			p.SelfCheck, p.SelfCheckReason = true, "close"
		}
	}
	if !p.Graded && !p.SelfCheck && p.Answer != "" {
		p.Draft = p.Answer
	}
	return p, nil
}

// submitFence loads and checks the current occurrence for a new grade.
func submitFence(ctx context.Context, tx *sql.Tx, presentationID string) (Presentation, string, int, int, error) {
	p, err := presentation(ctx, tx, presentationID)
	if err != nil {
		return p, "", 0, 0, err
	}
	var current sql.NullString
	if err = tx.QueryRowContext(ctx, "SELECT current_id FROM review_session WHERE singleton=1").Scan(&current); err != nil {
		return p, "", 0, 0, err
	}
	if !current.Valid || current.String != presentationID {
		return p, "", 0, 0, fmt.Errorf("%w: this occurrence is no longer current; reload review", ErrConflict)
	}
	if p.Graded {
		// An assistance fence always wins over a second tab's late success.
		// Fresh operation IDs are not authority to grade an occurrence twice.
		return p, "", 0, 0, fmt.Errorf("%w: this occurrence already has saved feedback; reload review", ErrConflict)
	}
	var pending bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM semantic_assessments WHERE presentation_id=? AND status='pending')", p.ID).Scan(&pending); err != nil {
		return p, "", 0, 0, err
	}
	if pending {
		return p, "", 0, 0, fmt.Errorf("%w: this answer is still being checked", ErrConflict)
	}
	var contentVersion, scheduleVersion, presentedSchedule int
	var archived bool
	var cardJSON, algorithm string
	err = tx.QueryRowContext(ctx, `SELECT q.version,(q.archived OR src.archived),sc.version,sc.card,sc.algorithm,p.schedule_version
	 FROM presentations p JOIN quizzes q ON q.id=p.quiz_id JOIN sources src ON src.id=q.source_id
	 JOIN schedules sc ON sc.quiz_id=q.id WHERE p.id=?`, presentationID).
		Scan(&contentVersion, &archived, &scheduleVersion, &cardJSON, &algorithm, &presentedSchedule)
	if err != nil {
		return p, "", 0, 0, err
	}
	if archived || contentVersion != p.Quiz.Version || scheduleVersion != presentedSchedule {
		return p, "", 0, 0, fmt.Errorf("%w: question or schedule changed; reload review", ErrConflict)
	}
	if algorithm != learning.Algorithm {
		return p, "", 0, 0, fmt.Errorf("%w: unsupported schedule algorithm %q", ErrConflict, algorithm)
	}
	return p, cardJSON, scheduleVersion, presentedSchedule, nil
}

// applyGrade writes one graded (or held) attempt: schedule transition, the
// immutable event naming its grading policy, and the occurrence state.
func applyGrade(ctx context.Context, tx *sql.Tx, p *Presentation, cardJSON string, scheduleVersion int, grading string, now int64) error {
	p.ReviewedAt, p.ReviewID = now, newID()
	afterJSON := cardJSON
	afterVersion := scheduleVersion
	if p.Rating != 0 {
		var card learning.Card
		if err := json.Unmarshal([]byte(cardJSON), &card); err != nil {
			return err
		}
		next, err := learning.Schedule(card, p.Rating, time.UnixMilli(now))
		if err != nil {
			return err
		}
		afterJSON, err = marshal(next)
		if err != nil {
			return err
		}
		p.DueAt = next.Due.UnixMilli()
		afterVersion++
		result, err := tx.ExecContext(ctx, "UPDATE schedules SET version=?,card=?,due_at=? WHERE quiz_id=? AND version=?", afterVersion, afterJSON, p.DueAt, p.Quiz.ID, scheduleVersion)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("%w: schedule changed", ErrConflict)
		}
	}
	snapshot, err := marshal(p.Quiz)
	if err != nil {
		return err
	}
	algorithm := learning.Algorithm
	if grading != "exact-v1" {
		algorithm = learning.EventAlgorithm(grading)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO review_events(id,presentation_id,snapshot,answer,outcome,rating,assisted,reviewed_at,due_at,algorithm,
	 schedule_before,schedule_after,schedule_version_before,schedule_version_after,grading) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ReviewID, p.ID, snapshot, p.Answer, p.Outcome, p.Rating, p.Assisted, now, p.DueAt, algorithm, cardJSON, afterJSON, scheduleVersion, afterVersion, grading)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE presentations SET answer=?,outcome=?,assisted=?,graded=?,rating=?,due_at=?,reviewed_at=?,review_id=? WHERE id=?`,
		p.Answer, p.Outcome, p.Assisted, p.Graded, p.Rating, p.DueAt, now, p.ReviewID, p.ID)
	if err != nil {
		return err
	}
	p.Authority = authorityOf(grading, p.Outcome, p.Graded)
	return nil
}

func (s *Store) Submit(ctx context.Context, presentationID, operationID, answer string, reveal bool) (Presentation, error) {
	if err := validOperation(operationID); err != nil {
		return Presentation{}, err
	}
	if err := validText("presentation ID", presentationID, 200, true); err != nil {
		return Presentation{}, err
	}
	if err := validText("answer", answer, 1024, !reveal); err != nil {
		return Presentation{}, err
	}
	hash := payloadHash(struct {
		Presentation, Answer string
		Reveal               bool
	}{presentationID, answer, reveal})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Presentation{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, "submit", hash)
	if err != nil {
		return Presentation{}, err
	}
	if found {
		var saved Presentation
		if err = json.Unmarshal([]byte(receipt), &saved); err != nil {
			return saved, err
		}
		if saved.AssessmentID != "" {
			a, assessmentErr := readAssessment(ctx, tx, saved.AssessmentID)
			if assessmentErr != nil {
				return Presentation{}, assessmentErr
			}
			// A replay reconciles; it never resends. A transmitted assessment
			// whose lease lapsed without a result is a definite failure that
			// keeps its unknown spend accounted.
			if err = reconcileInterrupted(ctx, tx, &a, s.now()); err != nil {
				return Presentation{}, err
			}
			p, presentationErr := presentation(ctx, tx, a.PresentationID)
			if presentationErr != nil {
				return Presentation{}, presentationErr
			}
			// Nonterminal receipts retain the response shape of their own
			// staged operation even when a later operation exists. A graded
			// or self-check outcome uses the occurrence's durable state.
			if (a.Status == "pending" || a.Status == "superseded") && !p.Graded {
				p = saved
				p.Answer = a.Answer
			}
			bindAssessment(&p, a)
			hideAnswer(&p)
			return p, tx.Commit()
		}
		return saved, tx.Commit()
	}
	p, cardJSON, scheduleVersion, presentedSchedule, err := submitFence(ctx, tx, presentationID)
	if err != nil {
		return Presentation{}, err
	}
	if p.SelfCheck {
		// After the key is visible the learner grades; only a failed automatic
		// check may be retried, and only for the same saved answer.
		if reveal || p.SelfCheckReason != "failed" || answer != p.Answer {
			return Presentation{}, fmt.Errorf("%w: compare your answer with the key and choose whether you got it", ErrConflict)
		}
	}
	if p.Quiz.Kind == "choice" && !reveal {
		valid := false
		for _, choice := range p.Quiz.Choices {
			if answer == choice {
				valid = true
				break
			}
		}
		if !valid {
			return Presentation{}, fmt.Errorf("%w: select one of the exact presented choices", ErrInvalid)
		}
	}
	outcome, rating := learning.Grade(p.Quiz.Kind, p.Quiz.Grading, p.Quiz.AnswerForm, p.Quiz.Answer, p.Quiz.Variants, answer, reveal)
	now := s.now()
	if outcome == "ungraded" && !reveal && (p.Quiz.Grading == "semantic" || (p.Quiz.Kind == "recall" && p.Quiz.AnswerForm == "flexible")) {
		policy := learning.ShortPolicyVersion
		if p.Quiz.Grading == "semantic" {
			policy = learning.SemanticPolicyVersion
		}
		assessmentID := newID()
		p.Answer, p.Draft, p.Outcome, p.Rating = answer, "", "", 0
		p.SelfCheck, p.SelfCheckReason = false, ""
		p.Pending, p.AssessmentID, p.AssessmentOperationID, p.AssessmentStatus = true, assessmentID, operationID, "pending"
		p.AssessmentDecision, p.AssessmentDetail = "", ""
		_, err = tx.ExecContext(ctx, `INSERT INTO semantic_assessments(id,presentation_id,operation_id,content_version,schedule_version,answer,
		 status,policy_version,request_model,request_json,created_at) VALUES(?,?,?,?,?,?,'pending',?,'','{}',?)`,
			assessmentID, p.ID, operationID, p.Quiz.Version, presentedSchedule, answer, policy, now)
		if err != nil {
			if strings.Contains(err.Error(), "one_pending_assessment") || strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return Presentation{}, fmt.Errorf("%w: this answer is still being checked", ErrConflict)
			}
			return Presentation{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE presentations SET answer=?,outcome='',assisted=?,graded=0,rating=0 WHERE id=?`,
			answer, p.Assisted, p.ID); err != nil {
			return Presentation{}, err
		}
		hideAnswer(&p)
		if err = saveOperation(ctx, tx, operationID, "submit", hash, p.ID, p, now); err != nil {
			return Presentation{}, err
		}
		if err = tx.Commit(); err != nil {
			return Presentation{}, err
		}
		return p, nil
	}
	preAssisted := p.Assisted
	p.Answer, p.Outcome, p.Rating = answer, outcome, rating
	p.Assisted, p.Graded = p.Assisted || reveal, rating != 0
	p.SelfCheck = outcome == "selfcheck"
	p.SelfCheckReason = ""
	if p.SelfCheck {
		p.SelfCheckReason = "close"
	}
	// Exposure makes this occurrence warm, but does not turn help into Again
	// or manufacture an FSRS success. Explicit Reveal retains its old contract.
	var exposed bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM assistance_exposures x, presentations current WHERE current.id=? AND x.quiz_id=current.quiz_id
	 AND x.content_version=current.content_version AND x.created_at>=current.created_at-86400000)`, p.ID).Scan(&exposed); err != nil {
		return Presentation{}, err
	}
	if exposed && !reveal && !p.SelfCheck {
		p.Assisted, p.Graded, p.Rating = true, true, 0
		p.Outcome = "warm_" + outcome
	} else if preAssisted && !reveal && outcome == "correct" {
		p.Assisted, p.Graded, p.Rating = true, true, 0
		p.Outcome = "warm_correct"
	}
	p.Draft = ""
	if err = applyGrade(ctx, tx, &p, cardJSON, scheduleVersion, "exact-v1", now); err != nil {
		return Presentation{}, err
	}
	if p.Quiz.Kind == "choice" && outcome == "wrong" && !reveal {
		if err = recordConfusion(ctx, tx, p, answer, now); err != nil {
			return Presentation{}, err
		}
	}
	hideAnswer(&p)
	if err = saveOperation(ctx, tx, operationID, "submit", hash, p.ID, p, now); err != nil {
		return Presentation{}, err
	}
	if err = tx.Commit(); err != nil {
		return Presentation{}, err
	}
	return p, nil
}

// recordConfusion notes that a wrong choice stood for another concept, and
// after two such confusions between the same pair schedules one bounded
// contrast job for them.
func recordConfusion(ctx context.Context, tx *sql.Tx, p Presentation, answer string, now int64) error {
	if p.Concept == nil || len(p.Quiz.ChoiceConcepts) != len(p.Quiz.Choices) {
		return nil
	}
	other := ""
	for i, choice := range p.Quiz.Choices {
		if choice == answer {
			other = p.Quiz.ChoiceConcepts[i]
		}
	}
	primary := p.Concept.ID
	if other == "" || other == primary {
		return nil
	}
	detail, err := marshal(map[string]string{"with": other})
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO evidence(id,kind,concept_id,quiz_id,presentation_id,review_id,detail,created_at) VALUES(?,?,?,?,?,?,?,?)`,
		newID(), "confusion", primary, p.Quiz.ID, p.ID, p.ReviewID, detail, now); err != nil {
		return err
	}
	var confusions int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM evidence WHERE kind='confusion' AND
	 ((concept_id=? AND json_extract(detail,'$.with')=?) OR (concept_id=? AND json_extract(detail,'$.with')=?))`, primary, other, other, primary).Scan(&confusions); err != nil {
		return err
	}
	if confusions < 2 {
		return nil
	}
	pair, reverse := contrastPayload(primary, other), contrastPayload(other, primary)
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE kind='contrast' AND payload IN (?,?))
	 OR EXISTS(SELECT 1 FROM concept_quizzes x JOIN concept_quizzes y ON y.quiz_id=x.quiz_id JOIN quizzes q ON q.id=x.quiz_id
	 WHERE q.archived=0 AND x.role='assesses' AND y.role='contrasts' AND ((x.concept_id=? AND y.concept_id=?) OR (x.concept_id=? AND y.concept_id=?)))`,
		pair, reverse, primary, other, other, primary).Scan(&exists); err != nil || exists {
		return err
	}
	var sourceID string
	var revision int
	err = tx.QueryRowContext(ctx, `SELECT src.id,src.revision FROM concepts c JOIN sources src ON src.id=c.source_id WHERE c.id=? AND src.archived=0`, primary).Scan(&sourceID, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	err = enqueue(ctx, tx, sourceID, revision, "contrast", json.RawMessage(pair), now)
	if errors.Is(err, ErrConflict) {
		return nil // the source is busy; a later confusion will try again
	}
	return err
}

func contrastPayload(a, b string) string {
	encoded, _ := json.Marshal(map[string][]string{"concepts": {a, b}})
	return string(encoded)
}

// SelfGrade records the learner's own judgment after comparing their saved
// answer with the key. It is learner authority, named as such in history.
func (s *Store) SelfGrade(ctx context.Context, presentationID, operationID string, correct bool) (Presentation, error) {
	if err := validOperation(operationID); err != nil {
		return Presentation{}, err
	}
	hash := payloadHash(struct {
		Presentation string
		Correct      bool
	}{presentationID, correct})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Presentation{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, "self-grade", hash)
	if err != nil {
		return Presentation{}, err
	}
	if found {
		var saved Presentation
		if err = json.Unmarshal([]byte(receipt), &saved); err != nil {
			return saved, err
		}
		return saved, tx.Commit()
	}
	p, cardJSON, scheduleVersion, _, err := submitFence(ctx, tx, presentationID)
	if err != nil {
		return Presentation{}, err
	}
	if !p.SelfCheck {
		return Presentation{}, fmt.Errorf("%w: this answer is not waiting for your check", ErrConflict)
	}
	now := s.now()
	p.Graded, p.SelfCheck, p.SelfCheckReason, p.Draft = true, false, "", ""
	if correct {
		p.Outcome, p.Rating = "self_correct", 3
		if p.Assisted {
			p.Outcome, p.Rating = "warm_correct", 0
		}
	} else {
		p.Outcome, p.Rating = "self_missed", 1
	}
	if err = applyGrade(ctx, tx, &p, cardJSON, scheduleVersion, learning.LearnerPolicyVersion, now); err != nil {
		return Presentation{}, err
	}
	if err = saveOperation(ctx, tx, operationID, "self-grade", hash, p.ID, p, now); err != nil {
		return Presentation{}, err
	}
	return p, tx.Commit()
}

// OverrideGrade lets the learner contest an automatic grade with one tap.
// The original event stays immutable; the override records its own schedule
// transition, recomputed from the pre-review card as if graded the other way.
// It applies only while that review is the question's latest transition.
func (s *Store) OverrideGrade(ctx context.Context, presentationID, operationID string, correct bool) (Presentation, error) {
	if err := validOperation(operationID); err != nil {
		return Presentation{}, err
	}
	hash := payloadHash(struct {
		Presentation string
		Correct      bool
	}{presentationID, correct})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Presentation{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, "override", hash)
	if err != nil {
		return Presentation{}, err
	}
	if found {
		var saved Presentation
		if err = json.Unmarshal([]byte(receipt), &saved); err != nil {
			return saved, err
		}
		return saved, tx.Commit()
	}
	p, err := presentation(ctx, tx, presentationID)
	if err != nil {
		return Presentation{}, err
	}
	if !p.Graded || p.ReviewID == "" || p.Override != "" {
		return Presentation{}, fmt.Errorf("%w: this result cannot be corrected here", ErrConflict)
	}
	var rating, versionBefore, versionAfter int
	var assisted bool
	var outcome, grading, before string
	var reviewedAt int64
	if err = tx.QueryRowContext(ctx, `SELECT rating,assisted,outcome,grading,schedule_before,schedule_version_before,schedule_version_after,reviewed_at
	 FROM review_events WHERE id=?`, p.ReviewID).Scan(&rating, &assisted, &outcome, &grading, &before, &versionBefore, &versionAfter, &reviewedAt); err != nil {
		return Presentation{}, notFound(err, "review")
	}
	automatic := grading == "exact-v1" || grading == learning.ShortPolicyVersion || grading == learning.SemanticPolicyVersion
	eligible := automatic && !assisted && ((correct && rating == 1 && outcome == "wrong") || (!correct && rating == 3 && outcome == "correct"))
	if !eligible {
		return Presentation{}, fmt.Errorf("%w: only an automatic grade of an unaided answer can be corrected", ErrConflict)
	}
	var currentVersion int
	var currentCard string
	if err = tx.QueryRowContext(ctx, "SELECT version,card FROM schedules WHERE quiz_id=?", p.Quiz.ID).Scan(&currentVersion, &currentCard); err != nil {
		return Presentation{}, err
	}
	if currentVersion != versionAfter {
		return Presentation{}, fmt.Errorf("%w: a newer review exists; flag this attempt from History instead", ErrConflict)
	}
	var card learning.Card
	if err = json.Unmarshal([]byte(before), &card); err != nil {
		return Presentation{}, err
	}
	newRating, direction := 1, "missed"
	if correct {
		newRating, direction = 3, "correct"
	}
	next, err := learning.Schedule(card, newRating, time.UnixMilli(reviewedAt))
	if err != nil {
		return Presentation{}, err
	}
	after, err := marshal(next)
	if err != nil {
		return Presentation{}, err
	}
	now := s.now()
	due := next.Due.UnixMilli()
	if _, err = tx.ExecContext(ctx, "UPDATE schedules SET version=version+1,card=?,due_at=? WHERE quiz_id=? AND version=?", after, due, p.Quiz.ID, currentVersion); err != nil {
		return Presentation{}, err
	}
	// A newer unanswered occurrence of this question was presented at the old
	// schedule version and could never be graded; retire it with the change.
	if err = retireUnanswered(ctx, tx, p.Quiz.ID, ""); err != nil {
		return Presentation{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO grade_overrides(id,review_id,presentation_id,direction,schedule_before,schedule_after,schedule_version_before,schedule_version_after,created_at)
	 VALUES(?,?,?,?,?,?,?,?,?)`, newID(), p.ReviewID, p.ID, direction, currentCard, after, currentVersion, currentVersion+1, now); err != nil {
		return Presentation{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE presentations SET due_at=? WHERE id=?", due, p.ID); err != nil {
		return Presentation{}, err
	}
	p.Override, p.DueAt = direction, due
	if err = saveOperation(ctx, tx, operationID, "override", hash, p.ID, p, now); err != nil {
		return Presentation{}, err
	}
	return p, tx.Commit()
}

// AcknowledgeIntro records that the learner read a new concept's note (or
// already knows it) and returns the next stream state. Reading is exposure,
// never a graded attempt.
func (s *Store) AcknowledgeIntro(ctx context.Context, conceptID, operationID string, known bool) (ReviewState, error) {
	if err := validOperation(operationID); err != nil {
		return ReviewState{}, err
	}
	hash := payloadHash(struct {
		Concept string
		Known   bool
	}{conceptID, known})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReviewState{}, err
	}
	defer tx.Rollback()
	_, _, found, err := existingOperation(ctx, tx, operationID, "intro", hash)
	if err != nil {
		return ReviewState{}, err
	}
	now := s.now()
	if !found {
		var active bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM concepts WHERE id=? AND status='active' AND origin<>'foundation')", conceptID).Scan(&active); err != nil {
			return ReviewState{}, err
		}
		if !active {
			return ReviewState{}, fmt.Errorf("%w: concept", ErrNotFound)
		}
		// Only the intro the stream offers now may be acknowledged: "I know
		// this" counts toward prerequisites, so an out-of-turn or stale
		// acknowledgment would bypass their ordering.
		var current sql.NullString
		if err = tx.QueryRowContext(ctx, "SELECT current_id FROM review_session WHERE singleton=1").Scan(&current); err != nil {
			return ReviewState{}, err
		}
		offered := nextChoice{}
		if !current.Valid {
			if offered, err = selectNext(ctx, tx, now, ""); err != nil {
				return ReviewState{}, err
			}
		}
		if offered.intro == nil || offered.intro.Concept.ID != conceptID {
			return ReviewState{}, fmt.Errorf("%w: this idea is no longer the one to read; reload", ErrConflict)
		}
		note, err := currentNote(ctx, tx, conceptID)
		if err != nil {
			return ReviewState{}, err
		}
		noteID := ""
		if note != nil {
			noteID = note.ID
		}
		kind := "read"
		if known {
			kind = "know"
		}
		detail, err := marshal(map[string]string{"context": "intro", "note_id": noteID})
		if err != nil {
			return ReviewState{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO evidence(id,kind,concept_id,detail,created_at) VALUES(?,?,?,?,?)`, newID(), kind, conceptID, detail, now); err != nil {
			return ReviewState{}, err
		}
		if err = saveOperation(ctx, tx, operationID, "intro", hash, conceptID, nil, now); err != nil {
			return ReviewState{}, err
		}
	}
	state, err := reviewState(ctx, tx, now, true)
	if err != nil {
		return ReviewState{}, err
	}
	return state, tx.Commit()
}

// Lifecycle changes retire only unanswered occurrences. A held graded snapshot
// remains visible even if its future content has been edited or archived.
func retireUnanswered(ctx context.Context, tx *sql.Tx, quizID, sourceID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE review_session SET current_id=NULL WHERE singleton=1 AND current_id IN
	 (SELECT p.id FROM presentations p JOIN quizzes q ON q.id=p.quiz_id WHERE p.graded=0 AND (q.id=? OR q.source_id=?))`, quizID, sourceID)
	return err
}
