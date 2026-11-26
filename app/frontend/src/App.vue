<script setup lang="ts">
import {
    computed,
    onBeforeUnmount,
    onMounted,
    ref,
    watch,
    type Component,
} from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Overview from '~icons/lucide/layout-dashboard'
import Server from '~icons/lucide/server'
import Store from '~icons/lucide/store'
import FileBadge from '~icons/lucide/file-badge'
import KeyRound from '~icons/lucide/key-round'
import Palette from '~icons/lucide/palette'
import Lock from '~icons/lucide/lock'
import Package from '~icons/lucide/package'
import PanelLeftClose from '~icons/lucide/panel-left-close'
import PanelLeftOpen from '~icons/lucide/panel-left-open'
import RotateCw from '~icons/lucide/rotate-cw'
import Languages from '~icons/lucide/languages'
import X from '~icons/lucide/x'
import SlidersHorizontal from '~icons/lucide/sliders-horizontal'
import elementZhCn from 'element-plus/es/locale/lang/zh-cn'
import elementZhTw from 'element-plus/es/locale/lang/zh-tw'
import elementEn from 'element-plus/es/locale/lang/en'
import elementRu from 'element-plus/es/locale/lang/ru'
import { animate, MotionConfig } from 'motion-v'
import { usePreferencesStore } from '@/stores/preferences'
import { useAuthStore } from '@/stores/auth'
import { useAppsStore } from '@/stores/apps'
import { useTabsStore, type PageTab } from '@/stores/tabs'
import { useCatalogText } from '@/composables/useCatalogText'
import {
    currentLocale,
    isLocale,
    localeName,
    supportedLocales,
    type Locale,
} from '@/i18n'
import { formatClock } from '@/i18n/format'
import { navigationPending, type RouteTitle } from '@/router'
import { readScroll } from '@/router/pageCache'
import AuthLayout from '@/components/AuthLayout.vue'
import RouteContent from '@/components/RouteContent.vue'
import { useInterfaceMotion } from '@/composables/useInterfaceMotion'
import { useSheetDrag } from '@/composables/useSheetDrag'
import { useTabLens } from '@/composables/useTabLens'
import { vGlass } from '@/theme/liquidGlass'
import { useModeToggle } from '@/composables/useModeToggle'
import { authActivity } from '@/api/activity'
import AccountAvatar from '@/components/AccountAvatar.vue'
import AccountCenter from '@/components/AccountCenter.vue'
import LoadingState from '@/components/LoadingState.vue'
import PreferencesPanel from '@/components/PreferencesPanel.vue'
import { sidebarWidth } from '@/theme/palettes'

const elementLocales = {
    'zh-CN': elementZhCn,
    en: elementEn,
    'zh-TW': elementZhTw,
    ru: elementRu,
} satisfies Record<Locale, unknown>

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const tabs = useTabsStore()
// The account centre opens as a dialog from the top bar.
const accountOpen = ref(false)
const refreshKey = ref(0)
const preferences = usePreferencesStore()
const { reduceMotion, transition } = useInterfaceMotion()
const motionPreference = computed(() =>
    preferences.reducedMotion ? 'always' : 'user',
)
const elementLocale = computed(() => elementLocales[currentLocale.value])
const version = __APP_VERSION__

