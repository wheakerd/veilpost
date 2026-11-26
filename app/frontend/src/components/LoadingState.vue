<script setup lang="ts">
defineProps<{ title: string }>()
</script>

<template>
    <div class="loading-state" role="status">
        <span class="boot-indicator" aria-hidden="true">
            <img src="/favicon.svg" width="36" height="36" alt="" />
        </span>
        <p>{{ title }}</p>
    </div>
</template>
