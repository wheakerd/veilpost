import { isLocale, type Locale } from '@/i18n/locales'

// Names live in the language packs under appearance.palettes.<id>. The
// colours are the light-mode primary and secondary from $palettes in
// styles/_themes.scss (which also holds the dark variants and the text colour
// on the primary); the preference chips show the primary as a swatch.
export const palettes = [
    { id: 'abyss', primary: '#4b4ee6', secondary: '#c2289f' },
    { id: 'sky', primary: '#0b96d4', secondary: '#7048e8' },
    { id: 'emerald', primary: '#0ea57d', secondary: '#2f6fe8' },
    { id: 'mist', primary: '#10a196', secondary: '#e2533f' },
    { id: 'amber', primary: '#d47b00', secondary: '#5b46e0' },
    { id: 'coral', primary: '#f3604a', secondary: '#0b8f8a' },
    { id: 'rose', primary: '#dc2a66', secondary: '#8a3ff0' },
    { id: 'violet', primary: '#9b3cf0', secondary: '#0a95c9' },
] as const

export type Palette = (typeof palettes)[number]['id']
// A stored id that no longer exists (lagoon, harbor and heather were retired)
// falls back to this one.
export const defaultPalette: Palette = 'abyss'
export type ColorMode = 'light' | 'dark' | 'system'
export const visions = ['standard', 'colorSafe', 'grayscale'] as const
export type Vision = (typeof visions)[number]
export const pageTransitions = ['fade', 'slide', 'zoom', 'rise'] as const
export type PageTransition = (typeof pageTransitions)[number]
export const preferencesKey = 'veilpost:preferences:v1'
// The key used before the Veilpost name; moved once to preferencesKey.
export const legacyPreferencesKey = 'vpn-console:preferences:v1'

// Carries saved preferences over from the old key, then removes it. Storage
// may be unavailable (private windows, blocked site data); that is not fatal.
export function migrateLegacyPreferences(getStorage: () => Storage) {
    try {
        const storage = getStorage()
        const legacy = storage.getItem(legacyPreferencesKey)
        if (legacy === null) return
        if (storage.getItem(preferencesKey) === null)
            storage.setItem(preferencesKey, legacy)
        storage.removeItem(legacyPreferencesKey)
    } catch {
        // Nothing to carry over.
    }
}

// Glass tint and strength are percentages; the sidebar width is in rem.
export const glassDefaults = { tint: 20, strength: 60 } as const
export const sidebarWidth = { default: 15.5, min: 13, max: 22 } as const

function percent(value: unknown, fallback: number) {
    return typeof value === 'number' && Number.isFinite(value)
        ? Math.min(100, Math.max(0, Math.round(value)))
        : fallback
}

function oneOf<T extends string>(
    options: readonly T[],
    value: unknown,
    fallback: T,
): T {
    return options.includes(value as T) ? (value as T) : fallback
}

export function sanitizePreferences(value: unknown) {
    const data =
        value && typeof value === 'object'
            ? (value as Record<string, unknown>)
            : {}
    const width = data.sidebarWidth
    return {
        palette: oneOf(
            palettes.map((palette) => palette.id),
            data.palette,
            defaultPalette,
        ),
        mode: oneOf<ColorMode>(
            ['light', 'dark', 'system'],
            data.mode,
            'system',
        ),
        reducedMotion: data.reducedMotion === true,
        // Undefined follows the browser language and is never written to storage.
        locale: isLocale(data.locale)
            ? data.locale
            : (undefined as Locale | undefined),
        // Undefined (expanded) is the default and is never written to storage.
        sidebarCollapsed: data.sidebarCollapsed === true ? true : undefined,
        vision: oneOf(visions, data.vision, 'standard'),
        glassTint: percent(data.glassTint, glassDefaults.tint),
        glassStrength: percent(data.glassStrength, glassDefaults.strength),
        pageTransition: oneOf(pageTransitions, data.pageTransition, 'fade'),
        // Undefined keeps the default width and is never written to storage.
        sidebarWidth:
            typeof width === 'number' &&
            width >= sidebarWidth.min &&
            width <= sidebarWidth.max
                ? Math.round(width * 4) / 4
                : (undefined as number | undefined),
        // Several sidebar groups may be open at once; false keeps one open.
        navMultiple: data.navMultiple !== false,
        tabBar: data.tabBar !== false,
        progressBar: data.progressBar !== false,
        pageCache: data.pageCache !== false,
    }
}

export type Preferences = ReturnType<typeof sanitizePreferences>
