package visualize

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apstndb/spannerplanviz/queryplan"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/protobuf/types/known/structpb"
	"sigs.k8s.io/yaml"
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

	qp := queryplan.New(queryStats.GetQueryPlan().GetPlanNodes())

	gvRootNode, err := renderTree(graph, rowType, nil, qp, param)
	if err != nil {
		return err
	}

	if queryStats != nil && (param.ShowQuery || param.ShowQueryStats) {
		n, err := setupQueryNode(graph, queryStats, param)
		if err != nil {
			return err
		}
		_, err = graph.CreateEdge("", gvRootNode, n)
		if err != nil {
			return err
		}
	}
	return nil
}

func renderTree(graph *cgraph.Graph, rowType *spanner.StructType, childLink *spanner.PlanNode_ChildLink, qp *queryplan.QueryPlan, param VisualizeParam) (*cgraph.Node, error) {
	node := qp.GetNodeByChildLink(childLink)
	gvNode, err := renderNode(graph, rowType, childLink, qp, param)
	if err != nil {
		return gvNode, err
	}
	for i, cl := range qp.VisibleChildLinks(node) {
		if gvChildNode, err := renderTree(graph, rowType, cl, qp, param); err != nil {
			return gvNode, err
		} else {
			ed, err := graph.CreateEdge("", gvChildNode, gvNode)
			if err != nil {
				return gvNode, err
			}

			var childType string
			if cl.GetType() == "" && strings.HasSuffix(node.GetDisplayName(), "Apply") && i == 0 {
				childType = "Input"
			} else {
				childType = cl.GetType()
			}

			ed.SetLabel(childType)
		}
	}
	return gvNode, nil
}

func renderNode(graph *cgraph.Graph, rowType *spanner.StructType, childLink *spanner.PlanNode_ChildLink, queryPlan *queryplan.QueryPlan, param VisualizeParam) (*cgraph.Node, error) {
	planNode := queryPlan.GetNodeByChildLink(childLink)
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
		return nil, err
	}
	return n, nil
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

func setupQueryNode(graph *cgraph.Graph, queryStats *spanner.ResultSetStats, param VisualizeParam) (*cgraph.Node, error) {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "<b>%s</b>", toLeftAlignedText(queryStats.GetQueryStats().GetFields()["query_text"].GetStringValue()))

	var stats []string
	if param.ShowQueryStats {
		for k, v := range queryStats.GetQueryStats().GetFields() {
			if k == "query_text" {
				continue
			}
			stats = append(stats, fmt.Sprintf("%s: %s", k, v.GetStringValue()))
		}
	}

	sort.Strings(stats)
	fmt.Fprintf(&buf, encloseIfNotEmpty("<i>", toLeftAlignedText(strings.Join(stats, "\n")), "</i>"))

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
	var statsStrings []string
	for k, v := range executionStatsFields {
		if k == "execution_summary" {
			continue
		}

		statsStrings = append(statsStrings, fmt.Sprintf("%s: %s\n", k, formatExecutionStatsValue(v)))
	}
	sort.Strings(statsStrings)
	return strings.Join(statsStrings, "")
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
			fmt.Fprintf(&metadataBuf, "%s=%v\n", k, v.AsInterface())
		}
	}
	s := metadataBuf.String()
	return s
}

func renderExecutionSummary(executionStatsFields map[string]*structpb.Value) string {
	var executionSummaryBuf bytes.Buffer
	if executionSummary, ok := executionStatsFields["execution_summary"]; ok {
		fmt.Fprintln(&executionSummaryBuf, "execution_summary:")
		var executionSummaryStrings []string
		for k, v := range executionSummary.GetStructValue().AsMap() {
			var value string
			if strings.HasSuffix(k, "timestamp") {
				value = tryToTimestampStr(fmt.Sprint(v))
			} else {
				value = fmt.Sprint(v)
			}
			executionSummaryStrings = append(executionSummaryStrings, fmt.Sprintf("   %s: %s\n", k, value))
		}
		sort.Strings(executionSummaryStrings)
		fmt.Fprint(&executionSummaryBuf, strings.Join(executionSummaryStrings, ""))
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

func tryToTimestampStr(s string) string {
	ss := strings.Split(s, ".")
	if len(ss) != 2 || len(ss[1]) > 6 {
		return s
	}
	sec, err := strconv.Atoi(ss[0])
	if err != nil {
		return s
	}
	usec, err := strconv.Atoi(ss[1])
	if err != nil {
		return s
	}
	return time.Unix(int64(sec), int64(usec)*1000).UTC().Format(RFC3339Micro)
}

func setupGvNode(graph *cgraph.Graph, planNode *spanner.PlanNode, nodeTitle string, metadataStr string) (*cgraph.Node, error) {
	n, err := graph.CreateNode(fmt.Sprintf("node%d", planNode.GetIndex()))
	if err != nil {
		return nil, err
	}

	n.SetShape(cgraph.BoxShape)

	b, err := yaml.Marshal(planNode)
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

func isInlined(nodes []*spanner.PlanNode, node *spanner.PlanNode) bool {
	return node.GetKind() == spanner.PlanNode_SCALAR && (len(node.GetChildLinks()) == 0 || nodes[node.GetChildLinks()[0].GetChildIndex()].GetKind() != spanner.PlanNode_RELATIONAL)
}

func renderSerializeResult(rowType *spanner.StructType, childLinks []*childLinkGroup) string {
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

func renderChildLinks(childLinks []*childLinkGroup) string {
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

type childLinkEntry struct {
	Variable  string
	PlanNodes *spanner.PlanNode
}

type childLinkGroup struct {
	Type      string
	PlanNodes []*childLinkEntry
}

func getScalarChildLinks(qp *queryplan.QueryPlan, node *spanner.PlanNode, filter func(link *spanner.PlanNode_ChildLink) bool) []*childLinkGroup {
	var result []*childLinkGroup
	typeToChildLinks := make(map[string]*childLinkGroup)
	for _, cl := range node.GetChildLinks() {
		childNode := qp.GetNodeByChildLink(cl)
		childType := cl.GetType()

		if !filter(cl) || childNode.GetKind() != spanner.PlanNode_SCALAR {
			continue
		}
		if _, ok := typeToChildLinks[childType]; !ok {
			newEntry := &childLinkGroup{Type: childType}
			typeToChildLinks[childType] = newEntry
			result = append(result, newEntry)
		}
		childLinks := typeToChildLinks[childType]
		childLinks.PlanNodes = append(childLinks.PlanNodes, &childLinkEntry{cl.GetVariable(), childNode})
	}
	return result
}

func getNonVariableChildLinks(plan *queryplan.QueryPlan, node *spanner.PlanNode) []*childLinkGroup {
	return getScalarChildLinks(plan, node, func(node *spanner.PlanNode_ChildLink) bool {
		return node.GetVariable() == ""
	})
}

func getVariableChildLinks(plan *queryplan.QueryPlan, node *spanner.PlanNode) []*childLinkGroup {
	return getScalarChildLinks(plan, node, func(node *spanner.PlanNode_ChildLink) bool {
		return node.GetVariable() != ""
	})
}
