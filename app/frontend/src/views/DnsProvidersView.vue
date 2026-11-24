<script setup lang="ts">
import {
    computed,
    nextTick,
    onBeforeUnmount,
    onMounted,
    reactive,
    ref,
} from 'vue'
import { useI18n } from 'vue-i18n'
import Key from '~icons/lucide/key-round'
import Plus from '~icons/lucide/plus'
import PageIntro from '@/components/PageIntro.vue'
import Reveal from '@/components/Reveal.vue'
import PasswordField from '@/components/PasswordField.vue'
import AuthFeedback from '@/components/AuthFeedback.vue'
import { AuthApiError, authErrorMessage } from '@/api/auth'
import {
    certsApi,
    dnsProviders,
    providerFields,
    type DnsCredential,
    type DnsProvider,
    type SecretField,
} from '@/api/certs'
import { useSessionExpiry } from '@/composables/useSessionExpiry'
import { formatDateTime, formatNumber } from '@/i18n/format'
import '@/styles/_certs.scss'
import { useCachedPage } from '@/composables/useCachedPage'
import DiscardPrompt from '@/components/DiscardPrompt.vue'
import { useDiscardGuard } from '@/composables/useDiscardGuard'

const refreshInterval = 30_000

const { t } = useI18n()
const redirectIfExpired = useSessionExpiry()
const credentials = ref<DnsCredential[] | null>(null)
const unavailable = ref(false)
// The open form: creating, editing or deleting one credential, if any.
const mode = ref<
    | { kind: 'create' }
    | { kind: 'edit'; credential: DnsCredential }
    | { kind: 'delete'; credential: DnsCredential }
    | null
>(null)
const name = ref('')
const provider = ref<DnsProvider>('cloudflare')
// Secret values and the password stay in this component's memory only and
// are cleared whenever the form closes. Stored values are never loaded.
const secrets = reactive<Partial<Record<SecretField, string>>>({})
const password = ref('')
const busy = ref(false)
const verifying = ref<string | null>(null)
const error = ref('')
const notice = ref('')
let refreshTimer: ReturnType<typeof setInterval> | undefined

const summary = computed(() => {
    if (unavailable.value && !credentials.value)
        return t('common.status.unavailable')
    if (!credentials.value) return t('common.status.loading')
    const count = credentials.value.length
    return t('certs.dns.count', { count: formatNumber(count) }, count)
})

const formProvider = computed(() =>
    mode.value?.kind === 'edit'
        ? mode.value.credential.provider
        : provider.value,
)
const fields = computed(() => providerFields[formProvider.value])

function clearSecrets() {
    for (const key of Object.keys(secrets) as SecretField[])
        Reflect.deleteProperty(secrets, key)
    password.value = ''
}

function closeForms() {
    mode.value = null
    name.value = ''
    provider.value = 'cloudflare'
    clearSecrets()
    error.value = ''
}

// Unsaved edits ask before Cancel or Escape drops them.
const {
    confirming: credentialConfirming,
    request: credentialRequest,
    keep: credentialKeep,
    discard: credentialDiscard,
} = useDiscardGuard({
    open: () =>
        mode.value && mode.value.kind !== 'delete' ? mode.value.kind : false,
    signature: () =>
        [
            name.value,
            provider.value,
            password.value !== '',
            ...Object.values(secrets).map((value) => Boolean(value)),
        ].join('|'),
    close: closeForms,
})

async function handleError(cause: unknown) {
    if (await redirectIfExpired(cause, clearSecrets)) return true
    error.value = authErrorMessage(cause)
    return false
}

async function load(background = false) {
    try {
        credentials.value = await certsApi.listCredentials({ background })
        unavailable.value = false
    } catch (cause) {
        if (await redirectIfExpired(cause, clearSecrets)) return
        unavailable.value = true
    }
}

async function focusField(id: string) {
    await nextTick()
    document.getElementById(id)?.focus()
}

async function openCreate() {
    closeForms()
    notice.value = ''
    mode.value = { kind: 'create' }
    await focusField('credential-name')
}

async function openEdit(credential: DnsCredential) {
    closeForms()
    notice.value = ''
    mode.value = { kind: 'edit', credential }
    name.value = credential.name
    await focusField('credential-name')
}