const sidebarGroupKeys = ['workspace', 'software', 'security'] as const
type Group = (typeof sidebarGroupKeys)[number]
interface NavItem {
    to: string
    title: RouteTitle
    icon: Component
    group?: Group
    mobileShortcut?: boolean
}
const navigation: NavItem[] = [
    { to: '/', title: 'overview', icon: Overview, group: 'workspace' },
    { to: '/nodes', title: 'nodes', icon: Server, group: 'workspace' },
    {
        to: '/store',
        title: 'store',
        icon: Store,
        group: 'software',
        mobileShortcut: true,
    },
    {
        to: '/certificates',
        title: 'certificates',
        icon: FileBadge,
        group: 'security',
        mobileShortcut: true,
    },
    {
        to: '/dns-providers',
        title: 'dnsProviders',
        icon: KeyRound,
        group: 'security',
        mobileShortcut: true,
    },
    {
        to: '/settings/security',
        title: 'security',
        icon: Lock,
        group: 'security',
    },
    {
        to: '/settings/appearance',
        title: 'appearance',
        icon: Palette,
    },
]
const sidebarNavigation = navigation.filter((item) => item.group !== undefined)
const sidebarGroups = sidebarGroupKeys.map((group) => ({
    group,
    items: sidebarNavigation.filter((item) => item.group === group),
}))
const mobileTabs = navigation.filter((item) => !item.mobileShortcut)
const mobileShortcuts = navigation.filter((item) => item.mobileShortcut)

// Narrow screens use the bottom bar, where groups do not fold.
const narrowQuery = window.matchMedia('(max-width: 47.5rem)')
const narrow = ref(narrowQuery.matches)
const collapsed = computed(
    () => !narrow.value && preferences.sidebarCollapsed === true,
)
const onNarrowChange = (event: MediaQueryListEvent) => {
    narrow.value = event.matches
}
narrowQuery.addEventListener('change', onNarrowChange)

// Open sidebar groups, in memory only. With navMultiple off, opening one
// group folds the others. The current page's group always opens.
const openGroups = ref(new Set<Group>(sidebarGroupKeys))
function groupOf(path: string) {
    return sidebarNavigation.find((item) => item.to === path)?.group
}
function openGroup(group: Group) {
    openGroups.value = preferences.navMultiple
        ? new Set([...openGroups.value, group])
        : new Set([group])
}
// The desktop sidebar is an Element Plus menu; its open groups follow
// openGroups both ways.
const menu = ref<{
    open: (index: string) => void
    close: (index: string) => void
}>()
function onMenuOpen(index: string) {
    if (sidebarGroupKeys.includes(index as Group)) openGroup(index as Group)
}
function onMenuClose(index: string) {
    if (!openGroups.value.has(index as Group)) return
    const next = new Set(openGroups.value)
    next.delete(index as Group)
    openGroups.value = next
}
watch(openGroups, (open) => {
    for (const group of sidebarGroupKeys)
        if (open.has(group)) menu.value?.open(group)
        else menu.value?.close(group)
})

watch(
    () => [route.path, preferences.navMultiple] as const,
    ([path, multiple], previous) => {
        const group = groupOf(path)
        // Turning navMultiple off keeps only the current page's group.
        if (previous[1] && !multiple)
            openGroups.value = new Set(group ? [group] : [])
        else if (group && !openGroups.value.has(group)) openGroup(group)
    },
)

// Sidebar width: dragging its edge tracks the pointer 1:1, resists softly
// past the limits and springs back inside them on release.
const remPixels = () =>
    parseFloat(getComputedStyle(document.documentElement).fontSize) || 16
