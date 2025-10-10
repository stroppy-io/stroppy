# Установка и настройка

## Системные требования

- **Операционная система**: Linux, macOS, Windows
- **Память**: минимум 4GB RAM
- **Дисковое пространство**: минимум 2GB свободного места
- **Сеть**: доступ к интернету для загрузки зависимостей

## Установка Stroppy Cloud Panel

### 1. Скачивание

```bash
# Скачайте последнюю версию
curl -L https://github.com/stroppy-io/stroppy-cloud-panel/releases/latest/download/stroppy-cloud-panel.tar.gz | tar -xz

# Или клонируйте репозиторий
git clone https://github.com/stroppy-io/stroppy-cloud-panel.git
cd stroppy-cloud-panel
```

### 2. Установка зависимостей

```bash
# Установка Node.js (для frontend)
curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
sudo apt-get install -y nodejs

# Установка Go (для backend)
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

### 3. Сборка проекта

```bash
# Сборка frontend
cd frontend
npm install
npm run build

# Сборка backend
cd ../backend
go mod download
go build -o stroppy-cloud-panel ./cmd/stroppy-cloud-panel
```

### 4. Запуск

```bash
# Запуск backend
./stroppy-cloud-panel

# Запуск frontend (в отдельном терминале)
cd frontend
npm run dev
```

## Первоначальная настройка

После установки откройте браузер и перейдите по адресу `http://localhost:3000` для доступа к веб-интерфейсу.

### Создание первого пользователя

1. Нажмите "Регистрация"
2. Заполните форму регистрации
3. Подтвердите email (если настроена почта)
4. Войдите в систему

## Следующие шаги

- [Создание первой конфигурации](../configuration/basic-config.md)
- [Запуск первого теста](../getting-started/first-test.md)
- [Анализ результатов](../getting-started/analyzing-results.md)
