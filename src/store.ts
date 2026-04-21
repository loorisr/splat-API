import { defineStore } from 'pinia';
// import { useLocalStorage } from '@vueuse/core';
import { randanimalSync } from 'randanimal';
import L from 'leaflet';
import GeoRasterLayer from 'georaster-layer-for-leaflet';
import parseGeoraster from 'georaster';
import 'leaflet-easyprint';
import { type Site, type SplatParams } from './types.ts';
import { cloneObject } from './utils.ts';
import { redPinMarker } from './layers.ts';

const API_COLORMAPS = new Set(['heat', 'jet', 'turbo', 'viridis', 'magma', 'plasma', 'inferno', 'hot', 'parula', 'gray', 'hsv', 'cubehelix', 'cividis', 'github']);
const RASTER_LAYER_RESOLUTION = 128;

const useStore = defineStore('store', {
  state() {
    return {
      map: undefined as undefined | L.Map,
      currentMarker: undefined as undefined | L.Marker,
      localSites: [] as Site[], //useLocalStorage('localSites', ),
      simulationState: 'idle',
      splatParams: <SplatParams>{
        transmitter: {
          name: randanimalSync(),
          tx_lat: 45.5,
          tx_lon: 6.0,
          tx_power: 0.1,
          tx_freq: 145.5,
          tx_height: 2.0,
          tx_gain: 2.0
        },
        receiver: {
          rx_sensitivity: -130.0,
          rx_height: 1.0,
          rx_gain: 2.0,
          rx_loss: 2.0
        },
        environment: {
          radio_climate: 'continental_temperate',
          polarization: 'vertical',
          clutter_height: 1.0,
          ground_dielectric: 15.0,
          ground_conductivity: 0.005
        },
        simulation: {
          situation_fraction: 95.0,
          time_fraction: 95.0,
          simulation_extent: 30.0,
          high_resolution: false,
          fast_option: false,
          dh_option: false
        },
        display: {
          color_scale: 'plasma',
          min_dbm: -130.0,
          max_dbm: -80.0,
          overlay_transparency: 50
        },
      }
    }
  },
  actions: {
    setTxCoords(lat: number, lon: number) {
      this.splatParams.transmitter.tx_lat = lat
      this.splatParams.transmitter.tx_lon = lon
    },
    removeSite(index: number) {
      if (!this.map) {
        return
      }
      const [removedSite] = this.localSites.splice(index, 1)
      if (removedSite?.rasterLayer && this.map.hasLayer(removedSite.rasterLayer)) {
        this.map.removeLayer(removedSite.rasterLayer as L.Layer);
      }
      this.redrawSites()
    },
    toggleSiteVisibility(index: number) {
      const site = this.localSites[index];
      if (!site) {
        return;
      }
      site.visible = !site.visible;
      this.redrawSites();
    },
    redrawSites() {
      if (!this.map) {
        return;
      }

      this.localSites.forEach((site: Site) => {
        const opacity = Math.max(0, Math.min(1, (100 - site.params.display.overlay_transparency) / 100));

        if (!site.rasterLayer) {
          const hasAlphaBand =
            Array.isArray(site.raster?.mins) &&
            site.raster.mins.length >= 4;

          site.rasterLayer = new GeoRasterLayer({
            georaster: site.raster,
            opacity,
            resolution: RASTER_LAYER_RESOLUTION,
            // Preserve alpha from RGBA GeoTIFFs returned by the API.
            // This avoids transparent pixels being rendered as gray.
            pixelValuesToColorFn: hasAlphaBand
              ? ((values: number[]) => {
                  if (!values || values.length < 4) {
                    return null;
                  }
                  const [r, g, b, a] = values;
                  if (!Number.isFinite(a) || a <= 0) {
                    return null;
                  }
                  return `rgba(${r}, ${g}, ${b}, ${a / 255})`;
                }) as any
              : undefined,
          });
        }

        const layer = site.rasterLayer as any;
        if (typeof layer.setOpacity === "function") {
          layer.setOpacity(opacity);
        }

        if (site.visible) {
          if (!this.map!.hasLayer(layer)) {
            layer.addTo(this.map as L.Map);
          }
          if (typeof layer.bringToFront === "function") {
            layer.bringToFront();
          }
        } else if (this.map!.hasLayer(layer)) {
          this.map!.removeLayer(layer);
        }
      });
    },
    initMap() {     
      this.map = L.map("map", {
        // center: [51.102167, -114.098667],
        zoom: 10,
        zoomControl: false,
      });
      const position: [number, number] = [this.splatParams.transmitter.tx_lat, this.splatParams.transmitter.tx_lon];
      this.map.setView(position, 10);

      L.control.zoom({ position: "bottomleft" }).addTo(this.map as L.Map);

      const cartoLight = L.tileLayer('https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png', {
        attribution: '© OpenStreetMap contributors © CARTO',
      });

      const streetLayer = L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '© OpenStreetMap contributors',
      })

      const satelliteLayer = L.tileLayer('https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}', {
        attribution: 'Tiles © Esri — Source: Esri, USGS, NOAA',
      });

      const topoLayer = L.tileLayer('https://{s}.tile.opentopomap.org/{z}/{x}/{y}.png', {
        attribution: 'Map data: © OpenStreetMap contributors, SRTM | OpenTopoMap',
      });

      streetLayer.addTo(this.map as L.Map);

      // Base Layers
      const baseLayers = {
        "OSM": streetLayer,
        "Carto Light": cartoLight,
        "Satellite": satelliteLayer,
        "Topo Map": topoLayer
      };

      // EasyPrint control
      (L as any).easyPrint({
        title: "Save",
        position: "bottomleft",
        sizeModes: ["A4Portrait", "A4Landscape"],
        filename: "sites",
        exportOnly: true
      }).addTo(this.map as L.Map);

      L.control.layers(baseLayers, {}, {
        position: "bottomleft",
      }).addTo(this.map as L.Map);

      this.map.on("baselayerchange", () => {
        this.redrawSites(); // Re-apply the GeoRasterLayer on top
      });
      this.currentMarker = L.marker(position, { icon: redPinMarker }).addTo(this.map as L.Map).bindPopup("Transmitter site"); // Variable to hold the current marker
      this.redrawSites();
    },
    async runSimulation() {
      console.log('Simulation running...')
      try {
        const txPowerWatts = Number(this.splatParams.transmitter.tx_power);
        if (!Number.isFinite(txPowerWatts) || txPowerWatts <= 0) {
          this.simulationState = 'failed';
          throw new Error("Transmitter power must be greater than 0 W.");
        }

        const selectedColormap = this.splatParams.display.color_scale;
        const colormap = API_COLORMAPS.has(selectedColormap) ? selectedColormap : 'heat';

        // Collect input values
        const payload = {
          // Transmitter parameters
          lat: this.splatParams.transmitter.tx_lat,
          lon: this.splatParams.transmitter.tx_lon,
          tx_height: this.splatParams.transmitter.tx_height,
          tx_power: 10 * Math.log10(txPowerWatts) + 30,
          tx_gain: this.splatParams.transmitter.tx_gain,
          frequency_mhz: this.splatParams.transmitter.tx_freq,

          // Receiver parameters
          rx_height: this.splatParams.receiver.rx_height,
          rx_gain: this.splatParams.receiver.rx_gain,
          signal_threshold: this.splatParams.receiver.rx_sensitivity,
          system_loss: this.splatParams.receiver.rx_loss,

          // Environment parameters
          clutter_height: this.splatParams.environment.clutter_height,
          ground_dielectric: this.splatParams.environment.ground_dielectric,
          ground_conductivity: this.splatParams.environment.ground_conductivity,
          radio_climate: this.splatParams.environment.radio_climate,
          polarization: this.splatParams.environment.polarization,

          // Simulation parameters
          radius: this.splatParams.simulation.simulation_extent * 1000,
          situation_fraction: Math.max(2, this.splatParams.simulation.situation_fraction),
          time_fraction: Math.max(2, this.splatParams.simulation.time_fraction),
          high_resolution: this.splatParams.simulation.high_resolution,
          fast: this.splatParams.simulation.fast_option,
          dh: this.splatParams.simulation.dh_option,

          // Display parameters
          colormap,
          min_dbm: this.splatParams.display.min_dbm,
          max_dbm: this.splatParams.display.max_dbm,
        };
    
        console.log("Payload:", payload);
        this.simulationState = 'running';
    
        // Send the request to the backend's /predict endpoint
        const predictResponse = await fetch("/predict", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify(payload),
        });
    
        if (!predictResponse.ok) {
          this.simulationState = 'failed';
          const errorDetails = await predictResponse.text();
          throw new Error(`Failed to start prediction: ${errorDetails}`);
        }
    
        const predictData = await predictResponse.json();
        const taskId = predictData.task_id;
    
        console.log(`Prediction started with task ID: ${taskId}`);

        // Poll for task status and result
        const pollInterval = 1000; // 1 seconds
        const pollStatus = async () => {
          const statusResponse = await fetch(
            `/status/${taskId}`,
          );
          if (!statusResponse.ok) {
            throw new Error("Failed to fetch task status.");
          }
    
          const statusData = await statusResponse.json();
          console.log("Task status:", statusData);
    
          if (statusData.status === "completed") {
            this.simulationState = 'completed';
            console.log("Simulation completed! Adding result to the map...");

            // Fetch the GeoTIFF data
            const resultResponse = await fetch(
              `/result/${taskId}`,
            );
            if (!resultResponse.ok) {
              throw new Error("Failed to fetch simulation result.");
            }
            else
            {
              const arrayBuffer = await resultResponse.arrayBuffer();
              const geoRaster = await parseGeoraster(arrayBuffer);
              this.localSites.push({
                params: cloneObject(this.splatParams),
                taskId,
                raster: geoRaster,
                visible: true,
                rasterLayer: undefined
              });
              this.currentMarker!.removeFrom(this.map as L.Map);
              this.splatParams.transmitter.name = await randanimalSync();
              this.redrawSites();
            }
          }
          else if (statusData.status === "failed") {
            this.simulationState = 'failed';
          } else {
            setTimeout(pollStatus, pollInterval); // Retry after interval
          }
        };
    
        pollStatus(); // Start polling
      } catch (error) {
        console.error("Error:", error);
      }
    }
  }
});

export { useStore }
