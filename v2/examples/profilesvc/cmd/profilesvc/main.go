package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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

	errs := make(chan error)
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errs <- fmt.Errorf("%s", <-c)
	}()

	go func() {
		logger.Info("transport started", "transport", "HTTP", "address", *httpAddr)

		errs <- http.ListenAndServe(*httpAddr, h)
	}()

	logger.Info("exit", "reason", <-errs)
}
