package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spannerplanviz/internal/schema"
	"github.com/apstndb/spannerplanviz/queryplan"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalln(err)
	}
}

const jsonSnippetLen = 140

type KeySpecElem struct {
	ColumnName      string
	IsDesc          bool
	OrdinalPosition int64
}
type KeySpec struct {
	StoredColumns []string
	Keys          []*KeySpecElem
}
type TableSpec struct {
	PrimaryKey    *KeySpec
	SecondaryKeys map[string]*KeySpec
}

func run(ctx context.Context) error {
	schemaFile := flag.String("schema-file", "", "")
	flag.Parse()

	var schema schema.InformationSchema
	{
		schemaB, err := os.ReadFile(*schemaFile)
		if err != nil {
			return err
		}
		err = json.Unmarshal(schemaB, &schema)
		if err != nil {
			return err
		}
	}
	tableByIndex, indexesByTable := buildIndexMaps(&schema)
	_ = indexesByTable
	columnsByTable := buildColumnsByTableMap(schema.Columns)

	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	stats, _, err := queryplan.ExtractQueryPlan(b)
	if err != nil {
		var collapsedStr string
		if len(b) > jsonSnippetLen {
			collapsedStr = "(collapsed)"
		}
		return fmt.Errorf("invalid input at protoyaml.Unmarshal:\nerror: %w\ninput: %.*s%s", err, jsonSnippetLen, strings.TrimSpace(string(b)), collapsedStr)
	}

	qp := queryplan.New(stats.GetQueryPlan().GetPlanNodes())

	variableToExp := buildVariableToNodeMap(qp)

	scanMap, err := buildScanMap(qp, tableByIndex)
	if err != nil {
		return err
	}

	pathMap := buildPathMap(qp)
	tableKeys := make(map[string]*TableSpec)

	for _, indexColumn := range schema.IndexColumns {
		if indexColumn.TableSchema != "" {
			continue
		}
		tableSpec, ok := tableKeys[indexColumn.TableName]
		if !ok {
			tableSpec = &TableSpec{
				SecondaryKeys: make(map[string]*KeySpec),
			}
			tableKeys[indexColumn.TableName] = tableSpec
		}

		keySpec, ok := tableSpec.SecondaryKeys[indexColumn.IndexName]
		if !ok {
			keySpec = &KeySpec{}
			tableSpec.SecondaryKeys[indexColumn.IndexName] = keySpec
		}

		if indexColumn.OrdinalPosition != nil {
			keySpecElem := KeySpecElem{
				IsDesc:          indexColumn.ColumnOrdering != nil && *indexColumn.ColumnOrdering == "DESC",
				ColumnName:      indexColumn.ColumnName,
				OrdinalPosition: *indexColumn.OrdinalPosition,
			}
			keySpec.Keys = append(keySpec.Keys, &keySpecElem)
		} else {
			keySpec.StoredColumns = append(keySpec.StoredColumns, indexColumn.ColumnName)
		}
	}
	for _, t := range tableKeys {
		for _, k := range t.SecondaryKeys {
			slices.Sort(k.StoredColumns)
			slices.SortFunc(k.Keys, func(a, b *KeySpecElem) bool {
				return a.OrdinalPosition < b.OrdinalPosition
			})
		}
		t.PrimaryKey = t.SecondaryKeys["PRIMARY_KEY"]
		delete(t.SecondaryKeys, "PRIMARY_KEY")
	}
	for tn, t := range tableKeys {
		pk := t.PrimaryKey
		var columnNames []string
		for _, column := range columnsByTable[tn] {
			_, found := lo.Find(pk.Keys, func(item *KeySpecElem) bool {
				return item.ColumnName == column.ColumnName
			})
			if !found {
				columnNames = append(columnNames, column.ColumnName)
			}
		}
		fmt.Printf("%v PRIMARY KEY (%v)\n", tn, renderKeySpec(pk.Keys))

		for kn, k := range t.SecondaryKeys {
			var stringClauseOpt string
			if k.StoredColumns != nil {
				stringClauseOpt = fmt.Sprintf(` STORING (%v)`, strings.Join(k.StoredColumns, ", "))
			}
			var pkPart []*KeySpecElem
			for _, pkElem := range pk.Keys {
				_, found := lo.Find(k.Keys, func(item *KeySpecElem) bool {
					return item.ColumnName == pkElem.ColumnName
				})
				if !found {
					pkPart = append(pkPart, pkElem)
				}
			}

			var pkPartOpt string
			if len(pkPart) > 0 {
				pkPartOpt = fmt.Sprintf("[, %v]", renderKeySpec(pkPart))
			}
			fmt.Printf("  %v ON %v (%v%v)%v\n", kn, tn,
				renderKeySpec(k.Keys), pkPartOpt, stringClauseOpt)
		}
	}

	if len(scanMap) > 0 {
		fmt.Println("Table Usages")
		for table, nodes := range scanMap {
			fmt.Printf("  %v %v\n", table, lo.Map(nodes, func(item *spannerpb.PlanNode, index int) string {
				return fmt.Sprintf("%v:%v(%v)", item.GetIndex(), strings.TrimSuffix(item.GetMetadata().GetFields()["scan_type"].GetStringValue(), "Scan")+" Scan", item.GetMetadata().AsMap()["scan_target"])
			}))
			switch len(nodes) {
			case 1:
			case 2:
				firstNode, secondNode := nodes[0], nodes[1]
				if firstNode.GetMetadata().AsMap()["scan_type"] != secondNode.GetMetadata().AsMap()["scan_type"] {
					first, second := pathMap[nodes[0].GetIndex()], pathMap[nodes[1].GetIndex()]
					ancestors := lca(first, second)
					fmt.Printf("    Joined at %v?\n", ancestors[len(ancestors)-1])
				}
			default:
				fmt.Printf("    Too many apearance to analyze joins\n")
			}
		}
	}

	for _, row := range qp.PlanNodes() {
		var msgs []string
		switch {
		case row.GetDisplayName() == "Filter":
			msgs = append(msgs, "Expensive operator Filter can't utilize index: Can't you use Filter Scan with Seek Condition?")
		case strings.Contains(row.GetDisplayName(), "Hash"):
			msgs = append(msgs, fmt.Sprintf("Expensive execution %s: Can't you modify to use Cross Apply or Merge Join?", row.GetDisplayName()))
		case strings.Contains(row.GetDisplayName(), "Minor Sort"):
			var order []string
			for _, cl := range row.GetChildLinks() {
				if cl.GetType() == "MajorKey" {
					order = append(order, descToKeyElem(variableToExp, qp.GetNodeByChildLink(cl).GetShortRepresentation().GetDescription()))
				}
			}
			for _, cl := range row.GetChildLinks() {
				if cl.GetType() == "MinorKey" {
					order = append(order, descToKeyElem(variableToExp, qp.GetNodeByChildLink(cl).GetShortRepresentation().GetDescription()))
				}
			}
			msgs = append(msgs, fmt.Sprintf("Expensive operator Minor Sort is cheaper than Sort but it may be not optimal: Can't you create the same ordered index? Order: %v", strings.Join(order, ", ")))
		case strings.Contains(row.GetDisplayName(), "Sort"):
			var order []string
			for _, cl := range row.GetChildLinks() {
				if cl.GetType() == "Key" {
					order = append(order, descToKeyElem(variableToExp, qp.GetNodeByChildLink(cl).GetShortRepresentation().GetDescription()))
				}
			}
			msgs = append(msgs, fmt.Sprintf("Expensive operator Sort: Can't you create the same ordered index? : %v", strings.Join(order, ", ")))
		}
		for _, childLink := range row.GetChildLinks() {
			var msg string
			switch {
			case childLink.GetType() == "Residual Condition":
				msg = "Expensive Residual Condition: Try to translate it to Scan Condition"
			}
			if msg != "" {
				msgs = append(msgs, fmt.Sprintf("%v: %v", childLink.GetType(), msg))
			}
		}
		for k, v := range row.GetMetadata().AsMap() {
			var msg string
			switch {
			case k == "Full scan" && v == "true":
				msg = "Expensive execution full scan: Do you really want full scan?"
			case k == "iterator_type" && v == "Hash":
				var order []string
				for _, cl := range row.GetChildLinks() {
					if cl.GetType() == "Key" {
						order = append(order, descToKeyElem(variableToExp, qp.GetNodeByChildLink(cl).GetShortRepresentation().GetDescription()))
					}
				}
				msg = fmt.Sprintf("Expensive execution Hash %s: Can't you modify to use Stream %s? Key: %v", row.GetDisplayName(), row.GetDisplayName(), strings.Join(order, ", "))
			}
			if msg != "" {
				msgs = append(msgs, msg)
			}
		}
		if len(msgs) > 0 {
			fmt.Printf("%v: %v\n", row.GetIndex(), queryplan.NodeTitle(row))
			for _, msg := range msgs {
				fmt.Printf("    %v\n", msg)
			}
		}
	}
	return nil
}

