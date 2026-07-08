package d2

import "github.com/apstndb/spannerplanviz/visualize"

// Options configures D2 source generation.
type Options struct {
	// BuildOptions controls which plan details are included in node labels.
	BuildOptions visualize.BuildOptions
	// ShowQuery adds a query-text node linked from the root.
	ShowQuery bool
	// ShowQueryStats adds the query statistics to the query-text node.
	ShowQueryStats bool
}
