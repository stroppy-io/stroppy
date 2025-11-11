VERSION=$(shell git describe --tags --always 2>/dev/null || echo "0.0.0")
LOCAL_BIN:=$(CURDIR)/bin
NODE_BIN:=$(CURDIR)/bin/node_bin/bin
PATH:=$(LOCAL_BIN):$(NODE_BIN):$(PATH)
GOPROXY:=https://goproxy.io,direct
BUILD_TARGET_DIR=$(CURDIR)/build
PROTO_BUILD_TARGET_DIR=$(CURDIR)/proto/build

OS := linux
UNAME_M := $(shell uname -m)
ifeq ($(UNAME_M),x86_64)
  ARCH := x86_64
else ifeq ($(UNAME_M),aarch64)
  ARCH := aarch_64
else
  $(error Unsupported architecture: $(UNAME_M))
endif

default: help

.PHONY: help
help: # Show help in Makefile
	@grep -E '^[a-zA-Z0-9 _-]+:.*#'  Makefile | sort | while read -r l; do printf "\033[1;32m$$(echo $$l | cut -f 1 -d':')\033[00m:$$(echo $$l | cut -f 2- -d'#')\n"; done

# List of required binaries (default checks PATH)
# Optional: Specify custom paths for binaries not in PATH
# Format: binary_name=/path/to/binary
REQUIRED_BINS = git node npm go curl unzip \
	protoc=$(LOCAL_BIN)/protoc \
	easyp=$(LOCAL_BIN)/easyp \
	protoc-gen-ts=$(NODE_BIN)/protoc-gen-ts \
	protoc-gen-go=$(LOCAL_BIN)/protoc-gen-go \
	protoc-gen-go-grpc=$(LOCAL_BIN)/protoc-gen-go-grpc \
	protoc-gen-validate=$(LOCAL_BIN)/protoc-gen-validate \
	protoc-gen-jsonschema=$(LOCAL_BIN)/protoc-gen-jsonschema \
	protoc-gen-doc=$(LOCAL_BIN)/protoc-gen-doc
	xk6=$(LOCAL_BIN)/xk6
