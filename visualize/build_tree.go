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
		childNode, err := buildTree(qp, qp.GetNodeByChildLink(cl), rowType, param)
		if err != nil {
			return nil, err
		}

		edge := buildLink(qp, cl, planNode, childNode)
		edges = append(edges, edge)
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
	gvChildNode, err := graph.NodeByName(edge.ChildNode.GetName())
	if err != nil {
		return err
	}

	gvNode, err := graph.NodeByName(parent.GetName())
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
	// Core data field
	planNodeProto *sppb.PlanNode

	// Essential fields for graph structure
	Children []*link

	// Removed fields:
	// plan *spannerplan.QueryPlan
	// Name string
	// Tooltip string
}

// nodeContent holds the raw, unformatted content extracted from a plan node,
// before any Mermaid-specific escaping or final text formatting.
type nodeContent struct {
	Title               string
	ShortRepresentation string
	ScanInfo            string
	SerializeResult     []string
	NonVarScalarLinks   []string
	Metadata            map[string]string
	VarScalarLinks      []string
	Stats               map[string]string
	ExecutionSummary    string
}

// escapeMermaidLabelContent prepares a string for safe inclusion in a Mermaid label.
// Copied from mermaid.go for use in treeNode.MermaidLabel.
func escapeMermaidLabelContent(content string) string {
	// Basic HTML escaping
	// content = strings.ReplaceAll(content, "&", "&") // Not strictly needed if #quot; etc. are used
	// content = strings.ReplaceAll(content, "<", "<") // Avoid if using <br/>, <b>
	// content = strings.ReplaceAll(content, ">", ">") // Avoid if using <br/>, <b>

	// Critical: Escape characters that break Mermaid syntax if not inside quotes,
	// or that break the label string itself.
	// Backticks are problematic.
	content = strings.ReplaceAll(content, "`", "#96;")
	// Double quotes are handled by the caller for the overall label["..."] syntax.
	// However, if content itself has double quotes that are part of the text,
	// they need to be escaped for the HTML-like context within the label.
	content = strings.ReplaceAll(content, "\"", "#quot;") // Escape internal quotes
	return content
}

// escapeGraphvizHTMLLabelContent prepares a string for safe inclusion in a Graphviz HTML label.
// It escapes HTML special characters like '&', '<', '>', but specifically avoids escaping single quotes.
func escapeGraphvizHTMLLabelContent(content string) string {
	// Order matters: escape '&' first to avoid double-escaping already escaped entities.
	content = strings.ReplaceAll(content, "&", "&amp;")
	content = strings.ReplaceAll(content, "<", "&lt;")
	content = strings.ReplaceAll(content, ">", "&gt;")
	// Do NOT escape single quotes (apostrophes) as per user feedback for Graphviz DOT HTML labels.
	return content
}

// getNodeContent extracts and formats the raw content of a treeNode into a structured nodeContent.
// This function centralizes the logic for gathering all relevant displayable information
// from a plan node, before any Mermaid-specific or plain-text-specific formatting.
func (n *treeNode) getNodeContent(qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) nodeContent {
	if n.planNodeProto == nil {
		return nodeContent{} // Return empty struct for nil proto
	}

	content := nodeContent{
		Title:               n.GetTitle(param),
		ShortRepresentation: n.GetShortRepresentation(),
		ScanInfo:            n.GetScanInfoOutput(param),
		SerializeResult:     []string{}, // Initialize as empty slice
		NonVarScalarLinks:   []string{},
		Metadata:            n.GetMetadata(param),
		VarScalarLinks:      []string{},
		Stats:               n.GetStats(param),
		ExecutionSummary:    n.GetExecutionSummary(param),
	}

	// Handle multi-line outputs
	if sroVal := n.GetSerializeResultOutput(qp, param, rowType); sroVal != "" {
		for _, line := range strings.Split(strings.TrimSuffix(sroVal, "\n"), "\n") {
			if line != "" {
				content.SerializeResult = append(content.SerializeResult, line)
			}
		}
	}
	if nvslVal := n.GetNonVarScalarLinksOutput(qp, param); nvslVal != "" {
		for _, line := range strings.Split(strings.TrimSuffix(nvslVal, "\n"), "\n") {
			if line != "" {
				content.NonVarScalarLinks = append(content.NonVarScalarLinks, line)
			}
		}
	}
	if vslVal := n.GetVarScalarLinksOutput(qp, param); vslVal != "" {
		for _, line := range strings.Split(strings.TrimSuffix(vslVal, "\n"), "\n") {
			if line != "" {
				content.VarScalarLinks = append(content.VarScalarLinks, line)
			}
		}
	}

	// Apply heuristic check for ScanInfo double-printing
	// This logic is moved from MermaidLabel to here to centralize content decision.
	isScanNode := n.planNodeProto.GetDisplayName() == "Scan" || strings.Contains(n.planNodeProto.GetDisplayName(), "Scan")
	if isScanNode && content.ScanInfo != "" && content.ScanInfo == content.ShortRepresentation && content.ShortRepresentation != "" {
		// If it's a scan node, and ScanInfo is identical to ShortRepresentation, and ShortRepresentation is not empty,
		// then clear ScanInfo to avoid double-printing.
		content.ScanInfo = ""
	}

	return content
}

