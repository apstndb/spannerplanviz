package schema

import (
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
)

func BuildColumnsByTableMap(columns []*InformationSchemaColumn) map[string][]*InformationSchemaColumn {
	filteredColumns := lo.Filter(columns, func(item *InformationSchemaColumn, _ int) bool {
		return item.TableSchema == ""
	})

	result := lo.GroupBy(filteredColumns, func(item *InformationSchemaColumn) string {
		return item.TableName
	})

	for _, columns := range result {
		slices.SortFunc(columns, func(a, b *InformationSchemaColumn) bool {
			return a.OrdinalPosition < b.OrdinalPosition
		})
	}

	return result
}

func BuildIndexMap(indexes []*InformationSchemaIndex) map[string]*InformationSchemaIndex {
	IndexesNotSystem := lo.Filter(indexes, func(item *InformationSchemaIndex, _ int) bool {
		return item.TableSchema == "" && item.IndexName != "PRIMARY_KEY"
	})
	return lo.KeyBy(IndexesNotSystem, func(item *InformationSchemaIndex) string {
		return item.IndexName
	})
}

func BuildTableMap(tables []*InformationSchemaTable) map[string]*InformationSchemaTable {
	return lo.KeyBy(lo.Filter(tables, func(item *InformationSchemaTable, _ int) bool {
		return item.TableSchema == ""
	}), func(item *InformationSchemaTable) string {
		return item.TableName
	})
}
