package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"scry/internal/safeerr"
	"strconv"
	"strings"
	"time"
)

// Stripe is the real processor. Each adapter is pinned to one Stripe mode,
// chosen explicitly at boot: a test-mode adapter accepts only test keys and
// test-mode webhooks, a live-mode adapter only live keys and live-mode
// webhooks. A key or event from the other mode is refused loudly, so a
// misconfigured deploy cannot charge real cards from a test build or record
// test events as real money. Cards are collected in the browser by
// Stripe.js; Scry only ever sees a payment method id.
type Stripe struct {
	secret, publishable, priceID, webhookSecret string
	live                                        bool
	BaseURL                                     string
	HTTP                                        *http.Client
}

// stripeVersion pins the API shape this adapter was written against.
const stripeVersion = "2024-06-20"

// NewStripe builds a test-mode adapter. It is NewStripeWithMode(false, ...).
func NewStripe(secret, publishable, priceID, webhookSecret string) (*Stripe, error) {
	return NewStripeWithMode(false, secret, publishable, priceID, webhookSecret)
}

func stripeModeName(live bool) string {
	if live {
		return "live"
	}
	return "test"
}

// NewStripeWithMode builds the adapter from credentials read from the
// environment, for live mode (live=true) or test mode. Keys must belong to
// the chosen mode.
func NewStripeWithMode(live bool, secret, publishable, priceID, webhookSecret string) (*Stripe, error) {
	want, other := stripeModeName(live), stripeModeName(!live)
	if strings.HasPrefix(secret, "sk_"+other+"_") || strings.HasPrefix(secret, "rk_"+other+"_") {
		return nil, fmt.Errorf("STRIPE_SECRET_KEY is a %s-mode key but Stripe is configured for %s mode: refusing to start", other, want)
	}
	if !strings.HasPrefix(secret, "sk_"+want+"_") && !strings.HasPrefix(secret, "rk_"+want+"_") {
		return nil, fmt.Errorf("STRIPE_SECRET_KEY must be a %s-mode key (sk_%s_… or rk_%s_…)", want, want, want)
	}
	if strings.HasPrefix(publishable, "pk_"+other+"_") {
		return nil, fmt.Errorf("STRIPE_PUBLISHABLE_KEY is a %s-mode key but Stripe is configured for %s mode: refusing to start", other, want)
	}
	if !strings.HasPrefix(publishable, "pk_"+want+"_") {
		return nil, fmt.Errorf("STRIPE_PUBLISHABLE_KEY must be a %s-mode key (pk_%s_…)", want, want)
	}
	if priceID == "" {
		return nil, errors.New("STRIPE_PRICE_ID is required: the monthly price of the plan")
	}
	if webhookSecret == "" {
		return nil, errors.New("STRIPE_WEBHOOK_SECRET is required: renewals arrive by webhook")
	}
	return &Stripe{secret: secret, publishable: publishable, priceID: priceID, webhookSecret: webhookSecret, live: live}, nil
}

// Name is "stripe-live" or "stripe-test", the mode the adapter is pinned to.
func (s *Stripe) Name() string { return "stripe-" + stripeModeName(s.live) }

// Live reports whether the adapter moves real money.
func (s *Stripe) Live() bool             { return s.live }
func (s *Stripe) PublishableKey() string { return s.publishable }

func (s *Stripe) Tokenize(string, string, string, time.Time) (string, error) {
	return "", errors.New("card details are collected by Stripe.js, never by Scry")
}

type stripeError struct {
	Error struct {
		Type    string `json:"type"`
		Code    string `json:"code"`
		Message string `json:"message"`
		// The PaymentIntent the error is about, when there is one (a
		// declined off-session confirm names the intent it failed).
		PaymentIntent struct {
			ID string `json:"id"`
		} `json:"payment_intent"`
	} `json:"error"`
}

