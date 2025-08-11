package config

import "fmt"

type Format uint

func (f Format) FormatConfigName(name string) string {
	return name + "." + f.String()
}

func NewFormatFromString(format string) (Format, error) {
	switch format {
	case "json":
		return FormatJSON, nil
	case "yaml":
		return FormatYAML, nil
	default:
		return 0, fmt.Errorf("unknown format: %s", format) //nolint: err113
	}
}

func (f Format) String() string {
	switch f {
	case FormatJSON:
		return "json"
	case FormatYAML:
		return "yaml"
	default:
		panic("unknown format type")
	}
}

const (
	FormatJSON Format = iota
	FormatYAML
)

var FormatIDs = map[Format][]string{ //nolint: gochecknoglobals // allow for static data
	FormatJSON: {FormatJSON.String()},
	FormatYAML: {FormatYAML.String()},
}
