// Package runner provides functionality to run TypeScript benchmark scripts with k6.
package runner

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/grafana/sobek"
	"github.com/grafana/sobek/parser"
	"google.golang.org/protobuf/encoding/protojson"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

// ErrNoConfigProvided is returned when script doesn't call defineConfig.
var ErrNoConfigProvided = errors.New("script did not call defineConfig with GlobalConfig")

// ExtractedConfig contains configuration extracted from a TypeScript script.
type ExtractedConfig struct {
	GlobalConfig *stroppy.GlobalConfig
}

// TranspileTypeScript transpiles TypeScript to JavaScript using esbuild.
func TranspileTypeScript(entryPath string) (string, error) {
	entryAbs, err := filepath.Abs(entryPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	dirAbs := filepath.Dir(entryAbs)

	result := api.Build(api.BuildOptions{
		EntryPoints:       []string{entryAbs},
		Bundle:            true,
		Platform:          api.PlatformNode,
		Format:            api.FormatESModule,
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
		return "", fmt.Errorf("esbuild error: %s", result.Errors[0].Text)
	}

	if len(result.OutputFiles) == 0 {
		return "", errors.New("no output from esbuild")
	}

	return string(result.OutputFiles[0].Contents), nil
}

// stroppyStub provides stub implementations for stroppy module functions
// that are used during config extraction (before k6 runtime).
type stroppyStub struct{}

// ParseConfig is a no-op during config extraction.
func (s stroppyStub) ParseConfig(_ []byte) {}

// RunUnit is a no-op during config extraction.
func (s stroppyStub) RunUnit(_ []byte) []byte { return nil }

// InsertValues is a no-op during config extraction.
func (s stroppyStub) InsertValues(_ []byte, _ int64) []byte { return nil }

// NotifyStep is a no-op during config extraction.
func (s stroppyStub) NotifyStep(_ string, _ int32) {}

// Teardown is a no-op during config extraction.
func (s stroppyStub) Teardown() error { return nil }

// ExtractConfigFromScript extracts GlobalConfig from a TypeScript script.
// The script should call defineConfig(globalConfig) at the top level.
func ExtractConfigFromScript(scriptPath string) (*ExtractedConfig, error) {
	// Transpile TypeScript to JavaScript
	jsCode, err := TranspileTypeScript(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("failed to transpile TypeScript: %w", err)
	}

	return ExtractConfigFromJS(jsCode)
}

// ExtractConfigFromJS extracts GlobalConfig from JavaScript code.
func ExtractConfigFromJS(jsCode string) (*ExtractedConfig, error) {
	vm := sobek.New()
	vm.SetParserOptions(parser.IsModule)
	vm.SetFieldNameMapper(sobek.UncapFieldNameMapper())

	// Inject TextEncoder/TextDecoder polyfill
	if err := injectEncoderPolyfill(vm); err != nil {
		return nil, fmt.Errorf("failed to inject encoder polyfill: %w", err)
	}

	// Set up stroppy stub module
	if err := vm.Set("stroppy", stroppyStub{}); err != nil {
		return nil, fmt.Errorf("failed to set stroppy stub: %w", err)
	}

	// Set up config extraction
	var extractedConfig *stroppy.GlobalConfig
	configCallback := func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) == 0 {
			return sobek.Undefined()
		}

		// Get the config object from JavaScript
		configObj := call.Argument(0).Export()
		if configObj == nil {
			return sobek.Undefined()
		}

		// Convert to JSON and then to protobuf
		jsonBytes, err := protojson.MarshalOptions{
			UseProtoNames: true,
		}.Marshal(&stroppy.GlobalConfig{})

		// Try to marshal the JS object to JSON first
		jsonStr, err := vm.RunString(fmt.Sprintf("JSON.stringify(%s)", call.Argument(0).String()))
		if err != nil {
			return sobek.Undefined()
		}

		// Parse JSON to GlobalConfig
		config := &stroppy.GlobalConfig{}
		if err := protojson.Unmarshal([]byte(jsonStr.String()), config); err != nil {
			// Try alternative parsing - the object might already be in the right format
			_ = jsonBytes // suppress unused warning
			return sobek.Undefined()
		}

		extractedConfig = config
		return sobek.Undefined()
	}

	if err := vm.Set("defineConfig", configCallback); err != nil {
		return nil, fmt.Errorf("failed to set defineConfig: %w", err)
	}

	// Run the script
	if _, err := vm.RunString(jsCode); err != nil {
		return nil, fmt.Errorf("failed to run script: %w", err)
	}

	if extractedConfig == nil {
		return nil, ErrNoConfigProvided
	}

	return &ExtractedConfig{
		GlobalConfig: extractedConfig,
	}, nil
}

