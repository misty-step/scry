package generation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/misty-step/scry/internal/store"
)

const exaTimeout = 20 * time.Second
const exaResponseLimit = 2 << 20

type exaClient struct {
	endpoint string
	key      string

	http *http.Client
}

// ValidateExaEndpoint rejects credential-bearing and plaintext off-host origins.
func ValidateExaEndpoint(endpoint string) error {
	_, err := newExaClient(strings.TrimSpace(endpoint), "", nil)
	return err
}

func newExaClient(endpoint, key string, transport *http.Client) (*exaClient, error) {
	if endpoint == "" {
		endpoint = "https://api.exa.ai"
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("Web search endpoint is invalid; use its HTTPS origin or a loopback gateway.")
	}
	loopback := strings.EqualFold(u.Hostname(), "localhost")
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, errors.New("Web search endpoint must use HTTPS, except for a loopback gateway.")
	}
	if len(key) > 4096 || strings.IndexFunc(key, unicode.IsControl) >= 0 {
		return nil, errors.New("Web search credentials are invalid.")
	}
	client := http.Client{}
	if transport != nil {
		client = *transport
	}
	client.Timeout = exaTimeout
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &exaClient{endpoint: strings.TrimRight(endpoint, "/"), key: key, http: &client}, nil
}

func (c *exaClient) research(ctx context.Context, kind, input string) ([]store.DocumentContent, *int64, error) {
	zero := int64(0)
	var path string
	var payload any
	switch kind {
	case "link":
		u, err := url.Parse(input)
		if err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return nil, &zero, errors.New("The saved link is invalid; add a complete web address.")
		}
		path, payload = "/contents", map[string]any{"urls": []string{input}, "text": map[string]int{"maxCharacters": 30000}}
	case "topic":
		query := []rune(input)
		if len(query) > 400 {
			query = query[:400]
		}
		path, payload = "/search", map[string]any{"query": string(query), "type": "auto", "numResults": 6, "contents": map[string]any{"text": map[string]int{"maxCharacters": 4000}}}
	default:
		// Only an explicitly chosen Topic is searched and only a chosen Link is
		// read; any other material, including legacy unlabeled text, never
		// leaves for the web.
		return nil, &zero, errors.New("Web research applies only to a Topic or a Link.")
	}
	if c.key == "" {
		return nil, &zero, nil
	}
	body, _ := json.Marshal(payload)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+path, bytes.NewReader(body))
	if err != nil {
		zero := int64(0)
		return nil, &zero, errors.New("Web search request could not be prepared.")
	}
	request.Header.Set("x-api-key", c.key)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		return nil, nil, errors.New("Web search did not complete; check its receipt before retrying.")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, exaResponseLimit+1))
	if err != nil || len(data) > exaResponseLimit {
		return nil, nil, errors.New("Web search response was interrupted or too large; check its receipt before retrying.")
	}
	if !utf8.Valid(data) || checkJSON(data, 24) != nil {
		return nil, nil, errors.New("Web search returned unreadable data; check its receipt before retrying.")
	}
	var envelope struct {
		Results []struct {
			URL       string `json:"url"`
			Title     string `json:"title"`
			Published string `json:"publishedDate"`
			Text      string `json:"text"`
		} `json:"results"`
		Cost struct {
			Total json.RawMessage `json:"total"`
		} `json:"costDollars"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return nil, nil, errors.New("Web search returned unreadable results.")
	}
	cost := costMicros(envelope.Cost.Total)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, cost, fmt.Errorf("Web search returned HTTP %d; check its receipt before retrying.", response.StatusCode)
	}
	if len(envelope.Results) > 6 && kind != "link" {
		envelope.Results = envelope.Results[:6]
	}
	if len(envelope.Results) > 1 && kind == "link" {
		envelope.Results = envelope.Results[:1]
	}
	documents := make([]store.DocumentContent, 0, len(envelope.Results))
	for _, hit := range envelope.Results {
		text := stripExaControls(hit.Text)
		link, title, published := stripExaControls(hit.URL), stripExaControls(hit.Title), stripExaControls(hit.Published)
		u, parseErr := url.Parse(link)
		if strings.TrimSpace(text) == "" || len(text) > 256<<10 || len(title) > 500 || len(published) > 64 ||
			parseErr != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || len(link) > 2048 {
			continue
		}
		kindValue := "search_result"
		if kind == "link" {
			kindValue = "page"
		}
		documents = append(documents, store.DocumentContent{Kind: kindValue, URL: link, Title: title, Published: published, Text: text, Provider: "exa"})
	}
	return documents, cost, nil
}

func stripExaControls(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			if r == '\n' || r == '\r' || r == '\t' {
				return ' '
			}
			return -1
		}
		return r
	}, s)
}
