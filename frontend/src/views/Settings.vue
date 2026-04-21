<template>
    <section>
        <header class="view-header">
            <h1 class="headline-md">Service Configuration</h1>
        </header>


        <!-- Apple AppStore Authentication -->
        <section class="glass-panel">
            <header class="view-header">
                <h3>Apple AppStore Authentication</h3>
                <p>Configure AppleID credentials for App Store access and app discovery.</p>
            </header>
            <div class="input-group">
                <label for="apple-email">AppleID Username (email)</label>
                <input id="apple-email" class="field" placeholder="username@example.com" type="text"
                    v-model="settings.auth.AppleEmail" />
                <label for="apple-password">AppleID Password</label>
                <input id="apple-password" class="field" placeholder="••••••••" type="password"
                    v-model="settings.auth.ApplePassword" />
            </div>
            <div class="action-row">
                <button class="btn ios" type="button" @click="save">Save</button>
                <span v-if="saveStatus" class="save-status">{{ saveStatus }}</span>
            </div>
        </section>


        <!-- Google Play Store Authentication -->
        <section class="glass-panel">
            <header class="view-header">
                <h3>Google Play Store Authentication</h3>
                <p>Configure Google Account credentials for Play Store access.</p>
            </header>
            <div class="input-group">
                <label for="google-email">Google Account Username (email)</label>
                <input id="google-email" class="field" placeholder="username@example.com" type="text"
                    v-model="settings.auth.GoogleEmail" />
                <label for="google-password">Google Account Password</label>
                <input id="google-password" class="field" placeholder="••••••••" type="password"
                    v-model="settings.auth.GooglePassword" />
            </div>
            <div class="action-row">
                <button class="btn android" type="button" @click="save">Save</button>
                <span v-if="saveStatus" class="save-status">{{ saveStatus }}</span>
            </div>
        </section>

        <!-- Report output location -->
        <section class="glass-panel">
            <header class="view-header">
                <h3>Report Output Location</h3>
                <p>Set the directory where generated reports will be saved.</p>
            </header>
            <div class="input-group">
                <label for="report-output-path">Report Output Directory</label>
                <input id="report-output-path" class="field" :value="settings.report.SavePath"
                    :placeholder="reportOutputPlaceholder" type="text" readonly />
            </div>
            <div class="action-row">
                <button class="btn neutral" type="button" @click="pickReportDir">Select Directory</button>
            </div>
        </section>


        <!-- Exodus API Configuration -->
        <section class="glass-panel">
            <header class="view-header">
                <h3>Exodus API Configuration</h3>
                <p>Configure your Exodus API key for seamless integration.</p>
            </header>
            <div class="input-group">
                <label for="exodus-api-key">Exodus API Key</label>
                <input id="exodus-api-key" class="field" placeholder="EXODUS_API_KEY" type="text"
                    v-model="settings.exodusApiKey.Key" />
            </div>
            <div class="action-row">
                <button class="btn neutral" type="button" @click="save">Save API Key</button>
                <span v-if="saveStatus" class="save-status">{{ saveStatus }}</span>
            </div>
        </section>


        <!-- Toggle AppStore options -->
        <section class="glass-panel">
            <header class="view-header">
                <h3>App Store Options</h3>
                <p>Configure options for App Store interactions.</p>
            </header>
            <div class="input-group">
                <label>
                    <input type="checkbox" v-model="settings.options.DownloadFromAppStore" />
                    Download apps directly from App Store
                </label>
                <label>
                    <input type="checkbox" v-model="settings.options.InstallOnDevice" />
                    Install apps on connected device
                </label>
            </div>
            <div class="action-row">
                <button class="btn neutral" type="button" @click="save">Save Options</button>
            </div>
        </section>

        <!-- Advanced, open settings file -->
        <section class="glass-panel">
            <header class="view-header">
                <h3>Advanced Configuration</h3>
                <p>For advanced users, you can directly edit the settings file.</p>
            </header>
            <div class="action-row">
                <button class="btn neutral" type="button" @click="openSettingsDir">Open Settings Directory</button>
            </div>
        </section>
    </section>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { GetSettings, SaveSettings } from '../../wailsjs/go/main/App'
import { SetReportSavePath, OpenSettingsDir } from '../../wailsjs/go/main/App'

const settings = ref({
    auth: { AppleEmail: '', ApplePassword: '' },
    options: { DownloadFromAppStore: true, InstallOnDevice: true },
    report: { SavePath: '' },
    exodusApiKey: { Key: '' }
})
const reportOutputPlaceholder = 'No directory selected yet'
const saveStatus = ref('')

onMounted(async () => {
    try {
        settings.value = await GetSettings()
    } catch (error) {
        console.error('Error loading settings:', error)
    }
})

async function save() {
    try {
        await SaveSettings(settings.value)
        saveStatus.value = 'Saved'
        setTimeout(() => { saveStatus.value = '' }, 2000)
    } catch (error) {
        saveStatus.value = 'Error saving'
        console.error('Error saving settings:', error)
    }
}

async function pickReportDir() {
    try {
        const dir = await SetReportSavePath()
        if (dir) {
            settings.value.report.SavePath = dir
            await SaveSettings(settings.value)
        }
    } catch (error) {
        console.error('Error selecting directory:', error)
    }
}

async function openSettingsDir() {
    try {
        await OpenSettingsDir()
    } catch (error) {
        console.error('Error opening settings directory:', error)
    }
}

</script>