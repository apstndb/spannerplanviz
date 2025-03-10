package stats

import (
	"encoding/json"
	"fmt"
)

type ExecutionStatsHistogram struct {
	Count      string `json:"count"`
	Percentage string `json:"percentage"`
	LowerBound string `json:"lower_bound"`
	UpperBound string `json:"upper_bound"`
}

type ExecutionStatsValue struct {
	Unit         string                    `json:"unit"`
	Total        string                    `json:"total"`
	Mean         string                    `json:"mean"`
	StdDeviation string                    `json:"std_deviation"`
	Histogram    []ExecutionStatsHistogram `json:"histogram"`
}

func (v ExecutionStatsValue) String() string {
	if v.Unit == "" {
		return v.Total
	} else {
		return fmt.Sprintf("%s %s", v.Total, v.Unit)
	}
}

type ExecutionStatsSummary struct {
	NumExecutions           string      `json:"num_executions"`
	CheckpointTime          string      `json:"checkpoint_time"`
	ExecutionEndTimestamp   string      `json:"execution_end_timestamp"`
	ExecutionStartTimestamp string      `json:"execution_start_timestamp"`
	NumCheckPoints          json.Number `json:"num_checkpoints"`
}

type ExecutionStats struct {
	DiskUsageKBytes                ExecutionStatsValue   `json:"Disk Usage (KBytes)"`
	DiskWriteLatencyMsecs          ExecutionStatsValue   `json:"Disk Write Latency (msecs)"`
	PeekBufferingMemoryUsageKBytes ExecutionStatsValue   `json:"Peak Buffering Memory Usage (KBytes)"`
	PeakMemoryUsageKBytes          ExecutionStatsValue   `json:"Peak Memory Usage (KBytes)"`
	RowsSpooled                    ExecutionStatsValue   `json:"Rows Spooled"`
	Rows                           ExecutionStatsValue   `json:"rows"`
	Latency                        ExecutionStatsValue   `json:"latency"`
	CpuTime                        ExecutionStatsValue   `json:"cpu_time"`
	DeletedRows                    ExecutionStatsValue   `json:"deleted_rows"`
	FilesystemDelaySeconds         ExecutionStatsValue   `json:"filesystem_delay_seconds"`
	FilteredRows                   ExecutionStatsValue   `json:"filtered_rows"`
	RemoteCalls                    ExecutionStatsValue   `json:"remote_calls"`
	ScannedRows                    ExecutionStatsValue   `json:"scanned_rows"`
	ExecutionSummary               ExecutionStatsSummary `json:"execution_summary"`
}
