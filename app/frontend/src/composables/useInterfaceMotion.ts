import { computed } from 'vue'
import { usePreferencesStore } from '@/stores/preferences'
import { useSystemReducedMotion } from '@/theme'
import type { PageTransition } from '@/theme/palettes'

// Critical damping: settle without bounce and preserve velocity on retarget.
export const interfaceSpring = {
    type: 'spring',
    stiffness: 400,
    damping: 40,
} as const

export function useInterfaceMotion() {
    const preferences = usePreferencesStore()
    const systemReducedMotion = useSystemReducedMotion()
    const reduceMotion = computed(
        () => preferences.reducedMotion || systemReducedMotion.value,
    )
    const transition = computed(() =>
        reduceMotion.value ? { duration: 0 } : interfaceSpring,
    )

    return { reduceMotion, transition }
}

// Where each page transition starts; every page springs to rest from here.
export const pageEnter: Record<
    PageTransition,
    { x?: number; y?: number; scale?: number }
> = {
    fade: {},
    slide: { x: 20 },
    zoom: { scale: 0.97 },
    rise: { y: 14 },
}
