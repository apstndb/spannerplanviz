package visualize

import (
	"context"
	"io"
	"log"

	"github.com/apstndb/spannerplan"

	"github.com/apstndb/spannerplanviz/option"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
)

func RenderImage(ctx context.Context, rowType *sppb.StructType, queryStats *sppb.ResultSetStats, format graphviz.Format, writer io.Writer, param option.Options) error {
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

	err = buildAndRenderGraph(graph, rowType, queryStats, param)
	if err != nil {
		return err
	}

	return g.Render(ctx, graph, format, writer)
}

func buildAndRenderGraph(graph *cgraph.Graph, rowType *sppb.StructType, queryStats *sppb.ResultSetStats, param option.Options) error {
	qp, err := spannerplan.New(queryStats.GetQueryPlan().GetPlanNodes())
	if err != nil {
		return err
	}

	rootNode, err := buildTree(qp, qp.GetNodeByIndex(0), rowType, param)
	if err != nil {
		return err
	}

	return renderGraph(graph, err, rootNode, param, queryStats)
}

func renderGraph(graph *cgraph.Graph, err error, rootNode *treeNode, param option.Options, queryStats *sppb.ResultSetStats) error {
	graph.SetRankDir(cgraph.BTRank)
	err = renderTree(graph, rootNode)
	if err != nil {
		return err
	}

	showQueryStats := param.ShowQueryStats
	needQueryNode := (param.ShowQuery || showQueryStats) && queryStats != nil
	if needQueryNode {
		err := renderQueryNodeWithEdge(graph, queryStats, showQueryStats, rootNode.Name)
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

func renderTree(graph *cgraph.Graph, node *treeNode) error {
	err := renderNode(graph, node)
	if err != nil {
		return err
	}

	for _, child := range node.Children {
		if err := renderTree(graph, child.ChildNode); err != nil {
			return err
		}

		err := renderEdge(graph, node, child)
		if err != nil {
			return err
		}
	}
	return nil
}

func renderNode(graph *cgraph.Graph, node *treeNode) error {
	n, err := graph.CreateNodeByName(node.Name)
	if err != nil {
		return err
	}

	n.SetShape(cgraph.BoxShape)
	n.SetTooltip(node.Tooltip)

	nodeHTML, err := graph.StrdupHTML(node.HTML())
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
