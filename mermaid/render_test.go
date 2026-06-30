package mermaid_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/MakeNowJust/heredoc/v2"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/apstndb/spannerplanviz/mermaid"
	"github.com/apstndb/spannerplanviz/visualize"
)

func testdataPath(name string) string {
	return filepath.Join("..", "visualize", "testdata", name)
}

func TestRenderer_simplePlan(t *testing.T) {
	node2Stats, _ := structpb.NewStruct(map[string]interface{}{
		"rows":    map[string]interface{}{"total": "10", "unit": "rows"},
		"latency": map[string]interface{}{"total": "1ms"},
	})
	node1Stats, _ := structpb.NewStruct(map[string]interface{}{
		"rows":    map[string]interface{}{"total": "20", "unit": "rows"},
		"latency": map[string]interface{}{"total": "2ms"},
	})
	node0Stats, _ := structpb.NewStruct(map[string]interface{}{
		"rows":    map[string]interface{}{"total": "20", "unit": "rows"},
		"latency": map[string]interface{}{"total": "3ms"},
	})

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
					ExecutionStats: node0Stats,
					Metadata:       &structpb.Struct{},
				},
				{
					Index:          1,
					DisplayName:    "Scan1",
					Kind:           sppb.PlanNode_RELATIONAL,
					ExecutionStats: node1Stats,
					Metadata:       &structpb.Struct{},
				},
				{
					Index:          2,
					DisplayName:    "Scan2",
					Kind:           sppb.PlanNode_RELATIONAL,
					ExecutionStats: node2Stats,
					Metadata:       &structpb.Struct{},
				},
			},
		},
		QueryStats: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"query_text": structpb.NewStringValue("SELECT 1"),
			},
		},
	}

	opts := visualize.BuildOptions{Full: true}
	opts.ApplyFull()

	plan, err := visualize.BuildPlan(nil, stats, opts)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	var buf bytes.Buffer
	err = mermaid.NewRenderer(mermaid.Options{BuildOptions: opts}).Render(context.Background(), &buf, plan)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	expectedMermaidOutput := heredoc.Doc(`
%%{ init: {"flowchart":{"curve":"linear","markdownAutoWrap":false,"useMaxWidth":false,"wrappingWidth":2000},"htmlLabels":true,"themeVariables":{"wrap":false}} }%%
graph TD
    node0["<b>Union</b>
<i>latency: 3ms</i>
<i>rows: 20&nbsp;rows</i>"]
    style node0 text-align:left;
    node1["<b>Scan1</b>
<i>latency: 2ms</i>
<i>rows: 20&nbsp;rows</i>"]
    style node1 text-align:left;
    node2["<b>Scan2</b>
<i>latency: 1ms</i>
<i>rows: 10&nbsp;rows</i>"]
    style node2 text-align:left;
    node0 -->|Input| node1
    node0 -->|Input| node2
`,
	)

	if diff := cmp.Diff(expectedMermaidOutput, buf.String()); diff != "" {
		t.Errorf("Mermaid output mismatch (-expected +actual):\n%s", diff)
	}
}

func TestRenderer_goldenDCAProfile(t *testing.T) {
	jsonBytes, err := os.ReadFile(testdataPath("dca_profile.json"))
	if err != nil {
		t.Fatalf("read dca_profile.json: %v", err)
	}

	var resultSet sppb.ResultSet
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshalOpts.Unmarshal(jsonBytes, &resultSet); err != nil {
		t.Fatalf("unmarshal dca_profile.json: %v", err)
	}

	opts := visualize.BuildOptions{Full: true}
	opts.ApplyFull()

	plan, err := visualize.BuildPlan(resultSet.GetMetadata().GetRowType(), resultSet.GetStats(), opts)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	var buf bytes.Buffer
	if err := mermaid.NewRenderer(mermaid.Options{BuildOptions: opts}).Render(context.Background(), &buf, plan); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	goldenMermaidPath := testdataPath("dca_profile.golden.mermaid")
	if os.Getenv("UPDATE_GOLDEN_FILES") == "true" {
		if err := os.WriteFile(goldenMermaidPath, buf.Bytes(), 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
		t.Fatal("golden file updated")
	}

	expectedMermaidBytes, err := os.ReadFile(goldenMermaidPath)
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}

	if diff := cmp.Diff(strings.TrimSpace(string(expectedMermaidBytes)), strings.TrimSpace(buf.String())); diff != "" {
		t.Errorf("Mermaid mismatch (-expected +actual):\n%s", diff)
	}
}

func TestSource_skipsPlanRowsWhenScalarFlagsDisabled(t *testing.T) {
	statsToRender := &sppb.ResultSetStats{
		QueryPlan: &sppb.QueryPlan{
			PlanNodes: []*sppb.PlanNode{{
				Index:       0,
				DisplayName: "Root",
				Kind:        sppb.PlanNode_RELATIONAL,
				ExecutionStats: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"broken": structpb.NewStringValue("not-a-stat-struct"),
					},
				},
			}},
		},
	}

	plan, err := visualize.BuildPlan(nil, statsToRender, visualize.BuildOptions{})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	src, err := mermaid.Source(plan)
	if err != nil {
		t.Fatalf("Source() error = %v", err)
	}
	if !strings.Contains(src, "Root") {
		t.Fatalf("Source() output = %q, want root label", src)
	}
}

func TestSourceWithOptions_overridesPlanBuild(t *testing.T) {
	statsToRender := &sppb.ResultSetStats{
		QueryPlan: &sppb.QueryPlan{
			PlanNodes: []*sppb.PlanNode{{
				Index:       0,
				DisplayName: "Root",
				Kind:        sppb.PlanNode_RELATIONAL,
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"execution_method": structpb.NewStringValue("Row"),
					},
				},
			}},
		},
	}

	plan, err := visualize.BuildPlan(nil, statsToRender, visualize.BuildOptions{})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	src, err := mermaid.SourceWithOptions(plan, mermaid.Options{
		BuildOptions: visualize.BuildOptions{Metadata: true},
	})
	if err != nil {
		t.Fatalf("SourceWithOptions() error = %v", err)
	}
	if !strings.Contains(src, "execution_method") {
		t.Fatalf("SourceWithOptions() output = %q, want metadata", src)
	}
}

func TestSourceWithOptions_canDisableMetadataFromFullPlan(t *testing.T) {
	statsToRender := &sppb.ResultSetStats{
		QueryPlan: &sppb.QueryPlan{
			PlanNodes: []*sppb.PlanNode{{
				Index:       0,
				DisplayName: "Root",
				Kind:        sppb.PlanNode_RELATIONAL,
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"execution_method": structpb.NewStringValue("Row"),
					},
				},
			}},
		},
	}

	plan, err := visualize.BuildPlan(nil, statsToRender, visualize.BuildOptions{Metadata: true})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	src, err := mermaid.SourceWithOptions(plan, mermaid.Options{
		BuildOptions: visualize.BuildOptions{},
	})
	if err != nil {
		t.Fatalf("SourceWithOptions() error = %v", err)
	}
	if strings.Contains(src, "execution_method") {
		t.Fatalf("SourceWithOptions() output = %q, want metadata disabled", src)
	}
}
