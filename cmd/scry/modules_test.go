package main

import (
	"os"
	"strings"
	"testing"
)

func TestG19_DirectDependenciesAreDeclaredDirect(t *testing.T) {
	b, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		for _, dep := range []string{"golang.org/x/crypto ", "modernc.org/sqlite "} {
			if strings.Contains(line, dep) && strings.Contains(line, "// indirect") {
				t.Errorf("direct dependency marked indirect: %s", line)
			}
		}
	}
}

func TestCorrection_NoAnthropicSDK(t *testing.T) {
	b, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "anthropic-sdk") {
		t.Fatal("Anthropic SDK must be removed")
	}
}
