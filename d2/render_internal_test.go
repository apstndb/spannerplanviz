package d2

import (
	"strings"
	"testing"

	"github.com/apstndb/spannerplanviz/internal/graphmodel"
)

// TestEmphasizeD2Line_leadingWhitespace verifies that a stat/summary line with
// leading indentation is emitted with the whitespace moved OUTSIDE the emphasis
// markers (as &nbsp; entities) and trailing whitespace trimmed from inside them,
// so CommonMark parses it as emphasis instead of rendering literal underscores.
func TestEmphasizeD2Line_leadingWhitespace(t *testing.T) {
	got := emphasizeD2Line("   num_executions: 1  ")
	want := "&nbsp;&nbsp;&nbsp;_num\\_executions: 1_"
	if got != want {
		t.Fatalf("emphasizeD2Line() = %q, want %q", got, want)
	}
	// The opening _ must hug non-whitespace and the closing _ must be preceded
	// by non-whitespace; assert no space sits directly inside the markers.
	if strings.Contains(got, "_ ") || strings.Contains(got, " _") {
		t.Errorf("emphasis markers must hug content, got %q", got)
	}
}

// TestEmphasizeD2Line_emptyAfterTrim verifies a line that is only whitespace gets
// no emphasis markers (just its preserved indent).
func TestEmphasizeD2Line_emptyAfterTrim(t *testing.T) {
	if got := emphasizeD2Line("   "); got != "&nbsp;&nbsp;&nbsp;" {
		t.Errorf("emphasizeD2Line(whitespace) = %q, want preserved indent only", got)
	}
	if got := emphasizeD2Line(""); got != "" {
		t.Errorf("emphasizeD2Line(empty) = %q, want empty", got)
	}
}

// TestRenderD2LabelMarkdown_hardBreaksNotBlankLines verifies body lines are joined
// with CommonMark hard line breaks (trailing backslash before newline) into a
// single paragraph, not separated by blank lines, and that indented summary lines
// carry valid emphasis.
func TestRenderD2LabelMarkdown_hardBreaksNotBlankLines(t *testing.T) {
	label := graphmodel.Label{
		Title: "Node",
		Stats: []graphmodel.KV{{Key: "rows", Value: "3069 rows"}},
		Summary: []string{
			"execution_summary:",
			"   num_executions: 1",
		},
	}
	got := renderD2LabelMarkdown(label, "node0")

	// The title is its own paragraph; the body is a single hard-break-joined one.
	if !strings.HasPrefix(got, "**Node**\n\n") {
		t.Errorf("title should be its own paragraph, got:\n%s", got)
	}
	body := strings.TrimPrefix(got, "**Node**\n\n")
	if strings.Contains(body, "\n\n") {
		t.Errorf("body lines must not be blank-line separated, got:\n%s", body)
	}
	if !strings.Contains(got, "\\\n") {
		t.Errorf("body lines must be joined with hard line breaks (\\\\n), got:\n%s", got)
	}
	// Indented summary child renders as valid emphasis with indent outside.
	if !strings.Contains(got, "&nbsp;&nbsp;&nbsp;_num\\_executions: 1_") {
		t.Errorf("indented summary line lacks valid emphasis, got:\n%s", got)
	}
	// No literal leading-space-inside-emphasis pattern remains.
	if strings.Contains(got, "_ ") || strings.Contains(got, " _") {
		t.Errorf("no emphasis marker may hug whitespace, got:\n%s", got)
	}
}
