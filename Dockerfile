FROM golang:1.25-alpine3.22 AS builder

RUN apk add --no-cache make curl git

WORKDIR /app

COPY go.mod go.sum ./
COPY cmd/xk6/go.mod cmd/xk6/go.sum ./cmd/xk6/
COPY cmd/xk6air/go.mod cmd/xk6air/go.sum ./cmd/xk6air/
COPY Makefile ./

RUN go mod download

RUN make .install-xk6

COPY . .

RUN make build-k6

FROM alpine:3.22 AS runner

COPY --from=builder /app/build/stroppy-k6 /k6
COPY --from=builder /app/internal/static/stroppy.pb.js /stroppy.pb.js

CMD [ "/k6", "run", "test.ts", "--out", "experimental-opentelemetry" ]
