package main

import (
	_ "embed"
	"testing"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/samber/lo"

	"github.com/apstndb/spannerplanviz/plantree"
	"github.com/apstndb/spannerplanviz/queryplan"
)

func sliceOf[T any](vs ...T) []T {
	return vs
}
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
	if v := trd.Columns[0]; v.Alignment != tw.AlignRight {
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
		opts      []plantree.Option
		want      string
	}{
		{
			"PLAN",
			dcaYAML,
			withStatsToRenderDefMap[false],
			nil,
			heredoc.Doc(`
+-----+-------------------------------------------------------------------------------------------+
| ID  | Operator                                                                                  |
+-----+-------------------------------------------------------------------------------------------+
|   0 | Distributed Union on AlbumsByAlbumTitle <Row>                                             |
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
`),
		},
		{
			"compact PLAN",
			dcaYAML,
			withStatsToRenderDefMap[false],
			sliceOf(plantree.EnableCompact()),
			heredoc.Doc(`
+-----+-----------------------------------------------------------------------------+
| ID  | Operator                                                                    |
+-----+-----------------------------------------------------------------------------+
|   0 | Distributed Union on AlbumsByAlbumTitle<Row>                                |
|  *1 | +Distributed Cross Apply<Row>                                               |
|   2 |  +[Input]Create Batch<Row>                                                  |
|   3 |  |+Local Distributed Union<Row>                                             |
|   4 |  | +Compute Struct<Row>                                                     |
|   5 |  |  +Index Scan on AlbumsByAlbumTitle<Row>(Full scan,scan_method:Automatic) |
|  11 |  +[Map]Serialize Result<Row>                                                |
|  12 |   +Cross Apply<Row>                                                         |
|  13 |    +[Input]Batch Scan on $v2<Row>(scan_method:Row)                          |
|  16 |    +[Map]Local Distributed Union<Row>                                       |
| *17 |     +Filter Scan<Row>(seekable_key_size:0)                                  |
|  18 |      +Index Scan on SongsBySongGenre<Row>(Full scan,scan_method:Row)        |
+-----+-----------------------------------------------------------------------------+
Predicates(identified by ID):
  1: Split Range: ($AlbumId = $AlbumId_1)
 17: Residual Condition: ($AlbumId = $batched_AlbumId_1)
`),
		},
		{
			"wrapped compact PLAN",
			dcaYAML,
			withStatsToRenderDefMap[false],
			sliceOf(plantree.EnableCompact(), plantree.WithWrapWidth(40)),
			heredoc.Doc(`
+-----+------------------------------------------+
| ID  | Operator                                 |
+-----+------------------------------------------+
|   0 | Distributed Union on AlbumsByAlbumTitle< |
|     | Row>                                     |
|  *1 | +Distributed Cross Apply<Row>            |
|   2 |  +[Input]Create Batch<Row>               |
|   3 |  |+Local Distributed Union<Row>          |
|   4 |  | +Compute Struct<Row>                  |
|   5 |  |  +Index Scan on AlbumsByAlbumTitle<Ro |
|     |  |   w>(Full scan,scan_method:Automatic) |
|  11 |  +[Map]Serialize Result<Row>             |
|  12 |   +Cross Apply<Row>                      |
|  13 |    +[Input]Batch Scan on $v2<Row>(scan_m |
|     |    |ethod:Row)                           |
|  16 |    +[Map]Local Distributed Union<Row>    |
| *17 |     +Filter Scan<Row>(seekable_key_size: |
|     |      0)                                  |
|  18 |      +Index Scan on SongsBySongGenre<Row |
|     |       >(Full scan,scan_method:Row)       |
+-----+------------------------------------------+
Predicates(identified by ID):
  1: Split Range: ($AlbumId = $AlbumId_1)
 17: Residual Condition: ($AlbumId = $batched_AlbumId_1)
`),
		},
		{
			"wrapped PLAN",
			dcaYAML,
			withStatsToRenderDefMap[false],
			sliceOf(plantree.WithWrapWidth(50)),
			heredoc.Doc(`
+-----+---------------------------------------------------+
| ID  | Operator                                          |
+-----+---------------------------------------------------+
|   0 | Distributed Union on AlbumsByAlbumTitle <Row>     |
|  *1 | +- Distributed Cross Apply <Row>                  |
|   2 |    +- [Input] Create Batch <Row>                  |
|   3 |    |  +- Local Distributed Union <Row>            |
|   4 |    |     +- Compute Struct <Row>                  |
|   5 |    |        +- Index Scan on AlbumsByAlbumTitle < |
|     |    |           Row> (Full scan, scan_method: Auto |
|     |    |           matic)                             |
|  11 |    +- [Map] Serialize Result <Row>                |
|  12 |       +- Cross Apply <Row>                        |
|  13 |          +- [Input] Batch Scan on $v2 <Row> (scan |
|     |          |  _method: Row)                         |
|  16 |          +- [Map] Local Distributed Union <Row>   |
| *17 |             +- Filter Scan <Row> (seekable_key_si |
|     |                ze: 0)                             |
|  18 |                +- Index Scan on SongsBySongGenre  |
|     |                   <Row> (Full scan, scan_method:  |
|     |                   Row)                            |
+-----+---------------------------------------------------+
Predicates(identified by ID):
  1: Split Range: ($AlbumId = $AlbumId_1)
 17: Residual Condition: ($AlbumId = $batched_AlbumId_1)
`),
		},
		{
			"PROFILE",
			dcaProfileYAML,
			withStatsToRenderDefMap[true],
			nil,
			heredoc.Doc(`
+-----+-------------------------------------------------------------------------------------------+------+-------+------------+
| ID  | Operator                                                                                  | Rows | Exec. | Latency    |
+-----+-------------------------------------------------------------------------------------------+------+-------+------------+
|   0 | Distributed Union on AlbumsByAlbumTitle <Row>                                             |   33 |     1 | 1.92 msecs |
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
`),
		},
		{
			"PROFILE with custom",
			dcaProfileYAML,
			lo.Must(customFileToTableRenderDef([]byte(
				heredoc.Doc(`
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
`)))),
			nil,
			heredoc.Doc(`
+-----+-------------------------------------------------------------------------------------------+------+---------+----------+
| ID  | Operator                                                                                  | Rows | Scanned | Filtered |
+-----+-------------------------------------------------------------------------------------------+------+---------+----------+
|   0 | Distributed Union on AlbumsByAlbumTitle <Row>                                             |   33 |         |          |
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
`),
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
			nil,
			heredoc.Doc(`
+-----+-------------------------------------------------------------------------------------------+------+---------+----------+
| ID  | Operator                                                                                  | Rows | Scanned | Filtered |
+-----+-------------------------------------------------------------------------------------------+------+---------+----------+
|   0 | Distributed Union on AlbumsByAlbumTitle <Row>                                             |   33 |         |          |
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
`),
		},
	}

	for _, tcase := range tests {
		stats, _, err := queryplan.ExtractQueryPlan(tcase.input)
		if err != nil {
			t.Fatalf("invalid input at protoyaml.Unmarshal:\nerror: %v", err)
		}

		opts := []plantree.Option{plantree.WithQueryPlanOptions(
			queryplan.WithTargetMetadataFormat(queryplan.TargetMetadataFormatOn),
			queryplan.WithExecutionMethodFormat(queryplan.ExecutionMethodFormatAngle),
			queryplan.WithKnownFlagFormat(queryplan.KnownFlagFormatLabel),
		)}

		opts = append(opts, tcase.opts...)

		rows, err := plantree.ProcessPlan(queryplan.New(stats.GetQueryPlan().GetPlanNodes()), opts...)
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
