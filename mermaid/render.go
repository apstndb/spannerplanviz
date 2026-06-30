package mermaid

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/apstndb/spannerplanviz/visualize"
)

// Renderer generates Mermaid.js source for a built plan.
type Renderer struct {
	Options Options
}

// NewRenderer returns a Mermaid renderer.
func NewRenderer(opts Options) *Renderer {
	return &Renderer{Options: opts}
}

// Source returns Mermaid.js source text using plan.Build settings.
func Source(plan *visualize.Plan) (string, error) {
	var buf strings.Builder
	if err := writeMermaid(&buf, plan, plan.Build); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// SourceWithOptions returns Mermaid.js source text using opts.BuildOptions instead of plan.Build.
func SourceWithOptions(plan *visualize.Plan, opts Options) (string, error) {
	var buf strings.Builder
	if err := writeMermaid(&buf, plan, opts.BuildOptions); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Render writes Mermaid.js source for plan to w.
func (r *Renderer) Render(ctx context.Context, w io.Writer, plan *visualize.Plan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return writeMermaid(w, plan, r.Options.BuildOptions)
}

func mermaidInitConfig() map[string]any {
	return map[string]any{
		"htmlLabels": true,
		"themeVariables": map[string]any{
			"wrap": false,
		},
		"flowchart": map[string]any{
			"curve":            "linear",
			"useMaxWidth":      false,
			"markdownAutoWrap": false,
			"wrappingWidth":    2000,
		},
	}
}

func writeMermaid(writer io.Writer, plan *visualize.Plan, build visualize.BuildOptions) error {
	if plan == nil || plan.Root == nil {
		return fmt.Errorf("cannot render mermaid: plan is nil")
	}

	build.ApplyFull()

	b, err := json.Marshal(mermaidInitConfig())
	if err != nil {
		return err
	}

	var sb strings.Builder
	fmt.Fprintln(&sb, `%%{ init: `+string(b)+` }%%`)
	sb.WriteString("graph TD\n")

	renderedNodes := make(map[string]bool)
	var edgesToRender []string

	styleTranslation := map[visualize.EdgeStyle]string{
		visualize.EdgeStyleSolid:  "-->",
		visualize.EdgeStyleDashed: "-.->",
		visualize.EdgeStyleDotted: "-.->",
	}

	var walk func(*visualize.TreeNode)
	walk = func(node *visualize.TreeNode) {
		if node == nil {
			return
		}
		nodeName := node.GetName()
		if _, visited := renderedNodes[nodeName]; visited {
			return
		}
		renderedNodes[nodeName] = true

		finalLabel := node.MermaidLabel(build, plan.RowType)

		fmt.Fprintf(&sb, "    %s[\"%s\"]\n", nodeName, finalLabel)
		fmt.Fprintf(&sb, "    style %s text-align:left;\n", nodeName)

		for _, edgeLink := range node.Children {
			arrow, ok := styleTranslation[edgeLink.Style]
			if !ok {
				arrow = "-->"
			}

			var edgeLabelPart string
			if edgeLink.ChildType != "" {
				edgeLabelPart = fmt.Sprintf("|%s|", escapeMermaidEdgeLabel(edgeLink.ChildType))
			}
			edgeStr := fmt.Sprintf("    %s %s%s %s\n", nodeName, arrow, edgeLabelPart, edgeLink.ChildNode.GetName())
			edgesToRender = append(edgesToRender, edgeStr)

			walk(edgeLink.ChildNode)
		}
	}

	walk(plan.Root)

	for _, edgeStr := range edgesToRender {
		sb.WriteString(edgeStr)
	}

	_, err = writer.Write([]byte(sb.String()))
	return err
}

var mermaidEdgeLabelReplacer = strings.NewReplacer(
	"\n", " ",
	"\r", " ",
	"|", "#124;",
	"#", "#35;",
	">", "#62;",
	"<", "#60;",
)

// escapeMermaidEdgeLabel prepares text for Mermaid flowchart edge labels (-->|label|).
func escapeMermaidEdgeLabel(label string) string {
	return mermaidEdgeLabelReplacer.Replace(label)
}
