package main

import "net/http"

// registerCustomRoutes is the user-owned hook for project-specific HTTP routes.
// Add handlers here; microgen will not overwrite this file on rerun.
func registerCustomRoutes(r *http.ServeMux) []generatedRouteEntry {
	_ = r
	return nil
}