func (s *Stripe) do(ctx context.Context, method, path string, form url.Values, key string, out any) (retErr error) {
	defer func() { retErr = safeerr.Redact(retErr, s.secret) }()
	base := s.BaseURL
	if base == "" {
		base = "https://api.stripe.com"
	}
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.secret, "")
	req.Header.Set("Stripe-Version", stripeVersion)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("card processor unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var se stripeError
		_ = json.Unmarshal(raw, &se)
		if se.Error.Type == "card_error" && se.Error.Message != "" {
			return &DeclineError{Reason: se.Error.Message, Ref: se.Error.PaymentIntent.ID}
		}
		return &ProcessorError{Status: resp.StatusCode, Type: se.Error.Type, Code: se.Error.Code, Message: se.Error.Message, Ref: se.Error.PaymentIntent.ID}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

type stripeSub struct {
	ID                 string `json:"id"`
	Status             string `json:"status"`
	CurrentPeriodStart int64  `json:"current_period_start"`
	CurrentPeriodEnd   int64  `json:"current_period_end"`
	LatestInvoice      struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Charge string `json:"charge"`
	} `json:"latest_invoice"`
}

func (s *Stripe) card(ctx context.Context, pm string) (Card, error) {
	var out struct {
		Card struct {
			Brand string `json:"brand"`
			Last4 string `json:"last4"`
		} `json:"card"`
	}
	if err := s.do(ctx, http.MethodGet, "/v1/payment_methods/"+url.PathEscape(pm), nil, "", &out); err != nil {
		return Card{}, err
	}
	brand := out.Card.Brand
	if brand != "" {
		brand = strings.ToUpper(brand[:1]) + brand[1:]
	}
	return Card{Brand: brand, Last4: out.Card.Last4}, nil
}

func (s *Stripe) Subscribe(ctx context.Context, p SubscribeParams) (Subscribed, error) {
	var cus struct {
		ID string `json:"id"`
	}
	// Attaching the card to a new customer validates it with the bank.
	err := s.do(ctx, http.MethodPost, "/v1/customers", url.Values{
		"email":          {p.Email},
		"payment_method": {p.CardToken},
		"invoice_settings[default_payment_method]": {p.CardToken},
		// The key is also kept on the customer and the subscription, so a
		// lost reply can be looked up later (FindSubscription).
		"metadata[scry_key]": {p.Key},
	}, p.Key+"-cus", &cus)
	if err != nil {
		return Subscribed{}, err
	}
	form := url.Values{
		"customer":               {cus.ID},
		"items[0][price]":        {s.priceID},
		"default_payment_method": {p.CardToken},
		"expand[]":               {"latest_invoice"},
		"metadata[scry_key]":     {p.Key},
	}
	if p.TrialEnd.IsZero() {
		form.Set("payment_behavior", "error_if_incomplete")
	} else {
		form.Set("trial_end", strconv.FormatInt(p.TrialEnd.Unix(), 10))
	}
	var sub stripeSub
	if err := s.do(ctx, http.MethodPost, "/v1/subscriptions", form, p.Key+"-sub", &sub); err != nil {
		return Subscribed{}, err
	}
	if cus.ID == "" || sub.ID == "" {
		return Subscribed{}, errors.New("card processor: subscription answer names no subscription")
	}
	out := s.subscribed(ctx, cus.ID, p.CardToken, sub)
	if p.TrialEnd.IsZero() {
		out.ChargeRef = sub.LatestInvoice.ID
	}
	return out, nil
}

// subscribed is a subscription that exists at Stripe. The card's brand and
// last digits are only for show: the plan is open (and may be paid for)
// whether or not they can be read, so failing to read them is not a failure.
func (s *Stripe) subscribed(ctx context.Context, customer, pm string, sub stripeSub) Subscribed {
	card, err := s.card(ctx, pm)
	if err != nil {
		card = Card{}
	}
	return Subscribed{CustomerRef: customer, SubscriptionRef: sub.ID, PaymentRef: pm, Card: card,
		PeriodStart: time.Unix(sub.CurrentPeriodStart, 0).UTC(), PeriodEnd: time.Unix(sub.CurrentPeriodEnd, 0).UTC()}
}

