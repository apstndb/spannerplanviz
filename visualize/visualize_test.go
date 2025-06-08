package visualize

import (
	"bytes"
	"context"
	"embed"

	// "fmt" // Removing based on persistent build error
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spannerplan"
	"github.com/apstndb/spannerplanviz/option"
	"github.com/goccy/go-graphviz"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

//go:embed testdata/*
var testdataFS embed.FS

func TestRenderImage(t *testing.T) {
	// 1. Read dca_profile.json
	jsonBytes, err := testdataFS.ReadFile("testdata/dca_profile.json")
	if err != nil {
		t.Fatalf("Failed to read dca_profile.json from embed.FS: %v", err)
	}

	// 2. Unmarshal into sppb.ResultSet
	var resultSet sppb.ResultSet
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	err = unmarshalOpts.Unmarshal(jsonBytes, &resultSet)
	if err != nil {
		t.Fatalf("Failed to unmarshal dca_profile.json into sppb.ResultSet: %v", err)
	}

	// 3. Extract stats and rowType
	statsToRender := resultSet.GetStats()
	rowTypeToRender := resultSet.GetMetadata().GetRowType()

	if statsToRender == nil || statsToRender.GetQueryPlan() == nil || len(statsToRender.GetQueryPlan().GetPlanNodes()) == 0 {
		t.Fatalf("dca_profile.json (via ResultSet.Stats) does not contain any plan nodes.")
	}

	param := option.Options{
		TypeFlag:          "svg",
		Full:              true,
		NonVariableScalar: true,
		VariableScalar:    true,
		Metadata:          true,
		ExecutionStats:    true,
		ExecutionSummary:  true,
		SerializeResult:   true,
		ShowQuery:         false,
		ShowQueryStats:    false,
	}

	var buf bytes.Buffer
	err = RenderImage(context.Background(), rowTypeToRender, statsToRender, graphviz.SVG, &buf, param)
	if err != nil {
		t.Fatalf("RenderImage failed: %v", err)
	}
	actualSVG := buf.String()

	expectedSVGBytes, err := testdataFS.ReadFile("testdata/full.svg")
	if err != nil {
		t.Fatalf("Failed to read testdata/full.svg from embed.FS: %v", err)
	}
	expectedSVG := string(expectedSVGBytes)

	if os.Getenv("UPDATE_GOLDEN_FILES") == "true" {
		// This block is for updating the golden file if the env var is set.
		// It assumes actualSVG holds the new correct content.
		goldenSVGPath := "testdata/full.svg" // Define path for clarity
		errWrite := os.WriteFile(goldenSVGPath, []byte(actualSVG), 0644)
		if errWrite != nil {
			t.Fatalf("Failed to write updated golden file %s: %v", goldenSVGPath, errWrite)
		}
		t.Logf("Successfully updated golden file %s. Please re-run tests without UPDATE_GOLDEN_FILES.", goldenSVGPath)
		t.Fatalf("Golden file updated. Re-run tests.")
	}

	if diff := cmp.Diff(strings.TrimSpace(expectedSVG), strings.TrimSpace(actualSVG)); diff != "" {
		t.Logf("SVG diff (-expected +actual):\n%s", diff)
		t.Errorf("Generated SVG does not match testdata/full.svg.")
		// t.Skip("Skipping SVG diff check as golden file update is required due to scan type formatting changes.") // Remove skip
	}
}

func TestRenderImage_WithQueryStats(t *testing.T) {
	jsonBytes, err := testdataFS.ReadFile("testdata/dca_profile.json")
	if err != nil {
		t.Fatalf("Failed to read dca_profile.json from embed.FS: %v", err)
	}

	var resultSet sppb.ResultSet
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	err = unmarshalOpts.Unmarshal(jsonBytes, &resultSet)
	if err != nil {
		t.Fatalf("Failed to unmarshal dca_profile.json into sppb.ResultSet: %v", err)
	}

	statsToRender := resultSet.GetStats()
	rowTypeToRender := resultSet.GetMetadata().GetRowType()

	if statsToRender == nil || statsToRender.GetQueryPlan() == nil || len(statsToRender.GetQueryPlan().GetPlanNodes()) == 0 {
		t.Fatalf("dca_profile.json (via ResultSet.Stats) does not contain any plan nodes.")
	}

	param := option.Options{
		TypeFlag:          "svg",
		Full:              true,
		NonVariableScalar: true,
		VariableScalar:    true,
		Metadata:          true,
		ExecutionStats:    true,
		ExecutionSummary:  true,
		SerializeResult:   true,
		ShowQuery:         true,
		ShowQueryStats:    true,
	}

	var buf bytes.Buffer
	err = RenderImage(context.Background(), rowTypeToRender, statsToRender, graphviz.SVG, &buf, param)
	if err != nil {
		t.Fatalf("RenderImage failed: %v", err)
	}
	actualSVG := buf.String()

	goldenSVGPath := "testdata/full_with_query_stats.svg"
	expectedSVGBytes, err := testdataFS.ReadFile(goldenSVGPath)
	if err != nil {
		t.Fatalf("Failed to read %s from embed.FS: %v", goldenSVGPath, err)
	}
	expectedSVG := string(expectedSVGBytes)

	if os.Getenv("UPDATE_GOLDEN_FILES") == "true" {
		// This block is for updating the golden file if the env var is set.
		// It assumes actualSVG holds the new correct content.
		errWrite := os.WriteFile(goldenSVGPath, []byte(actualSVG), 0644)
		if errWrite != nil {
			t.Fatalf("Failed to write updated golden file %s: %v", goldenSVGPath, errWrite)
		}
		t.Logf("Successfully updated golden file %s. Please re-run tests without UPDATE_GOLDEN_FILES.", goldenSVGPath)
		t.Fatalf("Golden file updated. Re-run tests.")
	}

	if diff := cmp.Diff(strings.TrimSpace(expectedSVG), strings.TrimSpace(actualSVG)); diff != "" {
		t.Logf("SVG diff (-expected +actual) for %s:\n%s", goldenSVGPath, diff)
		t.Errorf("Generated SVG does not match %s.", goldenSVGPath)
		// t.Skipf("Skipping SVG diff check for %s as golden file update is required due to scan type formatting changes.", goldenSVGPath) // Remove skip
	}
}

func TestRenderMermaid(t *testing.T) {
	node2Stats, _ := structpb.NewStruct(map[string]interface{}{"rows": "10", "latency": "1ms"})
	node1Stats, _ := structpb.NewStruct(map[string]interface{}{"rows": "20", "latency": "2ms"})
	node0Stats, _ := structpb.NewStruct(map[string]interface{}{"rows": "20", "latency": "3ms"})

	stats := &sppb.ResultSetStats{
		QueryPlan: &sppb.QueryPlan{
			PlanNodes: []*sppb.PlanNode{
				{
					Index:       0,
					DisplayName: "Union",
					Kind:        sppb.PlanNode_RELATIONAL,
					ChildLinks: []*sppb.PlanNode_ChildLink{
						{ChildIndex: 1, Type: "Input"},
						{ChildIndex: 2, Type: "Input"},
					},
					ExecutionStats: node0Stats,
					Metadata:       &structpb.Struct{},
				},
				{
					Index:          1,
					DisplayName:    "Scan1",
					Kind:           sppb.PlanNode_RELATIONAL,
					ExecutionStats: node1Stats,
					Metadata:       &structpb.Struct{},
				},
				{
					Index:          2,
					DisplayName:    "Scan2",
					Kind:           sppb.PlanNode_RELATIONAL,
					ExecutionStats: node2Stats,
					Metadata:       &structpb.Struct{},
				},
			},
		},
		QueryStats: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"query_text": structpb.NewStringValue("SELECT 1"),
			},
		},
	}

	var rowType *sppb.StructType = nil

	param := option.Options{
		TypeFlag:          "mermaid",
		Full:              true,
		NonVariableScalar: true,
		VariableScalar:    true,
		Metadata:          true,
		ExecutionStats:    true,
		ExecutionSummary:  true,
		ShowQuery:         true,
		ShowQueryStats:    false,
	}

	qp, err := spannerplan.New(stats.GetQueryPlan().GetPlanNodes())
	if err != nil {
		t.Fatalf("Failed to create QueryPlan: %v", err)
	}
	rootNode, err := buildTree(qp, qp.GetNodeByIndex(0), rowType, param)
	if err != nil {
		t.Fatalf("Failed to build tree: %v", err)
	}

	t.Logf("Logging HTML output for treeNodes using param: %+v", param)
	var logHTML func(n *treeNode)
	logHTML = func(n *treeNode) {
		if n == nil {
			return
		}
		// n.HTML() now requires qp, param and rowType. rowType is nil in this test.
		t.Logf("Node %s HTML: [%s]", n.GetName(), n.HTML(qp, param, nil))
		for _, childLink := range n.Children {
			logHTML(childLink.ChildNode)
		}
	}
	if rootNode != nil {
		t.Logf("Number of children for rootNode (node0) in treeNode: %d", len(rootNode.Children))
		if len(rootNode.Children) == 0 {
			physicalRootNode := qp.GetNodeByIndex(0)
			if physicalRootNode != nil {
				visibleLinks := qp.VisibleChildLinks(physicalRootNode)
				t.Logf("Number of visible child links for physical node 0 from qp.VisibleChildLinks: %d", len(visibleLinks))
				for i, link := range visibleLinks {
					t.Logf("Visible Link %d: Type=%s, ChildIndex=%d, Variable=%s", i, link.GetType(), link.GetChildIndex(), link.GetVariable())
				}
			} else {
				t.Logf("Could not get physical node 0 from qp for VisibleChildLinks check.")
			}
		}
		logHTML(rootNode)
	} else {
		t.Logf("rootNode is nil after buildTree.")
	}

	var buf bytes.Buffer
	err = RenderImage(context.Background(), rowType, stats, graphviz.SVG, &buf, param)

	if err != nil {
		t.Fatalf("TestRenderMermaid failed: expected no error, but got: %v. Output: %s", err, buf.String())
	}

	expectedMermaidOutput := strings.Join([]string{
		`graph TD`,
		`    node0["<b>Union</b><br/><i>latency: 3ms</i><br/><i>rows: 20</i>"]`,
		`    style node0 text-align:left;`,
		`    node1["<b>Scan1</b><br/><i>latency: 2ms</i><br/><i>rows: 20</i>"]`,
		`    style node1 text-align:left;`,
		`    node2["<b>Scan2</b><br/><i>latency: 1ms</i><br/><i>rows: 10</i>"]`,
		`    style node2 text-align:left;`,
		`    node0 -->|Input| node1`,
		`    node0 -->|Input| node2`,
		``,
	}, "\n")

	actualOutput := strings.ReplaceAll(buf.String(), "\r\n", "\n")
	expectedOutputNormalized := strings.ReplaceAll(expectedMermaidOutput, "\r\n", "\n")

	if strings.TrimSpace(actualOutput) != strings.TrimSpace(expectedOutputNormalized) {
		t.Errorf("TestRenderMermaid output mismatch:\nExpected:\n%s\nActual:\n%s", expectedOutputNormalized, actualOutput)
	}
}

