package graphviz_test

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

	"github.com/apstndb/spannerplanviz/graphviz"
	"github.com/apstndb/spannerplanviz/option"
	"github.com/apstndb/spannerplanviz/visualize"
)

func testdataPath(name string) string {
	return filepath.Join("..", "visualize", "testdata", name)
}

func TestRenderer_SVG(t *testing.T) {
	jsonBytes, err := os.ReadFile(testdataPath("dca_profile.json"))
	if err != nil {
		t.Fatalf("read dca_profile.json: %v", err)
	}

	var resultSet sppb.ResultSet
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshalOpts.Unmarshal(jsonBytes, &resultSet); err != nil {
		t.Fatalf("unmarshal dca_profile.json: %v", err)
	}

	opts := option.Options{
		TypeFlag:          "svg",
		Full:              true,
		NonVariableScalar: true,
		VariableScalar:    true,
		Metadata:          true,
		ExecutionStats:    true,
		ExecutionSummary:  true,
		SerializeResult:   true,
	}

	plan, err := visualize.BuildPlan(resultSet.GetMetadata().GetRowType(), resultSet.GetStats(), opts.BuildOptions())
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	var buf bytes.Buffer
	renderer := graphviz.NewRenderer(graphviz.Options{Format: graphviz.SVG})
	if err := renderer.Render(context.Background(), &buf, plan); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	expectedSVGBytes, err := os.ReadFile(testdataPath("full.svg"))
	if err != nil {
		t.Fatalf("read full.svg: %v", err)
	}

	if os.Getenv("UPDATE_GOLDEN_FILES") == "true" {
		if err := os.WriteFile(testdataPath("full.svg"), buf.Bytes(), 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
		t.Fatal("golden file updated")
	}

	if diff := cmp.Diff(strings.TrimSpace(string(expectedSVGBytes)), strings.TrimSpace(buf.String())); diff != "" {
		t.Fatalf("SVG mismatch (-expected +actual):\n%s", diff)
	}
}

func TestRenderer_WithQueryStats(t *testing.T) {
	jsonBytes, err := os.ReadFile(testdataPath("dca_profile.json"))
	if err != nil {
		t.Fatalf("read dca_profile.json: %v", err)
	}

	var resultSet sppb.ResultSet
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshalOpts.Unmarshal(jsonBytes, &resultSet); err != nil {
		t.Fatalf("unmarshal dca_profile.json: %v", err)
	}

	opts := option.Options{
		TypeFlag:          "svg",
		Full:              true,
		NonVariableScalar: true,
		VariableScalar:    true,
		Metadata:          true,
		ExecutionStats:    true,
		ExecutionSummary:  true,
		SerializeResult:   true,
		ShowQuery:         true,
		ShowQueryStats:    true,
	}

	plan, err := visualize.BuildPlan(resultSet.GetMetadata().GetRowType(), resultSet.GetStats(), opts.BuildOptions())
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	var buf bytes.Buffer
	renderer := graphviz.NewRenderer(graphviz.Options{
		Format:         graphviz.SVG,
		ShowQuery:      true,
		ShowQueryStats: true,
	})
	if err := renderer.Render(context.Background(), &buf, plan); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	expectedSVGBytes, err := os.ReadFile(testdataPath("full_with_query_stats.svg"))
	if err != nil {
		t.Fatalf("read full_with_query_stats.svg: %v", err)
	}

	if os.Getenv("UPDATE_GOLDEN_FILES") == "true" {
		if err := os.WriteFile(testdataPath("full_with_query_stats.svg"), buf.Bytes(), 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
		t.Fatal("golden file updated")
	}

	if diff := cmp.Diff(strings.TrimSpace(string(expectedSVGBytes)), strings.TrimSpace(buf.String())); diff != "" {
		t.Fatalf("SVG mismatch (-expected +actual):\n%s", diff)
	}
}

func TestRenderer_rendersSVG(t *testing.T) {
	jsonBytes, err := os.ReadFile(testdataPath("dca_profile.json"))
	if err != nil {
		t.Fatalf("read dca_profile.json: %v", err)
	}

	var resultSet sppb.ResultSet
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshalOpts.Unmarshal(jsonBytes, &resultSet); err != nil {
		t.Fatalf("unmarshal dca_profile.json: %v", err)
	}

	plan, err := visualize.BuildPlan(resultSet.GetMetadata().GetRowType(), resultSet.GetStats(), visualize.BuildOptions{})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	var buf bytes.Buffer
	renderer := graphviz.NewRenderer(graphviz.Options{Format: graphviz.SVG})
	if err := renderer.Render(context.Background(), &buf, plan); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(buf.String(), "<svg") {
		preview := buf.String()
		if len(preview) > 80 {
			preview = preview[:80]
		}
		t.Fatalf("Render() output = %q, want SVG", preview)
	}
}

func TestRenderer_requiresFormat(t *testing.T) {
	plan, err := visualize.BuildPlan(nil, &sppb.ResultSetStats{
		QueryPlan: &sppb.QueryPlan{
			PlanNodes: []*sppb.PlanNode{{
				Index:       0,
				DisplayName: "Root",
				Kind:        sppb.PlanNode_RELATIONAL,
			}},
		},
	}, visualize.BuildOptions{})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	err = graphviz.NewRenderer(graphviz.Options{}).Render(context.Background(), &bytes.Buffer{}, plan)
	if err == nil {
		t.Fatal("Render() error = nil, want missing format error")
	}
}