func buildColumnsByTableMap(columns []*schema.InformationSchemaColumn) map[string][]*schema.InformationSchemaColumn {
	filteredColumns := lo.Filter(columns, func(item *schema.InformationSchemaColumn, _ int) bool {
		return item.TableSchema != ""
	})
	result := lo.GroupBy(filteredColumns, func(item *schema.InformationSchemaColumn) string {
		return item.TableName
	})
	for _, columns := range result {
		slices.SortFunc(columns, func(a, b *schema.InformationSchemaColumn) bool {
			return a.OrdinalPosition < b.OrdinalPosition
		})
	}
	return result
}

func renderKeySpec(ks []*KeySpecElem) string {
	return strings.Join(lo.Map(ks, func(item *KeySpecElem, _ int) string {
		if item.IsDesc {
			return fmt.Sprintf("%v DESC", item.ColumnName)
		}
		return item.ColumnName
	}), ", ")
}

func buildVariableToNodeMap(qp *queryplan.QueryPlan) map[string]*spannerpb.PlanNode {
	variableToExp := make(map[string]*spannerpb.PlanNode)
	for _, row := range qp.PlanNodes() {
		for _, cl := range row.GetChildLinks() {
			if cl.GetVariable() != "" {
				variableToExp[cl.GetVariable()] = qp.GetNodeByChildLink(cl)
			}
		}
	}
	return variableToExp
}

