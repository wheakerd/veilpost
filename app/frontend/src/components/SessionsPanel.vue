<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Monitor from '~icons/lucide/monitor'
import AuthFeedback from '@/components/AuthFeedback.vue'
import { authApi, authErrorMessage } from '@/api/auth'
import { useSessionExpiry } from '@/composables/useSessionExpiry'
import { formatNumber } from '@/i18n/format'
import DisabledReason from '@/components/DisabledReason.vue'

// otherSessions is null until the status loads; unavailable marks a failed load.
defineProps<{ otherSessions: number | null; unavailable?: boolean }>()
const emit = defineEmits<{ revoked: [] }>()
const { t } = useI18n()
const redirectIfExpired = useSessionExpiry()
const busy = ref(false)
const error = ref('')
const notice = ref('')

async function revoke() {
    if (busy.value) return
    busy.value = true
    error.value = ''
    notice.value = ''
    try {
        await authApi.revokeOtherSessions()
        notice.value = t('security.sessions.revoked')
        emit('revoked')
    } catch (cause) {
        if (!(await redirectIfExpired(cause)))
            error.value = authErrorMessage(cause)
    } finally {
        busy.value = false
    }
}
</script>

<template>
    <section
        class="plain-panel security-panel"
        aria-labelledby="sessions-title"
    >
        <div class="setting-heading">
            <div class="security-heading">
                <span class="security-icon"
                    ><Monitor aria-hidden="true"
                /></span>
                <div>
                    <h2 id="sessions-title">
                        {{ t('security.sessions.title') }}
                    </h2>
                    <p>
                        {{
                            unavailable
                                ? t('security.sessions.unavailable')
                                : otherSessions === null
                                  ? t('security.sessions.loading')
                                  : otherSessions
                                    ? t(
                                          'security.sessions.others',
                                          {
                                              count: formatNumber(
                                                  otherSessions,
                                              ),
                                          },
                                          otherSessions,
                                      )
                                    : t('security.sessions.onlyCurrent')
                        }}
                    </p>
                </div>
            </div>
            <DisabledReason
                :reason="otherSessions === 0 && t('security.sessions.noOthers')"
            >
                <ElButton
                    :loading="busy"
                    :disabled="!otherSessions"
                    @click="revoke"
                    >{{ t('security.sessions.revoke') }}</ElButton
                >
            </DisabledReason>
        </div>
        <AuthFeedback :message="error" class="security-feedback" />
        <AuthFeedback
            :message="notice"
            tone="notice"
            class="security-feedback"
        />
    </section>
</template>
