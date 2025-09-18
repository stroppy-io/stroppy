# Stroppy Cloud Panel Backend

Golang backend для Stroppy Cloud Panel с использованием Gin, SQLite, DDD архитектуры и TDD подхода.

## Архитектура

Проект построен с использованием Domain-Driven Design (DDD) и имеет следующую структуру:

```
backend/
├── cmd/stroppy-cloud-pannel/    # Точка входа приложения
├── internal/
│   ├── domain/                  # Доменные модели и интерфейсы
│   │   ├── user/               # Доменная модель пользователя
│   │   └── run/                # Доменная модель запуска
│   ├── repository/sqlite/       # Реализация репозиториев для SQLite
│   ├── service/                # Бизнес-логика
│   ├── handler/                # HTTP обработчики
│   └── middleware/             # Middleware для аутентификации
└── pkg/                        # Переиспользуемые пакеты
    ├── auth/                   # JWT аутентификация
    └── database/               # Работа с базой данных
```

## Возможности

- ✅ Регистрация и аутентификация пользователей
- ✅ JWT токены для авторизации
- ✅ CRUD операции для запусков (Runs)
- ✅ Авторизация на уровне ресурсов
- ✅ Пагинация для списков
- ✅ Валидация входных данных
- ✅ Обработка ошибок
- ✅ Unit тесты
- ✅ SQLite база данных
- ✅ CORS поддержка

## API Endpoints

### Аутентификация

- `POST /api/v1/auth/register` - Регистрация пользователя
- `POST /api/v1/auth/login` - Вход в систему

### Пользователи

- `GET /api/v1/profile` - Получить профиль текущего пользователя

### Запуски

- `POST /api/v1/runs` - Создать новый запуск
- `GET /api/v1/runs` - Получить список запусков (с пагинацией)
- `GET /api/v1/runs/:id` - Получить запуск по ID
- `PUT /api/v1/runs/:id` - Обновить запуск
- `PUT /api/v1/runs/:id/status` - Обновить статус запуска
- `DELETE /api/v1/runs/:id` - Удалить запуск

### Служебные

- `GET /health` - Проверка состояния сервиса

## Установка и запуск

### Требования

- Go 1.21+
- SQLite3

### Установка зависимостей

```bash
make deps
```

### Запуск в режиме разработки

```bash
make run
```

### Сборка

```bash
make build
```

### Запуск тестов

```bash
make test
```

### Запуск тестов с покрытием

```bash
make test-coverage
```

## Конфигурация

Приложение настраивается через переменные окружения:

- `DB_PATH` - Путь к файлу базы данных SQLite (по умолчанию: `./stroppy.db`)
- `JWT_SECRET` - Секретный ключ для JWT токенов (по умолчанию: `your-secret-key-change-in-production`)
- `PORT` - Порт для HTTP сервера (по умолчанию: `8080`)

## Примеры использования API

### Регистрация

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "testpassword"
  }'
```

### Вход в систему

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "testpassword"
  }'
```

### Создание запуска

```bash
curl -X POST http://localhost:8080/api/v1/runs \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "name": "Test Run",
    "description": "Test run description",
    "config": "{\"param1\": \"value1\"}"
  }'
```

### Получение списка запусков

```bash
curl -X GET "http://localhost:8080/api/v1/runs?page=1&limit=10" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

## Тестирование

Проект включает unit тесты для всех основных компонентов:

- Доменные модели
- Сервисы
- JWT аутентификация
- Репозитории (интеграционные тесты)

Запуск тестов:

```bash
make test
```

## Разработка

### Форматирование кода

```bash
make fmt
```

### Линтинг

```bash
make lint
```

### Очистка

```bash
make clean
```

## Makefile команды

- `make build` - Сборка приложения
- `make run` - Запуск приложения
- `make dev` - Запуск в режиме разработки
- `make test` - Запуск тестов
- `make test-coverage` - Запуск тестов с покрытием
- `make deps` - Установка зависимостей
- `make clean` - Очистка артефактов сборки
- `make fmt` - Форматирование кода
- `make lint` - Линтинг
- `make help` - Показать справку
