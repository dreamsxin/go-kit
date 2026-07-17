package main

import "github.com/gorilla/mux"

// registerCustomRoutes is the user-owned hook for project-specific HTTP routes.
// Add handlers here; microgen will not overwrite this file on rerun.
func registerCustomRoutes(r *mux.Router) []generatedRouteEntry {
	_ = r
	return nil
}
