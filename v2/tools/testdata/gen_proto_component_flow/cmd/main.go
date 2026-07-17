// @title          UserService API
// @version        1.0
// @description    UserService handles user operations.
// @termsOfService http://swagger.io/terms/
//
// @contact.name   API Support
// @contact.url    http://example.com/support
// @contact.email  support@example.com
//
// @license.name   MIT
// @license.url    https://opensource.org/licenses/MIT
//
// @host           localhost:8080
// @BasePath       /
//
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
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

	"github.com/gorilla/mux"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"google.golang.org/grpc"
	"net"


	"example.com/gen_proto_component_flow/skill"
	"example.com/gen_proto_component_flow/config"
)

func printBanner(logger *kitlog.Logger, httpAddr string, grpcAddr string, withSwag bool, withSkill bool) {
	logger.Sugar().Info("------------------------------------------------------------")
	logger.Sugar().Infof(" Service: UserService ")
	logger.Sugar().Infof(" HTTP: http://localhost%s", httpAddr)
	if withSwag {
		logger.Sugar().Infof(" Swagger: http://localhost%s/swagger/index.html", httpAddr)
	}
	if withSkill {
		logger.Sugar().Infof(" Skill: http://localhost%s/skill", httpAddr)
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
		grpcAddr = flag.String("grpc.addr", cfg.Server.GRPCAddr, "gRPC listen address")
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





	r := mux.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, req)
			logger.Sugar().Infof("[HTTP] %s %s %v", req.Method, req.URL.Path, time.Since(start))
		})
	})

	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok","service":"UserService"}`)
	}).Methods("GET", "HEAD")

	r.HandleFunc("/skill", skill.Handler).Methods("GET")





	runtime.registerRoutes(r)
	customRoutes := registerCustomRoutes(r)

	if cfg.Debug.RoutesEnabled {
	r.HandleFunc("/debug/routes", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(generatedRouteEntries(runtime, customRoutes, false, true))
	}).Methods("GET")
	}


	allRoutes := generatedRouteEntries(runtime, customRoutes, false, true)
	if cfg.Debug.PrintRoutes {
		printAllRoutes(logger, allRoutes)
	}


	httpServer := &http.Server{
		Addr:         *httpAddr,
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Sugar().Fatalf("FATAL: HTTP server: %v", err)
		}
	}()

	lis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		logger.Sugar().Fatalf("FATAL: gRPC listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	runtime.registerGRPCServices(grpcServer)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			logger.Sugar().Fatalf("FATAL: gRPC server: %v", err)
		}
	}()


	printBanner(logger, *httpAddr, *grpcAddr, false, true)

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
