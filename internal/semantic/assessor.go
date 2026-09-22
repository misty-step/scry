package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/misty-step/scry/internal/learning"
	"github.com/misty-step/scry/internal/store"
)

// AssessmentService is the web boundary: a staged durable assessment is
// resumed outside SQL and then finalized through a separately fenced write.
type AssessmentService interface {
	Assess(context.Context, string) (store.Presentation, error)
}

type Assessor struct {
	store  *store.Store
	client Client
	model  string
}

func NewAssessor(repository *store.Store, client Client, model string) *Assessor {
	if model == "" {
		model = DefaultModel
	}
	return &Assessor{store: repository, client: client, model: model}
}

func (a *Assessor) Assess(ctx context.Context, id string) (store.Presentation, error) {
	if a == nil || a.store == nil || a.client == nil {
		return store.Presentation{}, errors.New("semantic assessor is not configured")
	}
	assessment, err := a.store.Assessment(ctx, id)
	if err != nil {
		return store.Presentation{}, err
	}
	if assessment.Status != "pending" {
		return a.store.FinalizeAssessment(ctx, id, store.AssessmentResult{})
	}
	if assessment.Quiz.Rubric == nil {
		return a.fail(ctx, id, Response{}, ErrMalformed)
	}
	request := Request{}
	var requestJSON []byte
	if assessment.Transmissions > 0 {
		requestJSON = []byte(assessment.RequestJSON)
		if err = json.Unmarshal(requestJSON, &request); err != nil || request.Model == "" {
			return a.fail(ctx, id, Response{}, ErrMalformed)
		}
	} else {
		request = BuildRecallRequest(a.model, RecallState{
			Prompt: assessment.Quiz.Prompt, ExpectedAnswer: assessment.Quiz.Answer,
			LearnerAnswer: assessment.Answer, Variants: assessment.Quiz.Variants, Rubric: *assessment.Quiz.Rubric,
		})
		requestJSON, err = json.Marshal(request)
		if err != nil {
			return a.fail(ctx, id, Response{}, ErrMalformed)
		}
	}
	assessment, err = a.store.BeginAssessmentTransmission(ctx, id, request.Model, requestJSON)
	if err != nil {
		return store.Presentation{}, err
	}
	if assessment.Status != "pending" {
		return a.store.FinalizeAssessment(ctx, id, store.AssessmentResult{})
	}
	response, decisionErr := a.client.Decide(ctx, request)
	if decisionErr != nil {
		return a.fail(ctx, id, response, decisionErr)
	}
	judgments, err := judgments(response, len(assessment.Quiz.Rubric.Required), len(assessment.Quiz.Rubric.Contradictions))
	if err != nil {
		return a.fail(ctx, id, response, ErrMalformed)
	}
	return a.store.FinalizeAssessment(ctx, id, assessmentResult(response, judgments, ""))
}

func (a *Assessor) fail(ctx context.Context, id string, response Response, cause error) (store.Presentation, error) {
	classification := "malformed"
	switch {
	case errors.Is(cause, ErrUnavailable):
		classification = "unavailable"
	case errors.Is(cause, ErrRejected):
		classification = "rejected"
	}
	return a.store.FailAssessment(ctx, id, assessmentResult(response, learning.SemanticJudgments{}, classification))
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