// injectEncoderPolyfill injects TextEncoder/TextDecoder polyfill into the VM.
func injectEncoderPolyfill(vm *sobek.Runtime) error {
	// Minified TextEncoder/TextDecoder polyfill
	const encodersDef = `'use strict';(function(r){function x(){}function y(){}var z=String.fromCharCode,v={}.toString,A=v.call(r.SharedArrayBuffer),B=v(),q=r.Uint8Array,t=q||Array,w=q?ArrayBuffer:t,C=w.isView||function(g){return g&&"length"in g},D=v.call(w.prototype);w=y.prototype;var E=r.TextEncoder,a=new (q?Uint16Array:t)(32);x.prototype.decode=function(g){if(!C(g)){var l=v.call(g);if(l!==D&&l!==A&&l!==B)throw TypeError("Failed to execute 'decode' on 'TextDecoder': The provided value is not of type '(ArrayBuffer or ArrayBufferView)'");
g=q?new t(g):g||[]}for(var f=l="",b=0,c=g.length|0,u=c-32|0,e,d,h=0,p=0,m,k=0,n=-1;b<c;){for(e=b<=u?32:c-b|0;k<e;b=b+1|0,k=k+1|0){d=g[b]&255;switch(d>>4){case 15:m=g[b=b+1|0]&255;if(2!==m>>6||247<d){b=b-1|0;break}h=(d&7)<<6|m&63;p=5;d=256;case 14:m=g[b=b+1|0]&255,h<<=6,h|=(d&15)<<6|m&63,p=2===m>>6?p+4|0:24,d=d+256&768;case 13:case 12:m=g[b=b+1|0]&255,h<<=6,h|=(d&31)<<6|m&63,p=p+7|0,b<c&&2===m>>6&&h>>p&&1114112>h?(d=h,h=h-65536|0,0<=h&&(n=(h>>10)+55296|0,d=(h&1023)+56320|0,31>k?(a[k]=n,k=k+1|0,n=-1):
(m=n,n=d,d=m))):(d>>=8,b=b-d-1|0,d=65533),h=p=0,e=b<=u?32:c-b|0;default:a[k]=d;continue;case 11:case 10:case 9:case 8:}a[k]=65533}f+=z(a[0],a[1],a[2],a[3],a[4],a[5],a[6],a[7],a[8],a[9],a[10],a[11],a[12],a[13],a[14],a[15],a[16],a[17],a[18],a[19],a[20],a[21],a[22],a[23],a[24],a[25],a[26],a[27],a[28],a[29],a[30],a[31]);32>k&&(f=f.slice(0,k-32|0));if(b<c){if(a[0]=n,k=~n>>>31,n=-1,f.length<l.length)continue}else-1!==n&&(f+=z(n));l+=f;f=""}return l};w.encode=function(g){g=void 0===g?"":""+g;var l=g.length|
0,f=new t((l<<1)+8|0),b,c=0,u=!q;for(b=0;b<l;b=b+1|0,c=c+1|0){var e=g.charCodeAt(b)|0;if(127>=e)f[c]=e;else{if(2047>=e)f[c]=192|e>>6;else{a:{if(55296<=e)if(56319>=e){var d=g.charCodeAt(b=b+1|0)|0;if(56320<=d&&57343>=d){e=(e<<10)+d-56613888|0;if(65535<e){f[c]=240|e>>18;f[c=c+1|0]=128|e>>12&63;f[c=c+1|0]=128|e>>6&63;f[c=c+1|0]=128|e&63;continue}break a}e=65533}else 57343>=e&&(e=65533);!u&&b<<1<c&&b<<1<(c-7|0)&&(u=!0,d=new t(3*l),d.set(f),f=d)}f[c]=224|e>>12;f[c=c+1|0]=128|e>>6&63}f[c=c+1|0]=128|e&63}}return q?
f.subarray(0,c):f.slice(0,c)};E||(r.TextDecoder=x,r.TextEncoder=y)})(globalThis)`

	_, err := vm.RunString(encodersDef)
	return err
}

