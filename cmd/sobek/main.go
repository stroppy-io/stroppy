package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/grafana/sobek"
	"github.com/grafana/sobek/file"
	"github.com/grafana/sobek/parser"
)

func transpileTypeScript(entry string) (code string, err error) {
	entryAbs, _ := filepath.Abs(entry)
	dirAbs := filepath.Dir(entryAbs)
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{entryAbs},
		Bundle:      true,
		Platform:    api.PlatformNode,
		// Platform: api.PlatformNeutral,
		Format: api.FormatESModule,
		// Format:        api.FormatCommonJS,
		Target:            api.ES2017,
		Sourcemap:         api.SourceMapInline,
		Write:             false, // keep outputs in-memory
		LogLevel:          api.LogLevelWarning,
		AbsWorkingDir:     dirAbs,
		External:          []string{"k6/x/*", "k6/*"},
		MainFields:        []string{"module", "main"},
		ResolveExtensions: []string{".ts", ".tsx", ".js", ".mjs", ".json"},
		Loader: map[string]api.Loader{
			".ts":   api.LoaderTS,
			".tsx":  api.LoaderTSX,
			".js":   api.LoaderJS,
			".mjs":  api.LoaderJS,
			".json": api.LoaderJSON,
		},
	})
	if len(result.Errors) > 0 {
		return "", errors.New(result.Errors[0].Text)
	}
	if len(result.OutputFiles) == 0 {
		return "", errors.New("no output from esbuild")
	}
	content := string(result.OutputFiles[0].Contents)
	return content, nil
}

type cacheEntry struct {
	js     string
	mtime  time.Time
	srcKey string
}

var bundleCache = map[string]cacheEntry{}

func cacheKey(path string) (string, time.Time, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", time.Time{}, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", time.Time{}, err
	}
	return abs, fi.ModTime(), nil
}

func loadOrBuild(entry string) (string, error) {
	key, mt, err := cacheKey(entry)
	if err != nil {
		return "", err
	}
	if ce, ok := bundleCache[key]; ok && !mt.After(ce.mtime) {
		return ce.js, nil
	}
	js, err := transpileTypeScript(entry)
	if err != nil {
		return "", err
	}
	bundleCache[key] = cacheEntry{js: js, mtime: mt, srcKey: key}
	return js, nil
}

type pass struct {
}

func (_ pass) parseConfig() {

}

func main() {
	vm := sobek.New()
	vm.SetParserOptions(parser.IsModule)

	// Optional: make Go struct/field names more JS-like (lowercase + tags)
	vm.SetFieldNameMapper(
		sobek.TagFieldNameMapper("json", true),
	)

	var configPath string
	flag.StringVar(&configPath, "i", "stroppy.json", "path to config file (json|yaml|yml)")
	flag.Parse()

	_, err := os.Stat(configPath)
	if err != nil {
		fmt.Fprintf(os.Stdout, "can't find file '%s': %s\n", configPath, err)
		flag.Usage()

		return
	}

	// var configBytes []byte

	// configBytes, err = os.ReadFile(configPath)
	// if err != nil {
	// 	fmt.Fprintf(os.Stdout, "can't read file: %s\n", err)

	// 	return
	// }

	js, err := loadOrBuild(configPath)

	// js, _, err := StripTypes(string(configBytes), configPath)
	// if err != nil {
	// 	log.Fatalf("build error: %v\n", err)
	// }

	vm.Set("stroppy", pass{})

	os.WriteFile("./test_bundle.js", []byte(js), 0o655)
	_, err = vm.RunString(js)
	if err != nil {
		log.Fatalf("js run error: %v\n", err)
	}
	obj := vm.Get("insert")
	fn, _ := sobek.AssertFunction(obj)
	fn(sobek.Undefined())
}

func StripTypes(src, filename string) (code string, srcMap []byte, err error) {
	opts := api.TransformOptions{
		Loader:         api.LoaderTS,
		Sourcefile:     filename,
		Target:         api.ESNext,
		Format:         api.FormatDefault,
		Sourcemap:      api.SourceMapExternal,
		SourcesContent: api.SourcesContentInclude,
		LegalComments:  api.LegalCommentsNone,
		Platform:       api.PlatformNeutral,
		LogLevel:       api.LogLevelSilent,
		Charset:        api.CharsetUTF8,
	}

	result := api.Transform(src, opts)

	if hasError, err := esbuildCheckError(&result); hasError {
		return "", nil, err
	}

	return string(result.Code), result.Map, nil
}

func esbuildCheckError(result *api.TransformResult) (bool, error) {
	if len(result.Errors) == 0 {
		return false, nil
	}

	msg := result.Errors[0]
	err := &parser.Error{Message: msg.Text}

	if msg.Location != nil {
		err.Position = file.Position{
			Filename: msg.Location.File,
			Line:     msg.Location.Line,
			Column:   msg.Location.Column,
		}
	}

	return true, err
}
