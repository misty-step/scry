package starter

import "testing"

// Story: a new user with an empty library can pick from about 30 excellent
// starter sets.
func TestStory_StarterShelf_SetsAreValid(t *testing.T) {
	for _, e := range Validate() {
		t.Error(e)
	}
	want := []string{"kitchen-numbers", "nato-alphabet", "the-creeds", "mental-models", "words-worth-owning"}
	for _, slug := range want {
		if _, ok := Get(slug); !ok {
			t.Errorf("missing starter set %q", slug)
		}
	}
	nato, _ := Get("nato-alphabet")
	if nato.Count() != 26 {
		t.Errorf("NATO alphabet has %d items, want 26", nato.Count())
	}
}
