# Аутентификация API

## Обзор

Stroppy Cloud Panel использует JWT (JSON Web Token) для аутентификации API запросов. Все защищенные endpoints требуют валидный токен в заголовке `Authorization`.

## Получение токена

### Регистрация пользователя

```http
POST /api/v1/auth/register
Content-Type: application/json

{
  "username": "testuser",
  "email": "test@example.com",
  "password": "securepassword123"
}
```

**Ответ:**
```json
{
  "user": {
    "id": "123e4567-e89b-12d3-a456-426614174000",
    "username": "testuser",
    "email": "test@example.com",
    "created_at": "2024-01-15T10:30:00Z"
  },
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

### Вход в систему

```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "username": "testuser",
  "password": "securepassword123"
}
```

**Ответ:**
```json
{
  "user": {
    "id": "123e4567-e89b-12d3-a456-426614174000",
    "username": "testuser",
    "email": "test@example.com",
    "created_at": "2024-01-15T10:30:00Z"
  },
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

## Использование токена

### Заголовок Authorization

```http
GET /api/v1/runs
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

### Пример с curl

```bash
curl -H "Authorization: Bearer YOUR_TOKEN_HERE" \
     -H "Content-Type: application/json" \
     https://api.stroppy.io/api/v1/runs
```

## Обновление токена

Токены имеют ограниченное время жизни. Для обновления используйте refresh token:

```http
POST /api/v1/auth/refresh
Content-Type: application/json

{
  "refresh_token": "your_refresh_token_here"
}
```

## Выход из системы

```http
POST /api/v1/auth/logout
Authorization: Bearer YOUR_TOKEN_HERE
```

## Обработка ошибок аутентификации

### 401 Unauthorized

```json
{
  "error": "unauthorized",
  "message": "Invalid or expired token",
  "code": 401
}
```

### 403 Forbidden

```json
{
  "error": "forbidden",
  "message": "Insufficient permissions",
  "code": 403
}
```

## Безопасность

### Рекомендации

1. **Храните токены безопасно** - не передавайте их в URL или логах
2. **Используйте HTTPS** - всегда используйте защищенное соединение
3. **Обновляйте токены** - используйте refresh tokens для продления сессии
4. **Ограничивайте время жизни** - устанавливайте разумное время жизни токенов

### Примеры небезопасного использования

❌ **Не делайте так:**
```bash
# Передача токена в URL
curl "https://api.stroppy.io/api/v1/runs?token=YOUR_TOKEN"

# Логирование токена
console.log("Token:", token);
```

✅ **Делайте так:**
```bash
# Использование заголовка Authorization
curl -H "Authorization: Bearer YOUR_TOKEN" \
     https://api.stroppy.io/api/v1/runs

# Безопасное логирование
console.log("Token received:", token ? "***" : "none");
```

## Интеграция с клиентскими приложениями

### JavaScript/TypeScript

```typescript
class StroppyAPI {
  private token: string | null = null;

  async login(username: string, password: string) {
    const response = await fetch('/api/v1/auth/login', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ username, password }),
    });

    const data = await response.json();
    this.token = data.token;
    return data;
  }

  async makeRequest(endpoint: string, options: RequestInit = {}) {
    if (!this.token) {
      throw new Error('Not authenticated');
    }

    return fetch(endpoint, {
      ...options,
      headers: {
        ...options.headers,
        'Authorization': `Bearer ${this.token}`,
      },
    });
  }
}
```

### Python

```python
import requests

class StroppyAPI:
    def __init__(self, base_url):
        self.base_url = base_url
        self.token = None

    def login(self, username, password):
        response = requests.post(
            f"{self.base_url}/api/v1/auth/login",
            json={"username": username, "password": password}
        )
        data = response.json()
        self.token = data["token"]
        return data

    def make_request(self, endpoint, method="GET", **kwargs):
        if not self.token:
            raise Exception("Not authenticated")
        
        headers = kwargs.get("headers", {})
        headers["Authorization"] = f"Bearer {self.token}"
        kwargs["headers"] = headers
        
        return requests.request(method, f"{self.base_url}{endpoint}", **kwargs)
```

### Go

```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
)

type StroppyAPI struct {
    BaseURL string
    Token   string
}

func (api *StroppyAPI) Login(username, password string) error {
    loginData := map[string]string{
        "username": username,
        "password": password,
    }
    
    jsonData, _ := json.Marshal(loginData)
    resp, err := http.Post(api.BaseURL+"/api/v1/auth/login", 
                          "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    api.Token = result["token"].(string)
    return nil
}

func (api *StroppyAPI) MakeRequest(endpoint, method string, body []byte) (*http.Response, error) {
    req, err := http.NewRequest(method, api.BaseURL+endpoint, bytes.NewBuffer(body))
    if err != nil {
        return nil, err
    }
    
    req.Header.Set("Authorization", "Bearer "+api.Token)
    req.Header.Set("Content-Type", "application/json")
    
    client := &http.Client{}
    return client.Do(req)
}
```

## Следующие шаги

- [Управление запусками](./runs.md)
- [Получение результатов](./results.md)
- [Webhook уведомления](./webhooks.md)
