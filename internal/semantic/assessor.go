package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/misty-step/scry/internal/learning"
	"github.com/misty-step/scry/internal/store"
)

// settleTimeout bounds the terminal write after a request has been sent. It
// is detached from the learner's request so a dropped connection cannot turn
// a transmitted judgment into an unknown outcome.
const settleTimeout = 10 * time.Second

// AssessmentService is the web boundary: a staged durable assessment is
// resumed outside SQL and then finalized through a separately fenced write.
type AssessmentService interface {
	Assess(context.Context, string) (store.Presentation, error)
}

// Spending is the per-assessment reservation policy in USD micros. The
// reservation is taken from the shared rolling allowance before a request
// leaves the process and is replaced by the measured cost afterwards.
type Spending = store.SemanticSpending

type Assessor struct {
	store    *store.Store
	client   Client
	model    string
	spending Spending
}

// NewAssessor wires the durable store, the bounded client, the pinned model,
// and the reservation policy. A zero Spending reserves nothing, which is only
// correct for the unconfigured no-network client.
func NewAssessor(repository *store.Store, client Client, model string, spending Spending) *Assessor {
	if model == "" {
		model = DefaultModel
	}
	return &Assessor{store: repository, client: client, model: model, spending: spending}
}

// Assess resumes one staged assessment. Exactly one caller ever obtains the
// send lease for an assessment; every other caller (a duplicate submit, an
// exact replay, a retry after a crash) receives the durable state without a
// second model request. A transmitted assessment with no recorded result is
// reconciled to failed with its reservation retained as unknown spend.
func (a *Assessor) Assess(ctx context.Context, id string) (store.Presentation, error) {
	if a == nil || a.store == nil || a.client == nil {
		return store.Presentation{}, errors.New("semantic assessor is not configured")
	}
	assessment, err := a.store.Assessment(ctx, id)
	if err != nil {
		return store.Presentation{}, err
	}
	if assessment.Status != "pending" || assessment.Transmissions > 0 {
		return a.store.FinalizeAssessment(ctx, id, "", store.AssessmentResult{})
	}
	var request Request
	switch assessment.PolicyVersion {
	case learning.ShortPolicyVersion:
		request = BuildShortAnswerRequest(a.model, ShortState{
			Prompt: assessment.Quiz.Prompt, ExpectedAnswer: assessment.Quiz.Answer,
			LearnerAnswer: assessment.Answer, Variants: assessment.Quiz.Variants,
		})
	case learning.SemanticPolicyVersion:
		if assessment.Quiz.Rubric == nil {
			return a.store.FinalizeAssessment(ctx, id, "", store.AssessmentResult{})
		}
		request = BuildRecallRequest(a.model, RecallState{
			Prompt: assessment.Quiz.Prompt, ExpectedAnswer: assessment.Quiz.Answer,
			LearnerAnswer: assessment.Answer, Variants: assessment.Quiz.Variants, Rubric: *assessment.Quiz.Rubric,
		})
	default:
		return a.store.FinalizeAssessment(ctx, id, "", store.AssessmentResult{})
	}
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return store.Presentation{}, err
	}
	lease, err := a.store.BeginAssessmentTransmission(ctx, id, request.Model, requestJSON, a.spending)
	if errors.Is(err, store.ErrBudget) {
		// A definite no-send failure is already durable; show it.
		return a.store.FinalizeAssessment(ctx, id, "", store.AssessmentResult{})
	}
	if err != nil {
		return store.Presentation{}, err
	}
	if !lease.Send {
		return a.store.FinalizeAssessment(ctx, id, "", store.AssessmentResult{})
	}
	response, decisionErr := a.client.Decide(ctx, request)
	// The request has left the process and may be billed whatever the caller
	// does next. Terminal persistence must not depend on the learner's
	// connection: settle under a bounded context detached from cancellation
	// so a dropped request still records the judgment or the failure.
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleTimeout)
	defer cancel()
	if decisionErr != nil {
		return a.fail(settleCtx, id, lease.Token, response, decisionErr)
	}
	if assessment.PolicyVersion == learning.ShortPolicyVersion {
		short, parseErr := shortJudgments(response)
		if parseErr != nil {
			return a.fail(settleCtx, id, lease.Token, response, ErrMalformed)
		}
		result := assessmentResult(response, learning.SemanticJudgments{}, "")
		result.Short = short
		return a.store.FinalizeAssessment(settleCtx, id, lease.Token, result)
	}
	judgments, err := judgments(response, len(assessment.Quiz.Rubric.Required), len(assessment.Quiz.Rubric.Contradictions))
	if err != nil {
		return a.fail(settleCtx, id, lease.Token, response, ErrMalformed)
	}
	return a.store.FinalizeAssessment(settleCtx, id, lease.Token, assessmentResult(response, judgments, ""))
}

