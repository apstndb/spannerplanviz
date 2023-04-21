package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"regexp"
	"strings"

	"cloud.google.com/go/spanner"
	"github.com/apstndb/spannerplanviz/internal/schema"
	"github.com/kenshaw/snaker"
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
	tableSchema := flag.String("table-schema", "", "")
	tableName := flag.String("table-name", "", "")

	flag.Parse()

	cli, err := spanner.NewClient(ctx, fmt.Sprintf("projects/%s/instances/%s/databases/%s", *project, *instance, *database))
	if err != nil {
		return err
	}
	defer cli.Close()

	var columns []*schema.InformationSchemaColumn
	statement := spanner.NewStatement(
		`
SELECT * FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = @tableSchema AND TABLE_NAME = @tableName 
`)
	statement.Params["tableSchema"] = *tableSchema
	statement.Params["tableName"] = *tableName
	err = cli.Single().Query(ctx, statement).Do(
		func(r *spanner.Row) error {
			var column schema.InformationSchemaColumn
			err := r.ToStruct(&column)
			if err != nil {
				return err
			}
			columns = append(columns, &column)
			return nil
		},
	)
	if err != nil {
		return err
	}
	fmt.Printf("type %v struct {\n", snaker.SnakeToCamel(*tableSchema)+snaker.SnakeToCamel(*tableName))
	for _, column := range columns {
		t, err := spannerTypeToGoType(*column.SpannerType, yesNoToBool(column.IsNullable))
		if err != nil {
			return err
		}
		fmt.Printf("%v %v `spanner:\"%v\" json:\"%v\"`\n", snaker.SnakeToCamelIdentifier(column.ColumnName), t, column.ColumnName, column.ColumnName)
	}
	fmt.Println("}")
	return nil
}

var spannerBaseTypeRe = regexp.MustCompile("^([^(<]*)")

func spannerTypeToGoType(spannerType string, isNullable bool) (string, error) {
	t, err := spannerTypeToGoTypePrimitive(spannerType)
	if err != nil {
		return "", err
	}
	if !isNullable {
		return t, nil
	}
	switch t {
	case "BYTES":
		return t, nil
	default:
		return "*" + t, nil
	}
}
func spannerTypeToGoTypePrimitive(spannerType string) (string, error) {
	switch t := spannerBaseTypeRe.FindString(spannerType); t {
	case "BOOL":
		return "bool", nil
	case "INT64":
		return "int64", nil
	case "FLOAT64":
		return "float64", nil
	case "TIMESTAMP":
		return "time.Time", nil
	case "DATE":
		return "civil.Date", nil
	case "STRING":
		return "string", nil
	case "BYTES":
		return "[]byte", nil
	case "NUMERIC":
		return "*big.Rat", nil
	case "JSON":
		return "spanner.NullJSON", nil
	// case "ARRAY":
	// case "STRUCT":
	// case "TYPE_CODE_UNSPECIFIED":
	default:
		return "", fmt.Errorf("unsupported Cloud Spanner type: %v", t)
	}
}

func yesNoToBool(s *string) bool {
	if s == nil {
		return false
	}
	if strings.ToUpper(*s) == "YES" {
		return true
	}
	return false
}