async function openDelete(credential: DnsCredential) {
    closeForms()
    notice.value = ''
    mode.value = { kind: 'delete', credential }
    await focusField('credential-delete-password')
}

function isSet(credential: DnsCredential, field: SecretField) {
    return credential.fields[field] === true
}

async function submit() {
    if (busy.value || !mode.value || mode.value.kind === 'delete') return
    error.value = ''
    const editing = mode.value.kind === 'edit' ? mode.value.credential : null
    // Only fields the user typed into are sent; editing keeps the others.
    const values: Partial<Record<SecretField, string>> = {}
    for (const field of fields.value) {
        const value = secrets[field.name]?.trim()
        if (value) values[field.name] = value
    }
    const missing = fields.value.some(
        (field) =>
            field.required &&
            !values[field.name] &&
            !(editing && isSet(editing, field.name)),
    )
    if (!name.value.trim() || !password.value || missing) {
        error.value = t('certs.dns.form.missing')
        return
    }
    busy.value = true
    try {
        const saved = editing
            ? await certsApi.updateCredential(
                  editing.id,
                  name.value.trim(),
                  values,
                  password.value,
              )
            : await certsApi.createCredential(
                  name.value.trim(),
                  provider.value,
                  values,
                  password.value,
              )
        closeForms()
        notice.value = t(
            editing ? 'certs.dns.form.updated' : 'certs.dns.form.created',
            { name: saved.name },
        )
        await load()
    } catch (cause) {
        password.value = ''
        if (await handleError(cause)) return
        const code = cause instanceof AuthApiError ? cause.code : ''
        if (code === 'CREDENTIAL_NOT_FOUND') {
            closeForms()
            error.value = authErrorMessage(cause)
            await load()
            return
        }
        await focusField(
            code === 'INVALID_CREDENTIAL_NAME'
                ? 'credential-name'
                : code === 'INVALID_SECRETS'
                  ? `credential-${fields.value[0]?.name}`
                  : 'credential-password',
        )
    } finally {
        busy.value = false
    }
}

async function submitDelete() {
    if (busy.value || mode.value?.kind !== 'delete') return
    const { credential } = mode.value
    error.value = ''
    if (!password.value) {
        error.value = t('certs.dns.form.missingPassword')
        return
    }
    busy.value = true
    try {
        await certsApi.deleteCredential(credential.id, password.value)
        closeForms()
        notice.value = t('certs.dns.delete.done', { name: credential.name })
        await load()
    } catch (cause) {
        password.value = ''
        if (await handleError(cause)) return
        if (
            cause instanceof AuthApiError &&
            ['CREDENTIAL_NOT_FOUND', 'CREDENTIAL_IN_USE'].includes(cause.code)
        ) {
            closeForms()
            error.value = authErrorMessage(cause)
            await load()
        }
    } finally {
        busy.value = false
    }
}

async function verify(credential: DnsCredential) {
    if (busy.value) return
    closeForms()
    notice.value = ''
    busy.value = true
    verifying.value = credential.id
    try {
        await certsApi.verifyCredential(credential.id)
        notice.value = t('certs.dns.verify.done', { name: credential.name })
    } catch (cause) {
        await handleError(cause)
    } finally {
        busy.value = false
        verifying.value = null
    }
}

function startTimer() {
    refreshTimer = setInterval(() => {
        if (!document.hidden && !mode.value) void load(true)
    }, refreshInterval)
}

onMounted(() => {
    void load()
    startTimer()
})
// Hidden in the page cache: typed provider secrets leave memory.
useCachedPage({
    pause: () => {
        clearInterval(refreshTimer)
        clearSecrets()
    },
    resume: () => {
        void load(true)
        startTimer()
    },
})
onBeforeUnmount(() => {
    clearInterval(refreshTimer)
    clearSecrets()
})
</script>

