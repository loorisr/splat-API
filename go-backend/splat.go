package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Splat wraps the signalserver binary for RF coverage prediction.
//
// Terrain data is automatically downloaded and cached from AWS Open Data:
// https://registry.opendata.aws/copernicus-dem/
//
// Concurrency is limited to one subprocess at a time via a channel semaphore,
// and DEM tile downloads are serialized per-tile to avoid redundant network requests.
type Splat struct {
	binaryPath               string
	demDir                   string
	antennaDir               string
	bucketName               string
	bucketNameHighResolution string
	maxConcurrentJobs        int
	jobTimeout               time.Duration
	semaphore                chan struct{}          // limits concurrent signalserver processes
	demLocksMu               sync.Mutex
	demLocks                 map[string]*sync.Mutex // per-tile download locks
}

// NewSplat creates a Splat service, verifying the signalserver binary exists
// and creating the DEM cache directory if needed.
//
// Args:
//
//	binaryPath  — directory containing the signalserver binary.
//	demDir      — directory to cache downloaded Copernicus DEM tiles.
//	antennaDir  — directory containing antenna pattern (.az/.el) files.
func NewSplat(binaryPath, demDir, antennaDir string) *Splat {
	// Ensure DEM cache directory exists.
	if err := os.MkdirAll(demDir, 0755); err != nil {
		log.Fatalf("Failed to create DEM directory %s: %v", demDir, err)
	}

	s := &Splat{
		binaryPath:               binaryPath,
		demDir:                   demDir,
		antennaDir:               antennaDir,
		bucketName:               "copernicus-dem-90m",       // standard 3-arcsecond / 90 m tiles
		bucketNameHighResolution: "copernicus-dem-30m",      // optional 1-arcsecond / 30 m tiles
		maxConcurrentJobs:        1,                          // signalserver is CPU/memory heavy
		jobTimeout:               120 * time.Second,
		semaphore:                make(chan struct{}, 1),
		demLocks:                 make(map[string]*sync.Mutex),
	}

	// Validate that the signalserver binary exists at the expected path.
	binaryFile := filepath.Join(binaryPath, "signalserver")
	if _, err := os.Stat(binaryFile); os.IsNotExist(err) {
		log.Fatalf("signalserver binary not found at %s", binaryFile)
	}

	log.Printf("Initialized SPLAT! — dem_dir: %s, max concurrent jobs: %d, timeout: %s",
		demDir, s.maxConcurrentJobs, s.jobTimeout)
	return s
}

