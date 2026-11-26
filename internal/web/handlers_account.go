package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"scry/internal/ai"
	"scry/internal/app"
	"scry/internal/auth"
	"scry/internal/billing"
	"scry/internal/srs"
	"scry/internal/store"
)

func (s *Server) home(w http.ResponseWriter, r *http.Request, c *rc) error {
	if c.signedIn {
		return back(w, r, "/study")
	}
	return s.render(w, r, c, http.StatusOK, "landing.html", map[string]any{
		"Title": "Keep what's worth knowing", "Price": s.App.Cfg.PriceCents, "TrialDays": s.App.Cfg.TrialDays, "Bare": true,
		"Specimen": landingSpecimen(),
	})
}

// landingSpecimen is the landing page's Study Specimen (DESIGN.md, Landing):
// one real card stepped from its prompt to a typed answer to its judgment.
// The card is the starter set's Hanlon's razor card asked the other way
// round (the razor, answered with what it says), the typed answer says the
// same thing in other words, and the one-line why is the grader's own plain
// sentence for the verdict. Its subject header ([S131-1]) is the one a
// study session shows for the card while it waits: the starter set's layers
// icon and name, the spoiler-safe "Set 1" and the count, never a "Study ·"
// prefix. Next is the one card coming up on the phone's picture of Study
// ([S135-1]).
func landingSpecimen() map[string]string {
	return map[string]string{
		"DeckIcon":    "layers",
		"Deck":        "Mental models",
		"Section":     "Set 1",
		"Count":       "4 / 11",
		"CountSpoken": "Card 4 of 11",
		"Next":        "NATO alphabet: K",
		"Prompt":      "Hanlon's razor",
		"Answer":      "don't assume malice when carelessness or stupidity explains it",
		"Expected":    "Never attribute to malice what is adequately explained by carelessness or stupidity",
		"Verdict":     string(srs.Right),
		"Icon":        "verdict-right",
		"Badge":       "Correct",
		"Why":         ai.PlainWhy(srs.Right),
	}
}

// clerkForm keeps the server-rendered copy aligned with the mounted component.
func (s *Server) clerkForm(w http.ResponseWriter, r *http.Request, c *rc, provider *auth.Clerk) error {
	signup := r.URL.Path == "/signup"
	title, lede := "Sign in", "Use your email and password: enter your email first, then your password. Forgotten your password? After you enter your email, choose “Forgot password” on the password step to reset it by email."
	if signup {
		title, lede = "Sign up", "Create your account with email and password. Verify your email before continuing to study or billing."
	}
	if !provider.Configured() {
		title, lede = "Sign-in not configured", "Please try again when the operator has connected Clerk."
		if signup {
			title = "Sign-up not configured"
		}
	}
	return s.render(w, r, c, http.StatusOK, "clerk.html", map[string]any{
		"Title": title, "Lede": lede, "Bare": true, "Configured": provider.Configured(), "SignUp": signup, "TrialDays": s.App.Cfg.TrialDays,
	})
}

func (s *Server) signupForm(w http.ResponseWriter, r *http.Request, c *rc) error {
	if provider, ok := s.App.Auth.(*auth.Clerk); ok {
		return s.clerkForm(w, r, c, provider)
	}

	if c.signedIn {
		return back(w, r, "/study")
	}
	return s.render(w, r, c, http.StatusOK, "signup.html", map[string]any{"Title": "Sign up", "Bare": true, "TrialDays": s.App.Cfg.TrialDays})
}

