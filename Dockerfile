# syntax=docker/dockerfile:1.7

ARG NODE_VERSION=20-alpine
ARG GO_VERSION=1.24
ARG VERSION=dev

FROM node:${NODE_VERSION} AS frontend-builder
WORKDIR /src/web
COPY web/package.json web/yarn.lock ./
COPY web ./
RUN corepack enable && yarn install --immutable
RUN yarn build

FROM golang:${GO_VERSION} AS backend-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X github.com/stroppy-io/stroppy-cloud-panel/internal/core/build.Version=${VERSION} -X github.com/stroppy-io/stroppy-cloud-panel/internal/core/build.ServiceName=stroppy-cloud-panel" \
    -o /out/stroppy-cloud-panel ./cmd

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=backend-builder /out/stroppy-cloud-panel ./stroppy-cloud-panel
COPY --from=backend-builder /src/config.yaml .
COPY --from=frontend-builder /src/web/dist ./frontend
ENV SERVICE_SERVER_STATIC_DIR=/app/frontend
EXPOSE 8080
CMD ["./stroppy-cloud-panel"]
