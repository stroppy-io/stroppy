VERSION=$(shell git describe --tags --always 2>/dev/null || echo "0.0.0")
LOCAL_BIN:=$(CURDIR)/bin
PATH:=$(PATH):$(LOCAL_BIN)
GOPROXY:=https://goproxy.io,direct

default: help

.PHONY: help
help: # Show help in Makefile
	@grep -E '^[a-zA-Z0-9 _-]+:.*#'  Makefile | sort | while read -r l; do printf "\033[1;32m$$(echo $$l | cut -f 1 -d':')\033[00m:$$(echo $$l | cut -f 2- -d'#')\n"; done

.PHONY: .install-linter
.install-linter: # Install golangci-lint
	$(info Installing golangci-lint...)
	mkdir -p $(LOCAL_BIN)
	GOBIN=$(LOCAL_BIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: .bin-deps
.bin-deps: .install-linter # Install binary dependencies in ./bin
	$(info Installing binary dependencies...)
	mkdir -p $(LOCAL_BIN)

GOPROXY:=https://goproxy.io,direct
.PHONY: .app-deps
.app-deps: # Install application dependencies in ./bin
	GOPROXY=$(GOPROXY) go mod tidy

.PHONY: linter
linter: # Start linter
	$(LOCAL_BIN)/golangci-lint cache clean
	$(LOCAL_BIN)/golangci-lint --config $(CURDIR)/.golangci.yml run

.PHONY: linter_fix
linter_fix: # Start linter with possible fixes
	$(LOCAL_BIN)/golangci-lint cache clean
	$(LOCAL_BIN)/golangci-lint --config $(CURDIR)/.golangci.yml run --fix

.PHONY: tests
tests: # Run tests with coverage
	go test -race ./... -coverprofile=coverage.out

STROPPY_BIN_NAME=stroppy
STROPPY_OUT_FILE=$(CURDIR)/build/$(STROPPY_BIN_NAME)
.PHONY: build
build: # Build binary stroppy
	go build -v -o $(STROPPY_OUT_FILE) $(CURDIR)/cmd/stroppy

