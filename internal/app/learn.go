package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"scry/internal/ai"
	"scry/internal/speech"
	"scry/internal/store"
)

// Learn asks the model for material on a topic, a question or a book's
// title, shaped to this learner. It returns at once with a draft id; the
// outline is written in the background and its items wait as suggestions.
// Nothing enters the library until the learner approves it.
func (a *App) Learn(ctx context.Context, u store.User, query string) (int64, error) {
	query = strings.Join(strings.Fields(query), " ")
	if query == "" {
		return 0, fieldErr("q", "Type a topic, a question, or a book's title.")
	}
	if len(query) > 300 {
		return 0, fieldErr("q", "Keep it under 300 characters.")
	}
	sub, err := a.Plan(ctx, u.ID)
	if err != nil {
		return 0, err
	}
	us := a.Store.For(u.ID)
	if !CanSpend(sub) {
		return us.CreateDraft(ctx, query, "allowance", a.now())
	}
	id, err := us.CreateDraft(ctx, query, "drafting", a.now())
	if err != nil {
		return 0, err
	}
	a.drafts.Add(1)
	go func() {
		defer a.drafts.Done()
		defer func() {
			if r := recover(); r != nil {
				a.Alert.Notify(context.Background(), "Drafting panicked", fmt.Sprint(r))
				_ = us.UpdateDraft(context.Background(), store.Draft{ID: id, Status: "failed", Note: "Something went wrong while drafting. Ask again."})
			}
		}()
		a.runDraft(context.Background(), u, id, query)
	}()
	return id, nil
}

// WaitDrafts blocks until every draft in flight has finished. Shutdown
// and tests use it.
func (a *App) WaitDrafts() { a.drafts.Wait() }

// Profile is what Scry tells the model about a learner so that new
// material is shaped to them.
func (a *App) Profile(ctx context.Context, u store.User) (ai.Learner, error) {
	us := a.Store.For(u.ID)
	titles, err := us.IdeaTitles(ctx)
	if err != nil {
		return ai.Learner{}, err
	}
	if len(titles) > 400 {
		titles = titles[:400]
	}
	l := ai.Learner{About: u.About, Knows: titles}
	right, total, err := us.RecentVerdicts(ctx, a.now().AddDate(0, 0, -30))
	if err == nil && total >= 10 {
		l.Level = fmt.Sprintf("In the last 30 days they answered %d of %d reviews right.", right, total)
	}
	return l, err
}

