package visualize

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/apstndb/spannerplan"

	"github.com/apstndb/spannerplanviz/option"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
)

func RenderImage(ctx context.Context, rowType *sppb.StructType, queryStats *sppb.ResultSetStats, format graphviz.Format, writer io.Writer, param option.Options) error {
	if queryStats == nil || queryStats.GetQueryPlan() == nil {
		// This handles cases where the input stats or the plan itself is fundamentally missing.
		return fmt.Errorf("cannot render image: queryStats or queryPlan is nil")
	}

	// queryStats and queryStats.GetQueryPlan() are non-nil.
	// queryStats.GetQueryPlan().GetPlanNodes() could still be empty or nil.
	// spannerplan.New is expected to handle this (e.g., return an error or an "empty" QueryPlan object).
	qp, err := spannerplan.New(queryStats.GetQueryPlan().GetPlanNodes())
	if err != nil {
		return fmt.Errorf("failed to create QueryPlan: %w", err)
	}

	rootNode, err := buildTree(qp, qp.GetNodeByIndex(0), rowType, param)
	if err != nil {
		return fmt.Errorf("failed to build tree: %w", err)
	}

	// 2. Fork based on TypeFlag
	if param.TypeFlag == "mermaid" {
		return renderMermaid(rootNode, writer, qp, param, rowType)
	}

	// 3. Graphviz path
	return renderGraphViz(ctx, rowType, queryStats, format, writer, param, rootNode, qp)
}

func renderGraphViz(ctx context.Context, rowType *sppb.StructType, queryStats *sppb.ResultSetStats,
	format graphviz.Format, writer io.Writer, param option.Options, rootNode *treeNode, qp *spannerplan.QueryPlan) error {
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

	// Set the graph start type to RegularStart to ensure deterministic layout behavior.
	// The default start type for Graphviz can be random, leading to inconsistent graph renderings.
	graph.SetStart(graphviz.RegularStart)
	graph.SetFontName("Times New Roman:style=Bold")

	// Call renderGraph directly with rootNode
	// renderGraph internally handles setting RankDir, rendering the tree, and adding the query node if needed.
	err = renderGraph(graph, rootNode, qp, param, queryStats, rowType) // Pass qp
	if err != nil {
		return fmt.Errorf("failed to render graph content: %w", err) // Wrap error from renderGraph
	}

	return g.Render(ctx, graph, format, writer)
}

func renderGraph(graph *cgraph.Graph, rootNode *treeNode, qp *spannerplan.QueryPlan, param option.Options, queryStats *sppb.ResultSetStats, rowType *sppb.StructType) error {
	graph.SetRankDir(cgraph.BTRank)
	err := renderTree(graph, rootNode, qp, param, rowType)
	if err != nil {
		return err
	}

	showQueryStats := param.ShowQueryStats
	needQueryNode := (param.ShowQuery || showQueryStats) && queryStats != nil
	if needQueryNode {
		err = renderQueryNodeWithEdge(graph, queryStats, showQueryStats, rootNode.GetName()) // Use GetName()
		if err != nil {
			return err
		}
	}
	return nil
}

func renderQueryNodeWithEdge(graph *cgraph.Graph, queryStats *sppb.ResultSetStats, showQueryStats bool, rootName string) error {
	str := formatQueryNode(queryStats.GetQueryStats().GetFields(), showQueryStats)

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

func renderTree(graph *cgraph.Graph, node *treeNode, qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) error {
	err := renderNode(graph, node, qp, param, rowType) // Pass qp
	if err != nil {
		return err
	}

	for _, child := range node.Children {
		if err := renderTree(graph, child.ChildNode, qp, param, rowType); err != nil {
			return err
		}

		err = renderEdge(graph, node, child)
		if err != nil {
			return err
		}
	}
	return nil
}

func renderNode(graph *cgraph.Graph, node *treeNode, qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) error {
	n, err := graph.CreateNodeByName(node.GetName())
	if err != nil {
		return err
	}

	n.SetShape(cgraph.BoxShape)

	tooltipStr, ttErr := node.GetTooltip()
	if ttErr != nil {
		return fmt.Errorf("error getting tooltip for node %s: %w", node.GetName(), ttErr)
	}

	n.SetTooltip(tooltipStr)

	nodeHTMLStr := node.HTML(qp, param, rowType)
	nodeHTML, err := graph.StrdupHTML(nodeHTMLStr)
	if err != nil {
		return err
	}

	n.SetLabel(nodeHTML)
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
