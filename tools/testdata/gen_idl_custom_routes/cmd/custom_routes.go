package main

import (
	"net/http"

	"github.com/gorilla/mux"
)

func registerCustomRoutes(r *mux.Router) []generatedRouteEntry {
	r.HandleFunc("/custom/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}).Methods("GET")
	return []generatedRouteEntry{
		{Method: "GET", Path: "/custom/ping", Handler: "custom-ping"},
	}
}
