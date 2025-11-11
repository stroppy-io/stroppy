# Stroppy Cloud Panel

Полноценная панель управления облачными нагрузками Stroppy. Репозиторий объединяет backend на Go и frontend на React, а также инфраструктурные манифесты.

## Структура

```
.
├── cmd/                     # Точки входа бинарей
├── internal/                # Бизнес-логика и HTTP-слой
├── pkg/                     # Переиспользуемые Go-пакеты (auth, db, proto)
├── web/                     # Frontend (React + Vite)
├── deployments/             # Docker/Compose/Helm/Caddy конфигурации
├── docs/                    # Дополнительная документация и примеры API
├── proto/                   # Источник protobuf-схем
├── Makefile                 # Унифицированные задачи для разработки
└── env*.example             # Образцы переменных окружения
```

## Backend (Go 1.24+)

Основная точка входа: `cmd/stroppy-cloud-panel/main.go`.

```bash
# Установка зависимостей
make deps

# Сборка бинаря в ./bin
make build

# Запуск локально
make run

# Тесты и покрытие
make test
make test-coverage
```

Полный список целей `make help`.

## Frontend (`web/`)

Stack: React 19, TypeScript, Vite, Tailwind CSS, Ant Design.

```bash
cd web
yarn install
yarn dev        # localhost:5173
yarn build      # продакшен сборка в web/dist
```

Переменные окружения описаны в `web/env.example`.

## Контейнеры и деплой

Все Docker/Compose/Helm файлы лежат в `deployments/`.

```bash
# Сборка production-образа (корневой Dockerfile)
docker build --build-arg VERSION=$(git describe --tags --always) -t stroppy-cloud-panel:latest .

# Продакшен compose-стек
make docker-up

# Dev-стек (hot reload для Go и React)
make docker-dev

# Остановка и очистка
make docker-stop
make docker-clean
```

HTTP-сервер теперь умеет отдавать собранный frontend. Путь к статике задаётся
в конфиге `service.server.static_dir` или через переменную окружения
`SERVICE_SERVER_STATIC_DIR` (Dockerfile пробрасывает `/app/frontend`).

Дополнительная документация по API и примерам запросов находится в `docs/`.
