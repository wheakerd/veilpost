import { nextTick, onBeforeUnmount, watch, type Ref } from 'vue'
import { animate, motionValue } from 'motion-v'
import { interfaceSpring } from '@/composables/useInterfaceMotion'

// The selection of the narrow-screen tab bar is a lens of glass. Pressing a
// tab lifts the lens and slides it under the finger; dragging moves it with
// the finger across the tabs, resisting past either end; releasing throws it
// to the tab its velocity points at, and that page opens. Every move starts
// from the lens's live position and velocity, so it can be caught mid-flight.

type Animation = { stop: () => void }

// A release settles with a little give only because a throw preceded it.
const throwSpring = { type: 'spring', stiffness: 420, damping: 30 } as const
// Follows the finger closely while keeping the velocity of the catch-up.
const followSpring = { type: 'spring', stiffness: 2600, damping: 102 } as const
const liftSpring = { type: 'spring', stiffness: 520, damping: 46 } as const
// Movement before a press becomes a drag.
const slop = 8

function rubberband(overshoot: number, dimension: number) {
    const constant = 0.55
    return (
        (overshoot * dimension * constant) /
        (dimension + constant * Math.abs(overshoot))
    )
}
// Where a throw comes to rest; a snappier rate than scrolling suits a bar of
// four tabs.
function project(velocity: number, decelerationRate = 0.99) {
    return ((velocity / 1000) * decelerationRate) / (1 - decelerationRate)
}

