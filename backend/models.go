package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// AvailableColormaps lists the 14 supported Matplotlib colormaps for GeoTIFF output.
var AvailableColormaps = []string{
	"heat", "jet", "turbo", "viridis", "magma", "plasma",
	"inferno", "hot", "parula", "gray", "hsv",
	"cubehelix", "cividis", "github",
}

// RadioClimates lists the 7 ITM radio climate options.
var RadioClimates = []string{
	"equatorial",
	"continental_subtropical",
	"maritime_subtropical",
	"desert",
	"continental_temperate",
	"maritime_temperate_land",
	"maritime_temperate_sea",
}

// ClimateMap maps radio climate names to the integer codes expected by signalserver.
var ClimateMap = map[string]int{
	"equatorial":                1,
	"continental_subtropical":   2,
	"maritime_subtropical":      3,
	"desert":                    4,
	"continental_temperate":     5,
	"maritime_temperate_land":   6,
	"maritime_temperate_sea":    7,
}

// CoveragePredictionRequest is the input payload for POST /predict.
//
// It mirrors the Python Pydantic model exactly, including all field names, types,
// default values, and validation constraints. Pointer fields (e.g. GroundDielectric)
// distinguish between "omitted" (nil) and "explicitly set to zero".
type CoveragePredictionRequest struct {
	// Transmitter parameters.
	Lat          float64 `json:"lat"`            // [-90, 90]  — transmitter latitude (degrees)
	Lon          float64 `json:"lon"`            // [-180, 180] — transmitter longitude (degrees)
	TxHeight     float64 `json:"tx_height"`      // >= 1  — TX height above ground (m), default 1
	TxPower      float64 `json:"tx_power"`       // > 0   — TX power (dBm)
	TxGain       float64 `json:"tx_gain"`        // >= 0  — TX antenna gain (dB), default 1
	FrequencyMHz float64 `json:"frequency_mhz"`  // [20, 30000] — operating frequency (MHz), default 905

	// Receiver parameters.
	RxHeight        float64 `json:"rx_height"`        // >= 1 — RX height above ground (m), default 1
	RxGain          float64 `json:"rx_gain"`          // >= 0 — RX antenna gain (dB), default 1
	SignalThreshold float64 `json:"signal_threshold"` // <= 0 — signal cutoff (dBm), default -100
	ClutterHeight   float64 `json:"clutter_height"`   // >= 0 — ground clutter height (m), default 0

	// Environmental parameters.
	GroundDielectric   *float64 `json:"ground_dielectric"`   // >= 1 — ground dielectric constant, default 15.0
	GroundConductivity *float64 `json:"ground_conductivity"` // >= 0 — ground conductivity (S/m), default 0.005

	// Model settings.
	Radius            float64  `json:"radius"`             // >= 1   — max range (m), capped at 100 km, default 1000
	SystemLoss        *float64 `json:"system_loss"`        // >= 0   — system loss (dB), default 0.0
	RadioClimate      string   `json:"radio_climate"`      //        — one of RadioClimates, default "continental_temperate"
	Polarization      string   `json:"polarization"`       //        — "horizontal" or "vertical", default "vertical"
	SituationFraction float64  `json:"situation_fraction"` // [1, 100] — location validity %, default 50
	TimeFraction      float64  `json:"time_fraction"`      // [1, 100] — time validity %, default 90

	// Output settings.
	Colormap string  `json:"colormap"` // one of AvailableColormaps, default "heat"
	MinDbm   float64 `json:"min_dbm"`  // colormap minimum dBm, default -130
	MaxDbm   float64 `json:"max_dbm"`  // colormap maximum dBm, default 0

	// Terrain options.
	HighResolution         bool `json:"high_resolution"`            // use 30 m DEM tiles instead of 90 m, default false
	DeltaHPoints           int  `json:"delta_h_points"`             // >= 0 — ITM delta-H interpolation points, default 0
	FastDeltaHEveryNPoints int  `json:"fast_delta_h_every_n_points"` // >= 0 — ITM fast delta-H interval, default 0

	// Propagation model.
	PropagationModel int `json:"propagation_model"` // [1, 13] — model code, default 1 (ITM)

	// Antenna pattern (optional).
	AntennaPattern  *string `json:"antenna_pattern"`  // antenna file basename (no .az/.el), omit for isotropic
	AntennaRotation float64 `json:"antenna_rotation"` // [0, 360) — antenna rotation (degrees), default 0
}

