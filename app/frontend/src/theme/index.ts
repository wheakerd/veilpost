import { computed, ref, watchEffect } from 'vue'
import type { usePreferencesStore } from '@/stores/preferences'

const systemReducedMotion = ref(false)
export const useSystemReducedMotion = () =>
    computed(() => systemReducedMotion.value)

// The light/dark switch: going dark, the dark view spreads out of a circle at
// the point the user pressed; going light, it gathers back into that point
// and uncovers the light view. The circle runs on the View Transitions API;
// without it, or under reduced motion, the mode switches instantly.
const switchDuration = 560
const switchEasing = 'cubic-bezier(0.22, 1, 0.36, 1)' // --app-ease
// A press older than this is not what started the switch (the segmented
// control's keyboard, the system colour scheme); those start at the centre.
const pressWindow = 1000
const switchTypes = ['theme-expand', 'theme-contract'] as const
type SwitchType = (typeof switchTypes)[number]

let lastPress: { x: number; y: number; time: number } | undefined
let running: ViewTransition | undefined

function recordPress(event: PointerEvent) {
    if (!event.isPrimary || event.button !== 0) return
    lastPress = { x: event.clientX, y: event.clientY, time: performance.now() }
}

// Keyboard activation of a known control (the mode button) starts the circle
// from that control; pointer presses are recorded by recordPress.
export function markThemeOrigin(x: number, y: number) {
    lastPress = { x, y, time: performance.now() }
}

function switchCircle() {
    const width = Math.max(
        window.innerWidth,
        document.documentElement.clientWidth,
    )
    const height = Math.max(
        window.innerHeight,
        document.documentElement.clientHeight,
    )
    const press =
        lastPress && performance.now() - lastPress.time <= pressWindow
            ? lastPress
            : undefined
    const x = press ? Math.min(Math.max(press.x, 0), width) : width / 2
    const y = press ? Math.min(Math.max(press.y, 0), height) : height / 2
    // Reach the farthest viewport corner so the circle covers everything.
    const radius = Math.ceil(
        Math.hypot(Math.max(x, width - x), Math.max(y, height - y)),
    )
    return { x, y, radius }
}

// Chromium 125+ and Safari 18.2+ take an options object with transition
// types; older engines take only the update callback, so a class on <html>
// selects the direction there.
const supportsTypes = () =>
    'ViewTransition' in window && 'types' in window.ViewTransition.prototype

const ignore = () => undefined

function startSwitch(toDark: boolean, apply: () => void) {
    const root = document.documentElement
    // A switch during a running one ends that one at its final state first.
    running?.skipTransition()
    const type: SwitchType = toDark ? 'theme-expand' : 'theme-contract'
    const { x, y, radius } = switchCircle()
    const typed = supportsTypes()
    root.classList.remove(...switchTypes)
    if (!typed) root.classList.add(type)
    const transition = typed
        ? document.startViewTransition({ update: apply, types: [type] })
        : document.startViewTransition(apply)
    running = transition
    // Each promise can reject (skipped, viewport resized, update timeout);
    // none of that may surface as an unhandled rejection.
    transition.updateCallbackDone.catch(ignore)
    let circle: Animation | undefined
    transition.ready
        .then(() => {
            const at = `at ${x}px ${y}px`
            const point = `circle(0px ${at})`
            const cover = `circle(${radius}px ${at})`
            circle = root.animate(
                { clipPath: toDark ? [point, cover] : [cover, point] },
                {
                    duration: switchDuration,
                    easing: switchEasing,
                    // Holds the last frame until teardown; cancelled after.
                    fill: 'forwards',
                    pseudoElement: toDark
                        ? '::view-transition-new(root)'
                        : '::view-transition-old(root)',
                },
            )
        })
        .catch(ignore)
    const settle = () => {
        // A filled animation would otherwise clip the next transition's
        // snapshot of the same pseudo-element.
        circle?.cancel()
        if (running !== transition) return
        running = undefined
        root.classList.remove(...switchTypes)
        // An aborted transition may not have run its update; apply() is
        // idempotent and writes the latest requested mode.
        apply()
    }
    transition.finished.then(settle, settle)
}

export function installTheme(
    preferences: ReturnType<typeof usePreferencesStore>,
) {
    const dark = window.matchMedia('(prefers-color-scheme: dark)')
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)')
    const systemDark = ref(dark.matches)
    systemReducedMotion.value = reducedMotion.matches
    const onDarkChange = (event: MediaQueryListEvent) => {
        systemDark.value = event.matches
    }
    const onMotionChange = (event: MediaQueryListEvent) => {
        systemReducedMotion.value = event.matches
    }
    dark.addEventListener('change', onDarkChange)
    reducedMotion.addEventListener('change', onMotionChange)
    window.addEventListener('pointerdown', recordPress, {
        capture: true,
        passive: true,
    })

    let applied: boolean | undefined
    // The latest requested mode. flip() always writes this one, so a late or
    // repeated update callback cannot bring back an older mode.
    let wantDark = false
    const flip = () => {
        const root = document.documentElement
        root.classList.toggle('dark', wantDark)
        root.style.colorScheme = wantDark ? 'dark' : 'light'
    }
    const stop = watchEffect(() => {
        const root = document.documentElement
        const isDark =
            preferences.mode === 'dark' ||
            (preferences.mode === 'system' && systemDark.value)
        const reduce = preferences.reducedMotion || systemReducedMotion.value
        root.dataset.theme = preferences.palette
        wantDark = isDark
        if (applied === undefined) flip()
        else if (applied !== isDark) {
            if (
                !reduce &&
                'startViewTransition' in document &&
                document.visibilityState === 'visible'
            )
                startSwitch(isDark, flip)
            else {
                running?.skipTransition()
                flip()
            }
        }
        applied = isDark
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
            window.removeEventListener('pointerdown', recordPress, {
                capture: true,
            })
        })
    }
}