// CoveragePrediction executes a SPLAT! coverage prediction and returns the
// resulting GeoTIFF as raw bytes.
//
// Progress is reported via progressCallback (0-100). The method ensures
// required DEM tiles are present, builds the CLI command, runs signalserver
// under the concurrency semaphore, reads the output GeoTIFF, and cleans up.
func (s *Splat) CoveragePrediction(req *CoveragePredictionRequest, progressCallback func(int)) ([]byte, error) {
	report := func(pct int) {
		if progressCallback != nil {
			progressCallback(pct)
		}
	}

	// Unique job ID used as the output filename prefix.
	jobID := newUUID()
	outputBase := filepath.Join(s.demDir, jobID)
	imgPath := outputBase + ".tif"

	// Always clean up the output file regardless of success or failure.
	defer os.Remove(imgPath)

	// Cap radius at 100 km (signalserver limit).
	radius := math.Min(req.Radius, 100000)
	if radius != req.Radius {
		log.Printf("Capping radius from %f m to 100 km", req.Radius)
	}

	// Phase 1 — download required DEM tiles (0-40 % progress).
	requiredTiles := calculateRequiredTerrainTiles(req.Lat, req.Lon, radius, req.HighResolution)
	n := len(requiredTiles)

	for i, tile := range requiredTiles {
		if err := s.ensureDEM(tile, req.HighResolution); err != nil {
			return nil, fmt.Errorf("failed to get DEM tile %s: %w", tile, err)
		}
		report(int(40 * float64(i+1) / float64(n)))
	}

	// Compute ERP (Effective Radiated Power) in watts from dBm + gains.
	// Formula: 10 ** ((tx_power + tx_gain - system_loss - 30) / 10)
	erpWatts := math.Pow(10, (req.TxPower+req.TxGain-*req.SystemLoss-30)/10)

	// Resolve radio climate to integer code.
	climateCode := ClimateMap[req.RadioClimate]
	if climateCode == 0 {
		return nil, fmt.Errorf("unknown radio climate: %s", req.RadioClimate)
	}

	// Phase 2 — build and execute the signalserver command (45-90 % progress).
	binary := filepath.Join(s.binaryPath, "signalserver")
	command := []string{
		binary,
		"-lat", strconv.FormatFloat(req.Lat, 'f', -1, 64),
		"-lon", strconv.FormatFloat(req.Lon, 'f', -1, 64),
		"-txh", strconv.FormatFloat(req.TxHeight, 'f', -1, 64),
		"-cl", strconv.Itoa(climateCode),
		"-terdic", strconv.FormatFloat(*req.GroundDielectric, 'f', -1, 64),
		"-tercon", strconv.FormatFloat(*req.GroundConductivity, 'f', -1, 64),
		"-f", strconv.FormatFloat(req.FrequencyMHz, 'f', -1, 64),
		"-rel", strconv.FormatFloat(req.TimeFraction, 'f', -1, 64),
		"-conf", strconv.FormatFloat(req.SituationFraction, 'f', -1, 64),
		"-erp", strconv.FormatFloat(erpWatts, 'f', -1, 64),
		"-color", req.Colormap,
		"-rxh", strconv.FormatFloat(req.RxHeight, 'f', -1, 64),
		"-rxg", strconv.FormatFloat(req.RxGain, 'f', -1, 64),
		"-R", strconv.FormatFloat(radius/1000.0, 'f', -1, 64), // signalserver expects km
		"-gc", strconv.FormatFloat(req.ClutterHeight, 'f', -1, 64),
		"-o", outputBase,
		"-geotiff",
		"-dbm",
		"-rt", strconv.FormatFloat(req.SignalThreshold, 'f', -1, 64),
		"-dem", s.demDir,
		"-pm", strconv.Itoa(req.PropagationModel),
	}

	// Optional antenna pattern and rotation.
	if req.AntennaPattern != nil && *req.AntennaPattern != "" {
		antPath := filepath.Join(s.antennaDir, *req.AntennaPattern)
		command = append(command, "-ant", antPath)
		if req.AntennaRotation != 0.0 {
			command = append(command, "-rot", strconv.FormatFloat(req.AntennaRotation, 'f', -1, 64))
		}
	}

	// Optional flags.
	if req.HighResolution {
		command = append(command, "-hd")
	}
	if req.FastDeltaHEveryNPoints > 0 {
		command = append(command, "-fast", strconv.Itoa(req.FastDeltaHEveryNPoints))
	}
	if req.DeltaHPoints > 0 {
		command = append(command, "-dh", strconv.Itoa(req.DeltaHPoints))
	}
	if req.Polarization == "horizontal" {
		command = append(command, "-hp") // default is vertical
	}

	report(45)
	log.Printf("Running splat: %v", command)

	// Acquire the concurrency semaphore (blocks if another job is running).
	s.semaphore <- struct{}{}
	defer func() { <-s.semaphore }()

	ctx := exec.Command(command[0], command[1:]...)
	t0 := time.Now()
	stdout, err := ctx.Output()

	elapsed := time.Since(t0)
	log.Printf("splat finished in %.1fs", elapsed.Seconds())

	report(90)

	// Handle subprocess errors.
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("splat failed (rc=%d)\nstdout: %s\nstderr: %s",
				exitErr.ExitCode(), string(stdout), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("splat failed: %w\nstdout: %s", err, string(stdout))
	}

	// Verify the GeoTIFF was actually produced.
	if _, err := os.Stat(imgPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no GeoTIFF output found at '%s'", imgPath)
	}

	// Read the GeoTIFF into memory and return it.
	geotiff, err := os.ReadFile(imgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read GeoTIFF: %w", err)
	}

	report(100)
	log.Println("Coverage prediction completed.")
	return geotiff, nil
}

