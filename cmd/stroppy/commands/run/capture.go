package run

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/report"
)

func (o *reportOutput) cleanupCapture(output io.Writer, run *report.Run, retained bool) {
	if !retained || o.noSave || o.requested() || o.captureDirectory != "" || run.Capture == nil {
		return
	}

	if err := report.RemoveCapture(".", run.Capture); err != nil {
		fmt.Fprintf(output, "warning: remove copied capture staging: %v\n", err)
	}
}

var errCaptureFlags = errors.New("invalid capture options")

func parseCaptureFlags(args []string, i int, parsed *runArgs) (int, error) {
	switch args[i] {
	case "--capture-logs":
		parsed.report.captureLogs = true

		return 1, nil
	case "--capture-metrics":
		parsed.report.captureMetrics = true

		return 1, nil
	}

	for _, flag := range []string{"--capture-dir", "--capture-interval", "--capture-max-bytes"} {
		value, equals := strings.CutPrefix(args[i], flag+"=")
		consumed := 1

		if args[i] == flag {
			var err error

			value, err = nextFlagValue(args, i)
			if err != nil {
				return 0, err
			}

			consumed = consumedPairFlag
		} else if !equals {
			continue
		}

		if value == "" {
			return 0, fmt.Errorf("%w: %s requires value", errCaptureFlags, flag)
		}

		switch flag {
		case "--capture-dir":
			parsed.report.captureDirectory = value
		case "--capture-interval":
			duration, err := time.ParseDuration(value)
			if err != nil || duration <= 0 {
				return 0, fmt.Errorf("%w: interval must be positive", errCaptureFlags)
			}

			parsed.report.captureInterval = duration
		case "--capture-max-bytes":
			size, err := strconv.ParseInt(value, 10, 64)
			if err != nil || size < 256<<10 {
				return 0, fmt.Errorf("%w: byte budget must be at least 262144", errCaptureFlags)
			}

			parsed.report.captureMaxBytes = size
		}

		return consumed, nil
	}

	return 0, nil
}

//nolint:nilnil // nil options explicitly mean recording disabled.
func (o *reportOutput) captureOptions() (*bench.CaptureOptions, error) {
	if !o.captureLogs && !o.captureMetrics {
		if o.captureDirectory != "" || o.captureInterval != 0 || o.captureMaxBytes != 0 {
			return nil, errCaptureFlags
		}

		return nil, nil
	}

	if o.disabled {
		return nil, errCaptureFlags
	}

	directory := o.captureDirectory
	if directory == "" {
		if o.noSave && o.file == "" && o.html == "" {
			return nil, fmt.Errorf("%w: no-save capture needs --capture-dir, --report-file, or --report-html", errCaptureFlags)
		}

		switch {
		case o.file != "":
			directory = filepath.Join(filepath.Dir(o.file), "captures")
		case o.html != "":
			directory = filepath.Join(filepath.Dir(o.html), "captures")
		default:
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}

			directory = filepath.Join(home, ".stroppy", "reports", "captures")
		}
	}

	interval := o.captureInterval
	if interval == 0 {
		interval = time.Second
	}

	return &bench.CaptureOptions{
		Directory: directory, Logs: o.captureLogs, Metrics: o.captureMetrics,
		Interval: interval, MaxBytes: o.captureMaxBytes,
	}, nil
}
