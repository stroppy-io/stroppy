VERSION=$(shell git describe --tags --always 2>/dev/null || echo "0.0.0")
LOCAL_BIN:=$(CURDIR)/bin
PATH:=$(PATH):$(LOCAL_BIN)
GOPROXY:=https://goproxy.io,direct

default: help

.PHONY: help
help: # Show help in Makefile
	@grep -E '^[a-zA-Z0-9 _-]+:.*#'  Makefile | sort | while read -r l; do printf "\033[1;32m$$(echo $$l | cut -f 1 -d':')\033[00m:$$(echo $$l | cut -f 2- -d'#')\n"; done

# List of required binaries (default checks PATH)
# Optional: Specify custom paths for binaries not in PATH
# Format: binary_name=/path/to/binary
REQUIRED_BINS = git go curl unzip \
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

.PHONY: .install-linter
.install-linter: # Install golangci-lint
	$(info Installing golangci-lint...)
	mkdir -p $(LOCAL_BIN)
	GOBIN=$(LOCAL_BIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8

.PHONY: .install-xk6
.install-xk6:
	$(info Installing xk6...)
	mkdir -p $(LOCAL_BIN)
	GOBIN=$(LOCAL_BIN) go install go.k6.io/xk6@v1.1.5

.PHONY: .bin-deps
.bin-deps: .install-linter .install-xk6 # Install binary dependencies in ./bin
	$(info Installing binary dependencies...)

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

SRC_PROTO_GO_PATH=$(CURDIR)/proto/build/go
TARGET_PROTO_GO_PATH=$(CURDIR)/pkg/common/proto
.PHONY: proto
proto:
	rm -rf $(TARGET_PROTO_GO_PATH)/*
	cd proto && $(MAKE) build
	mv $(SRC_PROTO_GO_PATH)/* $(TARGET_PROTO_GO_PATH)/

K6_OUT_FILE=$(CURDIR)/build/stroppy-k6
.PHONY: build-xk6
build-xk6: .check-bins # Build k6 module
	mkdir -p $(CURDIR)/build
	XK6_RACE_DETECTOR=0 PATH=$(LOCAL_BIN)/xk6:$(PATH) xk6 build --verbose \
		--with github.com/stroppy-io/stroppy/cmd/xk6=./cmd/xk6/ \
		--replace github.com/stroppy-io/stroppy=./ \
		--output $(K6_OUT_FILE)
	cp $(CURDIR)/build/stroppy-k6 internal/static/stroppy-xk6

STROPPY_BIN_NAME=stroppy
STROPPY_OUT_FILE=$(CURDIR)/build/$(STROPPY_BIN_NAME)
.PHONY: build
build: # Build binary stroppy
	echo $(VERSION)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -v -o $(STROPPY_OUT_FILE) \
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
