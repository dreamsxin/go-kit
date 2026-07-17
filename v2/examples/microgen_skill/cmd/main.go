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

	greeterEndpoint "github.com/dreamsxin/go-kit/v2/examples/microgen_skill/endpoint"
	"github.com/dreamsxin/go-kit/v2/examples/microgen_skill/service"
	greeterTransport "github.com/dreamsxin/go-kit/v2/examples/microgen_skill/transport"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
	"google.golang.org/grpc"
	"net"

	"github.com/dreamsxin/go-kit/v2/examples/microgen_skill/config"
	"github.com/dreamsxin/go-kit/v2/examples/microgen_skill/skill"
)

func printBanner(logger *kitlog.Logger, httpAddr string, grpcAddr string, withSkill bool) {
	logger.Sugar().Info("╔══════════════════════════════════════════╗")
	logger.Sugar().Infof("║  %-40s  ║", "Greeter Service")
	logger.Sugar().Info("╠══════════════════════════════════════════╣")
	logger.Sugar().Infof("║  HTTP  → http://localhost%s%-*s║", httpAddr, 23-len(httpAddr), "")
	if withSkill {
		skillURL := fmt.Sprintf("http://localhost%s/skill", httpAddr)
		logger.Sugar().Infof("║  Skill → %-30s    ║", skillURL)
	}
	logger.Sugar().Infof("║  gRPC  → %s%-*s║", grpcAddr, 32-len(grpcAddr), "")
	logger.Sugar().Info("╠══════════════════════════════════════════╣")
	logger.Sugar().Info("║  Press Ctrl+C to stop                    ║")
	logger.Sugar().Info("╚══════════════════════════════════════════╝")
}

func printAllRoutes(logger *kitlog.Logger) {
	type routeEntry struct {
		Method string
		Path   string
	}
	routes := []routeEntry{
		{"GET", "/health"},
		{"GET", "/debug/routes"},
		{"POST", "/sayhello"},
		{"GET", "/getstatus"},
		{"GET", "/skill"},
	}
	logger.Sugar().Info("─── Registered Routes ───────────────────────────")
	for _, rt := range routes {
		logger.Sugar().Infof("  %-7s %s", rt.Method, rt.Path)
	}
	logger.Sugar().Info("─────────────────────────────────────────────────")
}

func main() {
	// ─── 加载配置文件 ───
	// -config 优先，默认读取 config/config.yaml；文件不存在时使用内置默认值。
	configPath := flag.String("config", "config/config.yaml", "path to config file")
	// 先只解析 -config，以便后续用配置值作 flag 默认值
	flag.CommandLine.Parse(filterArgs(os.Args[1:], "-config"))

	cfg, err := config.Load(*configPath)
	if err != nil {
		panic("FATAL: load config: " + err.Error())
	}

	// ─── 命令行参数（可覆盖配置文件中的值）───
	var (
		httpAddr = flag.String("http.addr", cfg.Server.HTTPAddr, "HTTP listen address")
		grpcAddr = flag.String("grpc.addr", cfg.Server.GRPCAddr, "gRPC listen address")
	)
	flag.Parse()

	logger, _ := kitlog.NewDevelopment()
	defer logger.Sync() //nolint:errcheck
	logger.Sugar().Infof("Config loaded from: %s", *configPath)

	// ─── 初始化服务 ───
	greeterSvc := service.NewService(nil)

	// ─── 初始化端点（配置驱动中间件）───
	greeterEndpoints := greeterEndpoint.MakeServerEndpointsWithConfig(greeterSvc, logger, greeterEndpoint.MiddlewareConfig{
		CBEnabled:          cfg.Middleware.CircuitBreaker.Enabled,
		CBFailureThreshold: uint32(cfg.Middleware.CircuitBreaker.FailureThreshold),
		CBTimeout:          cfg.Middleware.CircuitBreaker.Timeout,
		RLEnabled:          cfg.Middleware.RateLimit.Enabled,
		RLRps:              cfg.Middleware.RateLimit.RequestsPerSecond,
		Timeout:            30 * time.Second,
	})

	// ─── 构建路由 ───
	r := http.NewServeMux()

	// 健康检查（无鉴权）
	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok","service":"Greeter"}`)
	}
	r.HandleFunc("GET /health", healthHandler)
	r.HandleFunc("HEAD /health", healthHandler)

	// AI Skill definition (for AI agents)
	r.HandleFunc("GET /skill", skill.Handler)

	// /debug/routes — 聚合所有服务路由，方便调试
	if cfg.Debug.RoutesEnabled {
		r.HandleFunc("GET /debug/routes", func(w http.ResponseWriter, req *http.Request) {
			type routeInfo struct {
				Method  string `json:"method"`
				Path    string `json:"path"`
				Handler string `json:"handler"`
			}
			var all []routeInfo
			all = append(all, routeInfo{"GET", "/health", "health"})
			all = append(all, routeInfo{"GET", "/debug/routes", "debug"})
			all = append(all, routeInfo{"POST", "/sayhello", "SayHello"})
			all = append(all, routeInfo{"GET", "/getstatus", "GetStatus"})
			all = append(all, routeInfo{"GET", "/skill", "skill"})
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			json.NewEncoder(w).Encode(all)
		})
	}

	greeterTransport.RegisterHTTPRoutes(r, greeterEndpoints, "")

	loggedHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		r.ServeHTTP(w, req)
		logger.Sugar().Infof("[HTTP] %s %s %v", req.Method, req.URL.Path, time.Since(start))
	})

	if cfg.Debug.PrintRoutes {
		printAllRoutes(logger)
	}

	httpServer := &http.Server{
		Addr:         *httpAddr,
		Handler:      loggedHandler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Sugar().Fatalf("FATAL: HTTP server: %v", err)
		}
	}()

	// ─── gRPC 服务 ───
	lis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		logger.Sugar().Fatalf("FATAL: gRPC listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	greeterTransport.RegisterGRPCServer(grpcServer, greeterEndpoints)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			logger.Sugar().Fatalf("FATAL: gRPC server: %v", err)
		}
	}()

	printBanner(logger, *httpAddr, *grpcAddr, true)

	// ─── 优雅关闭 ───
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

	grpcServer.GracefulStop()
	logger.Sugar().Info("gRPC stopped")

	logger.Sugar().Info("Server exited cleanly")
}

// filterArgs 从 args 中提取指定 flag 及其值，用于两阶段解析。
// 例如 filterArgs(os.Args[1:], "-config") 只返回 ["-config", "path/to/config.yaml"]
func filterArgs(args []string, name string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		// 支持 -config=value 和 -config value 两种写法
		if arg == name || arg == "-"+strings.TrimPrefix(name, "-") {
			out = append(out, arg)
			if i+1 < len(args) {
				out = append(out, args[i+1])
				i++
			}
		} else if strings.HasPrefix(arg, name+"=") || strings.HasPrefix(arg, "-"+strings.TrimPrefix(name, "-")+"=") {
			out = append(out, arg)
		}
	}
	return out
}
