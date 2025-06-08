package visualize

import (
	"fmt"
	"io"
	// "sort" // For stable output of map iteration - Removed as not used
	"strings"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb" // For sppb.StructType
	"github.com/apstndb/spannerplan"                   // For spannerplan.QueryPlan
	"github.com/goccy/go-graphviz/cgraph"              // For EdgeStyle constants

	"github.com/apstndb/spannerplanviz/option" // For option.Options
)

func renderMermaid(rootNode *treeNode, writer io.Writer, qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) error {
	var sb strings.Builder
	sb.WriteString(`%%{ init: {"theme": "default",
           "themeVariables": { "wrap": "false" },
           "flowchart": { "curve": "linear",
                          "markdownAutoWrap":"false",
                          "wrappingWidth": "600" }
           }
}%%
`)
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
		sb.WriteString(fmt.Sprintf("    style %s text-align:left;\n", nodeName))

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

			var edgeLabelPart string
			if edgeLabel != "" {
				edgeLabelPart = fmt.Sprintf("|%s|", edgeLabel)
			}
			edgeStr := fmt.Sprintf("    %s %s%s %s\n", nodeName, arrow, edgeLabelPart, edgeLink.ChildNode.GetName())
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
