import { ref } from 'vue'

// Read by AuthLayout, which owns the look of the exit.
export const enteringConsole = ref(false)

export async function enterConsole(navigate: () => Promise<unknown>) {
    enteringConsole.value = true
    try {
        // A refused or failed navigation returns the form.
        if (await navigate()) enteringConsole.value = false
    } catch (cause) {
        enteringConsole.value = false
        throw cause
    }
}

// Shakes an area once when the server rejects what was entered in it.
// --app-ease, spelled out for the Web Animations API.
export function shake(element: Element | null | undefined, reduce: boolean) {
    if (!element || reduce) return
    element.animate(
        [
            { transform: 'translateX(0)' },
            { transform: 'translateX(-8px)' },
            { transform: 'translateX(7px)' },
            { transform: 'translateX(-5px)' },
            { transform: 'translateX(3px)' },
            { transform: 'translateX(0)' },
        ],
        { duration: 360, easing: 'cubic-bezier(0.22, 1, 0.36, 1)' },
    )
}
