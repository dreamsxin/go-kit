package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	idl "example.com/gen_idl_minimal_runtime"
	userserviceendpoint "example.com/gen_idl_minimal_runtime/endpoint/userservice"
	userservicesvc "example.com/gen_idl_minimal_runtime/service/userservice"
	userservicetransport "example.com/gen_idl_minimal_runtime/transport/userservice"
	kitlog "github.com/dreamsxin/go-kit/log"
)

func main() {
	logger, err := kitlog.NewDevelopment()
	if err != nil {
		panic(err)
	}

	svc := userservicesvc.NewService(nil)
	endpoints := userserviceendpoint.MakeServerEndpoints(svc, logger)
	handler := userservicetransport.NewHTTPHandler(endpoints)

	reqBody := []byte(`{"username":"component-user","email":"component@example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/createuser", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		panic("unexpected status")
	}

	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "CreateUser: not implemented") {
		panic("unexpected body")
	}

	_ = idl.CreateUserRequest{}
}
