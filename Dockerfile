FROM golang:1.25-alpine3.22 AS builder

RUN apk add --no-cache make curl git

WORKDIR /app

COPY go.mod go.sum ./
COPY cmd/xk6air/go.mod cmd/xk6air/go.sum ./cmd/xk6air/
COPY Makefile ./

RUN go mod download

RUN make .install-xk6

COPY . .

RUN make build-all

FROM alpine:3.22 AS runner

COPY --from=builder /app/build/stroppy /stroppy

CMD [ "/stroppy", "run", "test.ts" ]
