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
	node, err := buildNode(planNode)
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
	planNode *sppb.PlanNode

	// Essential fields for graph structure
	Children []*link
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

// escapeMermaidLabelContent prepares a string for safe inclusion in a Mermaid label when htmlLabels:true is used.
// This function specifically handles escaping for Mermaid.js HTML-like label syntax.
// Note: Mermaid.js always processes Markdown-like syntax features in labels (such as backticks
// for code blocks), regardless of htmlLabels setting.
func escapeMermaidLabelContent(content string) string {
	return replacerForMermaid[true].Replace(content)
}

var replacerForMermaid = map[bool]*strings.Replacer{
	true:  newReplacerForMermaidHTMLLabel(true),
	false: newReplacerForMermaidHTMLLabel(false),
}

// newReplacerForMermaidHTMLLabel creates a strings.Replacer for escaping text content in Mermaid diagram labels.
// It handles both HTML-like character escaping and Mermaid-specific character escaping requirements.
//
// Parameters:
//   - replaceSpaceToNbsp: when true, replaces spaces with &nbsp; entities for preserving whitespace in HTML-like contexts
//
// The replacer handles three types of escaping:
//  1. HTML entity escaping for '<', '>', and '&'
//  2. Mermaid-specific character escaping for special syntax characters
//  3. Optional space-to-nbsp conversion
//
// The escaping is done in a specific order to prevent interference between different escape sequences.
func newReplacerForMermaidHTMLLabel(replaceSpaceToNbsp bool) *strings.Replacer {
	// HTML entity escapes must be handled in specific order to prevent interference:
	// 1. & escaped first (prevent double-escaping)
	// 2. < and > escaped for HTML safety
	htmlLikeEscapeChars := []string{
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	}

	// charsToEscape contains characters that need escaping in Mermaid labels.
	// These characters match the marked library's escape constant
	// (https://github.com/markedjs/marked/blob/v15.0.12/src/rules.ts)
	// and are sorted by ASCII code.
	// Note: htmlLikeEscapeChars has higher precedence than this list
	const charsToEscape = `!"#$%&'()*+,-./:;<=>?@[\]^_` + "`" + `{|}~`

	var escapeCharsForReplacer []string
	for _, r := range charsToEscape {
		escapeCharsForReplacer = append(escapeCharsForReplacer, string(r), `\`+string(r))
	}

	var oldNew []string
	if replaceSpaceToNbsp {
		oldNew = slices.Concat([]string{" ", "&nbsp;"}, htmlLikeEscapeChars, escapeCharsForReplacer)
	} else {
		oldNew = slices.Concat(htmlLikeEscapeChars, escapeCharsForReplacer)
	}

	return strings.NewReplacer(oldNew...)
}

// escapeGraphvizHTMLLabelContent prepares a string for safe inclusion in a Graphviz HTML-like label.
// This function escapes characters that have special meaning in XML/HTML contexts,
// as Graphviz HTML-like labels are parsed as a form of XML.
// It also handles characters known to cause issues, like backticks.
// escapeGraphvizHTMLLabelContent prepares a string for safe inclusion in a Graphviz HTML-like label.
// Note: Graphviz's HTML-like label parsing has its own specific rules that differ from standard
// HTML/XML parsing. This function escapes special characters according to Graphviz's requirements.
// See: https://graphviz.org/doc/info/shapes.html#html for details on Graphviz HTML-like labels.
func escapeGraphvizHTMLLabelContent(content string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`\`, `\\`,
	)
	return replacer.Replace(content)
}

// getNodeContent extracts and formats the raw content of a treeNode into a structured nodeContent.
// This function centralizes the logic for gathering all relevant displayable information
// from a plan node, before any Mermaid-specific or plain-text-specific formatting.
func (n *treeNode) getNodeContent(qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) nodeContent {
	content := nodeContent{
		Title:               n.GetTitle(param),
		ShortRepresentation: n.GetShortRepresentation(),
		ScanInfo:            n.GetScanInfoOutput(param),
		SerializeResult:     []string{}, // Initialize as empty slice
		NonVarScalarLinks:   []string{},
		Metadata:            n.GetMetadata(param),
		VarScalarLinks:      []string{},
		Stats:               n.GetStats(param),
		ExecutionSummary:    n.GetExecutionSummary(),
	}

	// Handle multi-line outputs
	if sroVal := n.GetSerializeResultOutput(qp, rowType); sroVal != "" {
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
	isScanNode := n.planNode.GetDisplayName() == "Scan"
	if isScanNode && content.ScanInfo != "" && content.ScanInfo == content.ShortRepresentation && content.ShortRepresentation != "" {
		// If it's a scan node, and ScanInfo is identical to ShortRepresentation, and ShortRepresentation is not empty,
		// then clear ScanInfo to avoid double-printing.
		content.ScanInfo = ""
	}

	return content
}

// MermaidLabel generates the label string for this node, suitable for use in Mermaid diagrams.
func (n *treeNode) MermaidLabel(qp *spannerplan.QueryPlan, param option.Options, rowType *sppb.StructType) string {
	content := n.getNodeContent(qp, param, rowType)
	var labelParts []string

	if content.Title != "" {
		labelParts = append(labelParts, fmt.Sprintf("%s", escapeMermaidLabelContent(content.Title)))
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
			statLines = append(statLines, fmt.Sprintf("%s: %s", escapeMermaidLabelContent(k), escapeMermaidLabelContent(content.Stats[k])))
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
			labelParts = append(labelParts, fmt.Sprintf("%s", strings.Join(summaryLines, "\n")))
		}
	}

	labelContent := strings.Join(labelParts, "\n")
	if labelContent == "" {
		labelContent = escapeMermaidLabelContent(n.GetName())
	}

	return strings.ReplaceAll(labelContent, "\"", "#quot;")
}

// GetName generates the node's unique ID for graph rendering.
func (n *treeNode) GetName() string {
	if n.planNode == nil {
		return "node_unknown" // Fallback for safety, though planNode should always be set
	}
	return fmt.Sprintf("node%d", n.planNode.GetIndex())
}

// GetTooltip generates the tooltip string (YAML of the planNode) on demand.
func (n *treeNode) GetTooltip() (string, error) {
	tooltipBytes, err := yaml.Marshal(n.planNode)
	if err != nil {
		return "", fmt.Errorf("failed to marshal planNode for tooltip: %w", err)
	}
	return string(tooltipBytes), nil
}

func (n *treeNode) GetTitle(param option.Options) string {
	// Always use HideMetadata() as per user feedback that implies metadata should be hidden from the title string itself.
	return spannerplan.NodeTitle(n.planNode, spannerplan.HideMetadata())
}

func (n *treeNode) GetShortRepresentation() string {
	return n.planNode.GetShortRepresentation().GetDescription()
}

func (n *treeNode) GetScanInfoOutput(param option.Options) string {
	if param.HideScanTarget {
		return ""
	}

	metadataFields := n.planNode.GetMetadata().GetFields()
	if scanTypeVal := metadataFields["scan_type"].GetStringValue(); scanTypeVal != "" {
		return fmt.Sprintf("%s: %s", strings.TrimSuffix(scanTypeVal, "Scan"), metadataFields["scan_target"].GetStringValue())
	}
	return ""
}

func (n *treeNode) GetSerializeResultOutput(qp *spannerplan.QueryPlan, rowType *sppb.StructType) string {
	if n.planNode.GetDisplayName() != "Serialize Result" {
		return ""
	}
	// formatSerializeResult expects childLinkGroups which getNonVariableChildLinks provides
	return formatSerializeResult(rowType, getNonVariableChildLinks(qp, n.planNode))
}

func (n *treeNode) GetNonVarScalarLinksOutput(qp *spannerplan.QueryPlan, param option.Options) string {
	return formatChildLinks(getNonVariableChildLinks(qp, n.planNode))
}

func (n *treeNode) GetVarScalarLinksOutput(qp *spannerplan.QueryPlan, param option.Options) string {
	return formatChildLinks(getVariableChildLinks(qp, n.planNode))
}

func (n *treeNode) GetMetadata(param option.Options) map[string]string {
	result := make(map[string]string)
	for k, v := range n.planNode.GetMetadata().GetFields() {
		if slices.Contains(param.HideMetadata, k) || slices.Contains(internalMetadataKeys, k) {
			continue
		}
		result[k] = fmt.Sprint(v.AsInterface())
	}
	return result
}

func (n *treeNode) GetStats(param option.Options) map[string]string {
	if !param.ExecutionStats {
		return nil
	}

	statsMap := make(map[string]string)
	for key, valProto := range n.planNode.GetExecutionStats().GetFields() {
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
			statsMap[key] = fmt.Sprint(valProto.AsInterface())
		}
	}
	return statsMap
}

func (n *treeNode) GetExecutionSummary() string {
	if n.planNode == nil || n.planNode.GetExecutionStats() == nil {
		return ""
	}
	return formatExecutionSummary(n.planNode.GetExecutionStats().GetFields())
}

// Metadata formats node content for GraphViz HTML-like labels.
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

	// All lines in labelLines are now raw strings (or escaped key=value), to be processed by toLeftAlignedText.
	labelHTMLPart := toLeftAlignedText(strings.Join(labelLines, "\n"))

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
	// All lines in statsAndSummaryPlainLines are raw strings, toLeftAlignedText will escape them.
	statsHTMLPart := markupIfNotEmpty(toLeftAlignedText(strings.Join(statsAndSummaryPlainLines, "\n")), "i")
	return labelHTMLPart + statsHTMLPart
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

func buildNode(planNode *sppb.PlanNode) (*treeNode, error) {
	if planNode == nil {
		return nil, fmt.Errorf("buildNode: received nil planNode")
	}

	return &treeNode{
		planNode: planNode,
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

func formatExecutionSummary(executionStatsFields map[string]*structpb.Value) string {
	executionSummary, ok := executionStatsFields["execution_summary"]
	if !ok {
		return ""
	}

	var executionSummaryBuf bytes.Buffer
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

		const indentLevel = 3
		executionSummaryStrings = append(executionSummaryStrings,
			fmt.Sprintf("%s%s: %s\n", strings.Repeat(" ", indentLevel), k, value))
	}
	sort.Strings(executionSummaryStrings)
	fmt.Fprint(&executionSummaryBuf, strings.Join(executionSummaryStrings, ""))
	return executionSummaryBuf.String()
}

var newlineOrEOSRe = regexp.MustCompile(`\n?$|\n`)

func toLeftAlignedText(str string) string {
	if str == "" {
		return ""
	}

	return newlineOrEOSRe.ReplaceAllString(str, `<br align="left" />`)
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
	if value == "" {
		return ""
	}

	return prefix + value
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

	return fmt.Sprintf("%s%s%s", total, meanStr, unitStr)
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
		for _, childLinkEntry := range cl.PlanNodes {
			if childLinkEntry.Variable == "" && cl.Type == "" {
				continue
			}

			// it is safe even if nil receiver
			description := childLinkEntry.PlanNodes.GetShortRepresentation().GetDescription()

			if childLinkEntry.Variable == "" {
				fmt.Fprintf(&buf, "%s%s\n", prefix, description)
			} else {
				fmt.Fprintf(&buf, "%s$%s:=%s\n", prefix, childLinkEntry.Variable, description)
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

			description := planNodeEntry.PlanNodes.GetShortRepresentation().GetDescription()
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
	buf.WriteString(markupIfNotEmpty(toLeftAlignedText(escapeGraphvizHTMLLabelContent(text)), "b")) // Changed to toLeftAlignedText
	if showQueryStats {
		statsStr := formatQueryStats(m)
		buf.WriteString(markupIfNotEmpty(toLeftAlignedText(escapeGraphvizHTMLLabelContent(statsStr)), "i")) // Changed to toLeftAlignedText
	}
	return buf.String()
}
