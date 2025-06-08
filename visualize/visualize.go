package visualize

import (
	"context"
	"io"
	"log"

	"github.com/apstndb/spannerplan"

	"github.com/apstndb/spannerplanviz/option"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"

	"fmt"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
)

func RenderImage(ctx context.Context, rowType *sppb.StructType, queryStats *sppb.ResultSetStats, format graphviz.Format, writer io.Writer, param option.Options) error {
	if queryStats == nil || queryStats.GetQueryPlan() == nil {
		// This handles cases where the input stats or the plan itself is fundamentally missing.
		if param.TypeFlag == "mermaid" {
			if _, err := writer.Write([]byte("graph TD\n")); err != nil {
				return fmt.Errorf("failed to write empty mermaid graph for nil plan: %w", err)
			}
			return nil
		}
		return fmt.Errorf("cannot render image: queryStats or queryPlan is nil")
	}

	// queryStats and queryStats.GetQueryPlan() are non-nil.
	// queryStats.GetQueryPlan().GetPlanNodes() could still be empty or nil.
	// spannerplan.New is expected to handle this (e.g., return an error or an "empty" QueryPlan object).
	qp, err := spannerplan.New(queryStats.GetQueryPlan().GetPlanNodes())
	if err != nil {
		// If spannerplan.New errors (e.g., it considers empty PlanNodes an error, or plan is malformed)
		if param.TypeFlag == "mermaid" {
			// Output an empty Mermaid graph on QueryPlan creation failure.
			if _, writeErr := writer.Write([]byte("graph TD\n")); writeErr != nil {
				return fmt.Errorf("failed to write empty mermaid graph after QueryPlan error: %w (original error: %v)", writeErr, err)
			}
			return nil
		}
		return fmt.Errorf("failed to create QueryPlan: %w", err)
	}

	// At this point, qp is a valid QueryPlan object returned by spannerplan.New (if err was nil).
	// Now, try to get the root node. If GetNodeByIndex(0) returns nil,
	// it implies the plan is empty or has no node at index 0 (which is assumed to be the root).
	physicalRootNode := qp.GetNodeByIndex(0)
	if physicalRootNode == nil {
		if param.TypeFlag == "mermaid" {
			if _, err := writer.Write([]byte("graph TD\n")); err != nil {
				return fmt.Errorf("failed to write empty mermaid graph (root node is nil): %w", err)
			}
			return nil
		}
		// For non-Mermaid, if physicalRootNode is nil, it means no drawable plan.
		return fmt.Errorf("cannot render image: query plan has no actionable root node (e.g., empty or rootless)")
	}

	// Proceed with physicalRootNode, which is non-nil here.
	rootNode, err := buildTree(qp, physicalRootNode, rowType, param)
	if err != nil {
		return fmt.Errorf("failed to build tree: %w", err)
	}

	// 2. Fork based on TypeFlag
	if param.TypeFlag == "mermaid" {
		return renderMermaid(rootNode, writer, qp, param, rowType)
	}

	// 3. Graphviz path
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

// renderGraph now accepts QueryPlan (qp)
func renderGraph(graph *cgraph.Graph, rootNode *treeNode, qp *spannerplan.QueryPlan, param option.Options, queryStats *sppb.ResultSetStats, rowType *sppb.StructType) error {
	graph.SetRankDir(cgraph.BTRank)
	err := renderTree(graph, rootNode, qp, param, rowType) // Pass qp
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

// renderTree now accepts QueryPlan (qp)
func renderTree(graph *cgraph.Graph, node *treeNode, qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) error {
	err := renderNode(graph, node, qp, param, rowType) // Pass qp
	if err != nil {
		return err
	}

	for _, child := range node.Children {
		if err := renderTree(graph, child.ChildNode, qp, param, rowType); err != nil { // Pass qp
			return err
		}

		err = renderEdge(graph, node, child)
		if err != nil {
			return err
		}
	}
	return nil
}

// renderNode now accepts QueryPlan (qp)
func renderNode(graph *cgraph.Graph, node *treeNode, qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) error {
	n, err := graph.CreateNodeByName(node.GetName()) // Use GetName()
	if err != nil {
		return err
	}

	n.SetShape(cgraph.BoxShape)
	tooltipStr, ttErr := node.GetTooltip()
	if ttErr != nil {
		// Log error or use a default tooltip string if desired
		tooltipStr = "Error generating tooltip"
		log.Printf("Error getting tooltip for node %s: %v", node.GetName(), ttErr)
	}
	n.SetTooltip(tooltipStr)

	nodeHTMLStr := node.HTML(qp, param, rowType) // Pass qp
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
