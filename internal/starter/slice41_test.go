package starter

import (
	"strings"
	"testing"
)

// Slice 41, item 1: no starter section is named after its own answers.
// "Alfa to India" told the learner two of the nine answers under it.
func TestSlice41_Spoiler_NoSectionTitleGivesAnAnswerAway(t *testing.T) {
	for _, set := range All() {
		for _, idea := range set.Ideas {
			for _, it := range idea.Items {
				if TitleGivesAway(idea.Title, it.A) {
					t.Errorf("%s / %q gives away the answer %q", set.Slug, idea.Title, it.A)
				}
			}
		}
	}
	nato, _ := Get("nato-alphabet")
	var titles []string
	for _, idea := range nato.Ideas {
		titles = append(titles, idea.Title)
		for _, it := range idea.Items {
			if strings.Contains(strings.ToLower(idea.Title+" "+idea.Summary+" "+nato.Title+" "+nato.Description), strings.ToLower(it.A)) {
				t.Errorf("the NATO set names the answer %q outside its card", it.A)
			}
		}
	}
	if got := strings.Join(titles, " | "); got != "Letters A–I | Letters J–R | Letters S–Z" {
		t.Errorf("NATO sections are %q, want neutral letter ranges", got)
	}
}

func TestSlice41_Spoiler_TitleGivesAway(t *testing.T) {
	for _, c := range []struct {
		title, answer string
		want          bool
	}{
		{"Alfa to India", "Alfa", true},
		{"Alfa to India", "India", true},
		{"Juliett to Romeo", "romeo", true},
		{"First principles and inversion", "Inversion", true},
		{"The sunk cost trap", "The sunk cost", true}, // a leading article is not the answer
		{"Letters A–I", "India", false},
		{"Letters A–I", "Alfa", false},
		{"Compound interest", "Interest earned on both the original amount and past interest", false}, // a long answer is not named by a topic
		{"Anything", "", false},
	} {
		if got := TitleGivesAway(c.title, c.answer); got != c.want {
			t.Errorf("TitleGivesAway(%q, %q) = %v, want %v", c.title, c.answer, got, c.want)
		}
	}
}

// Slice 41, item 6: the first row of the shelf is broad and instantly
// usable; the Creeds (and the other word-for-word texts) close the shelf.
func TestSlice41_ShelfOrder_BroadSetsLeadAndTheCreedsComeLast(t *testing.T) {
	all := All()
	if len(all) != 30 {
		t.Fatalf("the shelf holds %d sets, want all 30 kept", len(all))
	}
	var slugs []string
	at := map[string]int{}
	for i, s := range all {
		slugs = append(slugs, s.Slug)
		at[s.Slug] = i
	}
	if got := strings.Join(slugs[:4], ","); got != "nato-alphabet,mental-models,kitchen-numbers,world-geography" {
		t.Errorf("the first row is %s", got)
	}
	// Whatever the width of the grid (four columns at most), no word-for-word
	// text is in the first two rows.
	for _, slug := range []string{"the-creeds", "passages-worth-keeping"} {
		i, ok := at[slug]
		if !ok {
			t.Fatalf("%s was removed from the shelf; it must only move", slug)
		}
		if i < len(all)-2 {
			t.Errorf("%s is at place %d, want it at the end of the shelf", slug, i+1)
		}
	}
	for _, s := range all[:8] {
		for _, idea := range s.Ideas {
			if idea.Exact != nil {
				t.Errorf("%s (place %d) is a word-for-word text; the top of the shelf is for sets anyone can start at once", s.Slug, at[s.Slug]+1)
			}
		}
	}
}

// Slice 41, item 9: every set names one of the subject icons.
func TestSlice41_Icons_EverySetNamesAKnownIcon(t *testing.T) {
	known := map[string]bool{}
	for _, name := range Icons {
		known[name] = true
	}
	used := map[string]string{}
	for _, s := range All() {
		if !known[s.Icon] {
			t.Errorf("%s: icon %q is not in Icons", s.Slug, s.Icon)
		}
		if other, dup := used[s.Icon]; dup {
			t.Errorf("%s and %s share the icon %q", s.Slug, other, s.Icon)
		}
		used[s.Icon] = s.Slug
	}
}
