<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import Key from '~icons/lucide/key-round'
import PasswordField from '@/components/PasswordField.vue'
import Reveal from '@/components/Reveal.vue'
import AuthFeedback from '@/components/AuthFeedback.vue'
import { authApi, authErrorMessage } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { useSessionExpiry } from '@/composables/useSessionExpiry'
import DiscardPrompt from '@/components/DiscardPrompt.vue'
import { useDiscardGuard } from '@/composables/useDiscardGuard'

const emit = defineEmits<{ changed: [] }>()
const id = useId()
const currentPasswordId = `${id}-current-password`
const { t } = useI18n()
const auth = useAuthStore()
const redirectIfExpired = useSessionExpiry()
const editing = ref(false)
const current = ref('')
const next = ref('')
const confirmation = ref('')
const busy = ref(false)
const error = ref('')
const notice = ref('')
defineExpose({ busy })

function clearFields() {
    current.value = ''
    next.value = ''
    confirmation.value = ''
}

function close() {
    clearFields()
    editing.value = false
    error.value = ''
}

// Unsaved edits ask before Cancel or Escape drops them.
const {
    confirming: passwordConfirming,
    request: passwordRequest,
    keep: passwordKeep,
    discard: passwordDiscard,
} = useDiscardGuard({
    open: () => editing.value && 'password',
    signature: () =>
        [current.value, next.value, confirmation.value]
            .map((value) => value !== '')
            .join('|'),
    close: close,
})

async function open() {
    close()
    notice.value = ''
    editing.value = true
    await nextTick()
    document.getElementById(currentPasswordId)?.focus()
}

onBeforeUnmount(clearFields)

async function submit() {
    if (busy.value) return
    error.value = ''
    if (!current.value || !next.value) {
        error.value = t('security.password.missing')
        return
    }
    if (next.value !== confirmation.value) {
        error.value = t('security.password.mismatch')
        return
    }
    busy.value = true
    try {
        auth.accept(await authApi.changePassword(current.value, next.value))
        close()
        notice.value = t('security.password.changed')
        emit('changed')
    } catch (cause) {
        if (await redirectIfExpired(cause, clearFields)) return
        error.value = authErrorMessage(cause)
        current.value = ''
    } finally {
        busy.value = false
    }
    if (error.value) {
        await nextTick()
        document.getElementById(currentPasswordId)?.focus()
    }
}
</script>

<template>
    <section
        class="plain-panel security-panel"
        :aria-labelledby="`${id}-password-title`"
    >
        <div class="setting-heading">
            <div class="security-heading">
                <span class="security-icon"><Key aria-hidden="true" /></span>
                <div>
                    <h2 :id="`${id}-password-title`">
                        {{ t('security.password.title') }}
                    </h2>
                    <p>{{ t('security.password.description') }}</p>
                </div>
            </div>
            <ElButton v-if="!editing" @click="open">{{
                t('security.password.change')
            }}</ElButton>
        </div>
        <Reveal>
            <form
                v-if="editing"
                class="security-form auth-form"
                :aria-busy="busy"
                @keydown.esc.stop="passwordRequest"
                @submit.prevent="submit"
            >
                <PasswordField
                    :id="currentPasswordId"
                    v-model="current"
                    :label="t('common.password.current')"
                    :disabled="busy"
                />
                <PasswordField
                    :id="`${id}-new-password`"
                    v-model="next"
                    :label="t('security.password.next')"
                    autocomplete="new-password"
                    :disabled="busy"
                />
                <PasswordField
                    :id="`${id}-confirm-password`"
                    v-model="confirmation"
                    :label="t('security.password.confirm')"
                    autocomplete="new-password"
                    :disabled="busy"
                />
                <p class="field-hint">{{ t('security.password.hint') }}</p>
                <div v-if="!passwordConfirming" class="form-actions">
                    <ElButton
                        type="primary"
                        native-type="submit"
                        :loading="busy"
                        >{{ t('security.password.save') }}</ElButton><ElButton
                        :disabled="busy"
                        @click="passwordRequest"
                        >{{ t('common.actions.cancel') }}</ElButton
                    >
                </div>
                <DiscardPrompt
                    v-else
                    @keep="passwordKeep"
                    @discard="passwordDiscard"
                />
            </form>
        </Reveal>
        <AuthFeedback :message="error" class="security-feedback" />
        <AuthFeedback
            :message="notice"
            tone="notice"
            class="security-feedback"
        />
    </section>
</template>
