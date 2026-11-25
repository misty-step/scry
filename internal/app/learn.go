package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"scry/internal/ai"
	"scry/internal/speech"
	"scry/internal/store"
)

// MaxLearnChars is how much a learner may type or paste into Create
// (S106-2): a topic, a question, a book's title, or Markdown, prose or
// notes up to 16,000 characters.
const MaxLearnChars = 16000

// maxDraftTitle bounds a draft's title, so pasted material is named by
// its first heading or line, never by the whole text.
const maxDraftTitle = 80

// NormalizeLearn tidies what was typed or pasted into Create: line
// endings become \n and the ends are trimmed. A one-line request has its
// runs of spaces collapsed; pasted material keeps its line breaks.
func NormalizeLearn(query string) string {
	query = strings.ReplaceAll(query, "\r\n", "\n")
	query = strings.TrimSpace(strings.ReplaceAll(query, "\r", "\n"))
	if !strings.Contains(query, "\n") {
		query = strings.Join(strings.Fields(query), " ")
	}
	return query
}

// DraftTitle names a draft before the model does: a short request names
// itself; pasted material (several lines, or longer than a topic) is
// named by its first Markdown heading, or else its first line, cut to at
// most 80 characters at a word.
func DraftTitle(query string) string {
	if !ai.IsMaterial(query) {
		return query
	}
	first := ""
	for _, line := range strings.Split(query, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			first = line
			break
		}
		if first == "" && line != "" {
			first = line
		}
	}
	return ai.Clip(ai.PlainLine(first), maxDraftTitle)
}

