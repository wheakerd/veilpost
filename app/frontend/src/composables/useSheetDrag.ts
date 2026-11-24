import type { Ref } from 'vue'
import { animate } from 'motion-v'
import { interfaceSpring } from '@/composables/useInterfaceMotion'

// A bottom sheet that follows the pointer 1:1 from its header, resists being
// pulled up, and on release projects the throw to decide between closing and
// settling. Either way a spring continues at the release velocity, and the
// sheet can be grabbed again mid-flight.

// Scroll-like deceleration: where the sheet would come to rest if thrown.
const decelerationRate = 0.998
function project(velocity: number) {
    return ((velocity / 1000) * decelerationRate) / (1 - decelerationRate)
}
function rubberband(overshoot: number, dimension: number) {
    const constant = 0.55
    return (
        (overshoot * dimension * constant) /
        (dimension + constant * Math.abs(overshoot))
    )
}
// Movement before the drag commits, so a tap on the header stays a tap.
const slop = 6

export function useSheetDrag(options: {
    enabled: Ref<boolean>
    reduceMotion: Ref<boolean>
    close: () => void
}) {
    let animation: { stop: () => void } | undefined
    let sheet: HTMLElement | undefined
    let offset = 0

    function place(value: number) {
        offset = value
        if (sheet) sheet.style.transform = `translateY(${value}px)`
    }
    // Hands the sheet back to its own open and close transitions.
    function release() {
        animation?.stop()
        animation = undefined
        if (sheet) {
            sheet.style.transform = ''
            sheet.style.transition = ''
        }
        sheet = undefined
        offset = 0
    }
    // The live position, including one taken mid-transition.
    function presentOffset(element: HTMLElement) {
        const transform = getComputedStyle(element).transform
        return transform === 'none' ? 0 : new DOMMatrixReadOnly(transform).m42
    }

    function start(event: PointerEvent) {
        if (!options.enabled.value || event.button !== 0) return
        if ((event.target as Element).closest('button')) return
        const handle = event.currentTarget as HTMLElement
        const element = handle.closest<HTMLElement>('.el-drawer')
        if (!element) return
        animation?.stop()
        animation = undefined
        const height = element.getBoundingClientRect().height
        const startOffset = presentOffset(element)
        const startY = event.clientY
        let dragging = false
        let samples: { y: number; time: number }[] = []
        handle.setPointerCapture(event.pointerId)

        const move = (moveEvent: PointerEvent) => {
            const distance = moveEvent.clientY - startY
            if (!dragging) {
                if (Math.abs(distance) < slop) return
                dragging = true
                sheet = element
                // Inline, because the drawer rewrites its classes as it
                // renders; its CSS transition would lag the pointer.
                element.style.transition = 'none'
            }
            samples = [
                ...samples.slice(-4),
                { y: moveEvent.clientY, time: moveEvent.timeStamp },
            ]
            const raw = startOffset + distance
            place(raw >= 0 ? raw : rubberband(raw, height))
        }
        const end = () => {
            handle.removeEventListener('pointermove', move)
            handle.removeEventListener('pointerup', end)
            handle.removeEventListener('pointercancel', end)
            if (!dragging) return
            const first = samples[0]
            const last = samples[samples.length - 1]
            const seconds = first && last ? (last.time - first.time) / 1000 : 0
            const velocity =
                first && last && seconds > 0 ? (last.y - first.y) / seconds : 0
            const dismiss = offset + project(velocity) > height / 2
            if (options.reduceMotion.value) {
                if (dismiss) options.close()
                else release()
                return
            }
            animation = animate(offset, dismiss ? height : 0, {
                ...interfaceSpring,
                velocity,
                onUpdate: place,
                onComplete: () => {
                    animation = undefined
                    // Closing from below the screen: the sheet's own leave
                    // transition finds it already gone and only fades the
                    // scrim; release() runs once the drawer has closed.
                    if (dismiss) options.close()
                    else release()
                },
            })
        }
        handle.addEventListener('pointermove', move)
        handle.addEventListener('pointerup', end)
        handle.addEventListener('pointercancel', end)
    }

    return { start, release }
}