// state maps a Stripe subscription status; "" is one this adapter does not
// know, which settles nothing.
func (sub stripeSub) state() SubscriptionState {
	switch sub.Status {
	case "trialing":
		return SubscriptionTrialing
	case "active":
		return SubscriptionPaid
	case "past_due", "unpaid", "incomplete":
		return SubscriptionUnpaid
	case "canceled", "incomplete_expired":
		return SubscriptionOver
	}
	return ""
}

// FindSubscription looks up what became of a Subscribe: it lists the
// customers with its email created since then (a list, not search, so it
// is strongly consistent), and the subscriptions of those carrying its key
// in metadata. An open subscription is preferred over one that is over.
func (s *Stripe) FindSubscription(ctx context.Context, q SubscribeParams, since time.Time) (SubscriptionState, Subscribed, error) {
	type object struct {
		ID       string            `json:"id"`
		Metadata map[string]string `json:"metadata"`
		stripeSub
	}
	// list walks every page, or fails: a partial list proves nothing.
	list := func(path string, form url.Values) ([]stripeSub, error) {
		form.Set("limit", "100")
		var out []stripeSub
		for page := 0; page < findPages; page++ {
			var l struct {
				Object string   `json:"object"`
				Data   []object `json:"data"`
				More   bool     `json:"has_more"`
			}
			if err := s.do(ctx, http.MethodGet, path+"?"+form.Encode(), nil, "", &l); err != nil {
				return nil, err
			}
			if l.Object != "list" {
				break // not a list answer at all: no evidence of absence
			}
			for _, o := range l.Data {
				if o.Metadata["scry_key"] == q.Key {
					o.stripeSub.ID = o.ID
					out = append(out, o.stripeSub)
				}
			}
			if !l.More {
				return out, nil
			}
			if len(l.Data) == 0 {
				break
			}
			form.Set("starting_after", l.Data[len(l.Data)-1].ID)
		}
		return nil, ErrLookupIncomplete
	}
	customers, err := list("/v1/customers", url.Values{"email": {q.Email},
		"created[gte]": {strconv.FormatInt(since.Add(-10*time.Minute).Unix(), 10)}})
	if err != nil {
		return "", Subscribed{}, err
	}
	state, out := SubscriptionNotFound, Subscribed{}
	for _, cus := range customers {
		subs, err := list("/v1/subscriptions", url.Values{"customer": {cus.ID}, "status": {"all"}, "expand[]": {"data.latest_invoice"}})
		if err != nil {
			return "", Subscribed{}, err
		}
		for _, sub := range subs {
			st := sub.state()
			if st == "" {
				return "", Subscribed{}, fmt.Errorf("card processor: subscription %s is %q", sub.ID, sub.Status)
			}
			state, out = st, s.subscribed(ctx, cus.ID, q.CardToken, sub)
			if st == SubscriptionPaid {
				out.ChargeRef = sub.LatestInvoice.ID
			}
			if st != SubscriptionOver {
				return state, out, nil
			}
		}
	}
	return state, out, nil
}

func (s *Stripe) UpdateCard(ctx context.Context, customerRef, subscriptionRef, cardToken string) (string, Card, error) {
	if err := s.do(ctx, http.MethodPost, "/v1/payment_methods/"+url.PathEscape(cardToken)+"/attach",
		url.Values{"customer": {customerRef}}, "", nil); err != nil {
		return "", Card{}, err
	}
	if err := s.do(ctx, http.MethodPost, "/v1/customers/"+url.PathEscape(customerRef),
		url.Values{"invoice_settings[default_payment_method]": {cardToken}}, "", nil); err != nil {
		return "", Card{}, err
	}
	var sub stripeSub
	if err := s.do(ctx, http.MethodPost, "/v1/subscriptions/"+url.PathEscape(subscriptionRef),
		url.Values{"default_payment_method": {cardToken}, "expand[]": {"latest_invoice"}}, "", &sub); err != nil {
		return "", Card{}, err
	}
	// Retry the unpaid period now; the result also arrives by webhook.
	if sub.LatestInvoice.Status == "open" {
		if err := s.do(ctx, http.MethodPost, "/v1/invoices/"+url.PathEscape(sub.LatestInvoice.ID)+"/pay",
			url.Values{"payment_method": {cardToken}}, "", nil); err != nil {
			return "", Card{}, err
		}
	}
	card, err := s.card(ctx, cardToken)
	return cardToken, card, err
}

