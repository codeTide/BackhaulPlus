package cmd

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var tomlFenceRe = regexp.MustCompile("(?s)```toml\\s*\\n(.*?)```")

// TestReadmeServerSamplesAreValid extracts every ```toml fenced block in the
// README that defines a [[server]] and runs it through the real
// load -> defaults -> validate pipeline, ensuring no documented sample is
// invalid (e.g. an empty raw_ports with no SNI router).
func TestReadmeServerSamplesAreValid(t *testing.T) {
	data, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	matches := tomlFenceRe.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		t.Fatal("no toml code blocks found in README.md")
	}

	serverSamples := 0
	for i, m := range matches {
		block := m[1]
		if !strings.Contains(block, "[[server]]") {
			continue // skip client-only or snippet blocks
		}
		serverSamples++

		if err := loadAndValidate(t, block); err != nil {
			t.Errorf("README server sample #%d is invalid: %v\n---\n%s\n---", i, err, block)
		}
	}

	if serverSamples == 0 {
		t.Fatal("expected at least one [[server]] sample in README.md")
	}
	t.Logf("validated %d README server samples", serverSamples)
}
