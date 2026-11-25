<script setup lang="ts">
defineProps<{ username: string }>()
</script>

<template>
    <span class="account-avatar" aria-hidden="true">{{
        username.slice(0, 1).toUpperCase()
    }}</span>
</template>
