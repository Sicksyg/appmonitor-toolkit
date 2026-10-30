<template>
    <section>
        <header class="view-header">
            <h1 class="headline-md">Service Configuration</h1>
        </header>

        <!-- Report output location -->
        <section class="glass-panel">
            <header class="view-header">
                <h3>Report Output Location</h3>
                <p>Set the directory where generated reports will be saved.</p>
            </header>
            <div class="input-group">
                <label for="report-output-path">Report Output Directory</label>
                <input id="report-output-path" class="field" :value="settings.Output.ReportSavePath"
                    :placeholder="reportOutputPlaceholder" type="text" readonly />
            </div>
            <div class="action-row">
                <button class="btn neutral" type="button" @click="pickReportDir">Select Directory</button>
                <button class="btn neutral" type="button" @click="openOutputDir">Open output directory</button>
            </div>
        </section>

        <!-- Clear tmp directory of images and other files -->
        <section class="glass-panel">
            <header class="view-header">
                <h3>Clear Temporary Directory</h3>
                <p>Clear the temporary directory used for storing images and other files.</p>
            </header>
            <div class="action-row">
                <button class="btn neutral" type="button" @click="clearTmpDir">Clear Temporary Directory</button>
            </div>
        </section>

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

            <!-- Spacer -->
            <div class="spacer"></div>

            <header class="view-header">
                <h3>Sign in to Apple AppStore</h3>
                <p>Opens Terminal to run <code>ipatool auth login</code>. Enter your Apple ID, password and the 2FA
                    code from your trusted device. The session is stored in your keychain, so this is only needed once.</p>
            </header>
            <div class="action-row">
                <button class="btn ios" type="button" @click="openLogin">Open Terminal to sign in</button>
                <button class="btn ios" type="button" @click="refreshAuth">Check status</button>
                <span v-if="authStatus" class="save-status">{{ authStatus }}</span>
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
                <input id="google-email" class="field" placeholder="username@gmail.com" type="text"
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
import { GetSettings, SaveSettings, AuthenticateAppleID, IsAppleIDAuthenticated } from '../../wailsjs/go/main/App'
import { SetReportSavePath, OpenSettingsDir, OpenOutputDir, ClearTmpDir } from '../../wailsjs/go/main/App'

const settings = ref({
    auth: { AppleEmail: '', ApplePassword: '' },
    options: { DownloadFromAppStore: true, InstallOnDevice: true },
    Output: { ReportSavePath: '' },
    exodusApiKey: { Key: '' }
})
const reportOutputPlaceholder = 'No directory selected yet'
const saveStatus = ref('')
const authStatus = ref('')

onMounted(async () => {
    try {
        settings.value = await GetSettings()
    } catch (error) {
        console.error('Error loading settings:', error)
    }
    await refreshAuth()
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
            settings.value.Output.ReportSavePath = dir
            await SaveSettings(settings.value)
        }
    } catch (error) {
        console.error('Error selecting directory:', error)
    }
}

async function openOutputDir() {
    try {
        await OpenOutputDir()
    } catch (error) {
        console.error('Error opening output directory:', error)
    }
}

async function clearTmpDir() {
    try {
        await ClearTmpDir()
    } catch (error) {
        console.error('Error clearing temporary directory:', error)
    }
}

async function openSettingsDir() {
    try {
        await OpenSettingsDir()
    } catch (error) {
        console.error('Error opening settings directory:', error)
    }
}

async function refreshAuth() {
    try {
        authStatus.value = (await IsAppleIDAuthenticated()) ? 'Signed in (keychain)' : 'Not signed in'
    } catch (error) {
        console.error('Error checking Apple ID session:', error)
    }
}

async function openLogin() {
    try {
        await AuthenticateAppleID()
        authStatus.value = 'Finish signing in in Terminal, then click Check status'
    } catch (error) {
        authStatus.value = 'Could not open Terminal: ' + error
    }
}

</script>