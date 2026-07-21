package main

import (
	"context"

	"flag"
	"fmt"
	"net"
	"net/http"

	"os/signal"
	"syscall"
	"time"

	kitlog "github.com/dreamsxin/go-kit/v2/log"

	"google.golang.org/grpc"
)

func printBanner(logger *kitlog.Logger, httpAddr string, grpcAddr string, withOpenAPI bool) {
	logger.Sugar().Info("------------------------------------------------------------")
	logger.Sugar().Infof(" Service: UserService ")
	logger.Sugar().Infof(" HTTP: http://localhost%s", httpAddr)
	if withOpenAPI {
		logger.Sugar().Infof(" OpenAPI: http://localhost%s/openapi.json", httpAddr)
		logger.Sugar().Infof(" JSON Schema: http://localhost%s/schema.json", httpAddr)
		logger.Sugar().Infof(" API UI: http://localhost%s/swagger/index.html", httpAddr)
	}
	logger.Sugar().Infof(" gRPC: %s", grpcAddr)
	logger.Sugar().Info(" Press Ctrl+C to stop")
}

func printAllRoutes(logger *kitlog.Logger, routes []generatedRouteEntry) {
	logger.Sugar().Info("Registered Routes")
	for _, route := range routes {
		logger.Sugar().Infof("  %-7s %s", route.Method, route.Path)
	}
}

func main() {
	var (
		httpAddr = flag.String("http.addr", ":8080", "HTTP listen address")
		grpcAddr = flag.String("grpc.addr", ":8081", "gRPC listen address")
	)
	flag.Parse()

	logger, _ := kitlog.NewDevelopment()
	defer logger.Sync() //nolint:errcheck

	generated := initGeneratedServices(logger)
	runtime := generated.generatedRuntime()

	r := http.NewServeMux()
	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok","service":"UserService"}`)
	}
	r.HandleFunc("GET /health", healthHandler)
	r.HandleFunc("HEAD /health", healthHandler)

	runtime.registerRoutes(r)
	registerCustomRoutes(r)

	httpServer := &http.Server{
		Addr: *httpAddr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			r.ServeHTTP(w, req)
			logger.Sugar().Infof("[HTTP] %s %s %v", req.Method, req.URL.Path, time.Since(start))
		}),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
	}
	serverErr := make(chan error, 2)
	httpListener, err := net.Listen("tcp", *httpAddr)
	if err != nil {
		logger.Sugar().Fatalf("FATAL: HTTP listen: %v", err)
	}
	defer httpListener.Close() //nolint:errcheck

	lis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		logger.Sugar().Fatalf("FATAL: gRPC listen: %v", err)
	}
	defer lis.Close() //nolint:errcheck
	grpcServer := grpc.NewServer()
	runtime.registerGRPCServices(grpcServer)

	go func() {
		if err := httpServer.Serve(httpListener); err != nil && err != http.ErrServerClosed {
			serverErr <- fmt.Errorf("HTTP server: %w", err)
		}
	}()

	go func() {
		if err := grpcServer.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			serverErr <- fmt.Errorf("gRPC server: %w", err)
		}
	}()

	printBanner(logger, *httpAddr, *grpcAddr, false)

	runContext, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	select {
	case <-runContext.Done():
		logger.Sugar().Info("Received shutdown signal")
	case err := <-serverErr:
		logger.Sugar().Errorf("Server stopped unexpectedly: %v", err)
	}
	logger.Sugar().Info("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Sugar().Infof("HTTP shutdown error: %v", err)
	}
	grpcStopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(grpcStopped)
	}()
	select {
	case <-grpcStopped:
		logger.Sugar().Info("gRPC stopped")
	case <-ctx.Done():
		grpcServer.Stop()
		logger.Sugar().Infof("gRPC graceful stop timed out: %v", ctx.Err())
	}
	logger.Sugar().Info("Server exited cleanly")
}
