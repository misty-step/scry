package app_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"scry/internal/ai"
	"scry/internal/alert"
	"scry/internal/app"
	"scry/internal/billing"
	"scry/internal/speech"
	"scry/internal/store"
)

// stripeStandIn is Stripe's API over HTTP, as far as Subscribe uses it,
// with Stripe's idempotency rules: a key returns its first result; with
// other parameters it is refused (idempotency_error), and while its first
// request is running it is refused with 409.
type stripeStandIn struct {
	t   *testing.T
	srv *httptest.Server
	now func() time.Time

	mu       sync.Mutex
	saved    map[string]standInReply // by idempotency key
	running  map[string]bool
	posts    []string // idempotency keys of POST /v1/subscriptions, in order
	created  []*standInSub
	invoices int
	declined map[string]bool // payment methods refused when charged
	drop     bool            // the subscription is created, the reply is lost
	status   int             // POST /v1/subscriptions answers this, unsaved, doing nothing
	noList   bool            // lookups fail
	hold     chan struct{}   // POST /v1/subscriptions waits for this
	held     chan struct{}   // closed when one is waiting
}

type standInReply struct {
	params string
	status int
	body   any
}

type standInSub struct {
	id, customer, key, status, invoice string
	start, end                         int64
}

type standInCustomer struct{ id, email, key string }

func newStripeStandIn(t *testing.T, now func() time.Time) (*stripeStandIn, *billing.Stripe) {
	t.Helper()
	s := &stripeStandIn{t: t, now: now, saved: map[string]standInReply{}, running: map[string]bool{}, declined: map[string]bool{}}
	var customers []standInCustomer
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, status int, body any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
	list := func(data []any) any { return map[string]any{"object": "list", "data": data, "has_more": false} }
	stripeErr := func(typ, code, msg string) any {
		return map[string]any{"error": map[string]any{"type": typ, "code": code, "message": msg}}
	}
	// once runs a POST under Stripe's idempotency rules.
	once := func(w http.ResponseWriter, r *http.Request, run func(url.Values) (int, any)) {
		_ = r.ParseForm()
		key, params := r.Header.Get("Idempotency-Key"), r.PostForm.Encode()
		// lost drops the connection instead of replying (every time: Go's
		// transport itself resends a keyed POST whose connection dropped).
		lost := func() {
			if s.drop && strings.HasSuffix(r.URL.Path, "/subscriptions") {
				panic(http.ErrAbortHandler)
			}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.running[key] {
			reply(w, 409, stripeErr("idempotency_error", "idempotency_key_in_use", "in use"))
			return
		}
		if got, ok := s.saved[key]; ok {
			if got.params != params {
				reply(w, 400, stripeErr("idempotency_error", "", "Keys for idempotent requests can only be used with the same parameters"))
				return
			}
			lost()
			reply(w, got.status, got.body)
			return
		}
		if hold := s.hold; hold != nil && strings.HasSuffix(r.URL.Path, "/subscriptions") {
			s.running[key] = true
			close(s.held)
			s.hold = nil
			s.mu.Unlock()
			<-hold
			s.mu.Lock()
			delete(s.running, key)
		}
		status, body := run(r.PostForm)
		s.saved[key] = standInReply{params, status, body}
		lost()
		reply(w, status, body)
	}
	subJSON := func(sub *standInSub) map[string]any {
		return map[string]any{"object": "subscription", "id": sub.id, "customer": sub.customer, "status": sub.status,
			"current_period_start": sub.start, "current_period_end": sub.end, "metadata": map[string]string{"scry_key": sub.key},
			"latest_invoice": map[string]any{"id": sub.invoice, "status": "paid"}}
	}
	mux.HandleFunc("POST /v1/customers", func(w http.ResponseWriter, r *http.Request) {
		once(w, r, func(f url.Values) (int, any) {
			if f.Get("payment_method") == "pm_unknown" {
				return 400, stripeErr("invalid_request_error", "resource_missing", "No such PaymentMethod")
			}
			c := standInCustomer{fmt.Sprintf("cus_%d", len(customers)+1), f.Get("email"), f.Get("metadata[scry_key]")}
			customers = append(customers, c)
			return 200, map[string]any{"id": c.id}
		})
	})
	mux.HandleFunc("POST /v1/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.posts = append(s.posts, r.Header.Get("Idempotency-Key"))
		status := s.status
		s.mu.Unlock()
		if status != 0 {
			reply(w, status, stripeErr("api_error", "", "try later"))
			return
		}
		once(w, r, func(f url.Values) (int, any) {
			now := s.now()
			sub := &standInSub{customer: f.Get("customer"), key: f.Get("metadata[scry_key]"), status: "active",
				start: now.Unix(), end: now.AddDate(0, 1, 0).Unix()}
			if te := f.Get("trial_end"); te != "" {
				end, _ := strconv.ParseInt(te, 10, 64)
				if end <= now.Unix() {
					return 400, stripeErr("invalid_request_error", "", "Invalid timestamp: must be in the future")
				}
				sub.status, sub.end = "trialing", end
			} else if f.Get("payment_behavior") != "error_if_incomplete" {
				s.t.Errorf("immediate subscription without error_if_incomplete: %v", f)
			} else if s.declined[f.Get("default_payment_method")] {
				return 402, stripeErr("card_error", "card_declined", "Your card was declined.")
			}
			sub.id = fmt.Sprintf("sub_%d", len(s.created)+1)
			sub.invoice = "in_" + sub.id
			if sub.status == "active" {
				s.invoices++
			}
			s.created = append(s.created, sub)
			return 200, subJSON(sub)
		})
	})
	mux.HandleFunc("GET /v1/customers", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.noList {
			reply(w, 503, stripeErr("api_error", "", "down"))
			return
		}
		var out []any
		for _, c := range customers {
			if c.email == r.URL.Query().Get("email") {
				out = append(out, map[string]any{"id": c.id, "metadata": map[string]string{"scry_key": c.key}})
			}
		}
		reply(w, 200, list(out))
	})
	mux.HandleFunc("GET /v1/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var out []any
		for _, sub := range s.created {
			if sub.customer == r.URL.Query().Get("customer") {
				out = append(out, subJSON(sub))
			}
		}
		reply(w, 200, list(out))
	})
	mux.HandleFunc("GET /v1/payment_methods/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"card": map[string]any{"brand": "visa", "last4": "4242"}})
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	pay, err := billing.NewStripe("sk_test_standin", "pk_test_standin", "price_standin", "whsec_standin")
	if err != nil {
		t.Fatal(err)
	}
	pay.BaseURL = s.srv.URL
	return s, pay
}

