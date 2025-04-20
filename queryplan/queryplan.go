package queryplan

import (
	"cmp"
	"fmt"
	"sort"
	"strings"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/samber/lo"
)

type QueryPlan struct {
	planNodes []*sppb.PlanNode
	parentMap map[int32]int32
}

func New(planNodes []*sppb.PlanNode) *QueryPlan {
	if len(planNodes) == 0 {
		panic("planNodes is empty")
	}

	parentMap := make(map[int32]int32)
	for _, planNode := range planNodes {
		for _, childLink := range planNode.GetChildLinks() {
			parentMap[childLink.GetChildIndex()] = planNode.GetIndex()
		}
	}

	return &QueryPlan{planNodes, parentMap}
}

func (qp *QueryPlan) IsFunction(childLink *sppb.PlanNode_ChildLink) bool {
	// Known predicates are Condition(Filter, Hash Join) or Seek Condition(FilterScan) or Residual Condition(FilterScan, Hash Join) or Split Range(Distributed Union).
	// Agg(Aggregate) is a Function but not a predicate.
	child := qp.GetNodeByChildLink(childLink)
	return child.DisplayName == "Function"
}

func (qp *QueryPlan) IsPredicate(childLink *sppb.PlanNode_ChildLink) bool {
	// Known predicates are Condition(Filter, Hash Join) or Seek Condition(FilterScan) or Residual Condition(FilterScan, Hash Join) or Split Range(Distributed Union).
	// Agg(Aggregate) is a Function but not a predicate.
	if !qp.IsFunction(childLink) {
		return false
	}

	if strings.HasSuffix(childLink.GetType(), "Condition") || childLink.GetType() == "Split Range" {
		return true
	}
	return false
}

func (qp *QueryPlan) PlanNodes() []*sppb.PlanNode {
	return qp.planNodes
}

func (qp *QueryPlan) GetNodeByIndex(id int32) *sppb.PlanNode {
	return qp.planNodes[id]
}

func (qp *QueryPlan) IsVisible(link *sppb.PlanNode_ChildLink) bool {
	return qp.GetNodeByChildLink(link).GetKind() == sppb.PlanNode_RELATIONAL || link.GetType() == "Scalar"
}

func (qp *QueryPlan) VisibleChildLinks(node *sppb.PlanNode) []*sppb.PlanNode_ChildLink {
	var links []*sppb.PlanNode_ChildLink
	for _, link := range node.GetChildLinks() {
		if !qp.IsVisible(link) {
			continue
		}
		links = append(links, link)
	}
	return links
}

// GetNodeByChildLink returns PlanNode indicated by `link`.
// If `link` is nil, return the root node.
func (qp *QueryPlan) GetNodeByChildLink(link *sppb.PlanNode_ChildLink) *sppb.PlanNode {
	return qp.planNodes[link.GetChildIndex()]
}

func (qp *QueryPlan) GetParentNodeByChildIndex(index int32) *sppb.PlanNode {
	return qp.planNodes[qp.parentMap[index]]
}

func (qp *QueryPlan) GetParentNodeByChildLink(link *sppb.PlanNode_ChildLink) *sppb.PlanNode {
	return qp.GetParentNodeByChildIndex(link.GetChildIndex())
}

type option struct {
	executionMethodFormat ExecutionMethodFormat
	targetMetadataFormat  TargetMetadataFormat
	fullScanFormat        FullScanFormat
}

type Option func(o *option)

type ExecutionMethodFormat int64

const (
	// ExecutionMethodFormatRaw prints execution_method metadata as is.
	ExecutionMethodFormatRaw ExecutionMethodFormat = iota

	// ExecutionMethodFormatAngle prints execution_method metadata after display_name with angle bracket like `Scan <Row>`.
	ExecutionMethodFormatAngle
)

type TargetMetadataFormat int64

const (
	// TargetMetadataFormatRaw prints scan_target and distribution_table metadata as is.
	TargetMetadataFormatRaw TargetMetadataFormat = iota

	// TargetMetadataFormatOn prints scan_target and distribution_table metadata as `on <target>`.
	TargetMetadataFormatOn
)

