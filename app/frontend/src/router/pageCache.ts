// Scroll position each page was left at, keyed by path. Memory only. A page
// restored from the page cache returns to it; a freshly mounted page clears
// its entry and starts at the top.
export const pageScroll = new Map<string, number>()

// Where the page scrolls: on wide screens the window stays still and only the
// page panel (#main-content) scrolls; on narrow screens the window scrolls.
function scrollingPanel() {
    const main = document.getElementById('main-content')
    if (!main) return null
    const overflow = getComputedStyle(main).overflowY
    return overflow === 'auto' || overflow === 'scroll' ? main : null
}

export function readScroll() {
    return scrollingPanel()?.scrollTop ?? window.scrollY
}

// Returns the window position for the router when the window scrolls, and
// moves the panel itself otherwise.
export function restoreScroll(top: number) {
    const panel = scrollingPanel()
    if (!panel) return { top }
    panel.scrollTop = top
    return false as const
}
