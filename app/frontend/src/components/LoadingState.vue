<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

defineProps<{ title?: string }>()
const { t } = useI18n()
// A quick load never flashes: the state appears only after 180 ms, keeping
// its space from the start so nothing shifts when it does.
const shown = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
onMounted(() => {
    timer = setTimeout(() => {
        shown.value = true
    }, 180)
})
onBeforeUnmount(() => {
    clearTimeout(timer)
})
</script>

<template>
    <div
        class="loading-state"
        :class="{ 'is-waiting': !shown }"
        role="status"
        aria-live="polite"
    >
        <span class="loading-ring" aria-hidden="true"></span>
        <h1>{{ title ?? t('common.loading.title') }}</h1>
        <p>{{ t('common.loading.description') }}</p>
        <div class="loading-lines" aria-hidden="true">
            <span></span><span></span><span></span>
        </div>
    </div>
</template>
