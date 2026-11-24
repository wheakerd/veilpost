<script setup lang="ts">
import { useInterfaceMotion } from '@/composables/useInterfaceMotion'

// Wraps one v-if (or a v-if / v-else pair) so an inline form or notice opens
// and closes instead of pushing the content below it in a single frame.
// Blocks expand from zero height; table rows, which cannot animate their
// height, cross-fade. Each run starts from the element's current size, so a
// form closed mid-opening folds back from where it is.
const { reduceMotion } = useInterfaceMotion()

// --app-ease, spelled out for the Web Animations API.
const ease = 'cubic-bezier(0.22, 1, 0.36, 1)'
const running = new WeakMap<Element, Animation>()
const closed: Keyframe = {
    height: '0px',
    paddingTop: '0px',
    paddingBottom: '0px',
    marginTop: '0px',
    marginBottom: '0px',
    borderTopWidth: '0px',
    borderBottomWidth: '0px',
    opacity: 0,
}

function open(element: HTMLElement): Keyframe {
    const style = getComputedStyle(element)
    return {
        height: `${element.getBoundingClientRect().height}px`,
        paddingTop: style.paddingTop,
        paddingBottom: style.paddingBottom,
        marginTop: style.marginTop,
        marginBottom: style.marginBottom,
        borderTopWidth: style.borderTopWidth,
        borderBottomWidth: style.borderBottomWidth,
        opacity: 1,
    }
}

function run(target: Element, done: () => void, entering: boolean) {
    const element = target as HTMLElement
    // Measure the live size before the previous run lets go of it.
    const current = open(element)
    running.get(element)?.cancel()
    const fadeOnly = reduceMotion.value || element.tagName === 'TR'
    const frames = fadeOnly
        ? [{ opacity: entering ? 0 : 1 }, { opacity: entering ? 1 : 0 }]
        : entering
          ? [closed, open(element)]
          : [current, closed]
    const animation = element.animate(frames, {
        duration: fadeOnly ? 150 : entering ? 240 : 180,
        easing: ease,
    })
    if (!fadeOnly) element.style.overflow = 'hidden'
    running.set(element, animation)
    animation.onfinish = () => {
        element.style.overflow = ''
        running.delete(element)
        done()
    }
    animation.oncancel = () => {
        element.style.overflow = ''
    }
}
</script>

<template>
    <Transition
        :css="false"
        @enter="(element, done) => run(element, done, true)"
        @leave="(element, done) => run(element, done, false)"
    >
        <slot />
    </Transition>
</template>
