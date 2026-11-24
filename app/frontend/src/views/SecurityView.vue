<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { I18nT, useI18n } from 'vue-i18n'
import Lock from '~icons/lucide/lock'
import PageIntro from '@/components/PageIntro.vue'
import PasswordField from '@/components/PasswordField.vue'
import PasswordPanel from '@/components/PasswordPanel.vue'
import SessionsPanel from '@/components/SessionsPanel.vue'
import {
    authApi,
    AuthApiError,
    authErrorMessage,
    type SecurityStatus,
    type TwoFactorSetup,
} from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { motion } from 'motion-v'
import { useInterfaceMotion } from '@/composables/useInterfaceMotion'
import { useSessionExpiry } from '@/composables/useSessionExpiry'
import { formatNumber } from '@/i18n/format'
import AuthFeedback from '@/components/AuthFeedback.vue'
import DisabledReason from '@/components/DisabledReason.vue'

const { t } = useI18n()
const auth = useAuthStore()
const { reduceMotion, transition } = useInterfaceMotion()
const redirectIfExpired = useSessionExpiry()
const phase = ref<
    'idle' | 'password' | 'setup' | 'recovery' | 'disable' | 'regenerate'
>('idle')
const status = ref<SecurityStatus | null>(null)
const statusUnavailable = ref(false)
const regenerated = ref(false)
const password = ref('')
const code = ref('')
const setup = ref<TwoFactorSetup | null>(null)
const qrUnavailable = ref(false)
const recoveryCodes = ref<string[]>([])
const saved = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const enabled = computed(() => auth.user?.twoFactorEnabled ?? false)
const recoveryRemaining = computed(() => {
    if (!status.value) return ''
    const count = status.value.recoveryCodesRemaining
    return t(
        count <= 2
            ? 'security.twoFactor.recoveryLow'
            : 'security.twoFactor.recoveryRemaining',
        { count: formatNumber(count) },
        count,
    )
})
let timer: ReturnType<typeof setInterval>

function reset() {
    phase.value = 'idle'
    password.value = ''
    code.value = ''
    setup.value = null
    qrUnavailable.value = false
    recoveryCodes.value = []
    saved.value = false
    regenerated.value = false
    error.value = ''
    notice.value = ''
}

async function loadStatus() {
    try {
        status.value = await authApi.securityStatus()
        statusUnavailable.value = false
    } catch (cause) {
        if (!(await redirectIfExpired(cause, reset)))
            statusUnavailable.value = true
    }
}

function begin(next: 'password' | 'disable' | 'regenerate') {
    reset()
    phase.value = next
}

function warnBeforeUnload(event: BeforeUnloadEvent) {
    if (recoveryCodes.value.length) event.preventDefault()
}
// Leaving with unsaved recovery codes asks first; staying is the default.
onBeforeRouteLeave(async () => {
    if (!recoveryCodes.value.length) return true
    try {
        await ElMessageBox.confirm(
            t('security.recovery.leaveConfirm'),
            t('security.recovery.leaveTitle'),
            {
                type: 'warning',
                confirmButtonText: t('security.recovery.leave'),
                cancelButtonText: t('security.recovery.stay'),
                autofocus: false,
            },
        )
        return true
    } catch {
        return false
    }
})
onMounted(() => {
    void loadStatus()
    window.addEventListener('beforeunload', warnBeforeUnload)
    timer = setInterval(() => {
        if (
            setup.value &&
            Date.parse(setup.value.expiresAt) <= Date.now() &&
            !busy.value
        ) {
            reset()
            error.value = t('security.setup.expired')
        }
    }, 1000)
})
onBeforeUnmount(() => {
    clearInterval(timer)
    window.removeEventListener('beforeunload', warnBeforeUnload)
    reset()
})

async function handleError(cause: unknown) {
    if (await redirectIfExpired(cause, reset)) return
    if (cause instanceof AuthApiError && cause.code === 'SETUP_EXPIRED') reset()
    error.value = authErrorMessage(cause)
}

async function startSetup() {
    if (busy.value || !password.value) return
    busy.value = true
    error.value = ''
    try {
        const result = await authApi.beginSetup(password.value)
        if (Date.parse(result.expiresAt) <= Date.now())
            throw new AuthApiError('SETUP_EXPIRED')
        setup.value = result
        phase.value = 'setup'
    } catch (cause) {
        await handleError(cause)
    } finally {
        password.value = ''
        busy.value = false
        await nextTick()
        document
            .getElementById(
                phase.value === 'setup' ? 'setup-scan-title' : 'setup-password',
            )
            ?.focus()
    }
}

