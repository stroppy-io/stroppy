FROM golang AS base

ARG VERSION
ARG GITHUB_USER=komeet
ARG GITHUB_TOKEN

WORKDIR /app

COPY docker .

RUN echo "machine github.com login $GITHUB_USER password $GITHUB_TOKEN" >> ~/.netrc

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOPRIVATE="github.com/stroppy-io" go build  \
    -ldflags="-X github.com/stroppy-io/internal/core/build.Version=$VERSION -X github.com/stroppy-io/internal/core/build.ServiceName=stroppy-cloud-panel" \
     -v -o stroppy-cloud-panel ./cmd

FROM gcr.io/distroless/static-debian11

COPY --from=base /app/config.yaml .
COPY --from=base /app/komeet-server .

CMD ["./komeet-server"]