package mermaid

import "testing"

func Test_escapeMermaidEdgeLabel(t *testing.T) {
	t.Parallel()
	if got := escapeMermaidEdgeLabel("a|b"); got != "a#124;b" {
		t.Fatalf("escapeMermaidEdgeLabel() = %q", got)
	}
}