async function confirmSetup() {
    if (busy.value || !setup.value) return
    error.value = ''
    const value = code.value.replace(/\s/g, '')
    if (!/^\d{6}$/.test(value)) {
        error.value = t('common.code.required')
        return
    }
    busy.value = true
    try {
        const result = await authApi.confirmSetup(setup.value.setupId, value)
        auth.accept(result.session)
        setup.value = null
        recoveryCodes.value = result.recoveryCodes
        phase.value = 'recovery'
        void loadStatus()
    } catch (cause) {
        await handleError(cause)
    } finally {
        code.value = ''
        busy.value = false
        await nextTick()
        document
            .getElementById(
                phase.value === 'recovery' ? 'recovery-title' : 'setup-code',
            )
            ?.focus()
    }
}

async function disable() {
    if (busy.value || !password.value) return
    error.value = ''
    const value = code.value.replace(/\s/g, '')
    if (!/^\d{6}$/.test(value)) {
        error.value = t('common.code.required')
        return
    }
    busy.value = true
    try {
        auth.accept(await authApi.disable(password.value, value))
        reset()
        notice.value = t('security.disable.done')
        void loadStatus()
    } catch (cause) {
        await handleError(cause)
    } finally {
        password.value = ''
        code.value = ''
        busy.value = false
        if (error.value) {
            await nextTick()
            document.getElementById('disable-password')?.focus()
        }
    }
}

async function regenerate() {
    if (busy.value || !password.value) return
    error.value = ''
    const value = code.value.replace(/\s/g, '')
    if (!/^\d{6}$/.test(value)) {
        error.value = t('common.code.required')
        return
    }
    busy.value = true
    try {
        recoveryCodes.value = await authApi.regenerateRecoveryCodes(
            password.value,
            value,
        )
        regenerated.value = true
        phase.value = 'recovery'
        void loadStatus()
    } catch (cause) {
        await handleError(cause)
    } finally {
        password.value = ''
        code.value = ''
        busy.value = false
        await nextTick()
        document
            .getElementById(
                phase.value === 'recovery'
                    ? 'recovery-title'
                    : 'regenerate-password',
            )
            ?.focus()
    }
}

async function copy(value: string) {
    try {
        await navigator.clipboard.writeText(value)
        ElMessage.success({
            message: t('security.recovery.copied'),
            grouping: true,
        })
    } catch {
        ElMessage.error({
            message: t('common.actions.copyFailed'),
            grouping: true,
        })
    }
}

function finish() {
    if (!saved.value) return
    const wasRegenerated = regenerated.value
    reset()
    notice.value = wasRegenerated
        ? t('security.recovery.regenerated')
        : t('security.recovery.enabled')
}
</script>

