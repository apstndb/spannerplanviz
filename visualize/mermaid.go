package visualize

import (
	"encoding/json"
	"fmt"
	"io"
	// "sort" // For stable output of map iteration - Removed as not used
	"strings"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb" // For sppb.StructType
	"github.com/apstndb/spannerplan"                   // For spannerplan.QueryPlan
	"github.com/goccy/go-graphviz/cgraph"              // For EdgeStyle constants
	"github.com/samber/lo"

	"github.com/apstndb/spannerplanviz/option" // For option.Options
)

// renderMermaid renders the query plan as a Mermaid diagram.
// Note: We don't use Mermaid.js Markdown label syntax with backticks because:
// 1. There's no way to escape backticks within the syntax
// 2. HTML-like formatting is not properly supported
// Instead, we use direct string formatting for labels.
func renderMermaid(rootNode *treeNode, writer io.Writer, qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) error {
	// `nil` is better because GitHub handles light/dark theme differently.
	// This behavior is not performed except `nil`, even if it is "default".
	var theme = ""
	config := map[string]any{
		"theme": lo.EmptyableToPtr(theme),
		"themeVariables": map[string]any{
			"wrap": false,
		},
		"flowchart": map[string]any{
			"curve":            "linear",
			"markdownAutoWrap": false,
			"wrappingWidth":    2000,
		},
	}

	b, err := json.Marshal(config)
	if err != nil {
		return err
	}

	var sb strings.Builder
	fmt.Fprintln(&sb, `%%{ init: `+string(b)+` }%%`)
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

			var edgeLabelPart string
			if edgeLink.ChildType != "" {
				edgeLabelPart = fmt.Sprintf("|%s|", edgeLink.ChildType)
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

	_, err = writer.Write([]byte(sb.String()))
	return err
}

// escapeMermaidLabelContent function removed from here, assumed to be in build_tree.go now.
