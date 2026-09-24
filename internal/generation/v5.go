package generation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/misty-step/scry/internal/semantic"
	"github.com/misty-step/scry/internal/store"
)

func (w *Worker) generateV5(ctx context.Context, job *store.Job, input store.JobContext) (store.GenerationResult, *int64, *generationFailure) {
	zero := int64(0)
	if job.Kind == "research" {
		if w.exaError != "" {
			return store.GenerationResult{}, &zero, &generationFailure{message: w.exaError}
		}
		if w.exa == nil {
			return store.GenerationResult{}, &zero, &generationFailure{message: "Web search is not configured."}
		}
		mode := job.SourceMode
		if mode == "link" && w.cfg.ExaAPIKey == "" {
			return store.GenerationResult{}, &zero, &generationFailure{message: "Scry couldn't read that page. Paste its text as My text instead, or retry later."}
		}
		documents, cost, err := w.exa.research(ctx, mode, job.SourceText)
		if err != nil {
			return store.GenerationResult{}, cost, &generationFailure{message: err.Error()}
		}
		if mode == "link" && len(documents) == 0 {
			return store.GenerationResult{}, cost, &generationFailure{message: "Scry couldn't read that page. Paste its text as My text instead, or retry later."}
		}
		note, provider := "Web excerpts are ready for the study plan.", "exa"
		if w.cfg.ExaAPIKey == "" {
			note, provider = "Web search was unavailable; the study plan will use general knowledge.", "unavailable"
		}
		if len(documents) == 0 && w.cfg.ExaAPIKey != "" {
			note = "No readable web excerpts were found; the study plan will use general knowledge."
		}
		return store.GenerationResult{Documents: documents, Note: note, Model: provider, PromptVersion: "scry-research-v1"}, cost, nil
	}
	if job.Kind != "plan" && job.Kind != "questions" && job.Kind != "fix" && job.Kind != "transcribe" {
		return store.GenerationResult{}, &zero, &generationFailure{message: "This study preparation type is not supported."}
	}
	var request []byte
	var err error
	if job.Kind == "transcribe" {
		if input.Image == nil || len(input.Image.Bytes) == 0 || len(input.Image.Bytes) > 4<<20 || (input.Image.MIME != "image/jpeg" && input.Image.MIME != "image/png" && input.Image.MIME != "image/webp") {
			return store.GenerationResult{}, &zero, &generationFailure{message: "The photo is missing or exceeds its safe size limit."}
		}
		request, err = v5Request(w.cfg.Model, job.Kind, map[string]string{"caption": job.SourceText}, w.openRouter)
		if err == nil {
			var payload map[string]any
			if json.Unmarshal(request, &payload) == nil {
				payload["messages"] = []any{map[string]any{"role": "system", "content": transcribePrompt}, map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": string(mustJSON(map[string]string{"caption": job.SourceText}))}, map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:" + input.Image.MIME + ";base64," + base64.StdEncoding.EncodeToString(input.Image.Bytes)}}}}}
				request, err = json.Marshal(payload)
				if len(request) > 8<<20 {
					err = errors.New("photo request exceeds limit")
				}
			}
		}
	} else {
		if job.Kind == "plan" || job.Kind == "questions" {
			var contract coveragePlan
			taskText, taskKind := v5TaskMaterial(job, input)
			contract, err = planTask(taskText, taskKind)
			if err == nil {
				request, err = v5Request(w.cfg.Model, job.Kind, map[string]any{"input": v5SourceInput(job, input), "task_contract": contract}, w.openRouter)
			}
		} else {
			request, err = v5Request(w.cfg.Model, job.Kind, v5SourceInput(job, input), w.openRouter)
		}
	}
	if err != nil {
		return store.GenerationResult{}, &zero, &generationFailure{message: "The saved material is too large or invalid for preparation; split it and try again."}
	}
	response, failure := w.call(ctx, request)
	if failure != nil {
		return store.GenerationResult{}, response.cost, failure
	}
	result, err := validateV5Output(job, input, response.content)
	if err != nil {
		// Name the failed check, as v4 did, so the source page shows why.
		return store.GenerationResult{}, response.cost, &generationFailure{message: "Scry could not write material that passed its checks (" + checkReason(err) + "). Nothing was published, and the usage was recorded. Try again, or add clearer material."}
	}
	result.Model = response.model
	if result.Model == "" {
		result.Model = w.cfg.Model
	}
	result.PromptVersion = "scry-" + job.Kind + "-v1"
	if job.Kind == "plan" && w.cfg.Critic != nil {
		if err = w.dedupePlan(ctx, job, result.Plan); err != nil {
			return store.GenerationResult{}, response.cost, &generationFailure{message: "Concept matching could not be completed. The study plan was not published; check recorded usage before retrying."}
		}
	}
	return result, response.cost, nil
}

func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }

// checkReason bounds a validation error for display: control characters are
// dropped and the text is capped, because it may echo model output.
func checkReason(err error) string {
	reason := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, err.Error()))
	if len(reason) > 160 {
		reason = append(reason[:159], '…')
	}
	return string(reason)
}

func (w *Worker) dedupePlan(ctx context.Context, job *store.Job, plan *store.PlanContent) error {
	if plan == nil {
		return nil
	}
	for i := range plan.Concepts {
		concept := &plan.Concepts[i]
		if concept.ExistingID != "" {
			continue
		}
		found, err := w.store.SearchConcepts(ctx, concept.Name+" "+concept.Summary, 12)
		if err != nil {
			return err
		}
		if len(found) == 0 {
			continue
		}
		candidates := make([]semantic.DedupeConcept, len(found))
		for j, candidate := range found {
			candidates[j] = semantic.DedupeConcept{ID: candidate.ID, Name: candidate.Name, Summary: candidate.Summary}
		}
		request := semantic.BuildDedupeRequest(w.cfg.CriticModel, semantic.DedupeConcept{Name: concept.Name, Summary: concept.Summary}, candidates)
		requestJSON, err := json.Marshal(request)
		if err != nil {
			return err
		}
		id, token, send, err := w.store.BeginJudgment(ctx, job.ID, job.LeaseToken, "dedupe", i, map[string]any{"proposed": concept.Name, "candidates": candidates}, request.Model, requestJSON, w.cfg.CriticSpending)
		if err != nil {
			return err
		}
		if !send {
			continue
		}
		response, callErr := w.cfg.Critic.Decide(ctx, request)
		result := store.AssessmentResult{ResponseModel: response.Model, ResponseJSON: response.Raw, InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens, CostMicros: response.Usage.CostMicros, LatencyMS: response.LatencyMS}
		match := semantic.DedupeMatch(response, candidates)
		decision := "new"
		if match != "" {
			decision = "same:" + match
		}
		if callErr != nil {
			result.Error = "unavailable"
		} else if !semantic.DedupeValid(response, candidates) {
			result.Error = "malformed"
		}
		if result.Error != "" {
			decision = ""
		}
		settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleTimeout)
		err = w.store.FinishJudgment(settleCtx, id, token, result, decision)
		cancel()
		if err != nil {
			return fmt.Errorf("finish concept comparison: %w", err)
		}
		if match != "" && callErr == nil {
			concept.ExistingID = match
			concept.Note = nil
		}
	}
	return nil
}

func v5TaskMaterial(job *store.Job, input store.JobContext) (string, string) {
	if job.SourceMode == "photo" {
		for _, document := range input.Documents {
			if document.Kind == "transcript" {
				return document.Text, "source"
			}
		}
	}
	return job.SourceText, job.SourceKind
}
