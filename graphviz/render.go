package graphviz

import (
	"context"
	"fmt"
	"io"
	"log"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spannerplanviz/visualize"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
)

// Render writes a Graphviz diagram for plan to w.
func (r *Renderer) Render(ctx context.Context, w io.Writer, plan *visualize.Plan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if plan == nil || plan.Root == nil {
		return fmt.Errorf("cannot render graphviz: plan is nil")
	}
	if r.Options.Format == "" {
		return fmt.Errorf("graphviz format is required")
	}
	return render(ctx, w, r.Options.Format, plan, r.Options)
}

func render(ctx context.Context, w io.Writer, format Format, plan *visualize.Plan, opts Options) error {
	g, err := graphviz.New(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if err := g.Close(); err != nil {
			log.Print(err)
		}
	}()

	graph, err := g.Graph()
	if err != nil {
		return err
	}
	defer func() {
		if err := graph.Close(); err != nil {
			log.Print(err)
		}
	}()

	graph.SetStart(graphviz.RegularStart)
	graph.SetFontName("Times New Roman:style=Bold")

	if err := renderGraph(graph, plan, opts); err != nil {
		return fmt.Errorf("failed to render graph content: %w", err)
	}

	return g.Render(ctx, graph, graphviz.Format(format), w)
}

func renderGraph(graph *cgraph.Graph, plan *visualize.Plan, opts Options) error {
	graph.SetRankDir(cgraph.BTRank)
	if err := renderTree(graph, plan.Root, plan); err != nil {
		return err
	}

	needQueryNode := (opts.ShowQuery || opts.ShowQueryStats) && plan.QueryStats != nil
	if needQueryNode {
		if err := renderQueryNodeWithEdge(graph, plan.QueryStats, opts.ShowQueryStats, plan.Root.GetName()); err != nil {
			return err
		}
	}
	return nil
}

func renderQueryNodeWithEdge(graph *cgraph.Graph, queryStats *sppb.ResultSetStats, showQueryStats bool, rootName string) error {
	str := visualize.FormatQueryNode(queryStats.GetQueryStats().GetFields(), showQueryStats)

	n, err := renderQueryNode(graph, str)
	if err != nil {
		return err
	}

	gvRootNode, err := graph.NodeByName(rootName)
	if err != nil {
		return err
	}

	_, err = graph.CreateEdgeByName("", gvRootNode, n)
	if err != nil {
		return err
	}
	return nil
}

func renderTree(graph *cgraph.Graph, node *visualize.TreeNode, plan *visualize.Plan) error {
	if err := renderNode(graph, node, plan); err != nil {
		return err
	}

	for _, child := range node.Children {
		if err := renderTree(graph, child.ChildNode, plan); err != nil {
			return err
		}
		if err := renderEdge(graph, node, child); err != nil {
			return err
		}
	}
	return nil
}

func renderNode(graph *cgraph.Graph, node *visualize.TreeNode, plan *visualize.Plan) error {
	n, err := graph.CreateNodeByName(node.GetName())
	if err != nil {
		return err
	}

	n.SetShape(cgraph.BoxShape)

	tooltipStr, err := node.GetTooltip()
	if err != nil {
		return fmt.Errorf("error getting tooltip for node %s: %w", node.GetName(), err)
	}

	n.SetTooltip(tooltipStr)

	nodeHTMLStr := node.HTML(plan.Build, plan.RowType)
	nodeHTML, err := graph.StrdupHTML(nodeHTMLStr)
	if err != nil {
		return err
	}

	n.SetLabel(nodeHTML)
	return nil
}

func renderEdge(graph *cgraph.Graph, parent *visualize.TreeNode, edge *visualize.Link) error {
	gvChildNode, err := graph.NodeByName(edge.ChildNode.GetName())
	if err != nil {
		return err
	}

	gvNode, err := graph.NodeByName(parent.GetName())
	if err != nil {
		return err
	}

	ed, err := graph.CreateEdgeByName("", gvChildNode, gvNode)
	if err != nil {
		return err
	}

	ed.SetStyle(toCgraphEdgeStyle(edge.Style))
	ed.SetLabel(edge.ChildType)
	return nil
}

func renderQueryNode(graph *cgraph.Graph, queryNodeStr string) (*cgraph.Node, error) {
	s, err := graph.StrdupHTML(queryNodeStr)
	if err != nil {
		return nil, err
	}

	n, err := graph.CreateNodeByName("query")
	if err != nil {
		return nil, err
	}

	n.SetLabel(s)
	n.SetShape(cgraph.BoxShape)
	n.SetStyle(cgraph.RoundedNodeStyle)

	return n, nil
}

func toCgraphEdgeStyle(style visualize.EdgeStyle) cgraph.EdgeStyle {
	switch style {
	case visualize.EdgeStyleDashed:
		return cgraph.DashedEdgeStyle
	case visualize.EdgeStyleDotted:
		return cgraph.DottedEdgeStyle
	default:
		return cgraph.SolidEdgeStyle
	}
}
