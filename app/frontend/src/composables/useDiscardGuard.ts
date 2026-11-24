import { nextTick, ref, watch } from 'vue'

// Guards an inline form against losing edits to a stray Cancel or Escape.
// When the form opens, its signature is taken; closing it with changes asks
// in place (DiscardPrompt) instead of closing, and a second Escape returns
// to the form. Closing it by setting its state directly (saving, the page
// being hidden, the session ending) is never guarded. Signatures hold only
// non-secret values and whether a secret field is filled, so no secret is
// copied.
export function useDiscardGuard(options: {
    open: () => string | false
    signature: () => string
    close: () => void
}) {
    const confirming = ref(false)
    let baseline = ''
    let returnFocus: HTMLElement | null = null
    watch(
        options.open,
        (open) => {
            confirming.value = false
            if (open)
                void nextTick(() => {
                    baseline = options.signature()
                })
        },
        { immediate: true },
    )
    function keep() {
        confirming.value = false
        void nextTick(() => {
            returnFocus?.focus()
            returnFocus = null
        })
    }
    function request() {
        if (confirming.value) {
            keep()
            return
        }
        if (options.signature() === baseline) {
            options.close()
            return
        }
        returnFocus =
            document.activeElement instanceof HTMLElement
                ? document.activeElement
                : null
        confirming.value = true
    }
    function discard() {
        confirming.value = false
        returnFocus = null
        options.close()
    }
    return { confirming, request, keep, discard }
}
