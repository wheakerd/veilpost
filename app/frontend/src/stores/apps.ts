import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { appsApi, type StoreApp } from '@/api/apps'

// The catalog and installation state shared by the store page, the app
// pages and the sidebar. It holds no secrets and is never persisted; it is
// cleared when the session ends.
export const useAppsStore = defineStore('apps', () => {
    const apps = ref<StoreApp[] | null>(null)
    const installed = computed(
        () => apps.value?.filter((app) => app.installed) ?? [],
    )
    let loading: Promise<StoreApp[]> | undefined

    /** Reads the catalog from the server; errors reach the caller. */
    async function load(options: { background?: boolean } = {}) {
        loading ??= appsApi.list(options).finally(() => (loading = undefined))
        apps.value = await loading
        return apps.value
    }

    function find(id: string) {
        return apps.value?.find((app) => app.id === id)
    }

    /** Replaces one app with a newer server answer. */
    function update(app: StoreApp) {
        if (!apps.value) return
        apps.value = apps.value.map((item) => (item.id === app.id ? app : item))
    }

    function markUninstalled(id: string) {
        const app = find(id)
        if (app) update({ ...app, installed: null })
    }

    function clear() {
        apps.value = null
    }

    return { apps, installed, load, find, update, markUninstalled, clear }
})
