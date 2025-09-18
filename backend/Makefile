.PHONY: build run test clean deps dev

# Переменные
BINARY_NAME=stroppy-cloud-pannel
MAIN_PATH=./cmd/stroppy-cloud-pannel
BUILD_DIR=./bin

# Сборка приложения
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Запуск приложения
run:
	@echo "Running $(BINARY_NAME)..."
	@go run $(MAIN_PATH)

# Запуск в режиме разработки с автоперезагрузкой
dev:
	@echo "Starting development server..."
	@go run $(MAIN_PATH)

# Запуск тестов
test:
	@echo "Running tests..."
	@go test -v ./...

# Запуск тестов с покрытием
test-coverage:
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Установка зависимостей
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Очистка
clean:
	@echo "Cleaning up..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@rm -f *.db

# Инициализация базы данных (для разработки)
init-db:
	@echo "Initializing database..."
	@rm -f stroppy.db
	@go run $(MAIN_PATH) &
	@sleep 2
	@pkill -f $(BINARY_NAME) || true
	@echo "Database initialized"

# Форматирование кода
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Линтинг
lint:
	@echo "Running linter..."
	@golangci-lint run

# Создание Docker образа
docker-build:
	@echo "Building Docker image..."
	@docker build -t $(BINARY_NAME) .

# Помощь
help:
	@echo "Available commands:"
	@echo "  build         - Build the application"
	@echo "  run           - Run the application"
	@echo "  dev           - Run in development mode"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  deps          - Install dependencies"
	@echo "  clean         - Clean build artifacts"
	@echo "  init-db       - Initialize database"
	@echo "  fmt           - Format code"
	@echo "  lint          - Run linter"
	@echo "  docker-build  - Build Docker image"
	@echo "  help          - Show this help"
