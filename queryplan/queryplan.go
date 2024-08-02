package queryplan

import (
	"fmt"
	"sort"
	"strings"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
)

type QueryPlan struct {
	planNodes []*spannerpb.PlanNode
}

func New(planNodes []*spannerpb.PlanNode) *QueryPlan {
	if len(planNodes) == 0 {
		panic("planNodes is empty")
	}
	return &QueryPlan{planNodes}
}

func (qp *QueryPlan) IsFunction(childLink *spannerpb.PlanNode_ChildLink) bool {
	// Known predicates are Condition(Filter, Hash Join) or Seek Condition(FilterScan) or Residual Condition(FilterScan, Hash Join) or Split Range(Distributed Union).
	// Agg(Aggregate) is a Function but not a predicate.
	child := qp.GetNodeByChildLink(childLink)
	return child.DisplayName == "Function"
}

func (qp *QueryPlan) IsPredicate(childLink *spannerpb.PlanNode_ChildLink) bool {
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

func (qp *QueryPlan) PlanNodes() []*spannerpb.PlanNode {
	return qp.planNodes
}

func (qp *QueryPlan) GetNodeByIndex(id int32) *spannerpb.PlanNode {
	return qp.planNodes[id]
}

func (qp *QueryPlan) IsVisible(link *spannerpb.PlanNode_ChildLink) bool {
	return qp.GetNodeByChildLink(link).GetKind() == spannerpb.PlanNode_RELATIONAL || link.GetType() == "Scalar"
}

func (qp *QueryPlan) VisibleChildLinks(node *spannerpb.PlanNode) []*spannerpb.PlanNode_ChildLink {
	var links []*spannerpb.PlanNode_ChildLink
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
func (qp *QueryPlan) GetNodeByChildLink(link *spannerpb.PlanNode_ChildLink) *spannerpb.PlanNode {
	return qp.planNodes[link.GetChildIndex()]
}

func NodeTitle(node *spannerpb.PlanNode) string {
	metadataFields := node.GetMetadata().GetFields()

	operator := joinIfNotEmpty(" ",
		metadataFields["call_type"].GetStringValue(),
		metadataFields["iterator_type"].GetStringValue(),
		strings.TrimSuffix(metadataFields["scan_type"].GetStringValue(), "Scan"),
		node.GetDisplayName(),
	)

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
			fields = append(fields, fmt.Sprintf("%s: %s",
				strings.TrimSuffix(metadataFields["scan_type"].GetStringValue(), "Scan"),
				v.GetStringValue()))
		default:
			fields = append(fields, fmt.Sprintf("%s: %s", k, v.GetStringValue()))
		}
	}

	sort.Strings(fields)

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

func (qp *QueryPlan) ResolveChildLink(item *spannerpb.PlanNode_ChildLink) *ResolvedChildLink {
	return &ResolvedChildLink{
		ChildLink: item,
		Child:     qp.GetNodeByChildLink(item),
	}
}

type ResolvedChildLink struct {
	ChildLink *spannerpb.PlanNode_ChildLink
	Child     *spannerpb.PlanNode
}
