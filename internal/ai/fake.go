package ai

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"

	"scry/internal/srs"
	"scry/internal/starter"
)

// Fake is the test-mode model. It needs no keys and is deterministic.
//
// It returns a scripted fixture, never a keyword or fuzzy judgment.
// Only the real model adapter evaluates meaning; this adapter exercises UI
// and scheduling without making any claim about answer correctness.
type Fake struct {
	Judgment *Judgment // optional scripted result; default is a labelled right fixture
	// Delay is the pause before each drafted idea, so the "drafting" state
	// can be seen in the demo. Tests leave it zero.
	Delay time.Duration
	// Err, when set, is returned by every call. Tests use it to see what
	// the app does when the provider is down.
	Err error
	// Verdicts, when set, is a scripted sequence of verdicts the fake hands
	// out in turn, whatever the answer says (SCRY_TEST_JUDGMENTS in test
	// mode), each with that verdict's fixed criterion sentence. It lets the
	// running app show close and missed verdicts without any matcher.
	Verdicts []srs.Verdict
	// Paths, when set, is a scripted sequence of grading-chain paths
	// (SCRY_TEST_JUDGE_PATHS, see FakeJudgePaths), one per submission in
	// turn, whatever the answer says. Every submission runs through the
	// real Chain orchestrator with fixture steps; the default path is
	// "primary".
	Paths []string
	// JudgeDelay is how long each judgment waits before the chain starts
	// (SCRY_TEST_JUDGE_DELAY), so "checking" and "still checking" can be
	// seen in a browser. Tests leave it zero.
	JudgeDelay time.Duration
	// Log receives the chain's attempt logs; nil is slog.Default().
	Log      *slog.Logger
	mu       sync.Mutex
	next     int
	nextPath int
	calls    map[string]int
}

func (f *Fake) Name() string { return "test" }

// The fixture models of the test chain: a primary, two fallbacks and the
// escalation LLM.
const (
	FakeJudgeModel      = "test-judge"
	FakeFallbackModel1  = "test-judge-fallback-1"
	FakeFallbackModel2  = "test-judge-fallback-2"
	FakeEscalationModel = "test-judge-escalation"
)

// FakeJudgePaths are the chain paths the test adapter can be forced
// through ([S113-1]):
//
//	primary                  the primary decisions model answers
//	primary-unavailable      the primary cannot be reached; the first fallback answers
//	primary-error            the primary errors; the first fallback answers
//	primary-unparseable      the primary's reply cannot be read; the first fallback answers
//	chain-unavailable        every decisions model is unavailable; the escalation LLM answers
//	primary-low-confidence   below the confidence bar; the LLM answers, no fallback is asked
//	primary-low-probability  below the probability bar; the LLM answers, no fallback is asked
//	primary-thin-margin      below the margin bar; the LLM answers, no fallback is asked
//	primary-none             "none of these"; the LLM answers, no fallback is asked
//	all-fail                 every decisions model and the LLM fail: no verdict
var FakeJudgePaths = []string{"primary", "primary-unavailable", "primary-error", "primary-unparseable", "chain-unavailable",
	"primary-low-confidence", "primary-low-probability", "primary-thin-margin", "primary-none", "all-fail"}

// IsFakeJudgePath reports whether p is one of FakeJudgePaths.
func IsFakeJudgePath(p string) bool {
	for _, k := range FakeJudgePaths {
		if k == p {
			return true
		}
	}
	return false
}

// Calls reports how many times each fixture model was asked, by model.
func (f *Fake) Calls() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int{}
	for k, v := range f.calls {
		out[k] = v
	}
	return out
}

// Chain is the test grading chain: the real orchestrator over fixture steps.
func (f *Fake) Chain() *Chain {
	return &Chain{
		Decisions: []GradingStep{
			&fakeStep{f: f, model: FakeJudgeModel, pos: 0},
			&fakeStep{f: f, model: FakeFallbackModel1, pos: 1},
			&fakeStep{f: f, model: FakeFallbackModel2, pos: 2},
		},
		Escalation: &fakeStep{f: f, model: FakeEscalationModel, pos: -1},
		Log:        f.Log,
	}
}

type fixtureKey struct{}

// fixture is what one submission was scripted to do. It is chosen before
// the chain starts, and never from the answer.
type fixture struct {
	path string
	j    Judgment
	use  Usage
}

