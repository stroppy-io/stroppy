#!/usr/bin/env python3
"""
Скрипт для генерации тестовых данных в Stroppy Cloud Panel
Регистрирует пользователя и создает 1000 тестовых запусков
"""

import requests
import json
import random
import time
from datetime import datetime, timedelta
import uuid

# Конфигурация
BASE_URL = "http://localhost:8080"
API_BASE = f"{BASE_URL}/api/v1"
TOTAL_RUNS = 1000
BATCH_SIZE = 50  # Количество запросов в батче для лучшей производительности

# Генерация случайного пользователя
def generate_random_user():
    """Генерирует случайные данные пользователя"""
    adjectives = ["быстрый", "умный", "сильный", "ловкий", "мудрый", "храбрый", "тихий", "яркий"]
    nouns = ["тестер", "разработчик", "админ", "пользователь", "инженер", "аналитик", "архитектор"]
    
    username = f"{random.choice(adjectives)}_{random.choice(nouns)}_{random.randint(1000, 9999)}"
    password = f"test_password_{random.randint(100, 999)}"
    
    return {
        "username": username,
        "password": password
    }

# Генерация тестовых запусков
def generate_test_runs(count):
    """Генерирует список тестовых запусков"""
    statuses = ["pending", "running", "completed", "failed", "cancelled"]
    status_weights = [0.1, 0.2, 0.5, 0.15, 0.05]  # Веса для более реалистичного распределения
    
    run_types = [
        "Нагрузочное тестирование",
        "Функциональное тестирование", 
        "Интеграционное тестирование",
        "Smoke тестирование",
        "Регрессионное тестирование",
        "API тестирование",
        "UI тестирование",
        "Безопасность тестирование"
    ]
    
    environments = ["dev", "test", "staging", "prod"]
    
    runs = []
    
    for i in range(count):
        run_type = random.choice(run_types)
        env = random.choice(environments)
        run_id = str(uuid.uuid4())[:8]
        
        # Генерация конфигурации
        config = {
            "environment": env,
            "threads": random.randint(1, 20),
            "duration": random.randint(60, 3600),  # от 1 минуты до 1 часа
            "target_url": f"https://{env}.example.com/api",
            "timeout": random.randint(5, 30),
            "ramp_up": random.randint(10, 300),
            "test_data": {
                "users_count": random.randint(10, 1000),
                "iterations": random.randint(1, 100)
            }
        }
        
        # Генерация результатов для завершенных тестов
        result = None
        if random.choice(statuses) in ["completed", "failed"]:
            if random.random() > 0.2:  # 80% успешных
                result = {
                    "success": True,
                    "total_requests": random.randint(1000, 50000),
                    "successful_requests": random.randint(950, 49500),
                    "failed_requests": random.randint(0, 500),
                    "avg_response_time": round(random.uniform(50, 500), 2),
                    "max_response_time": round(random.uniform(500, 2000), 2),
                    "throughput": round(random.uniform(10, 500), 2),
                    "errors": []
                }
            else:  # 20% с ошибками
                result = {
                    "success": False,
                    "error": random.choice([
                        "Connection timeout",
                        "Server error 500",
                        "Authentication failed",
                        "Resource not found",
                        "Rate limit exceeded"
                    ]),
                    "total_requests": random.randint(100, 1000),
                    "successful_requests": random.randint(0, 500),
                    "failed_requests": random.randint(100, 900)
                }
        
        run = {
            "name": f"{run_type} #{i+1:04d} ({env})",
            "description": f"Автоматически сгенерированный {run_type.lower()} для окружения {env}. ID: {run_id}",
            "config": json.dumps(config),
            "status": random.choices(statuses, weights=status_weights)[0],
            "result": json.dumps(result) if result else None
        }
        
        runs.append(run)
    
    return runs

def register_user(user_data):
    """Регистрирует нового пользователя"""
    print(f"🔐 Регистрация пользователя: {user_data['username']}")
    
    try:
        response = requests.post(
            f"{API_BASE}/auth/register",
            headers={"Content-Type": "application/json"},
            json=user_data,
            timeout=10
        )
        
        if response.status_code == 201:
            print("✅ Пользователь успешно зарегистрирован")
            return True
        elif response.status_code == 409:
            print("⚠️  Пользователь уже существует, продолжаем...")
            return True
        else:
            print(f"❌ Ошибка регистрации: {response.status_code} - {response.text}")
            return False
            
    except requests.exceptions.RequestException as e:
        print(f"❌ Ошибка соединения при регистрации: {e}")
        return False