func (s *Server) signup(w http.ResponseWriter, r *http.Request, c *rc) error {
	if provider, ok := s.App.Auth.(*auth.Clerk); ok {
		return s.clerkForm(w, r, c, provider)
	}

	email, password := strings.TrimSpace(r.FormValue("email")), r.FormValue("password")
	u, err := s.App.SignUp(r.Context(), email, password, r.FormValue("tz"))
	var fe *app.FieldError
	if errors.As(err, &fe) {
		return s.render(w, r, c, http.StatusUnprocessableEntity, "signup.html", map[string]any{
			"Title": "Sign up", "Bare": true, "Email": email, "Field": fe.Field, "Error": fe.Msg, "TrialDays": s.App.Cfg.TrialDays})
	}
	if err != nil {
		return err
	}
	token, err := s.App.NewSession(r.Context(), u.ID)
	if err != nil {
		return err
	}
	s.setCookie(w, r, token, app.SessionTTL)
	// The no-card trial has started: straight into Scry, no card step.
	return s.land(w, r, &rc{user: u, us: s.App.Store.For(u.ID), signedIn: true})
}

// land sends a learner who has just started (or added a card to) a plan
// to where studying begins: the starter shelf while the library is empty,
// otherwise Study.
func (s *Server) land(w http.ResponseWriter, r *http.Request, c *rc) error {
	items, _, err := c.us.Counts(r.Context(), s.App.Now())
	if err != nil {
		return err
	}
	if items == 0 {
		return back(w, r, "/shelf")
	}
	return back(w, r, "/study")
}

func (s *Server) signinForm(w http.ResponseWriter, r *http.Request, c *rc) error {
	if provider, ok := s.App.Auth.(*auth.Clerk); ok {
		return s.clerkForm(w, r, c, provider)
	}

	if c.signedIn {
		return back(w, r, "/study")
	}
	return s.render(w, r, c, http.StatusOK, "signin.html", map[string]any{"Title": "Sign in", "Bare": true, "Next": r.URL.Query().Get("next"), "Demo": s.demo()})
}

func (s *Server) demo() map[string]string {
	if !s.DemoHint {
		return nil
	}
	return map[string]string{"Email": app.DemoEmail, "Password": app.DemoPassword}
}

func safeNext(next string) string {
	if u, err := url.Parse(next); err == nil && u.Host == "" && u.Scheme == "" && strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") {
		return next
	}
	return "/study"
}

func (s *Server) signin(w http.ResponseWriter, r *http.Request, c *rc) error {
	if provider, ok := s.App.Auth.(*auth.Clerk); ok {
		return s.clerkForm(w, r, c, provider)
	}

	email, password := strings.TrimSpace(r.FormValue("email")), r.FormValue("password")
	fail := func(msg string, status int) error {
		return s.render(w, r, c, status, "signin.html", map[string]any{
			"Title": "Sign in", "Bare": true, "Email": email, "Error": msg, "Next": r.FormValue("next"), "Demo": s.demo()})
	}
	if s.tooMany(clientIP(r) + "|" + strings.ToLower(email)) {
		return fail("Too many attempts. Wait a few minutes and try again.", http.StatusTooManyRequests)
	}
	u, err := s.App.SignIn(r.Context(), email, password)
	var fe *app.FieldError
	if errors.As(err, &fe) {
		return fail(fe.Msg, http.StatusUnprocessableEntity)
	}
	if err != nil {
		return err
	}
	token, err := s.App.NewSession(r.Context(), u.ID)
	if err != nil {
		return err
	}
	s.setCookie(w, r, token, app.SessionTTL)
	return back(w, r, safeNext(r.FormValue("next")))
}

func (s *Server) signout(w http.ResponseWriter, r *http.Request, c *rc) error {
	if err := s.App.SignOut(r.Context(), c.token); err != nil {
		return err
	}
	s.setCookie(w, r, "", 0)
	return back(w, r, "/")
}

