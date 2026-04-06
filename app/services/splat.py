import logging
import math
import os
import subprocess
import tempfile
import threading
import time
from typing import List, Tuple

import requests as http

import numpy as np

from app.models.CoveragePredictionRequest import CoveragePredictionRequest


logger = logging.getLogger(__name__)
logging.getLogger("urllib3").setLevel(logging.WARNING)


# ---------------------------------------------------------------------------
# Lightweight colormap implementation (replaces matplotlib)
# ---------------------------------------------------------------------------
# Each entry: list of (t, R, G, B) keypoints, t ∈ [0, 1], RGB ∈ [0, 255]
_COLORMAP_KEYS: dict[str, list] = {
    "viridis": [
        (0.000,  68,   1,  84),
        (0.125,  71,  44, 122),
        (0.250,  59,  82, 139),
        (0.375,  44, 114, 142),
        (0.500,  33, 145, 140),
        (0.625,  53, 183, 121),
        (0.750,  94, 201,  98),
        (0.875, 160, 218,  57),
        (1.000, 253, 231,  37),
    ],
    "plasma": [
        (0.000,  13,   8, 135),
        (0.250, 126,   3, 168),
        (0.500, 203,  70, 121),
        (0.750, 248, 149,  64),
        (1.000, 240, 249,  33),
    ],
    "hot": [
        (0.000,   0,   0,   0),
        (0.333, 255,   0,   0),
        (0.667, 255, 255,   0),
        (1.000, 255, 255, 255),
    ],
    "cool": [
        (0.0,   0, 255, 255),
        (1.0, 255,   0, 255),
    ],
    "jet": [
        (0.000,   0,   0, 143),
        (0.125,   0,   0, 255),
        (0.375,   0, 255, 255),
        (0.625, 255, 255,   0),
        (0.875, 255,   0,   0),
        (1.000, 127,   0,   0),
    ],
    "rainbow": [
        (0.000, 127,   0, 255),
        (0.200,   0,   0, 255),
        (0.400,   0, 255, 255),
        (0.600,   0, 255,   0),
        (0.800, 255, 255,   0),
        (1.000, 255,   0,   0),
    ],
    "turbo": [
        (0.00,  48,  18,  59),
        (0.10,  70,  50, 127),
        (0.20,  56, 101, 191),
        (0.30,  34, 149, 202),
        (0.40,  35, 193, 168),
        (0.50,  82, 213, 118),
        (0.60, 149, 225,  54),
        (0.70, 213, 210,  46),
        (0.80, 247, 163,  42),
        (0.90, 239,  91,  30),
        (1.00, 122,   4,   3),
    ],
    "CMRmap": [
        (0.000,   0,   0,   0),
        (0.150,  20,  20,  80),
        (0.300,   0, 100, 200),
        (0.450,   0, 200, 200),
        (0.550, 200, 200,   0),
        (0.700, 250, 150,   0),
        (0.850, 250,  50,   0),
        (1.000, 255, 255, 255),
    ],
}
# Aliases
_COLORMAP_KEYS["hsv"] = _COLORMAP_KEYS["rainbow"]


def _get_colormap(name: str, n: int = 256) -> np.ndarray:
    """
    Return an (n, 4) uint8 RGBA array for the named colormap.
    Falls back to 'viridis' for unknown names.
    """
    keys = _COLORMAP_KEYS.get(name, _COLORMAP_KEYS["viridis"])
    t_vals = np.linspace(0.0, 1.0, n)
    t_keys = [k[0] for k in keys]
    out = np.zeros((n, 4), dtype=np.uint8)
    for ch, col_idx in enumerate([1, 2, 3]):  # R, G, B
        channel_vals = [k[col_idx] for k in keys]
        out[:, ch] = np.clip(np.interp(t_vals, t_keys, channel_vals), 0, 255).astype(np.uint8)
    out[:, 3] = 255  # full alpha
    return out


def _apply_colormap(values: np.ndarray, name: str, vmin: float, vmax: float) -> np.ndarray:
    """Map a 1-D float array to (N, 4) uint8 RGBA using the named colormap."""
    cmap = _get_colormap(name, 255)
    indices = np.clip(
        ((values - vmin) / (vmax - vmin) * 254).astype(int), 0, 254
    )
    return cmap[indices]