func (f *Fake) Judge(ctx context.Context, in JudgeInput) (Judgment, Usage, error) {
	use := Usage{Model: FakeJudgeModel, InputTokens: int64(60 + len(in.Question+in.Expected+in.Answer)/4), OutputTokens: 40}
	if f.Err != nil {
		return Judgment{}, Usage{}, f.Err
	}
	fx := fixture{path: "primary", use: use}
	f.mu.Lock()
	switch {
	case f.Judgment != nil:
		fx.j = *f.Judgment
	case len(f.Verdicts) > 0:
		v := f.Verdicts[f.next%len(f.Verdicts)]
		f.next++
		fx.j = Judgment{Verdict: v, Reason: CriterionSentence(v)}
	default:
		fx.j = Judgment{Verdict: srs.Right, Reason: "Test fixture: right (not an assessment of your answer)."}
	}
	if len(f.Paths) > 0 {
		fx.path = f.Paths[f.nextPath%len(f.Paths)]
		f.nextPath++
	}
	f.mu.Unlock()
	if f.JudgeDelay > 0 {
		select {
		case <-ctx.Done():
			return Judgment{}, Usage{}, ctx.Err()
		case <-time.After(f.JudgeDelay):
		}
	}
	return f.Chain().Judge(context.WithValue(ctx, fixtureKey{}, fx), in)
}

// fakeStep is one fixture model of the test chain. What it does depends
// only on the submission's scripted path and its own place in the chain.
type fakeStep struct {
	f     *Fake
	model string
	pos   int // 0 the primary, 1 and 2 the fallbacks, -1 the escalation
}

func (s *fakeStep) Name() string           { return "test" }
func (s *fakeStep) RequestedModel() string { return s.model }

func (s *fakeStep) Judge(ctx context.Context, _ JudgeInput) (Judgment, Usage, error) {
	s.f.mu.Lock()
	if s.f.calls == nil {
		s.f.calls = map[string]int{}
	}
	s.f.calls[s.model]++
	s.f.mu.Unlock()
	fx, ok := ctx.Value(fixtureKey{}).(fixture)
	if !ok {
		return Judgment{}, Usage{}, notSent("no test fixture for this submission")
	}
	use := fx.use
	use.Model = s.model
	answer := func() (Judgment, Usage, error) { return fx.j, use, nil }
	billed := func(why string) (Judgment, Usage, error) { return Judgment{}, use, unavailable("test fixture: " + why) }
	down := func() (Judgment, Usage, error) {
		return Judgment{}, Usage{Model: s.model}, unavailable("test fixture: unavailable")
	}
	escalation := s.pos < 0
	switch fx.path {
	case "primary":
		return answer()
	case "primary-unavailable", "primary-error", "primary-unparseable":
		switch {
		case s.pos != 0:
			return answer()
		case fx.path == "primary-unavailable":
			return down()
		case fx.path == "primary-error":
			return billed("error")
		}
		return billed("unparseable reply")
	case "chain-unavailable":
		if escalation {
			return answer()
		}
		return down()
	case "primary-low-confidence", "primary-low-probability", "primary-thin-margin", "primary-none":
		switch {
		case escalation:
			return answer()
		case s.pos == 0:
			return Judgment{}, use, fmt.Errorf("%w: test fixture: %s", ErrDecisionUncertain, strings.TrimPrefix(fx.path, "primary-"))
		}
		// Never reached: an uncertain verdict is never shopped to another
		// decisions model. If it were, Calls would show it.
		return billed("a fallback was asked after an uncertain verdict")
	case "all-fail":
		if escalation {
			return billed("escalation failed")
		}
		return down()
	}
	return Judgment{}, Usage{}, notSent("unknown test fixture path " + fx.path)
}

