package graphviz

// Format is a Graphviz output format.
type Format string

const (
	SVG Format = "svg"
	PNG Format = "png"
	DOT Format = "dot"
)

// Options configures Graphviz rendering.
type Options struct {
	// Format is required (SVG, PNG, or DOT). The CLI always sets this from --type.
	Format         Format
	ShowQuery      bool
	ShowQueryStats bool
}

// Renderer renders a built plan with Graphviz.
type Renderer struct {
	Options Options
}

// NewRenderer returns a Graphviz renderer.
func NewRenderer(opts Options) *Renderer {
	return &Renderer{Options: opts}
}
