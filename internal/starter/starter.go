// Package starter holds the starter shelf: about thirty hand-written sets
// a new learner can add in one tap. Sets are embedded JSON, one per file.
package starter

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
)

//go:embed sets/*.json
var files embed.FS

// Set is one tile on the shelf. Added to a library it becomes a top-level
// idea whose sub-ideas are Ideas.
type Set struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Ideas       []Idea `json:"ideas"`
}

// Idea is one idea in a set, with its items or its exact text.
type Idea struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	// Links name related ideas: "Title" for an idea in the same set,
	// "slug:Title" for an idea in another set.
	Links []string   `json:"links,omitempty"`
	Items []Item     `json:"items,omitempty"`
	Exact *ExactText `json:"exact,omitempty"`
}

// Item is a question and its answer.
type Item struct {
	Q string `json:"q"`
	A string `json:"a"`
}

// ExactText is a text learned word for word, one item per line.
type ExactText struct {
	Title string   `json:"title"`
	Lines []string `json:"lines"`
}

// Count is the number of items the set adds.
func (s Set) Count() int {
	n := 0
	for _, i := range s.Ideas {
		n += len(i.Items)
		if i.Exact != nil {
			n += len(i.Exact.Lines)
		}
	}
	return n
}

// Link is a resolved reference to an idea in some set.
type Link struct{ Slug, Title string }

// ResolveLink splits a link written in set slug into its set and idea title.
func ResolveLink(slug, link string) Link {
	if i := strings.Index(link, ":"); i > 0 && slugRE.MatchString(link[:i]) {
		return Link{Slug: link[:i], Title: strings.TrimSpace(link[i+1:])}
	}
	return Link{Slug: slug, Title: link}
}

var (
	once   sync.Once
	sets   []Set
	bySlug map[string]Set
	slugRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

func load() {
	names, err := fs.Glob(files, "sets/*.json")
	if err != nil {
		panic(err)
	}
	sort.Strings(names)
	bySlug = map[string]Set{}
	for _, name := range names {
		raw, err := files.ReadFile(name)
		if err != nil {
			panic(err)
		}
		var s Set
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&s); err != nil {
			panic(fmt.Sprintf("starter set %s: %v", name, err))
		}
		sets = append(sets, s)
		bySlug[s.Slug] = s
	}
}

// All returns every set in shelf order.
func All() []Set { once.Do(load); return sets }

// Get returns the set with the given slug.
func Get(slug string) (Set, bool) { once.Do(load); s, ok := bySlug[slug]; return s, ok }

// Validate checks every embedded set against the shelf's rules and returns
// each problem found. The tests fail on any problem.
func Validate() []string {
	once.Do(load)
	var errs []string
	bad := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }
	names, _ := fs.Glob(files, "sets/*.json")
	sort.Strings(names)
	oneLine := func(s string) bool { return s != "" && !strings.ContainsAny(s, "\n\r\t") && strings.TrimSpace(s) == s }
	titles := map[Link]bool{}
	for _, s := range sets {
		for _, i := range s.Ideas {
			titles[Link{s.Slug, i.Title}] = true
		}
	}
	seen := map[string]bool{}
	for n, s := range sets {
		file := path.Base(names[n])
		where := file
		if !slugRE.MatchString(s.Slug) {
			bad("%s: slug %q must be lowercase words joined by hyphens", where, s.Slug)
		}
		if !regexp.MustCompile(`^\d\d-` + regexp.QuoteMeta(s.Slug) + `\.json$`).MatchString(file) {
			bad("%s: file name must be NN-%s.json", where, s.Slug)
		}
		if seen[s.Slug] {
			bad("%s: duplicate slug %q", where, s.Slug)
		}
		seen[s.Slug] = true
		if !oneLine(s.Title) || len([]rune(s.Title)) > 40 {
			bad("%s: title must be one line of at most 40 characters", where)
		}
		if !oneLine(s.Description) || len([]rune(s.Description)) > 90 {
			bad("%s: description must be one line of at most 90 characters (has %d)", where, len([]rune(s.Description)))
		}
		if len(s.Ideas) < 2 || len(s.Ideas) > 7 {
			bad("%s: needs 2 to 7 ideas, has %d", where, len(s.Ideas))
		}
		if c := s.Count(); c < 8 || c > 40 {
			bad("%s: needs 8 to 40 items, has %d", where, c)
		}
		ideaSeen, qSeen := map[string]bool{}, map[string]bool{}
		for _, i := range s.Ideas {
			iw := where + " / " + i.Title
			if !oneLine(i.Title) || len([]rune(i.Title)) > 48 {
				bad("%s: idea title must be one line of at most 48 characters", iw)
			}
			if strings.Contains(i.Title, ":") {
				bad("%s: idea titles may not contain a colon", iw)
			}
			if ideaSeen[strings.ToLower(i.Title)] {
				bad("%s: duplicate idea title", iw)
			}
			ideaSeen[strings.ToLower(i.Title)] = true
			if !oneLine(i.Summary) || len([]rune(i.Summary)) > 140 {
				bad("%s: summary must be one line of at most 140 characters (has %d)", iw, len([]rune(i.Summary)))
			}
			if (len(i.Items) > 0) == (i.Exact != nil) {
				bad("%s: an idea has either items or one exact text", iw)
			}
			if i.Exact == nil && len(i.Items) < 2 {
				bad("%s: needs at least 2 items", iw)
			}
			for _, it := range i.Items {
				if !oneLine(it.Q) || len([]rune(it.Q)) > 170 {
					bad("%s: question must be one line of at most 170 characters: %q", iw, it.Q)
				}
				if !oneLine(it.A) || len([]rune(it.A)) > 140 {
					bad("%s: answer must be one line of at most 140 characters: %q", iw, it.A)
				}
				if qSeen[strings.ToLower(it.Q)] {
					bad("%s: duplicate question %q", iw, it.Q)
				}
				qSeen[strings.ToLower(it.Q)] = true
			}
			if e := i.Exact; e != nil {
				if !oneLine(e.Title) || len(e.Lines) < 2 {
					bad("%s: an exact text needs a title and at least 2 lines", iw)
				}
				for _, l := range e.Lines {
					if !oneLine(l) || len(strings.Fields(l)) > 40 {
						bad("%s: exact lines are one line of at most 40 words: %q", iw, l)
					}
				}
			}
			for _, l := range i.Links {
				r := ResolveLink(s.Slug, l)
				if !titles[r] {
					bad("%s: link %q does not name an idea", iw, l)
				}
				if r == (Link{s.Slug, i.Title}) {
					bad("%s: links to itself", iw)
				}
			}
		}
	}
	if len(sets) < 28 || len(sets) > 32 {
		bad("the shelf holds about 30 sets; found %d", len(sets))
	}
	return errs
}
