package plantree

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/apstndb/spannerplanviz/queryplan"
	"github.com/apstndb/spannerplanviz/stats"
	"github.com/xlab/treeprint"
	"google.golang.org/genproto/googleapis/spanner/v1"
)

func init() {
	// Use only ascii characters to mitigate ambiguous width problem
	treeprint.EdgeTypeLink = "|"
	treeprint.EdgeTypeMid = "+-"
	treeprint.EdgeTypeEnd = "+-"

	treeprint.IndentSize = 2
}

type RowWithPredicates struct {
	ID             int32
	TreePart       string
	NodeText       string
	Predicates     []string
	ExecutionStats stats.ExecutionStats
}

func (r RowWithPredicates) Text() string {
	return r.TreePart + r.NodeText
}

func (r RowWithPredicates) FormatID() string {
	if len(r.Predicates) == 0 {
		return fmt.Sprint(r.ID)
	}
	return fmt.Sprintf("*%d", r.ID)
}

func ProcessPlan(qp *queryplan.QueryPlan) (rows []RowWithPredicates, err error) {
	tree := treeprint.New()

	renderTree(qp, tree, nil)
	var result []RowWithPredicates
	for _, line := range strings.Split(tree.String(), "\n") {
		if line == "" {
			continue
		}

		split := strings.SplitN(line, "\t", 2)
		branchText, protojsonText := split[0], split[1]

		var link spanner.PlanNode_ChildLink
		if err := json.Unmarshal([]byte(protojsonText), &link); err != nil {
			return nil, fmt.Errorf("unexpected JSON unmarshal error, tree line = %q", line)
		}

		node := qp.GetNodeByIndex(link.GetChildIndex())
		displayName := nodeTitle(node)

		var text string
		if link.GetType() != "" {
			text = fmt.Sprintf("[%s] %s", link.GetType(), displayName)
		} else {
			text = displayName
		}

		var predicates []string
		for _, cl := range node.GetChildLinks() {
			if !qp.IsPredicate(cl) {
				continue
			}
			predicates = append(predicates, fmt.Sprintf("%s: %s", cl.GetType(), qp.GetNodeByChildLink(cl).GetShortRepresentation().GetDescription()))
		}

		var executionStats stats.ExecutionStats
		if err := jsonRoundtrip(node.GetExecutionStats(), &executionStats, true); err != nil {
			return nil, err
		}

		result = append(result, RowWithPredicates{
			ID:             node.GetIndex(),
			Predicates:     predicates,
			TreePart:       branchText,
			NodeText:       text,
			ExecutionStats: executionStats,
		})
	}
	return result, nil
}

func renderTree(qp *queryplan.QueryPlan, tree treeprint.Tree, link *spanner.PlanNode_ChildLink) {
	if !qp.IsVisible(link) {
		return
	}

	b, _ := json.Marshal(link)
	// Prefixed by tab to ease to split
	str := "\t" + string(b)

	node := qp.GetNodeByChildLink(link)
	visibleChildLinks := qp.VisibleChildLinks(node)

	var branch treeprint.Tree

	if node.GetIndex() == 0 {
		tree.SetValue(str)
		branch = tree
	} else if len(visibleChildLinks) > 0 {
		branch = tree.AddBranch(str)
	} else {
		branch = tree.AddNode(str)
	}

	for _, child := range visibleChildLinks {
		renderTree(qp, branch, child)
	}
}

func nodeTitle(node *spanner.PlanNode) string {
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
				strings.TrimSuffix(metadataFields["scan_target"].GetStringValue(), "Scan"),
				v.GetStringValue()))
		default:
			fields = append(fields, fmt.Sprintf("%s: %s", k, v.GetStringValue()))
		}
	}

	sort.Strings(fields)

	return joinIfNotEmpty(" ", operator, encloseIfNotEmpty("(", strings.Join(fields, ", "), ")"))
}

func jsonRoundtrip(input interface{}, output interface{}, disallowUnknownFields bool) error {
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	if disallowUnknownFields {
		dec.DisallowUnknownFields()
	}
	err = dec.Decode(output)
	if err != nil {
		return err
	}
	return nil
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
