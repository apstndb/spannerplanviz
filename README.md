# spannerplanviz

Cloud Spanner Query Plan Visualizer using [goccy/go-graphviz](https://github.com/goccy/go-graphviz).

![query plan](docs/plan.png)


## Install

```
$ go get -u github.com/apstndb/spannerplanviz
```

## Usage

It can read [`ResultSet`](https://cloud.google.com/spanner/docs/reference/rest/v1/ResultSet?hl=en) in JSON or YAML.

(EXPERIMENTAL) It can also read JSON response of `executeStreamingSql` in GCP console.

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

## Disclaimer

This tool is PRE-ALPHA quality.