class Splat:
    def __init__(
        self,
        splat_path: str,
        dem_dir: str = ".splat_dem",
        bucket_name: str = "copernicus-dem-90m",
        bucket_name_high_resolution: str = "copernicus-dem-30m",
        bucket_prefix: str = "",
        max_concurrent_jobs: int = 1,
        job_timeout: int = 120,
    ):
        """
        RF coverage prediction wrapper using SPLAT!.
        Terrain data is automatically downloaded and cached from AWS Open Data:
        https://registry.opendata.aws/copernicus-dem/

        Args:
            splat_path (str): Directory containing the binaries.
            dem_dir (str): Directory to store converted SDF terrain files.
            bucket_name (str): S3 bucket name for terrain tiles.
            bucket_prefix (str): S3 prefix for terrain tiles (v2/skadi = 1-arcsecond).
        """
        if not os.path.isdir(splat_path):
            raise FileNotFoundError(f"Binary path '{splat_path}' is not a valid directory.")

        self.splat_binary = os.path.join(splat_path, "signalserver") # was splat

        binaries = [
            ("splat", self.splat_binary),
        ]

        for label, path in binaries:
            if not os.path.isfile(path) or not os.access(path, os.X_OK):
                raise FileNotFoundError(f"'{label}' binary not found or not executable at '{path}'")

        os.makedirs(dem_dir, exist_ok=True)
        self.dem_dir = dem_dir
        self.bucket_name = bucket_name
        self.bucket_name_high_resolution = bucket_name_high_resolution
        self.bucket_prefix = bucket_prefix
        self._semaphore = threading.Semaphore(max_concurrent_jobs)
        self._job_timeout = job_timeout

        logger.info(
            f"Initialized SPLAT! — dem_dir: '{dem_dir}', "
            f"max concurrent jobs: {max_concurrent_jobs}, timeout: {job_timeout}s"
        )

    def coverage_prediction(
        self,
        request: CoveragePredictionRequest,
        progress_callback=None,
    ) -> bytes:
        """
        Execute a SPLAT! coverage prediction.

        Args:
            request (CoveragePredictionRequest): Prediction parameters.
            progress_callback: Optional callable(int) receiving progress 0-100.

        Returns:
            bytes: GeoTIFF coverage map.
        """
        def report(pct: int):
            if progress_callback:
                progress_callback(pct)

        with tempfile.TemporaryDirectory() as tmpdir:
            try:
                radius = min(request.radius, 100000)
                if radius != request.radius:
                    logger.debug(f"Capping radius from {request.radius} m to 100 km.")

                required_tiles = Splat._calculate_required_terrain_tiles(
                    request.lat, request.lon, radius, high_resolution=request.high_resolution
                )
                n = len(required_tiles)

                for i, (copernicus) in enumerate(required_tiles):
                    sdf_path = self._ensure_dem(copernicus, high_resolution=request.high_resolution)
                    # tiles: 0 → 40%
                    report(int(40 * (i + 1) / n))

                dcf_path = os.path.join(tmpdir, "splat.dcf")
                with open(dcf_path, "wb") as f:
                    f.write(Splat._create_splat_dcf(
                        colormap_name=request.colormap,
                        min_dbm=request.min_dbm,
                        max_dbm=request.max_dbm,
                    ))

                climate_map = {
                    "equatorial": 1,
                    "continental_subtropical": 2,
                    "maritime_subtropical": 3,
                    "desert": 4,
                    "continental_temperate": 5,
                    "maritime_temperate_land": 6,
                    "maritime_temperate_sea": 7,
                }

                # expected : erp: Tx Total Effective Radiated Power in Watts (dBd) inc Tx+Rx gain. 2.14dBi = 0dBd\n");
                erp_watts = 10 ** ((request.tx_power + request.tx_gain - request.system_loss - 30) / 10)  

                binary = self.splat_binary
                command = [
                    binary,
                    "-lat", str(request.lat),
                    "-lon", str(request.lon),
                    "-txh", str(request.tx_height),
                    "-cl", str(climate_map[request.radio_climate]),
                    "-terdic", str(request.ground_dielectric),
                    "-tercon", str(request.ground_conductivity),
                    "-f", str(request.frequency_mhz),
                    "-rel", str(request.time_fraction),
                    "-conf", str(request.situation_fraction),
                    "-erp", str(erp_watts), 
                    "-color", dcf_path,
                    "-rxh", str(request.rx_height),
                    "-rxg", str(request.rx_gain), # not used in calculation
                    "-R", str(radius / 1000.0),
                    "-gc", str(request.clutter_height),
                   # "-ngs", "-N",
                    "-o", "output",
                    "-geotiff",
                    "-dbm",
                    "-rt", str(request.signal_threshold),
                    "-dem", self.dem_dir,
                ]
                if request.high_resolution:
                    command.append("-hd")
                if request.polarization == "horizontal":
                    command.append("-hp") ## default is vertical
                img_filename = "output.tif"

                report(45)
                logger.info(f"Running splat: {' '.join(command)}")

                with self._semaphore:
                    t0 = time.monotonic()
                    try:
                        result = subprocess.run(
                            command, cwd=tmpdir,
                            capture_output=True, text=True,
                            check=False, timeout=self._job_timeout,
                        )
                    except subprocess.TimeoutExpired:
                        raise RuntimeError(
                            f"splat timed out after {self._job_timeout}s"
                        )
                    elapsed = time.monotonic() - t0
                    logger.info(f"splat finished in {elapsed:.1f}s")
                    report(90)

                logger.info(f"splat stdout:\n{result.stdout}")
                if result.stderr:
                    logger.info(f"splat stderr:\n{result.stderr}")

                if result.returncode != 0:
                    raise RuntimeError(
                        f"splat failed (rc={result.returncode})\n"
                        f"stdout: {result.stdout}\nstderr: {result.stderr}"
                    )

                # Fall back to any tif file if the expected one isn't present
                if not os.path.exists(os.path.join(tmpdir, img_filename)):
                    candidates = [f for f in os.listdir(tmpdir) if f.endswith(".tif")]
                    logger.info(f"'{img_filename}' not found — tmpdir: {os.listdir(tmpdir)}")
                    if not candidates:
                        raise RuntimeError(f"No GeoTIFF output found. tmpdir: {os.listdir(tmpdir)}")
                    img_filename = candidates[0]
                    logger.info(f"Using output file: {img_filename}")

                with open(os.path.join(tmpdir, img_filename), "rb") as f:
                    geotiff = f.read()

                report(100)
                logger.info("Coverage prediction completed.")
                return geotiff

            except Exception as e:
                logger.error(f"Coverage prediction error: {e}")
                raise RuntimeError(f"Coverage prediction error: {e}")

    # ------------------------------------------------------------------
    # Terrain tile helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _copernicus_filename(latitude: float, longitude: float, high_resolution: bool = True) -> str:
        """Generate the Copernicus DEM filename."""
        res = "10" if high_resolution else "30"
        lat_val = int(math.floor(latitude))
        lon_val = int(math.floor(longitude))
        ns = "N" if lat_val >= 0 else "S"
        ew = "E" if lon_val >= 0 else "W"
        return f"Copernicus_DSM_COG_{res}_{ns}{abs(lat_val):02d}_00_{ew}{abs(lon_val):03d}_00_DEM.tif"
    
    @staticmethod
    def _calculate_required_terrain_tiles(
        lat: float, lon: float, radius: float,
        high_resolution: bool = False
    ) -> List[Tuple[str]]:
        """
        Return the list of copernicus_filename tuples
        covering the bounding box defined by lat/lon/radius.
        """
        earth_radius = 6378137
        delta_deg = (radius / earth_radius) * (180 / math.pi)
        lat_min = math.floor(lat - delta_deg)
        lat_max = math.floor(lat + delta_deg)
        lon_min = math.floor(lon - delta_deg / math.cos(math.radians(lat)))
        lon_max = math.floor(lon + delta_deg / math.cos(math.radians(lat)))

        tiles = []
        for lat_tile in range(lat_min, lat_max + 1):
            for lon_tile in range(lon_min, lon_max + 1):
                copernicus = Splat._copernicus_filename(lat_tile, lon_tile, high_resolution)
                tiles.append((copernicus))

        logger.debug(f"Required tiles: {tiles}")
        return tiles


    def _ensure_dem(self, tile_name: str, high_resolution: bool = False) -> str:
        """
        Return the path to the DEM file for the given Copernicus tile, downloading and
        converting it if not already present in dem_dir.
        """
        copernicus_filename = tile_name
        copernicus_path = os.path.join(self.dem_dir, copernicus_filename)

        if os.path.exists(copernicus_path):
            logger.info(f"DEM hit: {copernicus_filename}")
            return copernicus_path

        # Download Copernicus tif tile
        tile_dir = tile_name[:-4] # remove .tif

        if high_resolution: 
            base_url = f"https://{self.bucket_name_high_resolution}.s3.amazonaws.com"
            logger.info(f"high: {base_url}")
        else:
            base_url = f"https://{self.bucket_name}.s3.amazonaws.com"
            logger.info(f"low: {base_url}")
        tile_data = None
        url = f"{base_url}/{tile_dir}/{tile_name}"
        logger.info(f"Downloading {url}")
        resp = http.get(url, timeout=60)
        if resp.status_code == 200:
            tile_data = resp.content
        if resp.status_code != 404:
            resp.raise_for_status()
        if tile_data is None:
            raise FileNotFoundError(f"Terrain tile '{tile_name}' not found in S3.")

        with open(copernicus_path, "wb") as f:
            f.write(tile_data)
        logger.info(f"Stored {copernicus_filename} in {self.dem_dir}")
        return copernicus_path

    # ------------------------------------------------------------------
    # SPLAT! config file generators
    # ------------------------------------------------------------------

    @staticmethod
    def _create_splat_dcf(colormap_name: str, min_dbm: float, max_dbm: float) -> bytes:
        """Generate a SPLAT! .dcf signal level color definition file (32 levels)."""
        dbm_values = np.linspace(max_dbm, min_dbm, 32)
        rgb = _apply_colormap(dbm_values, colormap_name, min_dbm, max_dbm)
        lines = ["; SPLAT! Signal Level Color Definition\n;\n; dBm: R, G, B\n;\n"]
        for val, color in zip(dbm_values, rgb):
            lines.append(f"{int(val):+4d}: {color[0]:3d}, {color[1]:3d}, {color[2]:3d}\n")
        return "".join(lines).encode()


if __name__ == "__main__":
    logging.basicConfig(level=logging.DEBUG)
    splat_service = Splat(splat_path=".")