// set changes the stand-in under its lock.
func (s *stripeStandIn) set(f func()) { s.mu.Lock(); f(); s.mu.Unlock() }

// holdNext holds the next subscription POST open until release is called
// (or the test ends); held closes when it has arrived.
func (s *stripeStandIn) holdNext() (held <-chan struct{}, release func()) {
	hold, arrived := make(chan struct{}), make(chan struct{})
	release = sync.OnceFunc(func() { close(hold) })
	s.t.Cleanup(release)
	s.set(func() { s.hold, s.held = hold, arrived })
	return arrived, release
}

// keys are the distinct idempotency keys subscriptions were asked for with.
func (s *stripeStandIn) keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	seen := map[string]bool{}
	for _, k := range s.posts {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

func (s *stripeStandIn) counts() (posts, subs, invoices int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.posts), len(s.created), s.invoices
}

// newStripeTrialWorld is a real Scry (App, SQLite, the Stripe adapter over
// HTTP) whose learner is in the last 30 minutes of the no-card trial, at
// second 59 of a minute: a card added now is charged today.
func newStripeTrialWorld(t *testing.T) (*trialWorld, *stripeStandIn) {
	t.Helper()
	w := newTrialWorld(t)
	standIn, pay := newStripeStandIn(t, w.now)
	w.app.Pay = pay
	w.set(w.sub(t).PeriodEnd.Add(-30 * time.Minute).Truncate(time.Minute).Add(59 * time.Second))
	return w, standIn
}

