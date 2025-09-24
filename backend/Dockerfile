# Многоэтапная сборка для минимизации размера образа
FROM golang:1.22 AS builder

# Установка зависимостей для сборки
RUN apt-get update && apt-get install -y gcc libc6-dev && rm -rf /var/lib/apt/lists/*

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
RUN go build -ldflags="-s -w" -o bin/stroppy-cloud-pannel ./cmd/stroppy-cloud-pannel

# Финальный образ
FROM ubuntu:22.04

# Установка зависимостей времени выполнения
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Создание пользователя для безопасности
RUN useradd -r -s /bin/false appuser

# Установка рабочей директории
WORKDIR /app

# Копирование бинарного файла из builder образа
COPY --from=builder /app/bin/stroppy-cloud-pannel .

# Переключение на непривилегированного пользователя
USER appuser

# Настройка переменных окружения
ENV DB_HOST=postgres
ENV DB_PORT=5432
ENV DB_USER=stroppy
ENV DB_PASSWORD=stroppy
ENV DB_NAME=stroppy
ENV DB_SSLMODE=disable
ENV PORT=8080
ENV JWT_SECRET=change-this-in-production

# Открытие порта
EXPOSE 8080

# Команда запуска
CMD ["./stroppy-cloud-pannel"]
