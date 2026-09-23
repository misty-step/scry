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
	SchemaVersion       = 5
	ApplicationID       = 0x53435259 // SCRY
	MaxSourceBytes      = 32 * 1024
	MaxGeneratedQuizzes = 60
	MaxImageBytes       = 4 << 20
	MaxPlanConcepts     = 12
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
	Level       string           `json:"level,omitempty"`
	AnswerForm  string           `json:"answer_form,omitempty"`
	Citations   []Citation       `json:"citations,omitempty"`
	ConceptID   string           `json:"concept_id,omitempty"`
	Version     int              `json:"version"`
	Archived    bool             `json:"archived"`
	DueAt       int64            `json:"due_at"`
	AvailableAt int64            `json:"available_at"`
}

// Citation points at a stored source document (a web search result or a
// fetched page). It is provenance, not proof that the page is true.
type Citation struct {
	DocumentID string `json:"document_id,omitempty"`
	Title      string `json:"title"`
	URL        string `json:"url"`
}

type Source struct {
	ID        string           `json:"id"`
	Text      string           `json:"text"`
	Kind      string           `json:"kind"`
	Mode      string           `json:"mode"`
	Web       bool             `json:"web"`
	Status    string           `json:"status"`
	Revision  int              `json:"revision"`
	Archived  bool             `json:"archived"`
	CreatedAt int64            `json:"created_at"`
	GoalID    string           `json:"goal_id,omitempty"`
	HasImage  bool             `json:"has_image,omitempty"`
	Quizzes   []Quiz           `json:"quizzes,omitempty"`
	Job       *Job             `json:"job,omitempty"`
	Jobs      []Job            `json:"jobs,omitempty"`
	Documents []SourceDocument `json:"documents,omitempty"`
}

// CaptureInput is one explicit learner capture. Mode is the learner's choice:
// "topic" may be researched on the web; "text" (pasted material) never leaves
// for search; "link" reads one page; "photo" transcribes an image.
type CaptureInput struct {
	Text      string
	Mode      string
	Image     []byte
	ImageMIME string
}

type CaptureImage struct {
	MIME  string
	Bytes []byte
}

type SourceDocument struct {
	ID        string `json:"id"`
	SourceID  string `json:"source_id"`
	JobID     string `json:"job_id"`
	Kind      string `json:"kind"`
	Position  int    `json:"position"`
	URL       string `json:"url"`
	Title     string `json:"title"`
	Published string `json:"published"`
	Text      string `json:"text"`
	Provider  string `json:"provider"`
	CreatedAt int64  `json:"created_at"`
}

type DocumentContent struct {
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	Title     string `json:"title"`
	Published string `json:"published"`
	Text      string `json:"text"`
	Provider  string `json:"provider"`
}

type Presentation struct {
	ID                    string      `json:"id"`
	Quiz                  Quiz        `json:"quiz"`
	Concept               *ConceptRef `json:"concept,omitempty"`
	Answer                string      `json:"answer"`
	Draft                 string      `json:"draft,omitempty"`
	Outcome               string      `json:"outcome"`
	Assisted              bool        `json:"assisted"`
	Graded                bool        `json:"graded"`
	Disputed              bool        `json:"disputed"`
	SelfCheck             bool        `json:"self_check,omitempty"`
	SelfCheckReason       string      `json:"self_check_reason,omitempty"`
	Override              string      `json:"override,omitempty"`
	Authority             string      `json:"authority,omitempty"`
	Rating                int         `json:"rating"`
	DueAt                 int64       `json:"due_at"`
	ReviewedAt            int64       `json:"reviewed_at"`
	ReviewID              string      `json:"review_id"`
	Pending               bool        `json:"pending,omitempty"`
	AssessmentID          string      `json:"assessment_id,omitempty"`
	AssessmentOperationID string      `json:"assessment_operation_id,omitempty"`
	AssessmentStatus      string      `json:"assessment_status,omitempty"`
	AssessmentDecision    string      `json:"assessment_decision,omitempty"`
	AssessmentDetail      string      `json:"assessment_detail,omitempty"`
}

type ReviewState struct {
	Current        *Presentation `json:"current"`
	Preview        *Presentation `json:"preview"`
	Intro          *ConceptIntro `json:"intro,omitempty"`
	CurrentConcept *ConceptBrief `json:"current_concept,omitempty"`
	Preparing      []Preparing   `json:"preparing,omitempty"`
	Due            int           `json:"due"`
	Total          int           `json:"total"`
	NextDueAt      int64         `json:"next_due_at"`
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
	Override       string `json:"override,omitempty"`
	Authority      string `json:"authority"`
	ReviewedAt     int64  `json:"reviewed_at"`
	DueAt          int64  `json:"due_at"`
	Algorithm      string `json:"algorithm"`
	Grading        string `json:"grading"`
}

