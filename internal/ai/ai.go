// Package ai puts the model provider behind a small interface. The model
// does two jobs for Scry: it judges answers, and it drafts new material.
package ai

import (
	"context"

	"scry/internal/srs"
)

// Usage is what one call cost, in the provider's units.
type Usage struct {
	CostUSD      *float64 // exact response usage.cost for reporting; never inferred
	CostMicros   *int64   // known provider cost, or a documented estimate, in micro-dollars
	Model        string
	InputTokens  int64
	OutputTokens int64
}

// JudgeInput is one answer to judge.
type JudgeInput struct {
	AcceptedAnswers []string // optional alternatives; Expected is the single-answer fallback
	Question        string
	Expected        string
	Answer          string
}

// Judgment is the model's decision, always with a one-line reason.
type Judgment struct {
	Verdict srs.Verdict
	Reason  string
	// Close holds the parts of the learner's answer, quoted exactly, that
	// were close but not right. The page marks them with the highlighter.
	Close []string
}

// Learner is what the model is told about who it is drafting for.
type Learner struct {
	About string   // the learner's own words about themself
	Knows []string // titles of ideas already in their library
	Level string   // a sentence on how recent reviews have gone
}

// DraftInput asks for new material on a topic, a question or a book.
type DraftInput struct {
	Query   string
	Learner Learner
}

// DraftIdea is one idea of a draft outline with its items.
type DraftIdea struct {
	Title   string
	Summary string
	Links   []string // titles of related ideas, in this draft or the library
	Items   []DraftItem
}

// DraftItem is a question and its answer.
type DraftItem struct{ Question, Answer string }

// DraftHead names a draft as soon as the model knows what it is.
type DraftHead struct {
	Kind  string // "book" for a book's core ideas, otherwise "topic"
	Title string // the book's title or the topic, tidied
}

// DraftSink receives a draft piece by piece, so ideas can appear on the
// page as they are ready.
type DraftSink struct {
	Head func(DraftHead) error
	Idea func(DraftIdea) error
}

// DraftResult closes a draft.
type DraftResult struct {
	Ideas int
	Note  string // when Ideas is 0, why nothing useful was found; otherwise how it was shaped
}

// Model is the model provider.
type Model interface {
	Name() string
	// Judge decides whether an answer is right, close or missed and says
	// why in one line.
	Judge(ctx context.Context, in JudgeInput) (Judgment, Usage, error)
	// Draft writes an outline of ideas and items, sending each to sink as
	// it is ready.
	Draft(ctx context.Context, in DraftInput, sink DraftSink) (DraftResult, Usage, error)
}
