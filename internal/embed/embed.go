package embed

import (
	"embed"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

//go:embed *.cloud.config.yaml
var Content embed.FS

const (
	stroppyScript = "stroppy.cloud.config.yaml"
	orioleScript  = "oriole.cloud.config.yaml"
)

func GetOrioleInstallScript() (*panel.Script, error) {
	content, err := Content.ReadFile(orioleScript)
	if err != nil {
		return nil, err
	}
	//output := bytes.ReplaceAll(content, []byte("\r\n"), []byte(`\n`))
	//s, err := json.Marshal(content)
	//if err != nil {
	//	return nil, err
	//}
	return &panel.Script{
		Body: content,
	}, nil
}

func GetStroppyInstallScript() (*panel.Script, error) {
	content, err := Content.ReadFile(stroppyScript)
	if err != nil {
		return nil, err
	}
	//output := bytes.ReplaceAll(content, []byte("\r\n"), []byte(`\n`))
	return &panel.Script{
		Body: content,
	}, nil
}
