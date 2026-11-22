package app

import (
	"context"
	"errors"
	"time"

	"scry/internal/srs"
	"scry/internal/starter"
	"scry/internal/store"
)

// Demo account details, used only by seeded test mode.
const (
	DemoEmail      = "phaedrus@example.com"
	DemoPassword   = "know-thyself"
	DemoAgentToken = "scry_demo_reading_agent_token"
)

// SeedDemo fills an empty database with one learner mid-way through their
// first month: a library with history, items due today, a waiting inbox
// and a billing ledger. It does nothing if the demo account exists.
func (a *App) SeedDemo(ctx context.Context) error {
	if _, err := a.Store.UserByEmail(ctx, DemoEmail); err == nil {
		return nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	now := a.now()
	u, err := a.SignUp(ctx, DemoEmail, DemoPassword, "UTC")
	if err != nil {
		return err
	}
	us := a.Store.For(u.ID)
	if err := us.SetAbout(ctx, "I write for a living, read philosophy and history, cook most nights, and want the foundations of science to stick."); err != nil {
		return err
	}

	// The plan: trial started 20 days ago, first charge 13 days ago.
	tok, err := a.Pay.Tokenize("4242 4242 4242 4242", "12/"+now.AddDate(3, 0, 0).Format("06"), "123", now)
	if err != nil {
		return err
	}
	if err := a.StartTrial(ctx, u, tok, a.CardTerms(store.Subscription{})); err != nil {
		return err
	}
	sub, err := us.Subscription(ctx)
	if err != nil {
		return err
	}
	trialStart := now.AddDate(0, 0, -20)
	paid := trialStart.AddDate(0, 0, a.Cfg.TrialDays)
	sub.Status, sub.TrialEnd = "active", paid
	sub.PeriodStart, sub.PeriodEnd = paid, paid.AddDate(0, 1, 0)
	sub.AllowanceMicros, sub.UsedMicros = a.Cfg.AllowanceMicros, a.Cfg.AllowanceMicros*62/100
	sub.TopupMicros = 1_200_000
	if err := us.SaveSubscription(ctx, sub); err != nil {
		return err
	}
	if _, err := a.Store.DB().ExecContext(ctx, `UPDATE ledger SET at = ? WHERE user_id = ?`, trialStart.Unix(), u.ID); err != nil {
		return err
	}
	for _, e := range []store.LedgerEntry{
		{At: paid, Kind: "charge", Description: "Scry, " + period(sub.PeriodStart, sub.PeriodEnd), AmountCents: a.Cfg.PriceCents, Status: "paid", Ref: "seed-charge-1"},
		{At: now.AddDate(0, 0, -4), Kind: "topup", Description: "Top-up", AmountCents: a.Cfg.TopupCents, Status: "paid", Ref: "seed-topup-1"},
	} {
		if err := us.AddLedger(ctx, e); err != nil {
			return err
		}
	}

	// The library: four starter sets, two of them partly learned.
	for _, slug := range []string{"nato-alphabet", "mental-models", "kitchen-numbers", "the-creeds"} {
		set, ok := starter.Get(slug)
		if !ok {
			continue
		}
		parent, _, err := us.AddStarterSet(ctx, set, now.AddDate(0, 0, -19))
		if err != nil {
			return err
		}
		if slug != "nato-alphabet" && slug != "mental-models" {
			continue
		}
		items, err := us.Items(ctx, parent)
		if err != nil {
			return err
		}
		for i, it := range items {
			// A spread of histories, so some items are due today, some later
			// and some have never been seen.
			var plan []struct {
				daysAgo int
				v       srs.Verdict
			}
			switch i % 6 {
			case 0:
				plan = append(plan, struct {
					daysAgo int
					v       srs.Verdict
				}{5, srs.Right}) // due yesterday: due today
			case 1:
				plan = append(plan, struct {
					daysAgo int
					v       srs.Verdict
				}{17, srs.Right}, struct {
					daysAgo int
					v       srs.Verdict
				}{13, srs.Right}) // well learned
			case 2:
				plan = append(plan, struct {
					daysAgo int
					v       srs.Verdict
				}{6, srs.Right}, struct {
					daysAgo int
					v       srs.Verdict
				}{1, srs.Missed}) // missed yesterday: back today
			case 3:
				plan = append(plan, struct {
					daysAgo int
					v       srs.Verdict
				}{2, srs.Right}) // due in a couple of days
			case 4:
				plan = append(plan, struct {
					daysAgo int
					v       srs.Verdict
				}{9, srs.Close}, struct {
					daysAgo int
					v       srs.Verdict
				}{7, srs.Right})
			}
			state := it.Sched
			for _, p := range plan {
				at := now.Add(-time.Duration(p.daysAgo)*24*time.Hour - time.Hour)
				state = srs.Next(state, p.v, at)
				reason := "That matches the answer."
				answer := it.Answer
				if p.v != srs.Right {
					reason, answer = "The answer is “"+it.Answer+"”.", ""
				}
				if _, err := us.RecordReview(ctx, store.Review{ItemID: it.ID, At: at, Answer: answer, Verdict: p.v,
					Reason: reason, Detail: "{}", Via: "typed", Judge: "seed"}, state, false); err != nil {
					return err
				}
			}
		}
	}

	// Agents and their suggestions.
	for _, t := range []struct {
		name, token string
	}{{"reading agent", DemoAgentToken}, {"conversation agent", "scry_" + randomToken(30)}} {
		if err := us.CreateToken(ctx, t.name, hashToken(t.token), false, now.AddDate(0, 0, -10)); err != nil {
			return err
		}
	}
	for _, s := range []store.Suggestion{
		{Agent: "reading agent", Source: "The Federalist Papers", IdeaTitle: "Faction", IdeaSummary: "Groups united against the rights of others are inevitable; the cure is to control their effects.",
			Prompt: "In Federalist No. 10, what does Madison call a group united by a passion adverse to the rights of others?", Answer: "A faction"},
		{Agent: "reading agent", Source: "The Federalist Papers", IdeaTitle: "Faction",
			Prompt: "According to Federalist No. 10, what are the two ways of removing the causes of faction?", Answer: "Destroying liberty, or giving everyone the same opinions and interests"},
		{Agent: "reading agent", Source: "The Federalist Papers", IdeaTitle: "Separation of powers", IdeaSummary: "Each branch must have the means and the motive to resist the others.",
			Prompt: "Complete Madison's line from Federalist No. 51: \"Ambition must be made to…\"", Answer: "…counteract ambition"},
		{Agent: "reading agent", Source: "The Federalist Papers", IdeaTitle: "Separation of powers",
			Prompt: "Which Federalist paper argues for \"energy in the executive\", and who wrote it?", Answer: "Federalist No. 70, by Alexander Hamilton"},
		{Agent: "conversation agent", Source: "Chat about sourdough, " + now.AddDate(0, 0, -2).Format("Jan 2"), IdeaTitle: "Ratios worth knowing",
			Prompt: "In baker's percentages, what does 75% hydration mean?", Answer: "75 grams of water for every 100 grams of flour"},
		{Agent: "conversation agent", Source: "Chat about sourdough, " + now.AddDate(0, 0, -2).Format("Jan 2"), IdeaTitle: "Ratios worth knowing",
			Prompt: "What is the 1:2:3 sourdough ratio, by weight?", Answer: "1 part starter, 2 parts water, 3 parts flour"},
		{Agent: "conversation agent", Source: "Chat about a hiring decision, " + now.AddDate(0, 0, -1).Format("Jan 2"), IdeaTitle: "Reasoning from the ground up",
			Prompt: "Before a hiring decision, what does inversion ask you to list first?", Answer: "The ways the hire could fail, so you can avoid them"},
	} {
		if _, err := us.AddSuggestion(ctx, s, now.AddDate(0, 0, -1)); err != nil {
			return err
		}
	}
	a.Log.InfoContext(ctx, "demo data seeded", "email", DemoEmail, "password", DemoPassword)
	return nil
}
