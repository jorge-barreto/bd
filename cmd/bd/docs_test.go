package main

import (
	"strings"
	"testing"
)

func TestDocsText(t *testing.T) {
	if docsText == "" {
		t.Fatal("docsText is empty")
	}

	// Should reference all major commands
	for _, keyword := range []string{
		"create", "show", "update", "close", "ready",
		"list", "search", "dep", "deps", "delete",
		"--json", "--parent", "--title",
	} {
		if !strings.Contains(docsText, keyword) {
			t.Errorf("docsText missing keyword %q", keyword)
		}
	}

	// Should be concise (under 2KB)
	if len(docsText) > 2048 {
		t.Errorf("docsText is %d bytes, want <= 2048", len(docsText))
	}
}
