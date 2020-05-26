package protoyaml

import (
	"encoding/json"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/goccy/go-yaml"
)

func Unmarshal(b []byte, result proto.Message) error {
	j, err := yamlToJSON(b)
	if err != nil {
		return err
	}
	return protojson.Unmarshal(j, result)
}

func Marshal(m proto.Message) ([]byte, error) {
	j, err := protojson.Marshal(m)
	if err != nil {
		return nil, err
	}
	return jsonToYAML(j)
}

func yamlToJSON(y []byte) ([]byte, error) {
	var i interface{}
	err := yaml.Unmarshal(y, &i)
	if err != nil {
		return nil, err
	}
	return json.Marshal(i)
}

func jsonToYAML(j []byte) ([]byte, error) {
	var i interface{}
	err := json.Unmarshal(j, &i)
	if err != nil {
		return nil, err
	}
	return yaml.Marshal(i)
}