type Job struct {
	ID             string          `json:"id"`
	SourceID       string          `json:"source_id"`
	Kind           string          `json:"kind"`
	Payload        string          `json:"-"`
	Status         string          `json:"status"`
	Error          string          `json:"error"`
	Model          string          `json:"model"`
	LeaseToken     string          `json:"-"`
	SourceText     string          `json:"source_text"`
	SourceKind     string          `json:"source_kind"`
	SourceMode     string          `json:"source_mode"`
	SourceWeb      bool            `json:"source_web"`
	SourceRevision int             `json:"source_revision"`
	Attempts       int             `json:"attempts"`
	CreatedAt      int64           `json:"created_at"`
	UpdatedAt      int64           `json:"updated_at"`
	CostMicros     int64           `json:"cost_micros"`
	ReservedMicros int64           `json:"reserved_micros"`
	Published      int             `json:"published"`
	CostUnknown    bool            `json:"cost_unknown"`
	CriticStatus   string          `json:"critic_status"`
	Candidates     *CandidateBatch `json:"-"`
}

// CandidateBatch preserves validated generator output and its usage before any
// critic transmission. Critic retries never generate this material again.
type CandidateBatch struct {
	Result     GenerationResult `json:"result"`
	CostMicros *int64           `json:"cost_micros"`
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
	Concept     string           `json:"concept,omitempty"`
	Level       string           `json:"level,omitempty"`
	AnswerForm  string           `json:"answer_form,omitempty"`
	Citations   []Citation       `json:"citations,omitempty"`
}

type GenerationResult struct {
	Quizzes       []GeneratedQuiz   `json:"quizzes"`
	Partial       bool              `json:"partial"`
	Note          string            `json:"note"`
	Model         string            `json:"model"`
	PromptVersion string            `json:"prompt_version"`
	Documents     []DocumentContent `json:"documents,omitempty"`
	Plan          *PlanContent      `json:"plan,omitempty"`
}

// NoteContent is generated reference material for one concept. Evidence holds
// exact quotations from the learner's material or a stored web document;
// Citations name the web documents a web-basis note relies on.
type NoteContent struct {
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Basis     string     `json:"basis"`
	Evidence  []string   `json:"evidence"`
	Citations []Citation `json:"citations"`
}

// PlannedConcept is one concept in a generated study plan. Key is local to the
// plan; ExistingID reuses a concept the learner already has.
type PlannedConcept struct {
	Key          string       `json:"key"`
	ExistingID   string       `json:"existing_id"`
	Name         string       `json:"name"`
	Summary      string       `json:"summary"`
	Requires     []string     `json:"requires"`
	PartOf       []string     `json:"part_of"`
	ConfusedWith []string     `json:"confused_with"`
	Note         *NoteContent `json:"note"`
}

type PlanContent struct {
	Goal     string           `json:"goal"`
	Concepts []PlannedConcept `json:"concepts"`
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
	// Short carries short-v1 judgments; Judgments carries semantic-v1 rubric
	// judgments. The staged assessment's policy decides which one applies.
	Short        learning.ShortJudgments
	InputTokens  int
	OutputTokens int
	CostMicros   *int64
	LatencyMS    int64
	Error        string
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
	Sources        int           `json:"sources"`
	Goals          int           `json:"goals"`
	Concepts       int           `json:"concepts"`
	Quizzes        int           `json:"quizzes"`
	Due            int           `json:"due"`
	Reviews        int           `json:"reviews"`
	Assisted       int           `json:"assisted"`
	Disputed       int           `json:"disputed"`
	NextDueAt      int64         `json:"next_due_at"`
	CostMicros     int64         `json:"cost_micros"`
	DayCostMicros  int64         `json:"day_cost_micros"`
	WeekCostMicros int64         `json:"week_cost_micros"`
	CostUnknown    bool          `json:"cost_unknown"`
	LastBackup     *BackupRecord `json:"last_backup"`
}

type Concept struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
	Origin      string `json:"origin"`
	Status      string `json:"status"`
	SourceID    string `json:"source_id"`
}

