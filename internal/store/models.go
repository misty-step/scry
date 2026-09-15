// Package store is the single durable owner of Scry content, learning and jobs.
package store

import "errors"

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalid  = errors.New("invalid input")
	ErrBudget   = errors.New("generation budget exhausted")
)

const (
	SchemaVersion       = 2
	ApplicationID       = 0x53435259 // SCRY
	MaxSourceBytes      = 32 * 1024
	MaxGeneratedQuizzes = 60
)

type Quiz struct {
	ID          string   `json:"id"`
	SourceID    string   `json:"source_id"`
	Kind        string   `json:"kind"`
	Prompt      string   `json:"prompt"`
	Answer      string   `json:"answer"`
	Explanation string   `json:"explanation"`
	Evidence    string   `json:"evidence"`
	Basis       string   `json:"basis"`
	Choices     []string `json:"choices"`
	Variants    []string `json:"variants"`
	Version     int      `json:"version"`
	Archived    bool     `json:"archived"`
	DueAt       int64    `json:"due_at"`
}

type Source struct {
	ID        string         `json:"id"`
	Text      string         `json:"text"`
	Kind      string         `json:"kind"`
	Status    string         `json:"status"`
	Revision  int            `json:"revision"`
	Archived  bool           `json:"archived"`
	CreatedAt int64          `json:"created_at"`
	Quizzes   []Quiz         `json:"quizzes,omitempty"`
	Job       *Job           `json:"job,omitempty"`
	GoalID    string         `json:"goal_id"`
	Materials []Material     `json:"materials,omitempty"`
	Coverage  CoverageReport `json:"coverage"`
}

type Presentation struct {
	ID             string    `json:"id"`
	GoalID         string    `json:"goal_id"`
	GoalRevision   int       `json:"goal_revision"`
	GoalPinOrigin  string    `json:"goal_pin_origin"`
	Kind           string    `json:"kind"`
	Material       *Material `json:"material,omitempty"`
	BridgeID       string    `json:"bridge_id"`
	PlanningReason string    `json:"planning_reason"`
	Practice       bool      `json:"practice"`
	Quiz           Quiz      `json:"quiz"`
	Answer         string    `json:"answer"`
	Outcome        string    `json:"outcome"`
	Assisted       bool      `json:"assisted"`
	Graded         bool      `json:"graded"`
	Disputed       bool      `json:"disputed"`
	Rating         int       `json:"rating"`
	DueAt          int64     `json:"due_at"`
	ReviewedAt     int64     `json:"reviewed_at"`
	ReviewID       string    `json:"review_id"`
}

type ReviewState struct {
	Current   *Presentation   `json:"current"`
	Preview   *Presentation   `json:"preview"`
	Suspended []*Presentation `json:"suspended"`
	Bridge    *Bridge         `json:"bridge,omitempty"`
	Due       int             `json:"due"`
	Total     int             `json:"total"`
	NextDueAt int64           `json:"next_due_at"`
}

type ReviewEvent struct {
	ID             string         `json:"id"`
	PresentationID string         `json:"presentation_id"`
	Quiz           Quiz           `json:"quiz"`
	Answer         string         `json:"answer"`
	Outcome        string         `json:"outcome"`
	Rating         int            `json:"rating"`
	Assisted       bool           `json:"assisted"`
	Disputed       bool           `json:"disputed"`
	ReviewedAt     int64          `json:"reviewed_at"`
	DueAt          int64          `json:"due_at"`
	Algorithm      string         `json:"algorithm"`
	Material       *Material      `json:"material,omitempty"`
	Coverage       []CoverageLink `json:"coverage"`
}

type Job struct {
	ID                        string           `json:"id"`
	MetadataOnly              bool             `json:"metadata_only,omitempty"`
	SourceID                  string           `json:"source_id"`
	Status                    string           `json:"status"`
	Error                     string           `json:"error"`
	Model                     string           `json:"model"`
	LeaseToken                string           `json:"-"`
	SourceText                string           `json:"source_text"`
	SourceKind                string           `json:"source_kind"`
	SourceRevision            int              `json:"source_revision"`
	Attempts                  int              `json:"attempts"`
	CreatedAt                 int64            `json:"created_at"`
	UpdatedAt                 int64            `json:"updated_at"`
	CostMicros                int64            `json:"cost_micros"`
	ReservedMicros            int64            `json:"reserved_micros"`
	Published                 int              `json:"published"`
	Kind                      string           `json:"kind"`
	GoalID                    string           `json:"goal_id"`
	GoalRevision              int              `json:"goal_revision"`
	TargetMaterialID          string           `json:"target_material_id"`
	TargetMaterialVersion     int              `json:"target_material_version"`
	TargetPresentationID      string           `json:"target_presentation_id"`
	TargetPresentationVersion int              `json:"target_presentation_version"`
	SuggestionID              string           `json:"suggestion_id"`
	ObservationID             string           `json:"observation_id"`
	ParentJobID               string           `json:"parent_job_id"`
	RetryRootID               string           `json:"retry_root_id"`
	RetryAvailable            bool             `json:"retry_available"`
	RetryWarning              string           `json:"retry_warning"`
	Context                   KnowledgeContext `json:"context"`
	NewMaterials              int              `json:"new_materials"`
	ReusedMaterials           int              `json:"reused_materials"`
	NewQuizzes                int              `json:"new_quizzes"`
	Coverage                  CoverageReport   `json:"coverage"`
	CostUnknown               bool             `json:"cost_unknown"`
}

type GeneratedQuiz struct {
	Key              string          `json:"key"`
	ReuseID          string          `json:"reuse_id"`
	Level            string          `json:"level"`
	Links            []GeneratedLink `json:"links"`
	EstimatedSeconds int             `json:"estimated_seconds"`
	Kind             string          `json:"kind"`
	Prompt           string          `json:"prompt"`
	Answer           string          `json:"answer"`
	Explanation      string          `json:"explanation"`
	Evidence         string          `json:"evidence"`
	Basis            string          `json:"basis"`
	Choices          []string        `json:"choices"`
	Variants         []string        `json:"variants"`
}

type GenerationResult struct {
	Quizzes       []GeneratedQuiz       `json:"quizzes"`
	Coverage      CoverageReport        `json:"coverage"`
	Units         []GeneratedUnit       `json:"units"`
	Relations     []GeneratedRelation   `json:"relations"`
	Materials     []GeneratedMaterial   `json:"materials"`
	Suggestions   []GeneratedSuggestion `json:"suggestions"`
	Model         string                `json:"model"`
	PromptVersion string                `json:"prompt_version"`
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
	Sources        int           `json:"sources"`
	Goals          int           `json:"goals"`
	KnowledgeUnits int           `json:"knowledge_units"`
	Materials      int           `json:"materials"`
	Quizzes        int           `json:"quizzes"`
	Due            int           `json:"due"`
	Reviews        int           `json:"reviews"`
	Assisted       int           `json:"assisted"`
	Disputed       int           `json:"disputed"`
	NextDueAt      int64         `json:"next_due_at"`
	CostMicros     int64         `json:"cost_micros"`
	CostUnknown    bool          `json:"cost_unknown"`
	LastBackup     *BackupRecord `json:"last_backup"`
}