func (a *App) runDraft(ctx context.Context, u store.User, id int64, query string) {
	us := a.Store.For(u.ID)
	d := store.Draft{ID: id, Query: query, Kind: "topic", Title: query, Status: "drafting"}
	fail := func(err error) {
		a.Log.ErrorContext(ctx, "draft failed", "err", err, "user", u.ID, "draft", id)
		a.Alert.Notify(ctx, "Drafting is failing", err.Error())
		d.Status, d.Note = "failed", "Scry couldn't reach its model just now. Nothing was charged to your allowance beyond what was written. Ask again in a moment."
		_ = us.UpdateDraft(ctx, d)
	}
	profile, err := a.Profile(ctx, u)
	if err != nil {
		fail(err)
		return
	}
	sink := ai.DraftSink{
		Head: func(h ai.DraftHead) error {
			if h.Title != "" {
				d.Title = h.Title
			}
			if h.Kind == "book" {
				d.Kind = "book"
			}
			return us.UpdateDraft(ctx, d)
		},
		Idea: func(idea ai.DraftIdea) error {
			for _, it := range idea.Items {
				_, err := us.AddSuggestion(ctx, store.Suggestion{
					DraftID: id, Source: d.Title, ParentTitle: d.Title, IdeaTitle: idea.Title, IdeaSummary: idea.Summary,
					Links: idea.Links, Prompt: it.Question, Answer: it.Answer,
				}, a.now())
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	res, use, err := a.Model.Draft(ctx, ai.DraftInput{Query: query, Learner: profile}, sink)
	if use.Model != "" {
		a.meter(ctx, u.ID, "draft", use, 0)
	}
	if err != nil {
		fail(err)
		return
	}
	d.Note = res.Note
	if res.Ideas == 0 {
		d.Status = "empty"
		if d.Note == "" {
			d.Note = "Scry couldn't find anything useful on that. Try a narrower topic, a question, or a book's title."
		}
	} else {
		d.Status = "ready"
	}
	if err := us.UpdateDraft(ctx, d); err != nil {
		a.Log.ErrorContext(ctx, "draft update failed", "err", err, "draft", id)
	}
}

// AgentItem is one piece of material an agent sends.
type AgentItem struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Idea     string `json:"idea"`    // the idea it belongs to; created if new
	Summary  string `json:"summary"` // one line on the idea, used if it is new
	Exact    bool   `json:"exact"`   // judge word for word
}

func (it AgentItem) check() error {
	switch {
	case strings.TrimSpace(it.Question) == "":
		return errors.New("each item needs a question")
	case strings.TrimSpace(it.Answer) == "":
		return errors.New("each item needs an answer")
	case strings.TrimSpace(it.Idea) == "":
		return errors.New("each item needs an idea to belong to")
	case len(it.Question) > 1000 || len(it.Answer) > 2000 || len(it.Idea) > 120 || len(it.Summary) > 300:
		return errors.New("an item field is too long")
	}
	return nil
}

// AgentSuggest queues items from one of the learner's agents in their
// inbox, where they wait for approval.
func (a *App) AgentSuggest(ctx context.Context, t store.AgentToken, source string, items []AgentItem) ([]int64, error) {
	if err := checkAgentItems(source, items); err != nil {
		return nil, err
	}
	us := a.Store.For(t.UserID)
	var ids []int64
	for _, it := range items {
		id, err := us.AddSuggestion(ctx, store.Suggestion{Agent: t.Name, Source: strings.TrimSpace(source), IdeaTitle: it.Idea,
			IdeaSummary: strings.TrimSpace(it.Summary), Prompt: it.Question, Answer: it.Answer, Exact: it.Exact}, a.now())
		if err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ErrAgentMayNotAdd means the token only allows suggestions.
var ErrAgentMayNotAdd = errors.New("this token may suggest but not add")

// AgentAdd puts items from an agent straight into the library. The
// learner must have granted the token that right.
func (a *App) AgentAdd(ctx context.Context, t store.AgentToken, source string, items []AgentItem) ([]int64, error) {
	if !t.CanAdd {
		return nil, ErrAgentMayNotAdd
	}
	ids, err := a.AgentSuggest(ctx, t, source, items)
	if err != nil {
		return nil, err
	}
	us := a.Store.For(t.UserID)
	var added []int64
	for _, id := range ids {
		itemID, err := us.Approve(ctx, id, a.now())
		if err != nil {
			return added, err
		}
		added = append(added, itemID)
	}
	return added, nil
}

func checkAgentItems(source string, items []AgentItem) error {
	if len(items) == 0 {
		return errors.New("send at least one item")
	}
	if len(items) > 50 {
		return errors.New("send at most 50 items at a time")
	}
	if len(source) > 200 {
		return errors.New("source is too long")
	}
	for _, it := range items {
		if err := it.check(); err != nil {
			return err
		}
	}
	return nil
}

// ErrSpeechUnavailable means the speech provider could not be reached.
var ErrSpeechUnavailable = errors.New("speech is unavailable")

// Transcribe hears a spoken answer. Empty text means nothing was caught.
func (a *App) Transcribe(ctx context.Context, u store.User, audio speech.Audio) (string, error) {
	sub, err := a.Plan(ctx, u.ID)
	if err != nil {
		return "", err
	}
	if !CanSpend(sub) {
		return "", ErrAllowance
	}
	if audio.Seconds <= 0 || audio.Seconds > 120 {
		audio.Seconds = min(max(float64(len(audio.Data))/4000, 0.5), 120)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	text, err := a.Speech.Transcribe(ctx, audio)
	if err != nil {
		a.Log.ErrorContext(ctx, "speech failed", "err", err, "user", u.ID)
		a.Alert.Notify(ctx, "Spoken answers cannot be heard", err.Error())
		return "", ErrSpeechUnavailable
	}
	a.meter(ctx, u.ID, "speech", ai.Usage{Model: a.Speech.Name()}, audio.Seconds)
	return strings.Join(strings.Fields(text), " "), nil
}
