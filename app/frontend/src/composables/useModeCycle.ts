import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Sun from '~icons/lucide/sun'
import Moon from '~icons/lucide/moon'
import SunMoon from '~icons/lucide/sun-moon'
import { usePreferencesStore } from '@/stores/preferences'
import type { ColorMode } from '@/theme/palettes'
import { markThemeOrigin } from '@/theme'

const order: ColorMode[] = ['light', 'dark', 'system']
const icons = { light: Sun, dark: Moon, system: SunMoon }

// One button that steps through light, dark and the system setting.
export function useModeCycle() {
    const { t } = useI18n()
    const preferences = usePreferencesStore()
    const label = computed(() =>
        t('common.shell.mode', {
            mode: t(`common.shell.modes.${preferences.mode}`),
        }),
    )
    const icon = computed(() => icons[preferences.mode])
    function cycle(event?: Event) {
        // A keyboard click (detail 0) starts the light/dark circle from the
        // button; a pointer press is already recorded by the theme.
        if (
            event instanceof MouseEvent &&
            event.detail === 0 &&
            event.currentTarget instanceof Element
        ) {
            const box = event.currentTarget.getBoundingClientRect()
            markThemeOrigin(box.left + box.width / 2, box.top + box.height / 2)
        }
        const index = order.indexOf(preferences.mode)
        preferences.mode = order[(index + 1) % order.length] ?? 'system'
    }
    return { label, icon, cycle }
}
