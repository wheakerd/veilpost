import { onActivated, onDeactivated } from 'vue'

// For pages kept by the page cache: pause runs when the page is hidden
// (stop polling, clear secrets from memory), resume when it is shown again
// (refresh data in the background). A page that is not cached never calls
// either; it unmounts and remounts instead.
export function useCachedPage(hooks: {
    pause: () => void
    resume: () => void
}) {
    let hidden = false
    onDeactivated(() => {
        hidden = true
        hooks.pause()
    })
    onActivated(() => {
        if (!hidden) return
        hidden = false
        hooks.resume()
    })
}