// cardData is what the card step needs: the trial sentence's date and how
// the processor wants the card collected. On a no-card trial the date is
// the trial's own end (KeepTrial when that much of it is left to keep).
func (s *Server) cardData(c *rc, title string, sub ...store.Subscription) map[string]any {
	cfg := s.App.Cfg
	data := map[string]any{
		"Title": title, "Bare": true, "TrialDays": cfg.TrialDays, "Price": cfg.PriceCents,
		"TrialEnd":       s.App.Now().In(c.user.Location()).AddDate(0, 0, cfg.TrialDays),
		"PublishableKey": s.App.Pay.PublishableKey(),
	}
	// Terms is what the page tells the learner adding a card costs today,
	// carried in the form as billing_terms: StartTrial never charges today
	// unless the submitted form said so.
	data["Terms"] = app.TermsTrial
	if len(sub) == 1 && sub[0].Status != store.SubNone {
		terms := s.App.CardTerms(sub[0])
		data["Terms"] = terms
		if sub[0].NoCard() {
			data["NoCard"] = true
			data["TrialEnd"] = sub[0].PeriodEnd.In(c.user.Location())
			data["KeepTrial"] = sub[0].NoCardTrial() && terms == app.TermsTrial
			data["Restart"] = !s.App.HasAccess(sub[0])
		}
	}
	return data
}

// termsChangedNotice is what a card step submitted on stale terms says
// when it is shown again to confirm the immediate charge.
func (s *Server) termsChangedNotice(sub store.Subscription) string {
	price := money(s.App.Cfg.PriceCents)
	switch {
	case sub.NoCard() && s.App.HasAccess(sub):
		return "Your trial now ends within the hour, so adding a card starts Scry today and charges " + price +
			" now, then monthly. Your trial is unchanged until you confirm. To go ahead, enter your card again."
	case sub.NoCard():
		return "Your trial has ended, so adding a card now charges " + price + " today, then monthly. To go ahead, enter your card again."
	}
	return "Your plan is no longer active, so a new card starts it again and charges " + price + " today, then monthly. To go ahead, enter your card again."
}

// termsChanged shows the card step again, at 409, after StartTrial
// refused a charge the submitted page had not promised (ErrTermsChanged):
// it now states the immediate charge, and its form carries
// billing_terms=immediate so the next submit is the learner's consent.
func (s *Server) termsChanged(w http.ResponseWriter, r *http.Request, c *rc, update bool) error {
	var data map[string]any
	var sub store.Subscription
	if update {
		var err error
		if data, sub, err = s.billingCardData(r, c); err != nil {
			return err
		}
	} else {
		var err error
		sub, err = s.App.Plan(r.Context(), c.user.ID)
		if err != nil && !errors.Is(err, app.ErrPlanEnded) {
			return err
		}
		data = s.cardData(c, "Add a card", sub)
	}
	data["Terms"], data["KeepTrial"] = app.TermsImmediate, false
	data["Notice"] = s.termsChangedNotice(sub)
	return s.render(w, r, c, http.StatusConflict, "card.html", data)
}

// cardInProgressNotice is what a card step says when an earlier one by
// the same learner is still with the card processor.
const cardInProgressNotice = "Your card is already being processed. Refresh Billing in a moment — nothing extra was charged."

// cardInProgress answers such a card step, at 409, with no form to submit
// again.
func (s *Server) cardInProgress(w http.ResponseWriter, r *http.Request, c *rc, data map[string]any) error {
	data["InProgress"] = cardInProgressNotice
	return s.render(w, r, c, http.StatusConflict, "card.html", data)
}

// cardToken turns the submitted card step into the processor's token. In
// test mode the plain test-card fields are tokenized by the test
// processor; with Stripe the browser has already done it.
func (s *Server) cardToken(r *http.Request) (string, error) {
	if s.App.Pay.PublishableKey() != "" {
		pm := r.FormValue("payment_method")
		if pm == "" {
			return "", &billing.DeclineError{Reason: "Enter your card details."}
		}
		return pm, nil
	}
	return s.App.Pay.Tokenize(r.FormValue("number"), r.FormValue("expiry"), r.FormValue("cvc"), s.App.Now())
}

