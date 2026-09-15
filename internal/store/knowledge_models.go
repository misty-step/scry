package store

import (
	"time"

	"github.com/misty-step/scry/internal/learning"
)

const (
	MaxGeneratedUnits       = 96
	MaxGeneratedRelations   = 192
	MaxGeneratedMaterials   = 24
	MaxGeneratedSuggestions = 8
	MaxMaterialLinks        = 32
	KnowledgeContextVersion = "knowledge-context-v1"
)

type CoverageReport struct {
	Kind     string   `json:"kind"`
	Complete bool     `json:"complete"`
	Missing  []string `json:"missing"`
}

type PlanSettings struct {
	ExpectedRevision     int    `json:"expected_revision"`
	TimeBudgetSeconds    int    `json:"time_budget_seconds"`
	NewAssessmentsPerDay int    `json:"new_assessments_per_day"`
	Focus                string `json:"focus"`
	Reason               string `json:"reason"`
}

type Goal struct {
	ID                   string              `json:"id"`
	SourceID             string              `json:"source_id"`
	Title                string              `json:"title"`
	Revision             int                 `json:"revision"`
	SourceRevision       int                 `json:"source_revision"`
	Archived             bool                `json:"archived"`
	MetadataOnly         bool                `json:"metadata_only"`
	UnitCount            int                 `json:"unit_count"`
	MaterialCount        int                 `json:"material_count"`
	MissingCoverageCount int                 `json:"missing_coverage_count"`
	Coverage             CoverageReport      `json:"coverage"`
	Settings             PlanSettings        `json:"settings"`
	Units                []KnowledgeUnit     `json:"units"`
	Estimates            []learning.Estimate `json:"estimates"`
	Materials            []Material          `json:"materials"`
	Suggestions          []Suggestion        `json:"suggestions"`
	Decisions            []PlanDecision      `json:"decisions"`
	Jobs                 []Job               `json:"jobs"`
	CreatedAt            int64               `json:"created_at"`
	UpdatedAt            int64               `json:"updated_at"`
}

type Suggestion struct {
	ID               string                 `json:"id"`
	GoalID           string                 `json:"goal_id"`
	GoalRevision     int                    `json:"goal_revision"`
	Kind             string                 `json:"kind"`
	Title            string                 `json:"title"`
	Reason           string                 `json:"reason"`
	UnitIDs          []string               `json:"unit_ids"`
	MaterialIDs      []string               `json:"material_ids"`
	UnitVersions     []learning.UnitVersion `json:"unit_versions"`
	MaterialVersions []learning.UnitVersion `json:"material_versions"`
	Status           string                 `json:"status"`
	OriginJobID      string                 `json:"origin_job_id"`
	CreatedAt        int64                  `json:"created_at"`
	SupersedesID     string                 `json:"supersedes_id"`
}

type PlanDecision struct {
	ID              string                 `json:"id"`
	GoalID          string                 `json:"goal_id"`
	GoalRevision    int                    `json:"goal_revision"`
	Kind            string                 `json:"kind"`
	Reason          string                 `json:"reason"`
	Policy          string                 `json:"policy"`
	MaterialID      string                 `json:"material_id"`
	MaterialVersion int                    `json:"material_version"`
	SuggestionID    string                 `json:"suggestion_id"`
	Before          PlanSettings           `json:"before"`
	After           PlanSettings           `json:"after"`
	EvidenceIDs     []string               `json:"evidence_ids"`
	UnitVersions    []learning.UnitVersion `json:"unit_versions"`
	ReconsiderAt    int64                  `json:"reconsider_at"`
	CreatedAt       int64                  `json:"created_at"`
	UndoneAt        int64                  `json:"undone_at"`
	UndoOf          string                 `json:"undo_of"`
}

type Provenance struct {
	SourceID       string `json:"source_id"`
	SourceRevision int    `json:"source_revision"`
	JobID          string `json:"job_id"`
	Model          string `json:"model"`
	PromptVersion  string `json:"prompt_version"`
	Basis          string `json:"basis"`
	Evidence       string `json:"evidence"`
	Reason         string `json:"reason"`
	CreatedAt      int64  `json:"created_at"`
}

