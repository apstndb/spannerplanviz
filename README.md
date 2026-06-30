# spannerplanviz

Cloud Spanner Query Plan Visualizer using [goccy/go-graphviz](https://github.com/goccy/go-graphviz).

![query plan](docs/plan.png)

(Possibly) remote calls are rendered as dashed lines.

## Install

```sh
go install github.com/apstndb/spannerplanviz@latest
```

## Usage

It can read various types in JSON and YAML.

* [QueryPlan](https://cloud.google.com/spanner/docs/reference/rest/v1/ResultSetStats?hl=en#QueryPlan)
    * Can get easily by client libraries
        * [AnalyzeQuery()](https://pkg.go.dev/cloud.google.com/go/spanner#ReadOnlyTransaction.AnalyzeQuery)
        * [RowIterator.QueryPlan](https://pkg.go.dev/cloud.google.com/go/spanner#RowIterator)
* [ResultSetStats](https://cloud.google.com/spanner/docs/reference/rest/v1/ResultSetStats?hl=en)
    * Output of DOWNLOAD JSON in [the official query plan visualizer](https://cloud.google.com/spanner/docs/tune-query-with-visualizer?hl=en)
* [ResultSet](https://cloud.google.com/spanner/docs/reference/rest/v1/ResultSet?hl=en)
    * Output of `gcloud spanner databases execute-sql` and [execspansql](https://github.com/apstndb/execspansql)

### PLAN

```
$ gcloud spanner databases execute-sql --instance=sampleinstance sampledb --query-mode=PLAN --format=yaml \
  --sql="SELECT SongName FROM Songs WHERE STARTS_WITH(SongName, @prefix)" |
  spannerplanviz --full --type=svg --output plan.svg
```

### PROFILE

You see verbose profile information. (Currently, `histogram` is not shown.)

```
$ gcloud spanner databases execute-sql --instance=sampleinstance sampledb --query-mode=PROFILE --format=yaml \
  --sql "SELECT * FROM Singers JOIN Songs USING(SingerId) WHERE SongName LIKE 'Th%e'" |
  spannerplanviz --full --type=svg --output profile.svg
```

![full profile](docs/dca_full.png)

You can emit Mermaid.js using `--type mermaid` (EXPERIMENTAL).

```
spannerplanviz --full --type=mermaid --output profile.mermaid < dca_profile.json
```

The generated source uses HTML labels and a browser-friendly init block (`htmlLabels: true`, `useMaxWidth: false`). See `visualize/testdata/dca_profile.golden.mermaid` for a full example output.

## Library usage

Build a diagram model once, then render with the backend of your choice:

```go
stats, rowType, err := spannerplan.ExtractQueryPlan(input)
plan, err := visualize.BuildPlan(rowType, stats, visualize.StructureBuildOptions())
src, err := mermaid.Source(plan)
```

Presets:

- `visualize.StructureBuildOptions()` — operator structure for interactive viewers (lighter than `--full`)
- `visualize.FullBuildOptions()` — same detail level as CLI `--full`

Renderers:

- `mermaid.Source(plan)` — Mermaid.js source using `plan.Build`
- `mermaid.SourceWithOptions(plan, opts)` — override `plan.Build` at render time (including disabling flags)
- `mermaid.NewRenderer(opts).Render(ctx, w, plan)` — streaming render
- `graphviz.NewRenderer(opts).Render(ctx, w, plan)` — SVG/PNG/DOT via Graphviz

## Browser embedding

When rendering Mermaid in the browser (for example from Go WASM):

1. Call `visualize.BuildPlan` and `mermaid.Source(plan)`.
2. Pass the returned source to [mermaid.js](https://mermaid.js.org/) `render()`.
3. The source includes a `%%{ init: ... }%%` block with `htmlLabels: true` and `useMaxWidth: false`. Keep your global `mermaid.initialize()` consistent with those settings if you set defaults separately.
4. Prefer `StructureBuildOptions()` for large plans; `--full` output can be slow to lay out in the browser.

`BuildOptions` fields mirror CLI flags (`metadata`, `execution-stats`, `hide-metadata`, and so on). See `option.Options.BuildOptions()` for the full mapping.

## Stability

- The `spannerplanviz` CLI flags and behavior are treated as stable.
- The Go library API (`visualize.BuildPlan`, `mermaid.Source`, `graphviz.NewRenderer`, and related types) is experimental and may change between releases.
- This module follows v0 semver: breaking changes may appear in minor releases. See GitHub release notes for details.
- Text plan rendering moved to [`spannerplan/cmd/rendertree`](https://github.com/apstndb/spannerplan/tree/main/cmd/rendertree); the deprecated shim in this repository has been removed.

## Disclaimer

This tool is Alpha quality.