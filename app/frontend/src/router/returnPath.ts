const destinations = new Set([
    '/',
    '/nodes',
    '/store',
    '/certificates',
    '/dns-providers',
    '/settings/appearance',
    '/settings/security',
])
// Installed app pages, by catalog id (src/api/apps.ts).
const appPage = /^\/apps\/[a-z][a-z0-9-]{1,31}$/

export function safeReturnPath(value: unknown): string {
    if (
        typeof value !== 'string' ||
        !value.startsWith('/') ||
        value.startsWith('//') ||
        // Rejects backslashes, controls and spaces on purpose.
        // eslint-disable-next-line no-control-regex
        /[\\\u0000-\u0020]/.test(value)
    )
        return '/'
    try {
        const url = new URL(value, window.location.origin)
        if (
            url.origin !== window.location.origin ||
            !(destinations.has(url.pathname) || appPage.test(url.pathname))
        )
            return '/'
        return `${url.pathname}${url.search}${url.hash}`
    } catch {
        return '/'
    }
}
