<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import FileQuestion from '~icons/lucide/file-question-mark'
const router = useRouter()
const { t } = useI18n()
</script>

<template>
    <section class="plain-panel empty-state not-found">
        <span class="empty-icon" aria-hidden="true"><FileQuestion /></span>
        <p class="readout">404</p>
        <h1>{{ t('common.notFound.title') }}</h1>
        <p>{{ t('common.notFound.description') }}</p>
        <ElButton type="primary" @click="router.push('/')">{{
            t('common.notFound.back')
        }}</ElButton>
    </section>
</template>
