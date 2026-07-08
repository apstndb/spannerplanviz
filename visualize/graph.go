package visualize

import (
	"fmt"

	"github.com/apstndb/spannerplanviz/internal/graphmodel"
)

// GraphOptions controls optional content included when building the backend-neutral
// graph model.
type GraphOptions struct {
	// ShowQuery adds a query-text node built from the ResultSetStats query_stats.
	ShowQuery bool
	// ShowQueryStats adds the query statistics to the query-text node.
	ShowQueryStats bool
}

// BuildGraph resolves a built Plan into the backend-neutral graphmodel.Graph:
// node labels (via buildNodeLabel using plan.Build), tooltips (canonical YAML),
// edge styles, and the optional query-text node. Serializers consume the graph
// instead of walking the TreeNode graph directly.
//
// Node identity from the TreeNode graph is preserved: a TreeNode reached from
// multiple parents maps to a single *graphmodel.Node, so pointer sharing in the
// source survives into the model. Each serializer keeps its own traversal order
// (the dot path emits a full pre-order traversal; the mermaid path dedups by
// node ID), so the resolved model does not move any bytes.
//
// Labels are built with plan.Build. Callers that need to override build options
// at render time (for example mermaid.SourceWithOptions) do so by shallow-copying
// the Plan and setting Plan.Build before calling BuildGraph.
func BuildGraph(plan *Plan, opts GraphOptions) (*graphmodel.Graph, error) {
	if plan == nil || plan.Root == nil {
		return nil, fmt.Errorf("cannot build graph: plan is nil")
	}

	nodeMap := make(map[*TreeNode]*graphmodel.Node)

	var buildNode func(tn *TreeNode) (*graphmodel.Node, error)
	buildNode = func(tn *TreeNode) (*graphmodel.Node, error) {
		if existing, ok := nodeMap[tn]; ok {
			return existing, nil
		}

		tooltip, err := tn.GetTooltip()
		if err != nil {
			return nil, fmt.Errorf("error getting tooltip for node %s: %w", tn.GetName(), err)
		}

		gnode := &graphmodel.Node{
			ID:      tn.GetName(),
			Label:   tn.buildNodeLabel(plan.Build, plan.RowType),
			Tooltip: tooltip,
		}
		// Register before recursion so shared children (and any future cycles)
		// resolve to the same node.
		nodeMap[tn] = gnode

		for _, link := range tn.Children {
			child, err := buildNode(link.ChildNode)
			if err != nil {
				return nil, err
			}
			gnode.Children = append(gnode.Children, graphmodel.Edge{
				Style: toGraphEdgeStyle(link.Style),
				Label: link.ChildType,
				To:    child,
			})
		}
		return gnode, nil
	}

	root, err := buildNode(plan.Root)
	if err != nil {
		return nil, err
	}

	graph := &graphmodel.Graph{Root: root}

	if (opts.ShowQuery || opts.ShowQueryStats) && plan.QueryStats != nil {
		graph.Query = buildQueryText(plan.QueryStats.GetQueryStats().GetFields(), opts.ShowQueryStats)
	}

	return graph, nil
}

// toGraphEdgeStyle maps a visualize.EdgeStyle onto the backend-neutral
// graphmodel.EdgeStyle.
func toGraphEdgeStyle(style EdgeStyle) graphmodel.EdgeStyle {
	switch style {
	case EdgeStyleDashed:
		return graphmodel.EdgeStyleDashed
	case EdgeStyleDotted:
		return graphmodel.EdgeStyleDotted
	default:
		return graphmodel.EdgeStyleSolid
	}
}
