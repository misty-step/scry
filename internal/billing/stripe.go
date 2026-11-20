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

// Stripe is the real processor, and it stays in test mode: NewStripe
// refuses any key that is not a test key, and live-mode webhooks are
// rejected. Cards are collected in the browser by Stripe.js; Scry only
// ever sees a payment method id.
type Stripe struct {
	secret, publishable, priceID, webhookSecret string
	BaseURL                                     string
	HTTP                                        *http.Client
}

// stripeVersion pins the API shape this adapter was written against.
const stripeVersion = "2024-06-20"

// NewStripe builds the adapter from credentials read from the environment.
func NewStripe(secret, publishable, priceID, webhookSecret string) (*Stripe, error) {
	if !strings.HasPrefix(secret, "sk_test_") && !strings.HasPrefix(secret, "rk_test_") {
		return nil, errors.New("STRIPE_SECRET_KEY must be a test-mode key (sk_test_…): Scry's Stripe stays in test mode")
	}
	if !strings.HasPrefix(publishable, "pk_test_") {
		return nil, errors.New("STRIPE_PUBLISHABLE_KEY must be a test-mode key (pk_test_…)")
	}
	if priceID == "" {
		return nil, errors.New("STRIPE_PRICE_ID is required: the monthly price of the plan")
	}
	if webhookSecret == "" {
		return nil, errors.New("STRIPE_WEBHOOK_SECRET is required: renewals arrive by webhook")
	}
	return &Stripe{secret: secret, publishable: publishable, priceID: priceID, webhookSecret: webhookSecret}, nil
}

func (s *Stripe) Name() string           { return "stripe-test" }
func (s *Stripe) PublishableKey() string { return s.publishable }

func (s *Stripe) Tokenize(string, string, string, time.Time) (string, error) {
	return "", errors.New("card details are collected by Stripe.js, never by Scry")
}

type stripeError struct {
	Error struct {
		Type    string `json:"type"`
		Code    string `json:"code"`
		Message string `json:"message"`
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
			return &DeclineError{Reason: se.Error.Message}
		}
		return fmt.Errorf("card processor: status %d %s %s", resp.StatusCode, se.Error.Code, se.Error.Message)
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
	}, p.Key+"-cus", &cus)
	if err != nil {
		return Subscribed{}, err
	}
	form := url.Values{
		"customer":               {cus.ID},
		"items[0][price]":        {s.priceID},
		"default_payment_method": {p.CardToken},
		"expand[]":               {"latest_invoice"},
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
	card, err := s.card(ctx, p.CardToken)
	if err != nil {
		return Subscribed{}, err
	}
	out := Subscribed{CustomerRef: cus.ID, SubscriptionRef: sub.ID, PaymentRef: p.CardToken, Card: card,
		PeriodStart: time.Unix(sub.CurrentPeriodStart, 0).UTC(), PeriodEnd: time.Unix(sub.CurrentPeriodEnd, 0).UTC()}
	if p.TrialEnd.IsZero() {
		out.ChargeRef = sub.LatestInvoice.ID
	}
	return out, nil
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

// confirmSentence is said when an off-session payment needs the learner.
const confirmSentence = "Your bank needs you to confirm this payment. Update your card and try again."

// result maps a PaymentIntent's status to what became of the charge.
func (pi stripeIntent) result() ChargeResult {
	switch pi.Status {
	case "succeeded":
		return ChargeResult{Status: ChargeSucceeded, Ref: pi.ID}
	case "processing", "requires_capture":
		return ChargeResult{Status: ChargePending, Ref: pi.ID}
	}
	// canceled, requires_payment_method, requires_action, requires_confirmation:
	// an off-session charge in any of these will not be taken.
	reason := pi.LastPaymentError.Message
	if reason == "" {
		reason = confirmSentence
	}
	return ChargeResult{Status: ChargeFailed, Ref: pi.ID, Reason: reason}
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
	switch res := pi.result(); res.Status {
	case ChargeSucceeded:
		return pi.ID, nil
	case ChargePending:
		// Accepted but not finished: neither paid nor declined yet. The id
		// is returned so the attempt can be settled later.
		return pi.ID, &PendingError{Ref: pi.ID}
	default:
		return "", &DeclineError{Reason: res.Reason}
	}
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
					Data []struct {
						Period struct {
							Start int64 `json:"start"`
							End   int64 `json:"end"`
						} `json:"period"`
					} `json:"data"`
				} `json:"lines"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		return nil, err
	}
	if ev.Livemode {
		return nil, errors.New("live-mode webhook refused: Scry's Stripe stays in test mode")
	}
	o := ev.Data.Object
	out := Event{ID: ev.ID, At: time.Unix(ev.Created, 0).UTC()}
	switch ev.Type {
	case "invoice.paid":
		if o.Subscription == "" || o.AmountPaid == 0 { // a trial's zero invoice is not a charge
			return nil, nil
		}
		out.Kind, out.SubscriptionRef, out.AmountCents = InvoicePaid, o.Subscription, o.AmountPaid
		if len(o.Lines.Data) > 0 {
			out.PeriodStart = time.Unix(o.Lines.Data[0].Period.Start, 0).UTC()
			out.PeriodEnd = time.Unix(o.Lines.Data[0].Period.End, 0).UTC()
		}
	case "invoice.payment_failed":
		if o.Subscription == "" {
			return nil, nil
		}
		out.Kind, out.SubscriptionRef, out.AmountCents = InvoiceFailed, o.Subscription, o.AmountDue
		out.Reason = "Your card was declined."
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
		// Only Scry's own top-ups carry its key; a plan's invoice payments
		// are reported by the invoice events above.
		key := o.Metadata["scry_key"]
		if key == "" || o.ID == "" {
			return nil, nil
		}
		out.ChargeRef, out.ChargeKey, out.CustomerRef = o.ID, key, o.Customer
		switch ev.Type {
		case "payment_intent.succeeded":
			out.Kind = TopupPaid
		case "payment_intent.payment_failed":
			out.Kind, out.Reason = TopupFailed, o.LastPaymentError.Message
			if out.Reason == "" {
				out.Reason = "Your card was declined."
			}
		default: // canceled: nothing was taken, and there is no card fault to show
			out.Kind = TopupFailed
		}
	default:
		return nil, nil
	}
	return []Event{out}, nil
}
