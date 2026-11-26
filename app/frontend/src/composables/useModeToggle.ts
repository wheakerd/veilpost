import { computed, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import Sun from '~icons/lucide/sun'
import Moon from '~icons/lucide/moon'
import { usePreferencesStore } from '@/stores/preferences'
import { resolveColorMode, useSystemReducedMotion } from '@/theme'
import type { ColorMode } from '@/theme/palettes'

const icons = { light: Sun, dark: Moon }
let reveal: { transition: ViewTransition; mode: keyof typeof icons } | undefined

export function useModeToggle() {
    const { t } = useI18n()
    const preferences = usePreferencesStore()
    const systemReducedMotion = useSystemReducedMotion()
    const current = computed(() => resolveColorMode(preferences.mode))
    const label = computed(() =>
        t('common.shell.mode', {
            mode: t(`common.shell.modes.${current.value}`),
        }),
    )
    const icon = computed(() => icons[current.value])
    function setMode(next: ColorMode, press?: MouseEvent) {
        const target = resolveColorMode(next)
        reveal?.transition.skipTransition()
        reveal = undefined
        const root = document.documentElement
        root.classList.remove('appearance-reveal')
        if (
            !press ||
            target === current.value ||
            preferences.reducedMotion ||
            systemReducedMotion.value
        ) {
            preferences.mode = next
            return
        }
        const button = press.currentTarget
        async function applyMode() {
            preferences.mode = next
            await nextTick()
            if (button instanceof HTMLButtonElement)
                button.querySelector('svg')?.animate(
                    [
                        {
                            opacity: 0.45,
                            transform: 'rotate(-24deg) scale(0.84)',
                        },
                        { opacity: 1, transform: 'none' },
                    ],
                    {
                        duration: 150,
                        easing: getComputedStyle(button)
                            .getPropertyValue('--app-ease')
                            .trim(),
                    },
                )
        }
        if (!('startViewTransition' in document)) {
            void applyMode()
            return
        }
        const { clientX: x, clientY: y } = press
        const radius = Math.ceil(
            Math.hypot(
                Math.max(x, window.innerWidth - x),
                Math.max(y, window.innerHeight - y),
            ),
        )
        root.style.setProperty('--appearance-origin', `${x}px ${y}px`)
        root.style.setProperty('--appearance-radius', `${radius}px`)
        root.classList.add('appearance-reveal')
        const transition = document.startViewTransition(async () => {
            if (reveal?.transition === transition) await applyMode()
        })
        reveal = { transition, mode: target }
        void transition.finished.finally(() => {
            if (reveal?.transition === transition) {
                reveal = undefined
                root.classList.remove('appearance-reveal')
            }
        })
    }
    function toggle(event?: Event) {
        const next =
            (reveal?.mode ?? current.value) === 'light' ? 'dark' : 'light'
        setMode(
            next,
            event instanceof MouseEvent && event.detail > 0 ? event : undefined,
        )
    }
    return { label, icon, toggle, setMode }
}
