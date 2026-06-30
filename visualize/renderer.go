package visualize

import (
	"context"
	"io"
)

// Renderer renders a built plan to an output format.
type Renderer interface {
	Render(ctx context.Context, w io.Writer, plan *Plan) error
}

// RendererFunc adapts a function to Renderer.
type RendererFunc func(context.Context, io.Writer, *Plan) error

// Render implements Renderer.
func (f RendererFunc) Render(ctx context.Context, w io.Writer, plan *Plan) error {
	return f(ctx, w, plan)
}
