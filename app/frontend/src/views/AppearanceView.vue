<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import PageIntro from '@/components/PageIntro.vue'
import PreferencesPanel from '@/components/PreferencesPanel.vue'

const { t } = useI18n()
</script>

<template>
    <div>
        <PageIntro
            :title="t('common.routes.appearance')"
            :description="t('appearance.description')"
        />
        <section
            class="preferences-page"
            :aria-label="t('common.routes.appearance')"
        >
            <PreferencesPanel id-prefix="page-preferences" />
        </section>
    </div>
</template>
