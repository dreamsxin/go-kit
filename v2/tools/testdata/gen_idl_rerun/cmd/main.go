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

	"example.com/gen_idl_rerun/repository"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"example.com/gen_idl_rerun/config"
	docs "example.com/gen_idl_rerun/docs"
	"example.com/gen_idl_rerun/skill"
	httpSwagger "github.com/swaggo/http-swagger"
)

func printBanner(logger *kitlog.Logger, httpAddr string, withOpenAPI bool, withSkill bool) {
	logger.Sugar().Info("------------------------------------------------------------")
	logger.Sugar().Infof(" Service: UserService ")
	logger.Sugar().Infof(" HTTP: http://localhost%s", httpAddr)
	if withOpenAPI {
		logger.Sugar().Infof(" OpenAPI: http://localhost%s/openapi.json", httpAddr)
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
		httpAddr    = flag.String("http.addr", cfg.Server.HTTPAddr, "HTTP listen address")
		dsn         = flag.String("db.dsn", cfg.Database.DSN, "mysql DSN")
		autoMigrate = flag.Bool("auto-migrate", cfg.Database.AutoMigrate, "run database AutoMigrate on startup")
	)
	flag.Parse()
	cfg.Database.AutoMigrate = *autoMigrate

	logger, err := newConfiguredLogger(cfg.Logging)
	if err != nil {
		panic("FATAL: create logger: " + err.Error())
	}
	defer logger.Sync() //nolint:errcheck
	logger.Sugar().Infof("Config loaded from: %s", *configPath)

	db, err := gorm.Open(mysql.Open(*dsn), &gorm.Config{})
	if err != nil {
		logger.Sugar().Fatalf("FATAL: connect database failed: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)

	logger.Sugar().Infof("DB connected [driver=%s dsn=%s]", "mysql", redactDSN(*dsn))

	repoDB := repository.NewDB(db)

	generated := initGeneratedServices(logger, cfg, repoDB)
	runtime := generated.generatedRuntime()

	if cfg.Database.AutoMigrate {
		if err := runtime.autoMigrate(db); err != nil {
			logger.Sugar().Fatalf("FATAL: auto migrate failed: %v", err)
		}
		logger.Sugar().Info("DB migration done")
	} else {
		logger.Sugar().Info("DB migration skipped")
	}

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
	r.Handle("GET /swagger/", httpSwagger.Handler(
		httpSwagger.URL("/openapi.json"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("list"),
	))

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
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Sugar().Fatalf("FATAL: HTTP server: %v", err)
		}
	}()

	printBanner(logger, *httpAddr, true, true)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
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

func redactDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	return "<redacted>"
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
