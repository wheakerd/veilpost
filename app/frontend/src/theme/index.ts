import { computed, ref, watchEffect } from 'vue'
import type { usePreferencesStore } from '@/stores/preferences'
import type { ColorMode } from './palettes'

const systemDark = ref(false)
const systemReducedMotion = ref(false)
export const useSystemReducedMotion = () =>
    computed(() => systemReducedMotion.value)

export function resolveColorMode(mode: ColorMode): 'light' | 'dark' {
    if (mode !== 'system') return mode
    return systemDark.value ? 'dark' : 'light'
}

export function installTheme(
    preferences: ReturnType<typeof usePreferencesStore>,
) {
    const dark = window.matchMedia('(prefers-color-scheme: dark)')
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)')
    systemDark.value = dark.matches
    systemReducedMotion.value = reducedMotion.matches
    const onDarkChange = (event: MediaQueryListEvent) => {
        systemDark.value = event.matches
    }
    const onMotionChange = (event: MediaQueryListEvent) => {
        systemReducedMotion.value = event.matches
    }
    dark.addEventListener('change', onDarkChange)
    reducedMotion.addEventListener('change', onMotionChange)

    const stop = watchEffect(() => {
        const root = document.documentElement
        const mode = resolveColorMode(preferences.mode)
        root.dataset.theme = preferences.palette
        root.classList.toggle('dark', mode === 'dark')
        root.style.colorScheme = mode
        root.dataset.reducedMotion = String(
            preferences.reducedMotion || systemReducedMotion.value,
        )
        root.dataset.vision = preferences.vision
        // Glass for drawers, dialogs, menus and the top bar: tint mixes in
        // the primary colour, strength lowers opacity and raises the blur.
        root.style.setProperty(
            '--glass-tint',
            `${preferences.glassTint * 0.3}%`,
        )
        root.style.setProperty(
            '--glass-opacity',
            `${92 - preferences.glassStrength * 0.32}%`,
        )
        root.style.setProperty(
            '--glass-blur',
            `${4 + preferences.glassStrength * 0.28}px`,
        )
    })

    if (import.meta.webpackHot) {
        import.meta.webpackHot.dispose(() => {
            stop()
            dark.removeEventListener('change', onDarkChange)
            reducedMotion.removeEventListener('change', onMotionChange)
        })
    }
}
