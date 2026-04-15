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
	"syscall"
	"time"

	userserviceSvc "example.com/gen_idl_minimal_runtime/service/userservice"
	userserviceEndpoint "example.com/gen_idl_minimal_runtime/endpoint/userservice"
	userserviceTransport "example.com/gen_idl_minimal_runtime/transport/userservice"
	"github.com/gorilla/mux"
	kitlog "github.com/dreamsxin/go-kit/log"


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
	}
	logger.Sugar().Info("─── Registered Routes ───────────────────────────")
	for _, rt := range routes {
		logger.Sugar().Infof("  %-7s %s", rt.Method, rt.Path)
	}
	logger.Sugar().Info("─────────────────────────────────────────────────")
}

func main() {
	// ─── 命令行参数 ───
	var (
		httpAddr = flag.String("http.addr", ":8080", "HTTP listen address")
	)
	flag.Parse()

	logger, _ := kitlog.NewDevelopment()
	defer logger.Sync() //nolint:errcheck




	// ─── 初始化服务 ───
	userserviceSvcInst := userserviceSvc.NewService(nil)


	// ─── 初始化端点（配置驱动中间件）───
	userserviceEndpoints := userserviceEndpoint.MakeServerEndpoints(userserviceSvcInst, logger)


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





	// /debug/routes — 聚合所有服务路由，方便调试
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
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(all)
	}).Methods("GET")

	r.PathPrefix("").Handler(
		http.StripPrefix("", userserviceTransport.NewHTTPHandler(userserviceEndpoints)),
	)


	printAllRoutes(logger)

	httpServer := &http.Server{
		Addr:         *httpAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Sugar().Fatalf("FATAL: HTTP server: %v", err)
		}
	}()



	printBanner(logger, *httpAddr, false, false)

	// ─── 优雅关闭 ───
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


