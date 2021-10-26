package main

import "testing"

func Test_customFileToTableRenderDef(t *testing.T) {
	yamlContent := `
- name: ID
  template: '{{.FormatID}}'
  alignment: RIGHT
`

	_, err := customFileToTableRenderDef([]byte(yamlContent))
	if err != nil {
		t.Fatal(err)
	}
}