func TestRenderMermaid_TextContent(t *testing.T) {
	// 1. Load dca_profile.json using embed.FS
	jsonBytes, err := testdataFS.ReadFile("testdata/dca_profile.json")
	if err != nil {
		t.Fatalf("Failed to read dca_profile.json: %v", err)
	}

	var resultSet sppb.ResultSet
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	err = unmarshalOpts.Unmarshal(jsonBytes, &resultSet)
	if err != nil {
		t.Fatalf("Failed to unmarshal dca_profile.json into sppb.ResultSet: %v", err)
	}

	statsToProcess := resultSet.GetStats()
	rowTypeForProcessing := resultSet.GetMetadata().GetRowType()

	if statsToProcess == nil || statsToProcess.GetQueryPlan() == nil || len(statsToProcess.GetQueryPlan().GetPlanNodes()) == 0 {
		t.Fatalf("dca_profile.json (via ResultSet.Stats) does not contain any plan nodes.")
	}

	// 2. Build the treeNode structure
	param := option.Options{Full: true}

	qp, err := spannerplan.New(statsToProcess.GetQueryPlan().GetPlanNodes())
	if err != nil {
		t.Fatalf("spannerplan.New failed: %v", err)
	}

	rootTreeNode, err := buildTree(qp, qp.GetNodeByIndex(0), rowTypeForProcessing, param)
	if err != nil {
		t.Fatalf("buildTree failed: %v", err)
	}

	var allNodesTextContent []string
	var traverseAndFormat func(n *treeNode) // treeNode is visualize.treeNode
	traverseAndFormat = func(n *treeNode) {
		if n == nil {
			return
		}
		// Pass qp, param, and rowTypeForProcessing to formatNodeContentAsText
		nodeText := formatNodeContentAsText(n, qp, param, rowTypeForProcessing)
		allNodesTextContent = append(allNodesTextContent, nodeText...)
		for _, childLink := range n.Children {
			traverseAndFormat(childLink.ChildNode)
		}
	}
	traverseAndFormat(rootTreeNode)
	sort.Strings(allNodesTextContent)

	actualContent := strings.Join(allNodesTextContent, "\n") + "\n"

	// 3. Compare with a new golden file: testdata/dca_profile_plan_content.txt
	goldenFilePath := "testdata/dca_profile_plan_content.txt"

	if os.Getenv("UPDATE_GOLDEN_FILES") == "true" {
		// Golden file path relative to the package directory
		resolvedGoldenFilePath := filepath.Join("testdata", "dca_profile_plan_content.txt")

		// Ensure the target directory ("testdata") exists
		targetDir := filepath.Dir(resolvedGoldenFilePath) // This will be "testdata"
		if errMkdir := os.MkdirAll(targetDir, 0755); errMkdir != nil {
			t.Fatalf("Failed to create directory %s: %v", targetDir, errMkdir)
		}

		t.Logf("Attempting to update golden file: %s", resolvedGoldenFilePath)
		err = os.WriteFile(resolvedGoldenFilePath, []byte(actualContent), 0644)
		if err != nil {
			t.Fatalf("Failed to write golden file %s: %v", resolvedGoldenFilePath, err)
		}
		t.Logf("Successfully updated golden file %s. Please re-run tests without UPDATE_GOLDEN_FILES.", resolvedGoldenFilePath)
		// Fail after updating to ensure the next run compares against the new golden file.
		// Or use t.SkipNow() if preferred to not show as a "failure" during update runs.
		t.Fatalf("Golden file updated. Re-run tests.")
	}

	// Read directly from filesystem for comparison.
	// goldenFilePath is "testdata/dca_profile_plan_content.txt"
	// This path will be relative to the package directory when the test runs.
	expectedContentBytes, err := os.ReadFile(goldenFilePath)
	if err != nil {
		t.Fatalf("Failed to read golden file %s: %v. (Try running with UPDATE_GOLDEN_FILES=true env var)", goldenFilePath, err)
	}
	expectedContent := string(expectedContentBytes)

	if diff := cmp.Diff(expectedContent, actualContent); diff != "" {
		t.Errorf("Text content mismatch (-expected +actual) for %s:\n%s", goldenFilePath, diff)
		// t.Skipf("Skipping text content diff check for %s as golden file update is required due to scan type formatting changes.", goldenFilePath) // Remove skip
	}
}

