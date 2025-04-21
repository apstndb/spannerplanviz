package main

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/olekukonko/tablewriter"

	"github.com/apstndb/spannerplanviz/plantree"
	"github.com/apstndb/spannerplanviz/queryplan"
)

func Test_customFileToTableRenderDef(t *testing.T) {
	yamlContent := `
- name: ID
  template: '{{.FormatID}}'
  alignment: RIGHT
`

	trd, err := customFileToTableRenderDef([]byte(yamlContent))
	if err != nil {
		t.Fatal(err)
	}

	if v := len(trd.Columns); v != 1 {
		t.Fatalf("unexpected value: %v", v)
	}
	if v := trd.Columns[0]; v.Alignment != tablewriter.ALIGN_RIGHT {
		t.Fatalf("unexpected value: %v", v)
	}
}

//go:embed testdata/distributed_cross_apply.yaml
var dcaYAML []byte

func TestRenderTree(t *testing.T) {
	stats, _, err := queryplan.ExtractQueryPlan([]byte(dcaYAML))
	if err != nil {
		var collapsedStr string
		if len(dcaYAML) > jsonSnippetLen {
			collapsedStr = "(collapsed)"
		}
		t.Fatalf("invalid input at protoyaml.Unmarshal:\nerror: %v\ninput: %.*s%s", err, jsonSnippetLen, strings.TrimSpace(string(dcaYAML)), collapsedStr)
	}

	rows, err := plantree.ProcessPlan(queryplan.New(stats.GetQueryPlan().GetPlanNodes()), plantree.WithQueryPlanOptions(
		queryplan.WithTargetMetadataFormat(queryplan.TargetMetadataFormatOn),
		queryplan.WithExecutionMethodFormat(queryplan.ExecutionMethodFormatAngle),
		queryplan.WithFullScanFormat(queryplan.FullScanFormatLabel),
	))
	if err != nil {
		t.Fatal(err)
	}

	renderDef := withStatsToRenderDefMap[false]

	want := `+-----+-------------------------------------------------------------------------------------------+
| ID  | Operator                                                                                  |
+-----+-------------------------------------------------------------------------------------------+
|   0 | Distributed Union on AlbumsByAlbumTitle <Row> (split_ranges_aligned: false)               |
|  *1 | +- Distributed Cross Apply <Row>                                                          |
|   2 |    +- [Input] Create Batch <Row>                                                          |
|   3 |    |  +- Local Distributed Union <Row>                                                    |
|   4 |    |     +- Compute Struct <Row>                                                          |
|   5 |    |        +- Index Scan on AlbumsByAlbumTitle <Row> (Full scan, scan_method: Automatic) |
|  11 |    +- [Map] Serialize Result <Row>                                                        |
|  12 |       +- Cross Apply <Row>                                                                |
|  13 |          +- [Input] Batch Scan on $v2 <Row> (scan_method: Row)                            |
|  16 |          +- [Map] Local Distributed Union <Row>                                           |
| *17 |             +- Filter Scan <Row> (seekable_key_size: 0)                                   |
|  18 |                +- Index Scan on SongsBySongGenre <Row> (Full scan, scan_method: Row)      |
+-----+-------------------------------------------------------------------------------------------+
Predicates(identified by ID):
  1: Split Range: ($AlbumId = $AlbumId_1)
 17: Residual Condition: ($AlbumId = $batched_AlbumId_1)
`
	s, err := printResult(renderDef, rows, PrintPredicates)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(want, s); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

//go:embed testdata/distributed_cross_apply_profile.yaml
var dcaProfileYAML []byte

func TestRenderTreeProfile(t *testing.T) {
	y := dcaProfileYAML
	stats, _, err := queryplan.ExtractQueryPlan([]byte(y))
	if err != nil {
		var collapsedStr string
		if len(y) > jsonSnippetLen {
			collapsedStr = "(collapsed)"
		}
		t.Fatalf("invalid input at protoyaml.Unmarshal:\nerror: %v\ninput: %.*s%s", err, jsonSnippetLen, strings.TrimSpace(string(y)), collapsedStr)
	}

	rows, err := plantree.ProcessPlan(queryplan.New(stats.GetQueryPlan().GetPlanNodes()), plantree.WithQueryPlanOptions(
		queryplan.WithTargetMetadataFormat(queryplan.TargetMetadataFormatOn),
		queryplan.WithExecutionMethodFormat(queryplan.ExecutionMethodFormatAngle),
		queryplan.WithFullScanFormat(queryplan.FullScanFormatLabel),
	))
	if err != nil {
		t.Fatal(err)
	}

	renderDef := withStatsToRenderDefMap[true]

	want := `+-----+-------------------------------------------------------------------------------------------+------+-------+------------+
| ID  | Operator                                                                                  | Rows | Exec. | Latency    |
+-----+-------------------------------------------------------------------------------------------+------+-------+------------+
|   0 | Distributed Union on AlbumsByAlbumTitle <Row> (split_ranges_aligned: false)               |   33 |     1 | 1.92 msecs |
|  *1 | +- Distributed Cross Apply <Row>                                                          |   33 |     1 |  1.9 msecs |
|   2 |    +- [Input] Create Batch <Row>                                                          |      |       |            |
|   3 |    |  +- Local Distributed Union <Row>                                                    |    7 |     1 | 0.95 msecs |
|   4 |    |     +- Compute Struct <Row>                                                          |    7 |     1 | 0.94 msecs |
|   5 |    |        +- Index Scan on AlbumsByAlbumTitle <Row> (Full scan, scan_method: Automatic) |    7 |     1 | 0.93 msecs |
|  11 |    +- [Map] Serialize Result <Row>                                                        |   33 |     1 | 0.88 msecs |
|  12 |       +- Cross Apply <Row>                                                                |   33 |     1 | 0.87 msecs |
|  13 |          +- [Input] Batch Scan on $v2 <Row> (scan_method: Row)                            |    7 |     1 | 0.01 msecs |
|  16 |          +- [Map] Local Distributed Union <Row>                                           |   33 |     7 | 0.85 msecs |
| *17 |             +- Filter Scan <Row> (seekable_key_size: 0)                                   |      |       |            |
|  18 |                +- Index Scan on SongsBySongGenre <Row> (Full scan, scan_method: Row)      |   33 |     7 | 0.84 msecs |
+-----+-------------------------------------------------------------------------------------------+------+-------+------------+
Predicates(identified by ID):
  1: Split Range: ($AlbumId = $AlbumId_1)
 17: Residual Condition: ($AlbumId = $batched_AlbumId_1)
`
	s, err := printResult(renderDef, rows, PrintPredicates)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(want, s); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