// ConceptRef is stable identity for display inside a presentation. It never
// carries time-dependent state, so a saved result reads back identically.
type ConceptRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ConceptBrief is a concept with its current estimated state. It changes with
// time and evidence, so it never appears inside a Presentation.
type ConceptBrief struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Summary    string  `json:"summary,omitempty"`
	Status     string  `json:"status"`
	Recall     float64 `json:"recall"`
	Brightness int     `json:"brightness"`
}
type ConceptState = learning.ConceptState

// Note is one immutable version of reference material for a concept.
type Note struct {
	ID            string     `json:"id"`
	ConceptID     string     `json:"concept_id"`
	Title         string     `json:"title"`
	Body          string     `json:"body"`
	Basis         string     `json:"basis"`
	Evidence      []string   `json:"evidence"`
	Citations     []Citation `json:"citations"`
	SourceID      string     `json:"source_id"`
	JobID         string     `json:"job_id"`
	Model         string     `json:"model"`
	PromptVersion string     `json:"prompt_version"`
	Supersedes    string     `json:"supersedes"`
	CreatedAt     int64      `json:"created_at"`
}

type Goal struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	SourceID  string `json:"source_id"`
	Status    string `json:"status"`
	Focus     bool   `json:"focus"`
	CreatedAt int64  `json:"created_at"`
}

// Preparing describes a capture whose material is still being made, or whose
// preparation stopped, in learner terms.
type Preparing struct {
	SourceID  string `json:"source_id"`
	Title     string `json:"title"`
	Stage     string `json:"stage"`
	Label     string `json:"label"`
	Concepts  int    `json:"concepts"`
	Questions int    `json:"questions"`
	Failed    bool   `json:"failed"`
	Error     string `json:"error,omitempty"`
}

type GoalView struct {
	Goal      Goal           `json:"goal"`
	Concepts  []ConceptBrief `json:"concepts"`
	Edges     [][2]int       `json:"edges"`
	Questions int            `json:"questions"`
	Due       int            `json:"due"`
	Preparing *Preparing     `json:"preparing,omitempty"`
}

// QuizFix is the fix state of one question, read from its latest fix request:
// being written, a draft for the current version that pre-fills the edit form
// until the learner saves or edits, or a stop (paused until asked again, or
// failed for the current version).
type QuizFix struct {
	Writing     bool           `json:"writing"`
	Instruction string         `json:"instruction,omitempty"`
	Draft       *GeneratedQuiz `json:"draft,omitempty"`
	Stopped     *FixStop       `json:"stopped,omitempty"`
}

// FixStop is why a fix request stopped and what it cost; it carries nothing
// from the capture or the job beyond that.
type FixStop struct {
	Status      string `json:"status"`
	Error       string `json:"error"`
	CostMicros  int64  `json:"cost_micros"`
	CostUnknown bool   `json:"cost_unknown"`
}

type MapView struct {
	Goals     []GoalView  `json:"goals"`
	Unmapped  []Source    `json:"unmapped"`
	Preparing []Preparing `json:"preparing"`
}

type ConceptView struct {
	Concept          Concept          `json:"concept"`
	State            ConceptState     `json:"state"`
	Goals            []Goal           `json:"goals"`
	Note             *Note            `json:"note"`
	QuestionsPending bool             `json:"questions_pending"`
	Requires         []ConceptBrief   `json:"requires"`
	RequiredBy       []ConceptBrief   `json:"required_by"`
	PartOf           []ConceptBrief   `json:"part_of"`
	Parts            []ConceptBrief   `json:"parts"`
	ConfusedWith     []ConceptBrief   `json:"confused_with"`
	Questions        []Quiz           `json:"questions"`
	Documents        []SourceDocument `json:"documents"`
	Source           *Source          `json:"source,omitempty"`
}

type ConceptIntro struct {
	Concept ConceptRef `json:"concept"`
	Summary string     `json:"summary"`
	Note    *Note      `json:"note"`
}

type Preferences struct {
	Pace string `json:"pace"`
}

// ConceptContext is what a generation job needs to know about one concept.
type ConceptContext struct {
	ID              string
	Name            string
	Summary         string
	Note            *Note
	Requires        []string
	ExistingPrompts []string
}

// JobContext is kind-specific input for one claimed generation job.
type JobContext struct {
	Documents        []SourceDocument
	Image            *CaptureImage
	ExistingConcepts []ConceptBrief
	Goal             *Goal
	Concepts         []ConceptContext
	Quiz             *Quiz
	Instruction      string
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
