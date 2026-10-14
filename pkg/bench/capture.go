package bench

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/stroppy-io/stroppy/v6/internal/capture"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

// CaptureOptions records optional streams into a caller-selected persistent root.
type CaptureOptions struct {
	Directory string
	Logs      bool
	Metrics   bool
	Interval  time.Duration
	MaxBytes  int64
}

const captureCollectionTimeout = 5 * time.Second

var errCaptureNoReport = errors.New("capture requires report construction")

type runCapture struct {
	writer   *capture.Writer
	root     *rootState
	interval time.Duration
	metrics  bool
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
	sequence uint64
}

func (c *runCapture) start() {
	c.writer.Event("run_start", "", "", 0, 0, 0)

	if !c.metrics {
		close(c.done)

		return
	}
	go func() {
		defer close(c.done)

		c.sample()

		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		previous := time.Now()

		for {
			select {
			case <-c.stop:
				return
			case now := <-ticker.C:
				missed := int(now.Sub(previous)/c.interval) - 1
				if missed > 0 {
					c.writer.Missed(uint64(missed))
					c.sequence += uint64(missed)
				}

				previous = now

				c.sample()
			}
		}
	}()
}

func (c *runCapture) sample() {
	if !c.writer.Enabled("metrics") {
		return
	}

	c.sequence++
	started := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), min(c.interval, captureCollectionTimeout))
	defer cancel()

	var data metricdata.ResourceMetrics
	if err := c.root.manualReader.Collect(ctx, &data); err != nil {
		c.writer.Missed(1)
		c.writer.Event("collection_error", "", err.Error(), 0, 0, 0)

		return
	}

	mergeMetricWriters(&data)
	c.writer.Add("metrics", report.MetricSample{
		Sequence: c.sequence, Time: started.UTC(),
		CaptureID: c.writer.ID(), ReportID: c.writer.ReportID(),
		ElapsedSeconds: c.writer.Elapsed(started), CollectionSeconds: time.Since(started).Seconds(),
		Semantics: "cumulative counters and histograms; gauges as recorded; native rates are cumulative averages",
		Metrics:   reportMetrics(data, c.root.metricsPrefix),
	})
}

func (c *runCapture) stopSampling() {
	c.once.Do(func() { close(c.stop) })
	<-c.done
}

func (c *runCapture) finish(result *report.Run) {
	c.stopSampling()

	if c.metrics {
		c.sample()
	}

	c.writer.Event("run_end", "", string(result.Status), 0, 0, 0)
	result.Capture = c.writer.Close()
}
