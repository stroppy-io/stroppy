# Многоэтапная сборка для минимизации размера образа
FROM golang:1.22 AS builder

# Установка зависимостей для сборки
RUN apt-get update && apt-get install -y gcc libc6-dev libsqlite3-dev && rm -rf /var/lib/apt/lists/*

# Установка рабочей директории
WORKDIR /app

# Копирование go.mod и go.sum для кеширования зависимостей
COPY go.mod go.sum ./

# Загрузка зависимостей
RUN go mod download

# Копирование исходного кода
COPY . .

# Принудительное обновление кэша
ARG CACHE_BUST=1

# Сборка приложения
ENV CGO_ENABLED=1
ENV GOOS=linux
RUN go build -tags "sqlite_omit_load_extension" -o bin/stroppy-cloud-pannel ./cmd/stroppy-cloud-pannel

# Финальный образ
FROM ubuntu:22.04

# Установка зависимостей времени выполнения
RUN apt-get update && apt-get install -y ca-certificates sqlite3 && rm -rf /var/lib/apt/lists/*

# Создание пользователя для безопасности
RUN useradd -r -s /bin/false appuser

# Установка рабочей директории
WORKDIR /app

# Копирование бинарного файла из builder образа
COPY --from=builder /app/bin/stroppy-cloud-pannel .

# Создание директории для базы данных
RUN mkdir -p /app/data && chown appuser:appuser /app/data

# Переключение на непривилегированного пользователя
USER appuser

# Настройка переменных окружения
ENV DB_PATH=/app/data/stroppy.db
ENV PORT=8080
ENV JWT_SECRET=change-this-in-production

# Открытие порта
EXPOSE 8080

# Команда запуска
CMD ["./stroppy-cloud-pannel"]