// cardForm is the card step. A learner on the no-card trial (live or
// ended) adds a card here; one whose card-backed plan is live is sent to
// Study, and one whose card-backed plan ended restarts from Billing.
func (s *Server) cardForm(w http.ResponseWriter, r *http.Request, c *rc) error {
	sub, err := s.App.Plan(r.Context(), c.user.ID)
	switch {
	case err == nil && !sub.NoCard():
		return back(w, r, "/study")
	case errors.Is(err, app.ErrPlanEnded) && !sub.NoCard():
		return back(w, r, "/billing")
	case err != nil && !errors.Is(err, app.ErrPlanEnded) && !errors.Is(err, app.ErrNoPlan):
		return err
	}
	return s.render(w, r, c, http.StatusOK, "card.html", s.cardData(c, "Add a card", sub))
}

func (s *Server) cardSubmit(w http.ResponseWriter, r *http.Request, c *rc) error {
	sub, err := s.App.Plan(r.Context(), c.user.ID)
	switch {
	case err == nil && !sub.NoCard():
		return back(w, r, "/study")
	case errors.Is(err, app.ErrPlanEnded) && !sub.NoCard():
		return back(w, r, "/billing")
	case err != nil && !errors.Is(err, app.ErrPlanEnded) && !errors.Is(err, app.ErrNoPlan):
		return err
	}
	token, err := s.cardToken(r)
	if err == nil {
		err = s.App.StartTrial(r.Context(), c.user, token, app.ParseBillingTerms(r.FormValue("billing_terms")))
	}
	if errors.Is(err, app.ErrTermsChanged) {
		return s.termsChanged(w, r, c, false)
	}
	if errors.Is(err, app.ErrSubscribeInProgress) {
		return s.cardInProgress(w, r, c, s.cardData(c, "Add a card", sub))
	}
	var d *billing.DeclineError
	if errors.As(err, &d) {
		data := s.cardData(c, "Add a card", sub)
		data["Error"] = d.Reason
		return s.render(w, r, c, http.StatusPaymentRequired, "card.html", data)
	}
	if err != nil {
		return err
	}
	return s.land(w, r, c)
}

// billingView is everything the Billing page says, as sentences.
type billingView struct {
	Sub        store.Subscription
	State      string // trial-nocard, trial, active, cancelling, card-failed, ended
	Plan       string
	Allowance  string
	Covers     string
	Percent    int
	Topup      string
	CardFailed string
	GraceEnd   string
	CanTopUp   bool
	NoCard     bool // a no-card trial, live or ended: the next step is adding a card
	Ledger     []store.LedgerEntry
}

// trialLeft says how much of a trial is left, in whole days.
func trialLeft(d time.Duration) string {
	switch {
	case d <= 0:
		return "ended"
	case d < 24*time.Hour:
		return "last day"
	}
	return plural(int((d+24*time.Hour-1)/(24*time.Hour)), "day", "days") + " left"
}

