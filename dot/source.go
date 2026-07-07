// Package dot generates Graphviz DOT source text for a built plan without
// depending on a Graphviz runtime.
//
// It mirrors the graph semantics of the graphviz package (same nodes, edges,
// attributes, and traversal order) but only emits DOT source, so callers that
// lay out the graph elsewhere (for example in a browser-side Graphviz build)
// do not need to link github.com/goccy/go-graphviz and its WASM/font stack.
// Parity with the graphviz package is enforced by tests that parse both
// outputs and compare them structurally.
package dot

import (
	"context"
	"fmt"
	"io"
	"strings"

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

	var sb strings.Builder
	sb.WriteString("digraph {\n")
	// Keep these graph attributes in sync with graphviz.Renderer
	// (SetFontName/SetRankDir/SetStart in the graphviz package).
	sb.WriteString("\tgraph [fontname=" + quote(`Times New Roman:style=Bold`) + ", rankdir=" + quote("BT") + ", start=" + quote("regular") + "];\n")

	if err := writeTree(&sb, plan.Root, plan); err != nil {
		return err
	}

	if (opts.ShowQuery || opts.ShowQueryStats) && plan.QueryStats != nil {
		queryHTML := visualize.FormatQueryNode(plan.QueryStats.GetQueryStats().GetFields(), opts.ShowQueryStats)
		sb.WriteString("\t" + quote("query") + " [label=" + htmlString(queryHTML) + ", shape=" + quote("box") + ", style=" + quote("rounded") + "];\n")
		sb.WriteString("\t" + quote(plan.Root.GetName()) + " -> " + quote("query") + ";\n")
	}

	sb.WriteString("}\n")
	_, err := io.WriteString(w, sb.String())
	return err
}

// writeTree emits node and edge statements in the same pre-order traversal as
// the graphviz package: a node, then each child subtree followed by the edge
// from that child to the node.
func writeTree(sb *strings.Builder, node *visualize.TreeNode, plan *visualize.Plan) error {
	if err := writeNode(sb, node, plan); err != nil {
		return err
	}

	for _, child := range node.Children {
		if err := writeTree(sb, child.ChildNode, plan); err != nil {
			return err
		}
		writeEdge(sb, node, child)
	}
	return nil
}

func writeNode(sb *strings.Builder, node *visualize.TreeNode, plan *visualize.Plan) error {
	tooltip, err := node.GetTooltip()
	if err != nil {
		return fmt.Errorf("error getting tooltip for node %s: %w", node.GetName(), err)
	}

	sb.WriteString("\t" + quote(node.GetName()) +
		" [label=" + htmlString(node.HTML(plan.Build, plan.RowType)) +
		", shape=" + quote("box") +
		", tooltip=" + quote(tooltip) + "];\n")
	return nil
}

func writeEdge(sb *strings.Builder, parent *visualize.TreeNode, edge *visualize.Link) {
	// Edges point from child to parent; combined with rankdir=BT this places
	// children below their parent, matching the graphviz package.
	sb.WriteString("\t" + quote(edge.ChildNode.GetName()) + " -> " + quote(parent.GetName()) +
		" [label=" + quote(edge.ChildType) +
		", style=" + quote(toEdgeStyle(edge.Style)) + "];\n")
}

func toEdgeStyle(style visualize.EdgeStyle) string {
	switch style {
	case visualize.EdgeStyleDashed:
		return "dashed"
	case visualize.EdgeStyleDotted:
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
