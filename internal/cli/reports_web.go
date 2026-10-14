package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/stroppy-io/stroppy/v6/pkg/report"
	"github.com/stroppy-io/stroppy/v6/pkg/report/view"
)

const reportServerTimeout = 5 * time.Second

var errReportExportSource = errors.New("HTML output must not replace its source report")

func newReportsExportCommand() *cobra.Command {
	var path string

	command := &cobra.Command{
		Use: "export ID", Short: "Export a self-contained HTML report", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			history, err := report.DefaultHistory()
			if err != nil {
				return err
			}

			data, err := history.Get(args[0])
			if err != nil {
				return err
			}

			if path != "" {
				if err := checkReportExport(path, history, args[0], data); err != nil {
					return err
				}
			}

			if path != "" {
				return view.WriteCaptureFile(path, data, args[0], history.Directory)
			}

			return view.RenderCapture(cmd.OutOrStdout(), data, args[0], history.Directory, nil)
		},
	}
	command.Flags().StringVar(&path, "file", "", "HTML output path; omit to write stdout")

	return command
}

func newReportsServeCommand() *cobra.Command {
	var address string

	command := &cobra.Command{
		Use: "serve", Short: "Browse saved reports in a local read-only web viewer", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			history, err := report.DefaultHistory()
			if err != nil {
				return err
			}

			listener, err := (&net.ListenConfig{}).Listen(cmd.Context(), "tcp", address)
			if err != nil {
				return err
			}
			defer listener.Close()

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Report viewer: http://%s/\n", listener.Addr()); err != nil {
				return err
			}

			return serveReports(cmd.Context(), listener, view.Handler(history))
		},
	}
	command.Flags().StringVar(&address, "listen", "127.0.0.1:0", "listen address; default loopback and available port")

	return command
}

func serveReports(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: reportServerTimeout}

	stopped := make(chan struct{})
	shutdown := make(chan error, 1)

	go func() {
		select {
		case <-ctx.Done():
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), reportServerTimeout)
			defer cancel()

			if err := server.Shutdown(cleanup); err != nil {
				shutdown <- errors.Join(err, server.Close())
			} else {
				shutdown <- nil
			}
		case <-stopped:
			shutdown <- nil
		}
	}()

	err := server.Serve(listener)

	close(stopped)

	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}

	return errors.Join(err, <-shutdown)
}
