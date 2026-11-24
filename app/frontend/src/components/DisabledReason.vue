<script setup lang="ts">
import { useId } from 'vue'

// Says why a visible control cannot be used. While a reason is given, the
// control sits in a focusable wrapper that shows the reason as a tooltip on
// hover and focus and describes itself with it; with no reason the control
// renders as it is.
defineProps<{ reason?: string | false }>()
const id = useId()
</script>

<template>
    <ElTooltip
        v-if="reason"
        :content="reason"
        placement="top"
        :trigger="['hover', 'focus']"
        :show-after="200"
    >
        <span class="disabled-reason" tabindex="0" :aria-describedby="id"
            ><slot /><span :id="id" class="visually-hidden">{{
                reason
            }}</span></span
        >
    </ElTooltip>
    <slot v-else />
</template>
