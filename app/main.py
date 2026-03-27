"""
Signal Coverage Prediction API

Provides endpoints to predict radio signal coverage
using the ITM (Irregular Terrain Model) via SPLAT! (https://github.com/jmcmellen/splat).

Endpoints:
    - /predict: Accepts a signal coverage prediction request and starts a background task.
    - /status/{task_id}: Retrieves the status of a given prediction task.
    - /result/{task_id}: Retrieves the result (GeoTIFF file) of a given prediction task.
"""

import os
import threading
from datetime import datetime, timedelta
from fastapi import FastAPI, BackgroundTasks
from fastapi.responses import JSONResponse, StreamingResponse
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles
from uuid import uuid4
from app.services.splat import Splat
from app.models.CoveragePredictionRequest import CoveragePredictionRequest
import logging
import io

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# In-memory task store
_task_store: dict = {}
_store_lock = threading.Lock()
TASK_TTL_SECONDS = 300

def _store_set(key: str, value) -> None:
    with _store_lock:
        _task_store[key] = {"value": value, "expires": datetime.utcnow() + timedelta(seconds=TASK_TTL_SECONDS)}


def _store_get(key: str):
    with _store_lock:
        entry = _task_store.get(key)
        if entry is None:
            return None
        if datetime.utcnow() > entry["expires"]:
            del _task_store[key]
            return None
        return entry["value"]

# Initialize RF prediction service — binaries are in /app (project root in Docker)
splat_service = Splat(splat_path="/app", dem_dir="/app/DEM")

# Initialize FastAPI app
app = FastAPI()

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


def run_splat(task_id: str, request: CoveragePredictionRequest):
    """Execute the SPLAT! coverage prediction and store the resulting GeoTIFF in memory."""
    try:
        logger.info(f"Starting SPLAT! coverage prediction for task {task_id}.")
        _store_set(f"{task_id}:progress", 0)
        geotiff_data = splat_service.coverage_prediction(
            request,
            progress_callback=lambda pct: _store_set(f"{task_id}:progress", pct),
        )
        _store_set(task_id, geotiff_data)
        _store_set(f"{task_id}:status", "completed")
        logger.info(f"Task {task_id} marked as completed.")
    except Exception as e:
        logger.error(f"Error in SPLAT! task {task_id}: {e}")
        _store_set(f"{task_id}:status", "failed")
        _store_set(f"{task_id}:error", str(e))
        raise


@app.post("/predict")
async def predict(payload: CoveragePredictionRequest, background_tasks: BackgroundTasks) -> JSONResponse:
    """Start an async SPLAT! coverage prediction. Returns a task_id to poll for status/result."""
    task_id = str(uuid4())
    _store_set(f"{task_id}:status", "processing")
    background_tasks.add_task(run_splat, task_id, payload)
    return JSONResponse({"task_id": task_id})


@app.get("/status/{task_id}")
async def get_status(task_id: str):
    """Retrieve the status of a given prediction task."""
    status = _store_get(f"{task_id}:status")
    if status is None:
        logger.warning(f"Task {task_id} not found.")
        return JSONResponse({"error": "Task not found"}, status_code=404)
    progress = _store_get(f"{task_id}:progress") if status == "processing" else (100 if status == "completed" else None)
    return JSONResponse({"task_id": task_id, "status": status, "progress": progress})


@app.get("/result/{task_id}")
async def get_result(task_id: str):
    """Retrieve the GeoTIFF result for a completed task."""
    status = _store_get(f"{task_id}:status")
    if status is None:
        logger.warning(f"Task {task_id} not found.")
        return JSONResponse({"error": "Task not found"}, status_code=404)

    if status == "completed":
        geotiff_data = _store_get(task_id)
        if not geotiff_data:
            logger.error(f"No data found for completed task {task_id}.")
            return JSONResponse({"error": "No result found"}, status_code=500)
        return StreamingResponse(
            io.BytesIO(geotiff_data),
            media_type="image/tiff",
            headers={"Content-Disposition": f"attachment; filename={task_id}.tif"}
        )
    elif status == "failed":
        error = _store_get(f"{task_id}:error")
        return JSONResponse({"status": "failed", "error": error})

    logger.info(f"Task {task_id} is still processing.")
    return JSONResponse({"status": "processing"})
