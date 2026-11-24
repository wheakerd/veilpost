import { useI18n } from 'vue-i18n'

/**
 * Translates a key that a catalog manifest names (apps.catalog.<id>.*).
 * Keys come from the server, so one missing from the language packs shows
 * the fallback, such as the app id, instead of the raw key.
 * tests/dev/locales.test.ts checks that every manifest key exists.
 */
export function useCatalogText() {
    const i18n = useI18n()
    return (key: string | null, fallback = '') =>
        key && i18n.te(key) ? i18n.t(key as never) : fallback
}
