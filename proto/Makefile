LOCAL_BIN:=$(CURDIR)/bin
BUILD_TARGET_DIR=$(CURDIR)/build
PATH:=$(PATH):$(LOCAL_BIN)
GOPROXY:=https://goproxy.io,direct
VERSION=$(shell git describe --tags --always 2>/dev/null || echo "0.0.0")

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

# List of required binaries (default checks PATH)
# Optional: Specify custom paths for binaries not in PATH
# Format: binary_name=/path/to/binary
REQUIRED_BINS = git node npm go \
				protoc=$(LOCAL_BIN)/protoc \
				protoc=$(LOCAL_BIN)/easyp \
				protoc-gen-ts=$(LOCAL_BIN)/node_bin/bin/protoc-gen-ts \
				protoc-gen-go=$(LOCAL_BIN)/protoc-gen-go \
				protoc-gen-go-grpc=$(LOCAL_BIN)/protoc-gen-go-grpc \
				protoc-gen-validate=$(LOCAL_BIN)/protoc-gen-validate \
				protoc-gen-jsonschema=$(LOCAL_BIN)/protoc-gen-jsonschema \
				protoc-gen-doc=$(LOCAL_BIN)/protoc-gen-doc
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

.PHONY: .go-deps
.go-deps:
	mkdir -p $(LOCAL_BIN)
	GOBIN=$(LOCAL_BIN) GOPROXY=$(GOPROXY) go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.9
	GOBIN=$(LOCAL_BIN) GOPROXY=$(GOPROXY) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
	GOBIN=$(LOCAL_BIN) GOPROXY=$(GOPROXY) go install github.com/envoyproxy/protoc-gen-validate@v1.2.1
	GOBIN=$(LOCAL_BIN) GOPROXY=$(GOPROXY) go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.19.1

.PHONY: .node-deps
.node-deps:
	mkdir -p $(LOCAL_BIN)
	npm install --global --prefix $(LOCAL_BIN)/node_bin @protobuf-ts/plugin@2.11.1

.PHONY: .doc-deps
.doc-deps:
	GOBIN=$(LOCAL_BIN) GOPROXY=$(GOPROXY) go install github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@v1.5.1
	GOBIN=$(LOCAL_BIN) GOPROXY=$(GOPROXY) go install github.com/pubg/protoc-gen-jsonschema@v0.8.0

.PHONY: bin-deps
bin-deps: .install-protoc .install-easyp .go-deps .node-deps .doc-deps # Install binary dependencies in ./bin

.PHONY: proto-new
proto-new:
	PATH=$(LOCAL_BIN):$(PATH) $(LOCAL_BIN)/easyp generate

PROTOC_BIN=$(LOCAL_BIN)/protoc

TS_TARGET_DIR=$(BUILD_TARGET_DIR)/ts
TS_PLUGIN_DIR=$(LOCAL_BIN)/node_bin/bin/protoc-gen-ts
TS_BUNDLE_DIR=$(CURDIR)/ts_bundle
TMP_BUNDLE_DIR=$(TS_BUNDLE_DIR)/tmp
.PHONY: .build-ts-sdk
.build-ts-sdk: # Build ts sdk with single js file for proto files
	rm -rf $(TMP_BUNDLE_DIR)
	mkdir -p $(TS_TARGET_DIR)
	mkdir -p $(TMP_BUNDLE_DIR)
	mkdir -p $(TMP_BUNDLE_DIR)/ts_sdk
	cp -r $(TS_TARGET_DIR)/stroppy/* $(TMP_BUNDLE_DIR)/ts_sdk/
	cp -r $(TS_TARGET_DIR)/google/protobuf/* $(TMP_BUNDLE_DIR)/ts_sdk/
	cp $(TS_BUNDLE_DIR)/combine.js $(TMP_BUNDLE_DIR)/
	cp $(TS_BUNDLE_DIR)/package.json $(TMP_BUNDLE_DIR)/
	cp $(TS_BUNDLE_DIR)/tsconfig.json $(TMP_BUNDLE_DIR)/
	cp $(TS_BUNDLE_DIR)/webpack.config.js $(TMP_BUNDLE_DIR)/
	cd $(TMP_BUNDLE_DIR) && npm install && node combine.js
	cd $(TMP_BUNDLE_DIR) && npm run build
	cp $(TMP_BUNDLE_DIR)/stroppy.pb.ts $(TS_TARGET_DIR)/stroppy.pb.ts
	cp $(TMP_BUNDLE_DIR)/dist/bundle.js $(TS_TARGET_DIR)/stroppy.pb.js
	rm -rf $(TMP_BUNDLE_DIR)


PROTO_PATH=.
PROTO_PATH_FULL=$(CURDIR)/stroppy
PROTO_FILES=$(shell find "./stroppy" -type f -name '*.proto')
DOCS_TARGET_DIR=$(BUILD_TARGET_DIR)/docs
GO_TARGET_DIR=$(BUILD_TARGET_DIR)/go
.PHONY: .build-md-docs
.build-md-docs: # Build proto docs
	mkdir -p $(DOCS_TARGET_DIR)
	$(PROTOC_BIN) --plugin=$(LOCAL_BIN)/protoc-gen-doc \
	--doc_out=$(DOCS_TARGET_DIR) \
	--doc_opt=markdown,proto.md \
	--proto_path=$(PROTO_PATH) $(PROTO_FILES) ./easyp_vendor/validate/validate.proto

.PHONY: build
build: .check-bins
	mkdir -p $(DOCS_TARGET_DIR)
	PATH=$(LOCAL_BIN):$(LOCAL_BIN)/node_bin/bin:$(PATH) $(LOCAL_BIN)/easyp generate
	printf '// Code generated by stoppy. DO NOT EDIT.\npackage proto\n\nconst Version = "%s"\n' "$(VERSION)" > $(GO_TARGET_DIR)/stroppy/version.stroppy.pb.go
	$(MAKE) .build-ts-sdk

branch=main
.PHONY: revision
revision: # Recreate git tag with version tag=<semver>
	@if [ -e $(tag) ]; then \
		echo "error: Specify version 'tag='"; \
		exit 1; \
	fi
	git tag -d ${tag} || true
	git push --delete origin ${tag} || true
	git tag $(tag)
	git push origin $(tag)
