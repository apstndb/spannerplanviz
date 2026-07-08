package dot_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/apstndb/spannerplanviz/dot"
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

// The DOT source this package emits is exercised end-to-end by the graphviz
// package, which now generates it and hands it to the Graphviz runtime for
// svg/png rendering. graphviz.TestRenderer_specialCharacters covers the
// escaping/quoting boundary that the former parity test policed here.
