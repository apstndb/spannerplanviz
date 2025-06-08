package visualize

import (
	"fmt"
	"io"
	"sort" // For stable output of map iteration
	"strings"

	"github.com/goccy/go-graphviz/cgraph" // For EdgeStyle constants
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb" // For sppb.StructType
	"github.com/apstndb/spannerplan"                 // For spannerplan.QueryPlan
	"github.com/apstndb/spannerplanviz/option"       // For option.Options
)

func renderMermaid(rootNode *treeNode, writer io.Writer, qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) error {
	var sb strings.Builder
	sb.WriteString("graph TD\n") // Top-Down direction

	renderedNodes := make(map[string]bool) // To track rendered nodes and avoid duplicates
	var edgesToRender []string

	styleTranslation := map[cgraph.EdgeStyle]string{
		cgraph.SolidEdgeStyle:  "-->",
		cgraph.DashedEdgeStyle: "-.->",
		cgraph.DottedEdgeStyle: "-.->", // Mermaid doesn't have a distinct dotted style, use dashed
	}

	var buildMermaidRecursive func(*treeNode)
	buildMermaidRecursive = func(node *treeNode) {
		if node == nil {
			return
		}
		nodeName := node.GetName() // Use new getter
		if _, visited := renderedNodes[nodeName]; visited {
			return
		}
		renderedNodes[nodeName] = true

		// Use the new MermaidLabel method
		finalLabel := node.MermaidLabel(qp, param, rowType) // Pass qp, param, rowType

		sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", nodeName, finalLabel))

		// Edges
		for _, edgeLink := range node.Children {
			arrow, ok := styleTranslation[edgeLink.Style]
			if !ok {
				arrow = "-->" // Default to solid
			}

			// escapeMermaidLabelContent is now part of MermaidLabel or its helpers.
			// ChildType itself might need escaping if it can contain " or backtick,
			// but typically it's a simple string like "Input" or "SCALAR".
			// For now, assume ChildType is safe or pre-escaped if needed.
			// If ChildType needs robust escaping, a separate simple escaper for Mermaid edge labels might be needed.
			// Let's use a basic escape for ChildType here for quotes, as it's for the edge label.
			edgeLabel := strings.ReplaceAll(edgeLink.ChildType, "\"", "#quot;")

			edgeStr := fmt.Sprintf("    %s %s|%s| %s\n", nodeName, arrow, edgeLabel, edgeLink.ChildNode.GetName())
			edgesToRender = append(edgesToRender, edgeStr)

			buildMermaidRecursive(edgeLink.ChildNode)
		}
	}

	buildMermaidRecursive(rootNode)

	// Append all edges after all nodes are defined
	for _, edgeStr := range edgesToRender {
		sb.WriteString(edgeStr)
	}

	_, err := writer.Write([]byte(sb.String()))
	return err
}

// escapeMermaidLabelContent function removed from here, assumed to be in build_tree.go now.
