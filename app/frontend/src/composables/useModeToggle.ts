import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Sun from '~icons/lucide/sun'
import Moon from '~icons/lucide/moon'
import { usePreferencesStore } from '@/stores/preferences'
import { markThemeOrigin, resolveColorMode } from '@/theme'

const icons = { light: Sun, dark: Moon }

export function useModeToggle() {
    const { t } = useI18n()
    const preferences = usePreferencesStore()
    const current = computed(() => resolveColorMode(preferences.mode))
    const label = computed(() =>
        t('common.shell.mode', {
            mode: t(`common.shell.modes.${current.value}`),
        }),
    )
    const icon = computed(() => icons[current.value])
    function toggle(event?: Event) {
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
        preferences.mode = current.value === 'light' ? 'dark' : 'light'
    }
    return { label, icon, toggle }
}
