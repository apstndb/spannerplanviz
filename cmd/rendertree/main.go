package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/apstndb/spannerplanviz/plantree"
	"github.com/apstndb/spannerplanviz/protoyaml"
	"github.com/apstndb/spannerplanviz/queryplan"
	"github.com/olekukonko/tablewriter"
	"google.golang.org/genproto/googleapis/spanner/v1"
	"gopkg.in/yaml.v3"
)

func main() {
	if err := _main(); err != nil {
		log.Fatal(err)
	}
}

type tableRenderDef struct {
	Columns []columnRenderDef
}

func (tdef tableRenderDef) ColumnNames() []string {
	var columnNames []string
	for _, s := range tdef.Columns {
		columnNames = append(columnNames, s.Name)
	}
	return columnNames
}

func (tdef tableRenderDef) ColumnAlignments() []int {
	var alignments []int
	for _, s := range tdef.Columns {
		alignments = append(alignments, s.Alignment)
	}
	return alignments
}

func (tdef tableRenderDef) ColumnMapFunc(row plantree.RowWithPredicates) ([]string, error) {
	var columns []string
	for _, s := range tdef.Columns {
		v, err := s.MapFunc(row)
		if err != nil {
			return nil, err
		}
		columns = append(columns, v)
	}
	return columns, nil
}

type Alignment int

func (a *Alignment) MarshalJSON() ([]byte, error) {
	switch *a {
	case tablewriter.ALIGN_RIGHT:
		return []byte(`"RIGHT"`), nil
	case tablewriter.ALIGN_LEFT:
		return []byte(`"LEFT"`), nil
	case tablewriter.ALIGN_DEFAULT:
		return []byte(`"DEFAULT"`), nil
	case tablewriter.ALIGN_CENTER:
		return []byte(`"CENTER"`), nil
	default:
		return nil, fmt.Errorf("unknown Alignment: %d", int32(*a))
	}
}

func (a *Alignment) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return err
	}
	switch strings.TrimPrefix(s, "ALIGN_") {
	case "RIGHT":
		*a = tablewriter.ALIGN_RIGHT
	case "LEFT":
		*a = tablewriter.ALIGN_LEFT
	case "CENTER":
		*a = tablewriter.ALIGN_CENTER
	case "DEFAULT":
		*a = tablewriter.ALIGN_DEFAULT
	default:
		return fmt.Errorf("unknown Alignment: %s", s)
	}
	return nil
}

type plainColumnRenderDef struct {
	Template  string    `json:"template"`
	Name      string    `json:"name"`
	Alignment Alignment `json:"alignment"`
}

type columnRenderDef struct {
	MapFunc   func(row plantree.RowWithPredicates) (string, error)
	Name      string
	Alignment int
}

func templateMapFunc(tmplName, tmplText string) (func(row plantree.RowWithPredicates) (string, error), error) {
	tmpl, err := template.New(tmplName).Parse(tmplText)
	if err != nil {
		return nil, err
	}
	return func(row plantree.RowWithPredicates) (string, error) {
		var buf bytes.Buffer
		if err != nil {
			return "", err
		}
		err = tmpl.Execute(&buf, row)
		if err != nil {
			return "", err
		}
		return buf.String(), nil
	}, nil
}

var (
	idRenderDef = columnRenderDef{
		Name:      "ID",
		Alignment: tablewriter.ALIGN_RIGHT,
		MapFunc: func(row plantree.RowWithPredicates) (string, error) {
			return row.FormatID(), nil
		},
	}
	operatorRenderDef = columnRenderDef{
		Name:      "Operator",
		Alignment: tablewriter.ALIGN_LEFT,
		MapFunc: func(row plantree.RowWithPredicates) (string, error) {
			return row.Text(), nil
		},
	}
)
var (
	withStatsToRenderDefMap = map[bool]tableRenderDef{
		false: {
			Columns: []columnRenderDef{idRenderDef, operatorRenderDef},
		},
		true: {
			Columns: []columnRenderDef{
				idRenderDef,
				operatorRenderDef,
				{
					MapFunc: func(row plantree.RowWithPredicates) (string, error) {
						return row.ExecutionStats.Rows.Total, nil
					},
					Name:      "Rows",
					Alignment: tablewriter.ALIGN_RIGHT,
				},
				{
					MapFunc: func(row plantree.RowWithPredicates) (string, error) {
						return row.ExecutionStats.ExecutionSummary.NumExecutions, nil
					},
					Name:      "Exec.",
					Alignment: tablewriter.ALIGN_RIGHT,
				},
				{
					MapFunc: func(row plantree.RowWithPredicates) (string, error) {
						return row.ExecutionStats.Latency.String(), nil
					},
					Name:      "Latency",
					Alignment: tablewriter.ALIGN_RIGHT,
				},
			},
		},
	}
)

