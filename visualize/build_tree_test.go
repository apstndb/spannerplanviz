package visualize

import (
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/MakeNowJust/heredoc/v2"
	"github.com/apstndb/spannerplan"
	"github.com/apstndb/spannerplan/plantree"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/apstndb/spannerplanviz/option"
)

func applyTestOptions(opts option.Options) option.Options {
	opts.ApplyFullOption()
	return opts
}

func testBuildTree(t *testing.T, qp *spannerplan.QueryPlan, rowType *sppb.StructType, param option.Options) *treeNode {
	t.Helper()

	rowsByID, err := buildScalarLinkRowIndex(qp, param)
	if err != nil {
		t.Fatalf("buildScalarLinkRowIndex: %v", err)
	}
	rootNode, err := buildTree(qp, qp.GetNodeByIndex(0), rowType, param, rowsByID)
	if err != nil {
		t.Fatalf("buildTree: %v", err)
	}
	return rootNode
}

func planRowsFor(t *testing.T, qp *spannerplan.QueryPlan) map[int32]plantree.RowWithPredicates {
	t.Helper()

	rowsByID, err := buildPlanRowIndex(qp)
	if err != nil {
		t.Fatalf("buildPlanRowIndex: %v", err)
	}
	return rowsByID
}

func newTestQueryPlan(nodes []*sppb.PlanNode) (*spannerplan.QueryPlan, error) {
	if len(nodes) == 0 {
		return nil, spannerplan.ErrEmptyPlanNodes
	}

	indexToPos := make(map[int32]int32, len(nodes))
	for i, node := range nodes {
		indexToPos[node.GetIndex()] = int32(i)
	}

	normalized := make([]*sppb.PlanNode, len(nodes))
	for i, node := range nodes {
		clone := proto.Clone(node).(*sppb.PlanNode)
		if clone.GetKind() == sppb.PlanNode_KIND_UNSPECIFIED {
			clone.Kind = sppb.PlanNode_RELATIONAL
		}
		clone.Index = int32(i)
		for _, childLink := range clone.ChildLinks {
			if childLink == nil {
				continue
			}
			childLink.ChildIndex = indexToPos[childLink.ChildIndex]
		}
		normalized[i] = clone
	}

	return spannerplan.New(normalized)
}

func normalizedNodeRef(qp *spannerplan.QueryPlan, sourceNodes []*sppb.PlanNode, target *sppb.PlanNode) *sppb.PlanNode {
	if target == nil || qp == nil {
		return target
	}
	for i, n := range sourceNodes {
		if n.GetDisplayName() == target.GetDisplayName() && n.GetIndex() == target.GetIndex() {
			return qp.GetNodeByIndex(int32(i))
		}
	}
	return target
}

func TestToLeftAlignedText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
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
			name:  "html escape - no internal escaping by toLeftAlignedText",
			input: "a < b & c > d",
			want:  `a < b & c > d<br align="left" />`,
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

