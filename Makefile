LOCAL_BIN:=$(CURDIR)/bin
PATH:=$(LOCAL_BIN):$(PATH)
GOPROXY:=proxy.golang.org,direct
BUILD_TARGET_DIR=$(CURDIR)/build

VERSION=$(shell git describe --tags --always 2>/dev/null || echo "0.0.0")

default: help

.PHONY: help
help: # Show help in Makefile
	@grep -E '^[a-zA-Z0-9 _-]+:.*#'  Makefile | sort | while read -r l; do printf "\033[1;32m$$(echo $$l | cut -f 1 -d':')\033[00m:$$(echo $$l | cut -f 2- -d'#')\n"; done

# List of required binaries for a local build/check.
REQUIRED_BINS = git go curl unzip docker
.PHONY: .check-bins
.check-bins: # Check for required binaries if build locally
	@echo "Checking for required binaries..."
	@missing=0; \
	for bin in $(REQUIRED_BINS); do \
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

.PHONY: install-linter
install-linter: # Install golangci-lint
	$(info Installing golangci-lint...)
	mkdir -p $(LOCAL_BIN)
	GOBIN=$(LOCAL_BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2

.PHONY: install-bin-deps
install-bin-deps: install-linter # Install binary dependencies in ./bin
	$(info Installing binary dependencies...)

.PHONY: app-deps
app-deps: # Install application dependencies
	GOPROXY=$(GOPROXY) go mod tidy

# NOTE: the .pb.go types under pkg/common/proto/stroppy and pkg/datagen/dgproto
# are frozen hand-edited Go types (the driver/workload contract). There is no
# .proto source and no codegen step; do not regenerate.

.PHONY: linter linter_fix tests

linter: # Start linter (read-only check)
	$(LOCAL_BIN)/golangci-lint cache clean
	$(LOCAL_BIN)/golangci-lint --config $(CURDIR)/.golangci.yml run

linter_fix: # Start linter with possible fixes (NEVER run casually — rewrites the whole repo)
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

gen-tpcds-streams: # Generate TPC-DS query streams (DIALECT, SCALE, SEED, STREAMS, OUT)
	go run ./third_party/gotpcds/dsqgen/cmd/dsqgen \
		-dialect $(or $(DIALECT),postgres) -scale $(or $(SCALE),1) \
		-seed $(or $(SEED),19620718) -streams $(or $(STREAMS),1) \
		-out $(or $(OUT),./tpcds-streams)

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


# Stroppy build section

.PHONY: build build-debug build-all

STROPPY_BIN_NAME=stroppy
STROPPY_OUT_FILE=$(CURDIR)/build/$(STROPPY_BIN_NAME)
STROPPY_LDFLAGS=-ldflags "-s -w -X 'github.com/stroppy-io/stroppy/internal/version.Version=$(VERSION)'"

build-debug: # Build binary stroppy (with symbols)
	@mkdir -p $(CURDIR)/build
	echo $(VERSION)
	go build -trimpath -ldflags "-X 'github.com/stroppy-io/stroppy/internal/version.Version=$(VERSION)'" -o $(STROPPY_OUT_FILE) ./cmd/stroppy

build: # Build binary stroppy
	@mkdir -p $(CURDIR)/build
	echo $(VERSION)
	CGO_ENABLED=0 go build -trimpath $(STROPPY_LDFLAGS) -o $(STROPPY_OUT_FILE) ./cmd/stroppy

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
## Smoke runs (Go-native workloads)
##

# Gate one smoke run: fail on a non-zero exit OR any `level=error` line.
# stroppy exits 0 even when every iteration errors (e.g. a failed GlobalOnce
# load), so the exit code alone is not a reliable signal; default error mode
# logs every error at level=error, which this catches.
# $(1)=label, rest=command.
define smoke_run
	echo "== $(1) =="; \
	out=$$($(2) 2>&1); code=$$?; printf '%s\n' "$$out"; \
	if [ $$code -ne 0 ] || printf '%s' "$$out" | grep -q 'level=error'; then \
		echo "FAIL ($(1)): exit=$$code"; rc=1; \
	fi
endef

# Scenario-branch smoke on the noop driver (NO database). Every workload has
# two executor archetypes — constant-vus (DURATION set, throughput) and
# shared-iterations (power test). The noop driver runs the full lifecycle
# without a DB, so this catches executor/options regressions for ~free. The
# VUS=2/ITER=1 case also guards the shared-iterations "iterations < VUs" floor.
# Third field skips each workload's data-validation step (noop has no data to
# check); "-" means nothing to skip.
.PHONY: run-scenario-smoke
run-scenario-smoke: # Tier 0: scenario-branch smoke on noop (no DB), all workloads x both branches
	@rc=0;                                                                          \
	for spec in "tpcb/tx 1 -" "tpcc/tx 1 validate_population" "tpcds 0.01 -" "tpch/tx 0.01 validate_answers"; do \
		set -- $$spec; wl=$$1; sf=$$2; skip=$$3; ns=""; [ "$$skip" = "-" ] || ns="--no-steps $$skip"; \
		$(call smoke_run,noop constant-vus: $$wl,./build/stroppy run $$wl -d noop -e SCALE_FACTOR=$$sf -e DURATION=2s -e VUS=2 $$ns); \
		$(call smoke_run,noop shared-iters: $$wl,./build/stroppy run $$wl -d noop -e SCALE_FACTOR=$$sf -e VUS=2 -e ITER=1 $$ns); \
	done;                                                                           \
	exit $$rc

# Real-Postgres smoke of BOTH scenario branches for the light workloads at tiny
# scale (default pg preset = localhost:5432). Complements run-scenario-smoke by
# exercising the actual DB path (load + run) in throughput and power modes.
# tpch's validate_answers golden set is SF=1 only, so it is skipped at smoke
# scale; validate_population stays on for tpcc since it passes at SF=1.
# tpcds is intentionally NOT here: its fixed-cardinality dimensions do not
# shrink with SF, so even SF=0.01 is a multi-million-row load+index — too heavy
# for a free CI runner's Postgres. Its scenario branches are covered DB-free by
# run-scenario-smoke (noop) instead.
.PHONY: run-workload-branches
run-workload-branches: # Tier 1: real-Postgres smoke of both branches (tpcb/tpcc/tpch)
	@rc=0;                                                                          \
	for spec in "tpcb/tx 1 -" "tpcc/tx 1 -" "tpch/tx 0.01 validate_answers"; do \
		set -- $$spec; wl=$$1; sf=$$2; skip=$$3; ns=""; [ "$$skip" = "-" ] || ns="--no-steps $$skip"; \
		$(call smoke_run,pg constant-vus: $$wl,./build/stroppy run $$wl -e SCALE_FACTOR=$$sf -e DURATION=2s -e VUS=2 $$ns); \
		$(call smoke_run,pg shared-iters: $$wl,./build/stroppy run $$wl -e SCALE_FACTOR=$$sf -e VUS=2 -e ITER=1 $$ns); \
	done;                                                                           \
	exit $$rc

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

##
## Multi-DB tmpfs integration harness (postgres + mysql + picodata + ydb)
##

.PHONY: tmpfs-all-up tmpfs-all-down tmpfs-all-clean

tmpfs-all-up: # Start all 4 DBs (pg, mysql, picodata, ydb) on non-default ports
	docker compose -f test/compose.tmpfs-all.yml up -d --wait pg-tmpfs-all mysql-tmpfs-all picodata-tmpfs-all ydb-tmpfs-all
	docker compose -f test/compose.tmpfs-all.yml up picodata-init

tmpfs-all-down: # Stop + remove all 4 DBs and their volumes
	docker compose -f test/compose.tmpfs-all.yml down -v

tmpfs-all-clean: # Recycle the 4-DB harness; discards all data
	$(MAKE) tmpfs-all-down && $(MAKE) tmpfs-all-up
