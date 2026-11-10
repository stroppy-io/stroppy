package tpch

import (
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
)

var (
	errStreamPolicy = errors.New("TPC-H streams require one pass per stream using shared-iterations")
	errStreamCount  = errors.New("query-stream selects one stream and cannot be combined with streams greater than 1")
	errStreamSeed   = errors.New("query-seed plus stream ID must be less than 2147483647")
	errStreamQuery  = errors.New("TPC-H generated stream needs all 22 query bodies")
	errStreamDriver = errors.New("TPC-H generated streams require retained sessions: " +
		"use postgres, crdb, yugabyte, mysql or tidb")
)

type streamSelection struct {
	enabled bool
	count   int
	id      int
	seed    int64
	single  bool
}

func selectStreams(d *bench.Def) (streamSelection, bench.RunSettings, error) {
	count, countInfo := d.Param.Int("streams", 1,
		"Query streams, each executed once on its own session.", bench.Min(1))
	id, streamInfo := d.Param.Int("query-stream", 0,
		"One generated stream; 0 uses the Appendix A power order.", bench.Min(0))
	seed, seedInfo := d.Param.Int64("query-seed", 19620718,
		"Base query seed; stream s uses seed+s.", bench.Min(int64(1)), bench.Max(querySeedModulus-1))
	selection := streamSelection{
		enabled: countInfo.Explicit() || streamInfo.Explicit() || seedInfo.Explicit(),
		count:   count, id: id, seed: seed, single: streamInfo.Explicit(),
	}

	defaults := bench.RunDefaults{}
	if selection.enabled {
		defaults.Workers = count
		defaults.Iterations = int64(count)
	}

	settings := bench.RunParameters(&d.Param, defaults)

	return selection, settings, selection.validate(settings)
}

func (s streamSelection) validate(settings bench.RunSettings) error {
	if !s.enabled {
		return nil
	}

	if settings.Executor != "shared-iterations" || settings.Workers != s.count || settings.Iterations != int64(s.count) {
		return fmt.Errorf("%w: vus=%d, iterations=%d", errStreamPolicy, s.count, s.count)
	}

	if s.single && s.count != 1 {
		return errStreamCount
	}

	last := s.count
	if s.single {
		last = s.id
	}

	if int64(last) >= querySeedModulus-s.seed {
		return errStreamSeed
	}

	return nil
}

func streamDriverSupported(kind bench.DriverTypeName) bool {
	switch kind {
	case bench.DriverPostgres, bench.DriverCRDB, bench.DriverYugabyte,
		bench.DriverMySQL, bench.DriverTiDB, bench.DriverRecording:
		return true
	default:
		return false
	}
}

func (w *workload) prepareStreams(s streamSelection) error {
	if !s.enabled {
		return nil
	}

	if !streamDriverSupported(w.driverType) {
		return errStreamDriver
	}

	for _, name := range queryNames {
		if _, ok := w.sql.Lookup(name, "body"); !ok {
			return fmt.Errorf("%w: missing %s/body", errStreamQuery, name)
		}
	}

	w.streams = make([]queryStream, s.count)
	for i := range w.streams {
		id := i + 1
		if s.single {
			id = s.id
		}

		params, err := streamParams(w.scaleFactor, s.seed+int64(id))
		if err != nil {
			return err
		}

		w.streams[i] = queryStream{
			ID: id, Seed: s.seed + int64(id), Order: streamOrders[id%len(streamOrders)],
			Parameters: w.prepareParams(params),
		}
	}

	return nil
}
