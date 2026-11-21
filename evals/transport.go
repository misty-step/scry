package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// recorder is an http.RoundTripper that keeps a bounded copy of the one
// response a production adapter reads, so the bench can record exact
// usage.cost, the resolved model and the raw model text without changing
// the adapter. Error bodies are read in full (bounded) up front, because
// the adapters close them unread.
type recorder struct {
	base   http.RoundTripper
	status int
	buf    bytes.Buffer
}

const recordLimit = 4 << 20

func newRecorder() *recorder { return &recorder{base: http.DefaultTransport} }

func (r *recorder) client(timeout time.Duration) *http.Client {
	return &http.Client{Transport: r, Timeout: timeout}
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	r.status = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, recordLimit))
		resp.Body.Close()
		r.buf.Write(raw)
		resp.Body = io.NopCloser(bytes.NewReader(raw))
		return resp, nil
	}
	resp.Body = &teeBody{rc: resp.Body, buf: &r.buf}
	return resp, nil
}

type teeBody struct {
	rc  io.ReadCloser
	buf *bytes.Buffer
}

func (t *teeBody) Read(p []byte) (int, error) {
	n, err := t.rc.Read(p)
	if n > 0 && t.buf.Len() < recordLimit {
		t.buf.Write(p[:n])
	}
	return n, err
}

func (t *teeBody) Close() error { return t.rc.Close() }

// usageInfo is what the bench records about one call's billing.
type usageInfo struct {
	Cost       *float64
	Prompt     int64
	Completion int64
	Model      string
	Provider   string
	Seconds    float64
}

// parseUsage reads OpenRouter's usage from a JSON body: chat completions
// (prompt_tokens/completion_tokens), Decisions and STT (input_tokens/
// output_tokens, seconds).
func parseUsage(raw []byte) usageInfo {
	var out struct {
		Model    string `json:"model"`
		Provider string `json:"provider"`
		Usage    *struct {
			Cost       *float64 `json:"cost"`
			Prompt     int64    `json:"prompt_tokens"`
			Completion int64    `json:"completion_tokens"`
			Input      int64    `json:"input_tokens"`
			Output     int64    `json:"output_tokens"`
			Seconds    float64  `json:"seconds"`
		} `json:"usage"`
	}
	var u usageInfo
	if json.Unmarshal(raw, &out) != nil {
		return u
	}
	u.Model, u.Provider = out.Model, out.Provider
	if out.Usage != nil {
		u.Cost = validCost(out.Usage.Cost)
		u.Prompt, u.Completion = max(out.Usage.Prompt, out.Usage.Input), max(out.Usage.Completion, out.Usage.Output)
		u.Seconds = out.Usage.Seconds
	}
	return u
}

// validCost is c when it is a usable charge, else nil (unknown). A
// negative, NaN or infinite usage.cost is malformed: settling it as exact
// would lower the spend breaker's recorded spend, while an unknown cost is
// charged its reservation, which keeps the cap conservative.
func validCost(c *float64) *float64 {
	if c == nil || *c < 0 || math.IsNaN(*c) || math.IsInf(*c, 0) {
		return nil
	}
	return c
}

// errorSnippet is a short, key-free description of a failed response body.
func errorSnippet(status int, raw []byte, key string) string {
	var out struct {
		Error *struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	msg := ""
	if json.Unmarshal(raw, &out) == nil && out.Error != nil {
		msg = fmt.Sprintf("code=%v %s", out.Error.Code, out.Error.Message)
	} else {
		msg = string(raw)
	}
	msg = redact(oneLine(msg), key)
	if r := []rune(msg); len(r) > 300 {
		msg = string(r[:300]) + "…"
	}
	return fmt.Sprintf("HTTP %d: %s", status, msg)
}

// redact removes the key from anything the bench writes.
func redact(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "[redacted]")
}

// Price is one catalog entry's per-unit prices in USD.
type Price struct {
	Prompt     float64 `json:"prompt"`
	Completion float64 `json:"completion"`
	Audio      float64 `json:"audio,omitempty"`
}

// CatalogEntry is what the bench verified about one slug on OpenRouter.
type CatalogEntry struct {
	Slug             string   `json:"slug"`
	Listed           bool     `json:"listed"`
	Listing          string   `json:"listing,omitempty"`
	CanonicalSlug    string   `json:"canonical_slug,omitempty"`
	InputModalities  []string `json:"input_modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
	Endpoints        []string `json:"endpoints,omitempty"`
	Price            Price    `json:"price_per_unit_usd"`
	PriceRaw         any      `json:"pricing_raw,omitempty"`
}

// Catalog is the live OpenRouter model list, fetched without a key.
type Catalog map[string]CatalogEntry

// FetchCatalog reads the three public listings the bench's candidates
// come from.
func FetchCatalog(ctx context.Context, base string) (Catalog, error) {
	cat := Catalog{}
	for _, q := range []string{"", "?output_modalities=decisions", "?output_modalities=transcription"} {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/models"+q, nil)
		if err != nil {
			cancel()
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("model catalog unreachable")
		}
		var out struct {
			Data []struct {
				ID            string            `json:"id"`
				CanonicalSlug string            `json:"canonical_slug"`
				Pricing       map[string]any    `json:"pricing"`
				Endpoints     []string          `json:"endpoints"`
				Architecture  map[string]any    `json:"architecture"`
				Links         map[string]string `json:"links"`
			} `json:"data"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&out)
		resp.Body.Close()
		cancel()
		if err != nil {
			return nil, fmt.Errorf("model catalog unreadable: %w", err)
		}
		listing := "/api/v1/models" + q
		for _, m := range out.Data {
			if _, seen := cat[m.ID]; seen {
				continue
			}
			e := CatalogEntry{Slug: m.ID, Listed: true, Listing: listing, CanonicalSlug: m.CanonicalSlug, Endpoints: m.Endpoints, PriceRaw: m.Pricing}
			e.InputModalities = strs(m.Architecture["input_modalities"])
			e.OutputModalities = strs(m.Architecture["output_modalities"])
			e.Price = Price{Prompt: num(m.Pricing["prompt"]), Completion: num(m.Pricing["completion"]), Audio: num(m.Pricing["audio"])}
			cat[m.ID] = e
		}
	}
	return cat, nil
}

// Verify returns the entry for slug, or an unlisted marker.
func (c Catalog) Verify(slug string) CatalogEntry {
	if e, ok := c[slug]; ok {
		return e
	}
	return CatalogEntry{Slug: slug}
}

func strs(v any) []string {
	a, _ := v.([]any)
	var out []string
	for _, x := range a {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func num(v any) float64 {
	switch x := v.(type) {
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	case float64:
		return x
	}
	return 0
}
