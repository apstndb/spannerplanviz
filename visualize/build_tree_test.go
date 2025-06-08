package visualize

import (
	"fmt"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spannerplan"
	"github.com/apstndb/spannerplanviz/option"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/known/structpb"
	"sigs.k8s.io/yaml"
)

func TestToLeftAlignedText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "single line",
			input: "hello world",
			want:  "hello world<br align=\"left\" />",
		},
		{
			name:  "multiple lines",
			input: "line1\nline2\nline3",
			want:  "line1<br align=\"left\" />line2<br align=\"left\" />line3<br align=\"left\" />",
		},
		{
			name:  "html escape",
			input: "a < b & c > d",
			want:  `a &lt; b &amp; c &gt; d<br align="left" />`,
		},
		{
			name:  "trailing newline",
			input: "line1\n",
			want:  "line1<br align=\"left\" />",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toLeftAlignedText(tt.input)
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("toLeftAlignedText() mismatch (-got +want):\n%s", diff)
			}
		})
	}
}

// TestTreeNodeHTML is being re-added here.
func TestTreeNodeHTML(t *testing.T) {
	// Ensure necessary imports are present at the top of the file:
	// import (
	// 	"fmt"
	// 	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	// 	"github.com/apstndb/spannerplan"
	// 	"github.com/apstndb/spannerplanviz/option"
	// 	"github.com/google/go-cmp/cmp"
	// 	"google.golang.org/protobuf/types/known/structpb"
	// 	"sigs.k8s.io/yaml"
	// )

	testCases := []struct {
		name          string
		planNodeProto *sppb.PlanNode
		// plan field removed as it's constructed per test run
		param        option.Options
		rowType      *sppb.StructType // Can be nil if not testing Serialize Result
		expectedHTML string
	}{
		{
			name: "Simple Title Only",
			planNodeProto: &sppb.PlanNode{
				Index:       1,
				DisplayName: "Test Node Display Name",
			},
			param:        option.Options{Full: true},
			rowType:      nil,
			expectedHTML: `<b>Test Node Display Name</b>`,
		},
		{
			name: "Title and Metadata",
			planNodeProto: &sppb.PlanNode{
				Index:       2,
				DisplayName: "Node With Meta",
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"meta_key_1": structpb.NewStringValue("meta_val_1"),
						"meta_key_2": structpb.NewNumberValue(123),
					},
				},
			},
			param:        option.Options{Full: true},
			rowType:      nil,
			expectedHTML: `<b>Node With Meta</b><br align="CENTER"/>meta_key_1=meta_val_1<br align="left" />meta_key_2=123<br align="left" />`,
		},
		{
			name: "Title and Stats",
			planNodeProto: &sppb.PlanNode{
				Index:       3,
				DisplayName: "Node With Stats",
				ExecutionStats: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"latency": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{
								"total": structpb.NewStringValue("10s"),
							},
						}),
						"rows": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{
								"total": structpb.NewStringValue("100"),
								"unit":  structpb.NewStringValue("rows"),
							},
						}),
					},
				},
			},
			param:        option.Options{Full: true, ExecutionStats: true},
			rowType:      nil,
			expectedHTML: `<b>Node With Stats</b><br align="CENTER"/><i>latency: 10s<br align="left" />rows: 100 rows<br align="left" /></i>`,
		},
		{
			name: "Scan Node",
			planNodeProto: &sppb.PlanNode{
				Index:       4,
				DisplayName: "Table Scan",
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"scan_type":   structpb.NewStringValue("Full scan"),
						"scan_target": structpb.NewStringValue("MyTable"),
					},
				},
			},
			param:        option.Options{Full: true, HideScanTarget: false},
			rowType:      nil,
			expectedHTML: `<b>Full scan Table Scan</b><br align="CENTER"/>Full scan: MyTable<br align="left" />`,
		},
		{
			name: "Serialize Result Node",
			planNodeProto: &sppb.PlanNode{
				Index:       5,
				DisplayName: "Serialize Result",
				ChildLinks: []*sppb.PlanNode_ChildLink{
					{ChildIndex: 0, Type: "Output"},
				},
			},
			param: option.Options{Full: true},
			rowType: &sppb.StructType{
				Fields: []*sppb.StructType_Field{
					{Name: "col1", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
					{Name: "col2", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
				},
			},
			expectedHTML: `<b>Serialize Result</b>`,
		},
		{
			name: "Node with Scalar Child Links",
			planNodeProto: &sppb.PlanNode{
				Index:       6,
				DisplayName: "Scalar Node",
				ChildLinks: []*sppb.PlanNode_ChildLink{
					{ChildIndex: 7, Type: "SCALAR", Variable: "scalar_var1"},
					{ChildIndex: 8, Type: "SCALAR", Variable: "scalar_var2"},
				},
			},
			param:        option.Options{Full: true, NonVariableScalar: true},
			rowType:      nil,
			expectedHTML: `<b>Scalar Node</b>`,
		},
		{
			name: "Node with Variable Scalar Child Links",
			planNodeProto: &sppb.PlanNode{
				Index:       9,
				DisplayName: "VarScalarOp",
				ChildLinks: []*sppb.PlanNode_ChildLink{
					{ChildIndex: 10, Type: "SCALAR", Variable: "var1"},
				},
			},
			param:        option.Options{Full: true, VariableScalar: true}, // VariableScalar is true
			rowType:      nil,
			expectedHTML: `<b>VarScalarOp</b><br align="CENTER"/>SCALAR: $var1:=Scalar Output<br align="left" />`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			nodesForPlan := []*sppb.PlanNode{}
			switch tc.name {
			case "Serialize Result Node":
				// tc.planNodeProto (Index 5) links to ChildIndex 0.
				// For this case, ensure tc.planNodeProto is included, and its direct reference.
				// If spannerplan is slice-based, this might still be problematic if indices aren't dense.
				nodesForPlan = append(nodesForPlan, tc.planNodeProto)
				nodesForPlan = append(nodesForPlan, &sppb.PlanNode{Index: 0, DisplayName: "ChildForSerialize"})
			case "Node with Scalar Child Links":
				// tc.planNodeProto (Index 6) links to ChildIndex 7 and 8.
				// Re-applying dense slice hack assuming spannerplan might be slice-based.
				for i := 0; i < 6; i++ { // Dummy nodes for 0-5
					nodesForPlan = append(nodesForPlan, &sppb.PlanNode{Index: int32(i), DisplayName: fmt.Sprintf("Dummy %d", i)})
				}
				nodesForPlan = append(nodesForPlan, tc.planNodeProto) // Actual node at Index 6
				nodesForPlan = append(nodesForPlan, &sppb.PlanNode{Index: 7, DisplayName: "Scalar Child 1"}) // Actual node at Index 7
				nodesForPlan = append(nodesForPlan, &sppb.PlanNode{Index: 8, DisplayName: "Scalar Child 2"}) // Actual node at Index 8
			case "Node with Variable Scalar Child Links":
				// tc.planNodeProto (Index 9) links to ChildIndex 10.
				// Apply dense slice hack.
				for i := 0; i < 9; i++ { // Dummy nodes for 0-8
					nodesForPlan = append(nodesForPlan, &sppb.PlanNode{Index: int32(i), DisplayName: fmt.Sprintf("Dummy %d", i)})
				}
				nodesForPlan = append(nodesForPlan, tc.planNodeProto) // Actual node at Index 9
				nodesForPlan = append(nodesForPlan, &sppb.PlanNode{ // Actual Scalar Child for Var
					Index:       10,
					Kind:        sppb.PlanNode_SCALAR,
					DisplayName: "ScalarFunc",
					ShortRepresentation: &sppb.PlanNode_ShortRepresentation{Description: "Scalar Output"},
				})
			default:
				// Default behavior: only the node itself (if no child links are involved in the HTML output expectation).
				nodesForPlan = append(nodesForPlan, tc.planNodeProto)
			}

			currentPlan, err := spannerplan.New(nodesForPlan)
			if err != nil {
				nodeIndicesInPlan := []int32{}
				for _, n := range nodesForPlan {
					nodeIndicesInPlan = append(nodeIndicesInPlan, n.GetIndex())
				}
				t.Fatalf("spannerplan.New failed for test case %q with node indices %v: %v", tc.name, nodeIndicesInPlan, err)
			}

			node := &treeNode{
				planNodeProto: tc.planNodeProto,
				plan:          currentPlan,
				Name:          fmt.Sprintf("node%d", tc.planNodeProto.GetIndex()),
			}
			tooltipBytes, errYaml := yaml.Marshal(tc.planNodeProto)
			if errYaml != nil {
				t.Fatalf("Failed to marshal planNodeProto to YAML for tooltip: %v", errYaml)
			}
			node.Tooltip = string(tooltipBytes)

			gotHTML := node.HTML(tc.param, tc.rowType)
			if diff := cmp.Diff(tc.expectedHTML, gotHTML); diff != "" {
				t.Errorf("HTML() mismatch for test case %q (-expected +actual):\n%s", tc.name, diff)
			}
		})
	}
}

