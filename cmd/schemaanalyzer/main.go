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

	tableKeys := make(map[string]*TableSpec)

	for _, indexColumn := range is.IndexColumns {
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

	indexMap := schema.BuildIndexMap(is.Indexes)
	tableMap := schema.BuildTableMap(is.Tables)

	for tableName, t := range tableKeys {
		pk := t.PrimaryKey
		existsInCurrentPK := SliceToPredicateBy(pk.Keys, func(item *KeySpecElem) string {
			return item.ColumnName
		})

		columnNamesNotInPK := lo.FilterMap(columnsByTable[tableName], func(item *schema.InformationSchemaColumn, _ int) (string, bool) {
			if existsInCurrentPK(item.ColumnName) {
				return "", false
			}
			return item.ColumnName, true
		})

		fmt.Printf("%v PRIMARY KEY (%v)%v\n",
			tableName,
			renderKey(tableMap, tableKeys, lo.FromPtr(tableMap[tableName].ParentTableName), pk.Keys),
			IfOrEmpty(len(columnNamesNotInPK) > 0,
				fmt.Sprintf(` STORING (%v)`, strings.Join(columnNamesNotInPK, ", "))))

		for indexName, index := range t.SecondaryKeys {
			storingClauseStrOpt := IfOrEmpty(len(index.StoredColumns) > 0,
				fmt.Sprintf(` STORING (%v)`, strings.Join(index.StoredColumns, ", ")))

			existsInCurrentKey := SliceToPredicateBy(index.Keys, func(item *KeySpecElem) string {
				return item.ColumnName
			})

			columnNamesNotStoring := lo.Filter(columnNamesNotInPK, func(columnName string, _ int) bool {
				return !lo.Contains(index.StoredColumns, columnName) && !existsInCurrentKey(columnName)
			})

			notStoringClauseStrOpt := IfOrEmpty(len(columnNamesNotStoring) > 0,
				fmt.Sprintf(` NOT STORING (%v)`, strings.Join(columnNamesNotStoring, ", ")))

			pkPart := lo.Filter(pk.Keys, func(item *KeySpecElem, _ int) bool {
				return existsInCurrentKey(item.ColumnName)
			})

			implicitPKPartStrOpt := IfOrEmpty(len(pkPart) > 0,
				fmt.Sprintf("[, %v]", renderKeySpec(pkPart)))

			keyPartStr := renderKey(tableMap, tableKeys, indexMap[indexName].ParentTableName, index.Keys)
			fmt.Printf("  %v ON %v (%v%v)%v%v\n",
				indexName,
				tableName,
				keyPartStr,
				implicitPKPartStrOpt,
				storingClauseStrOpt,
				notStoringClauseStrOpt,
			)
		}
	}
	return nil
}

func renderKey(tableMap map[string]*schema.InformationSchemaTable, tableKeys map[string]*TableSpec, parentTableName string, pk []*KeySpecElem) string {
	if parentTableName == "" {
		return renderKeySpec(pk)
	}

	parent := tableMap[parentTableName]
	parentKeys := tableKeys[parentTableName].PrimaryKey.Keys
	parentPart := renderKey(tableMap, tableKeys, lo.FromPtr(parent.ParentTableName), pk[:len(parentKeys)])
	parentKeyPart := fmt.Sprintf("%v(%v)", parentTableName, parentPart)
	childPart := pk[len(parentKeys):]
	return strings.Join([]string{parentKeyPart, renderKeySpec(childPart)}, ", ")
}

func renderKeySpec(ks []*KeySpecElem) string {
	return strings.Join(lo.Map(ks, func(item *KeySpecElem, _ int) string {
		if item.IsDesc {
			return fmt.Sprintf("%v DESC", item.ColumnName)
		}
		return item.ColumnName
	}), ", ")
}

func GetIndexColumnColumnName(index *schema.InformationSchemaIndexColumn) string {
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

func IgnoreSecond[T1, T2, R any](f func(T1) R) func(T1, T2) R {
	return func(v T1, _ T2) R {
		return f(v)
	}
}
