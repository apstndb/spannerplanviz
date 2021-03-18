package queryplan

import (
	"errors"

	"github.com/apstndb/spannerplanviz/protoyaml"
	"google.golang.org/genproto/googleapis/spanner/v1"
	"gopkg.in/yaml.v3"
)
func ExtractQueryPlan(b []byte) (*spanner.ResultSetStats, *spanner.StructType, error) {
	var jsonObj map[string]interface{}
	err := yaml.Unmarshal(b, &jsonObj)
	if err != nil {
		return nil, nil, err
	}

	if _, ok := jsonObj["queryPlan"]; ok {
		var rss spanner.ResultSetStats
		err = protoyaml.Unmarshal(b, &rss)
		if err != nil {
			return nil,nil,  err
		}
		return &rss, nil, nil
	} else if _, ok := jsonObj["planNodes"]; ok {
		var qp spanner.QueryPlan
		err = protoyaml.Unmarshal(b, &qp)
		if err != nil {
			return nil, nil,  err
		}
		return &spanner.ResultSetStats{QueryPlan: &qp}, nil, nil
	} else if _, ok := jsonObj["stats"]; ok {
		var rs spanner.ResultSet
		err = protoyaml.Unmarshal(b, &rs)
		if err != nil {
			return nil, nil,  err
		}
		return rs.GetStats(), rs.GetMetadata().GetRowType(), nil
	}
	return nil, nil, errors.New("unknown input format")
}