func TestIsRemoteCall(t *testing.T) {
	tests := []struct {
		name string
		node *sppb.PlanNode
		cl   *sppb.PlanNode_ChildLink
		want bool
	}{
		{
			name: "subquery_cluster_node missing",
			node: &sppb.PlanNode{
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{},
				},
			},
			cl:   &sppb.PlanNode_ChildLink{ChildIndex: 0},
			want: false,
		},
		{
			name: "call_type is Local",
			node: &sppb.PlanNode{
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"subquery_cluster_node": structpb.NewStringValue("0"),
						"call_type":             structpb.NewStringValue("Local"),
					},
				},
			},
			cl:   &sppb.PlanNode_ChildLink{ChildIndex: 0},
			want: false,
		},
		{
			name: "call_type missing, subquery_cluster_node matches",
			node: &sppb.PlanNode{
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"subquery_cluster_node": structpb.NewStringValue("0"),
					},
				},
			},
			cl:   &sppb.PlanNode_ChildLink{ChildIndex: 0},
			want: true,
		},
		{
			name: "call_type missing, subquery_cluster_node does not match",
			node: &sppb.PlanNode{
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"subquery_cluster_node": structpb.NewStringValue("1"),
					},
				},
			},
			cl:   &sppb.PlanNode_ChildLink{ChildIndex: 0},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRemoteCall(tt.node, tt.cl)
			if got != tt.want {
				t.Errorf("isRemoteCall() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTryToTimestampStr(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantError bool
	}{
		{
			name:      "valid timestamp",
			input:     "1678881600.123456",
			want:      "2023-03-15T12:00:00.123456Z",
			wantError: false,
		},
		{
			name:      "valid timestamp - zero padded microseconds",
			input:     "1678881600.000123",
			want:      "2023-03-15T12:00:00.000123Z",
			wantError: false,
		},
		{
			name:      "valid timestamp with less than 6 microseconds",
			input:     "1678881600.123",
			want:      "",
			wantError: true,
		},
		{
			name:      "valid timestamp without microseconds",
			input:     "1678881600",
			want:      "",
			wantError: true,
		},
		{
			name:      "invalid format - too many microseconds",
			input:     "1678886400.1234567",
			want:      "",
			wantError: true,
		},
		{
			name:      "invalid format - non-numeric seconds",
			input:     "abc.123456",
			want:      "",
			wantError: true,
		},
		{
			name:      "invalid format - non-numeric microseconds",
			input:     "1678886400.def",
			want:      "",
			wantError: true,
		},
		{
			name:      "empty string",
			input:     "",
			want:      "",
			wantError: true,
		},
		{
			name:      "zero timestamp",
			input:     "0.0",
			want:      "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tryToTimestampStr(tt.input)
			if (err != nil) != tt.wantError {
				t.Errorf("tryToTimestampStr() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if !tt.wantError {
				if diff := cmp.Diff(got, tt.want); diff != "" {
					t.Errorf("tryToTimestampStr() mismatch (-got +want):\n%s", diff)
				}
			}
		})
	}
}