.PHONY: .check-bins
.check-bins: # Check for required binaries if build locally
	@echo "Checking for required binaries..."
	@missing=0; \
	for bin_spec in $(REQUIRED_BINS); do \
		bin=$${bin_spec%%=*}; \
		custom_path=$${bin_spec#*=}; \
		if [ "$$bin" != "$$custom_path" ]; then \
			# Check custom path first \
			if [ -x "$$custom_path" ]; then \
				echo "✓ $$bin is installed at $$custom_path"; \
				continue; \
			fi; \
		fi; \
		# Fall back to PATH check \
		if which $$bin > /dev/null; then \
			echo "✓ $$bin is installed in PATH"; \
		else \
			echo "✗ $$bin is NOT found"; \
			missing=1; \
		fi; \
	done; \
	if [ $$missing -eq 1 ]; then \
		echo "Error: Some required binaries are missing"; \
		exit 1; \
	else \
		echo "All required binaries are available"; \
	fi

PROTOC_VERSION ?= 32.1
PROTOC_BIN := $(LOCAL_BIN)/protoc
PROTOC_URL := https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/protoc-$(PROTOC_VERSION)-$(OS)-$(ARCH).zip
PROTOC_ZIP := /tmp/protoc-$(PROTOC_VERSION)-$(OS)-$(ARCH).zip
PROTOC_TMP := /tmp/protoc-$(PROTOC_VERSION)-$(OS)-$(ARCH)
.PHONY: .install-protoc
.install-protoc:
	@echo ">>> Installing protoc v$(PROTOC_VERSION) to $(PROTOC_BIN)"
	@mkdir -p $(LOCAL_BIN)
	@rm -rf $(PROTOC_TMP) && rm -rf $(PROTOC_ZIP) && rm -rf $(LOCAL_BIN)/include && rm -rf $(LOCAL_BIN)/protoc
	@echo ">>> Downloading $(PROTOC_URL)"м
	@curl -SL -o $(PROTOC_ZIP) $(PROTOC_URL)
	@unzip -o -q $(PROTOC_ZIP) -d $(PROTOC_TMP)
	@mkdir -p $(LOCAL_BIN)/include
	@cp $(PROTOC_TMP)/bin/protoc $(PROTOC_BIN)
	@cp -r $(PROTOC_TMP)/include/* $(LOCAL_BIN)/include/
	@chmod +x $(PROTOC_BIN)
	@rm $(PROTOC_ZIP) && rm -rf $(PROTOC_TMP)

.PHONY: .install-easyp
.install-easyp:
	mkdir -p $(LOCAL_BIN)
	GOBIN=$(LOCAL_BIN) GOPROXY=$(GOPROXY) go install github.com/easyp-tech/easyp/cmd/easyp@v0.7.15

.PHONY: .install-go-proto-deps
.install-go-proto-deps:
	mkdir -p $(LOCAL_BIN)
	GOBIN=$(LOCAL_BIN) GOPROXY=$(GOPROXY) go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.9
	GOBIN=$(LOCAL_BIN) GOPROXY=$(GOPROXY) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
	GOBIN=$(LOCAL_BIN) GOPROXY=$(GOPROXY) go install github.com/envoyproxy/protoc-gen-validate@v1.2.1
	GOBIN=$(LOCAL_BIN) GOPROXY=$(GOPROXY) go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.19.1
	GOBIN=$(LOCAL_BIN) GOPROXY=$(GOPROXY) go install github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@v1.5.1
	GOBIN=$(LOCAL_BIN) GOPROXY=$(GOPROXY) go install github.com/pubg/protoc-gen-jsonschema@v0.8.0

.PHONY: .install-node-proto-deps
.install-node-proto-deps:
	mkdir -p $(LOCAL_BIN)
	npm install --global --prefix $(LOCAL_BIN)/node_bin @protobuf-ts/plugin@2.11.1

TS_TARGET_DIR=$(PROTO_BUILD_TARGET_DIR)/ts
TS_BUNDLE_DIR=$(CURDIR)/proto/ts_bundle
TMP_BUNDLE_DIR=$(TS_BUNDLE_DIR)/tmp
.PHONY: .build-proto-ts-sdk
.build-proto-ts-sdk: # Build ts sdk with single js file for proto files
	rm -rf $(TMP_BUNDLE_DIR)
	mkdir -p $(TS_TARGET_DIR)
	mkdir -p $(TMP_BUNDLE_DIR)
	mkdir -p $(TMP_BUNDLE_DIR)/ts_sdk
	cp -r $(TS_TARGET_DIR)/google/protobuf/* $(TMP_BUNDLE_DIR)/ts_sdk/
	cp -r $(TS_TARGET_DIR)/proto/stroppy/* $(TMP_BUNDLE_DIR)/ts_sdk/
	cp $(TS_BUNDLE_DIR)/combine.js $(TMP_BUNDLE_DIR)/
	cp $(TS_BUNDLE_DIR)/package.json $(TMP_BUNDLE_DIR)/
	cp $(TS_BUNDLE_DIR)/tsconfig.json $(TMP_BUNDLE_DIR)/
	cp $(TS_BUNDLE_DIR)/webpack.config.js $(TMP_BUNDLE_DIR)/
	cd $(TMP_BUNDLE_DIR) && npm install && node combine.js
	cd $(TMP_BUNDLE_DIR) && npm run build
	cp $(TMP_BUNDLE_DIR)/stroppy.pb.ts $(TS_TARGET_DIR)/stroppy.pb.ts
	cp $(TMP_BUNDLE_DIR)/dist/bundle.js $(TS_TARGET_DIR)/stroppy.pb.js
	rm -rf $(TMP_BUNDLE_DIR)

.PHONY: .easyp-gen
.easyp-gen:
	$(LOCAL_BIN)/easyp generate

.PHONY: .install-linter
.install-linter: # Install golangci-lint
	$(info Installing golangci-lint...)
	mkdir -p $(LOCAL_BIN)
	GOPROXY=$(GOPROXY) GOBIN=$(LOCAL_BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.0

.PHONY: .install-xk6
.install-xk6:
	$(info Installing xk6...)
	mkdir -p $(LOCAL_BIN)
	GOBIN=$(LOCAL_BIN) go install go.k6.io/xk6@v1.1.5

.PHONY: .install-proto-deps
.install-proto-deps: .install-protoc .install-easyp .install-go-proto-deps .install-node-proto-deps

.PHONY: .bin-deps
.bin-deps: .install-linter .install-xk6 .install-proto-deps # Install binary dependencies in ./bin
	$(info Installing binary dependencies...)

.PHONY: .app-deps
.app-deps: # Install application dependencies in ./bin
	GOPROXY=$(GOPROXY) 											go mod tidy
	GOPROXY=$(GOPROXY) cd cmd/xk6/       && go mod tidy
	GOPROXY=$(GOPROXY) cd cmd/xk6air/    && go mod tidy
	GOPROXY=$(GOPROXY) cd cmd/sobek/     && go mod tidy
	GOPROXY=$(GOPROXY) cd cmd/config2go/ && go mod tidy
	GOPROXY=$(GOPROXY) cd cmd/inserter/  && go mod tidy

PROTO_BUILD_TARGET_DIR=$(CURDIR)/proto/build
.PHONY: proto
proto: .check-bins
	rm -rf $(CURDIR)/pkg/common/proto/*
	mkdir -p $(PROTO_BUILD_TARGET_DIR)/ts/stroppy
	mkdir -p $(PROTO_BUILD_TARGET_DIR)/docs
	mkdir -p $(PROTO_BUILD_TARGET_DIR)/go
	$(MAKE) .easyp-gen && $(MAKE) .build-proto-ts-sdk
# NOTE: easyp generates the code into the right place 'proto/stroppy' by itself
	printf '// Code generated by stoppy. DO NOT EDIT.\npackage stroppy\n\nconst Version = "%s"\n' "$(VERSION)" > ./pkg/common/proto/stroppy/version.stroppy.pb.go

	cp $(PROTO_BUILD_TARGET_DIR)/ts/stroppy.pb.ts $(CURDIR)/internal/static/
	cp $(PROTO_BUILD_TARGET_DIR)/ts/stroppy.pb.js $(CURDIR)/internal/static/
	cp $(PROTO_BUILD_TARGET_DIR)/docs/proto.md $(CURDIR)/docs
	# cp $(PROTO_BUILD_TARGET_DIR)/docs/config.schema.json $(CURDIR)/docs

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

K6_OUT_FILE=$(CURDIR)/build/stroppy-k6
.PHONY: build-k6
build-k6: # Build k6 module
	mkdir -p $(CURDIR)/build
	GOPROXY=$(GOPROXY) \
	PATH=$(LOCAL_BIN)/xk6:$(PATH) xk6 build --verbose \
		--with github.com/stroppy-io/stroppy/cmd/xk6air=./cmd/xk6air/ \
		--replace github.com/stroppy-io/stroppy=./ \
		--with github.com/oleiade/xk6-encoding@latest \
		--output $(K6_OUT_FILE)
	cp $(CURDIR)/build/stroppy-k6 internal/static/stroppy-k6

STROPPY_BIN_NAME=stroppy
STROPPY_OUT_FILE=$(CURDIR)/build/$(STROPPY_BIN_NAME)
.PHONY: build
build: # Build binary stroppy
	echo $(VERSION)
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
		go build -race -v -o $(STROPPY_OUT_FILE) \
		-ldflags "-X 'github.com/stroppy-io/stroppy/internal/version.Version=$(VERSION)'" \
		$(CURDIR)/cmd/stroppy

branch=main
.PHONY: revision
revision: # Recreate git tag with version tag=<semver>
	@if [ -e $(tag) ]; then \
		echo "error: Specify version 'tag='"; \
		exit 1; \
	fi
	git tag -d v${tag} || true
	git push --delete origin v${tag} || true
	git tag v$(tag)
	git push origin v$(tag)
