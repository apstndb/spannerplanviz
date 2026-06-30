package visualize

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spannerplan"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/protojson"
)

//go:embed testdata/*
var testdataFS embed.FS

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

	// 2. Build the TreeNode structure
	param := applyTestOptions(BuildOptions{Full: true})

	qp, err := spannerplan.New(statsToProcess.GetQueryPlan().GetPlanNodes())
	if err != nil {
		t.Fatalf("spannerplan.New failed: %v", err)
	}

	rootTreeNode := testBuildTree(t, qp, rowTypeForProcessing, param)

	var allNodesTextContent []string
	var traverseAndFormat func(n *TreeNode) // TreeNode is visualize.TreeNode
	traverseAndFormat = func(n *TreeNode) {
		if n == nil {
			return
		}
		// Pass qp, param, and rowTypeForProcessing to formatNodeContentAsText
		nodeText := formatNodeContentAsText(n, qp, param, rowTypeForProcessing)
		nodeName := n.GetName() // Get the node name
		for _, line := range nodeText {
			allNodesTextContent = append(allNodesTextContent, fmt.Sprintf("%s: %s", nodeName, line))
		}
		for _, childLink := range n.Children {
			traverseAndFormat(childLink.ChildNode)
		}
	}
	traverseAndFormat(rootTreeNode)

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

	// 4. Define build options
	param := applyTestOptions(BuildOptions{Full: true})

	// 5. Create spannerplan.QueryPlan
	qp, err := spannerplan.New(statsToProcess.GetQueryPlan().GetPlanNodes())
	if err != nil {
		t.Fatalf("Failed to create QueryPlan: %v", err)
	}

	// 6. Build the TreeNode structure
	rootTreeNode := testBuildTree(t, qp, rowTypeForProcessing, param)

	// 7. Create a slice to store all Mermaid labels
	var allLabels []string

	// 8. Implement a recursive function to traverse the TreeNode structure
	var traverseAndCollectLabels func(n *TreeNode)
	traverseAndCollectLabels = func(n *TreeNode) {
		if n == nil {
			return
		}
		label := n.MermaidLabel(param, rowTypeForProcessing)
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

// formatNodeContentAsText formats node.getNodeContent for test.
func formatNodeContentAsText(node *TreeNode, qp *spannerplan.QueryPlan, param BuildOptions, rowType *sppb.StructType) []string {
	if node == nil {
		return nil
	}
	content := node.getNodeContent(param, rowType)
	var result []string

	if content.Title != "" {
		result = append(result, fmt.Sprintf("Title: %s", content.Title))
	}
	if content.ShortRepresentation != "" {
		result = append(result, fmt.Sprintf("ShortRepresentation: %s", content.ShortRepresentation))
	}
	if content.ScanInfo != "" {
		result = append(result, fmt.Sprintf("ScanInfo: %s", content.ScanInfo))
	}

	for _, line := range content.SerializeResult {
		result = append(result, fmt.Sprintf("SerializeResult: %s", line))
	}

	for _, line := range content.NonVarScalarLinks {
		result = append(result, fmt.Sprintf("NonVarScalarLink: %s", line))
	}

	if len(content.Metadata) > 0 {
		var metaLines []string
		for k, v := range content.Metadata {
			metaLines = append(metaLines, fmt.Sprintf("Metadata: %s = %s", k, v))
		}
		sort.Strings(metaLines) // Ensure deterministic order for golden files
		result = append(result, metaLines...)
	}

	for _, line := range content.VarScalarLinks {
		result = append(result, fmt.Sprintf("VarScalarLink: %s", line))
	}

	if len(content.Stats) > 0 {
		var statLines []string
		for k, v := range content.Stats {
			statLines = append(statLines, fmt.Sprintf("Stat: %s: %s", k, v))
		}
		sort.Strings(statLines) // Ensure deterministic order for golden files
		result = append(result, statLines...)
	}

	if content.ExecutionSummary != "" {
		for _, line := range strings.Split(strings.TrimSuffix(content.ExecutionSummary, "\n"), "\n") {
			if line != "" {
				result = append(result, fmt.Sprintf("ExecutionSummary: %s", line))
			}
		}
	}

	return result
}
