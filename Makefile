LOCAL_BIN:=$(CURDIR)/bin
NODE_BIN:=$(CURDIR)/bin/node_bin/bin
PATH:=$(LOCAL_BIN):$(NODE_BIN):$(PATH)
GOPROXY:=proxy.golang.org,direct
BUILD_TARGET_DIR=$(CURDIR)/build
PROTO_BUILD_TARGET_DIR=$(CURDIR)/proto/build

VERSION=$(shell git describe --tags --always 2>/dev/null || echo "0.0.0")

UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

# Detect system info
ifeq ($(UNAME_S),Darwin)
  OS := osx
else ifeq ($(UNAME_S),Linux)
  OS := linux
else
  $(error Unsupported OS: $(UNAME_S))
endif

ifeq ($(UNAME_M),x86_64)
  ARCH := x86_64
else ifeq ($(UNAME_M),arm64)
  ARCH := aarch_64
else
  $(error Unsupported architecture: $(UNAME_M))
endif

# Set GOOS only if not already set
ifndef GOOS
  ifeq ($(OS),osx)
    GOOS := darwin
  else ifeq ($(OS),linux)
    GOOS := linux
  else
    $(error Unsupported OS for Go: $(OS))
  endif
endif

# Set GOARCH only if not already set
ifndef GOARCH
  ifeq ($(ARCH),aarch_64)
    GOARCH := arm64
  else ifeq ($(ARCH),x86_64)
    GOARCH := amd64
  else
    $(error Unsupported architecture for Go: $(ARCH))
  endif
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
	protoc-gen-doc=$(LOCAL_BIN)/protoc-gen-doc \
	protoc-gen-jsonschema=$(LOCAL_BIN)/protoc-gen-jsonschema \
	xk6=$(LOCAL_BIN)/xk6
.PHONY: .check-bins
.check-bins: # Check for required binaries if build locally
	@echo "Checking for required binaries..."
	@missing=0; \
	for bin_spec in $(REQUIRED_BINS); do \
		case "$$bin_spec" in \
			*=*) \
				bin=$${bin_spec%%=*}; \
				custom_path=$${bin_spec#*=}; \
				if [ -x "$$custom_path" ]; then \
					echo "✓ $$bin is installed at $$custom_path"; \
					continue; \
				else \
					echo "✗ $$bin expected at $$custom_path but not found"; \
					missing=1; \
					continue; \
				fi \
				;; \
			*) \
				bin=$$bin_spec; \
				;; \
		esac; \
		if command -v "$$bin" >/dev/null 2>&1; then \
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

VALIDATE_PROTO_PATH := $(HOME)/.easyp/mod/github.com/bufbuild/protoc-gen-validate/v1.2.1

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
	@echo ">>> Downloading $(PROTOC_URL)"
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
# Copy the entire directory structure to preserve relative imports
	cp -r $(TS_TARGET_DIR) $(TMP_BUNDLE_DIR)/ts_source
# Copy analyze_ddl source before building
	cp $(CURDIR)/internal/static/parse_sql.ts $(TMP_BUNDLE_DIR)/parse_sql.ts
	cp $(TS_BUNDLE_DIR)/build.js $(TMP_BUNDLE_DIR)/
	cp $(TS_BUNDLE_DIR)/package.json $(TMP_BUNDLE_DIR)/
	cd $(TMP_BUNDLE_DIR) && npm install
	cd $(TMP_BUNDLE_DIR) && node build.js
	cp $(TMP_BUNDLE_DIR)/stroppy.pb.ts $(TS_TARGET_DIR)/stroppy.pb.ts
	cp $(TMP_BUNDLE_DIR)/dist/bundle.js $(TS_TARGET_DIR)/stroppy.pb.js
# Bundle parse_sql with node-sql-parser (handled by build.js)
# TODO: make single bundle aka stroppy.js or automatically copy all from dist
	cp $(TMP_BUNDLE_DIR)/dist/parse_sql.js $(TS_TARGET_DIR)/parse_sql.js
	rm -rf $(TMP_BUNDLE_DIR)

.PHONY: .easyp-gen
.easyp-gen:
	$(LOCAL_BIN)/easyp generate

