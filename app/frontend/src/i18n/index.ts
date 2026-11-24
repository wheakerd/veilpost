import { computed, ref, watchEffect } from 'vue'
import { createI18n } from 'vue-i18n'
import zhCN, { type MessageSchema } from '@/locales/zh-CN'
import en from '@/locales/en'
import zhTW from '@/locales/zh-TW'
import ru from '@/locales/ru'
import { defaultLocale, matchLocale, type Locale } from './locales'

export { supportedLocales, isLocale, type Locale } from './locales'

declare module 'vue-i18n' {
    // Types the keys accepted by t() everywhere in the application. An
    // interface is required here for declaration merging.
    // eslint-disable-next-line @typescript-eslint/no-empty-object-type
    export interface DefineLocaleMessage extends MessageSchema {}
}

export const messages: Record<Locale, MessageSchema> = {
    'zh-CN': zhCN,
    en,
    'zh-TW': zhTW,
    ru,
}

// Russian has three plural forms: one (1, 21), few (2–4, 22) and many (5, 11).
function russianPlural(choice: number, choicesLength: number) {
    if (choicesLength < 3)
        return Math.min(choice === 1 ? 0 : 1, choicesLength - 1)
    const category = new Intl.PluralRules('ru').select(Math.abs(choice))
    return category === 'one' ? 0 : category === 'few' ? 1 : 2
}

export const i18n = createI18n({
    legacy: false,
    locale: defaultLocale,
    // Missing keys fall back to Simplified Chinese; development builds warn.
    fallbackLocale: defaultLocale,
    messages,
    pluralRules: { ru: russianPlural },
    missingWarn: import.meta.env.DEV,
    fallbackWarn: import.meta.env.DEV,
})

export type ErrorCode = keyof MessageSchema['errors']

export function isErrorCode(code: string): code is ErrorCode {
    return Object.hasOwn(zhCN.errors, code)
}

/** The current interface language as a reactive value. */
export const currentLocale = computed(() => i18n.global.locale.value)

const browserLanguages = ref<readonly string[]>([])
/** The language matched from the browser, used when no choice is saved. */
export const browserLocale = computed(() => matchLocale(browserLanguages.value))

function readBrowserLanguages() {
    browserLanguages.value = navigator.languages.length
        ? [...navigator.languages]
        : [navigator.language]
}

function setDescription(content: string) {
    let meta = document.head.querySelector<HTMLMetaElement>(
        'meta[name="description"]',
    )
    if (!meta) {
        meta = document.createElement('meta')
        meta.name = 'description'
        document.head.append(meta)
    }
    meta.content = content
}

/**
 * Applies the saved language, or the browser's language when none is saved,
 * to vue-i18n, <html lang> and the page description.
 */
export function installLocale(preferences: { locale?: Locale }) {
    readBrowserLanguages()
    window.addEventListener('languagechange', readBrowserLanguages)
    const stop = watchEffect(() => {
        const locale = preferences.locale ?? browserLocale.value
        i18n.global.locale.value = locale
        document.documentElement.lang = locale
        setDescription(i18n.global.t('common.app.description'))
    })

    if (import.meta.webpackHot) {
        import.meta.webpackHot.dispose(() => {
            stop()
            window.removeEventListener('languagechange', readBrowserLanguages)
        })
    }
}

export function localeName(locale: Locale) {
    return messages[locale].common.language.self
}
