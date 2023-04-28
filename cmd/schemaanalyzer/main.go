package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/apstndb/spannerplanviz/internal/lox"
	"github.com/apstndb/spannerplanviz/internal/schema"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalln(err)
	}
}

type KeySpec struct {
	StoredColumns []string
	KeyColumns    []*schema.InformationSchemaIndexColumn
}

type TableSpec struct {
	PrimaryKey      *KeySpec
	ParentTableName string
	SecondaryKeys   map[string]*KeySpec
}

func run(ctx context.Context) error {
	schemaFile := flag.String("schema-file", "", "")
	flag.Parse()

	var is schema.InformationSchema
	{
		b, err := os.ReadFile(*schemaFile)
		if err != nil {
			return err
		}
		err = json.Unmarshal(b, &is)
		if err != nil {
			return err
		}
	}

	columnsByTable := schema.BuildColumnsByTableMap(is.Columns)

	tableMap := schema.BuildTableMap(is.Tables)
	indexMap := schema.BuildIndexMap(is.Indexes)

	tableKeys := buildTableSpecs(is, tableMap)
	for tableName, t := range tableKeys {
		pk := t.PrimaryKey
		notExistsInCurrentPKPred := lox.Not(lox.SliceToPredicateBy(pk.KeyColumns, indexColumnToColumnName))

		tableColumnNames := lo.Map(columnsByTable[tableName], func(item *schema.InformationSchemaColumn, _ int) string {
			return item.ColumnName
		})
		columnNamesNotInPK := lox.FilterWithoutIndex(tableColumnNames, notExistsInCurrentPKPred)

		fmt.Printf("%v PRIMARY KEY (%v)%v\n",
			tableName,
			renderKey(tableKeys, lo.FromPtr(tableMap[tableName].ParentTableName), pk.KeyColumns),
			lox.IfOrEmpty(len(columnNamesNotInPK) > 0,
				fmt.Sprintf(` STORING (%v)`, strings.Join(columnNamesNotInPK, ", "))))

		for indexName, index := range t.SecondaryKeys {
			notExistsInCurrentKey := lox.Not(lox.SliceToPredicateBy(index.KeyColumns, indexColumnToColumnName))

			columnNamesNotStoring := lo.Filter(columnNamesNotInPK, func(columnName string, _ int) bool {
				return !lo.Contains(index.StoredColumns, columnName) && notExistsInCurrentKey(columnName)
			})

			pkPart := lox.FilterWithoutIndex(pk.KeyColumns, lox.Compose(notExistsInCurrentKey, indexColumnToColumnName))

			implicitPKPartStrOpt := lox.IfOrEmpty(len(pkPart) > 0,
				fmt.Sprintf("[, %v]", renderKeySpec(pkPart)))

			isIndex := indexMap[indexName]
			keyPartStr := renderKey(tableKeys, isIndex.ParentTableName, index.KeyColumns)
			fmt.Printf("  %v ON %v (%v%v) %v\n",
				indexName,
				tableName,
				keyPartStr,
				implicitPKPartStrOpt,
				strings.Join(lo.WithoutEmpty([]string{
					lox.IfOrEmpty(len(index.StoredColumns) > 0,
						fmt.Sprintf(`STORING (%v)`, strings.Join(index.StoredColumns, ", "))),
					lox.IfOrEmpty(len(columnNamesNotStoring) > 0,
						fmt.Sprintf(`NOT STORING (%v)`, strings.Join(columnNamesNotStoring, ", "))),
					lox.IfOrEmpty(isIndex.IsUnique, "UNIQUE"),
					lox.IfOrEmpty(isIndex.IsNullFiltered, "NULL_FILTERED"),
				}), " ",
				),
			)
		}
	}
	return nil
}

