package visualize

import (
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spannerplan"
	"github.com/apstndb/spannerplan/plantree"
	"github.com/google/go-cmp/cmp"
)

func TestFormatScalarChildLinks(t *testing.T) {
	tests := []struct {
		name  string
		links []plantree.ScalarChildLink
		want  string
	}{
		{
			name: "variable scalar with type",
			links: []plantree.ScalarChildLink{
				{Type: "SCALAR", Variable: "var1", Description: "Scalar Output"},
			},
			want: "SCALAR: $var1:=Scalar Output\n",
		},
		{
			name: "non-variable with type prefix for multiple entries",
			links: []plantree.ScalarChildLink{
				{Type: "Key", Description: "col_a"},
				{Type: "Key", Description: "col_b"},
			},
			want: "Key:\n  col_a\n  col_b\n",
		},
		{
			name: "skip empty type and variable",
			links: []plantree.ScalarChildLink{
				{Description: "ignored"},
				{Type: "Condition", Description: "x = 1"},
			},
			want: "Condition: x = 1\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatScalarChildLinks(tt.links)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("formatScalarChildLinks() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFilterScalarChildLinks(t *testing.T) {
	links := []plantree.ScalarChildLink{
		{Type: "Value", Description: "plain"},
		{Type: "SCALAR", Variable: "v", Description: "assigned"},
	}

	gotNonVar := filterScalarChildLinks(links, false)
	gotVar := filterScalarChildLinks(links, true)

	if len(gotNonVar) != 1 || gotNonVar[0].Description != "plain" {
		t.Fatalf("filterScalarChildLinks(false) = %#v, want plain link only", gotNonVar)
	}
	if len(gotVar) != 1 || gotVar[0].Variable != "v" {
		t.Fatalf("filterScalarChildLinks(true) = %#v, want variable link only", gotVar)
	}
}

func TestFormatSerializeResultFromLinks(t *testing.T) {
	rowType := &sppb.StructType{
		Fields: []*sppb.StructType_Field{
			{Name: "userID", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
			{Name: "", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
		},
	}
	links := []plantree.ScalarChildLink{
		{Type: "Key", Description: "ignored"},
		{Description: "U_ID"},
		{Description: "U_NAME"},
	}

	got := formatSerializeResultFromLinks(rowType, links)
	want := "Result.userID:U_ID\nResult.no_name<1>:U_NAME\n"
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("formatSerializeResultFromLinks() mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildPlanRowIndex(t *testing.T) {
	nodes := []*sppb.PlanNode{
		{
			Index:       0,
			DisplayName: "VarScalarOp",
			Kind:        sppb.PlanNode_RELATIONAL,
			ChildLinks: []*sppb.PlanNode_ChildLink{
				{ChildIndex: 1, Type: "SCALAR", Variable: "var1"},
			},
		},
		{
			Index:               1,
			Kind:                sppb.PlanNode_SCALAR,
			ShortRepresentation: &sppb.PlanNode_ShortRepresentation{Description: "Scalar Output"},
		},
	}

	qp, err := spannerplan.New(nodes)
	if err != nil {
		t.Fatalf("spannerplan.New() error = %v", err)
	}

	rowsByID, err := buildPlanRowIndex(qp)
	if err != nil {
		t.Fatalf("buildPlanRowIndex() error = %v", err)
	}

	row, ok := rowsByID[0]
	if !ok {
		t.Fatal("expected row for node 0")
	}
	if len(row.ScalarChildLinks) != 1 {
		t.Fatalf("ScalarChildLinks = %#v, want one link", row.ScalarChildLinks)
	}
	if row.ScalarChildLinks[0].Variable != "var1" {
		t.Fatalf("ScalarChildLinks[0].Variable = %q, want var1", row.ScalarChildLinks[0].Variable)
	}
}

func TestTreeNodeScalarLinksFromPlanRow(t *testing.T) {
	nodes := []*sppb.PlanNode{
		{
			Index:       0,
			DisplayName: "VarScalarOp",
			Kind:        sppb.PlanNode_RELATIONAL,
			ChildLinks: []*sppb.PlanNode_ChildLink{
				{ChildIndex: 1, Type: "SCALAR", Variable: "var1"},
			},
		},
		{
			Index:               1,
			Kind:                sppb.PlanNode_SCALAR,
			ShortRepresentation: &sppb.PlanNode_ShortRepresentation{Description: "Scalar Output"},
		},
	}

	qp, err := spannerplan.New(nodes)
	if err != nil {
		t.Fatalf("spannerplan.New() error = %v", err)
	}
	rowsByID, err := buildPlanRowIndex(qp)
	if err != nil {
		t.Fatalf("buildPlanRowIndex() error = %v", err)
	}

	node, err := buildNode(nodes[0], rowsByID)
	if err != nil {
		t.Fatalf("buildNode() error = %v", err)
	}

	got := node.GetVarScalarLinksOutput()
	want := "SCALAR: $var1:=Scalar Output\n"
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("GetVarScalarLinksOutput() mismatch (-want +got):\n%s", diff)
	}
}
