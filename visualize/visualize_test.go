package visualize

import (
	"context"
	"embed"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/goccy/go-graphviz"
	"github.com/google/go-cmp/cmp"
	"github.com/samber/lo"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/apstndb/spannerplanviz/option"
)

//go:embed testdata
var testdataFS embed.FS

func TestRenderImage(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		format     graphviz.Format
		param      option.Options
		wantString string
		wantErr    bool
	}{
		{
			name:   "svg full",
			input:  lo.Must(testdataFS.ReadFile("testdata/dca_profile.json")),
			format: graphviz.SVG,
			param: option.Options{
				Full:              true,
				NonVariableScalar: true,
				VariableScalar:    true,
				Metadata:          true,
				ExecutionStats:    true,
				ExecutionSummary:  true,
				SerializeResult:   true,
			},
			wantString: string(lo.Must(testdataFS.ReadFile("testdata/full.svg"))),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resultSet sppb.ResultSet
			err := protojson.Unmarshal(tt.input, &resultSet)
			if err != nil {
				t.Errorf("protojson.Unmarshal() error = %v", err)
				return
			}

			writer := &strings.Builder{}
			err = RenderImage(context.Background(), resultSet.GetMetadata().GetRowType(), resultSet.GetStats(), tt.format, writer, tt.param)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderImage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			got := writer.String()
			if diff := cmp.Diff(got, tt.wantString); diff != "" {
				t.Errorf("RenderImage() mismatch (-got +want):\n%s", diff)
			}
		})
	}
}
