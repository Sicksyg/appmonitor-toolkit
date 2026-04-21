<template>
    <section>
        <header class="view-header view-header-split">
            <div class="view-header-copy">
                <h1 class="headline-md">iOS App Analysis</h1>
                <p>Static analysis of iOS applications</p>
            </div>
            <button class="btn ios start-analysis-btn btn-icon" type="button" @click="handleStartAnalysis">
                <AppIcon name="play" :size="16" />
                Start Analysis
            </button>
        </header>

        <!-- App Search and Selection -->
        <section class="glass-panel">
            <h3>App discovery</h3>
            <div class="input-group">
                <label for="ios-search">Search App Store or select from device</label>
                <div class="search-row">
                    <input id="ios-search" v-model="searchTerm" class="field" :placeholder="placeholder" type="text"
                        @keyup.enter="handleSearchWild" />
                    <button class="btn ios btn-icon" type="button" @click="handleSearchWild" :disabled="loading">
                        <AppIcon name="search" :size="16" />
                        {{ loading ? 'Searching...' : 'Search' }}
                    </button>
                </div>
            </div>

            <div class="action-row wrap">
                <button class="btn ios btn-icon" type="button" @click="handleLoadFromPhone" :disabled="loading">
                    <AppIcon name="device" :size="16" />
                    Load App from Phone
                </button>
                <button class="btn tertiary" type="button" @click="handleLoadAppList">Load App List</button>
            </div>
        </section>

        <!-- Analysis Panel and report open -->
        <section class="glass-panel" v-if="analysisHasRun">
            <h3>Analysis</h3>
            <p class="status-line" v-if="statusMessage">{{ statusMessage }}</p>
            <div class="analysis-progress" role="progressbar" aria-valuemin="0" aria-valuemax="100"
                :aria-valuenow="analysisPercent">
                <div class="analysis-progress-bar" :style="{ width: `${analysisPercent}%` }" />
            </div>

            <p class="status-line" v-if="statusMessage === 'Analysis complete'">The analysis is complete. You can now
                open the generated report.</p>
            <div class="action-row" v-if="statusMessage === 'Analysis complete'">
                <button class="btn primary" type="button" @click="openReport">Open Report</button>
            </div>

        </section>

        <!-- Search Results table -->
        <section class="glass-panel results-panel">
            <div class="results-head">
                <h3>Search results</h3>
                <p>{{ results.length }} item{{ results.length === 1 ? '' : 's' }}</p>
            </div>

            <p v-if="resultsMessage" class="status-line">{{ resultsMessage }}</p>

            <div class="results-wrap" v-else-if="results.length">
                <table class="results-table">
                    <thead>
                        <tr>
                            <th>Logo</th>
                            <th>Title</th>
                            <th>Info</th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr v-for="item in results" :key="`${item.trackId}-${item.bundleId}`" class="result-row"
                            @click="handleSelectItem(item)">
                            <td>
                                <img v-if="item.artworkUrl60" :src="item.artworkUrl60" :alt="`${item.trackName} logo`"
                                    class="result-logo" />
                            </td>
                            <td>{{ item.trackName }}</td>
                            <td class="row-muted">{{ item.bundleId }} - {{ item.sellerName }}</td>
                        </tr>
                    </tbody>
                </table>
            </div>

            <p v-else class="status-line">No results yet.</p>
        </section>

        <!-- phone status info -->
        <section class="glass-panel">
            <header class="view-header view-header-split">
                <h3>Device Connection Status</h3>
                <div class="device-connection-pill" :class="isDeviceConnected ? 'online' : 'offline'">
                    <AppIcon name="device" :size="24" :color="isDeviceConnected ? '#38d39f' : '#ff6b6b'" />
                    <span>{{ isDeviceConnected ? 'Connected' : 'Disconnected' }}</span>
                </div>
            </header>

            <div class="results-wrap device-wrap">
                <table class="results-table device-table">
                    <tbody>
                        <tr>
                            <th>Device Name</th>
                            <td>{{ deviceInfo.DeviceName || '-' }}</td>
                        </tr>
                        <tr>
                            <th>Model</th>
                            <td>{{ deviceInfo.Model || '-' }}</td>
                        </tr>
                        <tr>
                            <th>OS Version</th>
                            <td>{{ deviceInfo.OSVersion || '-' }}</td>
                        </tr>
                        <tr>
                            <th>UDID</th>
                            <td class="udid-cell">
                                <span class="udid-value">{{ deviceInfo.Udid || '-' }}</span>
                                <button class="btn tertiary btn-copy" type="button" @click="copyUdid"
                                    :disabled="!deviceInfo.Udid">
                                    {{ copyState === 'done' ? 'Copied' : copyState === 'error' ? 'Failed' : 'Copy' }}
                                </button>
                            </td>
                        </tr>
                    </tbody>
                </table>
            </div>
        </section>

    </section>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref, computed } from 'vue'
import { EventsOn, EventsOff, ClipboardSetText } from '../../wailsjs/runtime/runtime'
import { LoadAppList, LoadFromPhone, SearchWild, SelectItem, StartAnalysis, OpenReportFileInDefaultApp } from '../../wailsjs/go/main/App'
import AppIcon from '../components/AppIcon.vue'

// --- State ---
const searchTerm = ref('')
const placeholder = ref('Example: Netflix')
const loading = ref(false)
const statusMessage = ref('')       // analysis lifecycle messages from Go backend
const resultsMessage = ref('')      // feedback shown inside the results panel
const analysisPercent = ref(0)      // drives the progress bar (0–100)
const results = ref([])             // flat array of iTunes result objects
const analysisHasRun = ref(false)   // keeps the status panel visible until the next analysis starts

