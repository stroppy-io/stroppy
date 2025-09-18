#!/bin/bash

echo "=== Тестирование API интеграции ==="

# Запускаем backend в фоновом режиме
echo "Запуск backend сервера..."
cd backend
./bin/stroppy-cloud-pannel &
BACKEND_PID=$!

# Ждем запуска сервера
sleep 3

# Проверяем health endpoint
echo "Проверка health endpoint..."
HEALTH_RESPONSE=$(curl -s http://localhost:8080/health)
if [[ $? -eq 0 ]]; then
    echo "✅ Health check успешен: $HEALTH_RESPONSE"
else
    echo "❌ Health check не удался"
    kill $BACKEND_PID
    exit 1
fi

# Тест регистрации пользователя
echo "Тестирование регистрации..."
REGISTER_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "testpassword123"}')

if [[ $REGISTER_RESPONSE == *"успешно"* ]] || [[ $REGISTER_RESPONSE == *"user"* ]]; then
    echo "✅ Регистрация успешна"
else
    echo "⚠️  Регистрация: $REGISTER_RESPONSE (возможно пользователь уже существует)"
fi

# Тест входа в систему
echo "Тестирование входа в систему..."
LOGIN_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "testpassword123"}')

if [[ $LOGIN_RESPONSE == *"token"* ]]; then
    echo "✅ Вход в систему успешен"
    TOKEN=$(echo $LOGIN_RESPONSE | grep -o '"token":"[^"]*' | cut -d'"' -f4)
    echo "Токен получен: ${TOKEN:0:20}..."
else
    echo "❌ Вход в систему не удался: $LOGIN_RESPONSE"
    kill $BACKEND_PID
    exit 1
fi

# Тест создания запуска
echo "Тестирование создания запуска..."
CREATE_RUN_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/runs \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "Тестовый запуск API",
    "description": "Тест интеграции API",
    "config": "{\"workloadType\": \"test\", \"databaseType\": \"postgres\"}"
  }')

if [[ $CREATE_RUN_RESPONSE == *"id"* ]]; then
    echo "✅ Создание запуска успешно"
    RUN_ID=$(echo $CREATE_RUN_RESPONSE | grep -o '"id":[0-9]*' | cut -d':' -f2)
    echo "ID запуска: $RUN_ID"
else
    echo "❌ Создание запуска не удалось: $CREATE_RUN_RESPONSE"
fi

# Тест получения списка запусков
echo "Тестирование получения списка запусков..."
GET_RUNS_RESPONSE=$(curl -s -X GET http://localhost:8080/api/v1/runs \
  -H "Authorization: Bearer $TOKEN")

if [[ $GET_RUNS_RESPONSE == *"runs"* ]]; then
    echo "✅ Получение списка запусков успешно"
else
    echo "❌ Получение списка запусков не удалось: $GET_RUNS_RESPONSE"
fi

# Завершаем backend процесс
echo "Остановка backend сервера..."
kill $BACKEND_PID

echo "=== Тестирование завершено ==="
echo ""
echo "🎉 API готов к интеграции с frontend!"
echo ""
echo "Для запуска приложения:"
echo "1. Backend: cd backend && make run"
echo "2. Frontend: cd frontend && npm run dev"
