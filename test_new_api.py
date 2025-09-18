#!/usr/bin/env python3
"""
Тестирование нового API для получения общих данных
"""

import requests
import json

BASE_URL = "http://localhost:8080"
API_BASE = f"{BASE_URL}/api/v1"

def test_api():
    print("🔍 Тестирование нового API...")
    
    # 1. Авторизация
    print("1. Авторизация...")
    login_data = {
        "username": "умный_тестер_9665",
        "password": "test_password_767"
    }
    
    try:
        response = requests.post(f"{API_BASE}/auth/login", json=login_data)
        if response.status_code == 200:
            auth_data = response.json()
            token = auth_data.get("token")
            print(f"✅ Авторизация успешна. Пользователь: {auth_data['user']['username']}")
        else:
            print(f"❌ Ошибка авторизации: {response.status_code} - {response.text}")
            return
    except Exception as e:
        print(f"❌ Ошибка соединения: {e}")
        return
    
    # 2. Получение данных
    print("2. Получение данных...")
    headers = {"Authorization": f"Bearer {token}"}
    
    try:
        response = requests.get(f"{API_BASE}/runs?page=1&limit=10", headers=headers)
        print(f"Статус ответа: {response.status_code}")
        
        if response.status_code == 200:
            data = response.json()
            print(f"✅ Данные получены успешно!")
            print(f"   Всего записей: {data.get('total', 'N/A')}")
            print(f"   Записей на странице: {len(data.get('runs', []))}")
            print(f"   Страница: {data.get('page', 'N/A')}")
            print(f"   Лимит: {data.get('limit', 'N/A')}")
            
            # Показываем первые несколько записей
            runs = data.get('runs', [])
            if runs:
                print("\n📊 Первые записи:")
                for i, run in enumerate(runs[:3]):
                    print(f"   {i+1}. ID: {run['id']}, Название: {run['name'][:50]}...")
                    print(f"      Статус: {run['status']}, Пользователь: {run['user_id']}")
            
        else:
            print(f"❌ Ошибка получения данных: {response.status_code}")
            print(f"   Ответ: {response.text}")
            
    except Exception as e:
        print(f"❌ Ошибка запроса: {e}")

if __name__ == "__main__":
    test_api()