// MermaidLabel generates the label string for this node, suitable for use in Mermaid diagrams.
func (n *treeNode) MermaidLabel(qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) string {
	if n.planNodeProto == nil {
		return escapeMermaidLabelContent("Error: nil planNodeProto")
	}

	content := n.getNodeContent(qp, param, rowType)
	var labelParts []string

	if content.Title != "" {
		labelParts = append(labelParts, fmt.Sprintf("<b>%s</b>", escapeMermaidLabelContent(content.Title)))
	}
	if content.ShortRepresentation != "" {
		labelParts = append(labelParts, escapeMermaidLabelContent(content.ShortRepresentation))
	}
	if content.ScanInfo != "" {
		labelParts = append(labelParts, escapeMermaidLabelContent(content.ScanInfo))
	}

	for _, line := range content.SerializeResult {
		labelParts = append(labelParts, escapeMermaidLabelContent(line))
	}
	for _, line := range content.NonVarScalarLinks {
		labelParts = append(labelParts, escapeMermaidLabelContent(line))
	}

	if len(content.Metadata) > 0 {
		var metaLines []string
		var metaKeys []string
		for k := range content.Metadata {
			metaKeys = append(metaKeys, k)
		}
		sort.Strings(metaKeys)
		for _, k := range metaKeys {
			metaLines = append(metaLines, fmt.Sprintf("%s: %s", escapeMermaidLabelContent(k), escapeMermaidLabelContent(content.Metadata[k])))
		}
		labelParts = append(labelParts, metaLines...)
	}

	for _, line := range content.VarScalarLinks {
		labelParts = append(labelParts, escapeMermaidLabelContent(line))
	}

	if len(content.Stats) > 0 {
		var statLines []string
		var statKeys []string
		for k := range content.Stats {
			statKeys = append(statKeys, k)
		}
		sort.Strings(statKeys)
		for _, k := range statKeys {
			statLines = append(statLines, fmt.Sprintf("<i>%s: %s</i>", escapeMermaidLabelContent(k), escapeMermaidLabelContent(content.Stats[k])))
		}
		labelParts = append(labelParts, statLines...)
	}

	if content.ExecutionSummary != "" {
		var summaryLines []string
		for _, line := range strings.Split(strings.TrimSuffix(content.ExecutionSummary, "\n"), "\n") {
			if line != "" {
				summaryLines = append(summaryLines, escapeMermaidLabelContent(line))
			}
		}
		if len(summaryLines) > 0 {
			labelParts = append(labelParts, fmt.Sprintf("<i>%s</i>", strings.Join(summaryLines, "<br/>")))
		}
	}

	labelContent := strings.Join(labelParts, "<br/>")
	if labelContent == "" {
		labelContent = escapeMermaidLabelContent(n.GetName())
	}

	return strings.ReplaceAll(labelContent, "\"", "#quot;")
}

// GetName generates the node's unique ID for graph rendering.
func (n *treeNode) GetName() string {
	if n.planNodeProto == nil {
		return "node_unknown" // Fallback for safety, though planNodeProto should always be set
	}
	return fmt.Sprintf("node%d", n.planNodeProto.GetIndex())
}

