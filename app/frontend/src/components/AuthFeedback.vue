<script setup lang="ts">
import { motion } from 'motion-v'
import { useInterfaceMotion } from '@/composables/useInterfaceMotion'
defineProps<{ message: string; tone?: 'error' | 'notice' }>()
const { reduceMotion, transition } = useInterfaceMotion()
</script>

<template>
    <motion.p
        v-if="message"
        :key="message"
        :class="tone === 'notice' ? 'form-notice' : 'form-error'"
        :role="tone === 'notice' ? 'status' : 'alert'"
        :initial="reduceMotion ? false : { opacity: 0, y: 4 }"
        :animate="{ opacity: 1, y: 0 }"
        :transition="transition"
        >{{ message }}</motion.p
    >
</template>
