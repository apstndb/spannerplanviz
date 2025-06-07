package visualize

import (
	"bytes"
	"fmt"
	"html"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/lox"
	"github.com/apstndb/spannerplan"
	"github.com/goccy/go-graphviz/cgraph"
	"google.golang.org/protobuf/types/known/structpb"
	"sigs.k8s.io/yaml"

	"github.com/apstndb/spannerplanviz/option"
)

// This file contains logics which are purely formatting strings and building tree structures.
// It is ok to depend on types in the cgraph package, but don't use graphviz.Graph in this file.

func buildTree(qp *spannerplan.QueryPlan, planNode *sppb.PlanNode, rowType *sppb.StructType, param option.Options) (*treeNode, error) {
	node, err := buildNode(rowType, planNode, qp, param)
	if err != nil {
		return nil, err
	}

	var edges []*link
	for _, cl := range qp.VisibleChildLinks(planNode) {
		if childNode, err := buildTree(qp, qp.GetNodeByChildLink(cl), rowType, param); err != nil {
			return nil, err
		} else {
			edge := buildLink(qp, cl, planNode, childNode)
			edges = append(edges, edge)
		}
	}

	node.Children = edges
	return node, nil
}

func buildLink(qp *spannerplan.QueryPlan, cl *sppb.PlanNode_ChildLink, node *sppb.PlanNode, child *treeNode) *link {
	return &link{
		ChildType: qp.GetLinkType(cl),
		// If it's a remote call, the connection will be rendered as a dashed line in the visualization.
		Style:     lox.IfOrEmpty(isRemoteCall(node, cl), cgraph.DashedEdgeStyle),
		ChildNode: child,
	}
}

type link struct {
	ChildType string
	Style     cgraph.EdgeStyle
	ChildNode *treeNode
}

func renderEdge(graph *cgraph.Graph, parent *treeNode, edge *link) error {
	gvChildNode, err := graph.NodeByName(edge.ChildNode.Name)
	if err != nil {
		return err
	}

	gvNode, err := graph.NodeByName(parent.Name)
	if err != nil {
		return err
	}

	ed, err := graph.CreateEdgeByName("", gvChildNode, gvNode)
	if err != nil {
		return err
	}

	ed.SetStyle(edge.Style)
	ed.SetLabel(edge.ChildType)
	return nil
}

// isRemoteCall determines if a link between nodes represents a remote call in the Spanner query plan.
// A remote call is identified if:
//  1. The parent node has a "subquery_cluster_node" metadata field, which contains the node ID of the
//     child that performs a remote operation.
//  2. The 'call_type' metadata field is not "Local". If 'call_type' is "Local", it is not a remote call.
//  3. The child link's index matches the value in "subquery_cluster_node", confirming this specific
//     child is the one executing remotely.
func isRemoteCall(node *sppb.PlanNode, cl *sppb.PlanNode_ChildLink) bool {
	metadataFields := node.GetMetadata().GetFields()

	subqueryClusterNode, ok := metadataFields["subquery_cluster_node"]
	if !ok {
		return false
	}

	callType := metadataFields["call_type"].GetStringValue()
	if callType == "Local" {
		return false
	}

	return subqueryClusterNode.GetStringValue() == strconv.Itoa(int(cl.GetChildIndex()))
}

type treeNode struct {
	Name, Label, Stats, Title, Tooltip string
	Children                           []*link
}

func (n *treeNode) Metadata() string {
	return toLeftAlignedText(n.Label) + markupIfNotEmpty(toLeftAlignedText(n.Stats), "i")
}

func (n *treeNode) HTML() string {
	return fmt.Sprintf(`%s<br align="CENTER" />%s`, markupIfNotEmpty(n.Title, "b"), n.Metadata())
}

func buildNode(rowType *sppb.StructType, planNode *sppb.PlanNode, queryPlan *spannerplan.QueryPlan, param option.Options) (*treeNode, error) {
	tooltipBytes, err := yaml.Marshal(planNode)
	if err != nil {
		return nil, err
	}

	return &treeNode{
		Label:   formatNodeLabel(planNode, queryPlan, param, rowType),
		Stats:   formatExecutionStats(planNode.GetExecutionStats(), param),
		Title:   spannerplan.NodeTitle(planNode, spannerplan.HideMetadata()),
		Name:    fmt.Sprintf("node%d", planNode.GetIndex()),
		Tooltip: string(tooltipBytes),
	}, nil
}