type KnowledgeUnit struct {
	ID         string     `json:"id"`
	Version    int        `json:"version"`
	Statement  string     `json:"statement"`
	Kind       string     `json:"kind"`
	Archived   bool       `json:"archived"`
	Provenance Provenance `json:"provenance"`
	CreatedAt  int64      `json:"created_at"`
}

type CoverageLink struct {
	ID              string     `json:"id"`
	MaterialID      string     `json:"material_id"`
	MaterialVersion int        `json:"material_version"`
	UnitID          string     `json:"unit_id"`
	UnitVersion     int        `json:"unit_version"`
	Role            string     `json:"role"`
	Provenance      Provenance `json:"provenance"`
}

type KnowledgeRelation struct {
	ID          string     `json:"id"`
	Version     int        `json:"version"`
	FromID      string     `json:"from_id"`
	FromVersion int        `json:"from_version"`
	ToID        string     `json:"to_id"`
	ToVersion   int        `json:"to_version"`
	Kind        string     `json:"kind"`
	Proposed    bool       `json:"proposed"`
	Archived    bool       `json:"archived"`
	Provenance  Provenance `json:"provenance"`
}

type RelationChange struct {
	ID              string `json:"id"`
	ExpectedVersion int    `json:"expected_version"`
	FromID          string `json:"from_id"`
	FromVersion     int    `json:"from_version"`
	ToID            string `json:"to_id"`
	ToVersion       int    `json:"to_version"`
	Kind            string `json:"kind"`
	Proposed        bool   `json:"proposed"`
	Archived        bool   `json:"archived"`
	Reason          string `json:"reason"`
	Evidence        string `json:"evidence"`
}

type Material struct {
	ID               string         `json:"id"`
	GoalID           string         `json:"goal_id,omitempty"`
	GoalRevision     int            `json:"goal_revision,omitempty"`
	SourceID         string         `json:"source_id"`
	Version          int            `json:"version"`
	Kind             string         `json:"kind"`
	Level            string         `json:"level"`
	Title            string         `json:"title"`
	Body             string         `json:"body"`
	Basis            string         `json:"basis"`
	Evidence         string         `json:"evidence"`
	ReferenceURL     string         `json:"reference_url"`
	StartSeconds     int            `json:"start_seconds"`
	EndSeconds       int            `json:"end_seconds"`
	EstimatedSeconds int            `json:"estimated_seconds"`
	Diagram          *Diagram       `json:"diagram,omitempty"`
	Quiz             *Quiz          `json:"quiz,omitempty"`
	Links            []CoverageLink `json:"links"`
	Provenance       Provenance     `json:"provenance"`
	Archived         bool           `json:"archived"`
	MetadataOnly     bool           `json:"metadata_only"`
	Unmapped         bool           `json:"unmapped"`
	DueAt            int64          `json:"due_at"`
	FirstPresentedAt int64          `json:"first_presented_at"`
	PlanningReason   string         `json:"planning_reason"`
	SelectionPolicy  string         `json:"selection_policy"`
	CreatedAt        int64          `json:"created_at"`
}

type KnowledgeCorrection struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	EntityID      string `json:"entity_id"`
	BeforeVersion int    `json:"before_version"`
	AfterVersion  int    `json:"after_version"`
	Reason        string `json:"reason"`
	CreatedAt     int64  `json:"created_at"`
}

type KnowledgeUnitDetail struct {
	Unit        KnowledgeUnit         `json:"unit"`
	Versions    []KnowledgeUnit       `json:"versions"`
	Relations   []KnowledgeRelation   `json:"relations"`
	Materials   []Material            `json:"materials"`
	Estimate    learning.Estimate     `json:"estimate"`
	Corrections []KnowledgeCorrection `json:"corrections"`
}

type Bridge struct {
	ID                    string   `json:"id"`
	GoalID                string   `json:"goal_id"`
	TargetPresentationID  string   `json:"target_presentation_id"`
	TargetMaterialID      string   `json:"target_material_id"`
	TargetMaterialVersion int      `json:"target_material_version"`
	Status                string   `json:"status"`
	Reason                string   `json:"reason"`
	Path                  []string `json:"path"`
	PathVersions          []int    `json:"path_versions"`
	Position              int      `json:"position"`
	JobID                 string   `json:"job_id"`
	Job                   *Job     `json:"job,omitempty"`
	CreatedAt             int64    `json:"created_at"`
	UpdatedAt             int64    `json:"updated_at"`
}

