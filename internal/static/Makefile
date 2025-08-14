

default: help

.PHONY: help
help: # Show help in Makefile
	@grep -E '^[a-zA-Z0-9 _-]+:.*#'  Makefile | sort | while read -r l; do printf "\033[1;32m$$(echo $$l | cut -f 1 -d':')\033[00m:$$(echo $$l | cut -f 2- -d'#')\n"; done


# List of required binaries (default checks PATH)
# Optional: Specify custom paths for binaries not in PATH
# Format: binary_name=/path/to/binary
REQUIRED_BINS := wget node npm
.PHONY: .check-dev-bins
.check-dev-bins: # Check for required binaries if build locally
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

DRIVER_BIN_NAME=stroppy-postgres
GO_OS= $(shell uname -s | tr '[:upper:]' '[:lower:]')
GO_ARCH= $(shell uname -m)
ifeq ($(GO_ARCH),x86_64)
    GO_ARCH := amd64
endif
ifeq ($(GO_ARCH),aarch64)
    GO_ARCH := arm64
endif
DRIVER_PKG_NAME   := $(DRIVER_BIN_NAME)_$(GO_OS)_$(GO_ARCH)
DRIVER_ARCHIVE    := $(DRIVER_PKG_NAME).tar.gz
DRIVER_URL        := https://github.com/stroppy-io/stroppy-postgres/releases/latest/download/$(DRIVER_ARCHIVE)
.PHONY: install-driver
install-driver: # Install driver plugin
	@# Here example how to install driver plugin from stoppy github.
	@# You can install driver plugin from other source using your own logic.
	$(info Installing postgres driver plugin...)
	@if [ -x "$(CURDIR)/$(DRIVER_BIN_NAME)" ]; then \
		echo "$(CURDIR)/$(DRIVER_BIN_NAME) is already installed."; \
	else \
		echo "Downloading $(DRIVER_BIN_NAME)..."; \
    	wget -q -O $(CURDIR)/$(DRIVER_ARCHIVE) "$(DRIVER_URL)"; \
    	tar -xzf $(CURDIR)/$(DRIVER_ARCHIVE) -C $(CURDIR); \
		chmod +x $(CURDIR)/$(DRIVER_BIN_NAME); \
		rm $(CURDIR)/$(DRIVER_ARCHIVE); \
	fi

.PHONY: .install-node-modules
.install-node-modules: # Install node modules
	$(info Installing node modules...)
	@if [ -d "node_modules" ] && [ -f "node_modules/.package-lock.json" ]; then \
		echo "Node modules are already installed."; \
	else \
		echo "Installing node modules..."; \
		npm install; \
	fi

STROPPY_CONFIG_NAME:=stroppy
STROPPY_CONFIG := $(or $(wildcard $(STROPPY_CONFIG_NAME).json), $(wildcard $(STROPPY_CONFIG_NAME).yaml))
.PHONY: validate-config
validate-config: # Validate config
	$(info Validating config...)
	@./stroppy config validate --config=$(STROPPY_CONFIG)

.PHONY: configure
configure: .check-dev-bins install-driver .install-node-modules validate-config # Configure development environment for stroppy

.PHONY: re-configure
re-configure: # Re-configure development environment
	$(info Re-configuring development environment...)
	@rm -rf node_modules
	@rm -rf stroppy-postgres
	@make configure