func TestTreeNodeMermaidLabel(t *testing.T) {
	testCases := []struct {
		name          string
		planNodeProto *sppb.PlanNode
		param         option.Options
		rowType       *sppb.StructType

		// TODO: nodesForPlan seems not robust workaround.
		nodesForPlan         []*sppb.PlanNode // For setting up QueryPlan
		expectedMermaidLabel string
	}{
		{
			name:                 "Nil PlanNodeProto",
			planNodeProto:        nil,
			param:                option.Options{TypeFlag: "mermaid"},
			rowType:              nil,
			nodesForPlan:         []*sppb.PlanNode{},
			expectedMermaidLabel: `node\_unknown`,
		},
		{
			name: "Simple Node (Title only)",
			planNodeProto: &sppb.PlanNode{
				Index:       0,
				DisplayName: "Test Node",
			},
			param:                option.Options{TypeFlag: "mermaid"},
			rowType:              nil,
			nodesForPlan:         []*sppb.PlanNode{{Index: 0, DisplayName: "Test Node"}},
			expectedMermaidLabel: "<b>Test&nbsp;Node</b>",
		},
		{
			name: "Node with Title and Metadata",
			planNodeProto: &sppb.PlanNode{
				Index:       1,
				DisplayName: "Meta Node",
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"meta_key": structpb.NewStringValue("meta_val"),
						"another":  structpb.NewNumberValue(42),
					},
				},
			},
			param:   option.Options{TypeFlag: "mermaid", Metadata: true}, // Ensure metadata is processed by GetMetadata
			rowType: nil,
			nodesForPlan: []*sppb.PlanNode{{
				Index:       1,
				DisplayName: "Meta Node",
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"meta_key": structpb.NewStringValue("meta_val"),
						"another":  structpb.NewNumberValue(42),
					},
				},
			}},
			expectedMermaidLabel: heredoc.Doc(`
<b>Meta&nbsp;Node</b>
another: 42
meta\_key: meta\_val`),
		},
		{
			name: "Node with Stats",
			planNodeProto: &sppb.PlanNode{
				Index:       2,
				DisplayName: "Stat Node",
				ExecutionStats: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"latency": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{"total": structpb.NewStringValue("1ms")},
						}),
					},
				},
			},
			param:   option.Options{TypeFlag: "mermaid", ExecutionStats: true}, // Ensure stats are processed
			rowType: nil,
			nodesForPlan: []*sppb.PlanNode{{
				Index:       2,
				DisplayName: "Stat Node",
				ExecutionStats: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"latency": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{"total": structpb.NewStringValue("1ms")},
						}),
					},
				},
			}},
			expectedMermaidLabel: heredoc.Doc(`<b>Stat&nbsp;Node</b>
<i>latency: 1ms</i>`),
		},
		{
			name: "Scan Node with ScanInfo",
			planNodeProto: &sppb.PlanNode{
				Index:       3,
				DisplayName: "Table Scan", // Contains "Scan"
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"scan_type":   structpb.NewStringValue("Full Scan"),
						"scan_target": structpb.NewStringValue("UsersTable"),
					},
				},
			},
			param:   option.Options{TypeFlag: "mermaid", HideScanTarget: false}, // Ensure ScanInfo is generated
			rowType: nil,
			nodesForPlan: []*sppb.PlanNode{{
				Index:       3,
				DisplayName: "Table Scan",
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"scan_type":   structpb.NewStringValue("Full Scan"),
						"scan_target": structpb.NewStringValue("UsersTable"),
					},
				},
			}},
			expectedMermaidLabel: heredoc.Doc(`
<b>Full&nbsp;&nbsp;Table&nbsp;Scan</b>
Full&nbsp;\:&nbsp;UsersTable`),
		},
		{
			name: "Serialize Result Node",
			planNodeProto: &sppb.PlanNode{
				Index:       0,
				DisplayName: "Serialize Result",
				ChildLinks: []*sppb.PlanNode_ChildLink{
					{ChildIndex: 1, Type: ""},
				},
			},
			param: option.Options{TypeFlag: "mermaid", SerializeResult: true},
			rowType: &sppb.StructType{
				Fields: []*sppb.StructType_Field{
					{Name: "userID", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
				},
			},
			nodesForPlan: []*sppb.PlanNode{
				{
					Index:       0,
					DisplayName: "Serialize Result",
					ChildLinks: []*sppb.PlanNode_ChildLink{
						{ChildIndex: 1, Type: ""},
					},
				},
				{
					Index: 1,
					Kind:  sppb.PlanNode_SCALAR,
					ShortRepresentation: &sppb.PlanNode_ShortRepresentation{Description: "U_ID"},
				},
			},
			expectedMermaidLabel: heredoc.Doc(`<b>Serialize&nbsp;Result</b>
Result\.userID\:U\_ID`),
		},
		{
			name: "Node with All Elements",
			planNodeProto: &sppb.PlanNode{
				Index:               5,
				DisplayName:         "Complex Node",
				ShortRepresentation: &sppb.PlanNode_ShortRepresentation{Description: "SR: Complex"},
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"meta_1": structpb.NewStringValue("val_1"),
						// scan_type/target deliberately omitted to not conflict with scaninfo if DisplayName was "Scan"
					},
				},
				ExecutionStats: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"cpu_time": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{"total": structpb.NewStringValue("5ms")},
						}),
						"execution_summary": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{"num_executions": structpb.NewStringValue("10")},
						}),
					},
				},
			},
			param:   option.Options{TypeFlag: "mermaid", Metadata: true, ExecutionStats: true, ExecutionSummary: true},
			rowType: nil,
			nodesForPlan: []*sppb.PlanNode{{
				Index:               5,
				DisplayName:         "Complex Node",
				ShortRepresentation: &sppb.PlanNode_ShortRepresentation{Description: "SR: Complex"},
				Metadata: &structpb.Struct{
					Fields: map[string]*structpb.Value{"meta_1": structpb.NewStringValue("val_1")},
				},
				ExecutionStats: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"cpu_time": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{"total": structpb.NewStringValue("5ms")},
						}),
						"execution_summary": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{"num_executions": structpb.NewStringValue("10")},
						}),
					},
				},
			}},
			// Order: Title, ShortRep, (ScanInfo - N/A), (SerializeResult - N/A), (NonVarScalar - N/A), Meta, (VarScalar - N/A), Stats, ExecSummary
			expectedMermaidLabel: heredoc.Doc(`
<b>Complex&nbsp;Node</b>
SR\:&nbsp;Complex
meta\_1: val\_1
<i>cpu\_time: 5ms</i>
<i>execution\_summary\:
&nbsp;&nbsp;&nbsp;num\_executions\:&nbsp;10</i>`),
		},
		{
			name: "Node with quotes in content",
			planNodeProto: &sppb.PlanNode{
				Index:               6,
				DisplayName:         "Node \"With Quotes\"",
				ShortRepresentation: &sppb.PlanNode_ShortRepresentation{Description: "Description with \"quotes\" and `backticks`"},
			},
			param:        option.Options{TypeFlag: "mermaid"},
			rowType:      nil,
			nodesForPlan: []*sppb.PlanNode{{Index: 6, DisplayName: "Node \"With Quotes\"", ShortRepresentation: &sppb.PlanNode_ShortRepresentation{Description: "Description with \"quotes\" and `backticks`"}}},
			expectedMermaidLabel: heredoc.Doc(`
<b>Node&nbsp;&quot;With&nbsp;Quotes&quot;</b>
Description&nbsp;with&nbsp;&quot;quotes&quot;&nbsp;and&nbsp;`) + "\\`backticks\\`",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// For tests involving child links (like Serialize Result), ensure the plan has the child nodes.
			// Most simple cases here don't need complex child setups for MermaidLabel, unlike HTML which might traverse.
			nodesInTestPlan := tc.nodesForPlan
			if nodesInTestPlan == nil { // Default if not specified by test case
				if tc.planNodeProto != nil {
					nodesInTestPlan = []*sppb.PlanNode{tc.planNodeProto}
				} else {
					nodesInTestPlan = []*sppb.PlanNode{} // Empty plan for nil proto test
				}
			}

			var currentPlan *spannerplan.QueryPlan
			var err error
			if len(nodesInTestPlan) > 0 || tc.name == "Serialize Result Node" { // Serialize Result needs QueryPlan even if node list is simple
				currentPlan, err = newTestQueryPlan(nodesInTestPlan)
				if err != nil {
					t.Fatalf("spannerplan.New failed for test case %q: %v", tc.name, err)
				}
			}
			// If nodesInTestPlan is empty and not Serialize Result, currentPlan can be nil.
			// Getters in MermaidLabel should handle nil QueryPlan if they don't use it.
			// GetSerializeResultOutput, GetNonVarScalarLinksOutput, GetVarScalarLinksOutput do use qp.

			node := &treeNode{
				planNode: normalizedNodeRef(currentPlan, nodesInTestPlan, tc.planNodeProto),
			}
			if currentPlan != nil {
				attachPlanRow(node, planRowsFor(t, currentPlan))
			}

			// The MermaidLabel method itself handles the final quote escaping for the overall label.
			// So, tc.expectedMermaidLabel should represent the content *before* that final step,
			// but *with* internal #quotquot; and #96; etc. from escapeMermaidLabelContent.
			// The current structure of MermaidLabel in build_tree.go does:
			// labelContent := strings.Join(labelParts, "<br/>")
			// ...
			// return strings.ReplaceAll(labelContent, "\"", "#quotquot;")
			// So expectedMermaidLabel should match `labelContent`
			// Let's adjust expectations to match the actual output of MermaidLabel directly.
			// This means expectedMermaidLabel already includes the final #quotquot; transformations if any part had a quote.

			gotLabel := node.MermaidLabel(currentPlan, tc.param, tc.rowType)

			if diff := cmp.Diff(tc.expectedMermaidLabel, gotLabel); diff != "" {
				t.Errorf("MermaidLabel() mismatch for test case %q (-expected +actual):\n%s", tc.name, diff)
				t.Logf("Got: %s", gotLabel)
			}
		})
	}
}

