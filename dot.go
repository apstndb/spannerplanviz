package spannerplanviz

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/apstndb/spannerplanviz/protoyaml"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

type VisualizeParam struct {
	ShowQuery        bool
	ShowQueryStats   bool
	NonVariableScala bool
	VariableScalar   bool
	Metadata         bool
	ExecutionStats   bool
	ExecutionSummary bool
	SerializeResult  bool
	HideScanTarget   bool
	HideMetadata     []string
}

func RenderImage(rowType *spanner.StructType, queryStats *spanner.ResultSetStats, format graphviz.Format, writer io.Writer, param VisualizeParam) error {
	g := graphviz.New()
	graph, err := g.Graph()
	if err != nil {
		return err
	}
	defer func() {
		if err := graph.Close(); err != nil {
			log.Fatal(err)
		}
		g.Close()
	}()

	err = buildGraphFromQueryPlan(graph, rowType, queryStats, param)
	if err != nil {
		return err
	}

	return g.Render(graph, format, writer)
}

func buildGraphFromQueryPlan(graph *cgraph.Graph, rowType *spanner.StructType, queryStats *spanner.ResultSetStats, param VisualizeParam) error {
	graph.SetRankDir(cgraph.BTRank)

	queryPlan := queryStats.GetQueryPlan()
	planNodes := queryPlan.GetPlanNodes()
	gvNodes := make([]*cgraph.Node, len(planNodes))

	for _, planNode := range planNodes {
		if isInlined(planNode) {
			continue
		}

		var labelStr string
		{
			var labelBuf bytes.Buffer
			metadataFields := planNode.GetMetadata().GetFields()

			childLinks := getNonVariableChildLinks(queryPlan, planNode)
			if param.SerializeResult && planNode.DisplayName == "Serialize Result" && rowType != nil {
				labelBuf.WriteString(renderSerializeResult(rowType, childLinks))
			}

			if !param.HideScanTarget && planNode.GetDisplayName() == "Scan" {
				scanType := strings.TrimSuffix(metadataFields["scan_type"].GetStringValue(), "Scan")
				scanTarget := metadataFields["scan_target"].GetStringValue()
				s := fmt.Sprintf("%s: %s\n", scanType, scanTarget)
				labelBuf.WriteString(s)
			}

			if param.NonVariableScala {
				labelBuf.WriteString(renderChildLinks(childLinks))
			}

			if param.Metadata {
				labelBuf.WriteString(renderMetadata(metadataFields, param))
			}

			if param.VariableScalar {
				childLinks := getVariableChildLinks(queryPlan, planNode)
				labelBuf.WriteString(renderChildLinks(childLinks))
			}
			labelStr = labelBuf.String()
		}

		statsStr := renderExecutionStatsOfNode(planNode, param)

		metadataStr := toLeftAlignedText(labelStr) + encloseIfNotEmpty(`<i>`, toLeftAlignedText(statsStr), `</i>`)

		n, err := setupGvNode(graph, planNode, getNodeTitle(planNode), metadataStr)
		if err != nil {
			return err
		}

		gvNodes[planNode.GetIndex()] = n
	}

	for _, node := range queryPlan.PlanNodes {
		if isInlined(node) {
			continue
		}
		for i, child := range node.ChildLinks {
			childPlanNode := queryPlan.PlanNodes[child.ChildIndex]
			if isInlined(childPlanNode) {
				continue
			}

			gvNode := gvNodes[node.Index]
			gvChildNode := gvNodes[child.ChildIndex]
			if gvNode == nil || gvChildNode == nil {
				return fmt.Errorf("invalid condition, some node is nil: node %v, childNode %v, gvNode %v, gvChildNode %v\n", node, childPlanNode, gvNode, gvChildNode)
			}
			ed, _ := graph.CreateEdge("", gvChildNode, gvNode)

			var childType string
			if child.GetType() == "" && strings.HasSuffix(node.GetDisplayName(), "Apply") && i == 0 {
				childType = "Input"
			} else {
				childType = child.GetType()
			}

			ed.SetLabel(childType)
		}
	}

	if queryStats != nil && (param.ShowQuery || param.ShowQueryStats) {
		n, err := setupQueryNode(graph, queryStats, param)
		if err != nil {
			return err
		}
		_, err = graph.CreateEdge("", gvNodes[0], n)
		if err != nil {
			return err
		}
	}
	return nil
}

func renderExecutionStatsOfNode(planNode *spanner.PlanNode, param VisualizeParam) string {
	var statsBuf bytes.Buffer

	executionStatsFields := planNode.GetExecutionStats().GetFields()
	if param.ExecutionStats {
		statsBuf.WriteString(renderExecutionStatsWithoutSummary(executionStatsFields))
	}

	if param.ExecutionSummary {
		statsBuf.WriteString(renderExecutionSummary(executionStatsFields))
	}
	return statsBuf.String()
}