func formatNodeLabel(planNode *sppb.PlanNode, queryPlan *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) string {
	var sb strings.Builder

	childLinks := getNonVariableChildLinks(queryPlan, planNode)
	if param.SerializeResult && planNode.DisplayName == "Serialize Result" && rowType != nil {
		sb.WriteString(formatSerializeResult(rowType, childLinks))
	}

	metadataFields := planNode.GetMetadata().GetFields()

	if !param.HideScanTarget && planNode.GetDisplayName() == "Scan" {
		scanType := strings.TrimSuffix(metadataFields["scan_type"].GetStringValue(), "Scan")
		scanTarget := metadataFields["scan_target"].GetStringValue()
		s := fmt.Sprintf("%s: %s\n", scanType, scanTarget)
		sb.WriteString(s)
	}

	if param.NonVariableScalar {
		sb.WriteString(formatChildLinks(childLinks))
	}

	if param.Metadata {
		sb.WriteString(formatMetadata(metadataFields, param.HideMetadata))
	}

	if param.VariableScalar {
		sb.WriteString(formatChildLinks(getVariableChildLinks(queryPlan, planNode)))
	}
	return sb.String()
}

func formatExecutionStats(executionStats *structpb.Struct, param option.Options) string {
	var statsBuf bytes.Buffer

	executionStatsFields := executionStats.GetFields()
	if param.ExecutionStats {
		statsBuf.WriteString(formatExecutionStatsWithoutSummary(executionStatsFields))
	}

	if param.ExecutionSummary {
		statsBuf.WriteString(formatExecutionSummary(executionStatsFields))
	}
	return statsBuf.String()
}

func formatQueryStats(stats map[string]*structpb.Value) string {
	var result []string
	for k, v := range stats {
		result = append(result, fmt.Sprintf("%s: %s", k, v.GetStringValue()))
	}

	sort.Strings(result)
	return strings.Join(result, "\n")
}

func formatQueryNode(queryStats map[string]*structpb.Value, showQueryStats bool) string {
	m := maps.Clone(queryStats)

	const queryTextKey = "query_text"
	text := m[queryTextKey].GetStringValue()
	delete(m, queryTextKey)

	var buf strings.Builder
	buf.WriteString(markupIfNotEmpty(toLeftAlignedText(text), "b"))
	if showQueryStats {
		statsStr := formatQueryStats(m)
		buf.WriteString(markupIfNotEmpty(toLeftAlignedText(statsStr), "i"))
	}
	return buf.String()
}

func formatExecutionStatsWithoutSummary(executionStatsFields map[string]*structpb.Value) string {
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

func formatMetadata(metadataFields map[string]*structpb.Value, hideMetadata []string) string {
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

func formatExecutionSummary(executionStatsFields map[string]*structpb.Value) string {
	var executionSummaryBuf bytes.Buffer
	if executionSummary, ok := executionStatsFields["execution_summary"]; ok {
		fmt.Fprintln(&executionSummaryBuf, "execution_summary:")
		var executionSummaryStrings []string
		for k, v := range executionSummary.GetStructValue().AsMap() {
			var value string
			if strings.HasSuffix(k, "timestamp") {
				formattedValue, err := tryToTimestampStr(fmt.Sprint(v))
				if err != nil {
					value = fmt.Sprintf("%s (error: %v)", fmt.Sprint(v), err)
				} else {
					value = formattedValue
				}
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

func tryToTimestampStr(s string) (string, error) {
	secStr, usecStr, found := strings.Cut(s, ".")

	sec, err := strconv.Atoi(secStr)
	if err != nil {
		return "", fmt.Errorf("invalid seconds in timestamp: %w", err)
	}

	if !found || len(usecStr) != 6 { // Modified condition
		return "", fmt.Errorf("invalid timestamp format: %s (microseconds must be exactly 6 digits)", s)
	}

	usec, err := strconv.Atoi(usecStr)
	if err != nil {
		return "", fmt.Errorf("invalid microseconds in timestamp: %w", err)
	}

	return time.Unix(int64(sec), int64(usec)*1000).UTC().Format(time.RFC3339Nano), nil
}

func prefixIfNotEmpty(prefix, value string) string {
	if value != "" {
		return prefix + value
	}
	return ""
}

func formatExecutionStatsValue(v *structpb.Value) string {
	fields := v.GetStructValue().GetFields()
	total := fields["total"].GetStringValue()
	unit := fields["unit"].GetStringValue()
	mean := fields["mean"].GetStringValue()
	stdDev := fields["std_deviation"].GetStringValue()

	stdDevStr := prefixIfNotEmpty("±", stdDev)
	meanStr := prefixIfNotEmpty("@", mean+stdDevStr)
	unitStr := prefixIfNotEmpty(" ", unit)

	value := fmt.Sprintf("%s%s%s", total, meanStr, unitStr)
	return value
}

func formatSerializeResult(rowType *sppb.StructType, childLinks []*childLinkGroup) string {
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

func formatChildLinks(childLinks []*childLinkGroup) string {
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
