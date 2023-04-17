package queryplan

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/genproto/googleapis/spanner/v1"
)

type QueryPlan struct {
	planNodes []*spanner.PlanNode
}

func New(planNodes []*spanner.PlanNode) *QueryPlan {
	if len(planNodes) == 0 {
		panic("planNodes is empty")
	}
	return &QueryPlan{planNodes}
}

func (qp *QueryPlan) IsPredicate(childLink *spanner.PlanNode_ChildLink) bool {
	// Known predicates are Condition(Filter, Hash Join) or Seek Condition(FilterScan) or Residual Condition(FilterScan, Hash Join) or Split Range(Distributed Union).
	// Agg(Aggregate) is a Function but not a predicate.
	child := qp.GetNodeByChildLink(childLink)
	if child.DisplayName != "Function" {
		return false
	}
	if strings.HasSuffix(childLink.GetType(), "Condition") || childLink.GetType() == "Split Range" {
		return true
	}
	return false
}

func (qp *QueryPlan) PlanNodes() []*spanner.PlanNode {
	return qp.planNodes
}

func (qp *QueryPlan) GetNodeByIndex(id int32) *spanner.PlanNode {
	return qp.planNodes[id]
}

func (qp *QueryPlan) IsVisible(link *spanner.PlanNode_ChildLink) bool {
	return qp.GetNodeByChildLink(link).GetKind() == spanner.PlanNode_RELATIONAL || link.GetType() == "Scalar"
}

func (qp *QueryPlan) VisibleChildLinks(node *spanner.PlanNode) []*spanner.PlanNode_ChildLink {
	var links []*spanner.PlanNode_ChildLink
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
func (qp *QueryPlan) GetNodeByChildLink(link *spanner.PlanNode_ChildLink) *spanner.PlanNode {
	return qp.planNodes[link.GetChildIndex()]
}

func NodeTitle(node *spanner.PlanNode) string {
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
