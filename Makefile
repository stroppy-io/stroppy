.PHONY: build run dev test test-coverage deps fmt lint clean init-db \
	docker-build docker-up docker-dev docker-stop docker-clean docker-logs \
	docker-logs-dev docker-shell docker-status docker-test-api docker-backup \
	docker-restore docker-update docker-monitor docker-info proto help

# Go application settings
BINARY_NAME ?= stroppy-cloud-panel
MAIN_PKG ?= ./cmd/stroppy-cloud-panel
BUILD_DIR ?= ./bin

# Container/deployment settings
IMAGE_NAME ?= stroppy-cloud-panel
VERSION ?= latest
CONTAINER_NAME ?= stroppy-cloud-panel
DEPLOYMENTS_DIR := ./deployments/docker
DOCKER_COMPOSE := $(DEPLOYMENTS_DIR)/docker-compose.yml
DOCKER_COMPOSE_DEV := $(DEPLOYMENTS_DIR)/docker-compose.dev.yml
DOCKERFILE := $(DEPLOYMENTS_DIR)/Dockerfile

# -----------------------------------------------------------------------------
# Go targets

build:
	@echo "➜ Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PKG)

run:
	@echo "➜ Running $(BINARY_NAME)..."
	@go run $(MAIN_PKG)

dev: run

test:
	@echo "➜ Running tests..."
	@go test ./...

test-coverage:
	@echo "➜ Running tests with coverage..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated at coverage.html"

deps:
	@echo "➜ Installing dependencies..."
	@go mod download
	@go mod tidy

fmt:
	@echo "➜ Formatting Go sources..."
	@go fmt ./...

lint:
	@echo "➜ Running golangci-lint..."
	@golangci-lint run

clean:
	@echo "➜ Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@rm -f *.db

init-db:
	@echo "➜ Initializing local database..."
	@rm -f stroppy.db
	@go run $(MAIN_PKG) &
	@sleep 2
	@pkill -f $(BINARY_NAME) || true
	@echo "Database initialized"

# -----------------------------------------------------------------------------
# Docker & Compose targets

docker-build:
	@echo "➜ Building container image $(IMAGE_NAME):$(VERSION)..."
	@docker build -f $(DOCKERFILE) -t $(IMAGE_NAME):$(VERSION) .

docker-up:
	@echo "➜ Starting stack (production compose)..."
	@docker compose -f $(DOCKER_COMPOSE) up -d --build
	@echo "Application available at http://localhost:8080"

docker-dev:
	@echo "➜ Starting development stack..."
	@docker compose -f $(DOCKER_COMPOSE_DEV) up -d --build
	@echo "Backend:  http://localhost:8080"
	@echo "Frontend: http://localhost:5173"

docker-stop:
	@echo "➜ Stopping compose stacks..."
	@docker compose -f $(DOCKER_COMPOSE) down
	@docker compose -f $(DOCKER_COMPOSE_DEV) down

docker-clean:
	@echo "➜ Cleaning compose resources..."
	@docker compose -f $(DOCKER_COMPOSE) down -v --remove-orphans
	@docker compose -f $(DOCKER_COMPOSE_DEV) down -v --remove-orphans
	@docker image prune -f
	@docker volume prune -f

docker-logs:
	@docker compose -f $(DOCKER_COMPOSE) logs -f

docker-logs-dev:
	@docker compose -f $(DOCKER_COMPOSE_DEV) logs -f

docker-shell:
	@docker exec -it $(CONTAINER_NAME) /bin/sh

docker-status:
	@docker compose -f $(DOCKER_COMPOSE) ps
	@echo ""
	@docker compose -f $(DOCKER_COMPOSE_DEV) ps

docker-test-api:
	@echo "➜ Smoke testing API..."
	@sleep 5
	@curl -s http://localhost:8080/health | grep -q "ok" && echo "✅ Health check passed" || echo "❌ Health check failed"
	@echo "➜ Testing registration endpoint..."
	@curl -s -X POST http://localhost:8080/api/v1/auth/register \
		-H "Content-Type: application/json" \
		-d '{"username": "testuser", "password": "testpass123"}' | grep -q "user" && echo "✅ Registration endpoint OK" || echo "⚠️  Registration may have failed or user already exists"

docker-backup:
	@echo "➜ Creating data backup..."
	@docker run --rm -v stroppy-cloud-panel_stroppy_data:/data -v $(PWD):/backup alpine tar czf /backup/stroppy_backup_$(shell date +%Y%m%d_%H%M%S).tar.gz -C /data .
	@echo "Backup created in $(PWD)"

docker-restore:
	@echo "Use the following command to restore from a backup:"
	@echo "docker run --rm -v stroppy-cloud-panel_stroppy_data:/data -v \$$(PWD):/backup alpine tar xzf /backup/your_backup_file.tar.gz -C /data"

docker-update: docker-stop docker-build docker-up

docker-monitor:
	@docker stats $(CONTAINER_NAME)

docker-info:
	@docker images $(IMAGE_NAME)
	@echo ""
	@docker inspect $(IMAGE_NAME):$(VERSION) | grep -A 5 -B 5 "Created\|Size"

# -----------------------------------------------------------------------------
# Proto generation

SRC_PROTO_PATH := $(CURDIR)/tools/stroppy/proto/build

proto:
	rm -rf $(CURDIR)/pkg/proto/*
	cd $(CURDIR)/tools/stroppy/proto && $(MAKE) build
	cp -r $(SRC_PROTO_PATH)/go/* $(CURDIR)/pkg/proto
	cp $(SRC_PROTO_PATH)/ts/* $(CURDIR)/web/src/proto
	cp $(SRC_PROTO_PATH)/docs/* $(CURDIR)/docs

# -----------------------------------------------------------------------------
# Helper

help:
	@echo "Common targets:"
	@echo "  build            - Build the Go binary"
	@echo "  run              - Run the application locally"
	@echo "  test             - Execute Go tests"
	@echo "  docker-up        - Start the production docker-compose stack"
	@echo "  docker-dev       - Start the development docker-compose stack"
	@echo "  docker-clean     - Remove compose resources and prune Docker"
	@echo "  proto            - Regenerate protobuf artefacts"
