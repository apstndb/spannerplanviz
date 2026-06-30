package visualize

import (
	"fmt"

	"github.com/apstndb/spannerplan"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

// BuildOptions controls which plan details are included when building a diagram model.
type BuildOptions struct {
	Full              bool
	NonVariableScalar bool
	VariableScalar    bool
	Metadata          bool
	ExecutionStats    bool
	ExecutionSummary  bool
	SerializeResult   bool
	HideScanTarget    bool
	HideMetadata      []string
}

// ApplyFull enables all detail flags used by the CLI --full preset.
func (o *BuildOptions) ApplyFull() {
	if o.Full {
		o.NonVariableScalar = true
		o.VariableScalar = true
		o.Metadata = true
		o.ExecutionStats = true
		o.ExecutionSummary = true
		o.SerializeResult = true
	}
}

// FullBuildOptions returns settings equivalent to CLI --full.
func FullBuildOptions() BuildOptions {
	opts := BuildOptions{Full: true}
	opts.ApplyFull()
	return opts
}

// StructureBuildOptions returns operator structure suitable for interactive viewers
// without execution stats-heavy output.
func StructureBuildOptions() BuildOptions {
	return BuildOptions{
		Metadata:        true,
		SerializeResult: true,
	}
}

// Plan is a built diagram model ready for backend renderers.
type Plan struct {
	Root       *TreeNode
	QueryPlan  *spannerplan.QueryPlan
	RowType    *sppb.StructType
	QueryStats *sppb.ResultSetStats
	Build      BuildOptions
}

// BuildPlan constructs a diagram model from query plan stats.
func BuildPlan(rowType *sppb.StructType, queryStats *sppb.ResultSetStats, opts BuildOptions) (*Plan, error) {
	opts.ApplyFull()

	if queryStats == nil || queryStats.GetQueryPlan() == nil {
		return nil, fmt.Errorf("cannot build plan: queryStats or queryPlan is nil")
	}

	qp, err := spannerplan.New(queryStats.GetQueryPlan().GetPlanNodes())
	if err != nil {
		return nil, fmt.Errorf("failed to create QueryPlan: %w", err)
	}

	rowsByID, err := buildScalarLinkRowIndex(qp, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to process plan rows: %w", err)
	}

	rootNode, err := buildTree(qp, qp.GetNodeByIndex(0), rowType, opts, rowsByID)
	if err != nil {
		return nil, fmt.Errorf("failed to build tree: %w", err)
	}

	return &Plan{
		Root:       rootNode,
		QueryPlan:  qp,
		RowType:    rowType,
		QueryStats: queryStats,
		Build:      opts,
	}, nil
}
