package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

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
		schemaB, err := os.ReadFile(*schemaFile)
		if err != nil {
			return err
		}
		err = json.Unmarshal(schemaB, &is)
		if err != nil {
			return err
		}
	}

	columnsByTable := schema.BuildColumnsByTableMap(is.Columns)

	tableMap := schema.BuildTableMap(is.Tables)
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

	indexMap := schema.BuildIndexMap(is.Indexes)

	for tableName, t := range tableKeys {
		pk := t.PrimaryKey
		notExistsInCurrentPKPred := Not(SliceToPredicateBy(pk.KeyColumns, indexColumnToColumnName))

		tableColumnNames := lo.Map(columnsByTable[tableName], func(item *schema.InformationSchemaColumn, _ int) string {
			return item.ColumnName
		})
		columnNamesNotInPK := FilterWithoutIndex(tableColumnNames, notExistsInCurrentPKPred)

		fmt.Printf("%v PRIMARY KEY (%v)%v\n",
			tableName,
			renderKey(tableKeys, lo.FromPtr(tableMap[tableName].ParentTableName), pk.KeyColumns),
			IfOrEmpty(len(columnNamesNotInPK) > 0,
				fmt.Sprintf(` STORING (%v)`, strings.Join(columnNamesNotInPK, ", "))))

		for indexName, index := range t.SecondaryKeys {
			notExistsInCurrentKey := Not(SliceToPredicateBy(index.KeyColumns, indexColumnToColumnName))

			columnNamesNotStoring := lo.Filter(columnNamesNotInPK, func(columnName string, _ int) bool {
				return !lo.Contains(index.StoredColumns, columnName) && notExistsInCurrentKey(columnName)
			})

			pkPart := FilterWithoutIndex(pk.KeyColumns, Compose(notExistsInCurrentKey, indexColumnToColumnName))

			implicitPKPartStrOpt := IfOrEmpty(len(pkPart) > 0,
				fmt.Sprintf("[, %v]", renderKeySpec(pkPart)))

			isIndex := indexMap[indexName]
			keyPartStr := renderKey(tableKeys, isIndex.ParentTableName, index.KeyColumns)
			fmt.Printf("  %v ON %v (%v%v) %v\n",
				indexName,
				tableName,
				keyPartStr,
				implicitPKPartStrOpt,
				strings.Join(lo.WithoutEmpty([]string{
					IfOrEmpty(len(index.StoredColumns) > 0,
						fmt.Sprintf(`STORING (%v)`, strings.Join(index.StoredColumns, ", "))),
					IfOrEmpty(len(columnNamesNotStoring) > 0,
						fmt.Sprintf(`NOT STORING (%v)`, strings.Join(columnNamesNotStoring, ", "))),
					IfOrEmpty(isIndex.IsUnique, "UNIQUE"),
					IfOrEmpty(isIndex.IsNullFiltered, "NULL_FILTERED"),
				}), " ",
				),
			)
		}
	}
	return nil
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

func columnToColumnName(index *schema.InformationSchemaIndexColumn) string {
	return index.ColumnName
}

func MapToPredicate[K comparable, V any](m map[K]V) func(K) bool {
	return func(v K) bool {
		_, ok := m[v]
		return ok
	}
}
func SliceToPredicateBy[K comparable, V any](s []V, f func(V) K) func(K) bool {
	return MapToPredicate(SliceToSetBy(s, f))
}

func SliceToPredicate[V comparable](s []V) func(V) bool {
	return MapToPredicate(SliceToSet(s))
}

func SliceToSet[V comparable](collection []V) map[V]struct{} {
	return SliceToSetBy(collection, Identity[V])
}

func SliceToSetBy[K comparable, V any](collection []V, iteratee func(item V) K) map[K]struct{} {
	return lo.Associate(collection, func(item V) (K, struct{}) {
		return iteratee(item), struct{}{}
	})
}

func Identity[T any](v T) T {
	return v
}

func IfOrEmpty[T any](condition bool, result T) T {
	return lo.Ternary(condition, result, lo.Empty[T]())
}
func IfOrEmptyF[T any](condition bool, f func() T) T {
	return lo.TernaryF(condition, f, lo.Empty[T])
}

func Compose[T1, T2, R any](f func(T2) R, g func(T1) T2) func(T1) R {
	return func(v T1) R {
		return f(g(v))
	}
}

func Not[T any](f func(T) bool) func(T) bool {
	return func(v T) bool {
		return !f(v)
	}
}

func IgnoreIndex[T1, T2, R any](f func(T1) R) func(T1, T2) R {
	return IgnoreSecond[T1, T2, R](f)
}

func IgnoreSecond[T1, T2, R any](f func(T1) R) func(T1, T2) R {
	return func(v T1, _ T2) R {
		return f(v)
	}
}

func FilterWithoutIndex[V any](collection []V, predicate func(item V) bool) []V {
	return lo.Filter(collection, IgnoreSecond[V, int, bool](predicate))
}
func MapWithoutIndex[T, R any](collection []T, iteratee func(item T) R) []R {
	return lo.Map(collection, IgnoreSecond[T, int, R](iteratee))
}
