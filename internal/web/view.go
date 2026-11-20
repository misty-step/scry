package web

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	"scry/internal/store"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

var funcs = template.FuncMap{
	"date":     func(t time.Time) string { return t.Format("January 2") },
	"dateY":    func(t time.Time) string { return t.Format("January 2, 2006") },
	"short":    func(t time.Time) string { return t.Format("Jan 2, 2006") },
	"money":    money,
	"amount":   func(cents int64) string { return fmt.Sprintf("$%d.%02d", cents/100, cents%100) },
	"micros":   func(m int64) string { return money((m + 5000) / 10000) },
	"plural":   plural,
	"add":      func(a, b int) int { return a + b },
	"sub":      func(a, b int) int { return a - b },
	"lower":    strings.ToLower,
	"yPct":     func(i, n int) string { return fmt.Sprintf("%.2f%%", (float64(i)+0.5)/float64(n)*100) },
	"truncate": truncate,
	"dict": func(kv ...any) map[string]any {
		m := map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[fmt.Sprint(kv[i])] = kv[i+1]
		}
		return m
	},
}

// money prints cents as dollars: $5 for round amounts, $4.50 otherwise.
func money(cents int64) string {
	if cents%100 == 0 {
		return fmt.Sprintf("$%d", cents/100)
	}
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func truncate(n int, s string) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// dueLabel says when an item is next due, in the learner's own days.
func dueLabel(u store.User, it store.Item, now time.Time) string {
	loc := u.Location()
	day := func(t time.Time) time.Time {
		t = t.In(loc)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
	days := int(day(it.Sched.Due).Sub(day(now)).Hours() / 24)
	switch {
	case days <= 0:
		return "due today"
	case days == 1:
		return "due tomorrow"
	default:
		return fmt.Sprintf("in %d days", days)
	}
}

// markParts wraps each of parts, where it occurs in text, in <mark>.
// It is how close parts of an answer and matched search words get their
// highlighter.
func markParts(text string, parts []string) template.HTML {
	type span struct{ a, b int }
	var spans []span
	lower := strings.ToLower(text)
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		from := 0
		for {
			i := strings.Index(lower[from:], p)
			if i < 0 {
				break
			}
			spans = append(spans, span{from + i, from + i + len(p)})
			from += i + len(p)
		}
	}
	if len(spans) == 0 || len(lower) != len(text) {
		return template.HTML(html.EscapeString(text))
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].a < spans[j].a })
	var b strings.Builder
	pos := 0
	for _, s := range spans {
		if s.a < pos {
			continue
		}
		b.WriteString(html.EscapeString(text[pos:s.a]))
		b.WriteString("<mark>" + html.EscapeString(text[s.a:s.b]) + "</mark>")
		pos = s.b
	}
	b.WriteString(html.EscapeString(text[pos:]))
	return template.HTML(b.String())
}
