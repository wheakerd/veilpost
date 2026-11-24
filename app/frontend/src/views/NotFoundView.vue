<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
const router = useRouter()
const { t } = useI18n()
</script>

<template>
    <div class="empty-state">
        <p class="eyebrow">404</p>
        <h1>{{ t('common.notFound.title') }}</h1>
        <p>{{ t('common.notFound.description') }}</p>
        <ElButton type="primary" @click="router.push('/')">{{
            t('common.notFound.back')
        }}</ElButton>
    </div>
</template>