// UnmarshalJSON implements json.Unmarshaler so that default values are applied
// after the JSON is decoded into the struct.
func (r *CoveragePredictionRequest) UnmarshalJSON(data []byte) error {
	type alias CoveragePredictionRequest
	aux := &struct {
		*alias
	}{
		alias: (*alias)(r),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	r.applyDefaults()
	return nil
}

// applyDefaults fills in zero/missing fields with their documented defaults,
// matching the Pydantic Field(default=...) values from the Python model.
func (r *CoveragePredictionRequest) applyDefaults() {
	if r.TxHeight == 0 {
		r.TxHeight = 1
	}
	if r.TxGain == 0 {
		r.TxGain = 1
	}
	if r.FrequencyMHz == 0 {
		r.FrequencyMHz = 905.0
	}
	if r.RxHeight == 0 {
		r.RxHeight = 1
	}
	if r.RxGain == 0 {
		r.RxGain = 1
	}
	if r.SignalThreshold == 0 {
		r.SignalThreshold = -100
	}
	if r.Radius == 0 {
		r.Radius = 1000.0
	}
	if r.GroundDielectric == nil {
		v := 15.0
		r.GroundDielectric = &v
	}
	if r.GroundConductivity == nil {
		v := 0.005
		r.GroundConductivity = &v
	}
	if r.SystemLoss == nil {
		v := 0.0
		r.SystemLoss = &v
	}
	if r.RadioClimate == "" {
		r.RadioClimate = "continental_temperate"
	}
	if r.Polarization == "" {
		r.Polarization = "vertical"
	}
	if r.SituationFraction == 0 {
		r.SituationFraction = 50
	}
	if r.TimeFraction == 0 {
		r.TimeFraction = 90
	}
	if r.Colormap == "" {
		r.Colormap = "heat"
	}
	if r.PropagationModel == 0 {
		r.PropagationModel = 1
	}
}

// ValidationError describes a single field-level validation failure.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors is a collection of ValidationError entries returned when
// multiple fields fail validation simultaneously.
type ValidationErrors []ValidationError

func (ve ValidationErrors) Error() string {
	var msgs []string
	for _, e := range ve {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// Validate checks all fields against their documented constraints and returns
// a ValidationErrors slice if any are violated, or nil if the request is valid.
func (r *CoveragePredictionRequest) Validate() error {
	var errs ValidationErrors

	// Transmitter.
	if r.Lat < -90 || r.Lat > 90 {
		errs = append(errs, ValidationError{"lat", "must be between -90 and 90"})
	}
	if r.Lon < -180 || r.Lon > 180 {
		errs = append(errs, ValidationError{"lon", "must be between -180 and 180"})
	}
	if r.TxHeight < 1 {
		errs = append(errs, ValidationError{"tx_height", "must be >= 1"})
	}
	if r.TxPower <= 0 {
		errs = append(errs, ValidationError{"tx_power", "must be > 0"})
	}
	if r.TxGain < 0 {
		errs = append(errs, ValidationError{"tx_gain", "must be >= 0"})
	}
	if r.FrequencyMHz < 20 || r.FrequencyMHz > 30000 {
		errs = append(errs, ValidationError{"frequency_mhz", "must be between 20 and 30000"})
	}

	// Receiver.
	if r.RxHeight < 1 {
		errs = append(errs, ValidationError{"rx_height", "must be >= 1"})
	}
	if r.RxGain < 0 {
		errs = append(errs, ValidationError{"rx_gain", "must be >= 0"})
	}
	if r.SignalThreshold > 0 {
		errs = append(errs, ValidationError{"signal_threshold", "must be <= 0"})
	}
	if r.ClutterHeight < 0 {
		errs = append(errs, ValidationError{"clutter_height", "must be >= 0"})
	}

	// Environmental.
	if r.GroundDielectric != nil && *r.GroundDielectric < 1 {
		errs = append(errs, ValidationError{"ground_dielectric", "must be >= 1"})
	}
	if r.GroundConductivity != nil && *r.GroundConductivity < 0 {
		errs = append(errs, ValidationError{"ground_conductivity", "must be >= 0"})
	}

	// Model settings.
	if r.Radius < 1 {
		errs = append(errs, ValidationError{"radius", "must be >= 1"})
	}
	if r.SystemLoss != nil && *r.SystemLoss < 0 {
		errs = append(errs, ValidationError{"system_loss", "must be >= 0"})
	}
	if !slices.Contains(RadioClimates, r.RadioClimate) {
		errs = append(errs, ValidationError{"radio_climate", fmt.Sprintf("must be one of: %s", strings.Join(RadioClimates, ", "))})
	}
	if r.Polarization != "horizontal" && r.Polarization != "vertical" {
		errs = append(errs, ValidationError{"polarization", "must be 'horizontal' or 'vertical'"})
	}
	if r.SituationFraction < 1 || r.SituationFraction > 100 {
		errs = append(errs, ValidationError{"situation_fraction", "must be between 1 and 100"})
	}
	if r.TimeFraction < 1 || r.TimeFraction > 100 {
		errs = append(errs, ValidationError{"time_fraction", "must be between 1 and 100"})
	}

	// Output.
	if !slices.Contains(AvailableColormaps, r.Colormap) {
		errs = append(errs, ValidationError{"colormap", fmt.Sprintf("must be one of: %s", strings.Join(AvailableColormaps, ", "))})
	}

	// Terrain / model.
	if r.DeltaHPoints < 0 {
		errs = append(errs, ValidationError{"delta_h_points", "must be >= 0"})
	}
	if r.FastDeltaHEveryNPoints < 0 {
		errs = append(errs, ValidationError{"fast_delta_h_every_n_points", "must be >= 0"})
	}
	if r.PropagationModel < 1 || r.PropagationModel > 13 {
		errs = append(errs, ValidationError{"propagation_model", "must be between 1 and 13"})
	}

	// Antenna.
	if r.AntennaRotation < 0 || r.AntennaRotation >= 360 {
		errs = append(errs, ValidationError{"antenna_rotation", "must be between 0 and 360"})
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}