type FullScanFormat int64

const (
	// FullScanFormatRaw prints Full scan metadata as is.
	FullScanFormatRaw FullScanFormat = iota

	// FullScanFormatLabel prints Full scan metadata as "Full scan" without value.
	FullScanFormatLabel
)

func WithExecutionMethodFormat(fmt ExecutionMethodFormat) Option {
	return func(o *option) {
		o.executionMethodFormat = fmt
	}
}

func WithTargetMetadataFormat(fmt TargetMetadataFormat) Option {
	return func(o *option) {
		o.targetMetadataFormat = fmt
	}
}

func WithFullScanFormat(fmt FullScanFormat) Option {
	return func(o *option) {
		o.fullScanFormat = fmt
	}
}

func NodeTitle(node *sppb.PlanNode, opts ...Option) string {
	var o option
	for _, opt := range opts {
		opt(&o)
	}

	metadataFields := node.GetMetadata().GetFields()

	executionMethod := metadataFields["execution_method"].GetStringValue()
	target := cmp.Or(metadataFields["scan_target"].GetStringValue(), metadataFields["distribution_table"].GetStringValue())

	operator := joinIfNotEmpty(" ",
		metadataFields["call_type"].GetStringValue(),
		metadataFields["iterator_type"].GetStringValue(),
		strings.TrimSuffix(metadataFields["scan_type"].GetStringValue(), "Scan"),
		node.GetDisplayName(),
		lo.Ternary(o.targetMetadataFormat == TargetMetadataFormatOn && len(target) > 0,
			"on "+target, ""),
		lo.Ternary(o.executionMethodFormat == ExecutionMethodFormatAngle && len(executionMethod) > 0,
			"<"+executionMethod+">", ""),
	)

	var needFullscanToFront bool
	fields := make([]string, 0)
	for k, v := range metadataFields {
		switch k {
		case "call_type", "iterator_type": // Skip because it is displayed in node title
			continue
		case "scan_type": // Skip because it is combined with scan_target
			continue
		case "subquery_cluster_node": // Skip because it is useless
			continue
		case "scan_target":
			if o.targetMetadataFormat != TargetMetadataFormatRaw {
				continue
			}

			fields = append(fields, fmt.Sprintf("%s: %s",
				strings.TrimSuffix(metadataFields["scan_type"].GetStringValue(), "Scan"),
				v.GetStringValue()))
			continue
		case "execution_method":
			if o.executionMethodFormat != ExecutionMethodFormatRaw {
				continue
			}
		case "distribution_table":
			if o.targetMetadataFormat != TargetMetadataFormatRaw {
				continue
			}
		case "Full scan":
			if o.fullScanFormat != FullScanFormatRaw && v.GetStringValue() == "true" {
				needFullscanToFront = true
				continue
			}
		}
		fields = append(fields, fmt.Sprintf("%s: %s", k, v.GetStringValue()))
	}

	sort.Strings(fields)
	if needFullscanToFront {
		fields = append([]string{"Full scan"}, fields...)
	}

	return joinIfNotEmpty(" ", operator, encloseIfNotEmpty("(", strings.Join(fields, ", "), ")"))
}

func encloseIfNotEmpty(open, input, close string) string {
	if input == "" {
		return ""
	}
	return open + input + close
}

func joinIfNotEmpty(sep string, input ...string) string {
	var filtered []string
	for _, s := range input {
		if s != "" {
			filtered = append(filtered, s)
		}
	}
	return strings.Join(filtered, sep)
}

func (qp *QueryPlan) ResolveChildLink(item *sppb.PlanNode_ChildLink) *ResolvedChildLink {
	return &ResolvedChildLink{
		ChildLink: item,
		Child:     qp.GetNodeByChildLink(item),
	}
}

type ResolvedChildLink struct {
	ChildLink *sppb.PlanNode_ChildLink
	Child     *sppb.PlanNode
}
