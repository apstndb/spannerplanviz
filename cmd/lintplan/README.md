# lintplan(EXPERIMENTAL)

```
$ gcloud spanner databases execute-sql $DATABASE_ID --project $PROJECT_ID --instance $INSTANCE_ID \
  --sql='SELECT s.SongGenre FROM Songs AS s ORDER BY SongGenre' --query-mode=PLAN --format=yaml > tmp_plan.yaml &&
   rendertree < tmp_plan.yaml &&
   echo "---" &&
   go run ./cmd/lintplan < tmp_plan.yaml
+----+-----------------------------------------------------------------------------+
| ID | Operator                                                                    |
+----+-----------------------------------------------------------------------------+
|  0 | Distributed Union (preserve_subquery_order: true)                           |
|  1 | +- Serialize Result                                                         |
|  2 |    +- Sort                                                                  |
|  3 |       +- Local Distributed Union                                            |
|  4 |          +- Table Scan (Full scan: true, Table: Songs, scan_method: Scalar) |
+----+-----------------------------------------------------------------------------+
---
2: Sort
    Expensive operator Sort: Can't you use the same order with the index?
4: Table Scan (Full scan: true, Table: Songs, scan_method: Scalar)
    Full scan=true: Expensive execution full scan: Do you really want full scan?
```