// Package dot generates Graphviz DOT source text for a built plan without
// depending on a Graphviz runtime.
//
// This package is the single source of truth for graph construction: the
// graphviz package renders svg/png by generating this DOT source and handing it
// to the Graphviz runtime, and callers that lay out the graph elsewhere (for
// example in a browser-side Graphviz build) can consume the source directly
// without linking github.com/goccy/go-graphviz and its WASM/font stack.
package dot

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/apstndb/spannerplanviz/internal/graphmodel"
	"github.com/apstndb/spannerplanviz/visualize"
)

// Options configures DOT source generation.
type Options struct {
	// ShowQuery adds a node containing the query text, linked from the root.
	ShowQuery bool
	// ShowQueryStats adds query statistics to the query text node.
	ShowQueryStats bool
}

// Renderer generates DOT source for a built plan.
type Renderer struct {
	Options Options
}

// NewRenderer returns a DOT source renderer.
func NewRenderer(opts Options) *Renderer {
	return &Renderer{Options: opts}
}

// Render writes DOT source for plan to w.
func (r *Renderer) Render(ctx context.Context, w io.Writer, plan *visualize.Plan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return writeDOT(w, plan, r.Options)
}

// Source returns DOT source text with default options.
func Source(plan *visualize.Plan) (string, error) {
	return SourceWithOptions(plan, Options{})
}

// SourceWithOptions returns DOT source text using opts.
func SourceWithOptions(plan *visualize.Plan, opts Options) (string, error) {
	var buf strings.Builder
	if err := writeDOT(&buf, plan, opts); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func writeDOT(w io.Writer, plan *visualize.Plan, opts Options) error {
	if plan == nil || plan.Root == nil {
		return fmt.Errorf("cannot render dot: plan is nil")
	}

	graph, err := visualize.BuildGraph(plan, visualize.GraphOptions{
		ShowQuery:      opts.ShowQuery,
		ShowQueryStats: opts.ShowQueryStats,
	})
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("digraph {\n")
	// These graph attributes (fontname, rankdir, start) are the sole source of
	// truth: the graphviz package embeds this DOT source and does not re-apply
	// them on the parsed graph.
	sb.WriteString("\tgraph [fontname=" + quote(`Times New Roman:style=Bold`) + ", rankdir=" + quote("BT") + ", start=" + quote("regular") + "];\n")

	writeTree(&sb, graph.Root)

	if graph.Query != nil {
		queryHTML := visualize.RenderQueryNodeHTML(graph.Query)
		sb.WriteString("\t" + quote("query") + " [label=" + htmlString(queryHTML) + ", shape=" + quote("box") + ", style=" + quote("rounded") + "];\n")
		sb.WriteString("\t" + quote(graph.Root.ID) + " -> " + quote("query") + ";\n")
	}

	sb.WriteString("}\n")
	_, err = io.WriteString(w, sb.String())
	return err
}

// writeTree emits node and edge statements in a pre-order traversal: a node,
// then each child subtree followed by the edge from that child to the node.
// The traversal is intentionally undeduplicated so a shared node's subtree is
// emitted once per incoming edge, matching the historical graphviz output.
func writeTree(sb *strings.Builder, node *graphmodel.Node) {
	writeNode(sb, node)

	for _, child := range node.Children {
		writeTree(sb, child.To)
		writeEdge(sb, node, child)
	}
}

func writeNode(sb *strings.Builder, node *graphmodel.Node) {
	sb.WriteString("\t" + quote(node.ID) +
		" [label=" + htmlString(visualize.RenderGraphvizNodeHTML(node.Label, node.ID)) +
		", shape=" + quote("box") +
		", tooltip=" + quote(node.Tooltip) + "];\n")
}

func writeEdge(sb *strings.Builder, parent *graphmodel.Node, edge graphmodel.Edge) {
	// Edges point from child to parent; combined with rankdir=BT this places
	// children below their parent, matching the graphviz package.
	sb.WriteString("\t" + quote(edge.To.ID) + " -> " + quote(parent.ID) +
		" [label=" + quote(edge.Label) +
		", style=" + quote(toEdgeStyle(edge.Style)) + "];\n")
}

func toEdgeStyle(style graphmodel.EdgeStyle) string {
	switch style {
	case graphmodel.EdgeStyleDashed:
		return "dashed"
	case graphmodel.EdgeStyleDotted:
		return "dotted"
	default:
		return "solid"
	}
}

// quote renders s as a DOT double-quoted string. In the DOT grammar the only
// escape inside a quoted string is \" — backslashes and newlines pass through
// verbatim. A value ending in a backslash would be misparsed (an escaping gap
// DOT itself has, since a trailing `\"` reads as an escaped quote); values
// emitted here cannot hit it: node names are node%d, edge labels are short
// Spanner link types, and tooltips are YAML documents ending in a newline.
func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// htmlString renders s as a DOT HTML-like string. The <...> form nests
// arbitrary balanced markup, which the HTML label builders guarantee.
func htmlString(s string) string {
	return "<" + s + ">"
}