// TestTreeNodeHTML is being re-added here.
func TestTreeNodeHTML(t *testing.T) {
	testCases := []struct {
		name          string
		planNodeProto *sppb.PlanNode
		param         option.Options
		rowType       *sppb.StructType // Can be nil if not testing Serialize Result
		expectedHTML  string
	}{
		{
			name: "Simple Title Only",
			planNodeProto: &sppb.PlanNode{
				Index:       1,
				DisplayName: "Test Node Display Name",
			},
			param:        applyTestOptions(option.Options{Full: true}),
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
			param:        applyTestOptions(option.Options{Full: true}),
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
			param:        applyTestOptions(option.Options{Full: true, ExecutionStats: true}),
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
			param:        applyTestOptions(option.Options{Full: true, HideScanTarget: false}),
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
			param: applyTestOptions(option.Options{Full: true}),
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
			param:        applyTestOptions(option.Options{Full: true, NonVariableScalar: true}),
			rowType:      nil,
			expectedHTML: `<b>Scalar Node</b>`,
		},
		{
			name: "Node with Variable Scalar Child Links",
			planNodeProto: &sppb.PlanNode{
				Index:       0,
				DisplayName: "VarScalarOp",
				ChildLinks: []*sppb.PlanNode_ChildLink{
					{ChildIndex: 1, Type: "SCALAR", Variable: "var1"},
				},
			},
			param:        applyTestOptions(option.Options{Full: true, VariableScalar: true}),
			rowType:      nil,
			expectedHTML: `<b>VarScalarOp</b><br align="CENTER"/>SCALAR: $var1:=Scalar Output<br align="left" />`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			nodesForPlan := []*sppb.PlanNode{}
			switch tc.name {
			case "Serialize Result Node":
				nodesForPlan = append(nodesForPlan, &sppb.PlanNode{
					Index:       0,
					DisplayName: "Serialize Result",
					ChildLinks: []*sppb.PlanNode_ChildLink{
						{ChildIndex: 1, Type: "Output"},
					},
				})
				nodesForPlan = append(nodesForPlan, &sppb.PlanNode{Index: 1, DisplayName: "ChildForSerialize", Kind: sppb.PlanNode_RELATIONAL})
			case "Node with Scalar Child Links":
				nodesForPlan = append(nodesForPlan, &sppb.PlanNode{
					Index:       0,
					DisplayName: "Scalar Node",
					ChildLinks: []*sppb.PlanNode_ChildLink{
						{ChildIndex: 1, Type: "SCALAR", Variable: "scalar_var1"},
						{ChildIndex: 2, Type: "SCALAR", Variable: "scalar_var2"},
					},
				})
				nodesForPlan = append(nodesForPlan,
					&sppb.PlanNode{Index: 1, Kind: sppb.PlanNode_SCALAR, DisplayName: "Scalar Child 1"},
					&sppb.PlanNode{Index: 2, Kind: sppb.PlanNode_SCALAR, DisplayName: "Scalar Child 2"},
				)
			case "Node with Variable Scalar Child Links":
				nodesForPlan = append(nodesForPlan, tc.planNodeProto)
				nodesForPlan = append(nodesForPlan, &sppb.PlanNode{
					Index:               1,
					Kind:                sppb.PlanNode_SCALAR,
					DisplayName:         "ScalarFunc",
					ShortRepresentation: &sppb.PlanNode_ShortRepresentation{Description: "Scalar Output"},
				})
			default:
				// Default behavior: only the node itself (if no child links are involved in the HTML output expectation).
				nodesForPlan = append(nodesForPlan, tc.planNodeProto)
			}

			currentPlan, err := newTestQueryPlan(nodesForPlan)
			if err != nil {
				nodeIndicesInPlan := []int32{}
				for _, n := range nodesForPlan {
					nodeIndicesInPlan = append(nodeIndicesInPlan, n.GetIndex())
				}
				t.Fatalf("spannerplan.New failed for test case %q with node indices %v: %v", tc.name, nodeIndicesInPlan, err)
			}

			node := &treeNode{
				planNode: normalizedNodeRef(currentPlan, nodesForPlan, tc.planNodeProto),
			}
			attachPlanRow(node, planRowsFor(t, currentPlan))

			gotHTML := node.HTML(currentPlan, tc.param, tc.rowType)
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