<template>
    <div>
        <PageIntro
            :eyebrow="t('common.shell.groups.security')"
            :title="t('common.routes.security')"
            :description="t('security.description')"
        />
        <section
            class="plain-panel security-panel"
            aria-labelledby="security-title"
        >
            <div class="setting-heading">
                <div class="security-heading">
                    <span class="security-icon"
                        ><Lock aria-hidden="true"
                    /></span>
                    <div>
                        <h2 id="security-title">
                            {{ t('security.twoFactor.title') }}
                        </h2>
                        <p>{{ t('security.twoFactor.description') }}</p>
                    </div>
                </div>
                <ElTag :type="enabled ? 'success' : 'info'" effect="plain">{{
                    enabled
                        ? t('security.twoFactor.enabled')
                        : t('security.twoFactor.disabled')
                }}</ElTag>
            </div>
            <motion.div
                :key="phase"
                :initial="reduceMotion ? false : { opacity: 0, y: 6 }"
                :animate="{ opacity: 1, y: 0 }"
                :transition="transition"
            >
                <div v-if="phase === 'idle'" class="security-summary">
                    <div>
                        <h3>
                            {{
                                enabled
                                    ? t('security.twoFactor.protectedTitle')
                                    : t('security.twoFactor.unprotectedTitle')
                            }}
                        </h3>
                        <p>
                            {{
                                enabled
                                    ? t('security.twoFactor.protectedBody')
                                    : t('security.twoFactor.unprotectedBody')
                            }}
                        </p>
                        <p
                            v-if="enabled && status"
                            :class="{
                                'recovery-low':
                                    status.recoveryCodesRemaining <= 2,
                            }"
                            data-testid="recovery-remaining"
                        >
                            {{ recoveryRemaining }}
                        </p>
                    </div>
                    <div class="form-actions">
                        <ElButton v-if="enabled" @click="begin('regenerate')">{{
                            t('security.twoFactor.regenerate')
                        }}</ElButton>
                        <ElButton
                            :type="enabled ? 'default' : 'primary'"
                            @click="begin(enabled ? 'disable' : 'password')"
                            >{{
                                enabled
                                    ? t('security.twoFactor.disable')
                                    : t('security.twoFactor.enable')
                            }}</ElButton
                        >
                    </div>
                </div>

                <form
                    v-if="phase === 'password'"
                    class="security-form auth-form"
                    :aria-busy="busy"
                    @submit.prevent="startSetup"
                >
                    <div>
                        <h3>{{ t('security.confirm.title') }}</h3>
                        <p class="auth-description">
                            {{ t('security.confirm.description') }}
                        </p>
                    </div>
                    <PasswordField
                        id="setup-password"
                        v-model="password"
                        :label="t('common.password.current')"
                        :disabled="busy"
                    />
                    <div class="form-actions">
                        <ElButton
                            type="primary"
                            native-type="submit"
                            :loading="busy"
                            >{{
                                t('security.confirm.submit')
                            }}</ElButton><ElButton
                            :disabled="busy"
                            @click="reset"
                            >{{ t('common.actions.cancel') }}</ElButton
                        >
                    </div>
                </form>

                <div v-if="phase === 'setup' && setup" class="setup-content">
                    <ol class="setup-steps">
                        <li>
                            <h3 id="setup-scan-title" tabindex="-1">
                                {{ t('security.setup.scanTitle') }}
                            </h3>
                            <p>{{ t('security.setup.scanBody') }}</p>
                        </li>
                    </ol>
                    <div class="setup-credentials">
                        <img
                            v-if="setup.qrCodeDataUrl && !qrUnavailable"
                            :src="setup.qrCodeDataUrl"
                            class="setup-qr"
                            width="320"
                            height="320"
                            :alt="t('security.setup.qrAlt')"
                            @error="qrUnavailable = true"
                        />
                        <div class="setup-instructions">
                            <div class="setup-account">
                                <span>{{ t('common.app.name') }}</span>
                                <strong>{{ auth.user?.username }}</strong>
                            </div>
                            <p>{{ t('security.setup.added') }}</p>
                            <p
                                v-if="!setup.qrCodeDataUrl || qrUnavailable"
                                role="status"
                            >
                                {{ t('security.setup.qrUnavailable') }}
                            </p>
                            <details
                                class="setup-manual"
                                :open="!setup.qrCodeDataUrl || qrUnavailable"
                            >
                                <summary>
                                    {{ t('security.setup.manualSummary') }}
                                </summary>
                                <div class="setup-key">
                                    <span>{{
                                        t('security.setup.manualKey')
                                    }}</span>
                                    <code data-testid="setup-secret">{{
                                        setup.secret
                                    }}</code>
                                    <ElButton
                                        size="small"
                                        @click="copy(setup.secret)"
                                        >{{
                                            t('security.setup.copyKey')
                                        }}</ElButton
                                    >
                                </div>
                            </details>
                        </div>
                    </div>
                    <form
                        class="security-form auth-form"
                        :aria-busy="busy"
                        @submit.prevent="confirmSetup"
                    >
                        <ol class="setup-steps" start="2">
                            <li>
                                <strong>{{
                                    t('security.setup.verifyTitle')
                                }}</strong>
                                <p>{{ t('security.setup.verifyBody') }}</p>
                            </li>
                        </ol>
                        <div class="form-field">
                            <label for="setup-code">{{
                                t('common.code.label')
                            }}</label><ElInput
                                id="setup-code"
                                v-model="code"
                                class="verification-input"
                                type="text"
                                inputmode="numeric"
                                autocomplete="one-time-code"
                                maxlength="12"
                                required
                                :disabled="busy"
                            />
                        </div>
                        <div class="form-actions">
                            <ElButton
                                type="primary"
                                native-type="submit"
                                :loading="busy"
                                >{{
                                    t('security.setup.submit')
                                }}</ElButton><ElButton
                                :disabled="busy"
                                @click="reset"
                                >{{ t('security.setup.cancel') }}</ElButton
                            >
                        </div>
                    </form>
                </div>

                <div v-if="phase === 'recovery'" class="recovery-content">
                    <h3 id="recovery-title" tabindex="-1">
                        {{ t('security.recovery.title') }}
                    </h3>
                    <p>{{ t('security.recovery.body') }}</p>
                    <ul
                        class="recovery-codes"
                        :aria-label="t('security.recovery.listLabel')"
                    >
                        <li
                            v-for="recoveryCode in recoveryCodes"
                            :key="recoveryCode"
                        >
                            <code>{{ recoveryCode }}</code>
                        </li>
                    </ul>
                    <ElButton @click="copy(recoveryCodes.join('\n'))">{{
                        t('security.recovery.copy')
                    }}</ElButton>
                    <ElCheckbox v-model="saved" class="save-confirmation">{{
                        t('security.recovery.saved')
                    }}</ElCheckbox>
                    <DisabledReason
                        :reason="!saved && t('security.recovery.confirmFirst')"
                    >
                        <ElButton
                            type="primary"
                            :disabled="!saved"
                            @click="finish"
                            >{{
                                regenerated
                                    ? t('security.recovery.finish')
                                    : t('security.recovery.finishSetup')
                            }}</ElButton
                        >
                    </DisabledReason>
                </div>

                <form
                    v-if="phase === 'regenerate'"
                    class="security-form auth-form"
                    :aria-busy="busy"
                    @submit.prevent="regenerate"
                >
                    <div>
                        <h3>{{ t('security.regenerate.title') }}</h3>
                        <p class="auth-description">
                            {{ t('security.regenerate.description') }}
                        </p>
                    </div>
                    <PasswordField
                        id="regenerate-password"
                        v-model="password"
                        :label="t('common.password.current')"
                        :disabled="busy"
                    />
                    <div class="form-field">
                        <label for="regenerate-code">{{
                            t('common.code.label')
                        }}</label><ElInput
                            id="regenerate-code"
                            v-model="code"
                            type="text"
                            inputmode="numeric"
                            autocomplete="one-time-code"
                            maxlength="12"
                            required
                            :disabled="busy"
                        />
                    </div>
                    <div class="form-actions">
                        <ElButton
                            type="primary"
                            native-type="submit"
                            :loading="busy"
                            >{{
                                t('security.regenerate.submit')
                            }}</ElButton><ElButton
                            :disabled="busy"
                            @click="reset"
                            >{{ t('common.actions.cancel') }}</ElButton
                        >
                    </div>
                </form>

                <form
                    v-if="phase === 'disable'"
                    class="security-form auth-form"
                    :aria-busy="busy"
                    @submit.prevent="disable"
                >
                    <div>
                        <h3>{{ t('security.disable.title') }}</h3>
                        <p class="auth-description">
                            {{ t('security.disable.description') }}
                        </p>
                    </div>
                    <PasswordField
                        id="disable-password"
                        v-model="password"
                        :label="t('common.password.current')"
                        :disabled="busy"
                    />
                    <div class="form-field">
                        <label for="disable-code">{{
                            t('common.code.label')
                        }}</label><ElInput
                            id="disable-code"
                            v-model="code"
                            type="text"
                            inputmode="numeric"
                            autocomplete="one-time-code"
                            maxlength="12"
                            required
                            :disabled="busy"
                        />
                    </div>
                    <div class="form-actions">
                        <ElButton
                            type="danger"
                            native-type="submit"
                            :loading="busy"
                            >{{
                                t('security.disable.submit')
                            }}</ElButton><ElButton
                            :disabled="busy"
                            @click="reset"
                            >{{ t('common.actions.cancel') }}</ElButton
                        >
                    </div>
                </form>
            </motion.div>
            <AuthFeedback :message="error" class="security-feedback" />
            <AuthFeedback
                :message="notice"
                tone="notice"
                class="security-feedback"
            />
        </section>
        <PasswordPanel @changed="loadStatus" />
        <SessionsPanel
            :other-sessions="status?.otherSessions ?? null"
            :unavailable="statusUnavailable"
            @revoked="loadStatus"
        />
        <I18nT keypath="security.lostEverything" tag="p" class="inline-note">
            <template #command
                ><code>make admin-reset-two-factor</code></template
            >
        </I18nT>
    </div>
</template>
