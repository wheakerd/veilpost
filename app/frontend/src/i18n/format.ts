import { currentLocale, i18n, type Locale } from '.'

// Intl formatters follow the interface language. Reading currentLocale makes
// callers inside computed values or templates re-render when it changes.
const dateTimeFormats = new Map<Locale, Intl.DateTimeFormat>()
const relativeFormats = new Map<Locale, Intl.RelativeTimeFormat>()
const numberFormats = new Map<Locale, Intl.NumberFormat>()

function cached<T>(cache: Map<Locale, T>, create: (locale: Locale) => T) {
    const locale = currentLocale.value
    let format = cache.get(locale)
    if (!format) {
        format = create(locale)
        cache.set(locale, format)
    }
    return format
}

/** Month, day, hour and minute, for timestamps within the recent past or future. */
export function formatDateTime(value: string | number | Date) {
    return cached(
        dateTimeFormats,
        (locale) =>
            new Intl.DateTimeFormat(locale, {
                month: 'numeric',
                day: 'numeric',
                hour: '2-digit',
                minute: '2-digit',
            }),
    ).format(new Date(value))
}

const dateFormats = new Map<Locale, Intl.DateTimeFormat>()

/** Year, month and day, for dates that may be months away (expiry, renewal). */
export function formatDate(value: string | number | Date) {
    return cached(
        dateFormats,
        (locale) =>
            new Intl.DateTimeFormat(locale, {
                year: 'numeric',
                month: 'short',
                day: 'numeric',
            }),
    ).format(new Date(value))
}

export function formatNumber(value: number) {
    return cached(
        numberFormats,
        (locale) => new Intl.NumberFormat(locale),
    ).format(value)
}

/** "just now", "5 minutes ago", then an absolute date after an hour. */
export function formatSince(value: string, now: number) {
    const seconds = Math.max(0, Math.round((now - Date.parse(value)) / 1000))
    if (seconds < 60) return i18n.global.t('common.time.justNow')
    if (seconds < 3600)
        return cached(
            relativeFormats,
            (locale) =>
                new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }),
        ).format(-Math.floor(seconds / 60), 'minute')
    return formatDateTime(value)
}

const clockFormats = new Map<Locale, Intl.DateTimeFormat>()

/** Hours, minutes and seconds on a 24-hour clock, for live readouts. */
export function formatClock(value: string | number | Date) {
    return cached(
        clockFormats,
        (locale) =>
            new Intl.DateTimeFormat(locale, {
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit',
                hourCycle: 'h23',
            }),
    ).format(new Date(value))
}
