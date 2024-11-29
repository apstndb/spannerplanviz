package visualize

import (
	"bytes"
	"context"
	"fmt"
	"github.com/apstndb/spannerplanviz/option"
	"html"
	"io"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"

	"github.com/apstndb/spannerplanviz/queryplan"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"google.golang.org/protobuf/types/known/structpb"
	"sigs.k8s.io/yaml"
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
	graph.SetFontName("Times New Roman:style=Bold")

	err = buildGraphFromQueryPlan(graph, rowType, queryStats, param)
	if err != nil {
		return err
	}

	return g.Render(ctx, graph, format, writer)
}

func buildGraphFromQueryPlan(graph *cgraph.Graph, rowType *sppb.StructType, queryStats *sppb.ResultSetStats, param option.Options) error {
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
		_, err = graph.CreateEdgeByName("", gvRootNode, n)
		if err != nil {
			return err
		}
	}
	return nil
}

func renderTree(graph *cgraph.Graph, rowType *sppb.StructType, childLink *sppb.PlanNode_ChildLink, qp *queryplan.QueryPlan, param option.Options) (*cgraph.Node, error) {
	node := qp.GetNodeByChildLink(childLink)
	gvNode, err := renderNode(graph, rowType, childLink, qp, param)
	if err != nil {
		return gvNode, err
	}
	for i, cl := range qp.VisibleChildLinks(node) {
		if gvChildNode, err := renderTree(graph, rowType, cl, qp, param); err != nil {
			return gvNode, err
		} else {
			ed, err := graph.CreateEdgeByName("", gvChildNode, gvNode)
			if err != nil {
				return gvNode, err
			}

			if isRemoteCall(node, cl) {
				ed.SetStyle(cgraph.DashedEdgeStyle)
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

func isRemoteCall(node *sppb.PlanNode, cl *sppb.PlanNode_ChildLink) bool {
	n, ok := node.GetMetadata().GetFields()["subquery_cluster_node"]
	if !ok {
		return false
	}
	if node.GetMetadata().GetFields()["call_type"].GetStringValue() == "Local" {
		return false
	}
	return n.GetStringValue() == strconv.Itoa(int(cl.GetChildIndex()))
}

func renderNode(graph *cgraph.Graph, rowType *sppb.StructType, childLink *sppb.PlanNode_ChildLink, queryPlan *queryplan.QueryPlan, param option.Options) (*cgraph.Node, error) {
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

		if param.NonVariableScalar {
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

func renderExecutionStatsOfNode(planNode *sppb.PlanNode, param option.Options) string {
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

func setupQueryNode(graph *cgraph.Graph, queryStats *sppb.ResultSetStats, param option.Options) (*cgraph.Node, error) {
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
	fmt.Fprint(&buf, encloseIfNotEmpty("<i>", toLeftAlignedText(strings.Join(stats, "\n")), "</i>"))

	n, err := graph.CreateNodeByName("query")
	if err != nil {
		return nil, err
	}
	s, err := graph.StrdupHTML(buf.String())
	if err != nil {
		return nil, err
	}
	n.SetLabel(s)
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

func renderMetadata(metadataFields map[string]*structpb.Value, param option.Options) string {
	var metadataBuf bytes.Buffer
	for k, v := range metadataFields {
		switch {
		case in(k, param.HideMetadata...):
			continue
		case in(k, "call_type", "scan_type", "scan_target", "iterator_type", "subquery_cluster_node"):
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

func setupGvNode(graph *cgraph.Graph, planNode *sppb.PlanNode, nodeTitle string, metadataStr string) (*cgraph.Node, error) {
	n, err := graph.CreateNodeByName(fmt.Sprintf("node%d", planNode.GetIndex()))
	if err != nil {
		return nil, err
	}

	n.SetShape(cgraph.BoxShape)

	b, err := yaml.Marshal(planNode)
	if err != nil {
		return nil, err
	}
	n.SetTooltip(string(b))

	nodeHTML, err := graph.StrdupHTML(fmt.Sprintf(`<b>%s</b><br align="CENTER" />%s`, nodeTitle, metadataStr))
	if err != nil {
		return nil, err
	}

	n.SetLabel(nodeHTML)
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

func getNodeTitle(planNode *sppb.PlanNode) string {
	fields := planNode.GetMetadata().GetFields()
	return strings.Join(skipEmpty(
		fields["call_type"].GetStringValue(),
		fields["iterator_type"].GetStringValue(),
		strings.TrimSuffix(fields["scan_type"].GetStringValue(), "Scan"),
		planNode.GetDisplayName(),
	), " ")
}

func renderSerializeResult(rowType *sppb.StructType, childLinks []*childLinkGroup) string {
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
	PlanNodes *sppb.PlanNode
}

type childLinkGroup struct {
	Type      string
	PlanNodes []*childLinkEntry
}

func getScalarChildLinks(qp *queryplan.QueryPlan, node *sppb.PlanNode, filter func(link *sppb.PlanNode_ChildLink) bool) []*childLinkGroup {
	var result []*childLinkGroup
	typeToChildLinks := make(map[string]*childLinkGroup)
	for _, cl := range node.GetChildLinks() {
		childNode := qp.GetNodeByChildLink(cl)
		childType := cl.GetType()

		if !filter(cl) || childNode.GetKind() != sppb.PlanNode_SCALAR {
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

func getNonVariableChildLinks(plan *queryplan.QueryPlan, node *sppb.PlanNode) []*childLinkGroup {
	return getScalarChildLinks(plan, node, func(node *sppb.PlanNode_ChildLink) bool {
		return node.GetVariable() == ""
	})
}

func getVariableChildLinks(plan *queryplan.QueryPlan, node *sppb.PlanNode) []*childLinkGroup {
	return getScalarChildLinks(plan, node, func(node *sppb.PlanNode_ChildLink) bool {
		return node.GetVariable() != ""
	})
}
