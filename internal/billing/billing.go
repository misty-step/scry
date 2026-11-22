// Package billing puts the card processor behind a small interface.
// The app decides what a plan is and what it costs; the processor only
// holds cards, charges them and reports what happened.
package billing

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// DeclineError is the processor refusing a card. Reason is the
// processor's own sentence and is shown to the learner as it is. Ref is
// the processor's id for the refused charge (a PaymentIntent), when it
// named one, so the failed attempt still records it.
type DeclineError struct{ Reason, Ref string }

func (e *DeclineError) Error() string { return e.Reason }

// PendingError is a one-off charge the processor accepted but has not
// finished (a PaymentIntent that is processing). Ref is the processor's id
// for it: keep it, and settle the charge later by lookup or by webhook.
// It is neither a success nor a decline.
type PendingError struct{ Ref string }

func (e *PendingError) Error() string { return "the card processor is still processing this charge" }

// CanceledError is a one-off charge the processor canceled without naming
// a card fault (a PaymentIntent canceled with no last_payment_error).
// Nothing was taken and the card is not at fault: the attempt is released
// with no decline sentence, as a lookup that finds it canceled does. Ref is
// the processor's id for the canceled charge, kept on the attempt.
type CanceledError struct{ Ref string }

func (e *CanceledError) Error() string {
	return "the card processor canceled this charge; nothing was taken"
}

// ProcessorError is any other error answer from the processor. Its outcome
// is unknown. Ref is the charge (PaymentIntent) it named, when it named
// one, so the attempt can be looked up by id later.
type ProcessorError struct {
	Status        int
	Type          string // the processor's error type, when it named one
	Code, Message string
	Ref           string
}

// NothingDone reports whether err is a definitive answer that a Subscribe
// did nothing: the card was refused, the charge was canceled, or the
// processor refused the request itself as invalid (a card token it does
// not know, a trial end in the past). Everything else says nothing about
// what happened: a lost reply, a timeout, a 5xx, a rate limit, a refused
// credential, and above all an idempotency error, which means an earlier
// request used the same key.
func NothingDone(err error) bool {
	var d *DeclineError
	var c *CanceledError
	var p *ProcessorError
	switch {
	case errors.As(err, &d), errors.As(err, &c):
		return true
	case errors.As(err, &p):
		return p.Type == "invalid_request_error" && (p.Status == 400 || p.Status == 402 || p.Status == 404)
	}
	return false
}

// KeyInUse reports whether err is the processor still working on another
// request with the same idempotency key.
func KeyInUse(err error) bool {
	var p *ProcessorError
	return errors.As(err, &p) && p.Status == 409
}

func (e *ProcessorError) Error() string {
	return fmt.Sprintf("card processor: status %d %s %s", e.Status, e.Code, e.Message)
}

// ErrLookupIncomplete is a lookup that could not see everything it needed
// to (a truncated list). It proves nothing either way; ask again later.
var ErrLookupIncomplete = errors.New("card processor lookup was incomplete")

// Card is what Scry keeps of a card: never the number.
type Card struct{ Brand, Last4 string }

// SubscribeParams starts a plan for a new customer. The same key with the
// same parameters never opens a second plan, for as long as the processor
// remembers the key (IdempotencyKeyLife).
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

// IdempotencyKeyLife is how long a repeated key is relied on to return the
// first request's result. Stripe forgets keys after 24 hours, and a
// forgotten key is a new request; this keeps an hour clear of that. Past
// it, only a lookup (SubscriptionFinder) can say what became of a request.
const IdempotencyKeyLife = 23 * time.Hour

// SubscriptionState is what the processor says a subscription is now.
type SubscriptionState string

const (
	SubscriptionNotFound SubscriptionState = "not_found" // a complete lookup has none with that key
	SubscriptionTrialing SubscriptionState = "trialing"  // open, not charged yet
	SubscriptionPaid     SubscriptionState = "paid"      // open, its period is paid for
	SubscriptionUnpaid   SubscriptionState = "unpaid"    // open, its last invoice was not paid
	SubscriptionOver     SubscriptionState = "over"      // canceled or expired: it charges nothing more
)

// SubscriptionFinder is implemented by an adapter that can look up what
// became of a Subscribe whose reply was lost. It never creates or changes
// anything. q is what that Subscribe was sent, and since is when. The
// Subscribed is filled in unless the state is SubscriptionNotFound, which
// is only reported after a complete lookup (else ErrLookupIncomplete).
type SubscriptionFinder interface {
	FindSubscription(ctx context.Context, q SubscribeParams, since time.Time) (SubscriptionState, Subscribed, error)
}

// EventKind is what the processor reports about a plan over time.
type EventKind string

const (
	InvoicePaid       EventKind = "invoice_paid"       // a period was charged
	InvoiceFailed     EventKind = "invoice_failed"     // the charge was refused
	SubscriptionEnded EventKind = "subscription_ended" // the plan is over
	TopupPaid         EventKind = "topup_paid"         // a one-off top-up charge succeeded
	TopupFailed       EventKind = "topup_failed"       // a one-off top-up charge failed or was canceled
)

// Event is one thing the processor did to a subscription.
type Event struct {
	ID              string
	Kind            EventKind
	SubscriptionRef string
	AmountCents     int64
	PeriodStart     time.Time
	PeriodEnd       time.Time
	Reason          string // for InvoiceFailed and TopupFailed, the processor's sentence
	At              time.Time
	// For top-up events: the processor's charge id, the Scry idempotency
	// key kept in its metadata, and its customer.
	ChargeRef   string
	ChargeKey   string
	CustomerRef string
}

// ChargeStatus is what the processor says became of a one-off charge.
type ChargeStatus string

const (
	ChargeSucceeded ChargeStatus = "succeeded" // the money was taken
	ChargeFailed    ChargeStatus = "failed"    // it was not, and will not be
	ChargePending   ChargeStatus = "pending"   // the processor is still working on it
	ChargeNotFound  ChargeStatus = "not_found" // the processor has no charge with that key (yet)
)

// ChargeQuery identifies one one-off charge for a lookup. Every field is
// what was recorded on the attempt when it was made, never the learner's
// current (mutable) plan: a resubscription opens a new customer.
type ChargeQuery struct {
	CustomerRef string    // the customer the charge was made for
	PaymentRef  string    // the card it was made on
	ChargeRef   string    // the processor's id for the charge, when it is known
	Key         string    // the idempotency key, also kept in the charge's metadata
	Since       time.Time // when the attempt began
}

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
	// A *DeclineError is a final refusal; a *CanceledError is a charge the
	// processor canceled with no card fault (nothing taken); a *PendingError
	// is a charge the processor accepted but has not finished; any other
	// error is unknown.
	Charge(ctx context.Context, customerRef, paymentRef string, amountCents int64, description, key string) (chargeRef string, err error)
	// FindCharge looks up what became of a one-off charge, for when the
	// reply to Charge was lost, the charge was still processing, or the app
	// restarted before it recorded the result. ChargeNotFound only means
	// "not visible now"; it is never proof that nothing will be charged.
	FindCharge(ctx context.Context, q ChargeQuery) (ChargeResult, error)
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
