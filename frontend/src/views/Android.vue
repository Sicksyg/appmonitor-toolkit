<template>
    <section>
        <header class="view-header view-header-split">
            <div class="view-header-copy">
                <h1 class="headline-md">Android App Analysis</h1>
                <p>Static analysis of Android applications</p>
            </div>
            <button class="btn android start-analysis-btn btn-icon" type="button" @click="handleStartAnalysis">
                <AppIcon name="play" :size="16" />
                Start Analysis
            </button>
        </header>

        <!-- Search Section, android has no load from phone option -->
        <section class="glass-panel">
            <p class="label-sm">app discovery</p>
            <div class="input-group">
                <label for="android-search">Search Google Play Store or select from device</label>
                <div class="search-row">
                    <input id="android-search" v-model="searchTerm" class="field" placeholder="Example: Netflix"
                        type="text" @keyup.enter="handleSearch" />
                    <button class="btn android btn-icon" type="button" @click="handleSearch" :disabled="loading">
                        <AppIcon name="search" :size="16" />
                        {{ loading ? 'Searching...' : 'Search' }}
                    </button>
                </div>
            </div>

            <p class="status-line" v-if="statusMessage">{{ statusMessage }}</p>
        </section>

        <section class="glass-panel results-panel">
            <div class="results-head">
                <p class="label-sm">search results</p>
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
    </section>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { SearchGooglePlay, SelectItem, StartAnalysis } from '../../wailsjs/go/main/App'
import AppIcon from '../components/AppIcon.vue'

const searchTerm = ref('')
const loading = ref(false)
const statusMessage = ref('')
const resultsMessage = ref('')
const analysisPercent = ref(0)
const results = ref([])

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

async function handleSearch() {
    if (!searchTerm.value.trim()) {
        resultsMessage.value = 'Enter a search term first.'
        return
    }

    loading.value = true
    resultsMessage.value = 'Searching...'
    try {
        const raw = await SearchGooglePlay(searchTerm.value.trim())
        results.value = parseResults(raw)
        resultsMessage.value = results.value.length ? '' : 'No results found.'
    } catch (error) {
        console.error(error)
        resultsMessage.value = 'Search failed.'
    } finally {
        loading.value = false
    }
}

async function handleSelectItem(item) {
    try {
        await SelectItem(item.trackName, item.trackId, item.bundleId)
        searchTerm.value = `${item.trackName} (${item.bundleId})`
        results.value = []
        resultsMessage.value = ''
    } catch (error) {
        console.error(error)
        resultsMessage.value = 'Failed to select app item.'
    }
}

function handleStartAnalysis() {
    StartAnalysis().catch((error) => {
        console.error(error)
        statusMessage.value = 'Failed to start analysis.'
    })
}

onMounted(() => {
    EventsOn('analysisStatus', (status) => {
        statusMessage.value = status?.message ?? ''
        analysisPercent.value = Math.max(0, Math.min(100, Number(status?.percent ?? 0)))
    })
})

onBeforeUnmount(() => {
    EventsOff('analysisStatus')
})
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


.wrap {
    flex-wrap: wrap;
}

@media (max-width: 700px) {
    .search-row {
        grid-template-columns: 1fr;
    }
}
</style>