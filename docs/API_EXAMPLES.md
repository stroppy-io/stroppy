# Примеры использования API

## Запуск сервера

```bash
# Из корневой директории проекта
make run

# Или напрямую
./bin/stroppy-cloud-panel
```

Сервер запустится на порту 8080 (по умолчанию).

## Проверка здоровья сервиса

```bash
curl -X GET http://localhost:8080/health
```

## Регистрация пользователя

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "testpassword123"
  }'
```

## Вход в систему

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "testpassword123"
  }'
```

Сохраните токен из ответа для использования в последующих запросах.

## Получение профиля пользователя

```bash
curl -X GET http://localhost:8080/api/v1/profile \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

## Создание запуска

```bash
curl -X POST http://localhost:8080/api/v1/runs \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "name": "Тестовый запуск",
    "description": "Описание тестового запуска",
    "config": "{\"param1\": \"value1\", \"param2\": \"value2\"}"
  }'
```

## Получение списка запусков

```bash
# Все запуски пользователя
curl -X GET http://localhost:8080/api/v1/runs \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"

# С пагинацией
curl -X GET "http://localhost:8080/api/v1/runs?page=1&limit=5" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

## Получение запуска по ID

```bash
curl -X GET http://localhost:8080/api/v1/runs/1 \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

## Обновление запуска

```bash
curl -X PUT http://localhost:8080/api/v1/runs/1 \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "name": "Обновленный запуск",
    "description": "Новое описание",
    "config": "{\"param1\": \"new_value1\"}"
  }'
```

## Обновление статуса запуска

```bash
curl -X PUT http://localhost:8080/api/v1/runs/1/status \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "status": "running",
    "result": "{\"progress\": 50}"
  }'
```

Возможные статусы: `pending`, `running`, `completed`, `failed`, `cancelled`

## Удаление запуска

```bash
curl -X DELETE http://localhost:8080/api/v1/runs/1 \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

## Полный пример тестирования

```bash
#!/bin/bash

# 1. Регистрация
echo "1. Регистрация пользователя..."
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "testpassword123"}'

echo -e "\n\n2. Вход в систему..."
# 2. Вход и получение токена
RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "testpassword123"}')

TOKEN=$(echo $RESPONSE | grep -o '"token":"[^"]*' | cut -d'"' -f4)
echo "Токен: $TOKEN"

echo -e "\n\n3. Создание запуска..."
# 3. Создание запуска
RUN_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/runs \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name": "Тестовый запуск", "description": "Описание", "config": "{\"test\": true}"}')

RUN_ID=$(echo $RUN_RESPONSE | grep -o '"id":[0-9]*' | cut -d':' -f2)
echo "ID запуска: $RUN_ID"

echo -e "\n\n4. Получение списка запусков..."
# 4. Получение списка запусков
curl -s -X GET http://localhost:8080/api/v1/runs \
  -H "Authorization: Bearer $TOKEN" | jq '.'

echo -e "\n\n5. Обновление статуса запуска..."
# 5. Обновление статуса
curl -s -X PUT http://localhost:8080/api/v1/runs/$RUN_ID/status \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"status": "completed", "result": "{\"success\": true}"}' | jq '.'
```
