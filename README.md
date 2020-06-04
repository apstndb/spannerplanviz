# spannerplanviz

Cloud Spanner Query Plan Visualizer using GraphViz.

## install

```
$ go get -u github.com/apstndb/spannerplanviz
```

## usage

### PROFILE

```
$ gcloud spanner databases execute-sql --instance=sampleinstance sampledb --query-mode=PROFILE --format=yaml \
  --sql "SELECT SongName FROM Songs" |
  spannerplanviz --full --type=svg --output profile.svg
```
### PLAN
```
$ gcloud spanner databases execute-sql --instance=sampleinstance sampledb --query-mode=PLAN --format=yaml \
  --sql="SELECT SongName FROM Songs WHERE STARTS_WITH(SongName, @prefix)" |
  spannerplanviz --full --type=svg --output plan.svg
```