// GetTooltip generates the tooltip string (YAML of the planNodeProto) on demand.
func (n *treeNode) GetTooltip() (string, error) {
	if n.planNodeProto == nil {
		return "", fmt.Errorf("cannot generate tooltip for nil planNodeProto")
	}
	tooltipBytes, err := yaml.Marshal(n.planNodeProto)
	if err != nil {
		return "", fmt.Errorf("failed to marshal planNodeProto for tooltip: %w", err)
	}
	return string(tooltipBytes), nil
}

// New on-demand formatting methods for treeNode
func (n *treeNode) GetTitle(param option.Options) string { // param is now unused by this specific method
	if n.planNodeProto == nil {
		return ""
	}
	// Always use HideMetadata() as per user feedback that implies metadata should be hidden from the title string itself.
	return spannerplan.NodeTitle(n.planNodeProto, spannerplan.HideMetadata())
}

func (n *treeNode) GetShortRepresentation() string {
	if n.planNodeProto == nil || n.planNodeProto.GetShortRepresentation() == nil {
		return ""
	}
	return n.planNodeProto.GetShortRepresentation().GetDescription()
}

func (n *treeNode) GetScanInfoOutput(param option.Options) string {
	if n.planNodeProto == nil || n.planNodeProto.GetMetadata() == nil || param.HideScanTarget {
		return ""
	}

	metadataFields := n.planNodeProto.GetMetadata().GetFields()
	scanTypeVal, okType := metadataFields["scan_type"]
	scanTargetVal, okTarget := metadataFields["scan_target"]

	if okType && okTarget {
		scanTypeString := scanTypeVal.GetStringValue()

		processedScanType := strings.TrimSuffix(scanTypeString, "Scan")
		// If TrimSuffix results in an empty string (e.g. scanTypeString was "Scan"),
		// and the original string was not empty, use the original string.
		if processedScanType == "" && scanTypeString != "" {
			processedScanType = scanTypeString
		}

		scanTarget := scanTargetVal.GetStringValue()
		return fmt.Sprintf("%s: %s", processedScanType, scanTarget)
	}
	return ""
}

func (n *treeNode) GetSerializeResultOutput(qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) string {
	if n.planNodeProto == nil || n.planNodeProto.GetDisplayName() != "Serialize Result" || rowType == nil || qp == nil {
		return ""
	}
	// formatSerializeResult expects childLinkGroups which getNonVariableChildLinks provides
	return formatSerializeResult(rowType, getNonVariableChildLinks(qp, n.planNodeProto))
}

func (n *treeNode) GetNonVarScalarLinksOutput(qp *spannerplan.QueryPlan, param option.Options) string {
	if n.planNodeProto == nil || qp == nil {
		return ""
	}
	return formatChildLinks(getNonVariableChildLinks(qp, n.planNodeProto))
}

func (n *treeNode) GetVarScalarLinksOutput(qp *spannerplan.QueryPlan, param option.Options) string {
	if n.planNodeProto == nil || qp == nil {
		return ""
	}
	return formatChildLinks(getVariableChildLinks(qp, n.planNodeProto))
}

func (n *treeNode) GetMetadata(param option.Options) map[string]string {
	mdMap := make(map[string]string)
	if n.planNodeProto == nil || n.planNodeProto.GetMetadata() == nil {
		return mdMap
	}
	for key, valProto := range n.planNodeProto.GetMetadata().GetFields() {
		if slices.Contains(internalMetadataKeys, key) || slices.Contains(param.HideMetadata, key) {
			continue
		}
		// Exclude scan_type and scan_target as they are handled by GetScanInfoOutput
		if key == "scan_type" || key == "scan_target" {
			continue
		}
		formattedVal, err := formatStructPBValue(valProto)
		if err != nil {
			mdMap[key] = fmt.Sprintf("[unsupported_metadata_type:%T err:%v]", valProto.GetKind(), err)
		} else {
			mdMap[key] = formattedVal
		}
	}
	return mdMap
}

