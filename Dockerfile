FROM golang:1.25-alpine3.22 AS builder

ARG VERSION=0.0.0

RUN apk add --no-cache make curl git

WORKDIR /app

# Copy dependency files first for better caching
COPY go.mod go.sum ./
COPY cmd/xk6air/go.mod cmd/xk6air/go.sum ./cmd/xk6air/
COPY Makefile ./

RUN go mod download

RUN make .install-xk6

# Copy source code
COPY . .

RUN VERSION=${VERSION} make build

FROM alpine:3.22 AS runner

# Install runtime dependencies if needed
RUN apk add --no-cache ca-certificates

WORKDIR /workspace

# Copy the binary
COPY --from=builder /app/build/stroppy /usr/local/bin/stroppy

# Copy workloads to workspace for easy access
COPY --from=builder /app/workloads /workloads

# Set stroppy as entrypoint for flexibility
ENTRYPOINT ["/usr/local/bin/stroppy"]

# Default to showing help
CMD ["--help"]