func buildTableSpecs(is schema.InformationSchema, tableMap map[string]*schema.InformationSchemaTable) map[string]*TableSpec {
	keyColumnsInNormalTable := lo.Filter(is.IndexColumns, func(item *schema.InformationSchemaIndexColumn, index int) bool {
		return item.TableSchema == ""
	})
	indexColumnByTableName := lo.GroupBy(keyColumnsInNormalTable, func(item *schema.InformationSchemaIndexColumn) string {
		return item.TableName
	})
	return lo.MapValues(indexColumnByTableName, func(indexColumnsInTable []*schema.InformationSchemaIndexColumn, tableName string) *TableSpec {
		indexColumnByIndexName := lo.GroupBy(indexColumnsInTable, indexColumnToIndexName)
		keySpecsByIndexName := lo.MapValues(indexColumnByIndexName, func(indexColumnsInIndex []*schema.InformationSchemaIndexColumn, _ string) *KeySpec {
			storedColumns := lox.MapWithoutIndex(
				lox.OnlyEmptyBy(indexColumnsInIndex, indexColumnToOrdinalPosition),
				indexColumnToColumnName)
			slices.Sort(storedColumns)

			keyColumns :=
				lox.WithoutEmptyBy(indexColumnsInIndex, indexColumnToOrdinalPosition)
			lox.SortBy(keyColumns, lox.Compose[*schema.InformationSchemaIndexColumn, *int64, int64](lo.FromPtr[int64], indexColumnToOrdinalPosition))

			return &KeySpec{
				StoredColumns: storedColumns,
				KeyColumns:    keyColumns,
			}
		})
		return &TableSpec{
			PrimaryKey:      keySpecsByIndexName["PRIMARY_KEY"],
			ParentTableName: lo.FromPtr(tableMap[tableName].ParentTableName),
			SecondaryKeys:   lo.OmitByKeys(keySpecsByIndexName, []string{"PRIMARY_KEY"}),
		}
	})

	/*
		tableKeys := make(map[string]*TableSpec)

			for _, indexColumn := range is.IndexColumns {
			if indexColumn.TableSchema != "" {
				continue
			}
			tableSpec, ok := tableKeys[indexColumn.TableName]
			if !ok {
				tableSpec = &TableSpec{
					SecondaryKeys:   make(map[string]*KeySpec),
					ParentTableName: lo.FromPtr(tableMap[indexColumn.TableName].ParentTableName),
				}
				tableKeys[indexColumn.TableName] = tableSpec
			}

			keySpec, ok := tableSpec.SecondaryKeys[indexColumn.IndexName]
			if !ok {
				keySpec = &KeySpec{}
				tableSpec.SecondaryKeys[indexColumn.IndexName] = keySpec
			}

			if indexColumn.OrdinalPosition != nil {
				keySpecElem := indexColumn
				keySpec.KeyColumns = append(keySpec.KeyColumns, keySpecElem)
			} else {
				keySpec.StoredColumns = append(keySpec.StoredColumns, indexColumn.ColumnName)
			}
		}

		for _, t := range tableKeys {
			for _, k := range t.SecondaryKeys {
				slices.Sort(k.StoredColumns)
				slices.SortFunc(k.KeyColumns, func(a, b *schema.InformationSchemaIndexColumn) bool {
					return lo.FromPtr(a.OrdinalPosition) < lo.FromPtr(b.OrdinalPosition)
				})
			}
			t.PrimaryKey = t.SecondaryKeys["PRIMARY_KEY"]
			delete(t.SecondaryKeys, "PRIMARY_KEY")
		}
		return tableKeys
	*/
}

func renderKey(tableSpecMap map[string]*TableSpec, parentTableName string, columns []*schema.InformationSchemaIndexColumn) string {
	if parentTableName == "" {
		return renderKeySpec(columns)
	}

	parentTable := tableSpecMap[parentTableName]
	parentKeyColumns := parentTable.PrimaryKey.KeyColumns
	return strings.Join([]string{
		fmt.Sprintf("%v(%v)", parentTableName, renderKey(tableSpecMap, parentTable.ParentTableName, columns[:len(parentKeyColumns)])),
		renderKeySpec(columns[len(parentKeyColumns):]),
	}, ", ")
}

func renderKeySpec(ks []*schema.InformationSchemaIndexColumn) string {
	return strings.Join(lo.Map(ks, func(item *schema.InformationSchemaIndexColumn, _ int) string {
		if lo.FromPtr(item.ColumnOrdering) == "DESC" {
			return fmt.Sprintf("%v DESC", item.ColumnName)
		}
		return item.ColumnName
	}), ", ")
}

func indexColumnToColumnName(index *schema.InformationSchemaIndexColumn) string {
	return index.ColumnName
}

func indexColumnToIndexName(index *schema.InformationSchemaIndexColumn) string {
	return index.IndexName
}

func indexColumnToOrdinalPosition(index *schema.InformationSchemaIndexColumn) *int64 {
	return index.OrdinalPosition
}
