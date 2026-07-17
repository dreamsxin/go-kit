package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	kitlog "github.com/dreamsxin/go-kit/v2/log"

	"example.com/gen_idl_extend_append/skill"
)

func printBanner(logger *kitlog.Logger, httpAddr string, withOpenAPI bool, withSkill bool) {
	logger.Sugar().Info("------------------------------------------------------------")
	logger.Sugar().Infof(" Service: UserService ")
	logger.Sugar().Infof(" HTTP: http://localhost%s", httpAddr)
	if withOpenAPI {
		logger.Sugar().Infof(" OpenAPI: http://localhost%s/openapi.json", httpAddr)
		logger.Sugar().Infof(" JSON Schema: http://localhost%s/schema.json", httpAddr)
		logger.Sugar().Infof(" API UI: http://localhost%s/swagger/index.html", httpAddr)
	}
	if withSkill {
		logger.Sugar().Infof(" Skill: http://localhost%s/skill", httpAddr)
	}
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

	r.HandleFunc("GET /skill", skill.Handler)

	runtime.registerRoutes(r)
	customRoutes := registerCustomRoutes(r)

	r.HandleFunc("GET /debug/routes", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(generatedRouteEntries(runtime, customRoutes, false, true))
	})

	allRoutes := generatedRouteEntries(runtime, customRoutes, false, true)
	printAllRoutes(logger, allRoutes)

	httpServer := &http.Server{
		Addr: *httpAddr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			r.ServeHTTP(w, req)
			logger.Sugar().Infof("[HTTP] %s %s %v", req.Method, req.URL.Path, time.Since(start))
		}),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Sugar().Fatalf("FATAL: HTTP server: %v", err)
		}
	}()

	printBanner(logger, *httpAddr, false, true)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Sugar().Info("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Sugar().Infof("HTTP shutdown error: %v", err)
	}
	logger.Sugar().Info("Server exited cleanly")
}
