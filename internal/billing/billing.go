// Package billing puts the card processor behind a small interface.
// The app decides what a plan is and what it costs; the processor only
// holds cards, charges them and reports what happened.
package billing

import (
	"context"
	"time"
)

// DeclineError is the processor refusing a card. Reason is the
// processor's own sentence and is shown to the learner as it is.
type DeclineError struct{ Reason string }

func (e *DeclineError) Error() string { return e.Reason }

// Card is what Scry keeps of a card: never the number.
type Card struct{ Brand, Last4 string }

// SubscribeParams starts a plan for a new customer.
type SubscribeParams struct {
	Email     string
	CardToken string    // the processor's token for the card
	TrialEnd  time.Time // first charge then; zero means charge now
	Key       string    // idempotency key
	Now       time.Time // the app's clock, for an adapter with none of its own
}

// Subscribed is a plan the processor has opened.
type Subscribed struct {
	CustomerRef     string
	SubscriptionRef string
	PaymentRef      string
	Card            Card
	PeriodStart     time.Time
	PeriodEnd       time.Time
	ChargeRef       string // set when the first charge was taken at once
}

// EventKind is what the processor reports about a plan over time.
type EventKind string

const (
	InvoicePaid       EventKind = "invoice_paid"       // a period was charged
	InvoiceFailed     EventKind = "invoice_failed"     // the charge was refused
	SubscriptionEnded EventKind = "subscription_ended" // the plan is over
)

// Event is one thing the processor did to a subscription.
type Event struct {
	ID              string
	Kind            EventKind
	SubscriptionRef string
	AmountCents     int64
	PeriodStart     time.Time
	PeriodEnd       time.Time
	Reason          string // for InvoiceFailed, the processor's sentence
	At              time.Time
}

// ChargeStatus is what the processor says became of a one-off charge.
type ChargeStatus string

const (
	ChargeSucceeded ChargeStatus = "succeeded" // the money was taken
	ChargeFailed    ChargeStatus = "failed"    // it was not, and will not be
	ChargePending   ChargeStatus = "pending"   // the processor is still working on it
	ChargeNotFound  ChargeStatus = "not_found" // the processor has no charge with that key
)

// ChargeResult is the processor's account of one charge.
type ChargeResult struct {
	Status ChargeStatus
	Ref    string // the processor's id, when there is a charge
	Reason string // for ChargeFailed, the processor's sentence
}

// Processor is the card processor.
type Processor interface {
	Name() string
	// PublishableKey is the browser-side key for the processor's card
	// fields; empty means plain test-card fields.
	PublishableKey() string
	// Tokenize turns typed card details into a token. Only the test
	// adapter implements it; real card numbers never reach Scry.
	Tokenize(number, expiry, cvc string, now time.Time) (string, error)
	Subscribe(ctx context.Context, p SubscribeParams) (Subscribed, error)
	// UpdateCard replaces the card on file and retries an unpaid period.
	UpdateCard(ctx context.Context, customerRef, subscriptionRef, cardToken string) (paymentRef string, card Card, err error)
	// Charge takes a one-off payment (a top-up) from the card on file.
	// key is the processor idempotency key: the same key never charges twice.
	Charge(ctx context.Context, customerRef, paymentRef string, amountCents int64, description, key string) (chargeRef string, err error)
	// FindCharge looks up what became of a one-off charge made with key,
	// for when the reply to Charge was lost or the app restarted before it
	// recorded the result. since is when the attempt began.
	FindCharge(ctx context.Context, customerRef, paymentRef, key string, since time.Time) (ChargeResult, error)
	// SetCancelAtPeriodEnd schedules or unschedules the plan's end.
	SetCancelAtPeriodEnd(ctx context.Context, subscriptionRef string, cancel bool) error
	// ParseWebhook verifies and decodes the processor's notifications.
	ParseWebhook(ctx context.Context, payload []byte, signature string, now time.Time) ([]Event, error)
}

// Renewer is implemented by the test adapter, which has no clock of its
// own: asked about a plan whose period has run out, it says what a real
// processor would have done by then.
type Renewer interface {
	Renew(subscriptionRef, paymentRef string, cancelAtEnd, pastDue bool, periodEnd time.Time, priceCents int64, now time.Time) Event
}
