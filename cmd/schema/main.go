package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spannerplanviz/internal/schema"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalln(err)
	}
}

func run(ctx context.Context) error {
	project := flag.String("project", "", "")
	database := flag.String("database", "", "")
	instance := flag.String("instance", "", "")
	strict := flag.Bool("strict", false, "")

	flag.Parse()

	client, err := spanner.NewClient(ctx, fmt.Sprintf("projects/%s/instances/%s/databases/%s", *project, *instance, *database))
	if err != nil {
		return err
	}
	defer client.Close()

	idxs, err := queryInformationSchema[schema.InformationSchemaIndex](ctx, client, *strict, "INDEXES")
	if err != nil {
		return err
	}

	tables, err := queryInformationSchema[schema.InformationSchemaTable](ctx, client, *strict, "TABLES")
	if err != nil {
		return err
	}

	indexColumns, err := queryInformationSchema[schema.InformationSchemaIndexColumn](ctx, client, *strict, "INDEX_COLUMNS")
	if err != nil {
		return err
	}

	columns, err := queryInformationSchema[schema.InformationSchemaColumn](ctx, client, *strict, "COLUMNS")
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.Encode(schema.InformationSchema{
		Indexes:      idxs,
		Tables:       tables,
		IndexColumns: indexColumns,
		Columns:      columns,
	})
	return nil
}

func queryInformationSchema[T any](ctx context.Context, client *spanner.Client, strict bool, name string) ([]*T, error) {
	var rows []*T
	err := client.Single().Query(ctx, spanner.NewStatement(fmt.Sprintf(`SELECT * FROM INFORMATION_SCHEMA.%v`, name))).Do(
		func(r *spanner.Row) error {
			var idx T
			var err error
			if strict {
				err = r.ToStruct(&idx)
			} else {
				err = r.ToStructLenient(&idx)
			}
			if err != nil {
				return err
			}
			rows = append(rows, &idx)
			return nil
		},
	)
	return rows, err
}