// Learn asks the model for material on a topic, a question or a book's
// title, or for cards from Markdown, prose or notes the learner pasted,
// shaped to this learner. It returns at once with a draft id; the outline
// is written in the background and its items wait as suggestions.
// Nothing enters the library until the learner approves it.
func (a *App) Learn(ctx context.Context, u store.User, query string) (int64, error) {
	query = NormalizeLearn(query)
	if query == "" {
		return 0, fieldErr("q", "Type a topic, a question or a book's title, or paste your notes.")
	}
	if utf8.RuneCountInString(query) > MaxLearnChars {
		return 0, fieldErr("q", "Keep it under 16,000 characters.")
	}
	if !ai.DraftFits(query) {
		// Under 16,000 characters but too many bytes for one draft's
		// budget (a script of several bytes a character).
		return 0, fieldErr("q", "That's too long to draft at once. Send it in parts.")
	}
	_, err := a.Plan(ctx, u.ID)
	if err != nil {
		return 0, err
	}
	us := a.Store.For(u.ID)
	reservation, err := a.reserve(ctx, u.ID, a.DraftBudget())
	if errors.Is(err, ErrAllowance) {
		return us.CreateDraft(ctx, query, "allowance", a.now())
	}
	if err != nil {
		return 0, err
	}
	id, err := us.CreateDraft(ctx, query, "drafting", a.now())
	if err != nil {
		a.settle(u.ID, reservation, "draft", ai.Usage{}, 0)
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
		a.runDraft(context.Background(), u, id, query, reservation)
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

func (a *App) runDraft(ctx context.Context, u store.User, id int64, query string, reservation string) {
	us := a.Store.For(u.ID)
	// The reservation is decided exactly once on every path, including a
	// panic (the deferred decision runs as the panic unwinds, before
	// Learn's recover marks the draft failed):
	//   - before the model is called (profile failure, panic): released;
	//   - the model reported usage: settled at that cost;
	//   - the model was called and its outcome is unknown (an error or a
	//     panic with no usage): held, never forgiven, for an operator to
	//     reconcile against the provider (G2, see NOTES.md).
	decided, called := false, false
	settle := func(use ai.Usage) {
		if decided {
			return
		}
		decided = true
		a.settle(u.ID, reservation, "draft", use, 0)
	}
	hold := func(why string) {
		if decided {
			return
		}
		decided = true
		a.Log.WarnContext(ctx, "draft provider outcome unknown; reservation held", "why", why, "user", u.ID, "draft", id, "reservation", reservation)
	}
	defer func() {
		if called {
			hold("interrupted during the model call")
		} else {
			settle(ai.Usage{})
		}
	}()
	d := store.Draft{ID: id, Query: query, Kind: "topic", Title: DraftTitle(query), Status: "drafting"}
	fail := func(err error) {
		a.Log.ErrorContext(ctx, "draft failed", "err", err, "user", u.ID, "draft", id)
		a.Alert.Notify(ctx, "Drafting is failing", err.Error())
		d.Status, d.Note = "failed", "Scry couldn't reach its model just now. Nothing was charged to your allowance beyond what was written. Ask again in a moment."
		_ = us.UpdateDraft(ctx, d)
	}
	profile, err := a.Profile(ctx, u)
	if err != nil {
		settle(ai.Usage{})
		fail(err)
		return
	}
	sink := ai.DraftSink{
		Head: func(h ai.DraftHead) error {
			if t := ai.Clip(ai.PlainLine(h.Title), maxDraftTitle); t != "" {
				d.Title = t
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
	called = true
	res, use, err := a.Model.Draft(ctx, ai.DraftInput{Query: query, Learner: profile}, sink)
	if use.Model != "" {
		settle(use)
	} else {
		hold("model error with no usage")
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

// AgentItem is one card a script or tool sends through the API.
type AgentItem struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Idea     string `json:"idea"`    // the idea it belongs to; created if new
	Summary  string `json:"summary"` // one line on the idea, used if it is new
	Exact    bool   `json:"exact"`   // judge word for word
}

// MaxAgentItems is how many cards one API request may carry (S106-4).
const MaxAgentItems = 50

func (it AgentItem) check(n int) error {
	switch {
	case strings.TrimSpace(it.Question) == "":
		return fmt.Errorf("item %d needs a question", n)
	case strings.TrimSpace(it.Answer) == "":
		return fmt.Errorf("item %d needs an answer", n)
	case strings.TrimSpace(it.Idea) == "":
		return fmt.Errorf("item %d needs an idea to belong to", n)
	case len(it.Question) > 1000:
		return fmt.Errorf("item %d's question is too long; keep it within 1,000 bytes", n)
	case len(it.Answer) > 2000:
		return fmt.Errorf("item %d's answer is too long; keep it within 2,000 bytes", n)
	case len(it.Idea) > 120:
		return fmt.Errorf("item %d's idea is too long; keep it within 120 bytes", n)
	case len(it.Summary) > 300:
		return fmt.Errorf("item %d's summary is too long; keep it within 300 bytes", n)
	}
	return nil
}

// AgentAdd puts cards from one of the learner's scripts or tools straight
// into their library (S106-4: there is no Inbox). Every live token may
// add, including one made before tokens could, and POST /api/v1/items and
// its alias POST /api/v1/suggestions both land here. The request is all or
// nothing: a refused item adds none. It returns the new items' ids.
func (a *App) AgentAdd(ctx context.Context, t store.AgentToken, source string, items []AgentItem) ([]int64, error) {
	if err := checkAgentItems(source, items); err != nil {
		return nil, err
	}
	sugs := make([]store.Suggestion, 0, len(items))
	for _, it := range items {
		sugs = append(sugs, store.Suggestion{Agent: t.Name, Source: strings.TrimSpace(source), IdeaTitle: it.Idea,
			IdeaSummary: strings.TrimSpace(it.Summary), Prompt: it.Question, Answer: it.Answer, Exact: it.Exact})
	}
	return a.Store.For(t.UserID).AddFromAgent(ctx, sugs, a.now())
}

func checkAgentItems(source string, items []AgentItem) error {
	if len(items) == 0 {
		return errors.New("send at least one item: 1 to 50 per request")
	}
	if len(items) > MaxAgentItems {
		return fmt.Errorf("send at most 50 items per request; this one had %d", len(items))
	}
	if len(source) > 200 {
		return errors.New("source is too long; keep it within 200 bytes")
	}
	for i, it := range items {
		if err := it.check(i + 1); err != nil {
			return err
		}
	}
	return nil
}

// ErrSpeechUnavailable means the speech provider could not be reached.
var ErrSpeechUnavailable = errors.New("speech is unavailable")

// Transcribe hears a spoken answer. Empty text means nothing was caught.
func (a *App) Transcribe(ctx context.Context, u store.User, audio speech.Audio) (string, error) {
	// Test text fixtures are explicit and never accepted by live adapters.
	if a.Cfg.TestMode && a.Speech.Name() == "test" && strings.HasPrefix(string(audio.Data), "say:") {
		audio.Seconds = 1
	} else {
		// A recording with nothing in it (stopped at once, too short for a
		// word, or silent) is "didn't catch that": no provider call, no cost.
		seconds, err := speech.Inspect(audio.Data)
		if errors.Is(err, speech.ErrNoSpeech) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		audio.Seconds = seconds
		audio.MIME = "audio/wav"
	}
	_, err := a.Plan(ctx, u.ID)
	if err != nil {
		return "", err
	}
	reservation, err := a.reserve(ctx, u.ID, SpeechBudgetMicros(audio.Seconds))
	if err != nil {
		return "", err
	}
	// Speech is priced by the recording's length, so its cost is known
	// before the provider answers. The reservation is always settled at
	// that cost, also when the provider fails or the call panics: the
	// outcome is uncertain, so the cost is never forgiven, and the
	// reservation is never left held.
	use := ai.Usage{Model: speech.ModelOf(a.Speech)}
	defer func() { a.settle(u.ID, reservation, "speech", use, audio.Seconds) }()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var text string
	if m, ok := a.Speech.(speech.Metered); ok {
		text, use.CostMicros, err = m.TranscribeMetered(ctx, audio)
	} else {
		text, err = a.Speech.Transcribe(ctx, audio)
	}
	if err != nil {
		a.Log.ErrorContext(ctx, "speech failed", "err", err, "user", u.ID)
		a.Alert.Notify(ctx, "Spoken answers cannot be heard", err.Error())
		return "", ErrSpeechUnavailable
	}
	return strings.Join(strings.Fields(text), " "), nil
}
