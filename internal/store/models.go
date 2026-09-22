// Package store is the single durable owner of Scry content, learning and jobs.
package store

import (
	"errors"

	"github.com/misty-step/scry/internal/learning"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalid  = errors.New("invalid input")
	ErrBudget   = errors.New("generation budget exhausted")
)

const (
	SchemaVersion       = 4
	ApplicationID       = 0x53435259 // SCRY
	MaxSourceBytes      = 32 * 1024
	MaxGeneratedQuizzes = 60
)

type Rubric = learning.Rubric
type RubricIdea = learning.RubricIdea
type RubricClaim = learning.RubricClaim

type Quiz struct {
	ID          string           `json:"id"`
	SourceID    string           `json:"source_id"`
	Kind        string           `json:"kind"`
	Grading     string           `json:"grading,omitempty"`
	Rubric      *learning.Rubric `json:"rubric,omitempty"`
	Prompt      string           `json:"prompt"`
	Answer      string           `json:"answer"`
	Explanation string           `json:"explanation"`
	Evidence    string           `json:"evidence"`
	Basis       string           `json:"basis"`
	Choices     []string         `json:"choices"`
	Variants    []string         `json:"variants"`
	Version     int              `json:"version"`
	Archived    bool             `json:"archived"`
	DueAt       int64            `json:"due_at"`
	AvailableAt int64            `json:"available_at"`
}

type Source struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	Revision  int    `json:"revision"`
	Archived  bool   `json:"archived"`
	CreatedAt int64  `json:"created_at"`
	Quizzes   []Quiz `json:"quizzes,omitempty"`
	Job       *Job   `json:"job,omitempty"`
}

type Presentation struct {
	ID                    string `json:"id"`
	Quiz                  Quiz   `json:"quiz"`
	Answer                string `json:"answer"`
	Draft                 string `json:"draft,omitempty"`
	BridgeRevision        int    `json:"bridge_revision"`
	Outcome               string `json:"outcome"`
	Assisted              bool   `json:"assisted"`
	Graded                bool   `json:"graded"`
	Disputed              bool   `json:"disputed"`
	Rating                int    `json:"rating"`
	DueAt                 int64  `json:"due_at"`
	ReviewedAt            int64  `json:"reviewed_at"`
	ReviewID              string `json:"review_id"`
	Pending               bool   `json:"pending,omitempty"`
	AssessmentID          string `json:"assessment_id,omitempty"`
	AssessmentOperationID string `json:"assessment_operation_id,omitempty"`
	AssessmentStatus      string `json:"assessment_status,omitempty"`
	AssessmentDecision    string `json:"assessment_decision,omitempty"`
	AssessmentDetail      string `json:"assessment_detail,omitempty"`
}

type ReviewState struct {
	Current   *Presentation `json:"current"`
	Preview   *Presentation `json:"preview"`
	Due       int           `json:"due"`
	Total     int           `json:"total"`
	NextDueAt int64         `json:"next_due_at"`
}

type ReviewEvent struct {
	ID             string `json:"id"`
	PresentationID string `json:"presentation_id"`
	Quiz           Quiz   `json:"quiz"`
	Answer         string `json:"answer"`
	Outcome        string `json:"outcome"`
	Rating         int    `json:"rating"`
	Assisted       bool   `json:"assisted"`
	Disputed       bool   `json:"disputed"`
	ReviewedAt     int64  `json:"reviewed_at"`
	DueAt          int64  `json:"due_at"`
	Algorithm      string `json:"algorithm"`
	Grading        string `json:"grading"`
}

type Job struct {
	ID               string `json:"id"`
	SourceID         string `json:"source_id"`
	Status           string `json:"status"`
	Error            string `json:"error"`
	Model            string `json:"model"`
	LeaseToken       string `json:"-"`
	SourceText       string `json:"source_text"`
	SourceKind       string `json:"source_kind"`
	SourceRevision   int    `json:"source_revision"`
	Attempts         int    `json:"attempts"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
	CostMicros       int64  `json:"cost_micros"`
	ReservedMicros   int64  `json:"reserved_micros"`
	Published        int    `json:"published"`
	CostUnknown      bool   `json:"cost_unknown"`
	Foundation       bool   `json:"foundation"`
	FoundationTarget *Quiz  `json:"-"`
}

type GeneratedQuiz struct {
	Kind        string           `json:"kind"`
	Grading     string           `json:"grading,omitempty"`
	Rubric      *learning.Rubric `json:"rubric,omitempty"`
	Prompt      string           `json:"prompt"`
	Answer      string           `json:"answer"`
	Explanation string           `json:"explanation"`
	Evidence    string           `json:"evidence"`
	Basis       string           `json:"basis"`
	Choices     []string         `json:"choices"`
	Variants    []string         `json:"variants"`
}

type GenerationResult struct {
	Quizzes       []GeneratedQuiz    `json:"quizzes"`
	Partial       bool               `json:"partial"`
	Note          string             `json:"note"`
	Model         string             `json:"model"`
	PromptVersion string             `json:"prompt_version"`
	Foundation    *FoundationContent `json:"foundation,omitempty"`
}

type Assessment struct {
	ID              string
	PresentationID  string
	OperationID     string
	ContentVersion  int
	ScheduleVersion int
	Answer          string
	Status          string
	PolicyVersion   string
	RequestModel    string
	RequestJSON     string
	Transmissions   int
	ReservedMicros  int64
	LeaseToken      string
	LeaseUntil      int64
	Decision        string
	Applied         bool
	Detail          string
	Error           string
	ReviewID        string
	Quiz            Quiz
}

// AssessmentLease is the outcome of BeginAssessmentTransmission: exactly one
// caller holds Token for one send; every other caller sees Send=false and the
// durable assessment state to return to the learner.
type AssessmentLease struct {
	Assessment Assessment
	Token      string
	Send       bool
}

type AssessmentResult struct {
	ResponseModel string
	ResponseJSON  []byte
	Judgments     learning.SemanticJudgments
	InputTokens   int
	OutputTokens  int
	CostMicros    *int64
	LatencyMS     int64
	Error         string
	// NoSend marks a definite failure in which no request left the process,
	// so the reservation can be released instead of retained as unknown spend.
	NoSend bool
}

type BackupRecord struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	RemoteKey string `json:"remote_key"`
	SHA256    string `json:"sha256"`
	Error     string `json:"error"`
	CreatedAt int64  `json:"created_at"`
	Bytes     int64  `json:"bytes"`
	Remote    bool   `json:"remote"`
}

type Summary struct {
	Sources     int           `json:"sources"`
	Quizzes     int           `json:"quizzes"`
	Due         int           `json:"due"`
	Reviews     int           `json:"reviews"`
	Assisted    int           `json:"assisted"`
	Disputed    int           `json:"disputed"`
	NextDueAt   int64         `json:"next_due_at"`
	CostMicros  int64         `json:"cost_micros"`
	CostUnknown bool          `json:"cost_unknown"`
	LastBackup  *BackupRecord `json:"last_backup"`
}

type Concept struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
}

type Reference struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Format    string `json:"format"`
	SourceURL string `json:"source_url,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

type ConceptDetail struct {
	Concept
	References    []Reference `json:"references,omitempty"`
	Quizzes       []Quiz      `json:"quizzes,omitempty"`
	Prerequisites []Concept   `json:"prerequisites,omitempty"`
}

type ReferenceDetail struct {
	Reference
	Concepts []Concept `json:"concepts,omitempty"`
}

type SearchResult struct {
	Concepts   []ConceptDetail   `json:"concepts"`
	References []ReferenceDetail `json:"references"`
}
