package visualize

import (
	"bytes"
	"context"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spannerplan"
	"github.com/apstndb/spannerplan/stats"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/apstndb/spannerplanviz/option"
)

func TestFormatExecutionStatsValue(t *testing.T) {
	tests := []struct {
		name  string
		input stats.ExecutionStatsValue
		want  string
	}{
		{
			name: "all fields present",
			input: stats.ExecutionStatsValue{
				Total:        "100",
				Unit:         "rows",
				Mean:         "10",
				StdDeviation: "2",
			},
			want: "100@10±2 rows",
		},
		{
			name: "no std_deviation",
			input: stats.ExecutionStatsValue{
				Total: "50",
				Unit:  "bytes",
				Mean:  "5",
			},
			want: "50@5 bytes",
		},
		{
			name: "no mean or std_deviation",
			input: stats.ExecutionStatsValue{
				Total: "200",
				Unit:  "ms",
			},
			want: "200 ms",
		},
		{
			name:  "empty value",
			input: stats.ExecutionStatsValue{},
			want:  "",
		},
		{
			name: "missing total",
			input: stats.ExecutionStatsValue{
				Unit:         "rows",
				Mean:         "10",
				StdDeviation: "2",
			},
			want: "@10±2 rows",
		},
		{
			name: "missing unit",
			input: stats.ExecutionStatsValue{
				Total:        "100",
				Mean:         "10",
				StdDeviation: "2",
			},
			want: "100@10±2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatExecutionStatsValue(tt.input)
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("formatExecutionStatsValue() mismatch (-got +want):\n%s", diff)
			}
		})
	}
}

func TestExecutionStatsToMap(t *testing.T) {
	node := &sppb.PlanNode{
		ExecutionStats: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"cpu_time": structpb.NewStructValue(&structpb.Struct{
					Fields: map[string]*structpb.Value{"total": structpb.NewStringValue("10ms")},
				}),
				"rows": structpb.NewStructValue(&structpb.Struct{
					Fields: map[string]*structpb.Value{
						"total": structpb.NewStringValue("100"),
						"unit":  structpb.NewStringValue("rows"),
					},
				}),
				"rows_returned": structpb.NewStructValue(&structpb.Struct{
					Fields: map[string]*structpb.Value{"total": structpb.NewStringValue("7")},
				}),
				"execution_summary": structpb.NewStructValue(&structpb.Struct{
					Fields: map[string]*structpb.Value{"num_executions": structpb.NewStringValue("1")},
				}),
			},
		},
	}

	es, err := extractExecutionStats(node)
	if err != nil {
		t.Fatalf("extractExecutionStats() error = %v", err)
	}

	got := executionStatsToMap(node, es)
	want := map[string]string{
		"cpu_time":      "10ms",
		"rows":          "100 rows",
		"rows_returned": "7",
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("executionStatsToMap() mismatch (-got +want):\n%s", diff)
	}
}

func TestFormatExecutionSummary(t *testing.T) {
	node := &sppb.PlanNode{
		ExecutionStats: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"execution_summary": structpb.NewStructValue(&structpb.Struct{
					Fields: map[string]*structpb.Value{
						"num_executions": structpb.NewStringValue("1"),
						"custom_metric":  structpb.NewStringValue("42"),
					},
				}),
			},
		},
	}

	got := formatExecutionSummary(node, stats.ExecutionStatsSummary{
		NumExecutions:           "1",
		CheckpointTime:          "0.28 msecs",
		ExecutionStartTimestamp: "1678881600.123456",
		ExecutionEndTimestamp:   "1678881600.654321",
		NumCheckPoints:          "19",
	})
	want := "execution_summary:\n" +
		"   checkpoint_time: 0.28 msecs\n" +
		"   custom_metric: 42\n" +
		"   execution_end_timestamp: 2023-03-15T12:00:00.654321Z\n" +
		"   execution_start_timestamp: 2023-03-15T12:00:00.123456Z\n" +
		"   num_checkpoints: 19\n" +
		"   num_executions: 1\n"
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("formatExecutionSummary() mismatch (-got +want):\n%s", diff)
	}
}

func TestTreeNodeGetStats(t *testing.T) {
	tests := []struct {
		name     string
		planNode *sppb.PlanNode
		param    option.Options
		want     map[string]string
	}{
		{
			name: "Node with stats",
			planNode: &sppb.PlanNode{
				ExecutionStats: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"cpu_time": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{"total": structpb.NewStringValue("10ms")},
						}),
						"rows": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{"total": structpb.NewStringValue("100")},
						}),
						"execution_summary": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{"num_executions": structpb.NewStringValue("1")},
						}),
					},
				},
			},
			param: option.Options{ExecutionStats: true},
			want: map[string]string{
				"cpu_time": "10ms",
				"rows":     "100",
			},
		},
		{
			name:     "Node with no stats",
			planNode: &sppb.PlanNode{},
			param:    option.Options{},
			want:     nil,
		},
		{
			name:     "Nil plan node",
			planNode: nil,
			param:    option.Options{},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &treeNode{planNode: tt.planNode}
			got := node.GetStats(tt.param)
			if diff := cmp.Diff(got, tt.want, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("GetStats() mismatch (-got +want):\n%s", diff)
			}
		})
	}
}