func (n *treeNode) GetStats(param option.Options) map[string]string {
	statsMap := make(map[string]string)
	if n.planNodeProto == nil || n.planNodeProto.GetExecutionStats() == nil || !param.ExecutionStats {
		return statsMap
	}

	for key, valProto := range n.planNodeProto.GetExecutionStats().GetFields() {
		if key == "execution_summary" { // Skip summary for this map
			continue
		}
		// Directly use formatExecutionStatsValue, assuming it's robust for stat structs
		// formatExecutionStatsValue returns a string, not an error.
		// It handles various fields within a stat struct.
		// If valProto itself is not a struct, formatExecutionStatsValue might misbehave.
		// It expects valProto to be the structpb.Value that IS the struct.
		if valProto.GetStructValue() != nil {
			statsMap[key] = formatExecutionStatsValue(valProto)
		} else {
			// If a top-level field in ExecutionStats is not a struct, it's unusual.
			// Use formatStructPBValue for simple types if they can appear here.
			simpleVal, err := formatStructPBValue(valProto) // Use the modified formatStructPBValue
			if err != nil {
				statsMap[key] = fmt.Sprintf("[unsupported_stat_field_type:%T]", valProto.GetKind())
			} else {
				statsMap[key] = simpleVal
			}
		}
	}
	return statsMap
}

func (n *treeNode) GetExecutionSummary(param option.Options) string {
	if n.planNodeProto == nil || n.planNodeProto.GetExecutionStats() == nil {
		return ""
	}
	return formatExecutionSummary(n.planNodeProto.GetExecutionStats().GetFields(), param.TypeFlag == "mermaid")
}

// New Metadata() and HTML() methods using on-demand getters
func (n *treeNode) Metadata(qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) string {
	content := n.getNodeContent(qp, param, rowType)
	var labelLines []string

	if content.ShortRepresentation != "" {
		labelLines = append(labelLines, escapeGraphvizHTMLLabelContent(content.ShortRepresentation))
	}
	if content.ScanInfo != "" {
		labelLines = append(labelLines, escapeGraphvizHTMLLabelContent(content.ScanInfo))
	}

	for _, line := range content.SerializeResult {
		labelLines = append(labelLines, escapeGraphvizHTMLLabelContent(line))
	}
	for _, line := range content.NonVarScalarLinks {
		labelLines = append(labelLines, escapeGraphvizHTMLLabelContent(line))
	}

	if len(content.Metadata) > 0 {
		var metaKVLines []string
		var metaKeys []string
		for k := range content.Metadata {
			metaKeys = append(metaKeys, k)
		}
		sort.Strings(metaKeys)
		for _, k := range metaKeys {
			metaKVLines = append(metaKVLines, fmt.Sprintf("%s=%s", escapeGraphvizHTMLLabelContent(k), escapeGraphvizHTMLLabelContent(content.Metadata[k])))
		}
		labelLines = append(labelLines, metaKVLines...)
	}

	for _, line := range content.VarScalarLinks {
		labelLines = append(labelLines, escapeGraphvizHTMLLabelContent(line))
	}

	// All lines in labelLines are now raw strings (or escaped key=value), to be processed by toLeftAlignedTextGraphviz.
	labelHTMLPart := toLeftAlignedTextGraphviz(strings.Join(labelLines, "\n"))

	// Reconstruct content similar to old n.Stats (detailed stats + summary)
	var statsAndSummaryPlainLines []string
	if len(content.Stats) > 0 {
		var statKVLines []string
		var statKeys []string
		for k := range content.Stats {
			statKeys = append(statKeys, k)
		}
		sort.Strings(statKeys)
		for _, k := range statKeys {
			statKVLines = append(statKVLines, fmt.Sprintf("%s: %s", escapeGraphvizHTMLLabelContent(k), escapeGraphvizHTMLLabelContent(content.Stats[k])))
		}
		statsAndSummaryPlainLines = append(statsAndSummaryPlainLines, statKVLines...)
	}

	if content.ExecutionSummary != "" {
		for _, line := range strings.Split(strings.TrimSuffix(content.ExecutionSummary, "\n"), "\n") {
			if line != "" {
				statsAndSummaryPlainLines = append(statsAndSummaryPlainLines, escapeGraphvizHTMLLabelContent(line))
			}
		}
	}
	// All lines in statsAndSummaryPlainLines are raw strings, toLeftAlignedTextGraphviz will escape them.
	statsHTMLPart := markupIfNotEmpty(toLeftAlignedTextGraphviz(strings.Join(statsAndSummaryPlainLines, "\n")), "i")

	if labelHTMLPart != "" && statsHTMLPart != "" {
		// toLeftAlignedText appends <br align="left"/> if its input is not empty.
		// So labelHTMLPart might end with it.
		return labelHTMLPart + statsHTMLPart
	}
	if labelHTMLPart != "" {
		return labelHTMLPart
	}
	return statsHTMLPart
}

