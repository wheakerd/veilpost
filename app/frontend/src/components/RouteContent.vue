<script setup lang="ts">
import {
    computed,
    defineComponent,
    h,
    nextTick,
    onActivated,
    onMounted,
    reactive,
    ref,
    watch,
    type Component,
} from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import { animate } from 'motion-v'
import { pageEnter, useInterfaceMotion } from '@/composables/useInterfaceMotion'
import { usePreferencesStore } from '@/stores/preferences'
import { useTabsStore } from '@/stores/tabs'
import { pageScroll } from '@/router/pageCache'

// Changing refreshKey remounts the current page, as a reload would.
const props = defineProps<{ refreshKey?: number }>()
const route = useRoute()
const router = useRouter()
const preferences = usePreferencesStore()
const tabs = useTabsStore()
const { reduceMotion, transition } = useInterfaceMotion()

const rest = { opacity: 1, x: 0, y: 0, scale: 1 }

function enter(element: HTMLElement | null) {
    if (!element || reduceMotion.value) return
    const from = pageEnter[preferences.pageTransition]
    // Start from the offset and spring to rest; nothing waits for it.
    animate(element, { ...rest, ...from }, { duration: 0 })
    animate(element, rest, transition.value)
}

// One wrapper per path, named after it, so KeepAlive can drop exactly the
// page whose tab closed. The page inside receives activated/deactivated.
const wrappers = new Map<string, Component>()
function wrapper(path: string) {
    let component = wrappers.get(path)
    if (!component) {
        component = defineComponent({
            name: `page:${path}`,
            setup(_, { slots }) {
                const element = ref<HTMLElement | null>(null)
                onMounted(() => {
                    pageScroll.delete(path)
                    enter(element.value)
                })
                onActivated(() => {
                    enter(element.value)
                })
                return () =>
                    h(
                        'div',
                        { ref: element, class: 'route-content' },
                        slots.default?.(),
                    )
            },
        })
        wrappers.set(path, component)
    }
    return component
}

// Kept pages: the open tabs, unless a page opts out (it holds secrets) or
// the preference is off. Closing a tab or signing out drops the page.
const refreshing = ref('')
const cached = computed(() =>
    preferences.pageCache
        ? tabs.tabs
              .filter(
                  (tab) =>
                      tab.path !== refreshing.value &&
                      !router.resolve(tab.path).meta.noCache,
              )
              .map((tab) => `page:${tab.path}`)
        : [],
)
const generation = reactive(new Map<string, number>())
watch(
    () => props.refreshKey,
    async () => {
        const path = route.path
        refreshing.value = path
        await nextTick()
        generation.set(path, (generation.get(path) ?? 0) + 1)
        await nextTick()
        refreshing.value = ''
    },
)
</script>

<template>
    <RouterView v-slot="{ Component: page, route: current }">
        <KeepAlive :include="cached" :max="8">
            <component
                :is="wrapper(current.path)"
                :key="`${current.path}:${generation.get(current.path) ?? 0}`"
            >
                <component :is="page" />
            </component>
        </KeepAlive>
    </RouterView>
</template>