func TestFormatExecutionStatsValue(t *testing.T) {
	tests := []struct {
		name  string
		input *structpb.Value
		want  string
	}{
		{
			name: "all fields present",
			input: structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{
					"total":         structpb.NewStringValue("100"),
					"unit":          structpb.NewStringValue("rows"),
					"mean":          structpb.NewStringValue("10"),
					"std_deviation": structpb.NewStringValue("2"),
				},
			}),
			want: "100@10±2 rows",
		},
		{
			name: "no std_deviation",
			input: structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{
					"total":         structpb.NewStringValue("50"),
					"unit":          structpb.NewStringValue("bytes"),
					"mean":          structpb.NewStringValue("5"),
					"std_deviation": structpb.NewStringValue(""),
				},
			}),
			want: "50@5 bytes",
		},
		{
			name: "no mean or std_deviation",
			input: structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{
					"total":         structpb.NewStringValue("200"),
					"unit":          structpb.NewStringValue("ms"),
					"mean":          structpb.NewStringValue(""),
					"std_deviation": structpb.NewStringValue(""),
				},
			}),
			want: "200 ms",
		},
		{
			name: "empty struct",
			input: structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{},
			}),
			want: "",
		},
		{
			name: "all fields empty strings",
			input: structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{
					"total":         structpb.NewStringValue(""),
					"unit":          structpb.NewStringValue(""),
					"mean":          structpb.NewStringValue(""),
					"std_deviation": structpb.NewStringValue(""),
				},
			}),
			want: "",
		},
		{
			name: "missing total",
			input: structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{
					"unit":          structpb.NewStringValue("rows"),
					"mean":          structpb.NewStringValue("10"),
					"std_deviation": structpb.NewStringValue("2"),
				},
			}),
			want: "@10±2 rows",
		},
		{
			name: "missing unit",
			input: structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{
					"total":         structpb.NewStringValue("100"),
					"mean":          structpb.NewStringValue("10"),
					"std_deviation": structpb.NewStringValue("2"),
				},
			}),
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

