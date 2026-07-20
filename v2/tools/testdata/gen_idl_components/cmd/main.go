package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	kitlog "github.com/dreamsxin/go-kit/v2/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"example.com/gen_idl_components/config"
	docs "example.com/gen_idl_components/docs"
	"example.com/gen_idl_components/skill"
	swaggerUI "github.com/swaggest/swgui/v5"
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

func newConfiguredLogger(cfg config.LoggingConfig) (*kitlog.Logger, error) {
	encoding := strings.ToLower(strings.TrimSpace(cfg.Format))
	if encoding == "" {
		encoding = "json"
	}
	if encoding != "json" && encoding != "console" {
		return nil, fmt.Errorf("unsupported logging format %q", cfg.Format)
	}

	levelText := strings.ToLower(strings.TrimSpace(cfg.Level))
	if levelText == "" {
		levelText = "info"
	}
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(levelText)); err != nil {
		return nil, fmt.Errorf("unsupported logging level %q: %w", cfg.Level, err)
	}

	zapConfig := zap.NewProductionConfig()
	if encoding == "console" {
		zapConfig = zap.NewDevelopmentConfig()
	}
	zapConfig.Encoding = encoding
	zapConfig.Level = zap.NewAtomicLevelAt(level)
	return zapConfig.Build()
}

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to config file")
	flag.CommandLine.Parse(filterArgs(os.Args[1:], "-config"))

	cfg, err := config.Load(*configPath)
	if err != nil {
		panic("FATAL: load config: " + err.Error())
	}

	var (
		httpAddr = flag.String("http.addr", cfg.Server.HTTPAddr, "HTTP listen address")
	)
	flag.Parse()

	logger, err := newConfiguredLogger(cfg.Logging)
	if err != nil {
		panic("FATAL: create logger: " + err.Error())
	}
	defer logger.Sync() //nolint:errcheck
	logger.Sugar().Infof("Config loaded from: %s", *configPath)

	generated := initGeneratedServices(logger, cfg)
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

	r.HandleFunc("GET /openapi.json", docs.Handler)
	r.HandleFunc("GET /schema.json", docs.SchemaHandler)
	r.Handle("GET /swagger/", swaggerUI.New("UserService API", "/openapi.json", "/swagger/"))

	runtime.registerRoutes(r)
	customRoutes := registerCustomRoutes(r)

	if cfg.Debug.RoutesEnabled {
		r.HandleFunc("GET /debug/routes", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			json.NewEncoder(w).Encode(generatedRouteEntries(runtime, customRoutes, true, true))
		})
	}

	allRoutes := generatedRouteEntries(runtime, customRoutes, true, true)
	if cfg.Debug.PrintRoutes {
		printAllRoutes(logger, allRoutes)
	}

	httpServer := &http.Server{
		Addr: *httpAddr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			r.ServeHTTP(w, req)
			logger.Sugar().Infof("[HTTP] %s %s %v", req.Method, req.URL.Path, time.Since(start))
		}),
		ReadTimeout:       cfg.Server.ReadTimeout,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       60 * time.Second,
	}
	serverErr := make(chan error, 2)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- fmt.Errorf("HTTP server: %w", err)
		}
	}()

	printBanner(logger, *httpAddr, true, true)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-quit:
		logger.Sugar().Infof("Received signal: %s", sig)
	case err := <-serverErr:
		logger.Sugar().Errorf("Server stopped unexpectedly: %v", err)
	}
	signal.Stop(quit)
	logger.Sugar().Info("Shutting down...")

	shutdownTimeout := cfg.Server.GracefulShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Sugar().Infof("HTTP shutdown error: %v", err)
	}
	logger.Sugar().Info("Server exited cleanly")
}

func filterArgs(args []string, name string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == name || arg == "-"+strings.TrimPrefix(name, "-") {
			out = append(out, arg)
			if i+1 < len(args) {
				out = append(out, args[i+1])
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, name+"=") || strings.HasPrefix(arg, "-"+strings.TrimPrefix(name, "-")+"=") {
			out = append(out, arg)
		}
	}
	return out
}
