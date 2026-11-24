import { defineStore } from 'pinia'
import { preferencesKey, sanitizePreferences } from '@/theme/palettes'

export const usePreferencesStore = defineStore('preferences', {
    state: () => sanitizePreferences(null),
    persist: {
        key: preferencesKey,
        pick: [
            'palette',
            'mode',
            'reducedMotion',
            'locale',
            'sidebarCollapsed',
            'vision',
            'glassTint',
            'glassStrength',
            'pageTransition',
            'sidebarWidth',
            'navMultiple',
            'tabBar',
            'progressBar',
            'pageCache',
        ],
        serializer: {
            // JSON omits undefined values, so only explicit choices are saved.
            serialize: JSON.stringify,
            deserialize: (value) => {
                try {
                    return sanitizePreferences(JSON.parse(value))
                } catch {
                    return sanitizePreferences(null)
                }
            },
        },
    },
})
