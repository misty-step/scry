package learning

import (
	"math"
	"sort"
	"strings"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"
)

// ConceptStatePolicy identifies how per-question evidence becomes a concept's
// displayed state. It is a presentation estimate, never a mastery claim, and it
// never changes a schedule.
const ConceptStatePolicy = "concept-state-v1"

// Attempt is one terminal learner observation on a question, oldest first.
// Rating is the effective rating after any learner override: 3 Good, 1 Again,
// 0 for helped work that carried no FSRS success (warm or assisted).
type Attempt struct {
	At       int64
	Rating   int
	Assisted bool
	Outcome  string
}

// QuestionEvidence is one question's current FSRS card and its attempts.
type QuestionEvidence struct {
	Card     Card
	Level    string
	Attempts []Attempt
}

// ConceptState is what Scry can honestly say about one concept right now.
// Recall is the level-weighted FSRS retrievability of questions that have been
// reviewed, or -1 when there is no reviewed question. Tally holds the latest
// attempts: "u" unaided success, "h" helped or revealed, "m" miss.
type ConceptState struct {
	Status     string   `json:"status"`
	Recall     float64  `json:"recall"`
	Brightness int      `json:"brightness"`
	Unaided    int      `json:"unaided"`
	Helped     int      `json:"helped"`
	Missed     int      `json:"missed"`
	LastAt     int64    `json:"last_at"`
	NextDueAt  int64    `json:"next_due_at"`
	Tally      []string `json:"tally"`
}

const tallyLength = 12

// Retrievability is the pinned scheduler's probability of recall for a card at
// now, or -1 for a card that has never been reviewed.
func Retrievability(card Card, now time.Time) float64 {
	if schedulerError != nil || card.State == fsrs.New || card.LastReview.IsZero() {
		return -1
	}
	value, err := scheduler.Retrievability(card, now.UTC())
	if err != nil || math.IsNaN(value) || value < 0 || value > 1 {
		return -1
	}
	return value
}

// IsNew reports whether a card has never been reviewed.
func IsNew(card Card) bool { return card.State == fsrs.New }

// IsLearning reports whether a card is in short learning or relearning steps.
func IsLearning(card Card) bool { return card.State == fsrs.Learning || card.State == fsrs.Relearning }

// LevelRank orders question levels from recognition upward. Unknown or empty
// levels (legacy questions) rank as recall.
func LevelRank(level string) int {
	switch level {
	case "recognize":
		return 0
	case "recall", "":
		return 1
	case "explain":
		return 2
	case "apply":
		return 3
	default:
		return 1
	}
}

func levelWeight(level string) float64 {
	switch level {
	case "recognize":
		return 1
	case "explain", "apply":
		return 3
	default:
		return 2
	}
}

// ComputeConceptState summarizes evidence without side effects.
func ComputeConceptState(questions []QuestionEvidence, now time.Time) ConceptState {
	state := ConceptState{Status: "new", Recall: -1, Tally: []string{}}
	var attempts []Attempt
	var weighted, weights, maxStability float64
	reviewed := false
	for _, question := range questions {
		attempts = append(attempts, question.Attempts...)
		if r := Retrievability(question.Card, now); r >= 0 {
			w := levelWeight(question.Level)
			weighted += r * w
			weights += w
			if question.Card.Stability > maxStability {
				maxStability = question.Card.Stability
			}
			if question.Card.State == fsrs.Review {
				reviewed = true
			}
			due := question.Card.Due.UnixMilli()
			if state.NextDueAt == 0 || due < state.NextDueAt {
				state.NextDueAt = due
			}
		}
	}
	sort.SliceStable(attempts, func(i, j int) bool { return attempts[i].At < attempts[j].At })
	var codes []string
	for _, attempt := range attempts {
		code := ""
		switch {
		case attempt.Rating == 3 && !attempt.Assisted:
			code = "u"
			state.Unaided++
		case attempt.Assisted || attempt.Outcome == "revealed" || strings.HasPrefix(attempt.Outcome, "warm_"):
			code = "h"
			state.Helped++
		case attempt.Rating == 1:
			code = "m"
			state.Missed++
		}
		if code == "" {
			continue
		}
		codes = append(codes, code)
		state.LastAt = attempt.At
	}
	if len(codes) > tallyLength {
		codes = codes[len(codes)-tallyLength:]
	}
	state.Tally = append(state.Tally, codes...)
	if weights > 0 {
		state.Recall = weighted / weights
	}
	if len(codes) == 0 {
		state.Brightness = 0
		return state
	}
	lastUnaided := codes[len(codes)-1] == "u"
	switch {
	case state.Recall >= 0.90 && maxStability >= 21 && lastUnaided:
		state.Status = "solid"
	case reviewed && state.Recall >= 0 && state.Recall < 0.80:
		state.Status = "fading"
	default:
		state.Status = "learning"
	}
	state.Brightness = 1
	if state.Recall >= 0 {
		state.Brightness = int(math.Round(state.Recall * 5))
		if state.Brightness < 1 {
			state.Brightness = 1
		}
		if state.Brightness > 5 {
			state.Brightness = 5
		}
	}
	switch state.Status {
	case "fading":
		state.Brightness = min(state.Brightness, 2)
	case "solid":
		state.Brightness = max(state.Brightness, 4)
	case "learning":
		state.Brightness = min(state.Brightness, 4)
	}
	return state
}
