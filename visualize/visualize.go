package visualize

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"io"
	"log"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apstndb/spannerplanviz/option"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"

	"github.com/apstndb/spannerplan"
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
	graph.SetStart(graphviz.SelfStart)

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
	qp := spannerplan.New(queryStats.GetQueryPlan().GetPlanNodes())

	graph.SetRankDir(cgraph.BTRank)
	gvRootNode, err := renderTree(graph, rowType, qp.GetNodeByIndex(0), qp, param)
	if err != nil {
		return err
	}

	renderQueryNode := (param.ShowQuery || param.ShowQueryStats) && queryStats != nil
	if renderQueryNode {
		n, err := setupQueryNode(graph, queryStats, param.ShowQueryStats)
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

func renderTree(graph *cgraph.Graph, rowType *sppb.StructType, node *sppb.PlanNode, qp *spannerplan.QueryPlan, param option.Options) (*cgraph.Node, error) {
	gvNode, err := renderNode(graph, rowType, node, qp, param)
	if err != nil {
		return nil, err
	}

	for i, cl := range qp.VisibleChildLinks(node) {
		if gvChildNode, err := renderTree(graph, rowType, qp.GetNodeByChildLink(cl), qp, param); err != nil {
			return nil, err
		} else {
			var childType string
			if cl.GetType() == "" && strings.HasSuffix(node.GetDisplayName(), "Apply") && i == 0 {
				childType = "Input"
			} else {
				childType = cl.GetType()
			}

			var style cgraph.EdgeStyle
			if isRemoteCall(node, cl) {
				style = cgraph.DashedEdgeStyle
			}

			ed, err := graph.CreateEdgeByName("", gvChildNode, gvNode)
			if err != nil {
				return nil, err
			}

			ed.SetStyle(style)
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

type prerenderedNode struct {
	Name, Label, Stats, Title, Tooltip string
}

func (n *prerenderedNode) Metadata() string {
	return toLeftAlignedText(n.Label) + encloseIfNotEmpty(`<i>`, toLeftAlignedText(n.Stats), `</i>`)
}

func (n *prerenderedNode) HTML() string {
	return fmt.Sprintf(`<b>%s</b><br align="CENTER" />%s`, n.Title, n.Metadata())
}

func renderNode(graph *cgraph.Graph, rowType *sppb.StructType, planNode *sppb.PlanNode, queryPlan *spannerplan.QueryPlan, param option.Options) (*cgraph.Node, error) {
	node, err := prerenderNode(queryPlan, planNode, param, rowType)
	if err != nil {
		return nil, err
	}

	n, err := setupGvNode(graph, node)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func prerenderNode(queryPlan *spannerplan.QueryPlan, planNode *sppb.PlanNode, param option.Options, rowType *sppb.StructType) (*prerenderedNode, error) {
	labelStr := renderLabel(planNode, queryPlan, param, rowType)
	statsStr := renderExecutionStats(planNode.GetExecutionStats(), param)
	titleStr := getNodeTitle(planNode)

	b, err := yaml.Marshal(planNode)
	if err != nil {
		return nil, err
	}

	node := &prerenderedNode{
		Label:   labelStr,
		Stats:   statsStr,
		Title:   titleStr,
		Name:    fmt.Sprintf("node%d", planNode.GetIndex()),
		Tooltip: string(b),
	}
	return node, err
}

func setupGvNode(graph *cgraph.Graph, node *prerenderedNode) (*cgraph.Node, error) {
	n, err := graph.CreateNodeByName(node.Name)
	if err != nil {
		return nil, err
	}

	n.SetShape(cgraph.BoxShape)
	n.SetTooltip(node.Tooltip)

	nodeHTML, err := graph.StrdupHTML(node.HTML())
	if err != nil {
		return nil, err
	}

	n.SetLabel(nodeHTML)
	return n, nil
}

func renderLabel(planNode *sppb.PlanNode, queryPlan *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) string {
	var sb strings.Builder

	childLinks := getNonVariableChildLinks(queryPlan, planNode)
	if param.SerializeResult && planNode.DisplayName == "Serialize Result" && rowType != nil {
		sb.WriteString(renderSerializeResult(rowType, childLinks))
	}

	metadataFields := planNode.GetMetadata().GetFields()

	if !param.HideScanTarget && planNode.GetDisplayName() == "Scan" {
		scanType := strings.TrimSuffix(metadataFields["scan_type"].GetStringValue(), "Scan")
		scanTarget := metadataFields["scan_target"].GetStringValue()
		s := fmt.Sprintf("%s: %s\n", scanType, scanTarget)
		sb.WriteString(s)
	}

	if param.NonVariableScalar {
		sb.WriteString(renderChildLinks(childLinks))
	}

	if param.Metadata {
		sb.WriteString(renderMetadata(metadataFields, param.HideMetadata))
	}

	if param.VariableScalar {
		childLinks := getVariableChildLinks(queryPlan, planNode)
		sb.WriteString(renderChildLinks(childLinks))
	}
	return sb.String()
}

func renderExecutionStats(executionStats *structpb.Struct, param option.Options) string {
	var statsBuf bytes.Buffer

	executionStatsFields := executionStats.GetFields()
	if param.ExecutionStats {
		statsBuf.WriteString(renderExecutionStatsWithoutSummary(executionStatsFields))
	}

	if param.ExecutionSummary {
		statsBuf.WriteString(renderExecutionSummary(executionStatsFields))
	}
	return statsBuf.String()
}

type queryNode struct {
	queryStats *sppb.ResultSetStats
}

func toQueryNode(queryStats *sppb.ResultSetStats) *queryNode {
	return &queryNode{queryStats}
}
func (qn *queryNode) QueryText() string {
	return qn.queryStats.GetQueryStats().GetFields()["query_text"].GetStringValue()
}

func (qn *queryNode) QueryStats() []string {
	var stats []string
	for k, v := range qn.queryStats.GetQueryStats().GetFields() {
		if k == "query_text" {
			continue
		}
		stats = append(stats, fmt.Sprintf("%s: %s", k, v.GetStringValue()))
	}

	sort.Strings(stats)
	return stats
}

func setupQueryNode(graph *cgraph.Graph, queryStats *sppb.ResultSetStats, showQueryStats bool) (*cgraph.Node, error) {
	qn := toQueryNode(queryStats)
	str := renderQueryNodeLabel(qn, showQueryStats)

	s, err := graph.StrdupHTML(str)
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

func renderQueryNodeLabel(qn *queryNode, showQueryStats bool) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "<b>%s</b>", toLeftAlignedText(qn.QueryText()))
	if showQueryStats {
		fmt.Fprint(&buf, encloseIfNotEmpty("<i>", toLeftAlignedText(strings.Join(qn.QueryStats(), "\n")), "</i>"))
	}

	return buf.String()
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

func renderMetadata(metadataFields map[string]*structpb.Value, hideMetadata []string) string {
	var metadataStrs []string
	for k, v := range metadataFields {
		switch {
		case in(k, hideMetadata...):
			continue
		case in(k, "call_type", "scan_type", "scan_target", "iterator_type", "subquery_cluster_node"):
			continue
		default:
			metadataStrs = append(metadataStrs, fmt.Sprintf("%s=%v", k, v.AsInterface()))
		}
	}

	slices.Sort(metadataStrs)
	metadataStr := strings.Join(metadataStrs, "\n")

	if metadataStr == "" {
		return ""
	}
	return metadataStr + "\n"
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
	return executionSummaryBuf.String()
}

var newlineOrEOSRe = regexp.MustCompile(`\n?$|\n`)

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

func getScalarChildLinks(qp *spannerplan.QueryPlan, node *sppb.PlanNode, filter func(link *sppb.PlanNode_ChildLink) bool) []*childLinkGroup {
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

func getNonVariableChildLinks(plan *spannerplan.QueryPlan, node *sppb.PlanNode) []*childLinkGroup {
	return getScalarChildLinks(plan, node, func(node *sppb.PlanNode_ChildLink) bool {
		return node.GetVariable() == ""
	})
}

func getVariableChildLinks(plan *spannerplan.QueryPlan, node *sppb.PlanNode) []*childLinkGroup {
	return getScalarChildLinks(plan, node, func(node *sppb.PlanNode_ChildLink) bool {
		return node.GetVariable() != ""
	})
}
