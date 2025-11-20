package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

const instrumentationName = "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgres/metrics"

func registerMetrics(pool *pgxpool.Pool) {
	meter := otel.GetMeterProvider().Meter(instrumentationName)
	acquireDuration, err := meter.Int64ObservableCounter(
		"acquire_duration",
		metric.WithDescription("The total duration of all successful acquires from the pool"),
		metric.WithUnit("milliseconds"),
	)
	if err != nil {
		panic(err)
	}
	acquiredCons, err := meter.Int64ObservableUpDownCounter(
		"acquired_cons",
		metric.WithDescription("The number of currently acquired connections in the pool"),
		metric.WithUnit("connections"),
	)
	if err != nil {
		panic(err)
	}

	canceledAcquireCount, err := meter.Int64ObservableUpDownCounter(
		"canceled_acquire_count",
		metric.WithDescription("The cumulative count of acquires from the pool that were canceled by a context"),
		metric.WithUnit("acquires"),
	)
	if err != nil {
		panic(err)
	}
	constructingCons, err := meter.Int64ObservableUpDownCounter(
		"constructing_cons",
		metric.WithDescription("The number of cons with construction in progress in the pool"),
		metric.WithUnit("connections"),
	)
	if err != nil {
		panic(err)
	}
	emptyAcquireCount, err := meter.Int64ObservableUpDownCounter(
		"empty_acquire_count",
		metric.WithDescription("The cumulative count of successful acquires from the pool that waited for a resource to be released or constructed because the pool was empty"),
		metric.WithUnit("acquires"),
	)
	if err != nil {
		panic(err)
	}
	idleCons, err := meter.Int64ObservableUpDownCounter(
		"idle_cons",
		metric.WithDescription("The number of currently idle conns in the pool"),
		metric.WithUnit("connections"),
	)
	if err != nil {
		panic(err)
	}
	maxCons, err := meter.Int64ObservableCounter(
		"max_cons",
		metric.WithDescription("The maximum size of the pool"),
		metric.WithUnit("connections"),
	)
	if err != nil {
		panic(err)
	}
	totalCons, err := meter.Int64ObservableCounter(
		"total_conns",
		metric.WithDescription("The total number of resources currently in the pool"),
		metric.WithUnit("connections"),
	)
	if err != nil {
		panic(err)
	}
	newConsCount, err := meter.Int64ObservableUpDownCounter(
		"new_cons_count",
		metric.WithDescription("The cumulative count of new connections created by the pool"),
		metric.WithUnit("connections"),
	)
	if err != nil {
		panic(err)
	}
	maxLifetimeDestroyCount, err := meter.Int64ObservableCounter(
		"max_lifetime_destroy_count",
		metric.WithDescription("The cumulative count of connections destroyed because they exceeded MaxConnLifetime"),
		metric.WithUnit("connections"),
	)
	if err != nil {
		panic(err)
	}
	maxIdleDestroyCount, err := meter.Int64ObservableCounter(
		"max_idle_destroy_count",
		metric.WithDescription("The cumulative count of connections destroyed because they exceeded MaxConnIdleTime"),
		metric.WithUnit("connections"),
	)
	if err != nil {
		panic(err)
	}
	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		observer.ObserveInt64(acquireDuration, pool.Stat().AcquireDuration().Milliseconds())
		observer.ObserveInt64(acquiredCons, int64(pool.Stat().AcquiredConns()))
		observer.ObserveInt64(canceledAcquireCount, pool.Stat().CanceledAcquireCount())
		observer.ObserveInt64(constructingCons, int64(pool.Stat().ConstructingConns()))
		observer.ObserveInt64(emptyAcquireCount, pool.Stat().EmptyAcquireCount())
		observer.ObserveInt64(idleCons, int64(pool.Stat().IdleConns()))
		observer.ObserveInt64(maxCons, int64(pool.Stat().MaxConns()))
		observer.ObserveInt64(totalCons, int64(pool.Stat().TotalConns()))
		observer.ObserveInt64(newConsCount, pool.Stat().NewConnsCount())
		observer.ObserveInt64(maxLifetimeDestroyCount, pool.Stat().MaxLifetimeDestroyCount())
		observer.ObserveInt64(maxIdleDestroyCount, pool.Stat().MaxIdleDestroyCount())
		return nil
	})
	if err != nil {
		panic(err)
	}
}
