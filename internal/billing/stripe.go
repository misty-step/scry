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

func (s *Stripe) Charge(ctx context.Context, customerRef, paymentRef string, amountCents int64, description, key string) (string, error) {
	var pi struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	err := s.do(ctx, http.MethodPost, "/v1/payment_intents", url.Values{
		"amount":         {strconv.FormatInt(amountCents, 10)},
		"currency":       {"usd"},
		"customer":       {customerRef},
		"payment_method": {paymentRef},
		"off_session":    {"true"},
		"confirm":        {"true"},
		"description":    {description},
	}, key, &pi)
	if err != nil {
		return "", err
	}
	if pi.Status != "succeeded" {
		return "", &DeclineError{Reason: "Your bank needs you to confirm this payment. Update your card and try again."}
	}
	return pi.ID, nil
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
				ID            string `json:"id"`
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
	default:
		return nil, nil
	}
	return []Event{out}, nil
}