export function useTabLens(options: {
    lens: Ref<HTMLElement | undefined>
    enabled: Ref<boolean>
    reduceMotion: Ref<boolean>
    path: Ref<string>
    navigate: (path: string) => Promise<unknown>
}) {
    const x = motionValue(0)
    const lift = motionValue(0)
    let move: Animation | undefined
    let raise: Animation | undefined
    let shown = false
    let pressed = false
    let suppressClick = false

    function render() {
        const lens = options.lens.value
        if (!lens) return
        const amount = lift.get()
        lens.style.transform = `translate3d(${x.get()}px, 0, 0) scale(${1 + amount * 0.16}, ${1 + amount * 0.1})`
        lens.style.setProperty('--lens-lift', String(amount))
    }
    x.on('change', render)
    lift.on('change', render)

    const nav = () => options.lens.value?.parentElement ?? undefined
    function links() {
        return [
            ...(nav()?.querySelectorAll<HTMLAnchorElement>('a.nav-link') ?? []),
        ].filter((link) => link.getClientRects().length > 0)
    }
    const pathOf = (link: HTMLAnchorElement) =>
        new URL(link.href, location.href).pathname

    function moveTo(target: number, spring: object, velocity?: number) {
        move?.stop()
        if (options.reduceMotion.value) {
            x.jump(target)
            return
        }
        move = animate(x, target, {
            ...spring,
            ...(velocity === undefined ? {} : { velocity }),
        })
    }
    function raiseTo(target: number) {
        raise?.stop()
        if (options.reduceMotion.value) {
            lift.jump(0)
            return
        }
        raise = animate(lift, target, liftSpring)
    }

    // Puts the lens on the open page's tab; hides it on pages without one.
    function layout(jump: boolean) {
        const lens = options.lens.value
        if (!lens || pressed) return
        const link = links().find((item) => pathOf(item) === options.path.value)
        if (!link) {
            lens.style.opacity = '0'
            shown = false
            return
        }
        lens.style.width = `${link.offsetWidth}px`
        lens.style.height = `${link.offsetHeight}px`
        lens.style.top = `${link.offsetTop}px`
        if (jump || !shown) {
            move?.stop()
            x.jump(link.offsetLeft)
        } else moveTo(link.offsetLeft, interfaceSpring)
        lens.style.opacity = '1'
        shown = true
        render()
    }

    function start(event: PointerEvent) {
        if (!options.enabled.value || event.button !== 0) return
        const bar = event.currentTarget as HTMLElement
        const link = (event.target as Element).closest<HTMLAnchorElement>(
            'a.nav-link',
        )
        const lens = options.lens.value
        if (!link || !lens || !bar.contains(link)) return
        const items = links()
        const first = items[0]
        const last = items[items.length - 1]
        if (!first || !last) return
        const width = link.offsetWidth
        const bounds = bar.getBoundingClientRect()
        const min = first.offsetLeft
        const max = last.offsetLeft
        // The lens centres under the finger, softly held inside the bar.
        const under = (clientX: number) => {
            const raw = clientX - bounds.left - width / 2
            const limit = Math.min(max, Math.max(min, raw))
            return limit + rubberband(raw - limit, width)
        }
        pressed = true
        suppressClick = false
        lens.style.opacity = '1'
        shown = true
        raiseTo(1)
        moveTo(link.offsetLeft, interfaceSpring)
        const startX = event.clientX
        let dragging = false
        let samples: { x: number; time: number }[] = []

        const follow = (moveEvent: PointerEvent) => {
            if (!dragging) {
                if (Math.abs(moveEvent.clientX - startX) < slop) return
                dragging = true
                bar.setPointerCapture(moveEvent.pointerId)
            }
            samples = [
                ...samples.slice(-4),
                { x: moveEvent.clientX, time: moveEvent.timeStamp },
            ]
            moveTo(under(moveEvent.clientX), followSpring)
        }
        const finish = (endEvent: PointerEvent) => {
            bar.removeEventListener('pointermove', follow)
            bar.removeEventListener('pointerup', finish)
            bar.removeEventListener('pointercancel', finish)
            pressed = false
            raiseTo(0)
            if (endEvent.type === 'pointercancel') {
                layout(false)
                return
            }
            // A tap and a throw both open the page here; the link's own
            // click, which follows, is set aside.
            suppressClick = true
            // A click that never comes must not swallow a later one.
            window.setTimeout(() => {
                suppressClick = false
            }, 400)
            let target = link
            let velocity = 0
            if (dragging) {
                const firstSample = samples[0]
                const lastSample = samples[samples.length - 1]
                const seconds =
                    firstSample && lastSample
                        ? (lastSample.time - firstSample.time) / 1000
                        : 0
                velocity =
                    firstSample && lastSample && seconds > 0
                        ? (lastSample.x - firstSample.x) / seconds
                        : 0
                const rest = x.get() + project(velocity)
                target = items.reduce((best, item) =>
                    Math.abs(item.offsetLeft - rest) <
                    Math.abs(best.offsetLeft - rest)
                        ? item
                        : best,
                )
            }
            moveTo(target.offsetLeft, throwSpring, velocity)
            const path = pathOf(target)
            if (path === options.path.value) return
            void options.navigate(path).finally(() =>
                nextTick(() => {
                    layout(false)
                }),
            )
        }
        bar.addEventListener('pointermove', follow)
        bar.addEventListener('pointerup', finish)
        bar.addEventListener('pointercancel', finish)
    }
    // Runs before the link's own handler; a prevented click is ignored by
    // RouterLink. Keyboard activation never sets the flag.
    function guardClick(event: MouseEvent) {
        if (!suppressClick) return
        suppressClick = false
        event.preventDefault()
    }

    // A link is natively draggable; that drag would cancel the pointer.
    function preventDrag(event: DragEvent) {
        event.preventDefault()
    }

    let resize: ResizeObserver | undefined
    let bound: HTMLElement | undefined
    function bind() {
        const bar = nav()
        if (bar === bound) return
        unbind()
        if (!bar) return
        bound = bar
        bar.addEventListener('pointerdown', start)
        bar.addEventListener('click', guardClick, true)
        bar.addEventListener('dragstart', preventDrag)
        resize = new ResizeObserver(() => {
            layout(true)
        })
        resize.observe(bar)
    }
    function unbind() {
        bound?.removeEventListener('pointerdown', start)
        bound?.removeEventListener('click', guardClick, true)
        bound?.removeEventListener('dragstart', preventDrag)
        resize?.disconnect()
        bound = undefined
        shown = false
    }

    watch(
        () => [options.lens.value, options.enabled.value] as const,
        () => {
            void nextTick(() => {
                bind()
                layout(true)
            })
        },
        { immediate: true },
    )
    watch(options.path, () => {
        void nextTick(() => {
            layout(false)
        })
    })
    onBeforeUnmount(() => {
        unbind()
        move?.stop()
        raise?.stop()
        x.destroy()
        lift.destroy()
    })
}