func (w *trialWorld) attempts(t *testing.T) []store.SubscriptionAttempt {
	t.Helper()
	a, err := w.us.SubscriptionAttempts(w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// paidOnce checks the learner is on one paid plan, charged once.
func (w *trialWorld) paidOnce(t *testing.T, s *stripeStandIn) {
	t.Helper()
	if _, subs, invoices := s.counts(); subs != 1 || invoices != 1 {
		t.Fatalf("%d subscriptions and %d paid invoices at the processor, want one of each", subs, invoices)
	}
	if sub := w.sub(t); sub.Status != store.SubActive || sub.NoCard() || sub.SubscriptionRef != "sub_1" ||
		sub.AllowanceMicros != w.app.Cfg.AllowanceMicros || sub.UsedMicros != 0 {
		t.Fatalf("plan %+v", sub)
	}
	led, err := w.us.Ledger(w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	paid := 0
	for _, e := range led {
		if e.Kind == "charge" {
			paid++
			if e.AmountCents != 500 || e.Status != "paid" || e.Ref != "in_sub_1" {
				t.Fatalf("ledger %+v", e)
			}
		}
	}
	if paid != 1 {
		t.Fatalf("%d paid entries in the ledger, want 1: %+v", paid, led)
	}
	for _, a := range w.attempts(t) {
		if a.Status == store.SubscriptionAttemptPending {
			t.Fatalf("claim still pending: %+v", a)
		}
	}
}

// Slice 36 pass 3: two confirmed upgrades across a minute boundary. The
// first is still with the processor (its POST held open at second 59) when
// the second arrives at second 61. The second is refused, having sent
// nothing: one subscription, one charge, one ledger line.
func TestSlice36_ConcurrentUpgradesAcrossMinuteBoundary(t *testing.T) {
	w, s := newStripeTrialWorld(t)
	held, release := s.holdNext()
	first := make(chan error, 1)
	go func() { first <- w.app.StartTrial(w.ctx, w.user, "pm_first", app.TermsImmediate) }()
	<-held
	w.set(w.now().Add(2 * time.Second)) // second 61: the next minute
	before := w.sub(t)
	if err := w.app.StartTrial(w.ctx, w.user, "pm_second", app.TermsImmediate); !errors.Is(err, app.ErrSubscribeInProgress) {
		t.Fatalf("second upgrade = %v, want ErrSubscribeInProgress", err)
	}
	if posts, _, _ := s.counts(); posts != 1 {
		t.Fatalf("%d subscription POSTs while the first is in flight, want 1", posts)
	}
	if got := w.sub(t); got != before || !got.NoCardTrial() {
		t.Fatalf("the refused request changed the plan: %+v", got)
	}
	release()
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	w.paidOnce(t, s)
	// A submit after it is done changes nothing more.
	if err := w.app.StartTrial(w.ctx, w.user, "pm_second", app.TermsImmediate); err != nil {
		t.Fatal(err)
	}
	if posts, _, _ := s.counts(); posts != 1 {
		t.Fatalf("%d subscription POSTs in all, want 1", posts)
	}
	w.paidOnce(t, s)
}

// Slice 36 pass 3: the same, from two processes sharing the database (a
// deploy overlapping): the in-process guard cannot see the other, but the
// durable claim makes both use one key, which the processor answers once.
func TestSlice36_ConcurrentUpgradesInTwoProcessesShareOneKey(t *testing.T) {
	w, s := newStripeTrialWorld(t)
	other := &app.App{Store: w.st, Pay: w.app.Pay, Alert: w.app.Alert, Log: w.app.Log, Now: w.app.Now, Cfg: w.app.Cfg,
		Model: &ai.Fake{}, Speech: speech.Fake{}}
	held, release := s.holdNext()
	first := make(chan error, 1)
	go func() { first <- w.app.StartTrial(w.ctx, w.user, "pm_first", app.TermsImmediate) }()
	<-held
	w.set(w.now().Add(2 * time.Second))
	if err := other.StartTrial(w.ctx, w.user, "pm_second", app.TermsImmediate); !errors.Is(err, app.ErrSubscribeInProgress) {
		t.Fatalf("second process = %v, want ErrSubscribeInProgress", err)
	}
	release()
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if keys := s.keys(); len(keys) != 1 {
		t.Fatalf("two processes sent keys %v", keys)
	}
	w.paidOnce(t, s)
}

// Slice 36 pass 3: many simultaneous submissions charge once.
func TestSlice36_ManySimultaneousUpgradesChargeOnce(t *testing.T) {
	w, s := newStripeTrialWorld(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := w.app.StartTrial(w.ctx, w.user, fmt.Sprintf("pm_%d", i), app.TermsImmediate); err != nil && !errors.Is(err, app.ErrSubscribeInProgress) {
				t.Errorf("upgrade %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	w.paidOnce(t, s)
}

// Slice 36 pass 3: the processor took the money but its reply was lost
// before anything was recorded. A retry two minutes later, with a newly
// entered card, repeats the same key and parameters: the processor returns
// what it already did, and the plan is recorded, charged once.
func TestSlice36_LostPaidResponseReconcilesWithoutDoubleCharge(t *testing.T) {
	w, s := newStripeTrialWorld(t)
	before := w.sub(t)
	s.drop = true
	if err := w.app.StartTrial(w.ctx, w.user, "pm_first", app.TermsImmediate); err == nil {
		t.Fatal("a lost reply was reported as success")
	}
	if _, subs, invoices := s.counts(); subs != 1 || invoices != 1 {
		t.Fatalf("stand-in: %d subscriptions, %d invoices", subs, invoices)
	}
	if got := w.sub(t); got != before {
		t.Fatalf("plan changed before the outcome was known: %+v", got)
	}
	if _, err := w.app.Plan(w.ctx, w.user.ID); err != nil {
		t.Fatalf("trial lost access after a lost reply: %v", err)
	}
	if a := w.attempts(t); len(a) != 1 || a[0].Status != store.SubscriptionAttemptPending || a[0].PaymentToken != "pm_first" {
		t.Fatalf("attempts %+v", a)
	}
	s.set(func() { s.drop = false })
	w.set(w.now().Add(2 * time.Minute))
	if err := w.app.StartTrial(w.ctx, w.user, "pm_second", app.TermsImmediate); err != nil {
		t.Fatal(err)
	}
	if keys := s.keys(); len(keys) != 1 {
		t.Fatalf("the retry sent a different key: %v", keys)
	}
	if posts, _, _ := s.counts(); posts < 2 {
		t.Fatalf("%d subscription POSTs: the retry was never sent", posts)
	}
	w.paidOnce(t, s)
	if a := w.attempts(t); len(a) != 1 || a[0].Status != store.SubscriptionAttemptSucceeded || a[0].SubscriptionRef != "sub_1" {
		t.Fatalf("attempts %+v", a)
	}
}

// Slice 36 pass 3: the same for a card added inside the trial (nothing
// charged today): the retry keeps the trial end first asked for, and the
// processor has one subscription.
func TestSlice36_LostTrialCardResponseReconciles(t *testing.T) {
	w, s := newStripeTrialWorld(t)
	before := w.sub(t)
	w.set(before.PeriodEnd.Add(-48 * time.Hour))
	s.drop = true
	if err := w.app.StartTrial(w.ctx, w.user, "pm_first", app.TermsTrial); err == nil {
		t.Fatal("a lost reply was reported as success")
	}
	s.set(func() { s.drop = false })
	w.set(w.now().Add(2 * time.Minute))
	if err := w.app.StartTrial(w.ctx, w.user, "pm_second", app.TermsTrial); err != nil {
		t.Fatal(err)
	}
	if _, subs, invoices := s.counts(); subs != 1 || invoices != 0 || len(s.keys()) != 1 {
		t.Fatalf("%d subscriptions, %d invoices, keys %v", subs, invoices, s.keys())
	}
	if got := w.sub(t); got.Status != store.SubTrialing || got.NoCard() || !got.PeriodEnd.Equal(before.PeriodEnd) || got.CustomerRef != "cus_1" {
		t.Fatalf("plan %+v", got)
	}
}

// Slice 36 pass 3: a paid plan and the cleared claim are one transaction.
// If the plan cannot be recorded, the claim stays pending and nothing of
// the plan is written; the retry repeats the key and both are recorded.
func TestSlice36_PlanAndClaimCommitTogether(t *testing.T) {
	w, s := newStripeTrialWorld(t)
	before := w.sub(t)
	if _, err := w.st.DB().Exec(`CREATE TRIGGER no_charge BEFORE INSERT ON ledger WHEN NEW.kind = 'charge' BEGIN SELECT RAISE(ABORT, 'disk full'); END`); err != nil {
		t.Fatal(err)
	}
	if err := w.app.StartTrial(w.ctx, w.user, "pm_first", app.TermsImmediate); err == nil {
		t.Fatal("an unrecorded plan was reported as success")
	}
	if got := w.sub(t); got != before {
		t.Fatalf("half a plan was recorded: %+v", got)
	}
	if a := w.attempts(t); len(a) != 1 || a[0].Status != store.SubscriptionAttemptPending {
		t.Fatalf("claim cleared without its plan: %+v", a)
	}
	if _, err := w.st.DB().Exec(`DROP TRIGGER no_charge`); err != nil {
		t.Fatal(err)
	}
	w.set(w.now().Add(5 * time.Minute))
	if err := w.app.StartTrial(w.ctx, w.user, "pm_second", app.TermsImmediate); err != nil {
		t.Fatal(err)
	}
	if keys := s.keys(); len(keys) != 1 {
		t.Fatalf("keys %v", keys)
	}
	w.paidOnce(t, s)
}

// Slice 36 pass 3: a declined card (402) settles its attempt as failed and
// touches nothing else: the no-card trial still gives access, and a good
// card is then tried under a new key and charged once.
func TestSlice36_DeclinedUpgradeClearsPendingAndPreservesTrial(t *testing.T) {
	w, s := newStripeTrialWorld(t)
	before := w.sub(t)
	s.declined["pm_bad"] = true
	err := w.app.StartTrial(w.ctx, w.user, "pm_bad", app.TermsImmediate)
	var d *billing.DeclineError
	if !errors.As(err, &d) || d.Reason != "Your card was declined." {
		t.Fatalf("declined upgrade = %v", err)
	}
	if a := w.attempts(t); len(a) != 1 || a[0].Status != store.SubscriptionAttemptFailed || a[0].Settled.IsZero() {
		t.Fatalf("attempts %+v", a)
	}
	if got := w.sub(t); got != before || !got.NoCardTrial() {
		t.Fatalf("a declined card changed the trial:\n%+v\n%+v", before, got)
	}
	if _, err := w.app.Plan(w.ctx, w.user.ID); err != nil {
		t.Fatalf("trial lost access: %v", err)
	}
	if err := w.us.ReserveAt(w.ctx, "after-decline", w.app.JudgeBudget(), w.now(), 7*24*time.Hour); err != nil {
		t.Fatalf("hold after a decline: %v", err)
	}
	if err := w.us.Settle(w.ctx, "after-decline", store.Usage{Kind: "judge"}, w.now()); err != nil {
		t.Fatal(err)
	}
	if err := w.app.StartTrial(w.ctx, w.user, "pm_good", app.TermsImmediate); err != nil {
		t.Fatal(err)
	}
	if keys := s.keys(); len(keys) != 2 {
		t.Fatalf("the good card reused the declined key: %v", keys)
	}
	w.paidOnce(t, s)
	if a := w.attempts(t); len(a) != 2 || a[1].Status != store.SubscriptionAttemptSucceeded || a[0].Key == a[1].Key {
		t.Fatalf("attempts %+v", a)
	}
}

// Slice 36 pass 3: a request the processor refuses as invalid did nothing
// either; and a reply lost on a declined card does not leave the learner
// stuck on that card: the repeat reports the decline, and the card just
// entered is tried at once under a new key.
func TestSlice36_RefusedAttemptsNeverStickTheLearner(t *testing.T) {
	w, s := newStripeTrialWorld(t)
	if err := w.app.StartTrial(w.ctx, w.user, "pm_unknown", app.TermsImmediate); err == nil {
		t.Fatal("unknown payment method accepted")
	}
	if a := w.attempts(t); len(a) != 1 || a[0].Status != store.SubscriptionAttemptFailed {
		t.Fatalf("attempts %+v", a)
	}
	s.declined["pm_bad"] = true
	s.drop = true
	if err := w.app.StartTrial(w.ctx, w.user, "pm_bad", app.TermsImmediate); err == nil {
		t.Fatal("lost reply reported as success")
	}
	if a := w.attempts(t); len(a) != 2 || a[1].Status != store.SubscriptionAttemptPending {
		t.Fatalf("attempts %+v", a)
	}
	s.set(func() { s.drop = false })
	w.set(w.now().Add(2 * time.Minute))
	if err := w.app.StartTrial(w.ctx, w.user, "pm_good", app.TermsImmediate); err != nil {
		t.Fatal(err)
	}
	w.paidOnce(t, s)
	if a := w.attempts(t); len(a) != 3 || a[1].Status != store.SubscriptionAttemptFailed || a[2].PaymentToken != "pm_good" {
		t.Fatalf("attempts %+v", a)
	}
}

// Slice 36 pass 3: an attempt with no definite answer (a 500, a 429, a
// refused credential) stays pending and keeps its key.
func TestSlice36_IndefiniteAnswersKeepTheKey(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			w, s := newStripeTrialWorld(t)
			s.status = status
			if err := w.app.StartTrial(w.ctx, w.user, "pm_first", app.TermsImmediate); err == nil {
				t.Fatal("no error")
			}
			if a := w.attempts(t); len(a) != 1 || a[0].Status != store.SubscriptionAttemptPending {
				t.Fatalf("attempts %+v", a)
			}
			s.set(func() { s.status = 0 })
			w.set(w.now().Add(90 * time.Second))
			if err := w.app.StartTrial(w.ctx, w.user, "pm_second", app.TermsImmediate); err != nil {
				t.Fatal(err)
			}
			if keys := s.keys(); len(keys) != 1 {
				t.Fatalf("keys %v", keys)
			}
			w.paidOnce(t, s)
		})
	}
}

// Slice 36 pass 3: a repeat the processor keeps failing (a 500 it answers
// for a day) does not hide a subscription that exists: it is looked up and
// recorded.
func TestSlice36_FailingRepeatIsLookedUp(t *testing.T) {
	w, s := newStripeTrialWorld(t)
	s.drop = true
	if err := w.app.StartTrial(w.ctx, w.user, "pm_first", app.TermsImmediate); err == nil {
		t.Fatal("a lost reply was reported as success")
	}
	s.set(func() { s.drop, s.status, s.noList = false, 500, true })
	w.set(w.now().Add(2 * time.Minute))
	if err := w.app.StartTrial(w.ctx, w.user, "pm_second", app.TermsImmediate); err == nil {
		t.Fatal("no answer was reported as success")
	}
	if a := w.attempts(t); len(a) != 1 || a[0].Status != store.SubscriptionAttemptPending {
		t.Fatalf("attempts %+v", a)
	}
	s.set(func() { s.noList = false })
	if err := w.app.StartTrial(w.ctx, w.user, "pm_second", app.TermsImmediate); err != nil {
		t.Fatal(err)
	}
	if keys := s.keys(); len(keys) != 1 {
		t.Fatalf("keys %v", keys)
	}
	w.paidOnce(t, s)
}

// Slice 36 pass 3: the processor forgets idempotency keys after a day, so
// a repeat then would be a second charge. A retry that late looks up what
// became of the attempt instead, and records it without asking again. If
// the lookup fails nothing is sent; only a complete lookup that has
// nothing releases the claim for a new attempt.
func TestSlice36_RetryAfterTheKeyIsForgottenLooksUpInsteadOfCharging(t *testing.T) {
	w, s := newStripeTrialWorld(t)
	s.drop = true
	if err := w.app.StartTrial(w.ctx, w.user, "pm_first", app.TermsImmediate); err == nil {
		t.Fatal("a lost reply was reported as success")
	}
	s.set(func() { s.drop, s.saved, s.noList = false, map[string]standInReply{}, true })
	sent, _, _ := s.counts()
	w.set(w.now().Add(25 * time.Hour))
	if err := w.app.StartTrial(w.ctx, w.user, "pm_second", app.TermsImmediate); err == nil {
		t.Fatal("an inconclusive lookup was reported as success")
	}
	s.set(func() { s.noList = false })
	if err := w.app.StartTrial(w.ctx, w.user, "pm_third", app.TermsImmediate); err != nil {
		t.Fatal(err)
	}
	if posts, _, _ := s.counts(); posts != sent {
		t.Fatalf("a forgotten key was sent again: %d POSTs, then %d", sent, posts)
	}
	w.paidOnce(t, s)

	t.Run("nothing there", func(t *testing.T) {
		w, s := newStripeTrialWorld(t)
		s.status = 503
		if err := w.app.StartTrial(w.ctx, w.user, "pm_first", app.TermsImmediate); err == nil {
			t.Fatal("no error")
		}
		s.set(func() { s.status = 0; s.saved = map[string]standInReply{} })
		w.set(w.now().Add(25 * time.Hour))
		if err := w.app.StartTrial(w.ctx, w.user, "pm_first", app.TermsImmediate); err != nil {
			t.Fatal(err)
		}
		if keys := s.keys(); len(keys) != 2 {
			t.Fatalf("keys %v", keys)
		}
		if _, subs, invoices := s.counts(); subs != 1 || invoices != 1 || w.sub(t).Status != store.SubActive {
			t.Fatalf("%d subscriptions, %d invoices, %+v", subs, invoices, w.sub(t))
		}
	})
}

// Slice 36 pass 3: a card added inside the trial whose reply was lost, and
// not retried until the trial is over. The processor has charged it by
// then (or tried to, or ended it): a confirmed upgrade records that rather
// than opening a second plan.
func TestSlice36_LostTrialCardFoundAfterTheTrial(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   store.SubscriptionStatus
		subs   int
		paid   int // charge lines in the ledger
	}{
		{"active", store.SubActive, 1, 1},
		{"past_due", store.SubPastDue, 1, 0},
		{"canceled", store.SubActive, 2, 1},
	} {
		t.Run(tc.status, func(t *testing.T) {
			w, s := newStripeTrialWorld(t)
			end := w.sub(t).PeriodEnd
			w.set(end.Add(-2 * time.Hour))
			s.drop = true
			if err := w.app.StartTrial(w.ctx, w.user, "pm_first", app.TermsTrial); err == nil {
				t.Fatal("a lost reply was reported as success")
			}
			w.set(end.Add(time.Hour))
			s.set(func() {
				s.drop = false
				c := s.created[0]
				c.status, c.start, c.end = tc.status, end.Unix(), end.AddDate(0, 1, 0).Unix()
			})
			if err := w.app.StartTrial(w.ctx, w.user, "pm_second", app.TermsTrial); !errors.Is(err, app.ErrTermsChanged) {
				t.Fatalf("unconfirmed charge = %v", err)
			}
			if err := w.app.StartTrial(w.ctx, w.user, "pm_second", app.TermsImmediate); err != nil {
				t.Fatal(err)
			}
			if _, subs, _ := s.counts(); subs != tc.subs {
				t.Fatalf("%d subscriptions at the processor, want %d", subs, tc.subs)
			}
			got := w.sub(t)
			if got.Status != tc.want || got.NoCard() || got.SubscriptionRef != fmt.Sprintf("sub_%d", tc.subs) {
				t.Fatalf("plan %+v", got)
			}
			if _, err := w.app.Plan(w.ctx, w.user.ID); err != nil {
				t.Fatalf("no access: %v", err)
			}
			paid := 0
			led, _ := w.us.Ledger(w.ctx)
			for _, e := range led {
				if e.Kind == "charge" && e.Status == "paid" {
					paid++
				}
			}
			if paid != tc.paid {
				t.Fatalf("%d paid lines, want %d: %+v", paid, tc.paid, led)
			}
			if a := w.attempts(t); a[len(a)-1].Status != store.SubscriptionAttemptSucceeded {
				t.Fatalf("attempts %+v", a)
			}
		})
	}
}

// Slice 36 pass 3: the database holds at most one pending attempt for a
// learner, whoever writes it, and learners do not block one another.
func TestSlice36_OnePendingAttemptPerLearner(t *testing.T) {
	w, _ := newStripeTrialWorld(t)
	other, err := w.app.CreateUser(w.ctx, "other@example.com", "other-subject", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	ins := `INSERT INTO subscription_attempts (user_id, idem_key, status, email, payment_token, created_at) VALUES (?, ?, ?, '', '', 1)`
	for _, q := range []struct {
		user   int64
		key    string
		status string
		ok     bool
	}{
		{w.user.ID, "k1", "pending", true},
		{w.user.ID, "k2", "pending", false}, // a second claim
		{w.user.ID, "k3", "failed", true},
		{w.user.ID, "k4", "succeeded", true},
		{w.user.ID, "k5", "bogus", false},
		{other.ID, "k1", "failed", false}, // a key is one attempt's
		{other.ID, "k6", "pending", true},
	} {
		if _, err := w.st.DB().Exec(ins, q.user, q.key, q.status); (err == nil) != q.ok {
			t.Errorf("%+v: %v", q, err)
		}
	}
}

// Slice 36 pass 3: a database from before subscription_attempts (a
// production one, with a learner mid-trial and one on a paid plan) opens,
// gains the table and its index, keeps every row, and upgrades a learner.
// Opening it again changes nothing; a snapshot from before restores.
func TestSlice36_ExistingDBMigratesCleanly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prod.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)
	w := &trialWorld{ctx: t.Context(), st: st, clock: now}
	cfg := app.DefaultConfig()
	cfg.TestMode = true
	standIn, pay := newStripeStandIn(t, w.now)
	newApp := func(st *store.Store) *app.App {
		return &app.App{Store: st, Pay: pay, Model: &ai.Fake{}, Speech: speech.Fake{}, Alert: &alert.Recorder{},
			Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: w.now, Cfg: cfg}
	}
	w.app = newApp(st)
	if w.user, err = w.app.CreateUser(w.ctx, "trial@example.com", "trial-subject", "UTC"); err != nil {
		t.Fatal(err)
	}
	payer, err := w.app.CreateUser(w.ctx, "payer@example.com", "payer-subject", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.For(payer.ID).SaveSubscription(w.ctx, store.Subscription{UserID: payer.ID, CustomerRef: "cus_prod", SubscriptionRef: "sub_prod",
		PaymentRef: "pm_prod", Status: store.SubActive, PeriodStart: now, PeriodEnd: now.AddDate(0, 1, 0), AllowanceMicros: 3_000_000, UsedMicros: 42}); err != nil {
		t.Fatal(err)
	}
	snap, err := st.Snapshot(w.ctx, filepath.Join(dir, "snaps"), now)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	dump := func(db *sql.DB) string {
		t.Helper()
		var b strings.Builder
		for _, table := range []string{"users", "subscriptions", "ledger", "topups", "processed_events"} {
			rows, err := db.Query(`SELECT * FROM ` + table)
			if err != nil {
				t.Fatal(err)
			}
			cols, _ := rows.Columns()
			for rows.Next() {
				vals := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range vals {
					ptrs[i] = &vals[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					t.Fatal(err)
				}
				fmt.Fprintln(&b, table, cols, vals)
			}
			rows.Close()
		}
		return b.String()
	}
	// Make the database and the snapshot what they were before this change.
	var before string
	for _, p := range []string{snap, path} {
		db, err := sql.Open("sqlite", "file:"+p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`DROP TABLE subscription_attempts`); err != nil {
			t.Fatal(err)
		}
		before = dump(db)
		db.Close()
	}
	for i := 0; i < 2; i++ { // the second open is a no-op
		if st, err = store.Open(path); err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		if after := dump(st.DB()); after != before {
			t.Fatalf("rows changed by the migration:\n%s\n%s", before, after)
		}
		for _, o := range [][2]string{{"table", "subscription_attempts"}, {"index", "subscription_attempts_pending"}} {
			var n int
			if err := st.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = ? AND name = ?`, o[0], o[1]).Scan(&n); err != nil || n != 1 {
				t.Fatalf("%v missing: %d %v", o, n, err)
			}
		}
		if err := st.Integrity(w.ctx); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			st.Close()
		}
	}
	defer st.Close()
	w.st, w.us, w.app = st, st.For(w.user.ID), newApp(st)
	w.set(w.sub(t).PeriodEnd.Add(-30 * time.Minute))
	if err := w.app.StartTrial(w.ctx, w.user, "pm_good", app.TermsImmediate); err != nil {
		t.Fatal(err)
	}
	w.paidOnce(t, standIn)
	if a := w.attempts(t); len(a) != 1 || a[0].Status != store.SubscriptionAttemptSucceeded {
		t.Fatalf("attempts %+v", a)
	}
	if _, err := st.RestoreUser(w.ctx, snap, "trial@example.com"); err != nil {
		t.Fatalf("restore from a snapshot without the table: %v", err)
	}
	if got := w.sub(t); !got.NoCardTrial() {
		t.Fatalf("restored plan %+v", got)
	}
}