// stripeIntent is the part of a PaymentIntent reconciliation reads.
type stripeIntent struct {
	ID               string            `json:"id"`
	Status           string            `json:"status"`
	Customer         string            `json:"customer"`
	Metadata         map[string]string `json:"metadata"`
	LastPaymentError struct {
		Message string `json:"message"`
	} `json:"last_payment_error"`
}

// declinedSentence is DeclinedSentence, kept for the adapter's tests.
const declinedSentence = DeclinedSentence

// result maps a PaymentIntent's status to what became of the charge, by
// the billing state machine (ClassifyPaymentIntent): only succeeded is
// paid, and only canceled and requires_payment_method are final refusals.
// Every other status (processing, requires_capture, requires_action,
// requires_confirmation, or one this adapter does not know) is not final:
// the charge is pending, keeps its intent id, and is settled later by
// lookup or webhook, never declined early.
func (pi stripeIntent) result() ChargeResult {
	st, reason := ClassifyPaymentIntent(PaymentIntentStatus(pi.Status), pi.LastPaymentError.Message)
	return ChargeResult{Status: st, Ref: pi.ID, Reason: reason}
}

func (s *Stripe) Charge(ctx context.Context, customerRef, paymentRef string, amountCents int64, description, key string) (string, error) {
	var pi stripeIntent
	err := s.do(ctx, http.MethodPost, "/v1/payment_intents", url.Values{
		"amount":         {strconv.FormatInt(amountCents, 10)},
		"currency":       {"usd"},
		"customer":       {customerRef},
		"payment_method": {paymentRef},
		"off_session":    {"true"},
		"confirm":        {"true"},
		"description":    {description},
		// The key is also kept on the PaymentIntent, so a lost reply can be
		// reconciled by looking the intent up (FindCharge), and its webhooks
		// can be matched to the attempt.
		"metadata[scry_key]": {key},
	}, key, &pi)
	if err != nil {
		return "", err
	}
	// Paid is nil; a refusal naming a card fault is a *DeclineError; a
	// cancellation with none is a *CanceledError (nothing was taken, the
	// card is not at fault: released like a lookup that finds it canceled);
	// an intent accepted but not finished (processing, requires_capture,
	// requires_action, requires_confirmation) is a *PendingError. The id is
	// always returned so the attempt can be settled later.
	return pi.ID, ChargeOutcome(PaymentIntentStatus(pi.Status), pi.ID, pi.LastPaymentError.Message)
}

// findPages bounds how far FindCharge lists. A list that is still
// truncated after this many pages is an incomplete lookup, never absence.
const findPages = 20

