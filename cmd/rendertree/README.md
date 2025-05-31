This tool render YAML or JSON representation of Cloud Spanner query plan as ascii format.

It can read various types.
* [QueryPlan](https://cloud.google.com/spanner/docs/reference/rest/v1/ResultSetStats?hl=en#QueryPlan)
  * Can get easily by client libraries
    * [AnalyzeQuery()](https://pkg.go.dev/cloud.google.com/go/spanner#ReadOnlyTransaction.AnalyzeQuery)
    * [RowIterator.QueryPlan](https://pkg.go.dev/cloud.google.com/go/spanner#RowIterator)
* [ResultSetStats](https://cloud.google.com/spanner/docs/reference/rest/v1/ResultSetStats?hl=en)
  * Output of DOWNLOAD JSON in [the official query plan visualizer](https://cloud.google.com/spanner/docs/tune-query-with-visualizer?hl=en)
* [ResultSet](https://cloud.google.com/spanner/docs/reference/rest/v1/ResultSet?hl=en)
  * Output of `gcloud spanner databases execute-sql` and [execspansql](https://github.com/apstndb/execspansql)

It can render both PLAN or PROFILE.

```
# from file
$ cat queryplan.yaml | rendertree --mode=PLAN
+----+-----------------------------------------+
| ID | Operator                                |
+----+-----------------------------------------+
| *0 | Distributed Union                       |
|  1 | +- Local Distributed Union              |
|  2 |    +- Serialize Result                  |
| *3 |       +- FilterScan                     |
|  4 |          +- Table Scan (Table: Singers) |
+----+-----------------------------------------+
Predicates(identified by ID):
 0: Split Range: ($SingerId = 1)
 3: Seek Condition: ($SingerId = 1)

# with gcloud spanner databases execute-sql
$ gcloud spanner databases execute-sql ${DATABASE_ID} --sql="SELECT * FROM Singers" --format=json --query-mode=PROFILE |
    rendertree --mode=PROFILE
+----+-------------------------------------------------------+------+-------+------------+
| ID | Operator                                              | Rows | Exec. | Latency    |
+----+-------------------------------------------------------+------+-------+------------+
|  0 | Distributed Union                                     | 1000 |     1 | 6.29 msecs |
|  1 | +- Local Distributed Union                            | 1000 |     1 | 6.21 msecs |
|  2 |    +- Serialize Result                                | 1000 |     1 | 6.16 msecs |
|  3 |       +- Table Scan (Full scan: true, Table: Singers) | 1000 |     1 | 5.78 msecs |
+----+-------------------------------------------------------+------+-------+------------+
```

Rendered stats columns are customizable using `--custom-file`.

```
$ cat custom.yaml
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

$ cat scan_profile.json | rendertree --mode=PROFILE --custom-file=custom.yaml
+-----+-------------------------------------------------------+------+---------+
| ID  | Operator                                              | Rows | Scanned |
+-----+-------------------------------------------------------+------+---------+
|   0 | Serialize Result                                      | 2960 |         |
|   1 | +- Union All                                          | 2960 |         |
|   2 |    +- Union Input                                     |      |         |
|  *3 |    |  +- Distributed Union                            |   84 |         |
|   4 |    |     +- Local Distributed Union                   |   84 |         |
|  *5 |    |        +- FilterScan                             |   84 |         |
|   6 |    |           +- Index Scan (Index: SongsBySongName) |   84 |   16942 |
|  24 |    +- Union Input                                     |      |         |
| *25 |       +- Distributed Union                            | 2876 |         |
|  26 |          +- Local Distributed Union                   | 2876 |         |
| *27 |             +- FilterScan                             | 2876 |         |
|  28 |                +- Index Scan (Index: SongsBySongName) | 2876 |    2876 |
+-----+-------------------------------------------------------+------+---------+
Predicates(identified by ID):
  3: Split Range: (STARTS_WITH($SongName, 'A') AND ($SongName LIKE 'A%z'))
  5: Seek Condition: STARTS_WITH($SongName, 'A')
     Residual Condition: ($SongName LIKE 'A%z')
 25: Split Range: STARTS_WITH($SongName_1, 'Thi')
 27: Seek Condition: STARTS_WITH($SongName_1, 'Thi')
```

## Options for narrower width

rendertree supports a compact format and wrapping for limited width environment.

- `--compact` enables the compact format:
  - Each level of depth in the Query Plan tree adds only one character to its indentation.
  - Whitespaces are not inserted for operator and metadata display unless it causes ambiguity.
- `--wrap-width` specifies the number of characters at which to wrap the content of the Operator column.
  - The tree won't be broken even when lines are wrapped.

```
$ rendertree --compact --wrap-width=60 < testdata/distributed_cross_apply.yaml 
+-----+--------------------------------------------------------------+
| ID  | Operator                                                     |
+-----+--------------------------------------------------------------+
|   0 | Distributed Union on AlbumsByAlbumTitle<Row>                 |
|  *1 | +Distributed Cross Apply<Row>                                |
|   2 |  +[Input]Create Batch<Row>                                   |
|   3 |  |+Local Distributed Union<Row>                              |
|   4 |  | +Compute Struct<Row>                                      |
|   5 |  |  +Index Scan on AlbumsByAlbumTitle<Row>(Full scan,scan_me |
|     |  |   thod:Automatic)                                         |
|  11 |  +[Map]Serialize Result<Row>                                 |
|  12 |   +Cross Apply<Row>                                          |
|  13 |    +[Input]Batch Scan on $v2<Row>(scan_method:Row)           |
|  16 |    +[Map]Local Distributed Union<Row>                        |
| *17 |     +Filter Scan<Row>(seekable_key_size:0)                   |
|  18 |      +Index Scan on SongsBySongGenre<Row>(Full scan,scan_met |
|     |       hod:Row)                                               |
+-----+--------------------------------------------------------------+
Predicates(identified by ID):
  1: Split Range: ($AlbumId = $AlbumId_1)
 17: Residual Condition: ($AlbumId = $batched_AlbumId_1)
```