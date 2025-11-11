package embed

import (
	"embed"
	"encoding/base64"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"strings"
)

//go:embed *.cloud.config.yaml
//go:embed tpcc.ts
var Content embed.FS

const (
	stroppyScript = "stroppy.cloud.config.yaml"
	orioleScript  = "oriole.cloud.config.yaml"
)

func getTpccScript() ([]byte, error) {
	return Content.ReadFile("tpcc.ts")
}

func GetOrioleInstallScript() (*panel.Script, error) {
	content, err := Content.ReadFile(orioleScript)
	if err != nil {
		return nil, err
	}
	return &panel.Script{
		Body: content,
	}, nil
}

func GetStroppyInstallScript() (*panel.Script, error) {
	content, err := Content.ReadFile(stroppyScript)
	if err != nil {
		return nil, err
	}
	script, err := getTpccScript()
	if err != nil {
		return nil, err
	}
	content = []byte(strings.ReplaceAll(string(content), "${tpcc.ts}", base64.StdEncoding.EncodeToString(script)))
	return &panel.Script{
		Body: content,
	}, nil
}
