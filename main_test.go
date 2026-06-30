package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_renderErrorStdout(t *testing.T) {
	err := runWithInput(t, `{"queryPlan": {"planNodes": []}}`, nil)
	if err == nil {
		t.Fatal("run() error = nil, want render error")
	}
	if strings.Contains(err.Error(), "remove") {
		t.Fatalf("run() error = %v, should not attempt file cleanup on stdout", err)
	}
}

func TestRun_renderErrorRemovesPartialFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "plan.svg")
	err := runWithInput(t, `{"queryPlan": {"planNodes": []}}`, []string{"--output", out})
	if err == nil {
		t.Fatal("run() error = nil, want render error")
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("partial output file still exists after render error: stat err = %v", statErr)
	}
}

func runWithInput(t *testing.T, input string, extraArgs []string) error {
	t.Helper()

	oldArgs := os.Args
	oldStdin := os.Stdin
	t.Cleanup(func() {
		os.Args = oldArgs
		os.Stdin = oldStdin
	})

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r
	go func() {
		_, _ = io.WriteString(w, input)
		_ = w.Close()
	}()

	os.Args = append([]string{"spannerplanviz"}, extraArgs...)
	return run(context.Background())
}
