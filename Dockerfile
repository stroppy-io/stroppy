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

# Статическая сборка без CGO - зависимости не нужны

# Установка рабочей директории
WORKDIR /app/backend

# Копирование go.mod и go.sum для кеширования зависимостей
COPY backend/go.mod backend/go.sum ./

# Загрузка зависимостей
RUN go mod download

# Копирование исходного кода backend
COPY backend/ ./

# Сборка backend приложения
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bin/stroppy-cloud-panel ./cmd/stroppy-cloud-panel

# Этап 3: Финальный образ
FROM gcr.io/distroless/static-debian12:nonroot

# Установка рабочей директории
WORKDIR /app

# Копирование бинарного файла backend из builder образа
COPY --from=backend-builder /app/backend/bin/stroppy-cloud-panel ./stroppy-cloud-panel

# Копирование собранного frontend из builder образа
COPY --from=frontend-builder /app/frontend/dist ./web

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

# Команда запуска
CMD ["/app/stroppy-cloud-panel"]
