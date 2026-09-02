LOCAL_BIN:=$(CURDIR)/bin
PATH:=$(LOCAL_BIN):$(PATH)
GOPROXY:=proxy.golang.org,direct
BUILD_TARGET_DIR=$(CURDIR)/build

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "unknown")

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

# Application configuration is plain Go under pkg/config. Regenerate the
# committed JSON Schema with `go generate ./pkg/config`; protobuf is retained
# only through external SDK dependencies.

.PHONY: linter linter_fix tests

linter: # Start linter (read-only check)
	$(LOCAL_BIN)/golangci-lint cache clean
	$(LOCAL_BIN)/golangci-lint --config $(CURDIR)/.golangci.yml run

linter_fix: # Start linter with possible fixes (NEVER run casually — rewrites the whole repo)
	$(LOCAL_BIN)/golangci-lint cache clean
	$(LOCAL_BIN)/golangci-lint --config $(CURDIR)/.golangci.yml run --fix

tests: # Run tests with coverage; pass TEST_FLAGS=-short for the CI-sized suite
	go test -race $(TEST_FLAGS) ./... -coverprofile=coverage.out

.PHONY: integration integration-optional integration-sf1

integration: # Run mandatory tagged PostgreSQL, MySQL, CSV, and OTEL integration tests
	@test -x $(STROPPY_OUT_FILE) || { echo "error: $(STROPPY_OUT_FILE) is missing; run 'make build' first"; exit 1; }
	go test -tags=integration -count=1 -timeout=15m ./test/integration

integration-optional: # Run optional Picodata and YDB tagged integration tests
	@test -x $(STROPPY_OUT_FILE) || { echo "error: $(STROPPY_OUT_FILE) is missing; run 'make build' first"; exit 1; }
	go test -tags='integration integration_optional' -count=1 -timeout=30m -run 'TestTpchLoadOn(Picodata|YDB)' ./test/integration

integration-sf1: # Run the explicit heavy TPC-H SF=1 answer validation
	@test -x $(STROPPY_OUT_FILE) || { echo "error: $(STROPPY_OUT_FILE) is missing; run 'make build' first"; exit 1; }
	go test -tags='integration integration_sf1' -count=1 -timeout=75m -run TestTpchAnswersSpotCheck ./test/integration


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

.PHONY: build build-debug build-all pgnoop-fetch

STROPPY_BIN_NAME=stroppy
STROPPY_OUT_FILE=$(CURDIR)/build/$(STROPPY_BIN_NAME)
STROPPY_LDFLAGS=-ldflags "-s -w -X 'github.com/stroppy-io/stroppy/internal/version.Version=$(VERSION)'"

# Extra go build tags (release builds pass -tags=pgnoop_embed to carry the
# pg-noop baseline server inside the stroppy binary).
GO_BUILD_TAGS ?=

# Keep in sync with internal/pgnoop.Version.
PGNOOP_VERSION=v0.1.2
PGNOOP_EMBED_DIR=$(CURDIR)/internal/pgnoop/embedded

build-debug: # Build binary stroppy (with symbols)
	@mkdir -p $(CURDIR)/build
	echo $(VERSION)
	go build -trimpath -ldflags "-X 'github.com/stroppy-io/stroppy/internal/version.Version=$(VERSION)'" -o $(STROPPY_OUT_FILE) ./cmd/stroppy

build: # Build binary stroppy
	@mkdir -p $(CURDIR)/build
	echo $(VERSION)
	CGO_ENABLED=0 go build -trimpath $(GO_BUILD_TAGS) $(STROPPY_LDFLAGS) -o $(STROPPY_OUT_FILE) ./cmd/stroppy

build-all: build

pgnoop-fetch: # Fetch the host-matching pg-noop server for -tags pgnoop_embed builds
	@mkdir -p $(PGNOOP_EMBED_DIR)
	@os=$$(uname -s | tr '[:upper:]' '[:lower:]'); arch=$$(uname -m); \
	case "$$os/$$arch" in \
		linux/x86_64)  asset=pg-noop-x86_64-unknown-linux-musl.tar.xz ;; \
		linux/aarch64) asset=pg-noop-aarch64-unknown-linux-musl.tar.xz ;; \
		darwin/x86_64) asset=pg-noop-x86_64-apple-darwin.tar.xz ;; \
		darwin/arm64)  asset=pg-noop-aarch64-apple-darwin.tar.xz ;; \
		*) echo "error: no pg-noop release asset for $$os/$$arch"; exit 1 ;; \
	esac; \
	base="https://github.com/stroppy-io/pg-noop/releases/download/$(PGNOOP_VERSION)"; \
	tmp=$$(mktemp -d); \
	curl -sSfL -o "$$tmp/$$asset" "$$base/$$asset" || exit 1; \
	curl -sSfL -o "$$tmp/$$asset.sha256" "$$base/$$asset.sha256" || exit 1; \
	if command -v sha256sum >/dev/null 2>&1; then \
		( cd "$$tmp" && sha256sum -c "$$asset.sha256" ) || exit 1; \
	else \
		( cd "$$tmp" && shasum -a 256 -c "$$asset.sha256" ) || exit 1; \
	fi; \
	tar -xf "$$tmp/$$asset" -C "$$tmp" || exit 1; \
	bin=$$(find "$$tmp" -name pgnoop -type f | head -1); \
	install -m 0755 "$$bin" $(PGNOOP_EMBED_DIR)/pgnoop || exit 1; \
	rm -rf "$$tmp"; \
	echo "installed $(PGNOOP_EMBED_DIR)/pgnoop"

