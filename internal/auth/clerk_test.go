package auth

import (
	"context"
	clerk "github.com/clerk/clerk-sdk-go/v2"
	"testing"
)

func TestClerkVerifiedEmail(t *testing.T) {
	id := "email_1"
	u := &clerk.User{ID: "user_1", PrimaryEmailAddressID: &id, EmailAddresses: []*clerk.EmailAddress{{ID: id, EmailAddress: "p@example.com", Verification: &clerk.Verification{Status: "unverified"}}}}
	if _, _, err := verifiedIdentity(u); err == nil {
		t.Fatal("unverified email allowed")
	}
	u.EmailAddresses[0].Verification.Status = "verified"
	if sub, email, err := verifiedIdentity(u); err != nil || sub != "user_1" || email != "p@example.com" {
		t.Fatal(sub, email, err)
	}
	u.Banned = true
	if _, _, err := verifiedIdentity(u); err == nil {
		t.Fatal("banned identity allowed")
	}
}
func TestClerkMissingKeysNoNetwork(t *testing.T) {
	c := &Clerk{}
	if c.Configured() {
		t.Fatal("configured")
	}
	if _, _, err := c.Verify(context.Background(), "invalid", "http://example.test"); err == nil {
		t.Fatal("accepted")
	}
}