func setupQueryNode(graph *cgraph.Graph, queryStats *spanner.ResultSetStats, param VisualizeParam) (*cgraph.Node, error){
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "<b>%s</b>", toLeftAlignedText(queryStats.GetQueryStats().GetFields()["query_text"].GetStringValue()))

	var statsBuf bytes.Buffer
	if param.ShowQueryStats {
		for k, v := range queryStats.GetQueryStats().GetFields() {
			if k == "query_text" {
				continue
			}
			fmt.Fprintf(&statsBuf, fmt.Sprintf("%s: %s\n", k, v.GetStringValue()))
		}
	}
	fmt.Fprintf(&buf, encloseIfNotEmpty("<i>", toLeftAlignedText(statsBuf.String()), "</i>"))

	n, err := graph.CreateNode("query")
	if err != nil {
		return nil, err
	}
	n.SetLabel(graph.StrdupHTML(buf.String()))
	n.SetShape(cgraph.BoxShape)
	n.SetStyle(cgraph.RoundedNodeStyle)

	return n, nil
}

func renderExecutionStatsWithoutSummary(executionStatsFields map[string]*structpb.Value) string {
	var buf bytes.Buffer
	for k, v := range executionStatsFields {
		if k == "execution_summary" {
			continue
		}

		fmt.Fprintf(&buf, "%s: %s\n", k, formatExecutionStatsValue(v))
	}
	s := buf.String()
	return s
}

func renderMetadata(metadataFields map[string]*structpb.Value, param VisualizeParam) string {
	var metadataBuf bytes.Buffer
	for k, v := range metadataFields {
		switch {
		case in(k, param.HideMetadata...):
			continue
		case in(k, "call_type", "scan_type", "scan_target", "iterator_type"):
			continue
		default:
			fmt.Fprintf(&metadataBuf, "%s=%s\n", k, toString(v))
		}
	}
	s := metadataBuf.String()
	return s
}

func renderExecutionSummary(executionStatsFields map[string]*structpb.Value) string {
	var executionSummaryBuf bytes.Buffer
	if executionSummary, ok := executionStatsFields["execution_summary"]; ok {
		fmt.Fprintln(&executionSummaryBuf, "execution_summary:")
		for k, v := range executionSummary.GetStructValue().GetFields() {
			var value string
			if strings.HasSuffix(k, "timestamp") {
				value = tryToTimestampStr(v)
			} else {
				value = toString(v)
			}
			fmt.Fprintf(&executionSummaryBuf, "   %s: %s\n", k, value)
		}
	}
	s := executionSummaryBuf.String()
	return s
}

var newlineOrEOSRe = regexp.MustCompile(`(?:\n?$|\n)`)

func toLeftAlignedText(str string) string {
	if str == "" {
		return ""
	}
	return newlineOrEOSRe.ReplaceAllString(html.EscapeString(str), `<br align="left" />`)
}

const RFC3339Micro = "2006-01-02T15:04:05.999999Z07:00"

func tryToTimestampStr(v *structpb.Value) string {
	ss := strings.Split(v.GetStringValue(), ".")
	if len(ss) != 2 || len(ss[1]) > 6 {
		return v.GetStringValue()
	}
	sec, err := strconv.Atoi(ss[0])
	if err != nil {
		return v.GetStringValue()
	}
	usec, err := strconv.Atoi(ss[1])
	if err != nil {
		return v.GetStringValue()
	}
	return time.Unix(int64(sec), int64(usec)*1000).UTC().Format(RFC3339Micro)
}

func setupGvNode(graph *cgraph.Graph, planNode *spanner.PlanNode, nodeTitle string, metadataStr string) (*cgraph.Node, error) {
	n, err := graph.CreateNode(fmt.Sprintf("node%d", planNode.GetIndex()))
	if err != nil {
		return nil, err
	}

	n.SetShape(cgraph.BoxShape)

	b, err := protoyaml.Marshal(planNode)
	if err != nil {
		return nil, err
	}
	n.SetTooltip(string(b))

	n.SetLabel(graph.StrdupHTML(fmt.Sprintf(`<b>%s</b><br align="CENTER" />%s`, nodeTitle, metadataStr)))
	return n, nil
}

func formatExecutionStatsValue(v *structpb.Value) string {
	fields := v.GetStructValue().GetFields()
	total := fields["total"].GetStringValue()
	unit := fields["unit"].GetStringValue()
	mean := fields["mean"].GetStringValue()
	stdDev := fields["std_deviation"].GetStringValue()

	var stdDevStr string
	if stdDev != "" {
		stdDevStr = fmt.Sprintf("±%s", stdDev)
	}
	var meanStr string
	if mean != "" {
		meanStr = fmt.Sprintf("@%s%s", mean, stdDevStr)
	}
	value := fmt.Sprintf("%s%s %s", total, meanStr, unit)
	return value
}