func (n *treeNode) HTML(qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) string {
	titleHTML := ""
	if t := n.GetTitle(param); t != "" {
		// n.GetTitle calls spannerplan.NodeTitle which already HTML escapes its content.
		titleHTML = markupIfNotEmpty(t, "b")
	}

	metadataHTML := n.Metadata(qp, param, rowType)

	if titleHTML == "" && metadataHTML == "" {
		return html.EscapeString(n.GetName())
	}
	if titleHTML == "" {
		return metadataHTML
	}
	if metadataHTML == "" {
		return titleHTML
	}
	return fmt.Sprintf(`%s<br align="CENTER"/>%s`, titleHTML, metadataHTML)
}

// formatStructPBValue converts a structpb.Value to a string representation.
// It's used for metadata and potentially for direct stat values that aren't complex structs.
func formatStructPBValue(value *structpb.Value) (string, error) {
	if value == nil {
		return "", fmt.Errorf("formatStructPBValue: received nil Value")
	}
	switch v := value.GetKind().(type) {
	case *structpb.Value_NullValue:
		return "NULL", nil
	case *structpb.Value_NumberValue:
		return fmt.Sprintf("%g", v.NumberValue), nil
	case *structpb.Value_StringValue:
		return v.StringValue, nil
	case *structpb.Value_BoolValue:
		return fmt.Sprintf("%t", v.BoolValue), nil
	case *structpb.Value_StructValue:
		// No longer tries to parse as ExecutionStatValue.
		// Return a placeholder for generic structs in metadata.
		return "[Struct]", nil
		// Alternative: could try a shallow string representation if needed later:
		// var parts []string
		// for sk, sv := range v.StructValue.GetFields() {
		//    svStr, err := formatStructPBValue(sv) // Recursive, careful with depth
		//    if err != nil { parts = append(parts, fmt.Sprintf("%s:[error]", sk)) }
		//    else { parts = append(parts, fmt.Sprintf("%s:%s", sk, svStr)) }
		// }
		// return "{ " + strings.Join(parts, ", ") + " }", nil
	case *structpb.Value_ListValue:
		var items []string
		for i, itemVal := range v.ListValue.GetValues() {
			if i > 2 && len(v.ListValue.GetValues()) > 3 { // Limit displayed items for long lists
				items = append(items, "...")
				break
			}
			itemStr, err := formatStructPBValue(itemVal) // Recursive call
			if err != nil {
				items = append(items, "[error]")
			} else {
				items = append(items, itemStr)
			}
		}
		return "[" + strings.Join(items, ", ") + "]", nil
	default:
		return "", fmt.Errorf("unknown Value kind: %T", v)
	}
}

func buildNode(rowType *sppb.StructType, planNode *sppb.PlanNode, queryPlan *spannerplan.QueryPlan, param option.Options) (*treeNode, error) {
	// rowType and param are passed for consistency with buildTree, though not directly used in buildNode itself
	// after the treeNode struct changes. queryPlan is used by the caller buildTree.
	if planNode == nil {
		return nil, fmt.Errorf("buildNode: received nil planNode")
	}

	// Name, Tooltip, and plan are no longer fields of treeNode.
	// Children are populated by the buildTree function.
	return &treeNode{
		planNodeProto: planNode,
	}, nil
}

// internalMetadataKeys lists metadata keys that are considered internal
// and should not be displayed in the formatted metadata output.
var internalMetadataKeys = []string{
	"call_type",
	"scan_type",
	"scan_target",
	"iterator_type",
	"subquery_cluster_node",
}

