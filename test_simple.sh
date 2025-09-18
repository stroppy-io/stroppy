#!/bin/bash

echo "=== Тестирование API ==="

# 1. Получаем токен
echo "1. Получение токена..."
RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "умный_тестер_9665", "password": "test_password_767"}')

echo "Ответ авторизации: $RESPONSE"

# Извлекаем токен (улучшенный способ)
TOKEN=$(echo "$RESPONSE" | python3 -c "import sys, json; print(json.load(sys.stdin)['token'])" 2>/dev/null)

if [ -z "$TOKEN" ]; then
    echo "Ошибка: не удалось получить токен"
    exit 1
fi

echo "Токен получен: ${TOKEN:0:50}..."

# 2. Тестируем получение данных
echo -e "\n2. Получение данных..."
curl -v -X GET "http://localhost:8080/api/v1/runs?page=1&limit=3" \
  -H "Authorization: Bearer $TOKEN"

echo -e "\n\n=== Конец теста ==="
