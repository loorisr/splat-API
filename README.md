# Signal Coverage Prediction API

A FastAPI-based REST service for predicting radio signal coverage using the Irregular Terrain Model (ITM) via SPLAT! (Signal Propagation, Loss, And Terrain)/Signal Server. This API provides asynchronous computation of coverage maps with GeoTIFF output.

It is based on [Meshtastic Site Planner](https://github.com/meshtastic/meshtastic-site-planner) and the API is 100% compatible but uses a more optimized SPLAT! version (modified Signal-Server).

## Features

- **Asynchronous Coverage Prediction**: Submit prediction requests and poll for results
- **GeoTIFF Output**: Returns coverage maps as standard GeoTIFF files
- **ITWOM v3.0 Model**: Uses the ITWOM for accurate signal propagation
- **Automatic Terrain Data**: Downloads and caches DEM (Digital Elevation Model) data from AWS Open Data
- **Multiple Colormaps**: Support for various visualization colormaps (viridis, plasma, hot, cool, jet, rainbow, turbo, CMRmap)
- **RESTful API**: Simple HTTP endpoints for integration
- **Docker Support**: Easy deployment with Docker and Docker Compose

## Architecture

The service consists of:
- **FastAPI** web server with async endpoints
- **SPLAT!** binary (`signalserver`) for signal propagation calculations
- **In-memory task store** for tracking prediction jobs
- **Background task processing** for long-running predictions
- **Automatic DEM fetching** from Copernicus DEM datasets

## API Endpoints

### `POST /predict`
Start a new coverage prediction task.

**Request Body** (JSON):
```json
{
  "lat": 48.8566,
  "lon": 2.3522,
  "tx_height": 30.0,
  "tx_power": 50.0,
  "tx_gain": 10.0,
  "frequency_mhz": 905.0,
  "rx_height": 1.5,
  "rx_gain": 2.0,
  "signal_threshold": -100.0,
  "clutter_height": 0.0,
  "ground_dielectric": 15.0,
  "ground_conductivity": 0.005,
  "atmosphere_bending": 301.0,
  "radius": 100000.0,
  "system_loss": 0.0,
  "radio_climate": "continental_temperate",
  "polarization": "vertical",
  "situation_fraction": 50.0,
  "colormap": "viridis",
  "high_resolution": False
}
```

**Response**:
```json
{
  "task_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

### `GET /status/{task_id}`
Check the status of a prediction task.

**Response**:
```json
{
  "task_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "processing",
  "progress": 45
}
```

Status values: `processing`, `completed`, `failed`

### `GET /result/{task_id}`
Download the GeoTIFF result for a completed task.

**Response**: GeoTIFF file with appropriate headers.

## Request Parameters

### Transmitter Parameters
- `lat` (float): Transmitter latitude in degrees (-90 to 90)
- `lon` (float): Transmitter longitude in degrees (-180 to 180)
- `tx_height` (float): Transmitter height above ground in meters (≥ 1 m)
- `tx_power` (float): Transmitter power in dBm (≥ 1 dBm)
- `tx_gain` (float): Transmitter antenna gain in dB (≥ 0)
- `frequency_mhz` (float): Operating frequency in MHz (20-30000 MHz)

### Receiver Parameters
- `rx_height` (float): Receiver height above ground in meters (≥ 1 m)
- `rx_gain` (float): Receiver antenna gain in dB (≥ 0)
- `signal_threshold` (float): Signal cutoff in dBm (≤ 0)

### Environmental Parameters
- `clutter_height` (float): Ground clutter height in meters (≥ 0)
- `ground_dielectric` (float): Ground dielectric constant (default: 15.0)
- `ground_conductivity` (float): Ground conductivity in S/m (default: 0.005)
- `atmosphere_bending` (float): Atmospheric bending constant in N-units (default: 301.0)

### Model Settings
- `radius` (float): Model maximum range in meters (≥ 1 m, capped at 100 km)
- `system_loss` (float): System loss in dB (default: 0.0)
- `radio_climate` (string): One of: "equatorial", "continental_subtropical", "maritime_subtropical", "desert", "continental_temperate", "maritime_temperate_land", "maritime_temperate_sea"
- `polarization` (string): "horizontal" or "vertical"
- `situation_fraction` (float): Percentage of locations where prediction is valid (1-100, default: 50)

### Visualization
- `colormap` (string): Colormap for visualization: "viridis", "plasma", "hot", "cool", "jet", "rainbow", "turbo", "CMRmap"
- `high_resolution` (boolean): Use high resolution (30m) instead of standard (90m). Calculation are 9 times slower (default: False)

## Quick Start

### Prerequisites
- Docker and Docker Compose
- Python 3.14+ (for local development)
- UV package manager (recommended)

### Using Docker (Recommended)

1. Clone the repository:
```bash
git clone https://github.com/loorisr/splat-API.git
cd splat_API
```

2. Build and run with Docker Compose:
```bash
docker-compose up --build
```

The API will be available at `http://localhost:8080`

### Local Development

1. Install dependencies with UV:
```bash
uv sync
```

2. Ensure the SPLAT! binary (`signalserver`) is in the project root and executable:
```bash
chmod +x signalserver
```

3. Run the FastAPI server:
```bash
uv run uvicorn app.main:app --host 0.0.0.0 --port 8080 --reload
```

## Technical Details

### SPLAT! Integration
The service uses a precompiled `signalserver` binary (SPLAT! variant) to perform ITM calculations. The binary is called via subprocess with generated configuration files.
The source code is available here: [Signal-Server](https://github.com/loorisr/Signal-Server)

### Terrain Data
Digital Elevation Model (DEM) data is automatically downloaded from AWS Open Data Copernicus DEM:
- 90m resolution dataset by default
- 30m high-resolution available
- Tiles are cached locally in the configured DEM directory
- Automatic fetching based on prediction location

### Task Management
- In-memory task store with TTL (5 minutes by default)
- Background task execution using FastAPI's `BackgroundTasks`
- Progress tracking via callback system
- Thread-safe operations with locking

### GeoTIFF Generation
- Coverage predictions are converted to GeoTIFF format
- Proper georeferencing with bounds and coordinate system
- Colormap application for visualization
- Rasterio library for GeoTIFF creation

## Configuration

### Environment Variables
- `DEM_DIR`: Directory for DEM cache (default: `/app/DEM`)
- `SPLAT_PATH`: Path to SPLAT! binaries (default: `/app`)
- `TASK_TTL_SECONDS`: Task time-to-live in seconds (default: 300)

## Performance Considerations

- **Radius Limitation**: Maximum radius is capped at 100 km (100,000 m)
- **DEM Resolution**: Higher resolution (30m) provides more details but requires more data download and longer computation (x9)
- **Concurrent Jobs**: Limited to 1 concurrent job by default (configurable)
- **Job Timeout**: Default 120 seconds timeout per prediction

## Limitations

- In-memory task store (not persistent across restarts)
- DEM downloads can be slow for first-time locations
- Maximum prediction radius of 100 km

## Acknowledgments

- SPLAT! by John A. Magliacane, KD2BD (https://www.qsl.net/kd2bd/splat.html)
- Irregular Terrain Model (ITM) by the Institute for Telecommunication Sciences
- Copernicus DEM data provided by ESA and AWS Open Data
- Meshtastic team for their [Meshtastic Site Planner](https://github.com/meshtastic/meshtastic-site-planner)