build-pgnoop: pgnoop-fetch # Build stroppy with the pg-noop baseline server embedded
	$(MAKE) build GO_BUILD_TAGS="-tags=pgnoop_embed"

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

# Gate one smoke run: fail on a non-zero exit, an error log, or the final
# completed-with-errors marker. Nonfatal iteration/query errors intentionally
# exit 0, so the summary marker keeps smoke runs strict.
# $(1)=label, rest=command.
define smoke_run
	echo "== $(1) =="; \
	out=$$($(2) 2>&1); code=$$?; printf '%s\n' "$$out"; \
	if [ $$code -ne 0 ] || printf '%s' "$$out" | grep -Eq 'level=error|completed with errors'; then \
		echo "FAIL ($(1)): exit=$$code"; rc=1; \
	fi
endef

# Scenario-branch smoke on the noop driver (NO database). Every workload has
# two executor archetypes — constant-vus (DURATION set, throughput) and
# shared-iterations (power test). Setup still runs against noop, while the
# database-dependent workload step is excluded: noop cannot return rows required
# by transactions such as TPC-C new_order. This still exercises each executor,
# typed options, setup lifecycle, and iteration scheduling without manufacturing
# expected query failures. The VUS=2/ITER=1 case also guards the
# shared-iterations "iterations < VUs" floor. Third field names any additional
# data-validation step to exclude; "-" means no additional step.
.PHONY: run-scenario-smoke
run-scenario-smoke: # Tier 0: scenario-branch smoke on noop (no DB), all workloads x both branches
	@rc=0;                                                                          \
	for spec in "tpcb/tx 1 -" "tpcc/tx 1 validate_population" "tpcds 0.01 -" "tpch/tx 0.01 validate_answers"; do \
		set -- $$spec; wl=$$1; sf=$$2; skip=$$3; ns="--no-steps workload"; [ "$$skip" = "-" ] || ns="--no-steps workload,$$skip"; \
		$(call smoke_run,noop constant-vus: $$wl,./build/stroppy run $$wl -d noop -e SCALE_FACTOR=$$sf -e DURATION=2s -e VUS=2 $$ns); \
		$(call smoke_run,noop shared-iters: $$wl,./build/stroppy run $$wl -d noop -e SCALE_FACTOR=$$sf -e VUS=2 -e ITER=1 $$ns); \
	done;                                                                           \
	exit $$rc

# Real-Postgres smoke of BOTH scenario branches for the light workloads at tiny
# scale (default pg preset = localhost:5432). Constant-vus uses one VU so this
# scenario-shape gate does not become a contention/retry test; shared-iterations
# keeps VUS=2/ITER=1 to guard the iterations-below-VUs floor.
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
		$(call smoke_run,pg constant-vus: $$wl,./build/stroppy run $$wl -e SCALE_FACTOR=$$sf -e DURATION=2s -e VUS=1 $$ns); \
		$(call smoke_run,pg shared-iters: $$wl,./build/stroppy run $$wl -e SCALE_FACTOR=$$sf -e VUS=2 -e ITER=1 $$ns); \
	done;                                                                           \
	exit $$rc

##
## Baseline PostgreSQL + MySQL integration harness
##

.PHONY: tmpfs-up tmpfs-down tmpfs-clean tmpfs-psql

tmpfs-up: # Start baseline tmpfs PostgreSQL and MySQL services
	docker compose -f test/compose.tmpfs.yml up -d --wait

tmpfs-down: # Stop baseline integration services and remove their volumes
	docker compose -f test/compose.tmpfs.yml down -v

tmpfs-clean: # Recycle baseline integration services; discard all data
	$(MAKE) tmpfs-down && $(MAKE) tmpfs-up

tmpfs-psql: # Open psql shell into the tmpfs Postgres container
	docker exec -it stroppy-pg-tmpfs psql -U postgres -d stroppy

##
## Optional multi-DB tmpfs integration harness
##

.PHONY: tmpfs-all-up tmpfs-all-down tmpfs-all-clean

tmpfs-all-up: # Start optional PostgreSQL, MySQL, Picodata, and YDB harness
	docker compose -f test/compose.tmpfs-all.yml up -d --wait pg-tmpfs-all mysql-tmpfs-all picodata-tmpfs-all ydb-tmpfs-all
	docker compose -f test/compose.tmpfs-all.yml up picodata-init

tmpfs-all-down: # Stop + remove all 4 DBs and their volumes
	docker compose -f test/compose.tmpfs-all.yml down -v

tmpfs-all-clean: # Recycle the 4-DB harness; discards all data
	$(MAKE) tmpfs-all-down && $(MAKE) tmpfs-all-up
