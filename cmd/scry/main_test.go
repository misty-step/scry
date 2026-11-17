package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestRule_TestModeNeverCallsAlertWebhook(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	t.Setenv("SCRY_MODE", "test")
	t.Setenv("SCRY_DB", filepath.Join(t.TempDir(), "test.db"))
	t.Setenv("ALERT_WEBHOOK_URL", srv.URL)
	a, err := build(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Store.Close()
	a.Alert.Notify(context.Background(), "test", "test")
	if calls.Load() != 0 {
		t.Fatal("test mode sent an external alert")
	}
}
