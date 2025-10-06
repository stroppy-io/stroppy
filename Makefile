.PHONY: build run dev stop clean logs help

# Переменные
IMAGE_NAME=stroppy-cloud-panel
CONTAINER_NAME=stroppy-cloud-panel
VERSION=latest

# Сборка Docker образа
build:
	@echo "Сборка Docker образа..."
	@docker build -t $(IMAGE_NAME):$(VERSION) .
	@echo "Образ $(IMAGE_NAME):$(VERSION) собран успешно"

# Запуск в продакшн режиме
run:
	@echo "Запуск контейнера в продакшн режиме..."
	@docker compose up -d --build
	@echo "Контейнер запущен на http://localhost:8080"

# Запуск в режиме разработки
dev:
	@echo "Запуск в режиме разработки..."
	@docker compose -f docker-compose.dev.yml up -d
	@echo "Backend: http://localhost:8080"
	@echo "Frontend: http://localhost:5173"

# Остановка контейнеров
stop:
	@echo "Остановка контейнеров..."
	@docker compose down
	@docker compose -f docker-compose.dev.yml down

# Перестройка и запуск
rebuild: clean build run

# Очистка
clean:
	@echo "Очистка Docker ресурсов..."
	@docker compose down -v --remove-orphans
	@docker compose -f docker-compose.dev.yml down -v --remove-orphans
	@docker image prune -f
	@docker volume prune -f

# Просмотр логов
logs:
	@docker compose logs -f

# Просмотр логов в режиме разработки
logs-dev:
	@docker compose -f docker-compose.dev.yml logs -f

# Вход в контейнер
shell:
	@docker exec -it $(CONTAINER_NAME) /bin/sh

# Проверка статуса
status:
	@docker compose ps
	@echo ""
	@docker compose -f docker-compose.dev.yml ps

# Тестирование API
test-api:
	@echo "Тестирование API..."
	@sleep 5
	@curl -s http://localhost:8080/health | grep -q "ok" && echo "✅ Health check прошел" || echo "❌ Health check не прошел"
	@echo "Тестирование регистрации..."
	@curl -s -X POST http://localhost:8080/api/v1/auth/register \
		-H "Content-Type: application/json" \
		-d '{"username": "testuser", "password": "testpass123"}' | grep -q "user" && echo "✅ Регистрация работает" || echo "⚠️  Пользователь может уже существовать"

# Резервное копирование данных
backup:
	@echo "Создание резервной копии данных..."
	@docker run --rm -v stroppy-cloud-panel_stroppy_data:/data -v $(PWD):/backup alpine tar czf /backup/stroppy_backup_$(shell date +%Y%m%d_%H%M%S).tar.gz -C /data .
	@echo "Резервная копия создана"

# Восстановление данных
restore:
	@echo "Для восстановления данных используйте:"
	@echo "docker run --rm -v stroppy-cloud-panel_stroppy_data:/data -v \$$(PWD):/backup alpine tar xzf /backup/your_backup_file.tar.gz -C /data"

# Обновление образа
update: stop build run

# Мониторинг ресурсов
monitor:
	@docker stats $(CONTAINER_NAME)

# Информация о образе
info:
	@docker images $(IMAGE_NAME)
	@echo ""
	@docker inspect $(IMAGE_NAME):$(VERSION) | grep -A 5 -B 5 "Created\|Size"

# Помощь
help:
	@echo "Доступные команды:"
	@echo "  build      - Сборка Docker образа"
	@echo "  run        - Запуск в продакшн режиме"
	@echo "  dev        - Запуск в режиме разработки"
	@echo "  stop       - Остановка контейнеров"
	@echo "  rebuild    - Перестройка и запуск"
	@echo "  clean      - Очистка Docker ресурсов"
	@echo "  logs       - Просмотр логов"
	@echo "  logs-dev   - Просмотр логов в режиме разработки"
	@echo "  shell      - Вход в контейнер"
	@echo "  status     - Проверка статуса контейнеров"
	@echo "  test-api   - Тестирование API"
	@echo "  backup     - Резервное копирование данных"
	@echo "  restore    - Информация о восстановлении данных"
	@echo "  update     - Обновление образа"
	@echo "  monitor    - Мониторинг ресурсов"
	@echo "  info       - Информация о образе"
	@echo "  help       - Показать эту справку"


SRC_PROTO_PATH=$(CURDIR)/tools/stroppy/proto/build
.PHONY: proto
proto:
	rm -rf $(CURDIR)/pkg/common/proto/*
	cd  $(CURDIR)/tools/stroppy/proto && $(MAKE) build
	cp -r $(SRC_PROTO_PATH)/go/* $(CURDIR)/backend/pkg/proto
	cp $(SRC_PROTO_PATH)/ts/* $(CURDIR)/frontend/src/proto
	cp $(SRC_PROTO_PATH)/docs/* $(CURDIR)/docs

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