func formatMetadata(metadataFields map[string]*structpb.Value, hideMetadata []string) string {
	if metadataFields == nil {
		return ""
	}
	var metadataStrs []string
	for k, v := range metadataFields {
		if slices.Contains(hideMetadata, k) || slices.Contains(internalMetadataKeys, k) {
			continue
		}
		metadataStrs = append(metadataStrs, fmt.Sprintf("%s=%v", k, v.AsInterface()))
	}
	slices.Sort(metadataStrs)
	metadataStr := strings.Join(metadataStrs, "\n")
	if metadataStr == "" {
		return ""
	}
	return metadataStr + "\n"
}

func formatExecutionSummary(executionStatsFields map[string]*structpb.Value, isMermaid bool) string {
	if executionStatsFields == nil {
		return ""
	}
	var executionSummaryBuf bytes.Buffer
	if executionSummary, ok := executionStatsFields["execution_summary"]; ok {
		fmt.Fprintln(&executionSummaryBuf, "execution_summary:")
		var executionSummaryStrings []string
		for k, v := range executionSummary.GetStructValue().AsMap() {
			var value string
			// Only apply tryToTimestampStr to specific timestamp fields
			if k == "execution_start_timestamp" || k == "execution_end_timestamp" {
				formattedValue, err := tryToTimestampStr(fmt.Sprint(v))
				if err != nil {
					value = fmt.Sprintf("%s (error: %v)", fmt.Sprint(v), err)
				} else {
					value = formattedValue
				}
			} else {
				value = fmt.Sprint(v)
			}
			if isMermaid {
				executionSummaryStrings = append(executionSummaryStrings, fmt.Sprintf("&nbsp;&nbsp;&nbsp;%s: %s\n", k, value))
			} else {
				executionSummaryStrings = append(executionSummaryStrings, fmt.Sprintf("   %s: %s\n", k, value))
			}
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

func toLeftAlignedTextGraphviz(str string) string {
	if str == "" {
		return ""
	}
	// Escaping for Graphviz HTML labels (no single quotes escaped)
	return newlineOrEOSRe.ReplaceAllString(escapeGraphvizHTMLLabelContent(str), `<br align="left" />`)
}

// tryToTimestampStr converts a string representation of a timestamp (seconds.microseconds)
// into a RFC3339Nano formatted string. For Spanner's execution_start_timestamp and
// execution_end_timestamp fields, the microseconds part is always expected to be
// exactly 6 digits long, with padding if necessary. Inputs that do not conform
// to this 6-digit microsecond format are considered invalid and will result in an error.
func tryToTimestampStr(s string) (string, error) {
	secStr, usecStr, found := strings.Cut(s, ".")

	sec, err := strconv.Atoi(secStr)
	if err != nil {
		return "", fmt.Errorf("invalid seconds in timestamp: %w", err)
	}

	if !found || len(usecStr) != 6 {
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

func formatExecutionStatsValue(v_struct *structpb.Value) string {
	if v_struct == nil || v_struct.GetStructValue() == nil {
		return ""
	}
	fields := v_struct.GetStructValue().GetFields()
	total := ""
	if totalVal, ok := fields["total"]; ok {
		total = totalVal.GetStringValue()
	}
	unit := ""
	if unitVal, ok := fields["unit"]; ok {
		unit = unitVal.GetStringValue()
	}
	mean := ""
	if meanVal, ok := fields["mean"]; ok {
		mean = meanVal.GetStringValue()
	}
	stdDev := ""
	if stdDevVal, okS := fields["std_deviation"]; okS {
		stdDev = stdDevVal.GetStringValue()
	}
	if total == "" && unit == "" && mean == "" && stdDev == "" {
		return ""
	}
	if total == "" && mean == "" && stdDev == "" && unit != "" {
		return ""
	}
	stdDevStr := prefixIfNotEmpty("±", stdDev)
	meanAndStdDevPart := ""
	if mean != "" {
		meanAndStdDevPart = "@" + mean + stdDevStr
	} else if stdDev != "" {
		meanAndStdDevPart = "@" + stdDevStr
	}
	value := total
	if meanAndStdDevPart != "" {
		value += meanAndStdDevPart
	}
	if unit != "" {
		if value != "" {
			value += " " + unit
		}
	}
	return value
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
		childLinks_ := typeToChildLinks[childType]
		childLinks_.PlanNodes = append(childLinks_.PlanNodes, &childLinkEntry{cl.GetVariable(), childNode})
	}
	return result
}

func getNonVariableChildLinks(plan *spannerplan.QueryPlan, node *sppb.PlanNode) []*childLinkGroup {
	return getScalarChildLinks(plan, node, func(node *sppb.PlanNode_ChildLink) bool { return node.GetVariable() == "" })
}

func getVariableChildLinks(plan *spannerplan.QueryPlan, node *sppb.PlanNode) []*childLinkGroup {
	return getScalarChildLinks(plan, node, func(node *sppb.PlanNode_ChildLink) bool { return node.GetVariable() != "" })
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
		for _, planNode_ := range cl.PlanNodes {
			if planNode_.Variable == "" && cl.Type == "" {
				continue
			}
			description := ""
			if psr := planNode_.PlanNodes.GetShortRepresentation(); psr != nil {
				description = psr.GetDescription()
			}
			if planNode_.Variable == "" {
				fmt.Fprintf(&buf, "%s%s\n", prefix, description)
			} else {
				fmt.Fprintf(&buf, "%s$%s:=%s\n", prefix, planNode_.Variable, description)
			}
		}
	}
	return buf.String()
}

func formatSerializeResult(rowType *sppb.StructType, childLinks []*childLinkGroup) string {
	var result bytes.Buffer
	for _, cl := range childLinks {
		if cl.Type != "" {
			continue
		}
		for i, planNodeEntry := range cl.PlanNodes {
			if rowType == nil || i >= len(rowType.GetFields()) {
				continue
			}
			name := rowType.GetFields()[i].GetName()
			if name == "" {
				name = fmt.Sprintf("no_name<%d>", i)
			}
			description := ""
			if psr := planNodeEntry.PlanNodes.GetShortRepresentation(); psr != nil {
				description = psr.GetDescription()
			}
			text := fmt.Sprintf("Result.%s:%s", name, description)
			fmt.Fprintln(&result, text)
		}
	}
	return result.String()
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
	buf.WriteString(markupIfNotEmpty(toLeftAlignedTextGraphviz(text), "b"))
	if showQueryStats {
		statsStr := formatQueryStats(m)
		buf.WriteString(markupIfNotEmpty(toLeftAlignedTextGraphviz(statsStr), "i"))
	}
	return buf.String()
}

func formatNodeContentAsText(node *treeNode, qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) []string {
	if node == nil {
		return nil
	}
	content := node.getNodeContent(qp, param, rowType)
	var result []string

	if content.Title != "" {
		result = append(result, fmt.Sprintf("Title: %s", content.Title))
	}
	if content.ShortRepresentation != "" {
		result = append(result, fmt.Sprintf("ShortRepresentation: %s", content.ShortRepresentation))
	}
	if content.ScanInfo != "" {
		result = append(result, fmt.Sprintf("ScanInfo: %s", content.ScanInfo))
	}

	for _, line := range content.SerializeResult {
		result = append(result, fmt.Sprintf("SerializeResult: %s", line))
	}

	for _, line := range content.NonVarScalarLinks {
		result = append(result, fmt.Sprintf("NonVarScalarLink: %s", line))
	}

	if len(content.Metadata) > 0 {
		var metaLines []string
		for k, v := range content.Metadata {
			metaLines = append(metaLines, fmt.Sprintf("Metadata: %s = %s", k, v))
		}
		sort.Strings(metaLines) // Ensure deterministic order for golden files
		result = append(result, metaLines...)
	}

	for _, line := range content.VarScalarLinks {
		result = append(result, fmt.Sprintf("VarScalarLink: %s", line))
	}

	if len(content.Stats) > 0 {
		var statLines []string
		for k, v := range content.Stats {
			statLines = append(statLines, fmt.Sprintf("Stat: %s: %s", k, v))
		}
		sort.Strings(statLines) // Ensure deterministic order for golden files
		result = append(result, statLines...)
	}

	if content.ExecutionSummary != "" {
		for _, line := range strings.Split(strings.TrimSuffix(content.ExecutionSummary, "\n"), "\n") {
			if line != "" {
				result = append(result, fmt.Sprintf("ExecutionSummary: %s", line))
			}
		}
	}

	return result
}

// End of visualize/build_tree.go