def login_user(user_data):
    """Авторизует пользователя и возвращает токен"""
    print(f"🔑 Авторизация пользователя: {user_data['username']}")
    
    try:
        response = requests.post(
            f"{API_BASE}/auth/login",
            headers={"Content-Type": "application/json"},
            json=user_data,
            timeout=10
        )
        
        if response.status_code == 200:
            data = response.json()
            token = data.get("token")
            if token:
                print("✅ Успешная авторизация")
                return token
            else:
                print("❌ Токен не найден в ответе")
                return None
        else:
            print(f"❌ Ошибка авторизации: {response.status_code} - {response.text}")
            return None
            
    except requests.exceptions.RequestException as e:
        print(f"❌ Ошибка соединения при авторизации: {e}")
        return None

def create_run(token, run_data):
    """Создает один запуск"""
    try:
        response = requests.post(
            f"{API_BASE}/runs",
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Bearer {token}"
            },
            json=run_data,
            timeout=10
        )
        
        if response.status_code == 201:
            data = response.json()
            run_id = data.get("id")
            
            # Если есть статус и результат, обновляем их
            if run_data.get("status") != "pending" or run_data.get("result"):
                update_run_status(token, run_id, run_data.get("status", "pending"), run_data.get("result"))
            
            return run_id
        else:
            print(f"❌ Ошибка создания запуска: {response.status_code} - {response.text}")
            return None
            
    except requests.exceptions.RequestException as e:
        print(f"❌ Ошибка соединения при создании запуска: {e}")
        return None

def update_run_status(token, run_id, status, result=None):
    """Обновляет статус запуска"""
    if not run_id:
        return False
        
    try:
        payload = {"status": status}
        if result:
            payload["result"] = result
            
        response = requests.put(
            f"{API_BASE}/runs/{run_id}/status",
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Bearer {token}"
            },
            json=payload,
            timeout=10
        )
        
        return response.status_code == 200
        
    except requests.exceptions.RequestException as e:
        print(f"❌ Ошибка обновления статуса: {e}")
        return False

def check_server():
    """Проверяет доступность сервера"""
    print("🔍 Проверка доступности сервера...")
    
    try:
        response = requests.get(f"{BASE_URL}/health", timeout=5)
        if response.status_code == 200:
            print("✅ Сервер доступен")
            return True
        else:
            print(f"❌ Сервер недоступен: {response.status_code}")
            return False
    except requests.exceptions.RequestException as e:
        print(f"❌ Ошибка соединения с сервером: {e}")
        return False

def main():
    """Основная функция"""
    print("🚀 Генератор тестовых данных для Stroppy Cloud Panel")
    print("=" * 60)
    
    # Проверка доступности сервера
    if not check_server():
        print("❌ Сервер недоступен. Убедитесь, что backend запущен на localhost:8080")
        return
    
    # Генерация пользователя
    user_data = generate_random_user()
    print(f"👤 Сгенерирован пользователь: {user_data['username']}")
    
    # Регистрация пользователя
    if not register_user(user_data):
        print("❌ Не удалось зарегистрировать пользователя")
        return
    
    # Авторизация
    token = login_user(user_data)
    if not token:
        print("❌ Не удалось получить токен авторизации")
        return
    
    # Генерация тестовых запусков
    print(f"\n📊 Генерация {TOTAL_RUNS} тестовых запусков...")
    runs = generate_test_runs(TOTAL_RUNS)
    
    # Создание запусков
    print(f"⚡ Создание запусков (батчами по {BATCH_SIZE})...")
    created_count = 0
    failed_count = 0
    
    for i in range(0, len(runs), BATCH_SIZE):
        batch = runs[i:i+BATCH_SIZE]
        batch_num = (i // BATCH_SIZE) + 1
        total_batches = (len(runs) + BATCH_SIZE - 1) // BATCH_SIZE
        
        print(f"📦 Обработка батча {batch_num}/{total_batches} ({len(batch)} запусков)...")
        
        for j, run in enumerate(batch):
            run_id = create_run(token, run)
            if run_id:
                created_count += 1
            else:
                failed_count += 1
            
            # Показываем прогресс каждые 10 запусков
            if (created_count + failed_count) % 10 == 0:
                progress = ((created_count + failed_count) / TOTAL_RUNS) * 100
                print(f"   📈 Прогресс: {created_count + failed_count}/{TOTAL_RUNS} ({progress:.1f}%)")
        
        # Небольшая пауза между батчами
        if i + BATCH_SIZE < len(runs):
            time.sleep(0.1)
    
    # Финальная статистика
    print("\n" + "=" * 60)
    print("📊 РЕЗУЛЬТАТЫ ГЕНЕРАЦИИ:")
    print(f"✅ Успешно создано запусков: {created_count}")
    print(f"❌ Ошибок при создании: {failed_count}")
    print(f"👤 Пользователь: {user_data['username']}")
    print(f"🔑 Пароль: {user_data['password']}")
    print("\n🎉 Генерация тестовых данных завершена!")
    print(f"🌐 Откройте http://localhost:5173 для просмотра данных")

if __name__ == "__main__":
    main()
