package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/apstndb/lox"
	"github.com/goccy/go-yaml"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/samber/lo"

	"github.com/apstndb/spannerplanviz/plantree"
	"github.com/apstndb/spannerplanviz/queryplan"
)

func main() {
	if err := run(); err != nil {
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

func (tdef tableRenderDef) ColumnAlignments() []tw.Align {
	var alignments []tw.Align
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

func parseAlignment(s string) (tw.Align, error) {
	switch strings.TrimPrefix(s, "ALIGN_") {
	case "RIGHT":
		return tw.AlignRight, nil
	case "LEFT":
		return tw.AlignLeft, nil
	case "CENTER":
		return tw.AlignCenter, nil
	case "DEFAULT":
		return tw.AlignDefault, nil
	case "NONE":
		return tw.AlignNone, nil
	default:
		return tw.AlignNone, fmt.Errorf("unknown Alignment: %s", s)
	}
}

type plainColumnRenderDef struct {
	Template  string   `json:"template"`
	Name      string   `json:"name"`
	Alignment tw.Align `json:"alignment"`
}

type columnRenderDef struct {
	MapFunc   func(row plantree.RowWithPredicates) (string, error)
	Name      string
	Alignment tw.Align
}

func templateMapFunc(tmplName, tmplText string) (func(row plantree.RowWithPredicates) (string, error), error) {
	tmpl, err := template.New(tmplName).Parse(tmplText)
	if err != nil {
		return nil, err
	}

	return func(row plantree.RowWithPredicates) (string, error) {
		var sb strings.Builder
		if err = tmpl.Execute(&sb, row); err != nil {
			return "", err
		}

		return sb.String(), nil
	}, nil
}

var (
	idRenderDef = columnRenderDef{
		Name:      "ID",
		Alignment: tw.AlignRight,
		MapFunc: func(row plantree.RowWithPredicates) (string, error) {
			return row.FormatID(), nil
		},
	}
	operatorRenderDef = columnRenderDef{
		Name:      "Operator",
		Alignment: tw.AlignLeft,
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
					Alignment: tw.AlignRight,
				},
				{
					MapFunc: func(row plantree.RowWithPredicates) (string, error) {
						return row.ExecutionStats.ExecutionSummary.NumExecutions, nil
					},
					Name:      "Exec.",
					Alignment: tw.AlignRight,
				},
				{
					MapFunc: func(row plantree.RowWithPredicates) (string, error) {
						return row.ExecutionStats.Latency.String(), nil
					},
					Name:      "Latency",
					Alignment: tw.AlignRight,
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

const jsonSnippetLen = 140

type PrintMode int

const (
	PrintPredicates PrintMode = iota
	PrintTyped
	PrintFull
)

func parsePrintMode(s string) PrintMode {
	switch strings.ToLower(s) {
	case "predicates":
		return PrintPredicates
	case "typed":
		return PrintTyped
	case "full":
		return PrintFull
	default:
		panic(fmt.Sprintf("unknown PrintMode: %s", s))
	}
}

func run() error {
	customFile := flag.String("custom-file", "", "")
	mode := flag.String("mode", "", "PROFILE or PLAN(ignore case)")
	printModeStr := flag.String("print", "predicates", "print node parameters(EXPERIMENTAL)")
	disallowUnknownStats := flag.Bool("disallow-unknown-stats", false, "error on unknown stats field")
	executionMethod := flag.String("execution-method", "angle", "Format execution method metadata: 'angle' or 'raw' (default: angle)")
	targetMetadata := flag.String("target-metadata", "on", "Format target metadata: 'on' or 'raw' (default: on)")
	fullscan := flag.String("full-scan", "", "Deprecated alias for --known-flag.")
	knownFlag := flag.String("known-flag", "", "Format known flags: 'label' or 'raw' (default: label)")
	compact := flag.Bool("compact", false, "Enable compact format")
	wrapWidth := flag.Int("wrap-width", 0, "Number of characters at which to wrap the Operator column content. 0 means no wrapping.")

	var custom stringList
	flag.Var(&custom, "custom", "")
	flag.Parse()

	if *fullscan != "" {
		if *knownFlag != "" {
			fmt.Fprintln(os.Stderr, "--full-scan and --known-flag are mutually exclusive.")
			flag.Usage()
			os.Exit(1)
		}

		fmt.Fprintln(os.Stderr, "--full-scan is deprecated. you must migrate to --known-flag.")

		*knownFlag = *fullscan
	}

	printMode := parsePrintMode(*printModeStr)

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

	var opts []plantree.Option
	if *disallowUnknownStats {
		opts = append(opts, plantree.DisallowUnknownStats())
	}

	if *compact {
		opts = append(opts, plantree.EnableCompact())
	}

	switch strings.ToUpper(*executionMethod) {
	case "", "ANGLE":
		opts = append(opts, plantree.WithQueryPlanOptions(queryplan.WithExecutionMethodFormat(queryplan.ExecutionMethodFormatAngle)))
	case "RAW":
		opts = append(opts, plantree.WithQueryPlanOptions(queryplan.WithExecutionMethodFormat(queryplan.ExecutionMethodFormatRaw)))
	default:
		fmt.Fprintf(os.Stderr, "Invalid value for -execution-method flag: %s.  Must be 'angle' or 'raw'.\n", *executionMethod)
		flag.Usage()
		os.Exit(1)
	}

	switch strings.ToUpper(*targetMetadata) {
	case "", "ON":
		opts = append(opts, plantree.WithQueryPlanOptions(queryplan.WithTargetMetadataFormat(queryplan.TargetMetadataFormatOn)))
	case "RAW":
		opts = append(opts, plantree.WithQueryPlanOptions(queryplan.WithTargetMetadataFormat(queryplan.TargetMetadataFormatRaw)))
	default:
		fmt.Fprintf(os.Stderr, "Invalid value for -target-metadata flag: %s.  Must be 'on' or 'raw'.\n", *targetMetadata)
		flag.Usage()
		os.Exit(1)
	}

	switch strings.ToUpper(*knownFlag) {
	case "", "LABEL":
		opts = append(opts, plantree.WithQueryPlanOptions(queryplan.WithKnownFlagFormat(queryplan.KnownFlagFormatLabel)))
	case "RAW":
		opts = append(opts, plantree.WithQueryPlanOptions(queryplan.WithKnownFlagFormat(queryplan.KnownFlagFormatRaw)))
	default:
		fmt.Fprintf(os.Stderr, "Invalid value for -known-flag flag: %s.  Must be 'label' or 'raw'.\n", *knownFlag)
		flag.Usage()
		os.Exit(1)
	}

	if *wrapWidth > 0 {
		opts = append(opts, plantree.WithWrapWidth(*wrapWidth))
	}

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

	rows, err := plantree.ProcessPlan(queryplan.New(stats.GetQueryPlan().GetPlanNodes()), opts...)
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
		b, err := os.ReadFile(*customFile)
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

	s, err := printResult(renderDef, rows, printMode)
	if err != nil {
		return err
	}

	_, err = os.Stdout.WriteString(s)
	return err
}

func unmarshalAlign(t *tw.Align, bytes []byte) error {
	var s string
	if err := yaml.Unmarshal(bytes, &s); err != nil {
		return err
	}

	align, err := parseAlignment(s)
	if err != nil {
		return err
	}

	*t = align
	return nil
}

func customFileToTableRenderDef(b []byte) (tableRenderDef, error) {
	decodeOpts := []yaml.DecodeOption{yaml.CustomUnmarshaler(unmarshalAlign)}

	var defs []plainColumnRenderDef
	if err := yaml.UnmarshalWithOptions(b, &defs, decodeOpts...); err != nil {
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
			Alignment: def.Alignment,
		})
	}
	return tdef, nil
}

func customListToTableRenderDef(custom []string) (tableRenderDef, error) {
	var columns []columnRenderDef
	for _, s := range custom {
		split := strings.SplitN(s, ":", 3)

		var align tw.Align
		switch len(split) {
		case 2:
			align = tw.AlignNone
		case 3:
			alignStr := split[2]
			var err error
			align, err = parseAlignment(alignStr)
			if err != nil {
				return tableRenderDef{}, fmt.Errorf("failed to parseAlignment(): %w", err)
			}
		default:
			return tableRenderDef{}, fmt.Errorf(`invalid format: must be "<name>:<template>[:<alignment>]", but: %v`, s)
		}

		name, templateStr := split[0], split[1]
		mapFunc, err := templateMapFunc(name, templateStr)
		if err != nil {
			return tableRenderDef{}, err
		}

		columns = append(columns, columnRenderDef{
			MapFunc:   mapFunc,
			Name:      name,
			Alignment: align,
		})
	}
	return tableRenderDef{Columns: columns}, nil
}

func printResult(renderDef tableRenderDef, rows []plantree.RowWithPredicates, printMode PrintMode) (string, error) {
	var b strings.Builder
	table := tablewriter.NewTable(&b,
		tablewriter.WithRenderer(
			renderer.NewBlueprint(tw.Rendition{Symbols: tw.NewSymbols(tw.StyleASCII)})),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithTrimSpace(tw.Off),
	)

	// Some config can't be correctly configured by tablewriter.Option.
	table.Configure(func(config *tablewriter.Config) {
		config.Row.ColumnAligns = renderDef.ColumnAlignments()
		config.Row.Formatting.AutoWrap = tw.WrapNone
		config.Header.Formatting.AutoFormat = false
	})

	table.Header(renderDef.ColumnNames())

	for _, row := range rows {
		values, err := renderDef.ColumnMapFunc(row)
		if err != nil {
			return "", err
		}
		if err = table.Append(values); err != nil {
			return "", err
		}
	}

	if len(rows) > 0 {
		if err := table.Render(); err != nil {
			return "", err
		}
	}

	var maxIDLength int
	for _, row := range rows {
		if length := len(fmt.Sprint(row.ID)); length > maxIDLength {
			maxIDLength = length
		}
	}

	var predicates []string
	var parameters []string
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

		i := 0
		for _, t := range lox.EntriesSortedByKey(row.ChildLinks) {
			typ, childLinks := t.Key, t.Value
			if printMode != PrintFull && typ == "" {
				continue
			}

			if i == 0 {
				prefix = fmt.Sprintf("%*d:", maxIDLength, row.ID)
			} else {
				prefix = strings.Repeat(" ", maxIDLength+1)
			}

			join := strings.Join(lo.Map(childLinks, func(item *queryplan.ResolvedChildLink, index int) string {
				if varName := item.ChildLink.GetVariable(); varName != "" {
					return fmt.Sprintf("$%s=%s", item.ChildLink.GetVariable(), item.Child.GetShortRepresentation().GetDescription())
				} else {
					return item.Child.GetShortRepresentation().GetDescription()
				}
			}), ", ")
			if join == "" {
				continue
			}
			i++
			typePartStr := lo.Ternary(typ != "", typ+": ", "")
			parameters = append(parameters, fmt.Sprintf("%s %s%s", prefix, typePartStr, join))
		}
	}

	switch printMode {
	case PrintFull, PrintTyped:
		if len(parameters) > 0 {
			fmt.Fprintln(&b, "Node Parameters(identified by ID):")
			for _, s := range parameters {
				fmt.Fprintf(&b, " %s\n", s)
			}
		}
	case PrintPredicates:
		if len(predicates) > 0 {
			fmt.Fprintln(&b, "Predicates(identified by ID):")
			for _, s := range predicates {
				fmt.Fprintf(&b, " %s\n", s)
			}
		}
	}
	return b.String(), nil
}