// ensureDEM returns the path to the DEM tile, downloading it from the
// Copernicus AWS S3 bucket if it is not already cached in demDir.
//
// Downloads are serialized per tile (via per-tile mutexes) so that concurrent
// requests for the same tile never issue duplicate downloads.
func (s *Splat) ensureDEM(tileName string, highResolution bool) error {
	tilePath := filepath.Join(s.demDir, tileName)

	// Fast path — tile already cached.
	if _, err := os.Stat(tilePath); err == nil {
		log.Printf("DEM hit: %s", tileName)
		return nil
	}

	// Acquire (or create) a per-tile lock to serialise downloads.
	s.demLocksMu.Lock()
	if _, ok := s.demLocks[tileName]; !ok {
		s.demLocks[tileName] = &sync.Mutex{}
	}
	tileLock := s.demLocks[tileName]
	s.demLocksMu.Unlock()

	tileLock.Lock()
	defer tileLock.Unlock()

	// Re-check after acquiring the lock — another goroutine may have downloaded it.
	if _, err := os.Stat(tilePath); err == nil {
		log.Printf("DEM hit (after lock): %s", tileName)
		return nil
	}

	// Build the S3 URL. Copernicus DEM tiles are stored as:
	//   s3://<bucket>/<tile_name_without_.tif>/<tile_name>
	tileDir := tileName[:len(tileName)-4]
	bucket := s.bucketName
	if highResolution {
		bucket = s.bucketNameHighResolution
	}
	url := fmt.Sprintf("https://%s.s3.amazonaws.com/%s/%s", bucket, tileDir, tileName)

	// Download the tile.
	log.Printf("Downloading %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("terrain tile '%s' not found in S3", tileName)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}

	f, err := os.Create(tilePath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", tilePath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("failed to write %s: %w", tilePath, err)
	}

	log.Printf("Stored %s in %s", tileName, s.demDir)
	return nil
}

// copernicusFilename generates the standard Copernicus DEM filename for a tile.
//
// Examples:
//
//	Copernicus_DSM_COG_30_N45_00_E006_00_DEM.tif   (90 m, lat 45, lon 6)
//	Copernicus_DSM_COG_10_S33_00_W070_00_DEM.tif   (30 m, lat -33, lon -70)
func copernicusFilename(lat, lon float64, highResolution bool) string {
	res := "30"
	if highResolution {
		res = "10"
	}
	latVal := int(math.Floor(lat))
	lonVal := int(math.Floor(lon))
	ns := "N"
	if latVal < 0 {
		ns = "S"
	}
	ew := "E"
	if lonVal < 0 {
		ew = "W"
	}
	return fmt.Sprintf("Copernicus_DSM_COG_%s_%s%02d_00_%s%03d_00_DEM.tif", res, ns, absInt(latVal), ew, absInt(lonVal))
}

// calculateRequiredTerrainTiles returns the list of Copernicus DEM filenames
// needed to cover the bounding box defined by lat/lon/radius.
//
// Uses a great-circle approximation to compute the bounding box in degrees.
func calculateRequiredTerrainTiles(lat, lon, radius float64, highResolution bool) []string {
	earthRadius := 6378137.0
	deltaDeg := (radius / earthRadius) * (180.0 / math.Pi)
	latMin := int(math.Floor(lat - deltaDeg))
	latMax := int(math.Floor(lat + deltaDeg))
	lonMin := int(math.Floor(lon - deltaDeg/math.Cos(lat*math.Pi/180.0)))
	lonMax := int(math.Floor(lon + deltaDeg/math.Cos(lat*math.Pi/180.0)))

	var tiles []string
	for latTile := latMin; latTile <= latMax; latTile++ {
		for lonTile := lonMin; lonTile <= lonMax; lonTile++ {
			copernicus := copernicusFilename(float64(latTile), float64(lonTile), highResolution)
			tiles = append(tiles, copernicus)
		}
	}
	log.Printf("Required tiles: %v", tiles)
	return tiles
}

// absInt returns the absolute value of x as an int.
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
