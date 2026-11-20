package billing

// PaymentIntentStatus is the processor's status for one one-off charge (a
// Stripe PaymentIntent). The lifecycle at the processor is
//
//	requires_payment_method -> requires_confirmation -> requires_action
//	                        -> processing -> succeeded
//	any non-final status    -> canceled
//	requires_payment_method <- a refused confirm (a decline)
//
// For a top-up, only succeeded is paid; canceled and
// requires_payment_method are final refusals; every other status, and any
// status this adapter does not know, is still in progress.
type PaymentIntentStatus string

const (
	PIRequiresPaymentMethod PaymentIntentStatus = "requires_payment_method"
	PIRequiresConfirmation  PaymentIntentStatus = "requires_confirmation"
	PIRequiresAction        PaymentIntentStatus = "requires_action"
	PIRequiresCapture       PaymentIntentStatus = "requires_capture"
	PIProcessing            PaymentIntentStatus = "processing"
	PISucceeded             PaymentIntentStatus = "succeeded"
	PICanceled              PaymentIntentStatus = "canceled"
)

// PaymentIntentStatuses lists every status Stripe documents.
var PaymentIntentStatuses = []PaymentIntentStatus{
	PIRequiresPaymentMethod, PIRequiresConfirmation, PIRequiresAction, PIRequiresCapture,
	PIProcessing, PISucceeded, PICanceled,
}

// Final reports whether the charge can no longer change at the processor
// for a top-up (Scry never confirms a refused intent again itself).
func (s PaymentIntentStatus) Final() bool {
	return s == PISucceeded || s == PICanceled || s == PIRequiresPaymentMethod
}

// DeclinedSentence is said for a refused charge the processor gave no
// sentence for.
const DeclinedSentence = "Your card was declined."

// ClassifyPaymentIntent maps a PaymentIntent's status, and the
// processor's sentence for its last payment error, to what became of the
// charge and the sentence to show:
//
//	succeeded               -> ChargeSucceeded
//	requires_payment_method -> ChargeFailed, the sentence or DeclinedSentence
//	canceled                -> ChargeFailed, the sentence only if one was named
//	                           (nothing was taken; no card fault otherwise)
//	anything else           -> ChargePending (never declined early)
//
// It is pure, and the reference the Stripe adapter's mapping is tested
// against.
func ClassifyPaymentIntent(status PaymentIntentStatus, declineReason string) (ChargeStatus, string) {
	switch status {
	case PISucceeded:
		return ChargeSucceeded, ""
	case PIRequiresPaymentMethod:
		if declineReason == "" {
			declineReason = DeclinedSentence
		}
		return ChargeFailed, declineReason
	case PICanceled:
		return ChargeFailed, declineReason
	}
	return ChargePending, ""
}

// ChargeOutcome is the answer a synchronous Charge gives for an intent in
// that status: nil when it was paid, a *DeclineError for a refusal naming
// a card fault, a *CanceledError for a cancellation with none, and a
// *PendingError for an intent still in progress. ref is the intent's id.
func ChargeOutcome(status PaymentIntentStatus, ref, declineReason string) error {
	switch st, reason := ClassifyPaymentIntent(status, declineReason); st {
	case ChargeSucceeded:
		return nil
	case ChargePending:
		return &PendingError{Ref: ref}
	default:
		if reason == "" {
			return &CanceledError{Ref: ref}
		}
		return &DeclineError{Reason: reason, Ref: ref}
	}
}

// TopupEventFor is the top-up webhook kind a PaymentIntent status change
// produces, if any: succeeded is TopupPaid; a refusal or a cancellation is
// TopupFailed; anything else produces none.
func TopupEventFor(status PaymentIntentStatus) (EventKind, bool) {
	switch status {
	case PISucceeded:
		return TopupPaid, true
	case PIRequiresPaymentMethod, PICanceled:
		return TopupFailed, true
	}
	return "", false
}