func (s *Server) billingView(c *rc, sub store.Subscription, ledger []store.LedgerEntry, reserved ...int64) billingView {
	cfg := s.App.Cfg
	loc := c.user.Location()
	sub.TrialEnd = sub.TrialEnd.In(loc)
	sub.PeriodStart = sub.PeriodStart.In(loc)
	sub.PeriodEnd = sub.PeriodEnd.In(loc)
	sub.PastDueSince = sub.PastDueSince.In(loc)
	ledger = append([]store.LedgerEntry(nil), ledger...)
	for i := range ledger {
		ledger[i].At = ledger[i].At.In(loc)
	}
	date := func(t interface{ Format(string) string }) string { return t.Format("January 2") }
	card := "your card"
	if sub.CardBrand != "" {
		card = "your " + sub.CardBrand + " ending " + sub.CardLast4
	}
	price := money(cfg.PriceCents)
	end := sub.PeriodEnd.In(loc)
	v := billingView{Sub: sub, Ledger: ledger, Percent: app.UsedPercent(sub), CanTopUp: s.App.HasAccess(sub) && !sub.NoCard()}
	trialCap := money(cfg.TrialMicros / 10000)
	switch {
	case sub.Status == "ended" && sub.NoCard():
		v.State = "ended"
		v.NoCard = true
		v.Plan = "Your " + fmt.Sprint(cfg.TrialDays) + "-day trial ended on " + date(end) + ". Your library and history are kept. Add a card to keep studying: " + price + " a month, charged today."
	case sub.NoCardTrial():
		v.State = "trial-nocard"
		v.NoCard = true
		v.Plan = fmt.Sprintf("You're on the %d-day trial, with no card on file. It ends on ", cfg.TrialDays) + date(end) +
			" (" + trialLeft(end.Sub(s.App.Now())) + "), and nothing is charged. Add a card any time to keep Scry after it: " + price +
			" a month, first charged when the trial ends."
	case sub.Status == "ended":
		v.State = "ended"
		v.Plan = "Your plan ended on " + date(end) + ". Your library and history are kept. Start again to keep studying: " + price + " a month, charged today."
	case sub.Status == "past_due":
		v.State = "card-failed"
		v.CardFailed = sub.CardError
		if v.CardFailed == "" {
			v.CardFailed = "Your card was declined."
		}
		grace := sub.PastDueSince.In(loc).AddDate(0, 0, cfg.GraceDays)
		v.GraceEnd = date(grace)
		v.Plan = "Scry is " + price + " a month. The last charge to " + card + " did not go through, so your plan is paused from " + v.GraceEnd + " unless you update your card."
	case sub.CancelAtEnd:
		v.State = "cancelling"
		v.Plan = "Your plan ends on " + date(end) + ". You keep full access until then, and nothing more will be charged."
	case sub.Status == "trialing":
		v.State = "trial"
		v.Plan = fmt.Sprintf("You're on the %d-day trial. It ends on ", cfg.TrialDays) + date(sub.TrialEnd.In(loc)) +
			", when " + card + " is first charged " + price + ". After that Scry is " + price + " a month."
	default:
		v.State = "active"
		v.Plan = "Scry is " + price + " a month, charged to " + card + ". The next charge is on " + date(end) + "."
	}
	if sub.NoCardTrial() {
		p := v.Percent
		switch {
		case p >= 100:
			v.Allowance = "You've used all of the trial's " + trialCap + " allowance, so judging, speaking and drafting are paused. Add a card to keep going."
		case p >= 80:
			v.Allowance = "You've used " + fmt.Sprint(p) + "% of the trial's " + trialCap + " allowance, so you're near its limit."
		default:
			v.Allowance = "You've used " + fmt.Sprint(p) + "% of the trial's " + trialCap + " allowance."
		}
		v.Topup = "Top-ups need a card on file."
	} else if sub.Status != "ended" {
		reset := "It resets on " + date(end) + "."
		if sub.Status == "trialing" {
			reset = "The trial's allowance lasts until " + date(end) + ", when the monthly allowance begins."
		}
		p := v.Percent
		switch {
		case p >= 100 && sub.TopupMicros > 0:
			v.Allowance = "You've used all of this month's allowance and are now drawing on your top-up credit. " + reset
		case p >= 100:
			v.Allowance = "You've used all of this month's allowance, so judging, speaking and drafting are paused. " + reset + " Top up to carry on before then."
		case p >= 80:
			v.Allowance = "You've used " + fmt.Sprint(p) + "% of this month's allowance, so you're near its limit. " + reset
		default:
			v.Allowance = "You've used " + fmt.Sprint(p) + "% of this month's allowance. " + reset
		}
		if sub.TopupMicros > 0 {
			v.Topup = "You also have " + money((sub.TopupMicros+5000)/10000) + " of top-up credit, which is used after the allowance and does not expire."
		}
	}

	if !s.App.HasAccess(sub) {
		// Study, judging, speech and drafting all need a live plan (the
		// same gate /learn and /study use), so nothing here may claim the
		// leftover credit is spendable now.
		v.Covers = "Studying, judging, speaking and drafting need an active plan, so nothing can be reserved until you start again."
		if sub.TopupMicros > 0 {
			v.Covers += " Your " + money((sub.TopupMicros+5000)/10000) + " of top-up credit is kept and becomes usable again when you restart your plan."
		}
		v.Covers += " The " + price + " covers running Scry and a monthly allowance of " + money(cfg.AllowanceMicros/10000) + " of model time. Prices cover costs plus a small margin."
		return v
	}
	remaining := max(int64(0), app.Unreserved(sub, 0))
	for _, amount := range reserved {
		remaining = max(int64(0), remaining-amount)
	}
	draft := s.App.DraftBudget()
	if remaining < draft {
		v.Allowance += " Not enough allowance left for a draft. Smaller operations may still fit."
	}
	usage := "charged"
	if sub.NoCardTrial() {
		usage = "counted"
	}
	reserve := "Your remaining allowance can reserve " + plural(int(remaining/draft), "draft", "drafts") +
		" at the worst-case cost. Each draft needs about " + money((draft+5000)/10000) + " available before it starts; only actual usage is " + usage + ". Judging and speech share this allowance. In-flight work reduces what is available."
	if sub.NoCardTrial() {
		v.Covers = "The trial includes " + trialCap + " of model time for judging, speaking, explanations and drafting, and nothing is charged. " + reserve +
			" After the trial, " + price + " a month covers running Scry and a monthly allowance of " + money(cfg.AllowanceMicros/10000) +
			" of model time, and a top-up adds " + money(cfg.TopupMicros/10000) + " more. Prices cover costs plus a small margin."
		return v
	}
	v.Covers = "The " + price + " covers running Scry and a monthly allowance of " + money(cfg.AllowanceMicros/10000) +
		" of model time. " + reserve + " A top-up adds " + money(cfg.TopupMicros/10000) + " more. Prices cover costs plus a small margin."

	return v
}

