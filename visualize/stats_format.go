package visualize

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spannerplan/stats"
	"google.golang.org/protobuf/types/known/structpb"
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

func executionStatsToMap(node *sppb.PlanNode, es *stats.ExecutionStats) map[string]string {
	if es == nil {
		return nil
	}

	statsMap := make(map[string]string)
	for _, field := range executionStatFields(*es) {
		if formatted := formatExecutionStatsValue(field.val); formatted != "" {
			statsMap[field.key] = formatted
		}
	}
	mergeUnknownExecutionStats(node, statsMap)
	return statsMap
}

func knownExecutionStatKeys() map[string]struct{} {
	keys := make(map[string]struct{}, len(executionStatFields(stats.ExecutionStats{}))+1)
	for _, field := range executionStatFields(stats.ExecutionStats{}) {
		keys[field.key] = struct{}{}
	}
	keys["execution_summary"] = struct{}{}
	return keys
}

func mergeUnknownExecutionStats(node *sppb.PlanNode, statsMap map[string]string) {
	if node == nil || statsMap == nil {
		return
	}

	knownKeys := knownExecutionStatKeys()
	for key, valProto := range node.GetExecutionStats().GetFields() {
		if key == "execution_summary" {
			continue
		}
		if _, ok := statsMap[key]; ok {
			continue
		}
		if _, known := knownKeys[key]; known {
			continue
		}
		if formatted := formatExecutionStatsValueFromProto(valProto); formatted != "" {
			statsMap[key] = formatted
		} else {
			statsMap[key] = fmt.Sprint(valProto.AsInterface())
		}
	}
}

func formatExecutionStatsValueFromProto(v *structpb.Value) string {
	if v.GetStructValue() == nil {
		return ""
	}
	fields := v.GetStructValue().GetFields()
	return formatExecutionStatsValue(stats.ExecutionStatsValue{
		Total:        fields["total"].GetStringValue(),
		Unit:         fields["unit"].GetStringValue(),
		Mean:         fields["mean"].GetStringValue(),
		StdDeviation: fields["std_deviation"].GetStringValue(),
	})
}

func formatExecutionStatsValue(v stats.ExecutionStatsValue) string {
	stdDevStr := prefixIfNotEmpty("±", v.StdDeviation)
	meanStr := prefixIfNotEmpty("@", v.Mean+stdDevStr)
	unitStr := prefixIfNotEmpty(" ", v.Unit)

	return fmt.Sprintf("%s%s%s", v.Total, meanStr, unitStr)
}

func formatExecutionSummary(node *sppb.PlanNode, summary stats.ExecutionStatsSummary) string {
	lines := typedExecutionSummaryLines(summary)
	mergeUnknownExecutionSummaryLines(node, lines)
	return renderExecutionSummaryLines(lines)
}

func typedExecutionSummaryLines(summary stats.ExecutionStatsSummary) map[string]string {
	lines := make(map[string]string)

	setLine := func(key, value string) {
		if value == "" {
			return
		}
		if key == "execution_start_timestamp" || key == "execution_end_timestamp" {
			formattedValue, err := tryToTimestampStr(value)
			if err != nil {
				value = fmt.Sprintf("%s (error: %v)", value, err)
			} else {
				value = formattedValue
			}
		}
		lines[key] = value
	}

	setLine("checkpoint_time", summary.CheckpointTime)
	setLine("execution_end_timestamp", summary.ExecutionEndTimestamp)
	setLine("execution_start_timestamp", summary.ExecutionStartTimestamp)
	setLine("num_checkpoints", summary.NumCheckPoints.String())
	setLine("num_executions", summary.NumExecutions)
	return lines
}

func mergeUnknownExecutionSummaryLines(node *sppb.PlanNode, lines map[string]string) {
	if node == nil || lines == nil {
		return
	}
	rawSummary, ok := node.GetExecutionStats().GetFields()["execution_summary"]
	if !ok || rawSummary.GetStructValue() == nil {
		return
	}
	for key, valProto := range rawSummary.GetStructValue().GetFields() {
		if _, exists := lines[key]; exists {
			continue
		}
		lines[key] = fmt.Sprint(valProto.AsInterface())
	}
}

func renderExecutionSummaryLines(lines map[string]string) string {
	if len(lines) == 0 {
		return ""
	}

	keys := make([]string, 0, len(lines))
	for key := range lines {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var executionSummaryStrings []string
	for _, key := range keys {
		const indentLevel = 3
		executionSummaryStrings = append(executionSummaryStrings,
			fmt.Sprintf("%s%s: %s\n", strings.Repeat(" ", indentLevel), key, lines[key]))
	}

	var executionSummaryBuf bytes.Buffer
	fmt.Fprintln(&executionSummaryBuf, "execution_summary:")
	fmt.Fprint(&executionSummaryBuf, strings.Join(executionSummaryStrings, ""))
	return executionSummaryBuf.String()
}
