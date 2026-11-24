<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { I18nT, useI18n } from 'vue-i18n'
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
const method = ref<'totp' | 'recovery'>('totp')
const code = ref('')
const busy = ref(false)
const error = ref('')
const verified = ref(false)
const form = ref<HTMLFormElement>()
const { reduceMotion } = useInterfaceMotion()
// The authenticator code drawn as six cells over the real input.
const digits = computed(() => code.value.replace(/\D/g, '').slice(0, 6))
// An accepted code holds its success state this long before continuing.
const passedHold = 420
const now = ref(Date.now())
const remaining = computed(() =>
    Math.max(
        0,
        Math.ceil(
            ((auth.challengeExpiresAt
                ? Date.parse(auth.challengeExpiresAt)
                : 0) -
                now.value) /
                1000,
        ),
    ),
)
const countdown = computed(
    () =>
        `${Math.floor(remaining.value / 60)}:${String(remaining.value % 60).padStart(2, '0')}`,
)
let timer: ReturnType<typeof setInterval>

function expired() {
    auth.clear()
    return router.replace({
        path: '/login',
        query: {
            redirect: safeReturnPath(route.query.redirect),
            reason: 'expired',
        },
    })
}

onMounted(() => {
    timer = setInterval(() => {
        now.value = Date.now()
        if (!remaining.value && !busy.value) void expired()
    }, 1000)
})
onBeforeUnmount(() => {
    clearInterval(timer)
    code.value = ''
})

function switchMethod() {
    method.value = method.value === 'totp' ? 'recovery' : 'totp'
    code.value = ''
    error.value = ''
}

async function submit() {
    if (busy.value) return
    if (!remaining.value) return expired()
    error.value = ''
    const value = code.value.replace(/\s/g, '')
    if (method.value === 'totp' ? !/^\d{6}$/.test(value) : !value) {
        error.value =
            method.value === 'totp'
                ? t('common.code.required')
                : t('auth.verify.recoveryRequired')
        return
    }
    busy.value = true
    try {
        const session = await authApi.verify(value, method.value)
        verified.value = true
        if (!reduceMotion.value)
            await new Promise((resolve) => setTimeout(resolve, passedHold))
        auth.accept(session)
        await enterConsole(reduceMotion.value, () =>
            router.replace(safeReturnPath(route.query.redirect)),
        )
    } catch (cause) {
        verified.value = false
        if (
            cause instanceof AuthApiError &&
            ['CHALLENGE_EXPIRED', 'SESSION_EXPIRED'].includes(cause.code)
        ) {
            await expired()
        } else {
            error.value = authErrorMessage(cause)
            // A rejected code shakes the form once.
            if (
                cause instanceof AuthApiError &&
                ['INVALID_CODE', 'INVALID_RECOVERY_CODE'].includes(cause.code)
            )
                shake(form.value, reduceMotion.value)
        }
    } finally {
        code.value = ''
        busy.value = false
        if (error.value) {
            await nextTick()
            document.getElementById('verification-code')?.focus()
        }
    }
}

async function back() {
    if (busy.value) return
    busy.value = true
    error.value = ''
    try {
        await auth.logout()
        await router.replace({
            path: '/login',
            query: { redirect: safeReturnPath(route.query.redirect) },
        })
    } catch (cause) {
        error.value = authErrorMessage(cause)
    } finally {
        busy.value = false
    }
}
</script>

<template>
    <p class="auth-step readout">{{ t('auth.verify.step') }}</p>
    <h1>{{ t('auth.verify.title') }}</h1>
    <p class="auth-description">
        {{
            method === 'totp'
                ? t('auth.verify.totpDescription')
                : t('auth.verify.recoveryDescription')
        }}
    </p>
    <I18nT keypath="auth.verify.remaining" tag="p" class="verification-window">
        <template #time
            ><strong>{{ countdown }}</strong></template
        >
    </I18nT>
    <form
        ref="form"
        class="auth-form"
        :aria-busy="busy"
        @submit.prevent="submit"
    >
        <div class="form-field">
            <label for="verification-code">{{
                method === 'totp'
                    ? t('common.code.label')
                    : t('auth.verify.recoveryLabel')
            }}</label>
            <div
                class="code-field"
                :class="{
                    'is-cells': method === 'totp',
                    'is-checking': busy && !verified,
                    'is-passed': verified,
                }"
            >
                <ElInput
                    id="verification-code"
                    :key="method"
                    v-model="code"
                    class="verification-input"
                    type="text"
                    :inputmode="method === 'totp' ? 'numeric' : 'text'"
                    :autocomplete="method === 'totp' ? 'one-time-code' : 'off'"
                    :maxlength="method === 'totp' ? 12 : 128"
                    autocapitalize="none"
                    :spellcheck="false"
                    aria-describedby="verification-help"
                    :disabled="busy"
                    required
                />
                <div
                    v-if="method === 'totp'"
                    class="code-cells"
                    aria-hidden="true"
                >
                    <span
                        v-for="index in 6"
                        :key="index"
                        class="code-cell"
                        :class="{
                            filled: digits.length >= index,
                            current: digits.length === index - 1,
                        }"
                        :style="{ '--cell': index - 1 }"
                        >{{ digits[index - 1] ?? '' }}</span
                    >
                </div>
            </div>
            <small id="verification-help">{{
                method === 'totp'
                    ? t('auth.verify.totpHelp')
                    : t('auth.verify.recoveryHelp')
            }}</small>
        </div>
        <AuthFeedback :message="error" />
        <p v-if="verified" class="form-notice" role="status">
            {{ t('auth.verify.passed') }}
        </p>
        <ElButton
            class="form-submit"
            type="primary"
            size="large"
            native-type="submit"
            :loading="busy"
            >{{
                busy ? t('auth.verify.verifying') : t('auth.verify.submit')
            }}</ElButton
        >
    </form>
    <div class="auth-alternatives">
        <ElButton link type="primary" :disabled="busy" @click="switchMethod">{{
            method === 'totp'
                ? t('auth.verify.useRecovery')
                : t('auth.verify.useAuthenticator')
        }}</ElButton>
        <ElButton link :disabled="busy" @click="back">{{
            t('auth.verify.back')
        }}</ElButton>
    </div>
</template>