.PHONY: install-linter
install-linter: # Install golangci-lint
	$(info Installing golangci-lint...)
	mkdir -p $(LOCAL_BIN)
	GOBIN=$(LOCAL_BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10.1

.PHONY: install-xk6
install-xk6:
	$(info Installing xk6...)
	mkdir -p $(LOCAL_BIN)
	GOBIN=$(LOCAL_BIN) go install go.k6.io/xk6@v1.1.5

.PHONY: .install-proto-deps
.install-proto-deps: .install-protoc .install-easyp .install-go-proto-deps .install-node-proto-deps

.PHONY: install-bin-deps
install-bin-deps: install-linter install-xk6 .install-proto-deps # Install binary dependencies in ./bin
	$(info Installing binary dependencies...)

.PHONY: app-deps
app-deps: # Install application dependencies in ./bin
	GOPROXY=$(GOPROXY)						go mod tidy
	GOPROXY=$(GOPROXY) cd cmd/xk6air/    && go mod tidy

.PHONY: proto
proto: .check-bins
	rm -rf $(CURDIR)/pkg/common/proto/*
	rm -rf $(PROTO_BUILD_TARGET_DIR)/ts
	mkdir -p $(PROTO_BUILD_TARGET_DIR)/ts/stroppy
	mkdir -p $(PROTO_BUILD_TARGET_DIR)/docs
	mkdir -p $(PROTO_BUILD_TARGET_DIR)/go
	$(MAKE) .easyp-gen && $(MAKE) .build-proto-ts-sdk
# NOTE: easyp generates the code into the right place 'proto/stroppy' by itself
	printf '// Code generated by stroppy. DO NOT EDIT.\npackage stroppy\n\nconst Version = "%s"\n' "$(VERSION)" > ./pkg/common/proto/stroppy/version.stroppy.pb.go

	cp $(PROTO_BUILD_TARGET_DIR)/ts/stroppy.pb.ts $(CURDIR)/internal/static/
	cp $(PROTO_BUILD_TARGET_DIR)/ts/stroppy.pb.js $(CURDIR)/internal/static/
	cp $(PROTO_BUILD_TARGET_DIR)/ts/parse_sql.js $(CURDIR)/internal/static/
	cp $(PROTO_BUILD_TARGET_DIR)/docs/proto.md $(CURDIR)/docs
	$(MAKE) jsonschema

.PHONY: jsonschema
jsonschema: # Generate JSON Schema for RunConfig (IDE autocomplete for stroppy-config.json)
	mkdir -p $(PROTO_BUILD_TARGET_DIR)/jsonschema
	$(PROTOC_BIN) \
		-I . \
		-I $(LOCAL_BIN)/include \
		-I $(VALIDATE_PROTO_PATH) \
		--plugin=protoc-gen-jsonschema=$(LOCAL_BIN)/protoc-gen-jsonschema \
		--jsonschema_out=$(PROTO_BUILD_TARGET_DIR)/jsonschema \
		--jsonschema_opt=pretty_json_output=true,entrypoint_message=RunConfig \
		proto/stroppy/run.proto
	mkdir -p $(CURDIR)/docs/jsonschema
	cp $(PROTO_BUILD_TARGET_DIR)/jsonschema/proto/stroppy/run.schema.json $(CURDIR)/docs/jsonschema/run.schema.json

.PHONY: linter linter_fix tests

linter: # Start linter
	$(LOCAL_BIN)/golangci-lint cache clean
	$(LOCAL_BIN)/golangci-lint --config $(CURDIR)/.golangci.yml run

linter_fix: # Start linter with possible fixes
	$(LOCAL_BIN)/golangci-lint cache clean
	$(LOCAL_BIN)/golangci-lint --config $(CURDIR)/.golangci.yml run --fix

tests: # Run tests with coverage
	go test -race ./... -coverprofile=coverage.out


##
## Reference-data JSON regeneration (build-time, run with upstream inputs)
##

.PHONY: gen-tpcds-json gen-tpch-json

gen-tpcds-json: # Regenerate workloads/tpcds/distributions.json from upstream .dst files
	@if [ -z "$(TPCDS_TOOLS_DIR)" ]; then \
		echo "error: TPCDS_TOOLS_DIR must point to the dsdgen tools directory holding .dst files (e.g. /path/to/DSGen/tools)"; \
		exit 2; \
	fi
	go run ./cmd/dstparse -in $(TPCDS_TOOLS_DIR) -out workloads/tpcds/distributions.json

gen-tpch-json: # Regenerate workloads/tpch/distributions.json and answers_sf1.json from upstream files
	@if [ -z "$(TPCH_DISTS)" ]; then \
		echo "error: TPCH_DISTS must point to upstream dists.dss"; \
		exit 2; \
	fi
	@if [ -z "$(TPCH_ANSWERS_DIR)" ]; then \
		echo "error: TPCH_ANSWERS_DIR must point to the upstream answers/ directory (q*.out / *.ans)"; \
		exit 2; \
	fi
	go run ./cmd/tpch-dists -in $(TPCH_DISTS) -out workloads/tpch/distributions.json
	go run ./cmd/tpch-answers -in $(TPCH_ANSWERS_DIR) -out workloads/tpch/answers_sf1.json


# K6/Stroppy build section

.PHONY: build-k6 build-k6-debug build-debug build build-all

STROPPY_BIN_NAME=stroppy
STROPPY_OUT_FILE=$(CURDIR)/build/$(STROPPY_BIN_NAME)
K6_OUT_FILE=$(CURDIR)/build/k6
K6_COMMON_FLAGS := --verbose \
		--k6-version v1.7.0 \
		--with github.com/stroppy-io/stroppy/cmd/xk6air=./cmd/xk6air/ \
		--replace github.com/stroppy-io/stroppy=./ \
		--with github.com/oleiade/xk6-encoding@v0.0.0-20251120082946-fbe7a8cbb88e \
		--with github.com/grafana/xk6-dashboard@v0.8.1 \
		--output $(K6_OUT_FILE)

build-k6: # Build k6 module
	@mkdir -p $(CURDIR)/build

	CGO_ENABLED=0 XK6_RACE_DETECTOR=0 \
	xk6 build $(K6_COMMON_FLAGS) \
		--build-flags -trimpath \
		--build-flags "-ldflags=-s -w -X 'github.com/stroppy-io/stroppy/internal/version.Version=$(VERSION)'"

build-k6-debug: # Build k6 module
	@mkdir -p $(CURDIR)/build

	xk6 build $(K6_COMMON_FLAGS) \
		--build-flags "-ldflags=-X 'github.com/stroppy-io/stroppy/internal/version.Version=$(VERSION)'"

build-debug: build-k6-debug # Build binary stroppy
	echo $(VERSION)
	cp $(K6_OUT_FILE) $(STROPPY_OUT_FILE)

build: build-k6 # Build binary stroppy
	echo $(VERSION)
	cp $(K6_OUT_FILE) $(STROPPY_OUT_FILE)

build-all: build

branch=main
.PHONY: revision
revision: # Recreate git tag with version tag=<semver>
	@if [ -z "$(tag)" ]; then \
		echo "error: Specify version 'tag='"; \
		exit 1; \
	fi
	git tag -d v${tag} || true
	git push --delete origin v${tag} || true
	git tag v$(tag)
	git push origin v$(tag)


##
## Local K6 fast tests
##

.PHONY: run-simple-test run-tpcb-test run-tpcc-test run-tpcc-mysql-test run-tpcds-test run-k6-tests

WORKDIR=dev

run-simple-test:
	rm -rf $(WORKDIR)
	./build/stroppy gen --workdir $(WORKDIR) --preset=simple
	cd $(WORKDIR) && ./stroppy run simple.ts

run-tpcb-test:
	LOG_LEVEL=DEBUG STROPPY_ERROR_MODE=throw \
		./build/stroppy run tpcb/procs.ts

run-tpcc-test:
	LOG_LEVEL=DEBUG STROPPY_ERROR_MODE=throw \
		./build/stroppy run tpcc/procs.ts

run-tpcc-mysql-test:
	LOG_LEVEL=DEBUG STROPPY_ERROR_MODE=throw \
		./build/stroppy run tpcc/procs.ts -d mysql -- -q

run-tpcds-test:
	./build/stroppy run tpcds tpcds-scale-1.sql

run-k6-tests: # Run SQL API integration tests
# rc - return code
# This allows to run all the test and to exit with the nonzero code if any failed
	@rc=0;                                                      \
	./build/stroppy run tests/sqlapi_test -- -q        || rc=1; \
	./build/stroppy run tests/multi_drivers_test -- -q || rc=1; \
	./build/stroppy run tests/transaction_test -- -q   || rc=1; \
	exit $$rc

##
## TypeScript Development
##

.PHONY: ts-setup ts-test ts-watch ts-typecheck

ts-setup: # Setup TypeScript testing environment
	@echo "Setting up TypeScript testing environment..."
	cd internal/static && npm install
	@echo "✓ TypeScript testing environment ready!"
	@echo "Run 'make ts-test' to run tests or 'make ts-watch' for watch mode"

ts-typecheck: # Typecheck TypeScript framework code (helpers.ts, parse_sql.ts, stroppy.d.ts)
	cd internal/static && npx tsc --noEmit

ts-test: # Run TypeScript unit tests
	cd internal/static && npm test

ts-watch: # Watch TypeScript files and run tests automatically
	cd internal/static && npm run test:watch

##
## Tmpfs Postgres integration harness
##

.PHONY: tmpfs-up tmpfs-down tmpfs-clean tmpfs-psql

tmpfs-up: # Start tmpfs Postgres container for integration tests
	docker compose -f test/compose.tmpfs.yml up -d --wait

tmpfs-down: # Stop and remove tmpfs Postgres container and volumes
	docker compose -f test/compose.tmpfs.yml down -v

tmpfs-clean: # Recycle the tmpfs Postgres container; discards all data
	$(MAKE) tmpfs-down && $(MAKE) tmpfs-up

tmpfs-psql: # Open psql shell into the tmpfs Postgres container
	docker exec -it stroppy-pg-tmpfs psql -U postgres -d stroppy