func TestFormatMetadata(t *testing.T) {
	tests := []struct {
		name         string
		input        map[string]*structpb.Value
		hideMetadata []string
		want         string
	}{
		{
			name: "standard metadata",
			input: map[string]*structpb.Value{
				"key1": structpb.NewStringValue("value1"),
				"key2": structpb.NewNumberValue(123),
				"key3": structpb.NewBoolValue(true),
			},
			hideMetadata: nil,
			want:         "key1=value1\nkey2=123\nkey3=true\n",
		},
		{
			name: "with hidden metadata",
			input: map[string]*structpb.Value{
				"key1":    structpb.NewStringValue("value1"),
				"hide_me": structpb.NewStringValue("hidden_value"),
				"key2":    structpb.NewNumberValue(123),
			},
			hideMetadata: []string{"hide_me"},
			want:         "key1=value1\nkey2=123\n",
		},
		{
			name: "with internal metadata fields",
			input: map[string]*structpb.Value{
				"key1":      structpb.NewStringValue("value1"),
				"call_type": structpb.NewStringValue("Local"),
				"scan_type": structpb.NewStringValue("Full"),
				"key2":      structpb.NewNumberValue(123),
			},
			hideMetadata: nil,
			want:         "key1=value1\nkey2=123\n",
		},
		{
			name:         "empty metadata",
			input:        map[string]*structpb.Value{},
			hideMetadata: nil,
			want:         "",
		},
		{
			name: "only internal metadata fields",
			input: map[string]*structpb.Value{
				"call_type": structpb.NewStringValue("Local"),
				"scan_type": structpb.NewStringValue("Full"),
			},
			hideMetadata: nil,
			want:         "",
		},
		{
			name:         "nil metadata map",
			input:        nil,
			hideMetadata: nil,
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMetadata(tt.input, tt.hideMetadata)
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("formatMetadata() mismatch (-got +want):\n%s", diff)
			}
		})
	}
}