func TestRenderMermaid_Golden(t *testing.T) {
	// 1. Read dca_profile.json
	jsonBytes, err := testdataFS.ReadFile("testdata/dca_profile.json")
	if err != nil {
		t.Fatalf("Failed to read dca_profile.json from embed.FS: %v", err)
	}

	// 2. Unmarshal into sppb.ResultSet
	var resultSet sppb.ResultSet
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	err = unmarshalOpts.Unmarshal(jsonBytes, &resultSet)
	if err != nil {
		t.Fatalf("Failed to unmarshal dca_profile.json into sppb.ResultSet: %v", err)
	}

	// 3. Extract stats and rowType
	statsToRender := resultSet.GetStats()
	rowTypeToRender := resultSet.GetMetadata().GetRowType()

	if statsToRender == nil || statsToRender.GetQueryPlan() == nil || len(statsToRender.GetQueryPlan().GetPlanNodes()) == 0 {
		t.Fatalf("dca_profile.json (via ResultSet.Stats) does not contain any plan nodes.")
	}

	// 4. Define option.Options
	param := option.Options{
		TypeFlag:          "mermaid",
		Full:              true,
		NonVariableScalar: true,
		VariableScalar:    true,
		Metadata:          true,
		ExecutionStats:    true,
		ExecutionSummary:  true,
		ShowQuery:         true,
		ShowQueryStats:    false,
	}

	// 5. Create a spannerplan.QueryPlan instance
	qp, err := spannerplan.New(statsToRender.GetQueryPlan().GetPlanNodes())
	if err != nil {
		t.Fatalf("Failed to create QueryPlan: %v", err)
	}

	// 6. Build the treeNode structure
	rootPhysicalNode := qp.GetNodeByIndex(0)
	if rootPhysicalNode == nil {
		t.Fatalf("Root physical plan node is nil")
	}
	rootTreeNode, err := buildTree(qp, rootPhysicalNode, rowTypeToRender, param)
	if err != nil {
		t.Fatalf("Failed to build tree: %v", err)
	}

	// 7. Create a bytes.Buffer to capture the output
	var buf bytes.Buffer

	// 8. Call visualize.renderMermaid
	err = renderMermaid(rootTreeNode, &buf, qp, param, rowTypeToRender)
	if err != nil {
		t.Fatalf("renderMermaid failed: %v", err)
	}

	// 9. Get the actual Mermaid output string
	actualMermaid := buf.String()

	// 10. Define the golden file path
	goldenMermaidPath := "testdata/dca_profile.mermaid.golden"

	// 11. Implement the golden file update logic
	if os.Getenv("UPDATE_GOLDEN_FILES") == "true" {
		errWrite := os.WriteFile(goldenMermaidPath, []byte(actualMermaid), 0644)
		if errWrite != nil {
			t.Fatalf("Failed to write updated golden file %s: %v", goldenMermaidPath, errWrite)
		}
		t.Logf("Successfully updated golden file %s. Please re-run tests without UPDATE_GOLDEN_FILES.", goldenMermaidPath)
		t.Fatalf("Golden file updated. Re-run tests.") // Or t.SkipNow()
	}

	// 12. If not updating golden files, read the expected content
	expectedMermaidBytes, err := os.ReadFile(goldenMermaidPath)
	if err != nil {
		t.Fatalf("Failed to read golden file %s: %v. (Try running with UPDATE_GOLDEN_FILES=true env var)", goldenMermaidPath, err)
	}
	expectedMermaid := string(expectedMermaidBytes)

	// 13. Compare the actualMermaid string with the expectedMermaid string
	if diff := cmp.Diff(strings.TrimSpace(expectedMermaid), strings.TrimSpace(actualMermaid)); diff != "" {
		t.Errorf("Generated Mermaid output does not match %s. Diff (-expected +actual):\n%s", goldenMermaidPath, diff)
	}
}