func (a *Assessor) fail(ctx context.Context, id, token string, response Response, cause error) (store.Presentation, error) {
	classification := "malformed"
	switch {
	case errors.Is(cause, ErrNotConfigured):
		classification = "unconfigured"
	case errors.Is(cause, ErrUnavailable):
		classification = "unavailable"
	case errors.Is(cause, ErrRejected):
		classification = "rejected"
	}
	result := assessmentResult(response, learning.SemanticJudgments{}, classification)
	// Only a client that provably never sent may release its reservation; a
	// timeout or transport error may already have been accepted and billed.
	result.NoSend = errors.Is(cause, ErrNotConfigured)
	return a.store.FailAssessment(ctx, id, token, result)
}

func assessmentResult(response Response, judgments learning.SemanticJudgments, failure string) store.AssessmentResult {
	return store.AssessmentResult{
		ResponseModel: response.Model,
		ResponseJSON:  append([]byte(nil), response.Raw...),
		Judgments:     judgments,
		InputTokens:   response.Usage.InputTokens,
		OutputTokens:  response.Usage.OutputTokens,
		CostMicros:    response.Usage.CostMicros,
		LatencyMS:     response.LatencyMS,
		Error:         failure,
	}
}

func judgments(response Response, ideas, contradictions int) (learning.SemanticJudgments, error) {
	result := learning.SemanticJudgments{
		Ideas: make([]float64, ideas), Contradictions: make([]float64, contradictions),
		RelationProbabilities: map[string]float64{},
	}
	if len(response.Answers) != ideas+contradictions+2 {
		return result, ErrMalformed
	}
	for index := 0; index < ideas; index++ {
		answer, ok := response.Answers[fmt.Sprintf("idea_%d", index)]
		if !ok || answer.Noul == nil {
			return result, ErrMalformed
		}
		result.Ideas[index] = *answer.Noul
	}
	for index := 0; index < contradictions; index++ {
		answer, ok := response.Answers[fmt.Sprintf("contradiction_%d", index)]
		if !ok || answer.Noul == nil {
			return result, ErrMalformed
		}
		result.Contradictions[index] = *answer.Noul
	}
	relation, ok := response.Answers["relation"]
	if !ok || (relation.Choice != "equivalent" && relation.Choice != "partial" && relation.Choice != "different" && relation.Choice != "unclear") {
		return result, ErrMalformed
	}
	for label, probability := range relation.Probabilities {
		result.RelationProbabilities[label] = probability
	}
	if _, ok = result.RelationProbabilities[relation.Choice]; !ok {
		return result, ErrMalformed
	}
	injection, ok := response.Answers["injection"]
	if !ok || injection.Noul == nil {
		return result, ErrMalformed
	}
	result.Relation, result.Injection = relation.Choice, *injection.Noul
	return result, nil
}

func shortJudgments(response Response) (learning.ShortJudgments, error) {
	result := learning.ShortJudgments{}
	if len(response.Answers) != 3 {
		return result, ErrMalformed
	}
	verdict, vok := response.Answers["verdict"]
	identity, iok := response.Answers["identity"]
	injection, jok := response.Answers["injection"]
	if !vok || !iok || !jok || (verdict.Type != "" && verdict.Type != "choice") ||
		(verdict.Choice != "accept" && verdict.Choice != "reject" && verdict.Choice != "unsure") ||
		verdict.Noul != nil || verdict.Score != nil || len(verdict.Probabilities) != 3 ||
		(identity.Type != "" && identity.Type != "noul") || identity.Noul == nil || identity.Choice != "" || identity.Score != nil || len(identity.Probabilities) != 0 ||
		(injection.Type != "" && injection.Type != "noul") || injection.Noul == nil || injection.Choice != "" || injection.Score != nil || len(injection.Probabilities) != 0 {
		return result, ErrMalformed
	}
	for _, label := range []string{"accept", "reject", "unsure"} {
		probability, ok := verdict.Probabilities[label]
		if !ok || probability < 0 || probability > 1 {
			return result, ErrMalformed
		}
	}
	if *identity.Noul < 0 || *identity.Noul > 1 || *injection.Noul < 0 || *injection.Noul > 1 {
		return result, ErrMalformed
	}
	return learning.ShortJudgments{Verdict: verdict.Choice, Probabilities: verdict.Probabilities, Identity: *identity.Noul, Injection: *injection.Noul}, nil
}
