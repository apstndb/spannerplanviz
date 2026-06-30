package option

import (
	"fmt"

	"github.com/apstndb/spannerplanviz/visualize"
)

type Options struct {
	Positional struct {
		Input string
	} `positional-args:"yes"`
	TypeFlag          string   `long:"type" description:"output type" default:"svg" choice:"svg" choice:"dot" choice:"png" choice:"mermaid"` // nolint:staticcheck
	Filename          string   `long:"output"`
	NonVariableScalar bool     `long:"non-variable-scalar"`
	VariableScalar    bool     `long:"variable-scalar"`
	Metadata          bool     `long:"metadata"`
	ExecutionStats    bool     `long:"execution-stats"`
	ExecutionSummary  bool     `long:"execution-summary"`
	SerializeResult   bool     `long:"serialize-result"`
	HideScanTarget    bool     `long:"hide-scan-target"`
	ShowQuery         bool     `long:"show-query"`
	ShowQueryStats    bool     `long:"show-query-stats"`
	Full              bool     `long:"full" description:"full output"`
	HideMetadata      []string `long:"hide-metadata"`
}

// BuildOptions maps CLI flags to library build settings.
func (o *Options) BuildOptions() visualize.BuildOptions {
	o.ApplyFullOption()
	return visualize.BuildOptions{
		Full:              o.Full,
		NonVariableScalar: o.NonVariableScalar,
		VariableScalar:    o.VariableScalar,
		Metadata:          o.Metadata,
		ExecutionStats:    o.ExecutionStats,
		ExecutionSummary:  o.ExecutionSummary,
		SerializeResult:   o.SerializeResult,
		HideScanTarget:    o.HideScanTarget,
		HideMetadata:      o.HideMetadata,
	}
}

func (o *Options) ApplyFullOption() {
	if o.Full {
		o.NonVariableScalar = true
		o.VariableScalar = true
		o.Metadata = true
		o.ExecutionStats = true
		o.ExecutionSummary = true
		o.SerializeResult = true
	}
}

// Normalize applies derived options and validates output settings for library callers.
func (o *Options) Normalize() error {
	o.ApplyFullOption()
	if o.TypeFlag == "" {
		o.TypeFlag = "svg"
	}
	switch o.TypeFlag {
	case "svg", "dot", "png", "mermaid":
		return nil
	default:
		return fmt.Errorf("unsupported output type %q", o.TypeFlag)
	}
}
