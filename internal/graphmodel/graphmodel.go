// Package graphmodel is the backend-neutral intermediate representation of a
// rendered query plan graph. It carries the fully resolved graph scope (nodes,
// edges, labels, tooltips and the optional query text) so that each backend
// serializer (dot, mermaid, d2, ...) can walk a single shared model instead of
// traversing the visualize.TreeNode graph and resolving formatting concerns on
// its own.
//
// It is the graph-scope analog of the per-node label IR: the model owns
// structure and semantics, and the serializers own only traversal order,
// markup and escaping. The model itself is free of any backend syntax.
package graphmodel

// KV is a key/value pair, used for metadata pairs, execution-stats pairs and
// query-stat pairs.
type KV struct {
	Key   string
	Value string
}

// Item is one line of a Label body: either free text (a content line such as
// the short representation, a scan-info line or a scalar-link line) or a
// metadata key/value pair. KV is nil for text items and set for pairs.
type Item struct {
	Text string
	KV   *KV
}

// Label is a backend-neutral representation of a node's label. The sections
// carry semantic style: Title is bold, Body is plain, and Stats/Summary are
// italic. Serializers render the sections in field order to reproduce the
// per-backend layout.
//
// Title holds the node title (already escaped for the dominant backend at build
// time). The dot HTML path composes Title separately with CENTER alignment; the
// dot body path deliberately omits it. The mermaid path places Title inline at
// the top of the label.
type Label struct {
	Title   string
	Body    []Item
	Stats   []KV
	Summary []string
}

// EdgeStyle describes how an edge between plan nodes should be drawn.
type EdgeStyle int

const (
	// EdgeStyleSolid is a solid line (the default, local links).
	EdgeStyleSolid EdgeStyle = iota
	// EdgeStyleDashed is a dashed line (possible remote calls).
	EdgeStyleDashed
	// EdgeStyleDotted is a dotted line.
	EdgeStyleDotted
)

// Node is a graph node with its resolved label and tooltip.
//
// Nodes can be shared: the same *Node may appear as the target of edges from
// multiple parents. Serializers that dedup (for example the mermaid path) use
// ID as the identity key; serializers that emit a full traversal (for example
// the dot path) walk Children directly.
type Node struct {
	ID       string
	Label    Label
	Tooltip  string
	Children []Edge
}

// Edge is a link from a parent Node to a child Node.
type Edge struct {
	Style EdgeStyle
	Label string
	To    *Node
}

// QueryText is the optional query-text node's content. Stats is sorted by key
// and is empty unless query stats were requested.
type QueryText struct {
	Text  string
	Stats []KV
}

// Graph is the whole resolved plan graph.
type Graph struct {
	Root  *Node
	Query *QueryText
}
