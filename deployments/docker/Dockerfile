# Многоэтапная сборка для полноценного контейнера
# Этап 1: Сборка frontend
FROM node:20-alpine AS frontend-builder

# Установка рабочей директории
WORKDIR /app/web

# Копирование package.json и yarn.lock для установки зависимостей
COPY web/package.json web/yarn.lock ./
RUN yarn install --frozen-lockfile

# Копирование исходного кода frontend
COPY web/ ./

# Установка переменных окружения для frontend
ARG VITE_API_BASE_URL=/api/v1
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL

# Сборка frontend для продакшена
RUN yarn build

# Этап 2: Сборка backend
FROM golang:1.22 AS backend-builder

# Установка рабочей директории
WORKDIR /app

# Копирование go.mod и go.sum для кеширования зависимостей
COPY go.mod go.sum ./

# Загрузка зависимостей
RUN go mod download

# Копирование исходного кода backend
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg

# Сборка backend приложения
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bin/stroppy-cloud-panel ./cmd/stroppy-cloud-panel

# Этап 3: Финальный образ
FROM gcr.io/distroless/static-debian12:nonroot

# Установка рабочей директории
WORKDIR /app

# Копирование бинарного файла backend из builder образа
COPY --from=backend-builder /app/bin/stroppy-cloud-panel ./stroppy-cloud-panel

# Копирование собранного frontend из builder образа
COPY --from=frontend-builder /app/web/dist ./web

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
