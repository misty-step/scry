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
		for _, dep := range []string{"github.com/anthropics/anthropic-sdk-go/v2 ", "golang.org/x/crypto ", "modernc.org/sqlite "} {
			if strings.Contains(line, dep) && strings.Contains(line, "// indirect") {
				t.Errorf("direct dependency marked indirect: %s", line)
			}
		}
	}
}
