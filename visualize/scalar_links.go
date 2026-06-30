package visualize

import (
	"bytes"
	"fmt"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spannerplan"
	"github.com/apstndb/spannerplan/plantree"

)

func buildScalarLinkRowIndex(qp *spannerplan.QueryPlan, param BuildOptions) (map[int32]plantree.RowWithPredicates, error) {
	if !needsScalarLinkRows(param) {
		return nil, nil
	}
	return buildPlanRowIndex(qp)
}

func needsScalarLinkRows(param BuildOptions) bool {
	return param.SerializeResult || param.NonVariableScalar || param.VariableScalar
}

func buildPlanRowIndex(qp *spannerplan.QueryPlan) (map[int32]plantree.RowWithPredicates, error) {
	rows, err := plantree.ProcessPlan(qp, plantree.WithQueryPlanOptions(spannerplan.HideMetadata()))
	if err != nil {
		return nil, err
	}

	rowsByID := make(map[int32]plantree.RowWithPredicates, len(rows))
	for _, row := range rows {
		rowsByID[row.ID] = row
	}
	return rowsByID, nil
}

func attachPlanRow(node *TreeNode, rowsByID map[int32]plantree.RowWithPredicates) {
	if node == nil || node.planNode == nil || rowsByID == nil {
		return
	}
	if row, ok := rowsByID[node.planNode.GetIndex()]; ok {
		rowCopy := row
		node.planRow = &rowCopy
	}
}

func filterScalarChildLinks(links []plantree.ScalarChildLink, variableOnly bool) []plantree.ScalarChildLink {
	var result []plantree.ScalarChildLink
	for _, link := range links {
		hasVariable := link.Variable != ""
		if variableOnly && !hasVariable {
			continue
		}
		if !variableOnly && hasVariable {
			continue
		}
		result = append(result, link)
	}
	return result
}

func formatScalarChildLinks(links []plantree.ScalarChildLink) string {
	var buf bytes.Buffer
	typeOrder := make([]string, 0)
	groups := make(map[string][]plantree.ScalarChildLink)

	for _, link := range links {
		if link.Variable == "" && link.Type == "" {
			continue
		}
		if _, ok := groups[link.Type]; !ok {
			typeOrder = append(typeOrder, link.Type)
		}
		groups[link.Type] = append(groups[link.Type], link)
	}

	for _, typ := range typeOrder {
		entries := groups[typ]
		var prefix string
		if typ != "" && typ != "Value" {
			if len(entries) == 1 {
				prefix = fmt.Sprintf("%s: ", typ)
			} else {
				prefix = "  "
				fmt.Fprintf(&buf, "%s:\n", typ)
			}
		}
		for _, link := range entries {
			if link.Variable == "" {
				fmt.Fprintf(&buf, "%s%s\n", prefix, link.Description)
			} else {
				fmt.Fprintf(&buf, "%s$%s:=%s\n", prefix, link.Variable, link.Description)
			}
		}
	}
	return buf.String()
}

func serializeResultScalarLinks(links []plantree.ScalarChildLink) []plantree.ScalarChildLink {
	var result []plantree.ScalarChildLink
	for _, link := range links {
		if link.Variable == "" && link.Type == "" {
			result = append(result, link)
		}
	}
	return result
}

func formatSerializeResultFromLinks(rowType *sppb.StructType, links []plantree.ScalarChildLink) string {
	var result bytes.Buffer
	for i, link := range serializeResultScalarLinks(links) {
		if rowType == nil || i >= len(rowType.GetFields()) {
			continue
		}
		name := rowType.GetFields()[i].GetName()
		if name == "" {
			name = fmt.Sprintf("no_name<%d>", i)
		}
		fmt.Fprintf(&result, "Result.%s:%s\n", name, link.Description)
	}
	return result.String()
}
