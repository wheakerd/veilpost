// Interface languages; the order is the order shown in the language selector.
// Kept free of imports so stores and Node tests can use it directly.
export const supportedLocales = ['zh-CN', 'en', 'zh-TW', 'ru'] as const
export type Locale = (typeof supportedLocales)[number]
export const defaultLocale: Locale = 'zh-CN'

export function isLocale(value: unknown): value is Locale {
    return supportedLocales.includes(value as Locale)
}

const traditionalChinese = new Set(['hant', 'tw', 'hk', 'mo'])

/**
 * Picks the interface language from the browser's preferred languages, in
 * order: Traditional Chinese (zh-TW, zh-HK, zh-MO, zh-Hant) → zh-TW, other
 * Chinese → zh-CN, Russian → ru, English → en. Falls back to zh-CN.
 */
export function matchLocale(languages: readonly string[]): Locale {
    for (const language of languages) {
        const [primary, ...subtags] = language.toLowerCase().split(/[-_]/)
        if (primary === 'zh') {
            if (subtags.includes('hans')) return 'zh-CN'
            return subtags.some((subtag) => traditionalChinese.has(subtag))
                ? 'zh-TW'
                : 'zh-CN'
        }
        if (primary === 'ru') return 'ru'
        if (primary === 'en') return 'en'
    }
    return defaultLocale
}
