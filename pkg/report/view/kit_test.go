package view

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func TestKitAssetsRemainSelfContained(t *testing.T) {
	t.Parallel()

	style := string(embeddedStyle())

	fonts, err := fs.Glob(kitAssets, "kit/fonts/*/*.woff2")
	if err != nil {
		t.Fatal(err)
	}

	if len(fonts) != 5 || strings.Count(style, "data:font/woff2;base64,") != len(fonts) ||
		strings.Contains(style, `url("./`) || strings.Contains(style, "@import") {
		t.Fatal("report fonts depend on external files")
	}

	for _, expected := range []string{
		"--st-canvas: #15171c", "--st-canvas: #f3f4f6", "@layer stroppy-kit", ".st-doc .st-stat",
		"prefers-color-scheme: light", "SIL OPEN FONT LICENSE", "Roboto Mono", "Inter",
	} {
		if !strings.Contains(style, expected) {
			t.Errorf("kit styling or attribution missing: %s", expected)
		}
	}

	var output bytes.Buffer

	data := []byte(`{"kind":"run","schema":4,"workload":"custom","status":"completed"}`)
	if err := Render(&output, data, ""); err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{`class="st-doc"`, `class="brand-mark"`, `data-tone="success"`, `class="st-button"`} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("kit component missing: %s", expected)
		}
	}

	if !strings.Contains(strings.Split(output.String(), "<script>")[0], `data-tone="success"`) {
		t.Fatal("kit styling requires script execution")
	}
}

func TestKitFontsAllowedByServerPolicy(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	Handler(&report.History{Directory: t.TempDir()}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	policy := response.Header().Get("Content-Security-Policy")
	if response.Code != http.StatusOK || !strings.Contains(policy, "font-src data:") {
		t.Fatal("server blocks embedded kit fonts")
	}
}