type stringList []string

func (s *stringList) String() string {
	return fmt.Sprint([]string(*s))
}

func (s *stringList) Set(s2 string) error {
	*s = append(*s, strings.Split(s2, ",")...)
	return nil
}

func _main() error {
	customFile := flag.String("custom-file", "", "")
	mode := flag.String("mode", "", "PROFILE or PLAN(ignore case)")
	var custom stringList
	flag.Var(&custom, "custom", "")
	flag.Parse()

	var withStats bool
	switch strings.ToUpper(*mode) {
	case "", "PLAN":
		withStats = false
	case "PROFILE":
		withStats = true
	default:
		flag.Usage()
		os.Exit(1)
	}

	b, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	var qp spanner.QueryPlan
	err = protoyaml.Unmarshal(b, &qp)
	if err != nil {
		return fmt.Errorf("invalid input at protoyaml.Unmarshal:\nerror: %w\ninput: %.140s", err, strings.TrimSpace(string(b)))
	}

	rows, err := plantree.ProcessPlan(queryplan.New(qp.GetPlanNodes()))
	if err != nil {
		return err
	}

	var renderDef tableRenderDef
	if len(custom) > 0 {
		renderDef, err = customListToTableRenderDef(custom)
		if err != nil {
			return err
		}
	} else if *customFile != "" {
		b, err := ioutil.ReadFile(*customFile)
		if err != nil {
			return err
		}
		renderDef, err = customFileToTableRenderDef(b)
		if err != nil {
			return err
		}
	} else {
		renderDef = withStatsToRenderDefMap[withStats]
	}
	return printResult(os.Stdout, renderDef, rows)
}

func customFileToTableRenderDef(b []byte) (tableRenderDef, error) {
	var defs []plainColumnRenderDef
	err := yaml.Unmarshal(b, &defs)
	if err != nil {
		return tableRenderDef{}, err
	}
	var tdef tableRenderDef
	for _, def := range defs {
		mapFunc, err := templateMapFunc(def.Name, def.Template)
		if err != nil {
			return tableRenderDef{}, err
		}
		tdef.Columns = append(tdef.Columns, columnRenderDef{
			MapFunc:   mapFunc,
			Name:      def.Name,
			Alignment: int(def.Alignment),
		})
	}
	return tdef, nil
}

func customListToTableRenderDef(custom []string) (tableRenderDef, error) {
	var columns []columnRenderDef
	for _, s := range custom {
		split := strings.SplitN(s, ":", 3)

		var align int
		if len(split) <= 2 {
			align = tablewriter.ALIGN_DEFAULT
		} else {
			switch strings.TrimPrefix(split[2], "ALIGN_") {
			case "LEFT":
				align = tablewriter.ALIGN_LEFT
			case "RIGHT":
				align = tablewriter.ALIGN_RIGHT
			case "DEFAULT":
				align = tablewriter.ALIGN_DEFAULT
			case "CENTER":
				align = tablewriter.ALIGN_CENTER
			default:
				log.Fatal("Unknown alignment", split[2])
			}
		}
		mapFunc, err := templateMapFunc(split[0], split[1])
		if err != nil {
			return tableRenderDef{}, err
		}
		columns = append(columns, columnRenderDef{
			MapFunc:   mapFunc,
			Name:      split[0],
			Alignment: align,
		})
	}
	return tableRenderDef{Columns: columns}, nil
}

func printResult(out io.Writer, renderDef tableRenderDef, rows []plantree.RowWithPredicates) error {
	table := tablewriter.NewWriter(out)
	table.SetAutoFormatHeaders(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetColumnAlignment(renderDef.ColumnAlignments())
	table.SetAutoWrapText(false)

	for _, row := range rows {
		values, err := renderDef.ColumnMapFunc(row)
		if err != nil {
			return err
		}
		table.Append(values)
	}
	table.SetHeader(renderDef.ColumnNames())
	if len(rows) > 0 {
		table.Render()
	}

	var maxIDLength int
	for _, row := range rows {
		if length := len(fmt.Sprint(row.ID)); length > maxIDLength {
			maxIDLength = length
		}
	}

	var predicates []string
	for _, row := range rows {
		var prefix string
		for i, predicate := range row.Predicates {
			if i == 0 {
				prefix = fmt.Sprintf("%*d:", maxIDLength, row.ID)
			} else {
				prefix = strings.Repeat(" ", maxIDLength+1)
			}
			predicates = append(predicates, fmt.Sprintf("%s %s", prefix, predicate))
		}
	}

	if len(predicates) > 0 {
		fmt.Fprintln(out, "Predicates(identified by ID):")
		for _, s := range predicates {
			fmt.Fprintf(out, " %s\n", s)
		}
	}
	return nil
}
