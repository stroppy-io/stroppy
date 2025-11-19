package scripting

import "embed"

//go:embed sh-script.tmpl
var embededContent embed.FS

const shScriptTemplate = "sh-script.tmpl"

func GetShScriptTemplate() ([]byte, error) {
	return embededContent.ReadFile(shScriptTemplate)
}
