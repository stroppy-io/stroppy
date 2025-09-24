# Многоэтапная сборка для полноценного контейнера
# Этап 1: Сборка frontend
FROM node:20-alpine AS frontend-builder

# Установка рабочей директории
WORKDIR /app/frontend

# Копирование package.json и yarn.lock для установки зависимостей
COPY frontend/package.json frontend/yarn.lock ./
RUN yarn install --frozen-lockfile

# Копирование исходного кода frontend
COPY frontend/ ./

# Сборка frontend для продакшена
RUN yarn build

# Этап 2: Сборка backend
FROM golang:1.22 AS backend-builder

# Установка зависимостей для сборки
RUN apt-get update && apt-get install -y \
    gcc \
    libc6-dev \
    && rm -rf /var/lib/apt/lists/*

# Установка рабочей директории
WORKDIR /app/backend

# Копирование go.mod и go.sum для кеширования зависимостей
COPY backend/go.mod backend/go.sum ./

# Загрузка зависимостей
RUN go mod download

# Копирование исходного кода backend
COPY backend/ ./

# Сборка backend приложения
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o bin/stroppy-cloud-pannel ./cmd/stroppy-cloud-pannel

# Этап 3: Финальный образ
FROM ubuntu:22.04

# Установка зависимостей времени выполнения
RUN apt-get update && apt-get install -y ca-certificates tzdata wget && rm -rf /var/lib/apt/lists/*

# Установка временной зоны
RUN ln -fs /usr/share/zoneinfo/Europe/Moscow /etc/localtime && \
    dpkg-reconfigure -f noninteractive tzdata

# Создание пользователя для безопасности
RUN useradd -r -s /bin/false appuser

# Установка рабочей директории
WORKDIR /app

# Копирование бинарного файла backend из builder образа
COPY --from=backend-builder /app/backend/bin/stroppy-cloud-pannel ./stroppy-cloud-pannel

# Копирование собранного frontend из builder образа
COPY --from=frontend-builder /app/frontend/dist ./web

# Переключение на непривилегированного пользователя
USER appuser

# Настройка переменных окружения
ENV DB_HOST=postgres
ENV DB_PORT=5432
ENV DB_USER=stroppy
ENV DB_PASSWORD=stroppy
ENV DB_NAME=stroppy
ENV DB_SSLMODE=disable
ENV STATIC_DIR=/app/web
ENV PORT=8080
ENV JWT_SECRET=change-this-in-production-please-use-strong-secret
ENV GIN_MODE=release

# Открытие порта
EXPOSE 8080

# Проверка здоровья контейнера
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Команда запуска
CMD ["./stroppy-cloud-pannel"]