// Device info structure, updated via events from helpers.go
const deviceInfo = ref({
    DeviceName: '',
    Model: '',
    OSVersion: '',
    Udid: '',
    Connected: false,
})
const copyState = ref('idle') // idle | done | error

const isDeviceConnected = computed(() => deviceInfo.value.Connected === true)

onMounted(() => {
    EventsOn('analysisStatus', (status) => {
        statusMessage.value = status?.message ?? ''
        analysisPercent.value = Math.max(0, Math.min(100, Number(status?.percent ?? 0)))
        analysisHasRun.value = true
    })
    EventsOn('deviceInfo', (info) => {
        handleDeviceInfoUpdate(info)
    })
})

onBeforeUnmount(() => {
    EventsOff('analysisStatus')
    EventsOff('deviceInfo')
})

function handleDeviceInfoUpdate(info) {
    deviceInfo.value = {
        DeviceName: info?.DeviceName ?? '',
        Model: info?.Model ?? '',
        OSVersion: info?.OSVersion ?? '',
        Udid: info?.Udid ?? '',
        Connected: info?.Connected === 'true',
    }
}

async function copyUdid() {
    if (!deviceInfo.value.Udid) {
        return
    }

    try {
        const ok = await ClipboardSetText(deviceInfo.value.Udid)
        copyState.value = ok ? 'done' : 'error'
    } catch (_error) {
        copyState.value = 'error'
    } finally {
        setTimeout(() => {
            copyState.value = 'idle'
        }, 1400)
    }
}

// Normalises the backend response — can be a single iTunes response or an array of them
function parseResults(raw) {
    try {
        const parsed = typeof raw === 'string' ? JSON.parse(raw) : raw
        if (Array.isArray(parsed)) {
            return parsed.flatMap((entry) => (Array.isArray(entry?.results) ? entry.results : []))
        }
        return Array.isArray(parsed?.results) ? parsed.results : []
    } catch (error) {
        console.error('Failed to parse search results:', error)
        return []
    }
}

// --- Handlers ---
async function handleSearchWild() {
    if (!searchTerm.value.trim()) {
        resultsMessage.value = 'Enter a search term first.'
        return
    }

    loading.value = true
    resultsMessage.value = 'Searching...'
    try {
        const raw = await SearchWild(searchTerm.value.trim())
        results.value = parseResults(raw)
        resultsMessage.value = results.value.length ? '' : 'No results found.'
    } catch (error) {
        console.error(error)
        resultsMessage.value = 'Search failed.'
    } finally {
        loading.value = false
    }
}

async function handleLoadFromPhone() {
    loading.value = true
    resultsMessage.value = 'Loading from phone...'
    try {
        const raw = await LoadFromPhone()
        results.value = parseResults(raw)
        resultsMessage.value = results.value.length ? '' : 'No results found.'
    } catch (error) {
        console.error(error)
        resultsMessage.value = 'Load from phone failed.'
    } finally {
        loading.value = false
    }
}

// Tells Go which app is selected (sets BundleID + Name), then clears the results table
async function handleSelectItem(item) {
    try {
        await SelectItem(item.trackName, item.trackId, item.bundleId)
        searchTerm.value = `${item.trackName} (${item.bundleId})`
        placeholder.value = item.trackName
        results.value = []
        resultsMessage.value = ''
    } catch (error) {
        console.error(error)
        resultsMessage.value = 'Failed to select app item.'
    }
}

function handleLoadAppList() {
    LoadAppList().catch((error) => {
        console.error(error)
        statusMessage.value = 'Failed to load app list.'
    })
}

// Fires the full analysis pipeline (download → Frida → report); progress via events
function handleStartAnalysis() {
    analysisHasRun.value = false
    statusMessage.value = ''
    analysisPercent.value = 0
    StartAnalysis().catch((error) => {
        console.error(error)
        statusMessage.value = 'Failed to start analysis.'
        analysisHasRun.value = true
    })
}

// Opens the generated report in the default browser
async function openReport() {
    try {
        await OpenReportFileInDefaultApp()
    } catch (error) {
        console.error('Failed to open report:', error)
        statusMessage.value = 'Failed to open report.'
    }
}


</script>

<style scoped>
.search-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 12px;
}

.results-panel {
    margin-top: 24px;
}

.results-head {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 12px;
}

.status-line {
    margin-top: 14px;
}

.results-wrap {
    margin-top: 14px;
    overflow-x: auto;
}

.results-table {
    width: 100%;
    border-collapse: collapse;
}

.results-table th,
.results-table td {
    padding: 10px 12px;
    text-align: left;
}

.results-table thead th {
    color: #7f8fb4;
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 0.07em;
    font-weight: 600;
}

.result-row {
    cursor: pointer;
    transition: background-color 120ms ease;
}

.result-row:nth-child(odd) {
    background: rgba(18, 26, 46, 0.82);
}

.result-row:nth-child(even) {
    background: rgba(23, 31, 51, 0.82);
}

.result-row:hover {
    background: rgba(34, 42, 61, 0.95);
}

.result-logo {
    width: 36px;
    height: 36px;
    border-radius: 8px;
    object-fit: cover;
}

.row-muted {
    color: #9eacc8;
}

.analysis-progress {
    margin-top: 14px;
    width: 100%;
    height: 10px;
    border-radius: 999px;
    background: rgba(218, 226, 253, 0.08);
    overflow: hidden;
}

.analysis-progress-bar {
    height: 100%;
    width: 0;
    background: linear-gradient(90deg, var(--primary), var(--primary-container));
    transition: width 180ms ease;
}

.wrap {
    flex-wrap: wrap;
}

@media (max-width: 700px) {
    .search-row {
        grid-template-columns: 1fr;
    }
}
</style>