func TestTreeNodeGetExecutionSummary(t *testing.T) {
	node := &treeNode{
		planNode: &sppb.PlanNode{
			ExecutionStats: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"execution_summary": structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"num_executions": structpb.NewStringValue("10"),
						},
					}),
				},
			},
		},
	}

	got := node.GetExecutionSummary(option.Options{ExecutionSummary: true})
	want := "execution_summary:\n   num_executions: 10\n"
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("GetExecutionSummary() mismatch (-got +want):\n%s", diff)
	}

	if gotDisabled := node.GetExecutionSummary(option.Options{}); gotDisabled != "" {
		t.Errorf("GetExecutionSummary() with disabled flag = %q, want empty", gotDisabled)
	}
}

func TestBuildScalarLinkRowIndex_skipsWhenFlagsDisabled(t *testing.T) {
	qp, err := spannerplan.New([]*sppb.PlanNode{{
		Index:       0,
		DisplayName: "Root",
		Kind:        sppb.PlanNode_RELATIONAL,
	}})
	if err != nil {
		t.Fatalf("spannerplan.New() error = %v", err)
	}

	rowsByID, err := buildScalarLinkRowIndex(qp, option.Options{})
	if err != nil {
		t.Fatalf("buildScalarLinkRowIndex() error = %v", err)
	}
	if rowsByID != nil {
		t.Fatalf("buildScalarLinkRowIndex() = %#v, want nil", rowsByID)
	}
}

func TestRenderImage_skipsPlanRowsWhenScalarFlagsDisabled(t *testing.T) {
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

	var buf bytes.Buffer
	err := RenderImage(context.Background(), nil, statsToRender, &buf, option.Options{TypeFlag: "mermaid"})
	if err != nil {
		t.Fatalf("RenderImage() error = %v", err)
	}
	if !strings.Contains(buf.String(), "Root") {
		t.Fatalf("RenderImage() output = %q, want root label", buf.String())
	}
}

func TestGetNodeContent_optionGating(t *testing.T) {
	nodes := []*sppb.PlanNode{
		{
			Index:       0,
			DisplayName: "Serialize Result",
			Kind:        sppb.PlanNode_RELATIONAL,
			Metadata: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"meta_key": structpb.NewStringValue("meta_val"),
				},
			},
			ChildLinks: []*sppb.PlanNode_ChildLink{
				{ChildIndex: 1, Type: ""},
				{ChildIndex: 2, Type: "SCALAR", Variable: "var1"},
			},
			ExecutionStats: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"latency": structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{"total": structpb.NewStringValue("1ms")},
					}),
					"execution_summary": structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{"num_executions": structpb.NewStringValue("1")},
					}),
				},
			},
		},
		{Index: 1, Kind: sppb.PlanNode_SCALAR, ShortRepresentation: &sppb.PlanNode_ShortRepresentation{Description: "col_a"}},
		{Index: 2, Kind: sppb.PlanNode_SCALAR, ShortRepresentation: &sppb.PlanNode_ShortRepresentation{Description: "assigned"}},
	}
	rowType := &sppb.StructType{
		Fields: []*sppb.StructType_Field{{Name: "col_a"}},
	}

	qp, err := spannerplan.New(nodes)
	if err != nil {
		t.Fatalf("spannerplan.New() error = %v", err)
	}
	rowsByID, err := buildPlanRowIndex(qp)
	if err != nil {
		t.Fatalf("buildPlanRowIndex() error = %v", err)
	}
	node, err := buildNode(qp.GetNodeByIndex(0), rowsByID)
	if err != nil {
		t.Fatalf("buildNode() error = %v", err)
	}

	t.Run("all disabled", func(t *testing.T) {
		content := node.getNodeContent(option.Options{}, rowType)
		if len(content.Metadata) != 0 {
			t.Errorf("Metadata = %v, want empty", content.Metadata)
		}
		if len(content.SerializeResult) != 0 {
			t.Errorf("SerializeResult = %v, want empty", content.SerializeResult)
		}
		if len(content.NonVarScalarLinks) != 0 {
			t.Errorf("NonVarScalarLinks = %v, want empty", content.NonVarScalarLinks)
		}
		if len(content.VarScalarLinks) != 0 {
			t.Errorf("VarScalarLinks = %v, want empty", content.VarScalarLinks)
		}
		if len(content.Stats) != 0 {
			t.Errorf("Stats = %v, want empty", content.Stats)
		}
		if content.ExecutionSummary != "" {
			t.Errorf("ExecutionSummary = %q, want empty", content.ExecutionSummary)
		}
	})

	t.Run("all enabled", func(t *testing.T) {
		content := node.getNodeContent(option.Options{
			Metadata:          true,
			SerializeResult:   true,
			NonVariableScalar: true,
			VariableScalar:    true,
			ExecutionStats:    true,
			ExecutionSummary:  true,
		}, rowType)
		if len(content.Metadata) == 0 {
			t.Error("Metadata should not be empty when enabled")
		}
		if len(content.SerializeResult) == 0 {
			t.Error("SerializeResult should not be empty when enabled")
		}
		if len(content.VarScalarLinks) == 0 {
			t.Error("VarScalarLinks should not be empty when enabled")
		}
		if len(content.Stats) == 0 {
			t.Error("Stats should not be empty when enabled")
		}
		if content.ExecutionSummary == "" {
			t.Error("ExecutionSummary should not be empty when enabled")
		}
	})
}
