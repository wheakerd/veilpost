<script setup lang="ts">
defineProps<{ eyebrow: string; title: string; description: string }>()
</script>

<template>
    <div class="page-intro">
        <div>
            <p class="eyebrow">{{ eyebrow }}</p>
            <h1>{{ title }}</h1>
            <p class="page-description">{{ description }}</p>
        </div>
        <slot />
    </div>
</template>
