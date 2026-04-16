<template>
    <span class="ui-icon" :class="toneClass" :style="iconStyle" role="img" :aria-label="label || undefined"
        :aria-hidden="label ? undefined : 'true'" />
</template>

<script setup>
import { computed } from 'vue'
import iconAndroid from '../assets/images/brand-android.svg'
import iconDevice from '../assets/images/device-mobile.svg'
import iconIos from '../assets/images/brand-apple.svg'
import iconPlay from '../assets/images/player-play.svg'
import iconSearch from '../assets/images/search.svg'
import iconSettings from '../assets/images/settings.svg'
import iconTools from '../assets/images/tool.svg'
import iconGithub from '../assets/images/brand-github.svg'
import IconWorld from '../assets/images/world.svg'

const props = defineProps({
    name: {
        type: String,
        required: true,
    },
    size: {
        type: [Number, String],
        default: 16,
    },
    label: {
        type: String,
        default: '',
    },
    tone: {
        type: String,
        default: 'current',
    },
    color: {
        type: String,
        default: '',
    },
})

const iconMap = {
    android: iconAndroid,
    device: iconDevice,
    ios: iconIos,
    play: iconPlay,
    search: iconSearch,
    settings: iconSettings,
    tools: iconTools,
    github: iconGithub,
    internet: IconWorld
}

const iconSource = computed(() => iconMap[props.name] || iconTools)

const toneClass = computed(() => {
    if (props.color || props.tone === 'current') {
        return null
    }
    return `ui-icon--${props.tone}`
})

const iconStyle = computed(() => {
    const normalizedSize = typeof props.size === 'number' ? `${props.size}px` : props.size
    return {
        width: normalizedSize,
        height: normalizedSize,
        WebkitMaskImage: `url(${iconSource.value})`,
        maskImage: `url(${iconSource.value})`,
        WebkitMaskRepeat: 'no-repeat',
        maskRepeat: 'no-repeat',
        WebkitMaskPosition: 'center',
        maskPosition: 'center',
        WebkitMaskSize: 'contain',
        maskSize: 'contain',
        backgroundColor: props.color || undefined,
    }
})
</script>