func (s *Server) billing(w http.ResponseWriter, r *http.Request, c *rc) error {
	// Settle any top-up whose processor reply was lost before the page
	// says what was paid; a failure here only leaves it pending.
	settled, err := s.App.ReconcileTopUps(r.Context(), c.user.ID)
	if err != nil {
		s.Log.WarnContext(r.Context(), "top-up reconciliation failed", "err", err, "user", c.user.ID)
	}
	sub, err := s.App.Plan(r.Context(), c.user.ID)
	if errors.Is(err, app.ErrNoPlan) {
		return back(w, r, "/signup/card")
	}
	if err != nil && !errors.Is(err, app.ErrPlanEnded) {
		return err
	}
	if sub.Status == "past_due" && !s.App.HasAccess(sub) {
		sub.Status = "ended" // the grace period is over
	}
	ledger, err := c.us.Ledger(r.Context())
	if err != nil {
		return err
	}
	reserved, err := c.us.ReservedMicros(r.Context())
	if err != nil {
		return err
	}
	// The monthly cap is counted in UTC calendar months, with the same
	// helper App.TopUp uses, whatever zone the clock is in.
	count, err := c.us.MonthTopups(r.Context(), s.App.Now())
	if err != nil {
		return err
	}
	_, reset := store.MonthBounds(s.App.Now())
	flash := map[string]string{
		"duplicate":   "This top-up was already processed. No additional credit was added.",
		"pending":     "Your top-up is being confirmed with the card processor. It will be charged at most once; reload Billing in a moment to see it.",
		"released":    "The card processor did not take that top-up, so nothing was charged. Try again.",
		"refund_due":  "That top-up went through after this month's $20 top-up limit was reached, so it was not added as credit. It will be refunded to your card.",
		"unconfirmed": "We could not confirm that top-up with the card processor yet, so we can't say whether your card was charged. If the charge went through, its credit is added automatically and it is never charged twice. Until it is confirmed, it counts toward this month's top-up limit.",
		"expired":     "That top-up form had expired, so nothing was charged. Try again.",
		"topup":       "Topped up. " + money(s.App.Cfg.TopupMicros/10000) + " of credit was added.",
		"card":        "Your card was updated.",
		"resumed":     "Your plan continues.",
		"started":     "Welcome back. Your plan is active again.",
		"subscribed":  "Your card is on file and your plan is active.",
		"card_added":  "Your card is on file. Your trial continues, and the first charge is on the date above.",
		"needs_card":  "Top-ups need a card on file. Add a card first.",
	}[r.URL.Query().Get("ok")]
	if settled > 0 {
		flash = "Topped up. " + money(s.App.Cfg.TopupMicros/10000) + " of credit was added."
	}
	return s.render(w, r, c, http.StatusOK, "billing.html", map[string]any{
		"Title": "Billing", "Nav": "billing", "B": s.billingView(c, sub, ledger, reserved), "Flash": flash,
		"TopupCapped": count >= store.TopupCap, "TopupReset": reset.Format("January 2, 2006") + " UTC",
		"TopupToken": s.topupFormToken(c),
		"TopupPrice": s.App.Cfg.TopupCents, "Price": s.App.Cfg.PriceCents, "TopupError": r.URL.Query().Get("declined"),
	})
}

