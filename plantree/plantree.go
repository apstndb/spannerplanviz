package plantree

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/lox"
	"github.com/apstndb/treeprint"
	"github.com/samber/lo"

	"github.com/apstndb/spannerplanviz/queryplan"
	"github.com/apstndb/spannerplanviz/stats"
)

type RowWithPredicates struct {
	ID             int32
	TreePart       string
	NodeText       string
	Predicates     []string
	Keys           map[string][]string
	ExecutionStats stats.ExecutionStats
	ChildLinks     map[string][]*queryplan.ResolvedChildLink
}

func (r RowWithPredicates) Text() string {
	return r.TreePart + r.NodeText
}

func (r RowWithPredicates) FormatID() string {
	return lox.IfOrEmpty(len(r.Predicates) != 0, "*") + strconv.Itoa(int(r.ID))
}

type options struct {
	disallowUnknownStats bool
	queryplanOptions     []queryplan.Option
	treeprintOptions     []treeprint.Option
	compact              bool
}

type Option func(*options)

func DisallowUnknownStats() Option {
	return func(o *options) {
		o.disallowUnknownStats = true
	}
}

func WithQueryPlanOptions(opts ...queryplan.Option) Option {
	return func(o *options) {
		o.queryplanOptions = append(o.queryplanOptions, opts...)
	}
}

func WithTreeprintOptions(opts ...treeprint.Option) Option {
	return func(o *options) {
		o.treeprintOptions = append(o.treeprintOptions, opts...)
	}
}

func EnableCompact() Option {
	return func(o *options) {
		o.compact = true
		o.queryplanOptions = append(o.queryplanOptions, queryplan.EnableCompact())
		o.treeprintOptions = append(
			o.treeprintOptions,
			treeprint.WithEdgeTypeLink("|"),
			treeprint.WithEdgeTypeMid("+"),
			treeprint.WithEdgeTypeEnd("+"),
			treeprint.WithIndentSize(0),
			treeprint.WithEdgeSeparator(""),
		)
	}
}

func ProcessPlan(qp *queryplan.QueryPlan, opts ...Option) (rows []RowWithPredicates, err error) {
	o := options{
		// default values to be override
		treeprintOptions: []treeprint.Option{
			treeprint.WithEdgeTypeLink("|"),
			treeprint.WithEdgeTypeMid("+-"),
			treeprint.WithEdgeTypeEnd("+-"),
			treeprint.WithIndentSize(2),
		},
	}
	for _, opt := range opts {
		opt(&o)
	}

	sep := lo.Ternary(!o.compact, " ", "")
	tree := treeprint.New()

	renderTree(qp, tree, nil)
	var result []RowWithPredicates
	for _, line := range strings.Split(tree.StringWithOptions(o.treeprintOptions...), "\n") {
		if line == "" {
			continue
		}

		branchText, protojsonText, found := strings.Cut(line, "\t")
		if !found {
			// Handle the case where the separator is not found
			return nil, fmt.Errorf("unexpected format, tree line = %q", line)
		}

		var link sppb.PlanNode_ChildLink
		if err := json.Unmarshal([]byte(protojsonText), &link); err != nil {
			return nil, fmt.Errorf("unexpected JSON unmarshal error, tree line = %q", line)
		}

		var linkType string
		if link.GetType() != "" {
			linkType = link.GetType()

			// Workaround to treat the first child of Apply as Input.
			// This is necessary because it is more consistent with the official query plan operator docs.
			// Note: Apply variants are Cross Apply, Anti Semi Apply, Semi Apply, Outer Apply, and their Distributed variants.
		} else if parent := qp.GetParentNodeByChildLink(&link); parent != nil &&
			strings.HasSuffix(parent.GetDisplayName(), "Apply") &&
			len(parent.GetChildLinks()) > 0 && parent.GetChildLinks()[0].GetChildIndex() == link.GetChildIndex() {
			linkType = "Input"
		}

		node := qp.GetNodeByIndex(link.GetChildIndex())

		var predicates []string
		for _, cl := range node.GetChildLinks() {
			if !qp.IsPredicate(cl) {
				continue
			}

			predicates = append(predicates, fmt.Sprintf("%s: %s",
				cl.GetType(),
				qp.GetNodeByChildLink(cl).GetShortRepresentation().GetDescription()))
		}

		resolvedChildLinks := lox.MapWithoutIndex(node.GetChildLinks(), qp.ResolveChildLink)

		scalarChildLinks := lox.FilterWithoutIndex(resolvedChildLinks, func(item *queryplan.ResolvedChildLink) bool {
			return item.Child.GetKind() == sppb.PlanNode_SCALAR
		})

		childLinks := lo.GroupBy(scalarChildLinks, func(item *queryplan.ResolvedChildLink) string {
			return item.ChildLink.GetType()
		})

		var executionStats stats.ExecutionStats
		if err := jsonRoundtrip(node.GetExecutionStats(), &executionStats, o.disallowUnknownStats); err != nil {
			return nil, err
		}

		result = append(result, RowWithPredicates{
			ID:             node.GetIndex(),
			Predicates:     predicates,
			ChildLinks:     childLinks,
			TreePart:       branchText,
			NodeText:       lox.IfOrEmpty(linkType != "", "["+linkType+"]"+sep) + queryplan.NodeTitle(node, o.queryplanOptions...),
			ExecutionStats: executionStats,
		})
	}
	return result, nil
}

func renderTree(qp *queryplan.QueryPlan, tree treeprint.Tree, link *sppb.PlanNode_ChildLink) {
	if !qp.IsVisible(link) {
		return
	}

	b, _ := json.Marshal(link)
	// Prefixed by tab to ease to split
	str := "\t" + string(b)

	node := qp.GetNodeByChildLink(link)
	visibleChildLinks := qp.VisibleChildLinks(node)

	var branch treeprint.Tree

	switch {
	case node.GetIndex() == 0:
		tree.SetValue(str)
		branch = tree
	case len(visibleChildLinks) > 0:
		branch = tree.AddBranch(str)
	default:
		branch = tree.AddNode(str)
	}

	for _, child := range visibleChildLinks {
		renderTree(qp, branch, child)
	}
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
