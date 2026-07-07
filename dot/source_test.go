package dot_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/goccy/go-graphviz/cgraph"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/apstndb/spannerplanviz/dot"
	"github.com/apstndb/spannerplanviz/graphviz"
	"github.com/apstndb/spannerplanviz/visualize"
)

func testdataPath(name string) string {
	return filepath.Join("..", "visualize", "testdata", name)
}

func buildPlanFromFixture(t *testing.T, name string) *visualize.Plan {
	t.Helper()

	jsonBytes, err := os.ReadFile(testdataPath(name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	var resultSet sppb.ResultSet
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshalOpts.Unmarshal(jsonBytes, &resultSet); err != nil {
		t.Fatalf("unmarshal %s: %v", name, err)
	}

	opts := visualize.BuildOptions{Full: true}
	opts.ApplyFull()

	plan, err := visualize.BuildPlan(resultSet.GetMetadata().GetRowType(), resultSet.GetStats(), opts)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	return plan
}

func TestSource_simplePlan(t *testing.T) {
	stats := &sppb.ResultSetStats{
		QueryPlan: &sppb.QueryPlan{
			PlanNodes: []*sppb.PlanNode{
				{
					Index:       0,
					DisplayName: "Union",
					Kind:        sppb.PlanNode_RELATIONAL,
					ChildLinks: []*sppb.PlanNode_ChildLink{
						{ChildIndex: 1, Type: "Input"},
						{ChildIndex: 2, Type: "Input"},
					},
					Metadata: &structpb.Struct{},
				},
				{Index: 1, DisplayName: "Scan1", Kind: sppb.PlanNode_RELATIONAL, Metadata: &structpb.Struct{}},
				{Index: 2, DisplayName: "Scan2", Kind: sppb.PlanNode_RELATIONAL, Metadata: &structpb.Struct{}},
			},
		},
	}

	plan, err := visualize.BuildPlan(nil, stats, visualize.BuildOptions{})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	got, err := dot.Source(plan)
	if err != nil {
		t.Fatalf("Source() error = %v", err)
	}

	for _, want := range []string{
		"digraph {",
		`graph [fontname="Times New Roman:style=Bold", rankdir="BT", start="regular"];`,
		`"node0" [label=<<b>Union</b>>, shape="box", tooltip="`,
		`"node1" -> "node0" [label="Input", style="solid"];`,
		`"node2" -> "node0" [label="Input", style="solid"];`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Source() output missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"query"`) {
		t.Errorf("Source() must not emit a query node by default:\n%s", got)
	}
}

func TestSource_nilPlan(t *testing.T) {
	if _, err := dot.Source(nil); err == nil {
		t.Error("Source(nil) expected error, got nil")
	}
	if _, err := dot.Source(&visualize.Plan{}); err == nil {
		t.Error("Source(&Plan{}) expected error, got nil")
	}
}

func TestRenderer_contextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var buf bytes.Buffer
	err := dot.NewRenderer(dot.Options{}).Render(ctx, &buf, nil)
	if err == nil {
		t.Error("Render() with canceled context expected error, got nil")
	}
}

func TestSource_goldenDCAProfile(t *testing.T) {
	plan := buildPlanFromFixture(t, "dca_profile.json")

	got, err := dot.SourceWithOptions(plan, dot.Options{ShowQuery: true, ShowQueryStats: true})
	if err != nil {
		t.Fatalf("SourceWithOptions() error = %v", err)
	}

	goldenPath := testdataPath("dca_profile.golden.dot")
	if os.Getenv("UPDATE_GOLDEN_FILES") == "true" {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
		t.Fatal("golden file updated")
	}

	expectedBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}

	if diff := cmp.Diff(strings.TrimSpace(string(expectedBytes)), strings.TrimSpace(got)); diff != "" {
		t.Errorf("DOT mismatch (-expected +actual):\n%s", diff)
	}
}

// TestSource_parityWithGraphvizRenderer is the conformance test for this
// package: the DOT source must describe the same graph as the graphviz
// package's DOT output. Both outputs are parsed with Graphviz itself and
// compared structurally (semantic attributes only — the graphviz package's
// output additionally contains layout attributes such as pos/bb, which have
// no counterpart in unlaid-out source).
func TestSource_parityWithGraphvizRenderer(t *testing.T) {
	for _, tt := range []struct {
		fixture string
		opts    dot.Options
	}{
		{"dca_profile.json", dot.Options{}},
		{"dca_profile.json", dot.Options{ShowQuery: true, ShowQueryStats: true}},
		{"various_characters_profile.json", dot.Options{}},
		{"various_characters_profile.json", dot.Options{ShowQuery: true, ShowQueryStats: true}},
	} {
		name := fmt.Sprintf("%s/query=%v/stats=%v", tt.fixture, tt.opts.ShowQuery, tt.opts.ShowQueryStats)
		t.Run(name, func(t *testing.T) {
			plan := buildPlanFromFixture(t, tt.fixture)

			source, err := dot.SourceWithOptions(plan, tt.opts)
			if err != nil {
				t.Fatalf("SourceWithOptions() error = %v", err)
			}

			var buf bytes.Buffer
			renderer := graphviz.NewRenderer(graphviz.Options{
				Format:         graphviz.DOT,
				ShowQuery:      tt.opts.ShowQuery,
				ShowQueryStats: tt.opts.ShowQueryStats,
			})
			if err := renderer.Render(context.Background(), &buf, plan); err != nil {
				t.Fatalf("graphviz Render() error = %v", err)
			}

			gotGraph := parseGraph(t, []byte(source))
			wantGraph := parseGraph(t, buf.Bytes())

			if diff := cmp.Diff(wantGraph, gotGraph); diff != "" {
				t.Errorf("graph structure mismatch (-graphviz +dot):\n%s", diff)
			}
		})
	}
}

// graphRepr is a renderer-independent description of the semantic content of
// a parsed DOT graph.
type graphRepr struct {
	GraphAttrs map[string]string
	Nodes      map[string]map[string]string
	Edges      []map[string]string
}

var (
	graphAttrKeys = []string{"fontname", "rankdir", "start"}
	nodeAttrKeys  = []string{"label", "shape", "style", "tooltip"}
	edgeAttrKeys  = []string{"label", "style"}
)

func parseGraph(t *testing.T, dotBytes []byte) graphRepr {
	t.Helper()

	g, err := cgraph.ParseBytes(dotBytes)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	defer func() {
		if err := g.Close(); err != nil {
			t.Errorf("close parsed graph: %v", err)
		}
	}()

	repr := graphRepr{
		GraphAttrs: map[string]string{},
		Nodes:      map[string]map[string]string{},
	}
	for _, key := range graphAttrKeys {
		repr.GraphAttrs[key] = g.GetStr(key)
	}

	for n, err := g.FirstNode(); n != nil; n, err = g.NextNode(n) {
		if err != nil {
			t.Fatalf("iterate nodes: %v", err)
		}

		name, err := n.Name()
		if err != nil {
			t.Fatalf("node name: %v", err)
		}

		attrs := map[string]string{}
		for _, key := range nodeAttrKeys {
			attrs[key] = n.GetStr(key)
		}
		repr.Nodes[name] = attrs

		for e, err := g.FirstOut(n); e != nil; e, err = g.NextOut(e) {
			if err != nil {
				t.Fatalf("iterate out-edges of %s: %v", name, err)
			}

			head, err := e.Head()
			if err != nil {
				t.Fatalf("edge head: %v", err)
			}
			headName, err := head.Name()
			if err != nil {
				t.Fatalf("edge head name: %v", err)
			}

			edgeAttrs := map[string]string{"tail": name, "head": headName}
			for _, key := range edgeAttrKeys {
				edgeAttrs[key] = e.GetStr(key)
			}
			repr.Edges = append(repr.Edges, edgeAttrs)
		}
	}

	sort.Slice(repr.Edges, func(i, j int) bool {
		a, b := repr.Edges[i], repr.Edges[j]
		if a["tail"] != b["tail"] {
			return a["tail"] < b["tail"]
		}
		if a["head"] != b["head"] {
			return a["head"] < b["head"]
		}
		return a["label"] < b["label"]
	})
	return repr
}
