package view

import (
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"io/fs"
	"strings"
)

//go:embed kit/tokens.css kit/base.css kit/fonts.css kit/fonts/*/*.woff2 kit/logo/mark.svg kit/OFL.txt kit/chart.js kit/chart.css kit/UPLOT-LICENSE.txt

var kitAssets embed.FS

func kitAsset(name string) string {
	data, err := kitAssets.ReadFile("kit/" + name)
	if err != nil {
		panic(err)
	}

	return string(data)
}

func kitStyle() string {
	tokens := kitAsset("tokens.css")
	_, light, _ := strings.Cut(tokens, `[data-theme="light"]`)
	fonts := kitAsset("fonts.css")

	files, err := fs.Glob(kitAssets, "kit/fonts/*/*.woff2")
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		data, err := kitAssets.ReadFile(file)
		if err != nil {
			panic(err)
		}

		fonts = strings.ReplaceAll(fonts, "./"+strings.TrimPrefix(file, "kit/"),
			"data:font/woff2;base64,"+base64.StdEncoding.EncodeToString(data))
	}

	return "/*\n" + kitAsset("OFL.txt") + "*/\n" + tokens +
		"\n@media (prefers-color-scheme: light) { :root:not([data-theme=dark]) " + light + "}\n" +
		"\n@media print { :root " + light + "}\n" + fonts +
		"\n@layer stroppy-kit {\n" + kitAsset("base.css") + "\n}\n" + kitAsset("chart.css") + stylesheet
}

func kitMark() template.HTML {
	mark := strings.Replace(kitAsset("logo/mark.svg"), `<svg `,
		`<svg class="brand-mark" aria-hidden="true" focusable="false" `, 1)

	return template.HTML(mark) //nolint:gosec // inspected embedded mark; no report content.
}

func statusTone(status any) string {
	switch fmt.Sprint(status) {
	case "completed":
		return "success"
	case "failed", "canceled":
		return "error"
	case "completed_with_errors":
		return "warning"
	default:
		return "muted"
	}
}
