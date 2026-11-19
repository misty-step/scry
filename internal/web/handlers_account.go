package web

import (
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
	"scry/internal/store"
)

func (s *Server) home(w http.ResponseWriter, r *http.Request, c *rc) error {
	if c.signedIn {
		return back(w, r, "/study")
	}
	return s.render(w, r, c, http.StatusOK, "landing.html", map[string]any{
		"Title": "Keep what's worth knowing", "Price": s.App.Cfg.PriceCents, "TrialDays": s.App.Cfg.TrialDays, "Bare": true,
	})
}

// clerkForm keeps the server-rendered copy aligned with the mounted component.
func (s *Server) clerkForm(w http.ResponseWriter, r *http.Request, c *rc, provider *auth.Clerk) error {
	signup := r.URL.Path == "/signup"
	title, lede := "Sign in", "Use your email and password. Forgotten your password? Choose “Forgot password” below to reset it by email."
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
		"Title": title, "Lede": lede, "Bare": true, "Configured": provider.Configured(), "SignUp": signup,
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
	return back(w, r, "/signup/card")
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
// the processor wants the card collected.
func (s *Server) cardData(c *rc, title string) map[string]any {
	cfg := s.App.Cfg
	return map[string]any{
		"Title": title, "Bare": true, "TrialDays": cfg.TrialDays, "Price": cfg.PriceCents,
		"TrialEnd":       s.App.Now().In(c.user.Location()).AddDate(0, 0, cfg.TrialDays),
		"PublishableKey": s.App.Pay.PublishableKey(),
	}
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

func (s *Server) cardForm(w http.ResponseWriter, r *http.Request, c *rc) error {
	_, err := s.App.Plan(r.Context(), c.user.ID)
	if err == nil {
		return back(w, r, "/study")
	}
	if errors.Is(err, app.ErrPlanEnded) {
		return back(w, r, "/billing")
	}
	if !errors.Is(err, app.ErrNoPlan) {
		return err
	}
	return s.render(w, r, c, http.StatusOK, "card.html", s.cardData(c, "Add a card"))
}

func (s *Server) cardSubmit(w http.ResponseWriter, r *http.Request, c *rc) error {
	if _, err := s.App.Plan(r.Context(), c.user.ID); err == nil {
		return back(w, r, "/study")
	}
	token, err := s.cardToken(r)
	if err == nil {
		err = s.App.StartTrial(r.Context(), c.user, token)
	}
	var d *billing.DeclineError
	if errors.As(err, &d) {
		data := s.cardData(c, "Add a card")
		data["Error"] = d.Reason
		return s.render(w, r, c, http.StatusPaymentRequired, "card.html", data)
	}
	if err != nil {
		return err
	}
	items, _, err := c.us.Counts(r.Context(), s.App.Now())
	if err != nil {
		return err
	}
	if items == 0 {
		return back(w, r, "/shelf")
	}
	return back(w, r, "/study")
}

// billingView is everything the Billing page says, as sentences.
type billingView struct {
	Sub        store.Subscription
	State      string // trial, active, cancelling, card-failed, ended
	Plan       string
	Allowance  string
	Covers     string
	Percent    int
	Topup      string
	CardFailed string
	GraceEnd   string
	CanTopUp   bool
	Ledger     []store.LedgerEntry
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
	v := billingView{Sub: sub, Ledger: ledger, Percent: app.UsedPercent(sub), CanTopUp: s.App.HasAccess(sub)}
	switch {
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
	if sub.Status != "ended" {
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

	remaining := max(int64(0), sub.AllowanceMicros-sub.UsedMicros+sub.TopupMicros)
	for _, amount := range reserved {
		remaining = max(int64(0), remaining-amount)
	}
	if remaining < ai.DraftBudgetMicros {
		v.Allowance += " Not enough allowance left for a draft. Smaller operations may still fit."
	}
	v.Covers = "The " + price + " covers running Scry and a monthly allowance of " + money(cfg.AllowanceMicros/10000) +
		" of model time. Your remaining allowance can reserve " + plural(int(remaining/ai.DraftBudgetMicros), "draft", "drafts") +
		" at the worst-case cost. Each draft needs about $0.90 available before it starts; only actual usage is charged. Judging and speech share this allowance. In-flight work reduces what is available. A top-up adds " + money(cfg.TopupMicros/10000) + " more. Prices cover costs plus a small margin."

	return v
}

func (s *Server) billing(w http.ResponseWriter, r *http.Request, c *rc) error {
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
	count, err := c.us.MonthTopups(r.Context(), s.App.Now())
	if err != nil {
		return err
	}
	now := s.App.Now().UTC()
	reset := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	flash := map[string]string{
		"duplicate": "This top-up was already processed. No additional credit was added.",
		"topup":     "Topped up. " + money(s.App.Cfg.TopupMicros/10000) + " of credit was added.",
		"card":      "Your card was updated.",
		"resumed":   "Your plan continues.",
		"started":   "Welcome back. Your plan is active again.",
	}[r.URL.Query().Get("ok")]
	return s.render(w, r, c, http.StatusOK, "billing.html", map[string]any{
		"Title": "Billing", "Nav": "billing", "B": s.billingView(c, sub, ledger, reserved), "Flash": flash,
		"TopupCapped": count >= 4, "TopupReset": reset.Format("January 2, 2006") + " UTC",
		"TopupPrice": s.App.Cfg.TopupCents, "Price": s.App.Cfg.PriceCents, "TopupError": r.URL.Query().Get("declined"),
	})
}

func (s *Server) billingTopup(w http.ResponseWriter, r *http.Request, c *rc) error {
	added, err := s.App.TopUpResult(r.Context(), c.user.ID)
	if errors.Is(err, store.ErrTopupCap) {
		return back(w, r, "/billing")
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
	sub, err := c.us.Subscription(r.Context())
	if errors.Is(err, store.ErrNotFound) {
		return nil, sub, app.ErrNoPlan
	}
	if err != nil {
		return nil, sub, err
	}
	data := s.cardData(c, "Update card")
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
	token, err := s.cardToken(r)
	ok := "card"
	if err == nil {
		if s.App.HasAccess(sub) {
			err = s.App.UpdateCard(r.Context(), c.user.ID, token)
		} else {
			// The plan is over: a new card starts it again, charged today.
			if sub.Status != "ended" {
				sub.Status = "ended"
				if err = c.us.SaveSubscription(r.Context(), sub); err != nil {
					return err
				}
			}
			err, ok = s.App.StartTrial(r.Context(), c.user, token), "started"
		}
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