// topupFormToken is a fresh operation token for one rendered top-up
// form: a random nonce bound to this learner and session by an HMAC, so
// the server can tell its own forms from made-up tokens.
func (s *Server) topupFormToken(c *rc) string {
	nonce := app.NewTopupToken()
	return nonce + "." + topupMAC(c, nonce)
}

func topupMAC(c *rc, nonce string) string {
	m := hmac.New(sha256.New, []byte(c.sess.CSRF))
	fmt.Fprintf(m, "scry-topup|%d|%s", c.user.ID, nonce)
	return hex.EncodeToString(m.Sum(nil))[:32]
}

// topupNonce checks a submitted form token and returns its nonce.
func topupNonce(c *rc, token string) (string, bool) {
	nonce, mac, ok := strings.Cut(token, ".")
	if !ok || nonce == "" || c.sess.CSRF == "" {
		return "", false
	}
	return nonce, hmac.Equal([]byte(mac), []byte(topupMAC(c, nonce)))
}

func (s *Server) billingTopup(w http.ResponseWriter, r *http.Request, c *rc) error {
	nonce, ok := topupNonce(c, r.FormValue("topup_token"))
	if !ok {
		return back(w, r, "/billing?ok=expired")
	}
	added, err := s.App.TopUpResult(r.Context(), c.user.ID, nonce)
	switch {
	case errors.Is(err, store.ErrTopupCap):
		return back(w, r, "/billing")
	case errors.Is(err, app.ErrTopupPending):
		return back(w, r, "/billing?ok=pending")
	case errors.Is(err, app.ErrTopupUnconfirmed):
		return back(w, r, "/billing?ok=unconfirmed")
	case errors.Is(err, app.ErrTopupReleased):
		return back(w, r, "/billing?ok=released")
	case errors.Is(err, store.ErrTopupCapExceededLate):
		return back(w, r, "/billing?ok=refund_due")
	case errors.Is(err, app.ErrTopupToken):
		return back(w, r, "/billing?ok=expired")
	case errors.Is(err, app.ErrTopupNeedsCard):
		return back(w, r, "/billing?ok=needs_card")
	}
	var d *billing.DeclineError
	if errors.As(err, &d) {
		return back(w, r, "/billing?declined="+url.QueryEscape(d.Reason))
	}
	if err != nil {
		return err
	}
	if !added {
		return back(w, r, "/billing?ok=duplicate")
	}
	return back(w, r, "/billing?ok=topup")
}

func (s *Server) billingCancel(w http.ResponseWriter, r *http.Request, c *rc) error {
	if _, err := s.App.SetCancel(r.Context(), c.user.ID, true); err != nil {
		return err
	}
	return back(w, r, "/billing")
}

func (s *Server) billingResume(w http.ResponseWriter, r *http.Request, c *rc) error {
	if _, err := s.App.SetCancel(r.Context(), c.user.ID, false); err != nil {
		return err
	}
	return back(w, r, "/billing?ok=resumed")
}