// FindCharge reconciles a top-up whose outcome is not known. With the
// PaymentIntent's id it retrieves that intent directly. Without it (the
// reply was lost) it lists the attempt's own customer's PaymentIntents
// created since the attempt began (a list, not search, so it is strongly
// consistent) and matches the Scry idempotency key kept in metadata. It
// reports ChargeNotFound only after a complete scan, and that only means
// "not visible yet". It never creates or confirms anything.
func (s *Stripe) FindCharge(ctx context.Context, q ChargeQuery) (ChargeResult, error) {
	if q.ChargeRef != "" {
		var pi stripeIntent
		if err := s.do(ctx, http.MethodGet, "/v1/payment_intents/"+url.PathEscape(q.ChargeRef), nil, "", &pi); err != nil {
			return ChargeResult{}, err
		}
		if pi.ID != q.ChargeRef || (pi.Metadata["scry_key"] != "" && pi.Metadata["scry_key"] != q.Key) {
			return ChargeResult{}, fmt.Errorf("card processor: payment %s does not belong to this top-up", q.ChargeRef)
		}
		return pi.result(), nil
	}
	if q.CustomerRef == "" {
		return ChargeResult{}, errors.New("card processor lookup: the top-up has no customer recorded")
	}
	form := url.Values{
		"customer":     {q.CustomerRef},
		"limit":        {"100"},
		"created[gte]": {strconv.FormatInt(q.Since.Add(-10*time.Minute).Unix(), 10)},
	}
	for page := 0; page < findPages; page++ {
		var list struct {
			Data    []stripeIntent `json:"data"`
			HasMore bool           `json:"has_more"`
		}
		if err := s.do(ctx, http.MethodGet, "/v1/payment_intents?"+form.Encode(), nil, "", &list); err != nil {
			return ChargeResult{}, err
		}
		for _, pi := range list.Data {
			if pi.Metadata["scry_key"] == q.Key {
				return pi.result(), nil
			}
		}
		if !list.HasMore {
			return ChargeResult{Status: ChargeNotFound}, nil // a complete scan
		}
		if len(list.Data) == 0 {
			break // "more" with nothing to continue from: not a complete scan
		}
		form.Set("starting_after", list.Data[len(list.Data)-1].ID)
	}
	return ChargeResult{}, ErrLookupIncomplete
}

func (s *Stripe) SetCancelAtPeriodEnd(ctx context.Context, subscriptionRef string, cancel bool) error {
	return s.do(ctx, http.MethodPost, "/v1/subscriptions/"+url.PathEscape(subscriptionRef),
		url.Values{"cancel_at_period_end": {strconv.FormatBool(cancel)}}, "", nil)
}

// webhookTolerance is how old a signed webhook may be.
const webhookTolerance = 5 * time.Minute

func (s *Stripe) verify(payload []byte, header string, now time.Time) error {
	var stamp string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			stamp = v
		case "v1":
			sigs = append(sigs, v)
		}
	}
	unix, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil || len(sigs) == 0 {
		return errors.New("webhook signature header is malformed")
	}
	if d := now.Sub(time.Unix(unix, 0)); d > webhookTolerance || d < -webhookTolerance {
		return errors.New("webhook timestamp is outside the tolerance")
	}
	mac := hmac.New(sha256.New, []byte(s.webhookSecret))
	mac.Write([]byte(stamp + "."))
	mac.Write(payload)
	want := mac.Sum(nil)
	for _, sig := range sigs {
		got, err := hex.DecodeString(sig)
		if err == nil && hmac.Equal(got, want) {
			return nil
		}
	}
	return errors.New("webhook signature does not match")
}

// intentEventStatus is the PaymentIntent status each top-up webhook reports.
var intentEventStatus = map[string]PaymentIntentStatus{
	"payment_intent.succeeded":      PISucceeded,
	"payment_intent.payment_failed": PIRequiresPaymentMethod,
	"payment_intent.canceled":       PICanceled,
}

// stripeLine is one invoice line; a subscription line names the period it bills.
type stripeLine struct {
	Period struct {
		Start int64 `json:"start"`
		End   int64 `json:"end"`
	} `json:"period"`
}

// invoicePeriod is the period an invoice bills, from its first line, or
// zero times when it has no lines (the app cannot order such an invoice).
func invoicePeriod(lines []stripeLine) (time.Time, time.Time) {
	if len(lines) == 0 {
		return time.Time{}, time.Time{}
	}
	return time.Unix(lines[0].Period.Start, 0).UTC(), time.Unix(lines[0].Period.End, 0).UTC()
}

