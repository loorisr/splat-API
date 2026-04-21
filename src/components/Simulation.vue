<template>
  <form novalidate>
        <div class="row g-2">
            <div class="col-6">
                <label for="situation_fraction" class="form-label">Situation Fraction (%)</label>
                <input v-model="simulation.situation_fraction" type="number" class="form-control form-control-sm" id="situation_fraction" required min="2" max="100" step="0.1" />
                <div class="invalid-feedback">Percentage must be between 1 and 100 (default: 50).</div>
            </div>
            <div class="col-6">
                <label for="time_fraction" class="form-label">Time Fraction (%)</label>
                <input v-model="simulation.time_fraction" type="number" class="form-control form-control-sm" id="time_fraction" required min="2" max="100" step="0.1" />
                <div class="invalid-feedback">Percentage must be between 1 and 100 (default: 90).</div>
            </div>
        </div>
        <div class="row g-2 mt-2">
            <div class="col-6">
                <label for="simulation_extent" class="form-label">Max Range (km)</label>
                <input v-model="simulation.simulation_extent" type="number" class="form-control form-control-sm" id="simulation_extent" required min="1" max="100" step="1" />
                <div class="invalid-feedback">Radius must be a positive number (default: 30 km).</div>
            </div>
        </div>
        <div class="row mt-3">
            <div class="col-12">
                <div class="form-check">
                    <input v-model="simulation.high_resolution" type="checkbox" class="form-check-input" id="high_resolution" />
                    <label class="form-check-label" for="high_resolution">High resolution terrain</label>
                </div>
            </div>
            <div class="col-12">
                <div class="form-check">
                    <input v-model="simulation.fast_option" type="checkbox" class="form-check-input" id="fast_option" />
                    <label class="form-check-label" for="fast_option">Fast option (`-fast`)</label>
                </div>
            </div>
            <div class="col-12">
                <div class="form-check">
                    <input v-model="simulation.dh_option" type="checkbox" class="form-check-input" id="dh_option" />
                    <label class="form-check-label" for="dh_option">Delta-H option (`-dh`)</label>
                </div>
            </div>
        </div>
    </form>
</template>

<script setup lang="ts">
import { useStore } from '../store.ts'
const simulation = useStore().splatParams.simulation
</script>
