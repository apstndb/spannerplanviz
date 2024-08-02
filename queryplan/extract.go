package queryplan

import (
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"errors"
	"github.com/apstndb/spannerplanviz/protoyaml"
	"gopkg.in/yaml.v3"
)

func ExtractQueryPlan(b []byte) (*spannerpb.ResultSetStats, *spannerpb.StructType, error) {
	var jsonObj map[string]interface{}
	err := yaml.Unmarshal(b, &jsonObj)
	if err != nil {
		return nil, nil, err
	}

	if _, ok := jsonObj["queryPlan"]; ok {
		var rss spannerpb.ResultSetStats
		err = protoyaml.Unmarshal(b, &rss)
		if err != nil {
			return nil, nil, err
		}
		return &rss, nil, nil
	} else if _, ok := jsonObj["planNodes"]; ok {
		var qp spannerpb.QueryPlan
		err = protoyaml.Unmarshal(b, &qp)
		if err != nil {
			return nil, nil, err
		}
		return &spannerpb.ResultSetStats{QueryPlan: &qp}, nil, nil
	} else if _, ok := jsonObj["stats"]; ok {
		var rs spannerpb.ResultSet
		err = protoyaml.Unmarshal(b, &rs)
		if err != nil {
			return nil, nil, err
		}
		return rs.GetStats(), rs.GetMetadata().GetRowType(), nil
	}
	return nil, nil, errors.New("unknown input format")
}
