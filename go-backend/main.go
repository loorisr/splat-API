// Signal Coverage Prediction API (Go implementation)
//
// Provides endpoints to predict radio signal coverage
// using the ITM (Irregular Terrain Model) via SPLAT! (https://github.com/jmcmellen/splat).
//
// Endpoints:
//   - /predict:  Accepts a signal coverage prediction request and starts a background task.
//   - /status/{task_id}: Retrieves the status of a given prediction task.
//   - /result/{task_id}: Retrieves the result (GeoTIFF file) of a given prediction task.

package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	// In-memory task store with TTL-based expiration (300 s).
	store := NewTaskStore()

	// Configuration via environment variables with sensible defaults matching the Docker setup.
	splatBinaryPath := envOrDefault("SPLAT_BINARY_PATH", "/app")
	demDir := envOrDefault("DEM_DIR", "/app/DEM")
	antennaDir := envOrDefault("ANTENNA_DIR", "/app/antenna")

	// Initialize the SPLAT! wrapper (validates binary existence, creates DEM cache dir).
	splatService := NewSplat(splatBinaryPath, demDir, antennaDir)

	// Wire up HTTP handlers.
	handler := NewHandler(store, splatService)

	mux := http.NewServeMux()

	// API routes — Go 1.22+ method-based routing.
	mux.HandleFunc("POST /predict", handler.Predict)
	mux.HandleFunc("GET /status/{task_id}", handler.GetStatus)
	mux.HandleFunc("GET /result/{task_id}", handler.GetResult)
	mux.HandleFunc("GET /health", handler.Health)

	// Frontend: if the Vue app was built to app/ui/, serve it as a SPA.
	// Otherwise return a JSON message so the API is still usable without the UI.
	uiDir := filepath.Join("app", "ui")
	if _, err := os.Stat(uiDir); err == nil {
		log.Printf("Serving frontend from %s", uiDir)
		mux.Handle("/", spaFileServer(uiDir))
	} else {
		log.Printf("Frontend build not found at %s", uiDir)
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{
				"message": "API is running. Build the Vue app to app/ui to serve it from the Go backend.",
			})
		})
	}

	server := &http.Server{
		Addr:    ":8080",
		Handler: corsMiddleware(mux),
	}

	log.Println("Starting Go SPLAT! API server on :8080")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// corsMiddleware adds permissive CORS headers and handles OPTIONS preflight requests.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// spaFileServer serves static files from root and falls back to index.html for
// client-side routing (standard SPA behavior).
func spaFileServer(root string) http.Handler {
	fs := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(root, filepath.Clean(r.URL.Path))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			http.ServeFile(w, r, filepath.Join(root, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})
}

// envOrDefault returns the value of the environment variable key, or defaultVal if unset.
func envOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// newUUID generates a random UUIDv4-style hex string using crypto/rand.
func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	buf := make([]byte, 36)
	hex.Encode(buf[:8], b[:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:], b[10:])
	return string(buf)
}
