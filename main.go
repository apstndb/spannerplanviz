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
	"fmt"
	"github.com/apstndb/spannerplanviz/option"
	"github.com/jessevdk/go-flags"
	"io"
	"log"
	"os"
	"strings"

	"github.com/apstndb/spannerplanviz/queryplan"
	"github.com/apstndb/spannerplanviz/visualize"
	"github.com/goccy/go-graphviz"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalln(err)
	}
}

type commaSeparated []string

func (cs *commaSeparated) Set(s string) error {
	*cs = append(*cs, strings.Split(s, ",")...)
	return nil
}
func (cs *commaSeparated) String() string {
	return fmt.Sprint([]string(*cs))
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

	var input io.ReadCloser
	if opts.Positional.Input != "" {
		if file, err := os.Open(opts.Positional.Input); err != nil {
			return err
		} else {
			input = file
		}
	} else {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			p.WriteHelp(os.Stderr)
			os.Exit(1)
		}
		input = os.Stdin
	}
	defer input.Close()

	b, err := io.ReadAll(input)
	if err != nil {
		return err
	}

	queryStats, rowType, err := queryplan.ExtractQueryPlan(b)
	if err != nil {
		return err
	}

	var writer io.WriteCloser
	if opts.Filename == "" {
		writer = os.Stdout
	} else if file, err := os.Create(opts.Filename); err != nil {
		return err
	} else {
		writer = file
	}
	defer writer.Close()

	if opts.Full {
		opts.NonVariableScalar = true
		opts.VariableScalar = true
		opts.Metadata = true
		opts.ExecutionStats = true
		opts.ExecutionSummary = true
		opts.SerializeResult = true
	}

	err = visualize.RenderImage(ctx, rowType, queryStats, graphviz.Format(opts.TypeFlag), writer, opts)
	if err != nil {
		os.Remove(opts.Filename)
	}
	return err
}
