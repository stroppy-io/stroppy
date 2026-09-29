package toolchain

import (
	"fmt"
	"runtime"
)

const downloadBaseURL = "https://go.dev/dl/"

type release struct {
	File   string
	SHA256 string
}

var releases = map[string]release{
	"darwin/amd64": {
		File:   "go1.26.8.darwin-amd64.tar.gz",
		SHA256: "186be014105aa6542b767d2c6ed5cca10a0214bdff809ef1724022a8c7894150",
	},
	"darwin/arm64": {
		File:   "go1.26.8.darwin-arm64.tar.gz",
		SHA256: "a012b25b571bd0138a03dcd25375ceba866fe5ca822f426d2c66a4de56fd3f4b",
	},
	"linux/amd64": {
		File:   "go1.26.8.linux-amd64.tar.gz",
		SHA256: "d0f743b33e8d8945e6b1f432edd15785c70507121d6e2a723b21285eddf8b57b",
	},
	"linux/arm64": {
		File:   "go1.26.8.linux-arm64.tar.gz",
		SHA256: "211ffced9dcb9633a55eac6364816ec0ddd951389a740e88fa8b3337971bdda0",
	},
}

func hostRelease() (release, error) {
	target := runtime.GOOS + "/" + runtime.GOARCH
	value, ok := releases[target]
	if !ok {
		return release{}, fmt.Errorf("%w: %s", ErrUnsupportedHost, target)
	}

	return value, nil
}
