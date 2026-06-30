package option

import "testing"

func TestOptionsNormalize(t *testing.T) {
	t.Run("defaults empty type to svg", func(t *testing.T) {
		var opts Options
		if err := opts.Normalize(); err != nil {
			t.Fatalf("Normalize() error = %v", err)
		}
		if opts.TypeFlag != "svg" {
			t.Fatalf("TypeFlag = %q, want svg", opts.TypeFlag)
		}
	})

	t.Run("rejects unsupported type", func(t *testing.T) {
		opts := Options{TypeFlag: "pdf"}
		err := opts.Normalize()
		if err == nil {
			t.Fatal("Normalize() error = nil, want unsupported type error")
		}
	})

	t.Run("applies full option", func(t *testing.T) {
		opts := Options{Full: true, TypeFlag: "dot"}
		if err := opts.Normalize(); err != nil {
			t.Fatalf("Normalize() error = %v", err)
		}
		if !opts.ExecutionStats || !opts.SerializeResult {
			t.Fatalf("Full option not applied: %+v", opts)
		}
	})
}