func TestMermaidLabel_Golden(t *testing.T) {
	// 1. Read dca_profile.json
	jsonBytes, err := testdataFS.ReadFile("testdata/dca_profile.json")
	if err != nil {
		t.Fatalf("Failed to read dca_profile.json from embed.FS: %v", err)
	}

	// 2. Unmarshal into sppb.ResultSet
	var resultSet sppb.ResultSet
	unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
	err = unmarshalOpts.Unmarshal(jsonBytes, &resultSet)
	if err != nil {
		t.Fatalf("Failed to unmarshal dca_profile.json into sppb.ResultSet: %v", err)
	}

	// 3. Extract stats and rowType
	statsToProcess := resultSet.GetStats()
	rowTypeForProcessing := resultSet.GetMetadata().GetRowType()

	if statsToProcess == nil || statsToProcess.GetQueryPlan() == nil || len(statsToProcess.GetQueryPlan().GetPlanNodes()) == 0 {
		t.Fatalf("dca_profile.json (via ResultSet.Stats) does not contain any plan nodes.")
	}

	// 4. Define option.Options
	param := option.Options{
		Full:              true,
		NonVariableScalar: true,
		VariableScalar:    true,
		Metadata:          true,
		ExecutionStats:    true,
		ExecutionSummary:  true,
	}

	// 5. Create spannerplan.QueryPlan
	qp, err := spannerplan.New(statsToProcess.GetQueryPlan().GetPlanNodes())
	if err != nil {
		t.Fatalf("Failed to create QueryPlan: %v", err)
	}

	// 6. Build the treeNode structure
	rootPhysicalNode := qp.GetNodeByIndex(0)
	if rootPhysicalNode == nil {
		t.Fatalf("Root physical plan node is nil")
	}
	rootTreeNode, err := buildTree(qp, rootPhysicalNode, rowTypeForProcessing, param)
	if err != nil {
		t.Fatalf("Failed to build tree: %v", err)
	}

	// 7. Create a slice to store all Mermaid labels
	var allLabels []string

	// 8. Implement a recursive function to traverse the treeNode structure
	var traverseAndCollectLabels func(n *treeNode)
	traverseAndCollectLabels = func(n *treeNode) {
		if n == nil {
			return
		}
		label := n.MermaidLabel(qp, param, rowTypeForProcessing)
		allLabels = append(allLabels, label)
		for _, childLink := range n.Children {
			traverseAndCollectLabels(childLink.ChildNode)
		}
	}

	// 9. Start the traversal from rootTreeNode
	traverseAndCollectLabels(rootTreeNode)

	// 10. Sort the allLabels slice
	sort.Strings(allLabels)

	// 11. Join the sorted labels into a single string
	actualLabelsContent := strings.Join(allLabels, "\n") + "\n"

	// 12. Define the golden file path
	goldenLabelsPath := "testdata/dca_profile.mermaid_labels.golden"

	// 13. Implement the golden file update logic
	if os.Getenv("UPDATE_GOLDEN_FILES") == "true" {
		errWrite := os.WriteFile(goldenLabelsPath, []byte(actualLabelsContent), 0644)
		if errWrite != nil {
			t.Fatalf("Failed to write updated golden file %s: %v", goldenLabelsPath, errWrite)
		}
		t.Logf("Successfully updated golden file %s. Please re-run tests without UPDATE_GOLDEN_FILES.", goldenLabelsPath)
		t.Fatalf("Golden file updated. Re-run tests.") // Or t.SkipNow()
	}

	// 14. If not updating, read the expected content from the golden file
	expectedLabelsBytes, err := os.ReadFile(goldenLabelsPath)
	if err != nil {
		t.Fatalf("Failed to read golden file %s: %v. (Try running with UPDATE_GOLDEN_FILES=true env var)", goldenLabelsPath, err)
	}
	expectedLabelsContent := string(expectedLabelsBytes)

	// 15. Compare actualLabelsContent with expectedLabelsContent
	if diff := cmp.Diff(expectedLabelsContent, actualLabelsContent); diff != "" {
		t.Errorf("Generated Mermaid labels do not match %s. Diff (-expected +actual):\n%s", goldenLabelsPath, diff)
	}
}
