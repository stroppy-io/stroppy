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


STATIC_PATH := $(CURDIR)/internal/static
K6_BIN_NAME=stroppy-xk6
GO_OS:= $(shell uname -s | tr '[:upper:]' '[:lower:]')
GO_ARCH:= $(shell uname -m)
ifeq ($(GO_ARCH),x86_64)
    GO_ARCH := amd64
endif
ifeq ($(GO_ARCH),aarch64)
    GO_ARCH := arm64
endif
K6_PKG_NAME   := $(K6_BIN_NAME)_$(GO_OS)_$(GO_ARCH)
K6_ARCHIVE    := $(K6_PKG_NAME).tar.gz
K6_URL        := https://github.com/stroppy-io/stroppy-xk6/releases/latest/download/$(K6_ARCHIVE)
.PHONY: .download-k6
.download-k6:
	@if [ -x "$(STATIC_PATH)/$(K6_BIN_NAME)" ]; then \
			echo "$(STATIC_PATH)/$(K6_BIN_NAME) is already installed."; \
	else \
		wget -q -O $(STATIC_PATH)/$(K6_ARCHIVE) "$(K6_URL)"; \
		tar -xzf $(STATIC_PATH)/$(K6_ARCHIVE) -C $(STATIC_PATH); \
		chmod +x $(STATIC_PATH)/$(K6_BIN_NAME); \
		rm $(STATIC_PATH)/$(K6_ARCHIVE); \
		echo "$(K6_BIN_NAME) installed to $(STATIC_PATH)/$(K6_BIN_NAME)"; \
	fi

STROPPY_PB_TS_NAME=stroppy.pb.ts
STROPPY_PB_TS_PATH=$(STATIC_PATH)/$(STROPPY_PB_TS_NAME)
STROPPY_PB_JS_NAME=stroppy.pb.js
STROPPY_PB_JS_PATH=$(STATIC_PATH)/$(STROPPY_PB_JS_NAME)
STROPPY_PROTO_URL=https://github.com/stroppy-io/stroppy-proto/releases/latest/download
.PHONY: .download-proto-static
.download-proto-static:
	@if [ -x "$(STROPPY_PB_TS_PATH)" ]; then \
		echo "$(STROPPY_PB_TS_PATH) is already installed."; \
	else \
		wget -q -O $(STROPPY_PB_TS_PATH) "$(STROPPY_PROTO_URL)/$(STROPPY_PB_TS_NAME)"; \
		echo "$(STROPPY_PB_TS_NAME) installed to $(STROPPY_PB_TS_PATH)"; \
	fi
	@if [ -x "$(STROPPY_PB_JS_PATH)" ]; then \
		echo "$(STROPPY_PB_JS_PATH) is already installed."; \
	else \
		wget -q -O $(STROPPY_PB_JS_PATH) "$(STROPPY_PROTO_URL)/$(STROPPY_PB_JS_NAME)"; \
		echo "$(STROPPY_PB_JS_NAME) installed to $(STROPPY_PB_JS_PATH)"; \
	fi

STROPPY_BIN_NAME=stroppy
STROPPY_OUT_FILE=$(CURDIR)/build/$(STROPPY_BIN_NAME)
.PHONY: build
build: .download-proto-static .download-k6 # Build binary stroppy
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -o $(STROPPY_OUT_FILE) $(CURDIR)/cmd/stroppy