func (s *Server) billingCardData(r *http.Request, c *rc) (map[string]any, store.Subscription, error) {
	sub, err := s.App.Plan(r.Context(), c.user.ID)
	if errors.Is(err, app.ErrNoPlan) {
		return nil, sub, app.ErrNoPlan
	}
	if err != nil && !errors.Is(err, app.ErrPlanEnded) {
		return nil, sub, err
	}
	data := s.cardData(c, "Update card", sub)
	if sub.NoCard() {
		data["Title"] = "Add a card"
	}
	data["Bare"] = false
	data["Nav"] = "billing"
	data["Update"] = true
	data["Restart"] = !s.App.HasAccess(sub)
	return data, sub, nil
}

func (s *Server) billingCardForm(w http.ResponseWriter, r *http.Request, c *rc) error {
	data, _, err := s.billingCardData(r, c)
	if err != nil {
		return err
	}
	return s.render(w, r, c, http.StatusOK, "card.html", data)
}

func (s *Server) billingCard(w http.ResponseWriter, r *http.Request, c *rc) error {
	data, sub, err := s.billingCardData(r, c)
	if err != nil {
		return err
	}
	agreed := app.ParseBillingTerms(r.FormValue("billing_terms"))
	token, err := s.cardToken(r)
	ok := "card"
	if err == nil {
		switch {
		case sub.NoCard():
			// The first card: added to the trial, or (trial over) the plan
			// starts today. Never an update of a customer that does not exist.
			if err = s.App.StartTrial(r.Context(), c.user, token, agreed); err == nil {
				ok = "subscribed"
				if now, nerr := c.us.Subscription(r.Context()); nerr == nil && now.Status == store.SubTrialing {
					ok = "card_added"
				}
			}
		case s.App.HasAccess(sub):
			err = s.App.UpdateCard(r.Context(), c.user.ID, token)
		case agreed != app.TermsImmediate:
			// The plan is over, so a new card would charge today, which the
			// submitted page did not say: confirm first, change nothing.
			err = app.ErrTermsChanged
		default:
			// The plan is over: a new card starts it again, charged today.
			// Only the lapsed plan this page read is ended: one another
			// request has restarted since is left alone.
			if _, err = c.us.ChangeSubscription(r.Context(), func(cur *store.Subscription) {
				if cur.SubscriptionRef == sub.SubscriptionRef && !s.App.HasAccess(*cur) {
					cur.Status = store.SubEnded
				}
			}); err != nil {
				return err
			}
			err, ok = s.App.StartTrial(r.Context(), c.user, token, agreed), "started"
		}
	}
	if errors.Is(err, app.ErrTermsChanged) {
		return s.termsChanged(w, r, c, true)
	}
	if errors.Is(err, app.ErrSubscribeInProgress) {
		return s.cardInProgress(w, r, c, data)
	}
	var d *billing.DeclineError
	if errors.As(err, &d) {
		data["Error"] = d.Reason
		return s.render(w, r, c, http.StatusPaymentRequired, "card.html", data)
	}
	if err != nil {
		return err
	}
	return back(w, r, "/billing?ok="+ok)
}

func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	payload, err := readLimited(r, 1<<20)
	if err != nil {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	events, err := s.App.Pay.ParseWebhook(r.Context(), payload, r.Header.Get("Stripe-Signature"), s.App.Now())
	if err != nil {
		s.Log.WarnContext(r.Context(), "webhook refused", "err", err)
		http.Error(w, "refused", http.StatusBadRequest)
		return
	}
	for _, ev := range events {
		if err := s.App.ApplyEvent(r.Context(), ev); err != nil {
			s.Log.ErrorContext(r.Context(), "webhook failed", "err", err, "event", ev.ID)
			s.App.Alert.Notify(r.Context(), "Billing webhooks are failing", err.Error())
			http.Error(w, "try again", http.StatusInternalServerError) // the processor retries
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
