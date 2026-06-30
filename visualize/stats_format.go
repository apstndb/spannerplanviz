package visualize

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spannerplan/stats"
)

type executionStatField struct {
	key string
	val stats.ExecutionStatsValue
}

func executionStatFields(es stats.ExecutionStats) []executionStatField {
	return []executionStatField{
		{"Disk Usage (KBytes)", es.DiskUsageKBytes},
		{"Disk Write Latency (msecs)", es.DiskWriteLatencyMsecs},
		{"Peak Buffering Memory Usage (KBytes)", es.PeekBufferingMemoryUsageKBytes},
		{"Peak Memory Usage (KBytes)", es.PeakMemoryUsageKBytes},
		{"Rows Spooled", es.RowsSpooled},
		{"Number of Batches", es.NumberOfBatches},
		{"cpu_time", es.CpuTime},
		{"deleted_rows", es.DeletedRows},
		{"filesystem_delay_seconds", es.FilesystemDelaySeconds},
		{"filtered_rows", es.FilteredRows},
		{"latency", es.Latency},
		{"remote_calls", es.RemoteCalls},
		{"rows", es.Rows},
		{"scanned_rows", es.ScannedRows},
	}
}

func extractExecutionStats(node *sppb.PlanNode) (*stats.ExecutionStats, error) {
	if node == nil || node.GetExecutionStats() == nil {
		return nil, nil
	}
	return stats.Extract(node, false)
}

func executionStatsToMap(es *stats.ExecutionStats) map[string]string {
	if es == nil {
		return nil
	}

	statsMap := make(map[string]string)
	for _, field := range executionStatFields(*es) {
		if formatted := formatExecutionStatsValue(field.val); formatted != "" {
			statsMap[field.key] = formatted
		}
	}
	return statsMap
}

func formatExecutionStatsValue(v stats.ExecutionStatsValue) string {
	stdDevStr := prefixIfNotEmpty("±", v.StdDeviation)
	meanStr := prefixIfNotEmpty("@", v.Mean+stdDevStr)
	unitStr := prefixIfNotEmpty(" ", v.Unit)

	return fmt.Sprintf("%s%s%s", v.Total, meanStr, unitStr)
}

func formatExecutionSummary(summary stats.ExecutionStatsSummary) string {
	type summaryField struct {
		key   string
		value string
	}

	fields := []summaryField{
		{"checkpoint_time", summary.CheckpointTime},
		{"execution_end_timestamp", summary.ExecutionEndTimestamp},
		{"execution_start_timestamp", summary.ExecutionStartTimestamp},
		{"num_checkpoints", summary.NumCheckPoints.String()},
		{"num_executions", summary.NumExecutions},
	}

	var executionSummaryStrings []string
	for _, field := range fields {
		if field.value == "" {
			continue
		}

		value := field.value
		if field.key == "execution_start_timestamp" || field.key == "execution_end_timestamp" {
			formattedValue, err := tryToTimestampStr(field.value)
			if err != nil {
				value = fmt.Sprintf("%s (error: %v)", field.value, err)
			} else {
				value = formattedValue
			}
		}

		const indentLevel = 3
		executionSummaryStrings = append(executionSummaryStrings,
			fmt.Sprintf("%s%s: %s\n", strings.Repeat(" ", indentLevel), field.key, value))
	}

	if len(executionSummaryStrings) == 0 {
		return ""
	}

	sort.Strings(executionSummaryStrings)

	var executionSummaryBuf bytes.Buffer
	fmt.Fprintln(&executionSummaryBuf, "execution_summary:")
	fmt.Fprint(&executionSummaryBuf, strings.Join(executionSummaryStrings, ""))
	return executionSummaryBuf.String()
}
