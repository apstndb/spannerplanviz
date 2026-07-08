package d2_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/apstndb/spannerplanviz/d2"
	"github.com/apstndb/spannerplanviz/visualize"
)

func testdataPath(name string) string {
	return filepath.Join("..", "visualize", "testdata", name)
}

func buildPlanFromFixture(t *testing.T, name string, opts visualize.BuildOptions) *visualize.Plan {
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

	got, err := d2.Source(plan)
	if err != nil {
		t.Fatalf("Source() error = %v", err)
	}

	for _, want := range []string{
		"direction: down",
		"node0.shape: rectangle",
		"node0.label: |md",
		"**Union**",
		"node0 -> node1: Input",
		"node0 -> node2: Input",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Source() output missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "query.shape") {
		t.Errorf("Source() must not emit a query node by default:\n%s", got)
	}
}

func TestSource_nilPlan(t *testing.T) {
	if _, err := d2.Source(nil); err == nil {
		t.Error("Source(nil) expected error, got nil")
	}
	if _, err := d2.Source(&visualize.Plan{}); err == nil {
		t.Error("Source(&Plan{}) expected error, got nil")
	}
}

func TestRenderer_contextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var buf bytes.Buffer
	err := d2.NewRenderer(d2.Options{}).Render(ctx, &buf, nil)
	if err == nil {
		t.Error("Render() with canceled context expected error, got nil")
	}
}

func TestSource_goldenDCAProfile(t *testing.T) {
	opts := visualize.BuildOptions{Full: true}
	opts.ApplyFull()
	plan := buildPlanFromFixture(t, "dca_profile.json", opts)

	got, err := d2.SourceWithOptions(plan, d2.Options{
		BuildOptions:   opts,
		ShowQuery:      true,
		ShowQueryStats: true,
	})
	if err != nil {
		t.Fatalf("SourceWithOptions() error = %v", err)
	}

	goldenPath := testdataPath("dca_profile.golden.d2")
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
		t.Errorf("D2 mismatch (-expected +actual):\n%s", diff)
	}

	validateWithD2CLI(t, got)
}

// TestRenderer_specialCharacters renders a plan whose labels, tooltips and query
// stats contain quotes, backslashes, pipes and angle brackets, and asserts the
// D2 source is produced without panicking and parses with the d2 CLI when it is
// available on PATH.
func TestRenderer_specialCharacters(t *testing.T) {
	opts := visualize.BuildOptions{Full: true}
	opts.ApplyFull()
	plan := buildPlanFromFixture(t, "various_characters_profile.json", opts)

	got, err := d2.SourceWithOptions(plan, d2.Options{
		BuildOptions:   opts,
		ShowQuery:      true,
		ShowQueryStats: true,
	})
	if err != nil {
		t.Fatalf("SourceWithOptions() error = %v", err)
	}

	for _, want := range []string{
		"direction: down",
		"node0.shape: rectangle",
		"node0.tooltip: |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Source() output missing %q", want)
		}
	}

	validateWithD2CLI(t, got)
}

// validateWithD2CLI writes src to a temp .d2 file and runs the d2 CLI on it,
// which parses, compiles and lays out the diagram. It skips when the d2 binary
// is not on PATH. The plain `d2 <input> <output>` form is the version-stable
// core invocation.
func validateWithD2CLI(t *testing.T, src string) {
	t.Helper()

	bin, err := exec.LookPath("d2")
	if err != nil {
		t.Skip("d2 CLI not found on PATH; skipping D2 validation")
	}

	dir := t.TempDir()
	in := filepath.Join(dir, "diagram.d2")
	out := filepath.Join(dir, "diagram.svg")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		t.Fatalf("write temp d2 file: %v", err)
	}

	cmd := exec.Command(bin, in, out)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("d2 failed to validate generated source: %v\n%s", err, output)
	}
}
