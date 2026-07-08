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
//     Summary lines are italic (`_..._`). Each rendered line is a separate
//     markdown paragraph (blank-line separated) so it stands on its own line
//     without relying on trailing-whitespace hard breaks.
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
// bold title, plain body lines / `key: value` pairs, and italic stat/summary
// lines, one markdown paragraph per line. fallbackID is used when the label is
// empty.
func renderD2LabelMarkdown(l graphmodel.Label, fallbackID string) string {
	var lines []string
	if l.Title != "" {
		lines = append(lines, "**"+escapeD2Markdown(l.Title)+"**")
	}
	for _, item := range l.Body {
		if item.KV != nil {
			lines = append(lines, escapeD2Markdown(item.KV.Key)+": "+escapeD2Markdown(item.KV.Value))
		} else {
			lines = append(lines, escapeD2Markdown(item.Text))
		}
	}
	for _, kv := range l.Stats {
		lines = append(lines, "_"+escapeD2Markdown(kv.Key)+": "+escapeD2Markdown(kv.Value)+"_")
	}
	for _, s := range l.Summary {
		lines = append(lines, "_"+escapeD2Markdown(s)+"_")
	}
	if len(lines) == 0 {
		lines = append(lines, escapeD2Markdown(fallbackID))
	}
	return strings.Join(lines, "\n\n")
}

// renderD2QueryMarkdown renders the query node label: the query text in bold
// (one paragraph per source line) followed by italic stat lines.
func renderD2QueryMarkdown(q *graphmodel.QueryText) string {
	var lines []string
	for _, line := range strings.Split(q.Text, "\n") {
		if line == "" {
			continue
		}
		lines = append(lines, "**"+escapeD2Markdown(line)+"**")
	}
	for _, kv := range q.Stats {
		lines = append(lines, "_"+escapeD2Markdown(kv.Key)+": "+escapeD2Markdown(kv.Value)+"_")
	}
	if len(lines) == 0 {
		lines = append(lines, "**query**")
	}
	return strings.Join(lines, "\n\n")
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
