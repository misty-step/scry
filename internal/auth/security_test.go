package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type failingTransport struct{}

func (failingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, errors.New("failed " + r.URL.String())
}
func TestG8_ProviderKeyNeverLeavesAdapter(t *testing.T) {
	const key = "test-secret-do-not-log"
	a := IdentityToolkit{APIKey: key, HTTP: &http.Client{Transport: failingTransport{}}}
	_, err := a.SignIn(context.Background(), "person@example.com", "password")
	if err == nil || strings.Contains(err.Error(), key) {
		t.Fatalf("unsafe provider error: %v", err)
	}
}
