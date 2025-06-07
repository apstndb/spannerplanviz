package visualize

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestToLeftAlignedText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "single line",
			input: "hello world",
			want:  "hello world<br align=\"left\" />",
		},
		{
			name:  "multiple lines",
			input: "line1\nline2\nline3",
			want:  "line1<br align=\"left\" />line2<br align=\"left\" />line3<br align=\"left\" />",
		},
		{
			name:  "html escape",
			input: "a < b & c > d",
			want:  `a &lt; b &amp; c &gt; d<br align="left" />`,
		},
		{
			name:  "trailing newline",
			input: "line1\n",
			want:  "line1<br align=\"left\" />",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toLeftAlignedText(tt.input)
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("toLeftAlignedText() mismatch (-got +want):\n%s", diff)
			}
		})
	}
}

func TestTryToTimestampStr(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantError bool
	}{
		{
			name:      "valid timestamp",
			input:     "1678881600.123456",
			want:      "2023-03-15T12:00:00.123456Z",
			wantError: false,
		},
		{
			name:      "valid timestamp - zero padded microseconds",
			input:     "1678881600.000123",
			want:      "2023-03-15T12:00:00.000123Z",
			wantError: false,
		},
		{
			name:      "valid timestamp with less than 6 microseconds",
			input:     "1678881600.123",
			want:      "",
			wantError: true,
		},
		{
			name:      "valid timestamp without microseconds",
			input:     "1678881600",
			want:      "",
			wantError: true,
		},
		{
			name:      "invalid format - too many microseconds",
			input:     "1678886400.1234567",
			want:      "",
			wantError: true,
		},
		{
			name:      "invalid format - non-numeric seconds",
			input:     "abc.123456",
			want:      "",
			wantError: true,
		},
		{
			name:      "invalid format - non-numeric microseconds",
			input:     "1678886400.def",
			want:      "",
			wantError: true,
		},
		{
			name:      "empty string",
			input:     "",
			want:      "",
			wantError: true,
		},
		{
			name:      "zero timestamp",
			input:     "0.0",
			want:      "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tryToTimestampStr(tt.input)
			if (err != nil) != tt.wantError {
				t.Errorf("tryToTimestampStr() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if !tt.wantError {
				if diff := cmp.Diff(got, tt.want); diff != "" {
					t.Errorf("tryToTimestampStr() mismatch (-got +want):\n%s", diff)
				}
			}
		})
	}
}

func TestFormatExecutionStatsValue(t *testing.T) {
	tests := []struct {
		name  string
		input *structpb.Value
		want  string
	}{
		{
			name: "all fields present",
			input: structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{
					"total":         structpb.NewStringValue("100"),
					"unit":          structpb.NewStringValue("rows"),
					"mean":          structpb.NewStringValue("10"),
					"std_deviation": structpb.NewStringValue("2"),
				},
			}),
			want: "100@10±2 rows",
		},
		{
			name: "no std_deviation",
			input: structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{
					"total":         structpb.NewStringValue("50"),
					"unit":          structpb.NewStringValue("bytes"),
					"mean":          structpb.NewStringValue("5"),
					"std_deviation": structpb.NewStringValue(""),
				},
			}),
			want: "50@5 bytes",
		},
		{
			name: "no mean or std_deviation",
			input: structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{
					"total":         structpb.NewStringValue("200"),
					"unit":          structpb.NewStringValue("ms"),
					"mean":          structpb.NewStringValue(""),
					"std_deviation": structpb.NewStringValue(""),
				},
			}),
			want: "200 ms",
		},
		{
			name: "empty struct",
			input: structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{},
			}),
			want: " ", // This is what the current implementation returns for empty fields
		},
		{
			name: "missing total",
			input: structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{
					"unit":          structpb.NewStringValue("rows"),
					"mean":          structpb.NewStringValue("10"),
					"std_deviation": structpb.NewStringValue("2"),
				},
			}),
			want: "@10±2 rows",
		},
		{
			name: "missing unit",
			input: structpb.NewStructValue(&structpb.Struct{
				Fields: map[string]*structpb.Value{
					"total":         structpb.NewStringValue("100"),
					"mean":          structpb.NewStringValue("10"),
					"std_deviation": structpb.NewStringValue("2"),
				},
			}),
			want: "100@10±2 ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatExecutionStatsValue(tt.input)
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("formatExecutionStatsValue() mismatch (-got +want):\n%s", diff)
			}
		})
	}
}

func TestFormatMetadata(t *testing.T) {
	tests := []struct {
		name         string
		input        map[string]*structpb.Value
		hideMetadata []string
		want         string
	}{
		{
			name: "standard metadata",
			input: map[string]*structpb.Value{
				"key1": structpb.NewStringValue("value1"),
				"key2": structpb.NewNumberValue(123),
				"key3": structpb.NewBoolValue(true),
			},
			hideMetadata: nil,
			want:         "key1=value1\nkey2=123\nkey3=true\n",
		},
		{
			name: "with hidden metadata",
			input: map[string]*structpb.Value{
				"key1":    structpb.NewStringValue("value1"),
				"hide_me": structpb.NewStringValue("hidden_value"),
				"key2":    structpb.NewNumberValue(123),
			},
			hideMetadata: []string{"hide_me"},
			want:         "key1=value1\nkey2=123\n",
		},
		{
			name: "with internal metadata fields",
			input: map[string]*structpb.Value{
				"key1":      structpb.NewStringValue("value1"),
				"call_type": structpb.NewStringValue("Local"),
				"scan_type": structpb.NewStringValue("Full"),
				"key2":      structpb.NewNumberValue(123),
			},
			hideMetadata: nil,
			want:         "key1=value1\nkey2=123\n",
		},
		{
			name:         "empty metadata",
			input:        map[string]*structpb.Value{},
			hideMetadata: nil,
			want:         "",
		},
		{
			name: "only internal metadata fields",
			input: map[string]*structpb.Value{
				"call_type": structpb.NewStringValue("Local"),
				"scan_type": structpb.NewStringValue("Full"),
			},
			hideMetadata: nil,
			want:         "",
		},
		{
			name:         "nil metadata map",
			input:        nil,
			hideMetadata: nil,
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMetadata(tt.input, tt.hideMetadata)
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("formatMetadata() mismatch (-got +want):\n%s", diff)
			}
		})
	}
}
