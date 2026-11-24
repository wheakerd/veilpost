import { defineStore } from 'pinia'
import type { RouteTitle } from '@/router'

// Pages opened in this browser tab, for the tab strip. Memory only: never
// persisted, cleared on sign-out.
export interface PageTab {
    path: string
    title: RouteTitle
    appId?: string
}

const limit = 8

export const useTabsStore = defineStore('tabs', {
    state: () => ({ tabs: [] as PageTab[] }),
    actions: {
        open(tab: PageTab) {
            if (this.tabs.some((entry) => entry.path === tab.path)) return
            this.tabs.push(tab)
            // Drop the oldest closable tab; the overview always stays first.
            while (this.tabs.length > limit) {
                const index = this.tabs.findIndex((entry) => entry.path !== '/')
                this.tabs.splice(index, 1)
            }
        },
        // Returns the path to show when the closed tab was the current one.
        close(path: string) {
            const index = this.tabs.findIndex((entry) => entry.path === path)
            if (index < 0 || path === '/') return undefined
            this.tabs.splice(index, 1)
            return (this.tabs[index] ?? this.tabs[index - 1])?.path ?? '/'
        },
        closeOthers(path: string) {
            this.tabs = this.tabs.filter(
                (entry) => entry.path === '/' || entry.path === path,
            )
        },
        clear() {
            this.tabs = []
        },
    },
})
