package queryplan

import (
	"errors"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/goccy/go-yaml"

	"github.com/apstndb/spannerplanviz/protoyaml"
)

func ExtractQueryPlan(b []byte) (*sppb.ResultSetStats, *sppb.StructType, error) {
	var jsonObj map[string]interface{}
	err := yaml.Unmarshal(b, &jsonObj)
	if err != nil {
		return nil, nil, err
	}

	if _, ok := jsonObj["queryPlan"]; ok {
		var rss sppb.ResultSetStats
		err = protoyaml.Unmarshal(b, &rss)
		if err != nil {
			return nil, nil, err
		}
		return &rss, nil, nil
	} else if _, ok := jsonObj["planNodes"]; ok {
		var qp sppb.QueryPlan
		err = protoyaml.Unmarshal(b, &qp)
		if err != nil {
			return nil, nil, err
		}
		return &sppb.ResultSetStats{QueryPlan: &qp}, nil, nil
	} else if _, ok := jsonObj["stats"]; ok {
		var rs sppb.ResultSet
		err = protoyaml.Unmarshal(b, &rs)
		if err != nil {
			return nil, nil, err
		}
		return rs.GetStats(), rs.GetMetadata().GetRowType(), nil
	}
	return nil, nil, errors.New("unknown input format")
}