type Interaction struct {
	ID               string                 `json:"id"`
	GoalID           string                 `json:"goal_id"`
	GoalRevision     int                    `json:"goal_revision"`
	GoalPinOrigin    string                 `json:"goal_pin_origin"`
	ReviewID         string                 `json:"review_id"`
	Disputed         bool                   `json:"disputed"`
	EntityKind       string                 `json:"entity_kind"`
	EntityID         string                 `json:"entity_id"`
	Policy           string                 `json:"policy"`
	UnitVersions     []learning.UnitVersion `json:"unit_versions"`
	MaterialVersions []learning.UnitVersion `json:"material_versions"`
	PresentationID   string                 `json:"presentation_id"`
	MaterialID       string                 `json:"material_id"`
	Answer           string                 `json:"answer"`
	Mode             string                 `json:"mode"`
	Practice         bool                   `json:"practice"`
	MaterialVersion  int                    `json:"material_version"`
	Kind             string                 `json:"kind"`
	Outcome          string                 `json:"outcome"`
	Assisted         bool                   `json:"assisted"`
	At               int64                  `json:"at"`
	Snapshot         Material               `json:"snapshot"`
	Reason           string                 `json:"reason"`
}

type KnowledgeContext struct {
	Version      string              `json:"version"`
	GoalID       string              `json:"goal_id"`
	GoalRevision int                 `json:"goal_revision"`
	Suggestion   *Suggestion         `json:"suggestion,omitempty"`
	GoalTitle    string              `json:"goal_title"`
	Settings     PlanSettings        `json:"settings"`
	Request      string              `json:"request"`
	Units        []KnowledgeUnit     `json:"units"`
	Relations    []KnowledgeRelation `json:"relations"`
	Materials    []Material          `json:"materials"`
	References   []string            `json:"references"`
	Evidence     []learning.Evidence `json:"evidence"`
	Omitted      ContextOmissions    `json:"omitted"`
	AsOf         time.Time           `json:"as_of"`
}

type ContextOmissions struct {
	Units      int `json:"units"`
	Relations  int `json:"relations"`
	Materials  int `json:"materials"`
	References int `json:"references"`
	Evidence   int `json:"evidence"`
}

type GeneratedUnit struct {
	Key       string `json:"key"`
	ReuseID   string `json:"reuse_id"`
	Statement string `json:"statement"`
	Kind      string `json:"kind"`
}

type GeneratedRelation struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Kind     string `json:"kind"`
	Evidence string `json:"evidence"`
}

type GeneratedLink struct {
	UnitKey string `json:"unit_key"`
	Role    string `json:"role"`
}

type GeneratedMaterial struct {
	Key              string          `json:"key"`
	ReuseID          string          `json:"reuse_id"`
	Kind             string          `json:"kind"`
	Title            string          `json:"title"`
	Body             string          `json:"body"`
	Basis            string          `json:"basis"`
	Evidence         string          `json:"evidence"`
	ReferenceURL     string          `json:"reference_url"`
	StartSeconds     int             `json:"start_seconds"`
	EndSeconds       int             `json:"end_seconds"`
	EstimatedSeconds int             `json:"estimated_seconds"`
	Diagram          *Diagram        `json:"diagram"`
	Links            []GeneratedLink `json:"links"`
}

type Diagram struct {
	Nodes   []DiagramNode `json:"nodes"`
	Edges   []DiagramEdge `json:"edges"`
	Caption string        `json:"caption"`
}

type DiagramNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type DiagramEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
}

type GeneratedSuggestion struct {
	Key          string   `json:"key"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Reason       string   `json:"reason"`
	UnitKeys     []string `json:"unit_keys"`
	MaterialKeys []string `json:"material_keys"`
}

type InspectionAccess struct {
	Allowed            bool  `json:"allowed"`
	RequiresAssistance bool  `json:"requires_assistance"`
	ExpiresAt          int64 `json:"expires_at"`
	CheckedAt          int64 `json:"checked_at"`
}
