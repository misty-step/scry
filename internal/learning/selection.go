package learning

import "sort"

// SelectionPolicy identifies how the next question is chosen above per-question
// FSRS. It only orders eligible questions; it never writes a schedule.
const SelectionPolicy = "select-v1"

// NewConceptBudget is how many not-yet-introduced concepts may start within a
// rolling 24 hours at each pace.
func NewConceptBudget(pace string) int {
	switch pace {
	case "light":
		return 3
	case "intense":
		return 12
	default:
		return 6
	}
}

// Candidate is one active question the store can present.
type Candidate struct {
	QuizID         string
	ConceptID      string // "" for legacy unmapped questions
	GoalID         string
	GoalFocus      bool
	GoalPaused     bool
	GoalCreated    int64
	Level          string
	New            bool // never reviewed
	Learning       bool // short learning/relearning steps
	AvailableAt    int64
	Retrievability float64
	CreatedAt      int64
	Order          int64 // stable tie-break (row order)
	RecentlySeen   bool  // presented within the last few minutes
}

// SelectionInput carries everything the policy may consider. Maps are keyed by
// concept ID unless noted. Nil maps behave as empty.
type SelectionInput struct {
	Now           int64
	Candidates    []Candidate
	LastConcept   string          // concept of the most recent presentation
	LastIntro     string          // concept whose intro was acknowledged after the last presentation
	Ready         map[string]bool // prerequisites satisfied
	Introduced    map[string]bool // intro acknowledged or already presented
	LevelUnlocked map[string]bool // keyed by quiz ID: lowest remaining new level of its concept
	Deferred      map[string]bool // implied prerequisite credit: defer due reviews
	Remedial      map[string]bool // keyed by quiz ID: prerequisite check after a recent miss
	Focus         string          // concept chosen with "Practice this"
	NewBudgetLeft int             // remaining new concepts in the rolling window
	UnmetPrereqs  map[string]int  // count of unmet prerequisites, for the dead-end fallback
}

// Selection names the chosen question and why.
type Selection struct {
	QuizID string
	Reason string // focus | due | remedial | new | fallback | none
}

// Select returns the next question. Order: an explicit practice focus, due
// reviews (short steps first, deferred-by-implied-credit last, most at risk
// first), prerequisite checks after a recent miss, then new material from ready
// concepts at the learner's pace. Consecutive questions avoid the same concept
// when another choice exists.
func Select(in SelectionInput) Selection {
	if in.Focus != "" {
		var focus []Candidate
		for _, c := range in.Candidates {
			if c.ConceptID == in.Focus && !c.RecentlySeen {
				focus = append(focus, c)
			}
		}
		sortFocus(focus)
		if len(focus) > 0 {
			return Selection{QuizID: focus[0].QuizID, Reason: "focus"}
		}
	}
	var due []Candidate
	for _, c := range in.Candidates {
		if !c.New && c.AvailableAt <= in.Now {
			due = append(due, c)
		}
	}
	sort.SliceStable(due, func(i, j int) bool {
		a, b := due[i], due[j]
		if a.Learning != b.Learning {
			return a.Learning
		}
		if da, db := in.Deferred[a.ConceptID] && !a.Learning, in.Deferred[b.ConceptID] && !b.Learning; da != db {
			return !da
		}
		if a.Retrievability != b.Retrievability {
			return a.Retrievability < b.Retrievability
		}
		if a.AvailableAt != b.AvailableAt {
			return a.AvailableAt < b.AvailableAt
		}
		return a.Order < b.Order
	})
	if pick, ok := interleave(due, in.LastConcept); ok {
		return Selection{QuizID: pick.QuizID, Reason: "due"}
	}
	var remedial []Candidate
	for _, c := range in.Candidates {
		if in.Remedial[c.QuizID] && !c.RecentlySeen {
			remedial = append(remedial, c)
		}
	}
	sort.SliceStable(remedial, func(i, j int) bool {
		if remedial[i].Retrievability != remedial[j].Retrievability {
			return remedial[i].Retrievability < remedial[j].Retrievability
		}
		return remedial[i].Order < remedial[j].Order
	})
	if len(remedial) > 0 {
		return Selection{QuizID: remedial[0].QuizID, Reason: "remedial"}
	}
	var fresh, blocked []Candidate
	for _, c := range in.Candidates {
		if !c.New || c.AvailableAt > in.Now || c.GoalPaused {
			continue
		}
		if c.ConceptID != "" && !in.LevelUnlocked[c.QuizID] {
			continue
		}
		if c.ConceptID != "" && !in.Introduced[c.ConceptID] && in.NewBudgetLeft <= 0 {
			continue
		}
		if c.ConceptID != "" && !in.Ready[c.ConceptID] {
			blocked = append(blocked, c)
			continue
		}
		fresh = append(fresh, c)
	}
	sortNew(fresh)
	// Learn, then retrieve: a concept whose intro was just acknowledged waits
	// for one other item when another is available.
	if in.LastIntro != "" && len(fresh) > 1 {
		for index, c := range fresh {
			if c.ConceptID != in.LastIntro {
				if index > 0 {
					return Selection{QuizID: c.QuizID, Reason: "new"}
				}
				break
			}
		}
	}
	if pick, ok := interleave(fresh, in.LastConcept); ok {
		return Selection{QuizID: pick.QuizID, Reason: "new"}
	}
	if len(blocked) > 0 {
		// Never dead-end: a prerequisite graph can be wrong or cyclic. Offer the
		// blocked question with the fewest unmet prerequisites.
		sortNew(blocked)
		sort.SliceStable(blocked, func(i, j int) bool {
			return in.UnmetPrereqs[blocked[i].ConceptID] < in.UnmetPrereqs[blocked[j].ConceptID]
		})
		return Selection{QuizID: blocked[0].QuizID, Reason: "fallback"}
	}
	return Selection{Reason: "none"}
}

func sortFocus(items []Candidate) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.New != b.New {
			return !a.New
		}
		if !a.New && a.Retrievability != b.Retrievability {
			return a.Retrievability < b.Retrievability
		}
		if ra, rb := LevelRank(a.Level), LevelRank(b.Level); ra != rb {
			return ra < rb
		}
		return a.Order < b.Order
	})
}

func sortNew(items []Candidate) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.GoalFocus != b.GoalFocus {
			return a.GoalFocus
		}
		if a.GoalCreated != b.GoalCreated {
			return a.GoalCreated > b.GoalCreated
		}
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt < b.CreatedAt
		}
		if ra, rb := LevelRank(a.Level), LevelRank(b.Level); ra != rb {
			return ra < rb
		}
		return a.Order < b.Order
	})
}

// interleave returns the first candidate from a different concept than the
// last one shown, or the first candidate when no alternative exists.
func interleave(items []Candidate, last string) (Candidate, bool) {
	if len(items) == 0 {
		return Candidate{}, false
	}
	if last != "" {
		for _, c := range items {
			if c.ConceptID == "" || c.ConceptID != last {
				return c, true
			}
		}
	}
	return items[0], true
}
