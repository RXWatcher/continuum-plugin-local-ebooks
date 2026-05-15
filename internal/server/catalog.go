// Package server mounts the ebook_backend.v1 HTTP routes onto an
// http.ServeMux. Task 30 wires this into the plugin's HttpRoutes service;
// for now nothing imports this package from main.go.
package server

import (
	"net/http"

	"github.com/ContinuumApp/continuum-plugin-local-ebooks/internal/grpc/ebookbackend"
)

// MountCatalog registers /catalog/* routes on mux. The handlers come from
// the ebookbackend package — this file is a thin adapter that maps URLs to
// methods so future routes (admin, scan, etc.) can be added alongside.
func MountCatalog(mux *http.ServeMux, srv *ebookbackend.Server) {
	mux.HandleFunc("GET /capabilities", handleCapabilities)
	mux.HandleFunc("GET /api/v1/capabilities", handleCapabilities)
	// Note: Go 1.22+ http.ServeMux ordering — the most specific pattern wins.
	// We register sub-paths (/cover, /file) BEFORE the catch-all /{id} so
	// they take precedence.
	mux.HandleFunc("GET /catalog/libraries", srv.Libraries)
	mux.HandleFunc("GET /catalog/authors", srv.Authors)
	mux.HandleFunc("GET /catalog/series", srv.SeriesList)
	mux.HandleFunc("GET /catalog/genres", srv.Genres)
	mux.HandleFunc("GET /catalog/{id}/cover", srv.Cover)
	mux.HandleFunc("GET /catalog/{id}/file", srv.File)
	mux.HandleFunc("GET /catalog/{id}", srv.Detail)
	mux.HandleFunc("GET /catalog", srv.List)
}

func handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"formats": []string{"epub", "pdf", "mobi", "azw", "azw3", "fb2"},
		"features": []string{
			"library_source",
			"metadata_provider",
			"multi_library",
			"admin_diagnostics",
			"scan_status",
			"scan_trigger",
			"metadata_queue_status",
			"browse_facets",
			"covers",
		},
		"max_concurrent_downloads": 8,
		"supports_range_requests":  true,
	})
}
