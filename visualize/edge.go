package visualize

// EdgeStyle describes how an edge between plan nodes should be drawn.
type EdgeStyle int

const (
	EdgeStyleSolid EdgeStyle = iota
	EdgeStyleDashed
	EdgeStyleDotted
)
