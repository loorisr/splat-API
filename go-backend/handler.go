package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// Handler groups the HTTP handlers for the SPLAT! API endpoints.
type Handler struct {
	store *TaskStore
	splat *Splat
}

// NewHandler creates a Handler with the given store and SPLAT! service.
func NewHandler(store *TaskStore, splat *Splat) *Handler {
	return &Handler{store: store, splat: splat}
}

// Predict handles POST /predict.
// It validates the request, creates a task in the store, and launches
// the SPLAT! coverage prediction asynchronously. Returns a task_id immediately.
func (h *Handler) Predict(w http.ResponseWriter, r *http.Request) {
	var req CoveragePredictionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON: " + err.Error()})
		return
	}

	if err := req.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	taskID := newUUID()
	h.store.Set(taskID+":status", "processing")
	h.store.Set(taskID+":progress", 0)

	go h.runSplat(taskID, &req)

	writeJSON(w, http.StatusOK, map[string]string{"task_id": taskID})
}

// GetStatus handles GET /status/{task_id}.
// Returns the current status ("processing", "completed", or "failed") and,
// when processing, the current progress percentage (0-100).
func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	status := h.store.Get(taskID + ":status")
	if status == nil {
		log.Printf("Task %s not found", taskID)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Task not found"})
		return
	}

	resp := map[string]interface{}{
		"task_id": taskID,
		"status":  status,
	}

	if status == "processing" {
		progress := h.store.Get(taskID + ":progress")
		if progress != nil {
			resp["progress"] = progress
		}
	} else if status == "completed" {
		resp["progress"] = 100
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetResult handles GET /result/{task_id}.
// For completed tasks it streams the GeoTIFF bytes (image/tiff) and removes
// the result from the store (single-consumption). For failed tasks it returns
// the error details. For still-processing tasks it returns status "processing".
func (h *Handler) GetResult(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	status := h.store.Get(taskID + ":status")
	if status == nil {
		log.Printf("Task %s not found", taskID)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Task not found"})
		return
	}

	if status == "completed" {
		geotiff := h.store.Pop(taskID)
		if geotiff == nil {
			log.Printf("No data found for completed task %s", taskID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No result found"})
			return
		}

		data := geotiff.([]byte)
		w.Header().Set("Content-Type", "image/tiff")
		w.Header().Set("Content-Disposition", "attachment; filename="+taskID+".tif")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		w.Write(data)
		return
	}

	if status == "failed" {
		errMsg := h.store.Get(taskID + ":error")
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "failed", "error": errMsg})
		return
	}

	log.Printf("Task %s is still processing", taskID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "processing"})
}

// Health handles GET /health, returning a simple liveness check.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// runSplat executes the coverage prediction in a background goroutine.
// It updates the task store with progress, the final GeoTIFF result, or
// an error if the prediction fails.
func (h *Handler) runSplat(taskID string, req *CoveragePredictionRequest) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in SPLAT! task %s: %v", taskID, r)
			h.store.Set(taskID+":status", "failed")
			h.store.Set(taskID+":error", "internal panic")
		}
	}()

	log.Printf("Starting SPLAT! coverage prediction for task %s", taskID)
	h.store.Set(taskID+":progress", 0)

	geotiffData, err := h.splat.CoveragePrediction(req, func(pct int) {
		h.store.Set(taskID+":progress", pct)
	})

	if err != nil {
		log.Printf("Error in SPLAT! task %s: %v", taskID, err)
		h.store.Set(taskID+":status", "failed")
		h.store.Set(taskID+":error", err.Error())
		return
	}

	h.store.Set(taskID, geotiffData)
	h.store.Set(taskID+":status", "completed")
	log.Printf("Task %s marked as completed", taskID)
}

// writeJSON serializes data as JSON and writes it with the given HTTP status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
