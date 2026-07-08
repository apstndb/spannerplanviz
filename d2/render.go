// Package d2 generates D2 (https://d2lang.com) source text for a built plan.
//
// It mirrors the mermaid package shape (Source / SourceWithOptions / NewRenderer)
// and consumes the backend-neutral graph IR via visualize.BuildGraph. The output
// is unlaid-out D2 source; render it with the d2 CLI, for example
// `d2 out.d2 out.svg`.
//
// # D2 dialect decisions
//
//   - Direction: `direction: down` is emitted explicitly. Edges point from parent
//     to child, so with the top-down flow the root sits at the top and children
//     below it.
//   - Nodes are declared with flat statements (`nodeN.shape: rectangle`,
//     `nodeN.label: |md ... |`, `nodeN.tooltip: |yaml ... |`) rather than an
//     inline `{ ... }` map, because D2 block strings span multiple lines and do
//     not compose cleanly inside a single-line inline map.
//   - Labels are markdown blocks. The label Title is a bold (`**...**`) line,
//     Body lines are plain, Body key/value pairs use the `key: value` dialect
//     (chosen over the graphviz `key=value` form for readability), and Stats and
//     Summary lines are italic (`_..._`). The body lines are joined into a single
//     markdown paragraph with CommonMark hard line breaks (a trailing backslash),
//     giving tight single-line spacing that matches the graphviz/mermaid
//     backends; the bold title is kept as its own paragraph for a small visual
//     separation from the body. (An earlier version emitted every line as its own
//     blank-line-separated paragraph, which rendered with a paragraph gap between
//     every line.)
//   - Emphasis markers hug their content: any leading whitespace on a Stats or
//     Summary line is moved OUTSIDE the `_..._` markers and re-encoded as
//     `&nbsp;` entities (which d2's markdown renderer honors) so the visual indent
//     survives, and trailing whitespace is trimmed from inside the markers.
//     CommonMark requires the opening `_` to be immediately followed by, and the
//     closing `_` immediately preceded by, a non-whitespace character; a
//     space-indented summary child line such as `_   num_executions: 1_` would
//     otherwise fail to parse as emphasis and render with literal underscores.
//     See emphasizeD2Line.
//   - Markdown metacharacters in label content are backslash-escaped
//     conservatively (see escapeD2Markdown). The pipe character is not escaped;
//     it is handled at the D2 block-string layer by widening the fence.
//   - Tooltips carry the canonical YAML in a `|yaml ... |` block string. D2
//     renders tooltips as plain-text HTML title attributes (markdown and syntax
//     highlighting are not applied), so the YAML is carried verbatim; only the
//     fence width matters for escaping.
//   - Block-string fences use the documented pipe-widening rule: a run of N
//     pipes where N is one greater than the longest run of consecutive pipes in
//     the content, so the content can never contain the closing delimiter.
//   - Edges carry the Spanner link type as an unquoted label (a simple closed
//     vocabulary such as "Map" or "Split Range"). Dashed and Dotted edges get
//     `{style.stroke-dash: 3}`; D2 has no separate dotted stroke style, so both
//     map to the same dash pattern (mirroring the mermaid backend, which also
//     collapses them).
//   - The optional query node is a distinct `query` shape with rounded corners
//     (`style.border-radius: 8`) whose label is the query text in bold followed
//     by italic stat lines. It is linked with `root -> query`, placing it below
//     the plan under the top-down flow.
package d2

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/apstndb/spannerplanviz/internal/graphmodel"
	"github.com/apstndb/spannerplanviz/visualize"
)

// Renderer generates D2 source for a built plan.
type Renderer struct {
	Options Options
}

// NewRenderer returns a D2 source renderer.
func NewRenderer(opts Options) *Renderer {
	return &Renderer{Options: opts}
}

// Source returns D2 source text using plan.Build settings.
func Source(plan *visualize.Plan) (string, error) {
	if plan == nil {
		return "", fmt.Errorf("cannot render d2: plan is nil")
	}
	return SourceWithOptions(plan, Options{BuildOptions: plan.Build})
}