const dragWidth = ref<number>()
const shownWidth = computed(
    () => dragWidth.value ?? preferences.sidebarWidth ?? sidebarWidth.default,
)
function rubberband(overshoot: number, dimension: number) {
    const constant = 0.55
    return (
        (overshoot * dimension * constant) /
        (dimension + constant * Math.abs(overshoot))
    )
}
function clampWidth(width: number) {
    return Math.min(sidebarWidth.max, Math.max(sidebarWidth.min, width))
}
function commitWidth(width: number) {
    const value = Math.round(clampWidth(width) * 4) / 4
    preferences.sidebarWidth =
        value === sidebarWidth.default ? undefined : value
}
let widthAnimation: { stop: () => void } | undefined
function startResize(event: PointerEvent) {
    if (event.button !== 0) return
    const handle = event.currentTarget as HTMLElement
    // No text selection, so no native drag of a nearby menu link, which
    // would cancel the pointer.
    event.preventDefault()
    handle.setPointerCapture(event.pointerId)
    widthAnimation?.stop()
    const rem = remPixels()
    const startX = event.clientX
    // Grab offset is kept: the edge stays where the pointer took it.
    const sidebar = handle.closest('.sidebar')
    const startWidth = sidebar
        ? sidebar.getBoundingClientRect().width / rem
        : shownWidth.value
    // Recent samples give the release velocity, handed to the spring.
    let samples: { x: number; time: number }[] = []
    const move = (moveEvent: PointerEvent) => {
        samples = [
            ...samples.slice(-4),
            { x: moveEvent.clientX, time: moveEvent.timeStamp },
        ]
        const raw = startWidth + (moveEvent.clientX - startX) / rem
        const limit = clampWidth(raw)
        dragWidth.value = limit + rubberband(raw - limit, 8)
    }
    const end = () => {
        handle.removeEventListener('pointermove', move)
        handle.removeEventListener('pointerup', end)
        handle.removeEventListener('pointercancel', end)
        const from = dragWidth.value ?? startWidth
        const to = clampWidth(from)
        commitWidth(to)
        if (from === to || reduceMotion.value) {
            dragWidth.value = undefined
            return
        }
        const first = samples[0]
        const last = samples[samples.length - 1]
        const seconds = first && last ? (last.time - first.time) / 1000 : 0
        const velocity =
            first && last && seconds > 0
                ? (last.x - first.x) / rem / seconds
                : 0
        widthAnimation = animate(from, to, {
            ...transition.value,
            velocity,
            onUpdate: (value: number) => {
                dragWidth.value = value
            },
            onComplete: () => {
                dragWidth.value = undefined
            },
        })
    }
    handle.addEventListener('pointermove', move)
    handle.addEventListener('pointerup', end)
    handle.addEventListener('pointercancel', end)
}
function resizeByKey(event: KeyboardEvent) {
    const steps: Record<string, number> = {
        ArrowLeft: shownWidth.value - 0.25,
        ArrowRight: shownWidth.value + 0.25,
        Home: sidebarWidth.min,
        End: sidebarWidth.max,
    }
    const next = steps[event.key]
    if (next === undefined) return
    event.preventDefault()
    commitWidth(next)
}
// Without a stored width the stylesheet's responsive default applies.
const shellStyle = computed(() =>
    collapsed.value ||
    narrow.value ||
    (dragWidth.value === undefined && preferences.sidebarWidth === undefined)
        ? undefined
        : { '--sidebar-width': `${shownWidth.value}rem` },
)

const preferencesOpen = ref(false)
// On narrow screens the tab bar's selection is a lens of glass that can be
// pressed and dragged across the tabs.
const tabLens = ref<HTMLElement>()
useTabLens({
    lens: tabLens,
    enabled: narrow,
    reduceMotion,
    path: computed(() => route.path),
    navigate: (path) => router.push(path),
})

// On narrow screens preferences open as a bottom sheet that can be pulled
// down to close.
const sheetDrag = useSheetDrag({
    enabled: narrow,
    reduceMotion,
    close: () => {
        preferencesOpen.value = false
    },
})

const contentBusy = computed(
    () => navigationPending.value || authActivity.pending > 0,
)
const progressPending = computed(
    () =>
        preferences.progressBar &&
        route.matched.length > 0 &&
        contentBusy.value,
)
const showProgress = ref(false)
// Only the indicator waits. Navigation and requests run as soon as they start.
watch(
    progressPending,
    (pending, _, onCleanup) => {
        if (!pending) {
            showProgress.value = false
            return
        }
        const timer = setTimeout(() => (showProgress.value = true), 150)
        onCleanup(() => {
            clearTimeout(timer)
        })
    },
    { flush: 'post' },
)

