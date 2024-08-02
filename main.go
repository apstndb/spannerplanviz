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
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/apstndb/spannerplanviz/queryplan"
	"github.com/apstndb/spannerplanviz/visualize"
	"github.com/goccy/go-graphviz"
)

func main() {
	if err := _main(); err != nil {
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

func _main() error {
	var (
		typeFlag          = flag.String("type", "", "output type [svg, dot]")
		filename          = flag.String("output", "", "")
		nonVariableScalar = flag.Bool("non-variable-scalar", false, "")
		variableScalar    = flag.Bool("variable-scalar", false, "")
		metadata          = flag.Bool("metadata", false, "")
		executionStats    = flag.Bool("execution-stats", false, "")
		executionSummary  = flag.Bool("execution-summary", false, "")
		serializeResult   = flag.Bool("serialize-result", false, "")
		hideScanTarget    = flag.Bool("hide-scan-target", false, "")
		showQuery         = flag.Bool("show-query", false, "")
		showQueryStats    = flag.Bool("show-query-stats", false, "")
		full              = flag.Bool("full", false, "full output")
		hideMetadata      commaSeparated
	)

	flag.Var(&hideMetadata, "hide-metadata", "")
	flag.Parse()

	var input io.ReadCloser
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(1)
	} else if flag.NArg() == 1 {
		if file, err := os.Open(flag.Arg(0)); err != nil {
			return err
		} else {
			input = file
		}
	} else {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			input = os.Stdin
		} else {
			flag.Usage()
			os.Exit(1)
		}
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
	if *filename == "" {
		writer = os.Stdout
	} else if file, err := os.Create(*filename); err == nil {
		writer = file

	} else {
		return err
	}
	defer writer.Close()

	var param visualize.VisualizeParam
	if *full {
		param = visualize.VisualizeParam{
			ShowQuery:        *showQuery,
			ShowQueryStats:   *showQueryStats,
			NonVariableScala: true,
			VariableScalar:   true,
			Metadata:         true,
			ExecutionStats:   true,
			ExecutionSummary: true,
			SerializeResult:  true,
			HideMetadata:     hideMetadata,
		}
	} else {
		param = visualize.VisualizeParam{
			ShowQuery:        *showQuery,
			ShowQueryStats:   *showQueryStats,
			NonVariableScala: *nonVariableScalar,
			VariableScalar:   *variableScalar,
			Metadata:         *metadata,
			ExecutionStats:   *executionStats,
			ExecutionSummary: *executionSummary,
			SerializeResult:  *serializeResult,
			HideScanTarget:   *hideScanTarget,
			HideMetadata:     hideMetadata,
		}
	}
	err = visualize.RenderImage(rowType, queryStats, graphviz.Format(*typeFlag), writer, param)
	if err != nil {
		os.Remove(*filename)
	}
	return err
}
