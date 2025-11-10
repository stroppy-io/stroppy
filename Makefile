VERSION := $(shell git describe --tags | sed -e 's/^v//g' | awk -F "-" '{print $$1}')

RUN_ARGS := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
$(eval $(RUN_ARGS):;@:)

ifneq (,$(wildcard ./.env))
    include .env
    export
endif

LOCAL_BIN:=$(CURDIR)/bin
PATH:=$(PATH):$(LOCAL_BIN)
GOPRIVATE:="github.com/stroppy-io"
GOPROXY:=https://goproxy.io,direct

default: help

.PHONY: help
help: # Показывает информацию о каждом рецепте в Makefile
	@grep -E '^[a-zA-Z0-9 _-]+:.*#'  Makefile | sort | while read -r l; do printf "\033[1;32m$$(echo $$l | cut -f 1 -d':')\033[00m:$$(echo $$l | cut -f 2- -d'#')\n"; done

.PHONY: .install-linter
.install-linter: # Install golangci-lint
	$(info Installing golangci-lint...)
	mkdir -p $(LOCAL_BIN)
	GOBIN=$(LOCAL_BIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8

.PHONY: .bin_deps
.bin_deps: .install-linter # Устанавливает зависимости необходимые для работы приложения
	mkdir -p $(LOCAL_BIN)
	GOBIN=$(LOCAL_BIN) GOPROXY=direct go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	GOBIN=$(LOCAL_BIN) GOPROXY=direct go install github.com/maoueh/zap-pretty/cmd/zap-pretty@latest
	GOBIN=$(LOCAL_BIN) GOPROXY=direct go install golang.org/x/tools/cmd/goimports@latest


.PHONY: .app_deps
.app_deps: submodules # Устанавливает необходимые go пакеты
	GOPROXY=$(GOPROXY) GOPRIVATE=$(GOPRIVATE) go mod tidy

.PHONY: .update_mod
.update_mod: # Устанавливает необходимые go пакеты
	GOPROXY=$(GOPROXY) GOPRIVATE=$(GOPRIVATE) go get -u ./...

.PHONY: .go_get
.go_get: # Хелпер для go get
	GOPROXY=$(GOPROXY) GOPRIVATE=$(GOPRIVATE) go get $(RUN_ARGS)

REQUIRED_BINS := docker go protoc protoc-gen-go protoc-gen-go-grpc easyp
.check-bins:
	$(foreach bin,$(REQUIRED_BINS),\
        $(if $(shell command -v $(bin) 2> /dev/null),$(info Found `$(bin)`),$(error Please install `$(bin)`)))

.PHONY: configure
configure: .check-bins .bin_deps .app_deps # Устанавливает все зависимости для работы приложения

.PHONY: submodules
submodules: # Загрузка подмодулей
	git submodule update --init --recursive --remote

PROTOCOL_PATH=$(CURDIR)/tools/proto
.PHONY: protocols
protocols:
	cd $(PROTOCOL_PATH) && easyp generate  && easyp -cfg easyp.ts.yaml generate

.PHONY: tests
tests: # Запускает юнит тесты с ковереджем
	docker compose down -v
	$(call .sql-migrate,apply,$(RUN_ARGS))
	go test -race -coverprofile=coverage.out ./...

.PHONY: tests_integration
tests_integration: # Запуск интеграционных тестов (пока только локально)
	go test -race -tags=integration -coverprofile=coverage.out ./...

.PHONY: show_cover
show_cover: # Открывает ковередж юнит тестов
	go tool cover -html=coverage.out

.PHONY: linter
linter: # Запуск линтеров
	$(LOCAL_BIN)/golangci-lint cache clean && \
	$(LOCAL_BIN)/golangci-lint run

.PHONY: linter_fix
linter_fix: # Запуск линтеров с фиксом
	$(LOCAL_BIN)/golangci-lint cache clean && \
	$(LOCAL_BIN)/golangci-lint run --fix

.PHONY: imports_fix
imports_fix: # Запуск фикса импортов для internal
	$(LOCAL_BIN)/goimports -w $(shell find $(CURDIR)/internal -type f -name '*.go')

.PHONY: quality
quality: linter tests_integration # Запуск линтеров и интеграционных тестов

.PHONY: .clean_cache
.clean_cache: # Очистка кеша go
	go clean -cache

binary_name=stroppy-cloud-panel-server
.PHONY: build
build: # Компиляция проекта
	go build -ldflags="-X github.com/stroppy-io/stroppy-cloud-panel/internal/core/build.Version=$(VERSION) -X github.com/stroppy-io/stroppy-cloud-panel/internal/core/build.ServiceName=stroppy-cloud-panel-server" \
			-v -o $(binary_name) ./cmd

.PHONY: run
run: build
	./$(binary_name) 2>&1 | $(LOCAL_BIN)/zap-pretty

.PHONY: build_docker
build_docker: # Сборка докер образа
	docker build --build-arg VERSION=$(VERSION) --build-arg GITHUB_TOKEN=$(GITHUB_TOKEN) -t stroppy-cloud-panel-server:$(VERSION) -f deployments/docker/Dockerfile .

.PHONY: mockery
mockery: # Создание моков
	$(LOCAL_BIN)/mockery --name $(name) --dir $(dir) --output $(dir)/mocks


# Add mocking interface as make mockery name=Interface dir=./path/to/interface/dir
.PHONY: mock
mock:

.PHONY: sql-diff
sql-diff: # Автоматическая генерация SQL миграций в tools/sql
	cd $(CURDIR)/tools/sql && $(MAKE) generate-diff

.PHONY: sqlc
sqlc: # Генерация SQL структуры из миграций
	cd $(CURDIR)/tools/sql && $(MAKE) generate-code


# slq atlas migrate
MY_UID=$(shell id -u)
MY_GID=$(shell id -g)
#.sql-migrate = $(LOCAL_BIN)/sql-migrate $1 -config=$(SQL_MIGRATE_CONFIG) -env=$(SQL_MIGRATE_ENV) $2
.sql-migrate = COMPOSE_PROFILES=migrate CURRENT_UID=$(MY_UID):$(MY_GID) ARG="$(1) $(2)" docker compose up --build --abort-on-container-exit migrate &&\
COMPOSE_PROFILES=migrate CURRENT_UID=$(MY_UID):$(MY_GID) docker compose down migrate

# sql migrations
.PHONY: migrate-status
migrate-status: # Статус миграций
	$(call .sql-migrate,status)

# make migrate_new NEW_MIGRATION_NAME
.PHONY: migrate-newf
migrate-new: # Создание миграции
	$(call .sql-migrate,new,$(RUN_ARGS))

# make migrate_up MIGRATION_ID
# if MIGRATION_ID is empty -> migrate to the latest version
.PHONY: migrate-up
migrate-up: # Применение миграции
	$(call .sql-migrate,apply,$(RUN_ARGS))

# make migrate_down MIGRATION_ID
# if MIGRATION_ID is empty -> migrate to the first version
.PHONY: migrate-down
migrate-down: # Откат миграции
	$(call .sql-migrate,down,$(RUN_ARGS))

.PHONY: migrate-clear
migrate-clear: migrate-down migrate-up # Откат базы через миграцию

.PHONY: .atlas
.atlas:
	$(call .sql-migrate,$(RUN_ARGS))


branch=main
.PHONY: revision
revision: # Создание тега
	@if [ -e $(tag) ]; then \
		echo "error: Specify version 'tag='"; \
		exit 1; \
	fi
	git tag -d ${tag} || true
	git push --delete origin ${tag} || true
	git tag $(tag)
	git push origin $(tag)


.PHONY: clean-infra
clean-infra: # Удаление данных инфраструктуры
	docker compose down -v

.PHONY: start-infra
start-infra:  # Запуск инфраструктуры
	docker compose up -d --build

.PHONY: stop-infra
stop-infra: # Остановка инфраструктуры
	docker compose down

.PHONY: restart-infra
restart-infra: stop-infra start-infra  # инфраструктуры


# -----------------------------------------------------------------------------

