// Package evals is Scry's 4-surface micro-eval bench: grading, the "Why?"
// explainer, drafting and speech. The datasets are embedded so the offline
// tests and the live runner (cmd/scry-eval) read the same bytes. Nothing in
// this package is used by the Scry service; only an explicit run of
// cmd/scry-eval sends data to a provider.
package evals

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

//go:embed datasets/grading.jsonl datasets/explainer.jsonl datasets/drafting.jsonl datasets/speech/manifest.jsonl datasets/speech/*.mp3
var datasetFS embed.FS

// GradingCase is one answer to grade under Scry's 3-way contract.
// Label is the ground truth: correct, close or incorrect (right, close, missed).
type GradingCase struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Prompt   string   `json:"prompt"`
	Accepted []string `json:"accepted_answers"`
	Answer   string   `json:"answer"`
	Label    string   `json:"label"`
}

// ExplainerCase is a missed or close card the learner asked "Why?" about.
// Each RequiredConcepts entry is a set of alternatives separated by "|";
// an explanation covers it when it contains any alternative.
type ExplainerCase struct {
	ID               string   `json:"id"`
	Domain           string   `json:"domain"`
	Question         string   `json:"question"`
	Expected         string   `json:"expected"`
	LearnerAnswer    string   `json:"learner_answer"`
	Verdict          string   `json:"verdict"`
	KeyMisconception string   `json:"key_misconception"`
	RequiredConcepts []string `json:"required_concepts"`
}

// DraftLearner mirrors ai.Learner.
type DraftLearner struct {
	About string   `json:"about"`
	Knows []string `json:"knows"`
	Level string   `json:"level"`
}

// DraftingCase mirrors ai.DraftInput plus the kind a correct head names.
type DraftingCase struct {
	ID           string       `json:"id"`
	Category     string       `json:"category"` // note, topic or book
	ExpectedKind string       `json:"expected_kind"`
	Query        string       `json:"query"`
	Learner      DraftLearner `json:"learner"`
}

// SpeechClip is one spoken answer and the card it answers, so the same
// clip measures transcription and audio-to-grade.
type SpeechClip struct {
	ID            string   `json:"id"`
	File          string   `json:"file"`
	DurationS     float64  `json:"duration_s"`
	TTSEngine     string   `json:"tts_engine"`
	Voice         string   `json:"voice"`
	SpokenText    string   `json:"spoken_text"`
	Prompt        string   `json:"prompt"`
	Accepted      []string `json:"accepted_answers"`
	ExpectedGrade string   `json:"expected_grade"`
	CriticalTerms []string `json:"critical_terms"`
	Audio         []byte   `json:"-"`
}

// Datasets is every case the bench runs.
type Datasets struct {
	Grading   []GradingCase
	Explainer []ExplainerCase
	Drafting  []DraftingCase
	Speech    []SpeechClip
}

func readJSONL[T any](path string) ([]T, error) {
	raw, err := datasetFS.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []T
	scan := bufio.NewScanner(bytes.NewReader(raw))
	scan.Buffer(make([]byte, 4096), 1<<20)
	for n := 1; scan.Scan(); n++ {
		line := strings.TrimSpace(scan.Text())
		if line == "" {
			continue
		}
		var r T
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, n, err)
		}
		rows = append(rows, r)
	}
	return rows, scan.Err()
}

// LoadDatasets reads and validates the embedded datasets.
func LoadDatasets() (*Datasets, error) {
	var d Datasets
	var err error
	if d.Grading, err = readJSONL[GradingCase]("datasets/grading.jsonl"); err != nil {
		return nil, err
	}
	if d.Explainer, err = readJSONL[ExplainerCase]("datasets/explainer.jsonl"); err != nil {
		return nil, err
	}
	if d.Drafting, err = readJSONL[DraftingCase]("datasets/drafting.jsonl"); err != nil {
		return nil, err
	}
	if d.Speech, err = readJSONL[SpeechClip]("datasets/speech/manifest.jsonl"); err != nil {
		return nil, err
	}
	for i := range d.Speech {
		if d.Speech[i].Audio, err = datasetFS.ReadFile("datasets/speech/" + d.Speech[i].File); err != nil {
			return nil, err
		}
	}
	return &d, d.Validate()
}

// Validate checks each row's schema; it does not check the curated counts
// (the tests do).
func (d *Datasets) Validate() error {
	ids := map[string]bool{}
	dup := func(id string) error {
		if id == "" || ids[id] {
			return fmt.Errorf("missing or duplicate id %q", id)
		}
		ids[id] = true
		return nil
	}
	for _, c := range d.Grading {
		if err := dup(c.ID); err != nil {
			return err
		}
		if c.Prompt == "" || c.Category == "" || len(c.Accepted) == 0 || !oneOf(c.Label, "correct", "close", "incorrect") {
			return fmt.Errorf("grading %s: invalid row", c.ID)
		}
	}
	for _, c := range d.Explainer {
		if err := dup(c.ID); err != nil {
			return err
		}
		if c.Question == "" || c.Expected == "" || c.LearnerAnswer == "" || c.KeyMisconception == "" || len(c.RequiredConcepts) == 0 ||
			!oneOf(c.Verdict, "missed", "close") || !oneOf(c.Domain, "STEM", "history", "language", "code", "finance") {
			return fmt.Errorf("explainer %s: invalid row", c.ID)
		}
	}
	for _, c := range d.Drafting {
		if err := dup(c.ID); err != nil {
			return err
		}
		if c.Query == "" || len(c.Learner.Knows) == 0 || c.Learner.About == "" || !oneOf(c.Category, "note", "topic", "book") || !oneOf(c.ExpectedKind, "topic", "book") {
			return fmt.Errorf("drafting %s: invalid row", c.ID)
		}
	}
	for _, c := range d.Speech {
		if err := dup(c.ID); err != nil {
			return err
		}
		if c.SpokenText == "" || c.Prompt == "" || len(c.Accepted) == 0 || len(c.CriticalTerms) == 0 || len(c.CriticalTerms) > 3 ||
			!oneOf(c.ExpectedGrade, "correct", "incorrect") || c.DurationS < 3 || c.DurationS > 6 || len(c.Audio) == 0 || !utf8.ValidString(c.SpokenText) {
			return fmt.Errorf("speech %s: invalid row", c.ID)
		}
	}
	return nil
}

func oneOf(s string, opts ...string) bool {
	for _, o := range opts {
		if s == o {
			return true
		}
	}
	return false
}
