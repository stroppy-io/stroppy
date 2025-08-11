package protoyaml

import (
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/yaml"
)

var (
	nonStrict = protojson.UnmarshalOptions{DiscardUnknown: true}  //nolint: gochecknoglobals
	strict    = protojson.UnmarshalOptions{DiscardUnknown: false} //nolint: gochecknoglobals
)

// Marshal writes the given proto.Message in YAML format.
func Marshal(m proto.Message) ([]byte, error) {
	json, err := protojson.Marshal(m)
	if err != nil {
		return nil, err
	}

	return yaml.JSONToYAML(json)
}

// Unmarshal reads the given []byte into the given proto.Message, discarding
// any unknown fields in the input.
func Unmarshal(b []byte, message proto.Message) error {
	json, err := yaml.YAMLToJSON(b)
	if err != nil {
		return err
	}

	return nonStrict.Unmarshal(json, message)
}

// UnmarshalStrict reads the given []byte into the given proto.Message. If there
// are any unknown fields in the input, an error is returned.
func UnmarshalStrict(b []byte, message proto.Message) error {
	json, err := yaml.YAMLToJSON(b)
	if err != nil {
		return err
	}

	return strict.Unmarshal(json, message)
}
