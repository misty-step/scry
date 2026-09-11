// Package generation turns durable source jobs into bounded, inspectable quizzes.
// No model call is made on the review or grading path.
package generation

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/misty-step/scry/internal/store"
)

const (
	requestTimeout   = 60 * time.Second
	jobLease         = 90 * time.Second
	settleTimeout    = 10 * time.Second
	promptVersion    = "scry-go-quiz-v3"
	maxSourceBytes   = 32 << 10
	maxRequestBytes  = 128 << 10
	maxResponseBytes = 1 << 20
	maxContentBytes  = 512 << 10
	maxQuizzes       = 60
	repairMarker     = "Generation quality repair pending: "
)

// Config requires an explicit provider endpoint, model, and spending allowance.
// Endpoint is the complete OpenAI-compatible chat-completions URL. An API key is
// required for OpenRouter; an explicitly configured private gateway may omit it.
// Provider is "openrouter" for OpenRouter behind a private proxy, or empty/"openai"
// for a generic compatible gateway. Direct OpenRouter URLs are detected as well.
// ReservationMicros must conservatively cover one bounded request for that model;
// token counts are never silently converted into an invented dollar price.
type Config struct {
	Endpoint          string
	APIKey            string
	Model             string
	Provider          string
	DailyBudgetMicros int64
	ReservationMicros int64
	PollInterval      time.Duration
	HTTPClient        *http.Client
}

// Worker is deliberately serial: one claimed attempt, one HTTP transmission.
// Store fences source revisions, leases, publication, and attempt accounting.
type Worker struct {
	store       *store.Store
	cfg         Config
	client      *http.Client
	configError string
	openRouter  bool
}

// New does not start goroutines or access the network. Configuration failures are
// persisted against queued captures by Run rather than replaced with fake quizzes.
func New(s *store.Store, cfg Config) *Worker {
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	if cfg.PollInterval < 250*time.Millisecond {
		cfg.PollInterval = 250 * time.Millisecond
	}
	if cfg.PollInterval > time.Minute {
		cfg.PollInterval = time.Minute
	}
	w := &Worker{store: s, cfg: cfg}
	w.configError = w.validateConfig()
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Transport: &http.Transport{
			Proxy:                  http.ProxyFromEnvironment,
			DialContext:            (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:      true,
			TLSHandshakeTimeout:    10 * time.Second,
			ResponseHeaderTimeout:  30 * time.Second,
			MaxResponseHeaderBytes: 64 << 10,
			IdleConnTimeout:        90 * time.Second,
			MaxIdleConns:           2,
			MaxIdleConnsPerHost:    1,
			MaxConnsPerHost:        1,
		}}
	}
	// Never mutate a shared client or follow a redirect with private source data.
	copyClient := *client
	if copyClient.Timeout <= 0 || copyClient.Timeout > requestTimeout {
		copyClient.Timeout = requestTimeout
	}
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	w.client = &copyClient
	return w
}

func (w *Worker) validateConfig() string {
	if w.cfg.Endpoint == "" {
		return "Generation is not configured: set the full HTTPS chat-completions endpoint, model, credentials, and spending limits, then retry this saved source."
	}
	u, err := url.Parse(w.cfg.Endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "Generation endpoint is invalid: use a complete HTTPS URL without embedded credentials, query, or fragment, then retry."
	}
	loopback := strings.EqualFold(u.Hostname(), "localhost")
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return "Generation endpoint must use HTTPS; HTTP is permitted only for a loopback gateway. Update configuration and retry."
	}
	directOpenRouter := strings.EqualFold(u.Hostname(), "openrouter.ai") || strings.HasSuffix(strings.ToLower(u.Hostname()), ".openrouter.ai")
	switch w.cfg.Provider {
	case "", "openai", "openrouter":
	default:
		return "Generation provider is invalid: select openai for a compatible gateway or openrouter for OpenRouter routing safeguards, then retry."
	}
	w.openRouter = directOpenRouter || w.cfg.Provider == "openrouter"
	if w.cfg.Model == "" || len(w.cfg.Model) > 200 || strings.IndexFunc(w.cfg.Model, unicode.IsControl) >= 0 || w.cfg.Model == "openrouter/auto" {
		return "Generation model is missing or invalid: choose one explicit model supporting structured JSON output, then retry."
	}
	if len(w.cfg.APIKey) > 4096 || strings.IndexFunc(w.cfg.APIKey, unicode.IsControl) >= 0 || (directOpenRouter && strings.TrimSpace(w.cfg.APIKey) == "") {
		return "Generation credentials are missing or invalid: configure the provider API key privately, then retry."
	}
	if w.cfg.DailyBudgetMicros <= 0 || w.cfg.ReservationMicros <= 0 || w.cfg.ReservationMicros > w.cfg.DailyBudgetMicros {
		return "Generation spending is not configured: set positive daily and per-attempt USD-micro allowances, with a reservation no larger than the daily limit, then retry."
	}
	return ""
}

// Run persists failures as job state and continues serving independent jobs.
// Only database/invariant failures stop the worker. Cancellation settles any
// already-transmitted attempt under a short independent deadline, preserving
// unknown spend when a response could have been lost after provider acceptance.
func (w *Worker) Run(ctx context.Context) error {
	if w.store == nil {
		return errors.New("generation: store is required")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		reservation, budget := w.cfg.ReservationMicros, w.cfg.DailyBudgetMicros
		if w.configError != "" {
			reservation, budget = 0, 0 // No network; explicitly known zero-cost failure.
		}
		job, err := w.store.ClaimJob(ctx, jobLease, reservation, budget)
		if err != nil && !errors.Is(err, store.ErrBudget) {
			return fmt.Errorf("claim generation job: %w", err)
		}
		if err == nil && job != nil {
			if err := w.process(ctx, job); err != nil {
				return err
			}
			continue
		}
		timer := time.NewTimer(w.cfg.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (w *Worker) process(ctx context.Context, job *store.Job) error {
	var result store.GenerationResult
	var failure *generationFailure
	var cost *int64
	if w.configError != "" {
		zero := int64(0)
		cost = &zero
		failure = &generationFailure{message: w.configError}
	} else if ctx.Err() != nil {
		zero := int64(0)
		cost = &zero
		failure = &generationFailure{message: "Generation stopped before transmission; the source is saved and can be retried."}
	} else {
		result, cost, failure = w.generate(ctx, job)
	}
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleTimeout)
	defer cancel()
	var err error
	if failure != nil {
		if w.configError == "" {
			version := promptVersion
			if job.Attempts == 2 && strings.HasPrefix(job.Error, repairMarker) {
				version += "-repair1"
			}
			if job.FoundationTarget != nil {
				version = foundationPromptVersion
			}
			failure.message += " Requested model: " + w.cfg.Model + "; prompt: " + version + "."
		}
		err = w.store.FailJob(settleCtx, job.ID, job.LeaseToken, failure.message, failure.retry, cost)
	} else {
		err = w.store.CompleteJob(settleCtx, job.ID, job.LeaseToken, result, cost)
	}
	// These are durable rejection/obsolete-owner outcomes, not permission to send
	// another request or to retry publication under a fresh lease token.
	if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrInvalid) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("settle generation attempt: %w", err)
	}
	return nil
}

type generationFailure struct {
	message string
	retry   bool
}
