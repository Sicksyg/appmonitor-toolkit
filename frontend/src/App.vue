<template>
    <div class="app-shell">
        <aside class="flux-sidebar">
            <div>
                <h1 class="brand-title">AppMonitor</h1>
                <p class="brand-subtitle">A toolkit for monitoring third-party tracking across iOS and Android apps</p>
            </div>

            <nav class="nav-stack">
                <RouterLink class="nav-link" to="/ios"><span class="nav-pill ios" />
                    <AppIcon name="ios" :size=22 />iOS
                </RouterLink>
                <RouterLink class="nav-link" to="/android"><span class="nav-pill android" />
                    <AppIcon name="android" :size="22" />Android
                </RouterLink>
                <!-- <RouterLink class="nav-link" to="/utilities"><span class="nav-pill utilities" />
                    <AppIcon name="tools" :size="22" />Utilities
                </RouterLink> -->
                <RouterLink class="nav-link" to="/settings"><span class="nav-pill settings" />
                    <AppIcon name="settings" :size="22" />Settings
                </RouterLink>
            </nav>
            <div class="sidebar-links">
                <a class="sidebar-link-external" href="https://example.com" target="_blank" rel="noreferrer noopener"
                    @click.prevent="handleExternalLink('https://example.com')">
                    <AppIcon name="internet" :size="18" tone="muted" />
                    Website
                </a>
                <a class="sidebar-link-external" href="https://github.com/Sicksyg/appmonitor-toolkit" target="_blank"
                    @click.prevent="handleExternalLink('https://github.com/Sicksyg/appmonitor-toolkit')">
                    <AppIcon name="github" :size="18" tone="muted" />
                    GitHub
                </a>
            </div>
        </aside>

        <section class="main-stack">

            <main class="workspace">
                <RouterView />
            </main>
        </section>
    </div>
</template>

<script setup>
import AppIcon from './components/AppIcon.vue'
import { BrowserOpenURL } from '../wailsjs/runtime/runtime';

function handleExternalLink(url) {
    BrowserOpenURL(url, '_blank', 'noopener,noreferrer')
}


</script>
