# AGENTS.md

This file provides guidance to Codex and other coding agents when working with code in this repository.

## Project Overview

spannerplanviz is a Cloud Spanner Query Plan Visualizer that converts Spanner query plans into visual diagrams using Graphviz and Mermaid.js. The tool reads JSON/YAML input containing Spanner query plans and generates visual output in various formats (SVG, PNG, DOT, Mermaid).

## Architecture

### Core Components

- **main.go**: Entry point that parses CLI options, reads input, and orchestrates visualization
- **option/options.go**: Defines CLI flags and options for controlling output format and content
- **visualize/**: Core visualization logic
  - **visualize.go**: Main rendering coordinator, handles both Graphviz and Mermaid output paths
  - **mermaid.go**: Mermaid.js-specific rendering logic with structured configuration
  - **build_tree.go**: Converts Spanner plan nodes into internal tree structure
  - **util.go**: Utility functions for formatting and text processing

### Dependencies

- **github.com/apstndb/spannerplan**: External library for parsing Spanner query plans
- **github.com/goccy/go-graphviz**: Graphviz rendering engine
- **cloud.google.com/go/spanner**: Google Cloud Spanner client library for protobuf definitions
- **github.com/jessevdk/go-flags**: CLI argument parsing

### Input/Output Flow

1. Input: JSON/YAML containing QueryPlan, ResultSetStats, or ResultSet from Spanner
2. Parse using spannerplan.ExtractQueryPlan()
3. Build internal tree structure via buildTree()
4. Render output based on --type flag (svg/png/dot/mermaid)

## Development Commands

### Testing

```bash
make test
# Or directly:
go test -v ./...

# Test with sample data:
go run . --type=mermaid --full < visualize/testdata/dca_profile.json
go run . --type=svg --full --output=test.svg < visualize/testdata/dca_profile.json
```

### Building

```bash
go build -o spannerplanviz .
```

### Running

```bash
# Basic usage - reads from stdin, outputs to stdout
echo '{"queryPlan": {...}}' | go run . --type=svg

# With file input/output
go run . --input=plan.json --output=plan.svg --type=svg --full
```

### CLI Options

- `--type`: Output format (svg, png, dot, mermaid)
- `--full`: Enable all metadata options (execution-stats, metadata, etc.)
- `--output`: Output file path
- `--show-query`: Include query text in visualization
- `--show-query-stats`: Include query statistics

### Test Data

Sample files in `visualize/testdata/` for testing:

- `dca_profile.json`: Complex distributed cross apply query with profile data
- `various_characters_profile.json`: Tests special character handling
- `*.golden.mermaid`: Expected Mermaid output for regression testing

## Code Conventions

- Standard Go formatting with gofmt
- Follow existing tree node helper methods for node operations, such as GetName(), HTML(), and MermaidLabel()
- Wrap errors with fmt.Errorf() and %w when preserving the underlying cause
- Defer cleanup for resources (files, graphviz objects)
- Use cgraph.EdgeStyle constants for edge styling
- Mermaid output uses structured JSON configuration for theming
