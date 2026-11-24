import { watchEffect } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import type { MessageSchema } from '@/locales/zh-CN'
import { i18n } from '@/i18n'
import { useAuthStore } from '@/stores/auth'
import { safeReturnPath } from './returnPath'
import { pageScroll, readScroll, restoreScroll } from './pageCache'

export type RouteTitle = keyof MessageSchema['common']['routes']

declare module 'vue-router' {
    interface RouteMeta {
        // Key under common.routes in the language packs.
        title?: RouteTitle
        authPage?: boolean
        // Never kept in the page cache: the page holds secrets in memory.
        noCache?: boolean
    }
}

export const router = createRouter({
    history: createWebHistory(),
    routes: [
        {
            path: '/login',
            name: 'login',
            component: () => import('@/views/LoginView.vue'),
            meta: { title: 'login', authPage: true },
        },
        {
            path: '/login/verify',
            name: 'verify',
            component: () => import('@/views/VerifyView.vue'),
            meta: { title: 'verify', authPage: true },
        },
        {
            path: '/',
            name: 'overview',
            component: () => import('@/views/OverviewView.vue'),
            meta: { title: 'overview' },
        },
        {
            path: '/nodes',
            name: 'nodes',
            component: () => import('@/views/NodesView.vue'),
            meta: { title: 'nodes' },
        },
        {
            path: '/store',
            name: 'store',
            component: () => import('@/views/StoreView.vue'),
            meta: { title: 'store' },
        },
        {
            path: '/apps/:id',
            name: 'app',
            component: () => import('@/views/AppView.vue'),
            meta: { title: 'installedApp' },
        },
        {
            path: '/certificates',
            name: 'certificates',
            component: () => import('@/views/CertificatesView.vue'),
            meta: { title: 'certificates' },
        },
        {
            path: '/dns-providers',
            name: 'dns-providers',
            component: () => import('@/views/DnsProvidersView.vue'),
            meta: { title: 'dnsProviders' },
        },
        {
            path: '/settings/appearance',
            name: 'appearance',
            component: () => import('@/views/AppearanceView.vue'),
            meta: { title: 'appearance' },
        },
        {
            path: '/settings/security',
            name: 'security',
            component: () => import('@/views/SecurityView.vue'),
            meta: { title: 'security', noCache: true },
        },
        {
            path: '/:pathMatch(.*)*',
            name: 'not-found',
            component: () => import('@/views/NotFoundView.vue'),
            meta: { title: 'notFound' },
        },
    ],
    // A page restored from the page cache returns to where it was left.
    // The replacement (useScrollRestoration) is still experimental.
    // eslint-disable-next-line @typescript-eslint/no-deprecated
    scrollBehavior: (to) => restoreScroll(pageScroll.get(to.path) ?? 0),
})

// Navigation re-reads the session from the server unless it was confirmed
// within this window, which keeps rapid page changes free of extra requests.
const sessionRecheckInterval = 15_000

router.beforeEach(async (to, from) => {
    if (from.matched.length) pageScroll.set(from.path, readScroll())
    const auth = useAuthStore()
    await auth.restore(sessionRecheckInterval)
    const redirect = safeReturnPath(
        to.meta.authPage ? to.query.redirect : to.fullPath,
    )
    if (auth.user) {
        if (to.meta.authPage) return redirect
        return
    }
    if (auth.challengeExpiresAt) {
        if (to.name !== 'verify')
            return { path: '/login/verify', query: { redirect } }
        return
    }
    if (to.name !== 'login') return { path: '/login', query: { redirect } }
    return
})

// Follows both the current route and the interface language.
watchEffect(() => {
    const { title } = router.currentRoute.value.meta
    const app = i18n.global.t('common.app.name')
    document.title = title
        ? `${i18n.global.t(`common.routes.${title}`)} · ${app}`
        : app
})
