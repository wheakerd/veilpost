<script setup lang="ts">
defineProps<{ title: string; description: string }>()
</script>

<template>
    <header class="page-intro">
        <div>
            <h1>{{ title }}</h1>
            <p class="page-description">{{ description }}</p>
        </div>
        <slot />
    </header>
</template>