// The sidebar lists installed apps; the catalog lives only in memory and
// follows the signed-in account. A failed read leaves the group empty.
const apps = useAppsStore()
const catalogText = useCatalogText()
watch(
    () => auth.user?.id,
    (id) => {
        if (id) apps.load({ background: true }).catch(() => undefined)
        else {
            apps.clear()
            tabs.clear()
        }
    },
    { immediate: true },
)

// Every console page visited in this browser tab gets a tab.
watch(
    () => [route.path, auth.user?.id] as const,
    ([path, user]) => {
        if (!user || route.meta.authPage || !route.meta.title) return
        if (!tabs.tabs.length) tabs.open({ path: '/', title: 'overview' })
        tabs.open({
            path,
            title: route.meta.title,
            appId: route.name === 'app' ? String(route.params.id) : undefined,
        })
    },
    { immediate: true },
)

function tabTitle(tab: PageTab) {
    if (tab.appId) {
        const app = apps.find(tab.appId)
        return app ? catalogText(app.name, app.id) : tab.appId
    }
    return t(`common.routes.${tab.title}`)
}

async function closeTab(path: string) {
    const next = tabs.close(path)
    if (next && route.path === path) await router.push(next)
}

function toggleSidebar() {
    preferences.sidebarCollapsed = collapsed.value ? undefined : true
}

const mode = useModeToggle()

function chooseLocale(value: string) {
    preferences.locale = isLocale(value) ? value : undefined
}

const now = ref(Date.now())
let clock: ReturnType<typeof setInterval> | undefined
// The top bar shows its edge only while content passes beneath it.
const scrolled = ref(false)
function onScroll() {
    scrolled.value = readScroll() > 0
}
onMounted(() => {
    clock = setInterval(() => {
        now.value = Date.now()
    }, 1000)
    onScroll()
    // The window or the page panel, whichever scrolls; scroll events do not
    // bubble, so the listener captures.
    document.addEventListener('scroll', onScroll, {
        capture: true,
        passive: true,
    })
})
onBeforeUnmount(() => {
    clearInterval(clock)
    document.removeEventListener('scroll', onScroll, { capture: true })
    narrowQuery.removeEventListener('change', onNarrowChange)
})
</script>

