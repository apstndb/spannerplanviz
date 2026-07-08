package graphviz

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/apstndb/spannerplanviz/dot"
	"github.com/apstndb/spannerplanviz/visualize"
	"github.com/goccy/go-graphviz"
)

// Render writes a Graphviz diagram for plan to w.
func (r *Renderer) Render(ctx context.Context, w io.Writer, plan *visualize.Plan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if plan == nil || plan.Root == nil {
		return fmt.Errorf("cannot render graphviz: plan is nil")
	}
	if r.Options.Format == "" {
		return fmt.Errorf("graphviz format is required")
	}
	return render(ctx, w, r.Options.Format, plan, r.Options)
}

// render lays out and renders plan by generating DOT source with the dot
// package (the single source of truth for graph construction) and handing it to
// the Graphviz runtime. Graph attributes (fontname, rankdir, start) are already
// embedded in the DOT source, so they are not re-applied on the parsed graph.
func render(ctx context.Context, w io.Writer, format Format, plan *visualize.Plan, opts Options) error {
	source, err := dot.SourceWithOptions(plan, dot.Options{
		ShowQuery:      opts.ShowQuery,
		ShowQueryStats: opts.ShowQueryStats,
	})
	if err != nil {
		return fmt.Errorf("failed to generate DOT source: %w", err)
	}

	g, err := graphviz.New(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := g.Close(); err != nil {
			log.Print(err)
		}
	}()

	graph, err := graphviz.ParseBytes([]byte(source))
	if err != nil {
		return fmt.Errorf("failed to parse DOT source: %w", err)
	}
	defer func() {
		if err := graph.Close(); err != nil {
			log.Print(err)
		}
	}()

	return g.Render(ctx, graph, graphviz.Format(format), w)
}
