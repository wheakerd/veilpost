<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { I18nT, useI18n } from 'vue-i18n'
import ArrowRight from '~icons/lucide/arrow-right'
import PasswordField from '@/components/PasswordField.vue'
import { authApi, AuthApiError, authErrorMessage } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { safeReturnPath } from '@/router/returnPath'
import AuthFeedback from '@/components/AuthFeedback.vue'
import { useInterfaceMotion } from '@/composables/useInterfaceMotion'
import { enterConsole, shake } from '@/composables/useSignInExit'

const { t } = useI18n()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const username = ref('')
const password = ref('')
const busy = ref(false)
const error = ref('')
const reconnecting = ref(false)
const form = ref<HTMLFormElement>()
const { reduceMotion } = useInterfaceMotion()

async function retry() {
    if (busy.value) return
    busy.value = true
    reconnecting.value = true
    error.value = ''
    try {
        await auth.restore()
        if (auth.user || auth.challengeExpiresAt)
            await router.replace(safeReturnPath(route.query.redirect))
    } finally {
        busy.value = false
        reconnecting.value = false
        await nextTick()
        if (!auth.setupRequired && !auth.serviceError)
            document.getElementById('username')?.focus()
    }
}

async function submit() {
    if (busy.value || auth.setupRequired) return
    error.value = ''
    if (!username.value.trim() || !password.value) {
        error.value = t('auth.login.missingCredentials')
        return
    }
    busy.value = true
    try {
        const session = await authApi.login(
            username.value.trim(),
            password.value,
        )
        auth.accept(session)
        const redirect = safeReturnPath(route.query.redirect)
        if (session.status === 'mfa_required')
            await router.replace({ path: '/login/verify', query: { redirect } })
        else
            await enterConsole(reduceMotion.value, () =>
                router.replace(redirect),
            )
    } catch (cause) {
        error.value = authErrorMessage(cause)
        // Rejected credentials shake the form once.
        if (
            cause instanceof AuthApiError &&
            cause.code === 'INVALID_CREDENTIALS'
        )
            shake(form.value, reduceMotion.value)
    } finally {
        password.value = ''
        busy.value = false
        if (error.value) {
            await nextTick()
            document.getElementById('password')?.focus()
        }
    }
}
</script>

<template>
    <p class="auth-step readout">{{ t('auth.login.step') }}</p>
    <h1>{{ t('auth.login.title') }}</h1>
    <p class="auth-description">{{ t('auth.login.description') }}</p>
    <p
        v-if="route.query.reason === 'expired'"
        class="form-notice"
        role="status"
    >
        {{ t('auth.login.expired') }}
    </p>
    <AuthFeedback :message="auth.serviceError" tone="notice" />
    <ElButton
        v-if="auth.serviceError"
        class="retry-connection"
        :loading="busy"
        @click="retry"
        >{{ t('auth.login.retry') }}</ElButton
    >
    <I18nT
        v-if="auth.setupRequired"
        keypath="auth.login.setupRequired"
        tag="p"
        class="form-notice setup-required"
        role="status"
    >
        <template #command><code>make admin-create</code></template>
    </I18nT>
    <ElButton
        v-if="auth.setupRequired"
        class="retry-connection"
        :loading="busy"
        @click="retry"
        >{{ t('auth.login.recheck') }}</ElButton
    >
    <form
        ref="form"
        class="auth-form"
        :aria-busy="busy"
        @submit.prevent="submit"
    >
        <div class="form-field">
            <label for="username">{{ t('auth.login.username') }}</label>
            <ElInput
                id="username"
                v-model="username"
                name="username"
                autocomplete="username"
                autocapitalize="none"
                :spellcheck="false"
                required
                maxlength="64"
                :disabled="busy || auth.setupRequired"
            />
        </div>
        <PasswordField
            id="password"
            v-model="password"
            :disabled="busy || auth.setupRequired"
        />
        <AuthFeedback id="login-error" :message="error" />
        <ElButton
            class="form-submit"
            type="primary"
            size="large"
            native-type="submit"
            :loading="busy"
            :disabled="auth.setupRequired"
        >
            {{
                reconnecting
                    ? t('auth.login.connecting')
                    : busy
                      ? t('auth.login.signingIn')
                      : t('auth.login.submit')
            }}
            <ArrowRight v-if="!busy" class="button-icon" />
        </ElButton>
        <I18nT keypath="auth.login.keyHint" tag="p" class="key-hint">
            <template #key><kbd>Enter</kbd></template>
        </I18nT>
    </form>
    <p class="auth-help">
        {{ t('auth.login.helpTwoFactor') }}<br />{{
            t('auth.login.helpContact')
        }}
    </p>
</template>
