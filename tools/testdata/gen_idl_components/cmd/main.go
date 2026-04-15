// @title          UserService API
// @version        1.0
// @description    UserService microservice API
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

	userserviceSvc "example.com/gen_idl_components/service/userservice"
	userserviceEndpoint "example.com/gen_idl_components/endpoint/userservice"
	userserviceTransport "example.com/gen_idl_components/transport/userservice"
	"github.com/gorilla/mux"
	kitlog "github.com/dreamsxin/go-kit/log"


	httpSwagger "github.com/swaggo/http-swagger"
	docs "example.com/gen_idl_components/docs"
	"example.com/gen_idl_components/skill"
	"example.com/gen_idl_components/config"
)

func printBanner(logger *kitlog.Logger, httpAddr string, withSwag bool, withSkill bool) {
	logger.Sugar().Info("╔══════════════════════════════════════════╗")
	logger.Sugar().Infof("║  %-40s  ║", "UserService Service")
	logger.Sugar().Info("╠══════════════════════════════════════════╣")
	logger.Sugar().Infof("║  HTTP  → http://localhost%s%-*s║", httpAddr, 23-len(httpAddr), "")
	if withSwag {
		swaggerURL := fmt.Sprintf("http://localhost%s/swagger/index.html", httpAddr)
		logger.Sugar().Infof("║  Swagger → %-30s  ║", swaggerURL)
	}
	if withSkill {
		skillURL := fmt.Sprintf("http://localhost%s/skill", httpAddr)
		logger.Sugar().Infof("║  Skill → %-30s    ║", skillURL)
	}
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
		{"POST", "/createuser"},
		{"GET", "/getuser"},
		{"GET", "/listusers"},
		{"DELETE", "/deleteuser"},
		{"PUT", "/updateuser"},
		{"GET", "/findbyemail"},
		{"GET", "/searchusers"},
		{"GET", "/querystats"},
		{"DELETE", "/removeexpired"},
		{"PUT", "/editprofile"},
		{"PUT", "/modifyemail"},
		{"PUT", "/patchstatus"},
		{"GET", "/swagger/"},
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
	)
	flag.Parse()

	logger, _ := kitlog.NewDevelopment()
	defer logger.Sync() //nolint:errcheck
	logger.Sugar().Infof("Config loaded from: %s", *configPath)




	// ─── 初始化服务 ───
	userserviceSvcInst := userserviceSvc.NewService(nil)


	// ─── 初始化端点（配置驱动中间件）───
	userserviceEndpoints := userserviceEndpoint.MakeServerEndpointsWithConfig(userserviceSvcInst, logger, userserviceEndpoint.MiddlewareConfig{
		CBEnabled:          cfg.Middleware.CircuitBreaker.Enabled,
		CBFailureThreshold: uint32(cfg.Middleware.CircuitBreaker.FailureThreshold),
		CBTimeout:          cfg.Middleware.CircuitBreaker.Timeout,
		RLEnabled:          cfg.Middleware.RateLimit.Enabled,
		RLRps:              cfg.Middleware.RateLimit.RequestsPerSecond,
		Timeout:            30 * time.Second,
	})


	// ─── 构建路由 ───
	r := mux.NewRouter()

	// 请求日志中间件
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, req)
			logger.Sugar().Infof("[HTTP] %s %s %v", req.Method, req.URL.Path, time.Since(start))
		})
	})

	// 健康检查（无鉴权）
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok","service":"UserService"}`)
	}).Methods("GET", "HEAD")


	// AI Skill definition (for AI agents)
	r.HandleFunc("/skill", skill.Handler).Methods("GET")



	// ─── Swagger UI ───
	// 用运行时 httpAddr 动态更新 SwaggerInfo.Host，优先使用配置文件中的 swagger_host。
	{
		swaggerHost := cfg.Server.SwaggerHost
		if swaggerHost == "" {
			// ":8080" → "localhost:8080"；"0.0.0.0:8080" → "localhost:8080"
			addr := *httpAddr
			if len(addr) > 0 && addr[0] == ':' {
				swaggerHost = "localhost" + addr
			} else {
				// 将绑定 IP 替换为 localhost，保持端口
				if i := strings.LastIndex(addr, ":"); i >= 0 {
					swaggerHost = "localhost" + addr[i:]
				} else {
					swaggerHost = addr
				}
			}
		}
		docs.SwaggerInfo.Host = swaggerHost
	}
	r.PathPrefix("/swagger/").Handler(httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("list"),
	))


	// /debug/routes — 聚合所有服务路由，方便调试
	if cfg.Debug.RoutesEnabled {
	r.HandleFunc("/debug/routes", func(w http.ResponseWriter, req *http.Request) {
		type routeInfo struct {
			Method  string `json:"method"`
			Path    string `json:"path"`
			Handler string `json:"handler"`
		}
		var all []routeInfo
		all = append(all, routeInfo{"GET", "/health", "health"})
		all = append(all, routeInfo{"GET", "/debug/routes", "debug"})
		all = append(all, routeInfo{"POST", "/createuser", "CreateUser"})
		all = append(all, routeInfo{"GET", "/getuser", "GetUser"})
		all = append(all, routeInfo{"GET", "/listusers", "ListUsers"})
		all = append(all, routeInfo{"DELETE", "/deleteuser", "DeleteUser"})
		all = append(all, routeInfo{"PUT", "/updateuser", "UpdateUser"})
		all = append(all, routeInfo{"GET", "/findbyemail", "FindByEmail"})
		all = append(all, routeInfo{"GET", "/searchusers", "SearchUsers"})
		all = append(all, routeInfo{"GET", "/querystats", "QueryStats"})
		all = append(all, routeInfo{"DELETE", "/removeexpired", "RemoveExpired"})
		all = append(all, routeInfo{"PUT", "/editprofile", "EditProfile"})
		all = append(all, routeInfo{"PUT", "/modifyemail", "ModifyEmail"})
		all = append(all, routeInfo{"PUT", "/patchstatus", "PatchStatus"})
		all = append(all, routeInfo{"GET", "/swagger/", "swagger-ui"})
		all = append(all, routeInfo{"GET", "/skill", "skill"})
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(all)
	}).Methods("GET")
	}

	r.PathPrefix("").Handler(
		http.StripPrefix("", userserviceTransport.NewHTTPHandler(userserviceEndpoints)),
	)


	if cfg.Debug.PrintRoutes {
		printAllRoutes(logger)
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



	printBanner(logger, *httpAddr, true, true)

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