<template>
    <div>
        <PageIntro
            :eyebrow="t('common.shell.groups.security')"
            :title="t('common.routes.dnsProviders')"
            :description="t('certs.dns.description')"
        >
            <ElButton
                type="primary"
                size="large"
                :disabled="mode?.kind === 'create' || busy"
                @click="openCreate"
                ><Plus class="leading-icon" aria-hidden="true" />{{
                    t('certs.dns.add')
                }}</ElButton
            >
        </PageIntro>

        <section class="plain-panel" aria-labelledby="credentials-title">
            <div class="panel-heading">
                <h2 id="credentials-title">{{ t('certs.dns.listTitle') }}</h2>
                <ElTag
                    type="info"
                    effect="plain"
                    data-testid="credentials-summary"
                    >{{ summary }}</ElTag
                >
            </div>

            <Reveal>
                <form
                    v-if="mode && mode.kind !== 'delete'"
                    class="node-form certs-form auth-form"
                    :aria-busy="busy"
                    novalidate
                    @keydown.esc="credentialRequest"
                    @submit.prevent="submit"
                >
                    <div>
                        <h3>
                            {{
                                mode.kind === 'edit'
                                    ? t('certs.dns.form.editTitle', {
                                          name: mode.credential.name,
                                      })
                                    : t('certs.dns.form.createTitle')
                            }}
                        </h3>
                        <p class="auth-description">
                            {{
                                mode.kind === 'edit'
                                    ? t('certs.dns.form.editDescription')
                                    : t('certs.dns.form.description')
                            }}
                        </p>
                    </div>
                    <div class="form-field">
                        <label for="credential-name">{{
                            t('certs.dns.form.name')
                        }}</label>
                        <ElInput
                            id="credential-name"
                            v-model="name"
                            type="text"
                            maxlength="64"
                            autocomplete="off"
                            :disabled="busy"
                        />
                        <small>{{ t('certs.dns.form.nameHint') }}</small>
                    </div>
                    <div v-if="mode.kind === 'create'" class="form-field">
                        <label for="credential-provider">{{
                            t('certs.dns.form.provider')
                        }}</label>
                        <ElSelect
                            id="credential-provider"
                            v-model="provider"
                            :disabled="busy"
                            @change="clearSecrets"
                        >
                            <ElOption
                                v-for="item in dnsProviders"
                                :key="item"
                                :value="item"
                                :label="t(`certs.dns.providers.${item}`)"
                            />
                        </ElSelect>
                        <small>{{ t(`certs.dns.hints.${provider}`) }}</small>
                    </div>
                    <p v-else class="field-note">
                        {{
                            t(`certs.dns.providers.${mode.credential.provider}`)
                        }}
                        ·
                        {{ t(`certs.dns.hints.${mode.credential.provider}`) }}
                    </p>
                    <div
                        v-for="field in fields"
                        :key="field.name"
                        class="form-field"
                    >
                        <label :for="`credential-${field.name}`">{{
                            t(`certs.dns.fields.${field.name}`)
                        }}</label>
                        <ElInput
                            :id="`credential-${field.name}`"
                            v-model="secrets[field.name]"
                            :name="`credential-${field.name}`"
                            :type="field.masked ? 'password' : 'text'"
                            maxlength="512"
                            autocomplete="off"
                            autocapitalize="off"
                            spellcheck="false"
                            :placeholder="
                                mode.kind === 'edit' &&
                                isSet(mode.credential, field.name)
                                    ? t('certs.dns.form.keepValue')
                                    : field.name === 'region'
                                      ? 'us-east-1'
                                      : ''
                            "
                            :disabled="busy"
                        />
                    </div>
                    <PasswordField
                        id="credential-password"
                        v-model="password"
                        :label="t('common.password.current')"
                        :disabled="busy"
                    />
                    <div v-if="!credentialConfirming" class="form-actions">
                        <ElButton
                            type="primary"
                            native-type="submit"
                            :loading="busy"
                            >{{
                                mode.kind === 'edit'
                                    ? t('certs.dns.form.submitEdit')
                                    : t('certs.dns.form.submitCreate')
                            }}</ElButton><ElButton
                            :disabled="busy"
                            @click="credentialRequest"
                            >{{ t('common.actions.cancel') }}</ElButton
                        >
                    </div>
                    <DiscardPrompt
                        v-else
                        @keep="credentialKeep"
                        @discard="credentialDiscard"
                    />
                </form>
            </Reveal>

            <AuthFeedback :message="error" class="security-feedback" />
            <AuthFeedback
                :message="notice"
                tone="notice"
                class="security-feedback"
            />

            <p v-if="unavailable" class="nodes-unavailable" role="status">
                {{
                    credentials
                        ? t('certs.dns.unavailable.stale')
                        : t('certs.dns.unavailable.empty')
                }}
            </p>

            <ul
                v-if="credentials && credentials.length"
                class="node-list"
                :aria-label="t('certs.dns.listLabel')"
            >
                <li
                    v-for="credential in credentials"
                    :key="credential.id"
                    class="node-item cert-item"
                    :data-credential-id="credential.id"
                >
                    <div class="node-summary">
                        <span
                            class="node-dot is-online"
                            aria-hidden="true"
                        ></span>
                        <div class="node-name">
                            <strong>{{ credential.name }}</strong>
                            <small>{{
                                t(`certs.dns.providers.${credential.provider}`)
                            }}</small>
                        </div>
                    </div>
                    <dl class="node-meta">
                        <div>
                            <dt>{{ t('certs.dns.meta.updatedAt') }}</dt>
                            <dd>{{ formatDateTime(credential.updatedAt) }}</dd>
                        </div>
                        <div>
                            <dt>{{ t('certs.dns.meta.usage') }}</dt>
                            <dd>
                                {{
                                    credential.certificates
                                        ? t(
                                              'certs.dns.meta.usedBy',
                                              {
                                                  count: formatNumber(
                                                      credential.certificates,
                                                  ),
                                              },
                                              credential.certificates,
                                          )
                                        : t('certs.dns.meta.unused')
                                }}
                            </dd>
                        </div>
                    </dl>
                    <Reveal>
                        <div
                            v-if="
                                mode?.kind !== 'delete' ||
                                mode.credential.id !== credential.id
                            "
                            class="node-action"
                        >
                            <ElButton
                                :disabled="busy"
                                :loading="verifying === credential.id"
                                @click="verify(credential)"
                                >{{
                                    t('certs.dns.verify.open')
                                }}</ElButton><ElButton
                                :disabled="busy"
                                @click="openEdit(credential)"
                                >{{ t('certs.dns.edit') }}</ElButton
                            ><ElButton
                                :disabled="busy"
                                @click="openDelete(credential)"
                                >{{ t('certs.dns.delete.open') }}</ElButton
                            >
                        </div>
                    </Reveal>
                    <ul class="secret-status" :aria-label="credential.name">
                        <li
                            v-for="field in providerFields[credential.provider]"
                            :key="field.name"
                            :data-field="field.name"
                        >
                            <span>{{
                                t(`certs.dns.fields.${field.name}`)
                            }}</span>
                            <ElTag
                                :type="
                                    isSet(credential, field.name)
                                        ? 'success'
                                        : 'info'
                                "
                                effect="plain"
                                size="small"
                                >{{
                                    isSet(credential, field.name)
                                        ? t('certs.dns.status.set')
                                        : t('certs.dns.status.unset')
                                }}</ElTag
                            >
                        </li>
                    </ul>
                    <Reveal>
                        <form
                            v-if="
                                mode?.kind === 'delete' &&
                                mode.credential.id === credential.id
                            "
                            class="node-revoke security-form auth-form"
                            :aria-busy="busy"
                            @submit.prevent="submitDelete"
                        >
                            <p class="auth-description">
                                {{ t('certs.dns.delete.description') }}
                            </p>
                            <PasswordField
                                id="credential-delete-password"
                                v-model="password"
                                :label="t('common.password.current')"
                                :disabled="busy"
                            />
                            <div class="form-actions">
                                <ElButton
                                    type="danger"
                                    native-type="submit"
                                    :loading="busy"
                                    >{{
                                        t('certs.dns.delete.submit')
                                    }}</ElButton><ElButton
                                    :disabled="busy"
                                    @click="closeForms"
                                    >{{ t('common.actions.cancel') }}</ElButton
                                >
                            </div>
                        </form>
                    </Reveal>
                </li>
            </ul>

            <div v-else-if="credentials" class="empty-state">
                <span class="empty-icon"><Key /></span>
                <h3>{{ t('certs.dns.empty.title') }}</h3>
                <p>
                    {{ t('certs.dns.empty.first') }}<br />{{
                        t('certs.dns.empty.second')
                    }}
                </p>
            </div>
        </section>
        <p class="inline-note">{{ t('certs.dns.note') }}</p>
    </div>
</template>