func (s *Stripe) ParseWebhook(ctx context.Context, payload []byte, signature string, now time.Time) ([]Event, error) {
	if err := s.verify(payload, signature, now); err != nil {
		return nil, err
	}
	var ev struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Created  int64  `json:"created"`
		Livemode bool   `json:"livemode"`
		Data     struct {
			Object struct {
				ID               string            `json:"id"`
				Object           string            `json:"object"`
				Customer         string            `json:"customer"`
				Metadata         map[string]string `json:"metadata"`
				LastPaymentError struct {
					Message string `json:"message"`
				} `json:"last_payment_error"`
				Subscription  string `json:"subscription"`
				AmountPaid    int64  `json:"amount_paid"`
				AmountDue     int64  `json:"amount_due"`
				PaymentIntent string `json:"payment_intent"`
				Lines         struct {
					Data []stripeLine `json:"data"`
				} `json:"lines"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		return nil, err
	}
	if ev.Livemode != s.live {
		return nil, fmt.Errorf("%s-mode webhook refused: this Scry's Stripe is in %s mode", stripeModeName(ev.Livemode), stripeModeName(s.live))
	}
	o := ev.Data.Object
	out := Event{ID: ev.ID, At: time.Unix(ev.Created, 0).UTC()}
	switch ev.Type {
	case "invoice.paid":
		if o.Subscription == "" || o.AmountPaid == 0 { // a trial's zero invoice is not a charge
			return nil, nil
		}
		out.Kind, out.SubscriptionRef, out.AmountCents = InvoicePaid, o.Subscription, o.AmountPaid
		out.PeriodStart, out.PeriodEnd = invoicePeriod(o.Lines.Data)
	case "invoice.payment_failed":
		if o.Subscription == "" {
			return nil, nil
		}
		out.Kind, out.SubscriptionRef, out.AmountCents = InvoiceFailed, o.Subscription, o.AmountDue
		// The period the failed invoice was for, so the app can tell a stale
		// failure (for a period already settled) from the open invoice.
		out.PeriodStart, out.PeriodEnd = invoicePeriod(o.Lines.Data)
		out.Reason = DeclinedSentence
		if o.PaymentIntent != "" {
			var pi struct {
				LastPaymentError struct {
					Message string `json:"message"`
				} `json:"last_payment_error"`
			}
			if err := s.do(ctx, http.MethodGet, "/v1/payment_intents/"+url.PathEscape(o.PaymentIntent), nil, "", &pi); err == nil && pi.LastPaymentError.Message != "" {
				out.Reason = pi.LastPaymentError.Message
			}
		}
	case "customer.subscription.deleted":
		out.Kind, out.SubscriptionRef = SubscriptionEnded, o.ID
	case "payment_intent.succeeded", "payment_intent.payment_failed", "payment_intent.canceled":
		// Scry's top-ups carry its key, but an intent without it is still
		// passed on by id: the attempt it belongs to may have recorded that
		// id (charge_ref) already, and is then settled by it. An intent no
		// attempt knows (a plan's invoice payment, reported by the invoice
		// events above) matches nothing and is ignored by the app. A real
		// PaymentIntent always has an id; an event whose object has none is
		// malformed and ignored even if it carries a key, so it can never
		// settle an attempt under a blank charge_ref.
		key := o.Metadata["scry_key"]
		if o.ID == "" {
			return nil, nil
		}
		out.ChargeRef, out.ChargeKey, out.CustomerRef = o.ID, key, o.Customer
		// Each event type is the PaymentIntent status it reports, and goes
		// through the same state machine as a lookup: succeeded is paid; a
		// payment failure (back to requires_payment_method) is a decline,
		// with DeclinedSentence when Stripe gave none; a cancellation took
		// nothing and names a card fault only if Stripe named one.
		st := intentEventStatus[ev.Type]
		out.Kind, _ = TopupEventFor(st)
		_, out.Reason = ClassifyPaymentIntent(st, o.LastPaymentError.Message)
	default:
		return nil, nil
	}
	return []Event{out}, nil
}
