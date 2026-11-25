<script setup lang="ts">
import { useTemplateRef } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AccountSecurity from '@/components/AccountSecurity.vue'
import PageIntro from '@/components/PageIntro.vue'

const { t } = useI18n()
const security = useTemplateRef<{ confirmLeave: () => Promise<boolean> }>(
    'security',
)

onBeforeRouteLeave(() => security.value?.confirmLeave() ?? true)
</script>

<template>
    <div>
        <PageIntro
            :eyebrow="t('common.shell.groups.security')"
            :title="t('common.routes.security')"
            :description="t('security.description')"
        />
        <AccountSecurity ref="security" />
    </div>
</template>
