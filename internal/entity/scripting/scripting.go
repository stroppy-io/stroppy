package scripting

import (
	"bytes"
	"encoding/base64"
	"text/template"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
)

func base64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func BuildBase64Script(params *crossplane.Deployment_Strategy_Scripting) (string, error) {
	// Load raw template file
	templateText, err := GetShScriptTemplate()
	if err != nil {
		return "", err
	}

	// Parse template with functions
	tmpl, err := template.
		New("script").
		Funcs(template.FuncMap{
			"base64Encode": base64Encode,
		}).
		Parse(string(templateText))
	if err != nil {
		return "", err
	}

	// Output buffer
	var buf bytes.Buffer

	// Execute template with given params
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", err
	}

	return base64Encode(buf.Bytes()), nil
}
