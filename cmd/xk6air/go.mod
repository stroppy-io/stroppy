module xk6air

go 1.26

replace github.com/stroppy-io/stroppy => ../../

require (
	github.com/oklog/ulid/v2 v2.1.1
	github.com/orcaman/concurrent-map/v2 v2.0.1
	github.com/stroppy-io/stroppy v1.0.2 // TODO: change to proper version v2.x.x and update packages to .../stroppy/v2...
	go.k6.io/k6 v1.8.0
	go.uber.org/zap v1.28.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/google/uuid v1.6.0
	github.com/grafana/sobek v0.0.0-20260429085637-a66d4790012b
	github.com/shopspring/decimal v1.4.0
)

require (
	buf.build/gen/go/gogo/protobuf/protocolbuffers/go v1.36.11-20240617172848-e1dbca2775a7.1 // indirect
	buf.build/gen/go/prometheus/prometheus/protocolbuffers/go v1.36.11-20260331160422-eae785f0a21d.1 // indirect
	connectrpc.com/connect v1.20.0 // indirect
	dario.cat/mergo v1.0.1 // indirect
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/Azure/go-ntlmssp v0.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/PuerkitoBio/goquery v1.12.0 // indirect
	github.com/Soontao/goHttpDigestClient v0.0.0-20170320082612-6d28bb1415c5 // indirect
	github.com/andybalholm/brotli v1.2.1 // indirect
	github.com/andybalholm/cascadia v1.3.4 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bufbuild/protocompile v0.14.1 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chromedp/cdproto v0.0.0-20250803210736-d308e07a266d // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/docker/docker v28.3.3+incompatible // indirect
	github.com/envoyproxy/protoc-gen-validate v1.3.3 // indirect
	github.com/evanw/esbuild v0.28.1 // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/go-sql-driver/mysql v1.10.0 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/pprof v0.0.0-20260604005048-7023385849c0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/grafana/k6-cloud-openapi-client-go v0.0.2 // indirect
	github.com/grafana/k6provider v0.5.0 // indirect
	github.com/grafana/xk6-dashboard v0.8.1 // indirect
	github.com/grafana/xk6-dashboard-assets v0.1.2 // indirect
	github.com/grafana/xk6-redis v0.3.6 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/influxdata/influxdb1-client v0.0.0-20220302092344-a9ab5670611c // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx-shopspring-decimal v0.0.0-20220624020537-1d36b5a1853e // indirect
	github.com/jackc/pgx/v5 v5.10.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jhump/protoreflect v1.18.0 // indirect
	github.com/jhump/protoreflect/v2 v2.0.0-beta.2 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/magiconair/properties v1.8.10 // indirect
	github.com/mailru/easyjson v0.9.2 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/moby/go-archive v0.1.0 // indirect
	github.com/mstoykov/atlas v0.0.0-20220811071828-388f114305dd // indirect
	github.com/mstoykov/envconfig v1.5.0 // indirect
	github.com/mstoykov/k6-taskqueue-lib v0.1.3 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/picodata/picodata-go v1.1.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.69.0 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/r3labs/sse/v2 v2.10.0 // indirect
	github.com/redis/go-redis/v9 v9.17.2 // indirect
	github.com/serenize/snaker v0.0.0-20201027110005-a7ad2135616e // indirect
	github.com/shirou/gopsutil/v4 v4.25.5 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/tidwall/gjson v1.19.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tklauser/go-sysconf v0.3.13 // indirect
	github.com/ydb-platform/ydb-go-genproto v0.0.0-20260428144813-1c07baab7f7b // indirect
	github.com/ydb-platform/ydb-go-sdk/v3 v3.141.2 // indirect
	github.com/ydb-platform/ydb-go-yc-metadata v0.6.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/crypto/x509roots/fallback v0.0.0-20260626155920-5b7f84159940 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/term v0.44.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260622175928-b703f567277d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260622175928-b703f567277d // indirect
	google.golang.org/grpc v1.81.1 // indirect
	gopkg.in/cenkalti/backoff.v1 v1.1.0 // indirect
	gopkg.in/guregu/null.v3 v3.5.0 // indirect
	gopkg.in/validator.v2 v2.0.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
