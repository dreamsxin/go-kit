package main

	import "net/http"

	func registerCustomRoutes(r *http.ServeMux) []generatedRouteEntry {
		r.HandleFunc("GET /custom/ping", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(204)
		})
		return []generatedRouteEntry{
			{Method: "GET", Path: "/custom/ping", Handler: "custom-ping"},
		}
	}
