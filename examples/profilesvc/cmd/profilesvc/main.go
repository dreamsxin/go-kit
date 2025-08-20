package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/dreamsxin/go-kit/examples/profilesvc"
	"github.com/dreamsxin/go-kit/log"
	"go.uber.org/zap"
)

func main() {
	var (
		httpAddr = flag.String("http.addr", ":8080", "HTTP listen address")
	)
	flag.Parse()

	logger, err := log.NewDevelopment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Log error: %v\n", err)
		os.Exit(1)
	}

	var s profilesvc.Service
	{
		s = profilesvc.NewInmemService()
		s = profilesvc.LoggingMiddleware(logger)(s)
	}

	var h http.Handler
	{
		h = profilesvc.MakeHTTPHandler(s, logger.With(zap.String("component", "HTTP")))

	}

	errs := make(chan error)
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errs <- fmt.Errorf("%s", <-c)
	}()

	go func() {
		logger.Sugar().Info("transport", "HTTP", "addr", *httpAddr)

		errs <- http.ListenAndServe(*httpAddr, h)
	}()

	logger.Sugar().Info("exit", <-errs)
}
