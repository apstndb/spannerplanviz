package mermaid

import (
	"encoding/json"
	"testing"
)

func Test_mermaidInitConfig(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(mermaidInitConfig())
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if _, ok := config["theme"]; ok {
		t.Fatalf("init config must not set theme: %v", config["theme"])
	}
	if config["htmlLabels"] != true {
		t.Fatalf("htmlLabels = %v, want true", config["htmlLabels"])
	}

	flowchart, ok := config["flowchart"].(map[string]any)
	if !ok {
		t.Fatalf("flowchart = %#v, want object", config["flowchart"])
	}
	if flowchart["useMaxWidth"] != false {
		t.Fatalf("useMaxWidth = %v, want false", flowchart["useMaxWidth"])
	}
	if flowchart["htmlLabels"] != nil {
		t.Fatalf("deprecated flowchart.htmlLabels must not be set")
	}
}