// books is the small shelf the test model can turn into core ideas.
var books = []struct {
	match []string
	title string
	ideas []DraftIdea
}{
	{[]string{"meditations", "marcus aurelius"}, "Meditations", []DraftIdea{
		{"The dichotomy of control", "Some things are up to us and some are not; peace comes from caring only about the first.", []string{"The view from above"}, []DraftItem{
			{"In Meditations, what does Marcus Aurelius say is always within our power?", "Our own judgments and actions"},
			{"According to Meditations, what disturbs people: events, or something else?", "Their judgments about events, not the events themselves"}}},
		{"The view from above", "Seen from far enough away, most troubles are small and brief.", []string{"Memento mori"}, []DraftItem{
			{"What exercise does Marcus Aurelius use in Meditations to shrink his troubles?", "Picturing human affairs from high above, where they look small and brief"},
			{"In Meditations, what does the view from above show about fame?", "That it is short-lived and soon forgotten"}}},
		{"Memento mori", "Remembering death is a way to act well today, not a reason for gloom.", []string{"The dichotomy of control"}, []DraftItem{
			{"How does Meditations say to act, given that life could end at any moment?", "Do each act as if it were your last"},
			{"In Meditations, what does death have in common with birth?", "Both are natural processes, and nothing natural is evil"}}},
		{"The common good", "People are made for one another; what harms the hive harms the bee.", []string{"The dichotomy of control"}, []DraftItem{
			{"Complete the line from Meditations: \"What is not good for the hive…\"", "…is not good for the bee"},
			{"According to Meditations, what are human beings made for?", "For one another: to work together"}}},
	}},
	{[]string{"federalist"}, "The Federalist Papers", []DraftIdea{
		{"Faction", "Groups united by a passion or interest against the rights of others are inevitable; the cure is to control their effects.", []string{"The extended republic"}, []DraftItem{
			{"Which Federalist paper gives Madison's argument about factions?", "Federalist No. 10"},
			{"In Federalist No. 10, what does Madison say causes faction?", "The unequal distribution of property, and human nature itself"}}},
		{"The extended republic", "A large republic is safer than a small one because no single faction can easily form a majority.", []string{"Faction"}, []DraftItem{
			{"Why does Madison argue a large republic guards against faction better than a small one?", "More interests compete, so a majority faction is harder to form"},
			{"Whose view that republics must be small did The Federalist Papers argue against?", "Montesquieu's"}}},
		{"Separation of powers", "Each branch must have the means and the motive to resist the others.", []string{"Faction"}, []DraftItem{
			{"Complete Madison's line from Federalist No. 51: \"Ambition must be made to…\"", "…counteract ambition"},
			{"In Federalist No. 51, what does Madison say would make government unnecessary?", "If men were angels"}}},
		{"An energetic executive", "Good government needs a single executive with energy, not a committee.", []string{"Separation of powers"}, []DraftItem{
			{"Which Federalist paper argues for \"energy in the executive\", and who wrote it?", "Federalist No. 70, by Alexander Hamilton"},
			{"Who were the three authors of The Federalist Papers?", "Alexander Hamilton, James Madison and John Jay"}}},
	}},
	{[]string{"republic", "plato"}, "The Republic", []DraftIdea{
		{"Justice in the soul", "A just person is one whose reason rules spirit and appetite, each doing its own work.", []string{"The just city"}, []DraftItem{
			{"What are the three parts of the soul in Plato's Republic?", "Reason, spirit and appetite"},
			{"In the Republic, what is justice in the individual?", "Each part of the soul doing its own work, with reason ruling"}}},
		{"The just city", "The city is the soul written large: rulers, guardians and producers each keep to their task.", []string{"Justice in the soul", "The philosopher-king"}, []DraftItem{
			{"What are the three classes of Plato's ideal city in the Republic?", "Rulers, auxiliaries (guardians) and producers"},
			{"Why does Socrates describe a city in the Republic at all?", "To see justice written large before finding it in the soul"}}},
		{"The allegory of the cave", "Most people take shadows for reality; education turns the whole soul toward the light.", []string{"The philosopher-king"}, []DraftItem{
			{"In Plato's allegory of the cave, what do the prisoners take for reality?", "Shadows cast on the wall"},
			{"In the allegory of the cave, what does the sun stand for?", "The Form of the Good"}}},
		{"The philosopher-king", "Cities will have no rest from evils until philosophers rule or rulers become philosophers.", []string{"The allegory of the cave"}, []DraftItem{
			{"Who should rule the ideal city, according to Plato's Republic?", "Philosophers: those who know the Good"},
			{"In the Republic, why must the philosopher return to the cave?", "To govern: those who have seen the Good owe the city their rule"}}},
	}},
}

func meaningless(q string) bool {
	letters, vowels := 0, 0
	for _, r := range strings.ToLower(q) {
		if unicode.IsLetter(r) {
			letters++
			if strings.ContainsRune("aeiouy", r) {
				vowels++
			}
		}
	}
	return letters < 3 || vowels == 0
}

