package visualize

import "testing"

func TestFullBuildOptions(t *testing.T) {
	opts := FullBuildOptions()
	if !opts.Full || !opts.Metadata || !opts.ExecutionStats || !opts.ExecutionSummary {
		t.Fatalf("FullBuildOptions() = %+v, want all detail flags enabled", opts)
	}
}

func TestStructureBuildOptions(t *testing.T) {
	opts := StructureBuildOptions()
	if !opts.Metadata || !opts.SerializeResult {
		t.Fatalf("StructureBuildOptions() = %+v, want metadata and serialize result", opts)
	}
	if opts.ExecutionStats || opts.ExecutionSummary || opts.Full {
		t.Fatalf("StructureBuildOptions() = %+v, want no execution stats or full preset", opts)
	}
}
