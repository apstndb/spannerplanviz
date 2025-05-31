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
	"github.com/mattn/go-runewidth"
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
	treeLines := strings.Split(r.TreePart, "\n")
	nodeLines := strings.Split(r.NodeText, "\n")
	var sb strings.Builder
	for i, line := range nodeLines {
		if len(treeLines) > i {
			sb.WriteString(strings.TrimSuffix(treeLines[i], "\n"))
		}
		sb.WriteString(line)
		sb.WriteRune('\n')
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

func (r RowWithPredicates) FormatID() string {
	return lox.IfOrEmpty(len(r.Predicates) != 0, "*") + strconv.Itoa(int(r.ID))
}

type options struct {
	disallowUnknownStats bool
	queryplanOptions     []queryplan.Option
	treeprintOptions     []treeprint.Option
	compact              bool
	indentSize           int
	wrapWidth            *int
	wrapper              *runewidth.Condition
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

func WithWrapWidth(width int) Option {
	return func(o *options) {
		o.wrapWidth = &width
	}
}

func WithWrapper(wrapper *runewidth.Condition) Option {
	return func(o *options) {
		o.wrapper = wrapper
	}
}

// EnableCompact enables compact node title mode.
func EnableCompact() Option {
	return func(o *options) {
		o.indentSize = 0
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
		indentSize: 2,
		// default values to be override
		treeprintOptions: []treeprint.Option{
			treeprint.WithEdgeTypeLink("|"),
			treeprint.WithEdgeTypeMid("+-"),
			treeprint.WithEdgeTypeEnd("+-"),
			treeprint.WithIndentSize(2),
			treeprint.WithEdgeSeparator(" "),
		},
	}
	for _, opt := range opts {
		opt(&o)
	}

	if o.wrapper == nil {
		o.wrapper = runewidth.NewCondition()
	}

	tree := treeprint.New()

	if err := buildTree(qp, tree, nil, 0, &o); err != nil {
		return nil, fmt.Errorf("failed on buildTree, err: %w", err)
	}

	var result []RowWithPredicates
	for _, line := range strings.Split(tree.StringWithOptions(o.treeprintOptions...), "\000\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		split := strings.Split(line, "\t")

		if len(split) != 3 {
			// Handle the case where the separator is not found
			return nil, fmt.Errorf("unexpected format, tree line = %q", line)
		}

		branchText := strings.TrimPrefix(split[0], "\n")
		nodeTextJson := split[1]
		var nodeText string
		if err := json.Unmarshal([]byte(nodeTextJson), &nodeText); err != nil {
			return nil, fmt.Errorf("unexpected JSON unmarshal error, tree line = %q", line)
		}

		protojsonText := split[2]

		var link sppb.PlanNode_ChildLink
		if err := json.Unmarshal([]byte(protojsonText), &link); err != nil {
			return nil, fmt.Errorf("unexpected JSON unmarshal error, tree line = %q", line)
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
			NodeText:       nodeText,
			ExecutionStats: executionStats,
		})
	}
	return result, nil
}

func getLinkType(qp *queryplan.QueryPlan, link *sppb.PlanNode_ChildLink) string {
	var linkType string
	if link.GetType() != "" {
		linkType = link.GetType()

		// Workaround to treat the first child of Apply as Input.
		// This is necessary because it is more consistent with the official query plan operator docs.
		// Note: Apply variants are Cross Apply, Anti Semi Apply, Semi Apply, Outer Apply, and their Distributed variants.
	} else if parent := qp.GetParentNodeByChildLink(link); parent != nil &&
		strings.HasSuffix(parent.GetDisplayName(), "Apply") &&
		len(parent.GetChildLinks()) > 0 && parent.GetChildLinks()[0].GetChildIndex() == link.GetChildIndex() {
		linkType = "Input"
	}
	return linkType
}

func buildTree(qp *queryplan.QueryPlan, tree treeprint.Tree, link *sppb.PlanNode_ChildLink, level int, opts *options) error {
	if !qp.IsVisible(link) {
		// empty tree
		return nil
	}

	b, err := json.Marshal(link)
	if err != nil {
		return fmt.Errorf("unexpected error: link can't be marshalled to JSON: %w", err)
	}

	node := qp.GetNodeByChildLink(link)
	visibleChildLinks := qp.VisibleChildLinks(node)
	linkType := getLinkType(qp, link)
	sep := lo.Ternary(!opts.compact, " ", "")

	// node := qp.GetNodeByIndex(link.GetChildIndex())
	nodeText := lox.IfOrEmpty(linkType != "", "["+linkType+"]"+sep) + queryplan.NodeTitle(node, opts.queryplanOptions...)
	if opts.wrapWidth != nil {
		nodeText = opts.wrapper.Wrap(nodeText, *opts.wrapWidth-level*(opts.indentSize+1)-opts.wrapper.StringWidth(sep))
	}

	newlineCount := strings.Count(nodeText, "\n")
	nodeTextJson, err := json.Marshal(nodeText)
	if err != nil {
		return fmt.Errorf("unexpected error: nodeText can't be marshalled to JSON: %w", err)
	}

	// Prefixed by tab and terminated by null character to ease to split
	str := strings.Repeat("\n", newlineCount) + "\t" + string(nodeTextJson) + "\t" + string(b) + "\000"

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
		if err := buildTree(qp, branch, child, level+1, opts); err != nil {
			return fmt.Errorf("unexpected error: buildTree failed on link %v, err: %w", link, err)
		}
	}
	return nil
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
