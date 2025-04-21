package main

import (
	_ "embed"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/olekukonko/tablewriter"
	"github.com/samber/lo"

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

//go:embed testdata/distributed_cross_apply_profile.yaml
var dcaProfileYAML []byte

func TestRenderTree(t *testing.T) {
	tests := []struct {
		desc      string
		input     []byte
		renderDef tableRenderDef
		want      string
	}{
		{
			"PLAN",
			dcaYAML,
			withStatsToRenderDefMap[false],
			`+-----+-------------------------------------------------------------------------------------------+
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
`,
		},
		{
			"PROFILE",
			dcaProfileYAML,
			withStatsToRenderDefMap[true],
			`+-----+-------------------------------------------------------------------------------------------+------+-------+------------+
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
`,
		},
		{
			"PROFILE with custom",
			dcaProfileYAML,
			lo.Must(customFileToTableRenderDef([]byte(`
- name: ID
  template: '{{.FormatID}}'
  alignment: RIGHT
- name: Operator
  template: '{{.Text}}'
  alignment: LEFT
- name: Rows
  template: '{{.ExecutionStats.Rows.Total}}'
  alignment: RIGHT
- name: Scanned
  template: '{{.ExecutionStats.ScannedRows.Total}}'
  alignment: RIGHT
- name: Filtered
  template: '{{.ExecutionStats.FilteredRows.Total}}'
  alignment: RIGHT
`))),
			`+-----+-------------------------------------------------------------------------------------------+------+---------+----------+
| ID  | Operator                                                                                  | Rows | Scanned | Filtered |
+-----+-------------------------------------------------------------------------------------------+------+---------+----------+
|   0 | Distributed Union on AlbumsByAlbumTitle <Row> (split_ranges_aligned: false)               |   33 |         |          |
|  *1 | +- Distributed Cross Apply <Row>                                                          |   33 |         |          |
|   2 |    +- [Input] Create Batch <Row>                                                          |      |         |          |
|   3 |    |  +- Local Distributed Union <Row>                                                    |    7 |         |          |
|   4 |    |     +- Compute Struct <Row>                                                          |    7 |         |          |
|   5 |    |        +- Index Scan on AlbumsByAlbumTitle <Row> (Full scan, scan_method: Automatic) |    7 |       7 |        0 |
|  11 |    +- [Map] Serialize Result <Row>                                                        |   33 |         |          |
|  12 |       +- Cross Apply <Row>                                                                |   33 |         |          |
|  13 |          +- [Input] Batch Scan on $v2 <Row> (scan_method: Row)                            |    7 |         |          |
|  16 |          +- [Map] Local Distributed Union <Row>                                           |   33 |         |          |
| *17 |             +- Filter Scan <Row> (seekable_key_size: 0)                                   |      |         |          |
|  18 |                +- Index Scan on SongsBySongGenre <Row> (Full scan, scan_method: Row)      |   33 |      63 |       30 |
+-----+-------------------------------------------------------------------------------------------+------+---------+----------+
Predicates(identified by ID):
  1: Split Range: ($AlbumId = $AlbumId_1)
 17: Residual Condition: ($AlbumId = $batched_AlbumId_1)
`,
		},
		{
			"PROFILE with custom list",
			dcaProfileYAML,
			lo.Must(customListToTableRenderDef([]string{
				`ID:{{.FormatID}}:RIGHT`,
				`Operator:{{.Text}}`,
				`Rows:{{.ExecutionStats.Rows.Total}}:RIGHT`,
				`Scanned:{{.ExecutionStats.ScannedRows.Total}}:RIGHT`,
				`Filtered:{{.ExecutionStats.FilteredRows.Total}}:RIGHT`,
			})),
			`+-----+-------------------------------------------------------------------------------------------+------+---------+----------+
| ID  | Operator                                                                                  | Rows | Scanned | Filtered |
+-----+-------------------------------------------------------------------------------------------+------+---------+----------+
|   0 | Distributed Union on AlbumsByAlbumTitle <Row> (split_ranges_aligned: false)               |   33 |         |          |
|  *1 | +- Distributed Cross Apply <Row>                                                          |   33 |         |          |
|   2 |    +- [Input] Create Batch <Row>                                                          |      |         |          |
|   3 |    |  +- Local Distributed Union <Row>                                                    |    7 |         |          |
|   4 |    |     +- Compute Struct <Row>                                                          |    7 |         |          |
|   5 |    |        +- Index Scan on AlbumsByAlbumTitle <Row> (Full scan, scan_method: Automatic) |    7 |       7 |        0 |
|  11 |    +- [Map] Serialize Result <Row>                                                        |   33 |         |          |
|  12 |       +- Cross Apply <Row>                                                                |   33 |         |          |
|  13 |          +- [Input] Batch Scan on $v2 <Row> (scan_method: Row)                            |    7 |         |          |
|  16 |          +- [Map] Local Distributed Union <Row>                                           |   33 |         |          |
| *17 |             +- Filter Scan <Row> (seekable_key_size: 0)                                   |      |         |          |
|  18 |                +- Index Scan on SongsBySongGenre <Row> (Full scan, scan_method: Row)      |   33 |      63 |       30 |
+-----+-------------------------------------------------------------------------------------------+------+---------+----------+
Predicates(identified by ID):
  1: Split Range: ($AlbumId = $AlbumId_1)
 17: Residual Condition: ($AlbumId = $batched_AlbumId_1)
`,
		},
	}

	for _, tcase := range tests {
		stats, _, err := queryplan.ExtractQueryPlan(tcase.input)
		if err != nil {
			t.Fatalf("invalid input at protoyaml.Unmarshal:\nerror: %v", err)
		}

		rows, err := plantree.ProcessPlan(queryplan.New(stats.GetQueryPlan().GetPlanNodes()), plantree.WithQueryPlanOptions(
			queryplan.WithTargetMetadataFormat(queryplan.TargetMetadataFormatOn),
			queryplan.WithExecutionMethodFormat(queryplan.ExecutionMethodFormatAngle),
			queryplan.WithFullScanFormat(queryplan.FullScanFormatLabel),
		))
		if err != nil {
			t.Fatal(err)
		}

		got, err := printResult(tcase.renderDef, rows, PrintPredicates)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(tcase.want, got); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	}
}