func (f *Fake) Draft(ctx context.Context, in DraftInput, sink DraftSink) (DraftResult, Usage, error) {
	use := Usage{Model: "test-draft", InputTokens: int64(400 + len(in.Query)/4 + 8*len(in.Learner.Knows))}
	if f.Err != nil {
		return DraftResult{}, Usage{}, f.Err
	}
	q := strings.ToLower(strings.TrimSpace(in.Query))
	if meaningless(q) {
		use.OutputTokens = 30
		return DraftResult{Note: "Scry couldn't find anything worth learning in that. Try a narrower topic, a question, or a book's title."}, use, nil
	}
	head := DraftHead{Kind: "topic", Title: strings.TrimSpace(in.Query)}
	var ideas []DraftIdea
	if IsMaterial(in.Query) {
		// Pasted Markdown, prose or notes: a fixed outline from its own
		// headings or paragraphs (S106-2), never a book or starter match.
		head, ideas = materialDraft(in.Query)
		if len(ideas) == 0 {
			use.OutputTokens = 30
			return DraftResult{Note: "Scry couldn't find anything to make cards from in that. Paste notes with a few full lines, or type a topic."}, use, nil
		}
	}
	material := ideas != nil
	for _, b := range books {
		for _, m := range b.match {
			if !material && strings.Contains(q, m) {
				head, ideas = DraftHead{Kind: "book", Title: b.title}, b.ideas
			}
		}
	}
	if ideas == nil {
		// A topic the starter shelf covers is drafted from it.
		for _, s := range starter.All() {
			name := strings.ToLower(strings.TrimPrefix(s.Title, "Core ideas in "))
			if strings.Contains(q, name) || strings.Contains(name, q) {
				head.Title = s.Title
				for _, i := range s.Ideas {
					d := DraftIdea{Title: i.Title, Summary: i.Summary}
					for _, l := range i.Links {
						d.Links = append(d.Links, starter.ResolveLink(s.Slug, l).Title)
					}
					for _, it := range i.Items {
						d.Items = append(d.Items, DraftItem{it.Q, it.A})
					}
					if len(d.Items) > 0 {
						ideas = append(ideas, d)
					}
				}
				break
			}
		}
	}
	if ideas == nil {
		// Anything else gets a plainly labelled placeholder outline: the
		// test model knows nothing, and says so.
		t := head.Title
		ideas = []DraftIdea{
			{"What " + t + " is", "A placeholder idea: test mode has no model to write about " + t + ".", []string{"Why " + t + " matters"}, []DraftItem{
				{"Which topic did you ask Scry's test model to draft?", t},
				{"What writes Scry's drafts when it is not in test mode?", "A language model"}}},
			{"Why " + t + " matters", "A second placeholder, so the outline has ideas to link.", []string{"What " + t + " is"}, []DraftItem{
				{"In test mode, are Scry's drafts written by a model or by a script?", "By a script"},
				{"Where do drafted items wait until you approve them?", "In the draft outline; nothing enters the library until approved"}}},
		}
	}
	knows := map[string]bool{}
	for _, k := range in.Learner.Knows {
		knows[strings.ToLower(k)] = true
	}
	if sink.Head != nil {
		if err := sink.Head(head); err != nil {
			return DraftResult{}, use, err
		}
	}
	res := DraftResult{}
	skipped := 0
	for _, idea := range ideas {
		// Shaped to the learner: ideas already in their library are left out.
		if knows[strings.ToLower(idea.Title)] {
			skipped++
			continue
		}
		if f.Delay > 0 {
			select {
			case <-ctx.Done():
				return res, use, ctx.Err()
			case <-time.After(f.Delay):
			}
		}
		if err := sink.Idea(idea); err != nil {
			return res, use, err
		}
		res.Ideas++
		use.OutputTokens += 450
	}
	switch {
	case res.Ideas == 0:
		res.Note = "Everything Scry would draft on that is already in your library. Try a narrower or neighbouring topic."
	case skipped > 0:
		res.Note = fmt.Sprintf("Shaped to you: left out %d ideas already in your library.", skipped)
		if skipped == 1 {
			res.Note = "Shaped to you: left out 1 idea already in your library."
		}
	case in.Learner.About != "":
		res.Note = "Shaped to you: written with what you told Scry about yourself in mind."
	}
	return res, use, nil
}
