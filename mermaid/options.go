package mermaid

import "github.com/apstndb/spannerplanviz/visualize"

// Options configures Mermaid source generation.
type Options struct {
	visualize.BuildOptions
	// ShowQuery adds a query-text node linked from the root with an unlabeled
	// solid edge. Defaults to false so existing output is unchanged.
	ShowQuery bool
	// ShowQueryStats adds the query statistics to the query-text node.
	ShowQueryStats bool
}
