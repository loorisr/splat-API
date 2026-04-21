<template>
    <form novalidate>
        <div class="row g-2">
            <div class="col-6">
                <label for="min_dbm" class="form-label">Minimum dBm</label>
                <input v-model="display.min_dbm" type="number" class="form-control form-control-sm" id="min_dbm" required step="0.1" />
                <div class="invalid-feedback">Minimum dBm must be provided (default: -130.0).</div>
            </div>
            <div class="col-6">
                <label for="max_dbm" class="form-label text-secondary">Maximum dBm</label>
                <input v-model="display.max_dbm" type="number" class="form-control form-control-sm bg-light text-secondary border-secondary-subtle" id="max_dbm" required step="0.1" />
                <div class="invalid-feedback">Maximum dBm must be provided (default: -30.0).</div>
            </div>
        </div>
        <div class="row g-2 mt-2">
            <div class="col-6">
                <label for="color_scale" class="form-label">Color Scale</label>
                <select v-model="display.color_scale" id="color_scale" class="form-select form-select-sm" required>
                    <option value="heat">Heat</option>
                    <option value="jet">Jet</option>
                    <option value="turbo">Turbo</option>
                    <option value="viridis">Viridis</option>
                    <option value="magma">Magma</option>
                    <option value="plasma">Plasma</option>
                    <option value="inferno">Inferno</option>
                    <option value="hot">Hot</option>
                    <option value="parula">Parula</option>
                    <option value="gray">Gray</option>
                    <option value="hsv">HSV</option>
                    <option value="cubehelix">Cubehelix</option>
                    <option value="cividis">Cividis</option>
                    <option value="github">GitHub</option>
                </select>
                <div class="invalid-feedback">Please select a color scale.</div>
            </div>
            <div class="col-6">
                <label for="overlay_transparency" class="form-label">Transparency (%)</label>
                <input v-model="display.overlay_transparency" type="number" class="form-control form-control-sm" id="overlay_transparency" required min="0" max="100" step="1" />
                <div class="invalid-feedback">Transparency must be between 0 and 100 (default: 50).</div>
            </div>
        </div>
    <div class="mt-3 text-center">
      <div>
        <img
          v-if="hasColorbarPreview"
          :src="`/colormaps/${display.color_scale}.png`"
          alt="Colorbar"
          width="256"
          height="30"
          style="border: 1px solid #ccc; display: block; margin: 0 auto;"
        />
        <small v-else class="text-secondary">No local preview image for this colormap.</small>
      </div>
      <div class="d-flex justify-content-between mt-1">
        <span class="badge bg-primary">{{ display.min_dbm }} dBm</span>
        <span class="badge bg-secondary">{{ display.max_dbm }} dBm</span>
      </div>
    </div>
    </form>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useStore } from "../store.ts";
const display = useStore().splatParams.display;
const PREVIEW_COLORMAPS = new Set(["heat", "jet", "turbo", "viridis", "magma", "plasma", "inferno", "hot", "parula", "gray", "hsv", "cubehelix", "cividis", "github"]);
const hasColorbarPreview = computed(() => PREVIEW_COLORMAPS.has(display.color_scale));
</script>