// SourceWithOptions returns D2 source text using opts.
func SourceWithOptions(plan *visualize.Plan, opts Options) (string, error) {
	var buf strings.Builder
	if err := writeD2(&buf, plan, opts); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Render writes D2 source for plan to w.
func (r *Renderer) Render(ctx context.Context, w io.Writer, plan *visualize.Plan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return writeD2(w, plan, r.Options)
}

func writeD2(w io.Writer, plan *visualize.Plan, opts Options) error {
	if plan == nil || plan.Root == nil {
		return fmt.Errorf("cannot render d2: plan is nil")
	}

	build := opts.BuildOptions
	build.ApplyFull()

	// Labels are built from plan.Build; carry the render-time build options via
	// a shallow copy so BuildGraph resolves labels with them.
	planForGraph := *plan
	planForGraph.Build = build
	graph, err := visualize.BuildGraph(&planForGraph, visualize.GraphOptions{
		ShowQuery:      opts.ShowQuery,
		ShowQueryStats: opts.ShowQueryStats,
	})
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("direction: down\n\n")

	visited := make(map[string]bool)
	var edges []string

	var walk func(n *graphmodel.Node)
	walk = func(n *graphmodel.Node) {
		if n == nil || visited[n.ID] {
			return
		}
		visited[n.ID] = true
		writeD2Node(&sb, n)
		for _, edge := range n.Children {
			edges = append(edges, formatD2Edge(n.ID, edge))
			walk(edge.To)
		}
	}
	walk(graph.Root)

	if graph.Query != nil {
		writeD2QueryNode(&sb, graph.Query)
		edges = append(edges, fmt.Sprintf("%s -> query\n", graph.Root.ID))
	}

	for _, edge := range edges {
		sb.WriteString(edge)
	}

	_, err = io.WriteString(w, sb.String())
	return err
}

func writeD2Node(sb *strings.Builder, n *graphmodel.Node) {
	sb.WriteString(n.ID + ".shape: rectangle\n")
	writeD2BlockString(sb, n.ID+".label", "md", renderD2LabelMarkdown(n.Label, n.ID))
	if n.Tooltip != "" {
		writeD2BlockString(sb, n.ID+".tooltip", "yaml", n.Tooltip)
	}
}

func writeD2QueryNode(sb *strings.Builder, q *graphmodel.QueryText) {
	sb.WriteString("query.shape: rectangle\n")
	sb.WriteString("query.style.border-radius: 8\n")
	writeD2BlockString(sb, "query.label", "md", renderD2QueryMarkdown(q))
}

// writeD2BlockString emits `key: |<fence><tag>` / content / `<fence>`, choosing
// the fence width dynamically so the content can never contain the closing
// delimiter.
func writeD2BlockString(sb *strings.Builder, key, tag, content string) {
	fence := d2PipeFence(content)
	sb.WriteString(key + ": " + fence + tag + "\n")
	sb.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString(fence + "\n")
}

// d2PipeFence returns a run of pipes one longer than the longest run of
// consecutive pipes in content, per D2's documented block-string widening rule
// (|md -> ||md -> |||md ...).
func d2PipeFence(content string) string {
	maxRun, run := 0, 0
	for _, r := range content {
		if r == '|' {
			run++
			if run > maxRun {
				maxRun = run
			}
		} else {
			run = 0
		}
	}
	return strings.Repeat("|", maxRun+1)
}

// renderD2LabelMarkdown renders a graphmodel.Label as markdown-block content:
// a bold title paragraph, then a single body paragraph holding the plain body
// lines / `key: value` pairs and the italic stat/summary lines, joined with
// CommonMark hard line breaks for tight single-line spacing. fallbackID is used
// when the label is empty.
func renderD2LabelMarkdown(l graphmodel.Label, fallbackID string) string {
	var title string
	if l.Title != "" {
		title = "**" + escapeD2Markdown(l.Title) + "**"
	}
	var body []string
	for _, item := range l.Body {
		if item.KV != nil {
			body = append(body, escapeD2Markdown(item.KV.Key)+": "+escapeD2Markdown(item.KV.Value))
		} else {
			body = append(body, escapeD2Markdown(item.Text))
		}
	}
	for _, kv := range l.Stats {
		body = append(body, emphasizeD2Line(kv.Key+": "+kv.Value))
	}
	for _, s := range l.Summary {
		body = append(body, emphasizeD2Line(s))
	}
	return joinD2LabelParagraphs(title, body, fallbackID)
}

// renderD2QueryMarkdown renders the query node label: the query text in bold
// followed by italic stat lines, all joined into a single paragraph with
// CommonMark hard line breaks so it renders with tight single-line spacing.
func renderD2QueryMarkdown(q *graphmodel.QueryText) string {
	var lines []string
	for _, line := range strings.Split(q.Text, "\n") {
		if line == "" {
			continue
		}
		lines = append(lines, "**"+escapeD2Markdown(line)+"**")
	}
	for _, kv := range q.Stats {
		lines = append(lines, emphasizeD2Line(kv.Key+": "+kv.Value))
	}
	if len(lines) == 0 {
		return "**query**"
	}
	return joinD2HardBreaks(lines)
}

// joinD2LabelParagraphs assembles the final label markdown: the bold title (when
// present) as its own paragraph and the body lines joined into one paragraph with
// hard line breaks. When both are empty it falls back to the escaped node ID.
func joinD2LabelParagraphs(title string, body []string, fallbackID string) string {
	var paras []string
	if title != "" {
		paras = append(paras, title)
	}
	if len(body) > 0 {
		paras = append(paras, joinD2HardBreaks(body))
	}
	if len(paras) == 0 {
		return escapeD2Markdown(fallbackID)
	}
	return strings.Join(paras, "\n\n")
}

// joinD2HardBreaks joins lines into a single markdown paragraph using CommonMark
// hard line breaks (a backslash immediately before the newline). d2's markdown
// renderer turns each into a <br/>, so the lines share one paragraph with tight
// single-line spacing rather than the paragraph gaps that blank-line separation
// produces. A trailing backslash is verified to survive d2's block-string layer.
func joinD2HardBreaks(lines []string) string {
	return strings.Join(lines, "\\\n")
}

// emphasizeD2Line wraps a stat/summary line as CommonMark emphasis (`_..._`).
// Any leading whitespace is moved OUTSIDE the emphasis markers and re-encoded as
// &nbsp; entities so the visual indent is preserved, and trailing whitespace is
// trimmed from inside the markers. This is required because CommonMark only opens
// emphasis when the `_` is immediately followed by a non-whitespace character and
// only closes it when the `_` is immediately preceded by one; a space-indented
// summary child line would otherwise render with literal underscores. A line that
// is empty after trimming gets no emphasis markers (just its preserved indent).
func emphasizeD2Line(raw string) string {
	trimmedLeft := strings.TrimLeft(raw, " \t")
	// Leading whitespace is ASCII (space/tab), so the byte-length difference is
	// also the character count; render one &nbsp; per leading whitespace char.
	indent := strings.Repeat("&nbsp;", len(raw)-len(trimmedLeft))
	inner := strings.TrimRight(trimmedLeft, " \t")
	if inner == "" {
		return indent
	}
	return indent + "_" + escapeD2Markdown(inner) + "_"
}

// formatD2Edge renders a parent->child connection with the link type as its
// label and a dashed stroke for remote (Dashed/Dotted) links.
func formatD2Edge(parentID string, edge graphmodel.Edge) string {
	var b strings.Builder
	b.WriteString(parentID + " -> " + edge.To.ID)
	if label := sanitizeD2EdgeLabel(edge.Label); label != "" {
		b.WriteString(": " + label)
	}
	if edge.Style == graphmodel.EdgeStyleDashed || edge.Style == graphmodel.EdgeStyleDotted {
		b.WriteString(" {style.stroke-dash: 3}")
	}
	b.WriteString("\n")
	return b.String()
}

// sanitizeD2EdgeLabel keeps edge labels on a single line. Spanner link types are
// a simple closed vocabulary (for example "Map" or "Split Range") that needs no
// quoting; newlines are collapsed defensively.
func sanitizeD2EdgeLabel(label string) string {
	label = strings.ReplaceAll(label, "\r\n", " ")
	label = strings.ReplaceAll(label, "\n", " ")
	label = strings.ReplaceAll(label, "\r", " ")
	return strings.TrimSpace(label)
}

// d2MarkdownEscaper backslash-escapes the ASCII markdown metacharacters that
// could otherwise trigger formatting. Every replaced character is ASCII
// punctuation, which CommonMark allows to be backslash-escaped, so escaping is
// always safe. The pipe character is deliberately not escaped: it is handled at
// the block-string layer by widening the fence.
var d2MarkdownEscaper = strings.NewReplacer(
	`\`, `\\`,
	"`", "\\`",
	"*", `\*`,
	"_", `\_`,
	"{", `\{`,
	"}", `\}`,
	"[", `\[`,
	"]", `\]`,
	"(", `\(`,
	")", `\)`,
	"#", `\#`,
	"+", `\+`,
	"-", `\-`,
	".", `\.`,
	"!", `\!`,
	"<", `\<`,
	">", `\>`,
	"~", `\~`,
	"&", `\&`,
)

func escapeD2Markdown(s string) string {
	return d2MarkdownEscaper.Replace(s)
}