func getNodeTitle(planNode *spanner.PlanNode) string {
	fields := planNode.GetMetadata().GetFields()
	return strings.Join(skipEmpty(
		fields["call_type"].GetStringValue(),
		fields["iterator_type"].GetStringValue(),
		strings.TrimSuffix(fields["scan_type"].GetStringValue(), "Scan"),
		planNode.GetDisplayName(),
	), " ")
}

var inlinedOperators = []string{"Function", "Reference", "Constant", "Array Constructor", "Parameter"}

func isInlined(node *spanner.PlanNode) bool {
	return in(node.GetDisplayName(), inlinedOperators...)
}

func renderSerializeResult(rowType *spanner.StructType, childLinks []*ChildLinkGroup) string {
	var result bytes.Buffer
	for _, cl := range childLinks {
		if cl.Type != "" {
			continue
		}
		for i, planNode := range cl.PlanNodes {
			name := rowType.GetFields()[i].GetName()
			if name == "" {
				name = fmt.Sprintf("no_name<%d>", i)
			}
			text := fmt.Sprintf("Result.%s:%s", name, planNode.PlanNodes.GetShortRepresentation().GetDescription())
			fmt.Fprintln(&result, text)
		}
	}
	return result.String()
}

func renderChildLinks(childLinks []*ChildLinkGroup) string {
	var buf bytes.Buffer
	for _, cl := range childLinks {
		var prefix string
		if cl.Type != "" && cl.Type != "Value" {
			if len(cl.PlanNodes) == 1 {
				prefix = fmt.Sprintf("%s: ", cl.Type)
			} else {
				prefix = "  "
				fmt.Fprintf(&buf, "%s:\n", cl.Type)
			}
		}
		for _, planNode := range cl.PlanNodes {
			if planNode.Variable == "" && cl.Type == "" {
				continue
			}
			description := planNode.PlanNodes.GetShortRepresentation().GetDescription()
			if planNode.Variable == "" {
				fmt.Fprintf(&buf, "%s%s\n", prefix, description)
			} else {
				fmt.Fprintf(&buf, "%s$%s:=%s\n", prefix, planNode.Variable, description)
			}
		}
	}
	return buf.String()
}

func toString(v *structpb.Value) string {
	switch x := v.GetKind().(type) {
	case *structpb.Value_StringValue:
		return x.StringValue
	case *structpb.Value_NumberValue:
		return fmt.Sprint(x.NumberValue)
	case *structpb.Value_BoolValue:
		return strconv.FormatBool(x.BoolValue)
	default:
		return v.String()
	}
}

type ChildLinkEntry struct {
	Variable  string
	PlanNodes *spanner.PlanNode
}

type ChildLinkGroup struct {
	Type      string
	PlanNodes []*ChildLinkEntry
}

func getScalarChildLinks(plan *spanner.QueryPlan, node *spanner.PlanNode, filter func(link *spanner.PlanNode_ChildLink) bool) []*ChildLinkGroup {
	var result []*ChildLinkGroup
	typeToChildLinks := make(map[string]*ChildLinkGroup)
	for _, cl := range node.GetChildLinks() {
		childIndex := cl.GetChildIndex()
		childNode := plan.PlanNodes[childIndex]
		childType := cl.GetType()

		if !filter(cl) || childNode.GetKind() != spanner.PlanNode_SCALAR {
			continue
		}
		if childLinks, ok := typeToChildLinks[childType]; ok {
			childLinks.PlanNodes = append(childLinks.PlanNodes, &ChildLinkEntry{cl.GetVariable(), plan.PlanNodes[childIndex]})
		} else {
			current := &ChildLinkGroup{Type: childType, PlanNodes: []*ChildLinkEntry{{Variable: cl.GetVariable(), PlanNodes: plan.PlanNodes[childIndex]}}}
			typeToChildLinks[childType] = current
			result = append(result, current)
		}
	}
	return result
}

func getNonVariableChildLinks(plan *spanner.QueryPlan, node *spanner.PlanNode) []*ChildLinkGroup {
	return getScalarChildLinks(plan, node, func(node *spanner.PlanNode_ChildLink) bool {
		return node.GetVariable() == ""
	})
}

func getVariableChildLinks(plan *spanner.QueryPlan, node *spanner.PlanNode) []*ChildLinkGroup {
	return getScalarChildLinks(plan, node, func(node *spanner.PlanNode_ChildLink) bool {
		return node.GetVariable() != ""
	})
}
