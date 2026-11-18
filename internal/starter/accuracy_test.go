package starter

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

// Story: an empty-library newcomer can start with about 30 excellent sets.
// These fingerprints protect the reviewed public-domain wording, not just shape.
// Editions and review scope are recorded in AUDIT-STARTER.md.
func TestStory_StarterShelf_ReviewedTextAndCounts(t *testing.T) {
	counts := map[string]int{
		"kitchen-numbers":        22,
		"nato-alphabet":          26,
		"the-creeds":             40,
		"mental-models":          24,
		"cognitive-biases":       24,
		"logic-and-fallacies":    24,
		"words-worth-owning":     24,
		"latin-and-greek-roots":  24,
		"rhetoric-and-writing":   20,
		"philosophy":             24,
		"psychology":             24,
		"economics":              24,
		"mathematics":            24,
		"statistics":             24,
		"physics":                24,
		"chemistry":              24,
		"biology":                24,
		"computer-science":       24,
		"astronomy":              24,
		"earth-and-climate":      24,
		"human-body":             24,
		"world-history":          24,
		"american-civics":        24,
		"world-geography":        24,
		"world-religions":        24,
		"art-history":            22,
		"music":                  24,
		"great-books":            24,
		"personal-finance":       24,
		"passages-worth-keeping": 36,
	}
	if len(All()) != len(counts) {
		t.Fatalf("got %d sets, want %d", len(All()), len(counts))
	}
	for slug, want := range counts {
		s, ok := Get(slug)
		if !ok || s.Count() != want {
			t.Errorf("%s: count %d, want %d", slug, s.Count(), want)
		}
	}
	hashes := map[string]string{
		"the-creeds/Apostles' Creed":                         "66552f10bbdb7fb73272df171ca3e489a5f130448647aa3c349203b7d97d5ace",
		"the-creeds/Nicene Creed":                            "3598d143aef548e1d4b5e3b2efa721d0a4e5c2b34fb976d5f3f972eb59d885ec",
		"passages-worth-keeping/The Preamble":                "9bb5c9cde7df5fba2bfdba61ee0c21a01d91490c9d2fca54453300e0b264516d",
		"passages-worth-keeping/Psalm 23":                    "0d86040244ee1686a195412eb43286ceab261ae44a917f225084cd7825ca1b11",
		"passages-worth-keeping/Gettysburg Address, opening": "380a69210e2152fe465bf74766329e8f5f94887f3d00756febc7b1a70436a34e",
		"passages-worth-keeping/Ozymandias":                  "fcf94a55492168a109fb2cb3cad194d126ac87eca82f0daee367eb07d99218d3",
	}
	for _, s := range All() {
		for _, i := range s.Ideas {
			if i.Exact != nil {
				key := s.Slug + "/" + i.Title
				got := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(i.Exact.Lines, " "))))
				if got != hashes[key] {
					t.Errorf("%s: reviewed wording changed; verify against named edition", key)
				}
			}
		}
	}
	nato, _ := Get("nato-alphabet")
	want := strings.Fields("Alfa Bravo Charlie Delta Echo Foxtrot Golf Hotel India Juliett Kilo Lima Mike November Oscar Papa Quebec Romeo Sierra Tango Uniform Victor Whiskey X-ray Yankee Zulu")
	n := 0
	for _, i := range nato.Ideas {
		for _, it := range i.Items {
			if n >= len(want) {
				t.Fatal("too many NATO entries")
			}
			if it.Q != fmt.Sprintf("NATO alphabet: %c", 'A'+n) || it.A != want[n] {
				t.Errorf("NATO entry %d: %q / %q", n, it.Q, it.A)
			}
			n++
		}
	}
	if n != len(want) {
		t.Errorf("NATO entries: %d", n)
	}
}
