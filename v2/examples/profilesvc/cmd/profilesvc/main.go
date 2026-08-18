package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dreamsxin/go-kit-examples/v2/profilesvc"
)

func main() {
	var (
		httpAddr = flag.String("http.addr", ":8080", "HTTP listen address")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	var s profilesvc.Service
	s = profilesvc.LoggingMiddleware(logger)(profilesvc.NewInmemService())

	h := profilesvc.MakeHTTPHandler(s, logger.With("component", "HTTP"))

	srv := &http.Server{Addr: *httpAddr, Handler: h}

	errs := make(chan error, 1)
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errs <- fmt.Errorf("%s", <-c)
	}()

	go func() {
		logger.Info("transport started", "transport", "HTTP", "address", *httpAddr)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errs <- err
		}
	}()

	reason := <-errs
	logger.Info("exit", "reason", reason)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}