func buildScanMap(qp *queryplan.QueryPlan, tableByIndex map[string]*schema.InformationSchemaTable) (map[string][]*spannerpb.PlanNode, error) {
	scanMap := make(map[string][]*spannerpb.PlanNode)
	for _, node := range qp.PlanNodes() {
		if node.GetDisplayName() != "Scan" {
			continue
		}
		switch node.GetMetadata().AsMap()["scan_type"] {
		case "IndexScan":
			scanTarget := node.GetMetadata().GetFields()["scan_target"].GetStringValue()
			if table, ok := tableByIndex[scanTarget]; ok {
				scanMap[table.TableName] = append(scanMap[scanTarget], node)
			} else {
				return nil, fmt.Errorf("Unknown index name: %v\n", scanTarget)
			}
		case "TableScan":
			scanTarget := node.GetMetadata().GetFields()["scan_target"].GetStringValue()
			scanMap[scanTarget] = append(scanMap[scanTarget], node)
		}
	}
	return scanMap, nil
}

func lca(first []int32, last []int32) []int32 {
	var result []int32
	for i := 0; i < len(first) && i < len(last) && first[i] == last[i]; i++ {
		result = append(result, first[i])
	}
	return result
}

func lookupVar(varToExp map[string]*spannerpb.PlanNode, ref string) string {
	if !strings.HasPrefix(ref, "$") {
		return ref
	}

	if v, ok := varToExp[strings.TrimPrefix(ref, "$")]; ok {
		return lookupVar(varToExp, v.GetShortRepresentation().GetDescription())
	}

	return ref
}

func descToKeyElem(varToExp map[string]*spannerpb.PlanNode, desc string) string {
	first, last, found := strings.Cut(desc, " ")
	keyElem := lookupVar(varToExp, first)
	if found {
		keyElem = keyElem + " " + strings.TrimSuffix(strings.TrimPrefix(last, "("), ")")
	}
	return keyElem
}

func buildTableMap(tables []*schema.InformationSchemaTable) map[string]*schema.InformationSchemaTable {
	return lo.KeyBy(lo.Filter(tables, func(item *schema.InformationSchemaTable, index int) bool {
		return item.TableSchema == ""
	}), func(item *schema.InformationSchemaTable) string {
		return item.TableName
	})
}

func buildIndexMaps(is *schema.InformationSchema) (tableByIndex map[string]*schema.InformationSchemaTable, indexesByTable map[string][]*schema.InformationSchemaIndex) {
	tableMap := buildTableMap(is.Tables)

	tableByIndex = make(map[string]*schema.InformationSchemaTable)
	indexesByTable = make(map[string][]*schema.InformationSchemaIndex)

	for _, idx := range is.Indexes {
		if idx.TableCatalog != "" || idx.TableSchema != "" {
			continue
		}
		tableByIndex[idx.IndexName] = tableMap[idx.TableName]
		indexesByTable[idx.TableName] = append(indexesByTable[idx.TableName], idx)
	}
	return tableByIndex, indexesByTable
}

func buildPathMap(qp *queryplan.QueryPlan) map[int32][]int32 {
	node := qp.PlanNodes()[0]
	return buildPathMapInternal(qp, node, nil)
}

func buildPathMapInternal(qp *queryplan.QueryPlan, node *spannerpb.PlanNode, parentPath []int32) map[int32][]int32 {
	result := make(map[int32][]int32)
	currentPath := append(slices.Clone(parentPath), node.GetIndex())
	result[node.GetIndex()] = currentPath
	for _, link := range node.GetChildLinks() {
		for k, v := range buildPathMapInternal(qp, qp.GetNodeByChildLink(link), currentPath) {
			result[k] = v
		}
	}
	return result
}
