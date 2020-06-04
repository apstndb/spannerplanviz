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
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/apstndb/spannerplanviz/protoyaml"
	"github.com/goccy/go-graphviz"
	pb "google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

type visualizeParam struct {
	showQuery        bool
	showQueryStats   bool
	nonVariableScala bool
	variableScalar   bool
	metadata         bool
	executionStats   bool
	executionSummary bool
	serializeResult  bool
	hideScanTarget   bool
	hideMetadata     []string
}

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

	b, err := ioutil.ReadAll(input)
	if err != nil {
		return err
	}

	queryStats, rowType, err := extractStatsAndRowType(b)
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

	var param visualizeParam
	if *full {
		param = visualizeParam{*showQuery, *showQueryStats, true, true, true, true, true, true, false, hideMetadata}
	} else {
		param = visualizeParam{
			showQuery:        *showQuery,
			showQueryStats:   *showQueryStats,
			nonVariableScala: *nonVariableScalar,
			variableScalar:   *variableScalar,
			metadata:         *metadata,
			executionStats:   *executionStats,
			executionSummary: *executionSummary,
			serializeResult:  *serializeResult,
			hideScanTarget:   *hideScanTarget,
			hideMetadata:     hideMetadata,
		}
	}
	err = renderImage(rowType, queryStats, graphviz.Format(*typeFlag), writer, param)
	if err != nil {
		os.Remove(*filename)
	}
	return err
}

func extractStatsAndRowType(b []byte) (stats *pb.ResultSetStats, rowType *pb.StructType, err error) {
	// Parse ResultSet(ignore error).
	{
		var result pb.ResultSet
		if err := protoyaml.Unmarshal(b, &result); err == nil {
			return result.GetStats(), result.GetMetadata().GetRowType(), nil
		}
	}
	// Parse jsonpb of []PartialResultSet from Cloud Spanner console.
	// Only the last PartialResultSet contains stats.
	j, err := takeLastElemJson(b)
	if err != nil {
		return nil, nil, err
	}
	var partialResultSet pb.PartialResultSet
	if err := protojson.Unmarshal(j, &partialResultSet); err != nil {
		return nil, nil, err
	}
	return partialResultSet.GetStats(), partialResultSet.GetMetadata().GetRowType(), nil
}

func takeLastElemJson(input []byte) ([]byte, error) {
	var jsonArray []json.RawMessage
	if err := json.Unmarshal(input, &jsonArray); err != nil {
		return nil, err
	}

	return jsonArray[len(jsonArray)-1], nil
}
