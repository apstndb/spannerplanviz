package mermaid

import "testing"

func Test_escapeMermaidEdgeLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "pipe", input: "a|b", want: "a#124;b"},
		{name: "hash", input: "a#b", want: "a#35;b"},
		{name: "angle brackets", input: "a<b>", want: "a#60;b#62;"},
		{name: "newline", input: "a\nb", want: "a b"},
		{name: "combined", input: "Map|Split\nRange", want: "Map#124;Split Range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := escapeMermaidEdgeLabel(tt.input); got != tt.want {
				t.Fatalf("escapeMermaidEdgeLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