<template>
    <ElConfigProvider :locale="elementLocale">
        <MotionConfig :reduced-motion="motionPreference">
            <div
                v-if="showProgress"
                class="request-progress"
                aria-hidden="true"
            >
                <span></span>
            </div>
            <a class="skip-link" href="#main-content">{{
                t('common.shell.skipLink')
            }}</a>
            <AuthLayout v-if="route.meta.authPage"><RouteContent /></AuthLayout>
            <div
                v-else-if="auth.user && route.matched.length"
                class="app-shell"
                :class="{
                    'is-collapsed': collapsed,
                    'is-resizing': dragWidth !== undefined,
                }"
                :style="shellStyle"
            >
                <header
                    class="topbar"
                    :data-scrolled="scrolled ? '' : undefined"
                >
                    <div class="topbar-start">
                        <RouterLink
                            class="brand"
                            to="/"
                            :aria-label="t('common.app.home')"
                        >
                            <img
                                src="/favicon.svg"
                                width="28"
                                height="28"
                                alt=""
                            />
                            <strong class="brand-name">{{
                                t('common.app.name')
                            }}</strong>
                        </RouterLink>
                        <ElTooltip
                            :content="
                                collapsed
                                    ? t('common.shell.expand')
                                    : t('common.shell.collapse')
                            "
                            placement="bottom"
                            :show-after="300"
                        >
                            <!-- The anchor takes the tooltip's ARIA so the
                                 button keeps aria-expanded. -->
                            <span class="tooltip-anchor"
                                ><button
                                    type="button"
                                    class="icon-button sidebar-toggle"
                                    :aria-label="
                                        collapsed
                                            ? t('common.shell.expand')
                                            : t('common.shell.collapse')
                                    "
                                    :aria-expanded="!collapsed"
                                    @click="toggleSidebar"
                                >
                                    <PanelLeftOpen
                                        v-if="collapsed"
                                        aria-hidden="true"
                                    />
                                    <PanelLeftClose
                                        v-else
                                        aria-hidden="true"
                                    /></button
                            ></span>
                        </ElTooltip>
                        <span class="breadcrumb"
                            ><span class="breadcrumb-root">{{
                                t('common.shell.breadcrumb')
                            }}</span>
                            <span class="breadcrumb-separator">/</span>
                            <strong v-if="route.meta.title">{{
                                route.name === 'app'
                                    ? tabTitle({
                                          path: route.path,
                                          title: 'installedApp',
                                          appId: String(route.params.id),
                                      })
                                    : t(`common.routes.${route.meta.title}`)
                            }}</strong></span
                        >
                    </div>
                    <div class="toolbar">
                        <div v-glass class="tool-group">
                            <ElTooltip
                                :content="t('common.shell.refresh')"
                                placement="bottom"
                                :show-after="300"
                            >
                                <button
                                    type="button"
                                    class="icon-button"
                                    :aria-label="t('common.shell.refresh')"
                                    @click="refreshKey++"
                                >
                                    <RotateCw
                                        class="refresh-icon"
                                        :style="{
                                            '--refresh-turn': `${refreshKey}turn`,
                                        }"
                                        aria-hidden="true"
                                    />
                                </button>
                            </ElTooltip>
                            <ElTooltip
                                :content="mode.label.value"
                                placement="bottom"
                                :trigger="['hover', 'focus']"
                                :trigger-keys="[]"
                                :show-after="300"
                            >
                                <button
                                    type="button"
                                    class="icon-button"
                                    :aria-label="mode.label.value"
                                    @click="mode.toggle"
                                >
                                    <component
                                        :is="mode.icon.value"
                                        aria-hidden="true"
                                    />
                                </button>
                            </ElTooltip>
                            <ElDropdown
                                trigger="click"
                                placement="bottom-end"
                                popper-class="glass-popper"
                                @command="chooseLocale"
                            >
                                <button
                                    type="button"
                                    class="icon-button"
                                    :aria-label="t('common.shell.language')"
                                >
                                    <Languages aria-hidden="true" />
                                </button>
                                <template #dropdown>
                                    <ElDropdownMenu>
                                        <ElDropdownItem command="">{{
                                            t('common.shell.followBrowser')
                                        }}</ElDropdownItem>
                                        <ElDropdownItem
                                            v-for="locale in supportedLocales"
                                            :key="locale"
                                            :command="locale"
                                            :lang="locale"
                                            >{{
                                                localeName(locale)
                                            }}</ElDropdownItem
                                        >
                                    </ElDropdownMenu>
                                </template>
                            </ElDropdown>
                            <ElTooltip
                                :content="t('common.shell.preferences')"
                                placement="bottom"
                                :show-after="300"
                            >
                                <button
                                    type="button"
                                    class="icon-button"
                                    :aria-label="t('common.shell.preferences')"
                                    @click="preferencesOpen = true"
                                >
                                    <SlidersHorizontal aria-hidden="true" />
                                </button>
                            </ElTooltip>
                        </div>
                        <time
                            class="readout clock"
                            :aria-label="t('common.shell.clock')"
                            >{{ formatClock(now) }}</time
                        >
                        <button
                            v-glass
                            type="button"
                            class="account-trigger"
                            aria-haspopup="dialog"
                            :aria-expanded="accountOpen"
                            :aria-label="t('common.shell.account.open')"
                            @click="accountOpen = true"
                        >
                            <AccountAvatar :username="auth.user.username" />
                            <span class="account-name">{{
                                auth.user.username
                            }}</span>
                        </button>
                    </div>
                </header>
                <aside class="sidebar">
                    <nav
                        v-if="!narrow"
                        class="sidebar-nav"
                        :aria-label="t('common.shell.navigation')"
                    >
                        <ElMenu
                            ref="menu"
                            class="sidebar-menu"
                            :class="{ 'is-rail': collapsed }"
                            :default-active="route.path"
                            :default-openeds="[...openGroups]"
                            :unique-opened="!preferences.navMultiple"
                            :collapse-transition="false"
                            @open="onMenuOpen"
                            @close="onMenuClose"
                        >
                            <template v-if="!collapsed">
                                <ElSubMenu
                                    v-for="entry in sidebarGroups"
                                    :key="entry.group"
                                    :index="entry.group"
                                >
                                    <template #title>{{
                                        t(`common.shell.groups.${entry.group}`)
                                    }}</template>
                                    <ElMenuItem
                                        v-for="item in entry.items"
                                        :key="item.to"
                                        :index="item.to"
                                    >
                                        <RouterLink
                                            :to="item.to"
                                            class="menu-link"
                                        >
                                            <component
                                                :is="item.icon"
                                                aria-hidden="true"
                                            />
                                            <span class="nav-label">{{
                                                t(`common.routes.${item.title}`)
                                            }}</span>
                                        </RouterLink>
                                    </ElMenuItem>
                                </ElSubMenu>
                            </template>
                            <template v-else>
                                <ElTooltip
                                    v-for="item in sidebarNavigation"
                                    :key="item.to"
                                    :content="t(`common.routes.${item.title}`)"
                                    placement="right"
                                    :trigger="['hover', 'focus']"
                                    :trigger-keys="[]"
                                    :show-after="200"
                                >
                                    <ElMenuItem :index="item.to">
                                        <RouterLink
                                            :to="item.to"
                                            class="menu-link"
                                            :aria-label="
                                                t(`common.routes.${item.title}`)
                                            "
                                        >
                                            <component
                                                :is="item.icon"
                                                aria-hidden="true"
                                            />
                                        </RouterLink>
                                    </ElMenuItem>
                                </ElTooltip>
                            </template>
                        </ElMenu>
                    </nav>
                    <nav
                        v-if="narrow"
                        v-glass
                        class="primary-nav"
                        :aria-label="t('common.shell.navigation')"
                    >
                        <span
                            ref="tabLens"
                            v-glass="{ material: false, depth: 10, scale: 8 }"
                            class="tab-lens"
                            aria-hidden="true"
                        ></span>
                        <RouterLink
                            v-for="item in mobileTabs"
                            :key="item.to"
                            :to="item.to"
                            class="nav-link"
                        >
                            <component :is="item.icon" aria-hidden="true" />
                            <span class="nav-label">{{
                                t(`common.routes.${item.title}`)
                            }}</span>
                        </RouterLink>
                    </nav>
                    <div class="sidebar-apps">
                        <template v-if="narrow">
                            <RouterLink
                                v-for="item in mobileShortcuts"
                                :key="item.to"
                                :to="item.to"
                                class="nav-link installed-link"
                            >
                                <component :is="item.icon" aria-hidden="true" />
                                <span class="nav-label">{{
                                    t(`common.routes.${item.title}`)
                                }}</span>
                            </RouterLink>
                        </template>
                        <span
                            v-if="apps.installed.length"
                            class="installed-caption"
                            aria-hidden="true"
                            >{{ t('common.shell.installedApps') }}</span
                        >
                        <nav
                            v-if="apps.installed.length"
                            class="installed-apps"
                            :aria-label="t('common.shell.installedApps')"
                        >
                            <ElTooltip
                                v-for="app in apps.installed"
                                :key="app.id"
                                :content="catalogText(app.name, app.id)"
                                placement="right"
                                :trigger="['hover', 'focus']"
                                :trigger-keys="[]"
                                :disabled="!collapsed"
                                :show-after="200"
                            >
                                <RouterLink
                                    :to="`/apps/${app.id}`"
                                    class="nav-link installed-link"
                                    :aria-label="
                                        collapsed
                                            ? catalogText(app.name, app.id)
                                            : undefined
                                    "
                                >
                                    <Package aria-hidden="true" />
                                    <span class="nav-label">{{
                                        catalogText(app.name, app.id)
                                    }}</span>
                                </RouterLink>
                            </ElTooltip>
                        </nav>
                    </div>
                    <div
                        v-if="!narrow && !collapsed"
                        class="sidebar-resizer"
                        role="separator"
                        tabindex="0"
                        aria-orientation="vertical"
                        :aria-label="t('appearance.layout.sidebarWidth.handle')"
                        :aria-valuemin="sidebarWidth.min"
                        :aria-valuemax="sidebarWidth.max"
                        :aria-valuenow="shownWidth"
                        @pointerdown="startResize"
                        @keydown="resizeByKey"
                        @dblclick="preferences.sidebarWidth = undefined"
                    ></div>
                    <p class="sidebar-foot">
                        <span class="readout">{{
                            t('common.shell.version', { version })
                        }}</span>
                    </p>
                </aside>

                <div class="workspace">
                    <nav
                        v-if="preferences.tabBar && tabs.tabs.length"
                        class="page-tabs"
                        :aria-label="t('common.shell.openPages')"
                    >
                        <div
                            v-for="tab in tabs.tabs"
                            :key="tab.path"
                            class="page-tab"
                            :class="{ active: route.path === tab.path }"
                        >
                            <RouterLink
                                :to="tab.path"
                                :aria-current="
                                    route.path === tab.path ? 'page' : undefined
                                "
                                >{{ tabTitle(tab) }}</RouterLink
                            >
                            <button
                                v-if="tab.path !== '/'"
                                type="button"
                                class="tab-close"
                                :aria-label="
                                    t('common.shell.closeTab', {
                                        title: tabTitle(tab),
                                    })
                                "
                                @click="closeTab(tab.path)"
                            >
                                <X aria-hidden="true" />
                            </button>
                        </div>
                        <button
                            v-if="tabs.tabs.length > 2"
                            type="button"
                            class="tabs-close-others"
                            @click="tabs.closeOthers(route.path)"
                        >
                            {{ t('common.shell.closeOthers') }}
                        </button>
                        <ElTooltip
                            :content="t('common.shell.hideTabs')"
                            placement="bottom"
                            :show-after="300"
                        >
                            <button
                                type="button"
                                class="tab-close tabs-hide"
                                :aria-label="t('common.shell.hideTabs')"
                                @click="preferences.tabBar = false"
                            >
                                <X aria-hidden="true" />
                            </button>
                        </ElTooltip>
                    </nav>
                    <main
                        id="main-content"
                        tabindex="-1"
                        :aria-busy="contentBusy"
                    >
                        <RouteContent :refresh-key="refreshKey" />
                    </main>
                </div>
                <AccountCenter v-model="accountOpen" />
                <ElDrawer
                    v-model="preferencesOpen"
                    class="glass-drawer"
                    :class="{ 'is-sheet': narrow }"
                    :title="t('common.shell.preferences')"
                    :direction="narrow ? 'btt' : 'rtl'"
                    :size="narrow ? '88%' : '25rem'"
                    append-to-body
                    @closed="sheetDrag.release"
                >
                    <template #header="{ titleClass }">
                        <div
                            class="drawer-heading"
                            @pointerdown="sheetDrag.start"
                        >
                            <span
                                v-if="narrow"
                                class="sheet-grabber"
                                aria-hidden="true"
                            ></span>
                            <span
                                :class="titleClass"
                                role="heading"
                                aria-level="2"
                                >{{ t('common.shell.preferences') }}</span
                            >
                        </div>
                    </template>
                    <PreferencesPanel
                        id-prefix="drawer-preferences"
                        section-heading="h3"
                    />
                </ElDrawer>
            </div>
            <main v-else id="main-content" class="session-loading">
                <LoadingState :title="t('common.loading.title')" />
            </main>
        </MotionConfig>
    </ElConfigProvider>
</template>
