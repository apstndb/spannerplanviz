//
// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package main

import (
	"context"
	"errors"
	"io"
	"log"
	"os"

	"github.com/apstndb/spannerplan"
	"github.com/jessevdk/go-flags"

	"github.com/apstndb/spannerplanviz/d2"
	"github.com/apstndb/spannerplanviz/dot"
	"github.com/apstndb/spannerplanviz/graphviz"
	"github.com/apstndb/spannerplanviz/mermaid"
	"github.com/apstndb/spannerplanviz/option"
	"github.com/apstndb/spannerplanviz/visualize"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalln(err)
	}
}

func run(ctx context.Context) error {
	var opts option.Options
	p := flags.NewParser(&opts, flags.Default)
	args, err := p.Parse()
	if err != nil {
		return err
	}

	if len(args) > 0 {
		p.WriteHelp(os.Stderr)
		os.Exit(1)
	}

	if err := opts.Normalize(); err != nil {
		return err
	}

	var input io.ReadCloser
	if opts.Positional.Input != "" {
		file, err := os.Open(opts.Positional.Input)
		if err != nil {
			return err
		}
		input = file
	} else {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			p.WriteHelp(os.Stderr)
			os.Exit(1)
		}
		input = os.Stdin
	}
	defer func() {
		_ = input.Close()
	}()

	b, err := io.ReadAll(input)
	if err != nil {
		return err
	}

	queryStats, rowType, err := spannerplan.ExtractQueryPlan(b)
	if err != nil {
		return err
	}

	var writer io.WriteCloser
	needsClose := false
	if opts.Filename == "" {
		writer = os.Stdout
	} else if file, err := os.Create(opts.Filename); err != nil {
		return err
	} else {
		writer = file
		needsClose = true
	}
	defer func() {
		if needsClose {
			_ = writer.Close()
		}
	}()

	plan, err := visualize.BuildPlan(rowType, queryStats, opts.BuildOptions())
	if err != nil {
		if opts.Filename != "" && needsClose {
			needsClose = false
			_ = writer.Close()
			if innerErr := os.Remove(opts.Filename); innerErr != nil && !os.IsNotExist(innerErr) {
				return errors.Join(err, innerErr)
			}
		}
		return err
	}

	err = render(ctx, writer, plan, opts)
	if err != nil {
		if opts.Filename != "" {
			needsClose = false
			_ = writer.Close()
			if innerErr := os.Remove(opts.Filename); innerErr != nil && !os.IsNotExist(innerErr) {
				return errors.Join(err, innerErr)
			}
		}
		return err
	}
	return nil
}

func render(ctx context.Context, w io.Writer, plan *visualize.Plan, opts option.Options) error {
	switch opts.TypeFlag {
	case "mermaid":
		return mermaid.NewRenderer(mermaid.Options{
			BuildOptions:   plan.Build,
			ShowQuery:      opts.ShowQuery,
			ShowQueryStats: opts.ShowQueryStats,
		}).Render(ctx, w, plan)
	case "d2":
		return d2.NewRenderer(d2.Options{
			BuildOptions:   plan.Build,
			ShowQuery:      opts.ShowQuery,
			ShowQueryStats: opts.ShowQueryStats,
		}).Render(ctx, w, plan)
	case "dot":
		// --type dot emits pure DOT source via the dot package (the single
		// source of truth for graph construction) rather than routing through
		// the Graphviz runtime, so it is unlaid-out source with no layout
		// attributes (pos/bb/etc.).
		return dot.NewRenderer(dot.Options{
			ShowQuery:      opts.ShowQuery,
			ShowQueryStats: opts.ShowQueryStats,
		}).Render(ctx, w, plan)
	case "svg", "png":
		return graphviz.NewRenderer(graphviz.Options{
			Format:         graphviz.Format(opts.TypeFlag),
			ShowQuery:      opts.ShowQuery,
			ShowQueryStats: opts.ShowQueryStats,
		}).Render(ctx, w, plan)
	default:
		return errors.New("unsupported output type")
	}
}
