<script setup lang="ts">
import {
    computed,
    nextTick,
    onBeforeUnmount,
    onMounted,
    reactive,
    ref,
} from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { credentialLabel } from '@/i18n/certs'
import DocumentChecked from '~icons/lucide/file-badge'
import Plus from '~icons/lucide/plus'
import PageIntro from '@/components/PageIntro.vue'
import Reveal from '@/components/Reveal.vue'
import AuthFeedback from '@/components/AuthFeedback.vue'
import { AuthApiError, authErrorMessage } from '@/api/auth'
import {
    certsApi,
    chainFileName,
    type Certificate,
    type DirectoryKind,
    type DnsCredential,
} from '@/api/certs'
import type { MessageSchema } from '@/locales/zh-CN'
import { useSessionExpiry } from '@/composables/useSessionExpiry'
import { formatDate, formatDateTime, formatNumber } from '@/i18n/format'
import '@/styles/_certs.scss'
import { useCachedPage } from '@/composables/useCachedPage'
import DiscardPrompt from '@/components/DiscardPrompt.vue'
import { useDiscardGuard } from '@/composables/useDiscardGuard'

type IssueErrorCode =
    keyof MessageSchema['certs']['certificates']['issueErrors']

// While an issuance runs the list refreshes quickly; otherwise rarely.
const activeInterval = 3_000
const idleInterval = 30_000
const shownDomains = 3
function domainPreview(certificate: Certificate) {
    return certificate.domains.slice(0, shownDomains).join(', ')
}
function hasMoreDomains(certificate: Certificate) {
    return certificate.domains.length > shownDomains
}
const directoryOptions: { kind: DirectoryKind; label: string }[] = [
    { kind: 'letsencrypt', label: 'letsencrypt' },
    { kind: 'letsencrypt-staging', label: 'letsencryptStaging' },
    { kind: 'custom', label: 'custom' },
]

const { t } = useI18n()
const redirectIfExpired = useSessionExpiry()
const certificates = ref<Certificate[] | null>(null)
const credentials = ref<DnsCredential[] | null>(null)
const unavailable = ref(false)
const requesting = ref(false)
// The EAB HMAC key is a secret: it lives only here and is cleared with the form.
const form = reactive<{
    domains: string
    credentialId: string
    directory: DirectoryKind
    directoryUrl: string
    email: string
    eabKeyId: string
    eabHmacKey: string
}>({
    domains: '',
    credentialId: '',
    directory: 'letsencrypt',
    directoryUrl: '',
    email: '',
    eabKeyId: '',
    eabHmacKey: '',
})
const deleting = ref<string | null>(null)
const busy = ref(false)
const error = ref('')
const notice = ref('')
let refreshTimer: ReturnType<typeof setTimeout> | undefined
let unmounted = false

const summary = computed(() => {
    if (unavailable.value && !certificates.value)
        return t('common.status.unavailable')
    if (!certificates.value) return t('common.status.loading')
    const count = certificates.value.length
    return t('certs.certificates.count', { count: formatNumber(count) }, count)
})

function parseDomains(text: string) {
    // Line breaks, spaces, commas and the ideographic comma separate names.
    return text
        .split(/[\s,，、]+/)
        .map((name) => name.trim())
        .filter(Boolean)
}

function state(certificate: Certificate) {
    switch (certificate.status) {
        case 'pending':
            return { tag: 'warning', tone: 'pending' } as const
        case 'valid':
            return { tag: 'success', tone: 'online' } as const
        case 'failed':
            return { tag: 'danger', tone: 'revoked' } as const
        default:
            return { tag: 'info', tone: 'expired' } as const
    }
}

function directoryLabel(certificate: Certificate) {
    if (certificate.directory === 'letsencrypt')
        return t('certs.certificates.directories.letsencrypt')
    if (certificate.directory === 'letsencrypt-staging')
        return t('certs.certificates.directories.letsencryptStaging')
    try {
        return new URL(certificate.directoryUrl).host
    } catch {
        return t('certs.certificates.directories.custom')
    }
}

function issueMessage(code: string) {
    const known = Object.hasOwn(
        {
            CREDENTIAL_UNAVAILABLE: 1,
            ACME_UNREACHABLE: 1,
            ACME_ACCOUNT_FAILED: 1,
            ACME_RATE_LIMITED: 1,
            ACME_REJECTED: 1,
            DNS_PROVIDER_FAILED: 1,
            DNS_PROPAGATION_TIMEOUT: 1,
            ISSUANCE_FAILED: 1,
        } satisfies Record<IssueErrorCode, 1>,
        code,
    )
    return t(
        `certs.certificates.issueErrors.${(known ? code : 'ISSUANCE_FAILED') as IssueErrorCode}`,
    )
}

function clearSecrets() {
    form.eabHmacKey = ''
}

function closeForms() {
    requesting.value = false
    deleting.value = null
    Object.assign(form, {
        domains: '',
        credentialId: '',
        directory: 'letsencrypt',
        directoryUrl: '',
        email: '',
        eabKeyId: '',
        eabHmacKey: '',
    })
    error.value = ''
}

// Unsaved edits ask before Cancel or Escape drops them.
const {
    confirming: requestConfirming,
    request: requestRequest,
    keep: requestKeep,
    discard: requestDiscard,
} = useDiscardGuard({
    open: () => requesting.value && 'request',
    signature: () =>
        [
            form.domains,
            form.credentialId,
            form.directory,
            form.directoryUrl,
            form.email,
            form.eabKeyId,
            form.eabHmacKey !== '',
        ].join('|'),
    close: closeForms,
})

async function handleError(cause: unknown) {
    if (await redirectIfExpired(cause, clearSecrets)) return true
    error.value = authErrorMessage(cause)
    return false
}

function schedule() {
    clearTimeout(refreshTimer)
    if (unmounted) return
    const active = certificates.value?.some(
        (certificate) =>
            certificate.status === 'pending' || certificate.issuing,
    )
    refreshTimer = setTimeout(
        () => {
            if (!document.hidden) void load(true)
            else schedule()
        },
        active ? activeInterval : idleInterval,
    )
}

async function load(background = false) {
    try {
        certificates.value = await certsApi.listCertificates({ background })
        unavailable.value = false
    } catch (cause) {
        if (await redirectIfExpired(cause, clearSecrets)) return
        // Keep the last real list on a failed refresh; never invent one.
        unavailable.value = true
    }
    schedule()
}

async function loadCredentials() {
    try {
        credentials.value = await certsApi.listCredentials()
    } catch (cause) {
        await handleError(cause)
    }
}

async function openRequest() {
    closeForms()
    notice.value = ''
    requesting.value = true
    await loadCredentials()
    const [only, ...others] = credentials.value ?? []
    if (only && !others.length) form.credentialId = only.id
    await nextTick()
    document.getElementById('certificate-domains')?.focus()
}

async function focusField(id: string) {
    await nextTick()
    document.getElementById(id)?.focus()
}

async function submit() {
    if (busy.value) return
    error.value = ''
    const domains = parseDomains(form.domains)
    const custom = form.directory === 'custom'
    if (!domains.length) {
        error.value = t('certs.certificates.form.missingDomains')
        return focusField('certificate-domains')
    }
    if (!form.credentialId) {
        error.value = t('certs.certificates.form.missingCredential')
        return focusField('certificate-credential')
    }
    if (custom && !/^https:\/\/\S+$/i.test(form.directoryUrl.trim())) {
        error.value = t('certs.certificates.form.missingDirectoryUrl')
        return focusField('certificate-directory-url')
    }
    const keyId = custom ? form.eabKeyId.trim() : ''
    const hmacKey = custom ? form.eabHmacKey.trim() : ''
    if (Boolean(keyId) !== Boolean(hmacKey)) {
        error.value = t('certs.certificates.form.incompleteEab')
        return focusField(
            keyId ? 'certificate-eab-hmac' : 'certificate-eab-kid',
        )
    }
    busy.value = true
    try {
        await certsApi.request({
            domains,
            dnsCredentialId: form.credentialId,
            directory: form.directory,
            directoryUrl: custom ? form.directoryUrl.trim() : '',
            email: form.email.trim(),
            eab: { keyId, hmacKey },
        })
        closeForms()
        notice.value = t('certs.certificates.requested')
        await load()
    } catch (cause) {
        if (await handleError(cause)) return
        const code = cause instanceof AuthApiError ? cause.code : ''
        const field = {
            INVALID_DOMAIN: 'certificate-domains',
            TOO_MANY_DOMAINS: 'certificate-domains',
            INVALID_DIRECTORY: 'certificate-directory-url',
            INVALID_EMAIL: 'certificate-email',
            INVALID_EAB: 'certificate-eab-kid',
            CREDENTIAL_NOT_FOUND: 'certificate-credential',
        }[code]
        if (code === 'CREDENTIAL_NOT_FOUND') {
            form.credentialId = ''
            await loadCredentials()
        }
        if (field) await focusField(field)
    } finally {
        busy.value = false
    }
}

async function reissue(certificate: Certificate) {
    if (busy.value) return
    closeForms()
    notice.value = ''
    busy.value = true
    try {
        await certsApi.reissue(certificate.id)
        notice.value = t('certs.certificates.reissued')
    } catch (cause) {
        if (await handleError(cause)) return
    } finally {
        busy.value = false
    }
    await load()
}

async function download(certificate: Certificate) {
    if (busy.value) return
    error.value = ''
    notice.value = ''
    busy.value = true
    try {
        const chain = await certsApi.chain(certificate.id)
        const url = URL.createObjectURL(
            new Blob([chain], { type: 'application/pem-certificate-chain' }),
        )
        const link = document.createElement('a')
        link.href = url
        link.download = chainFileName(certificate)
        document.body.append(link)
        link.click()
        link.remove()
        setTimeout(() => {
            URL.revokeObjectURL(url)
        }, 0)
        notice.value = t('certs.certificates.downloaded')
    } catch (cause) {
        await handleError(cause)
    } finally {
        busy.value = false
    }
}

async function openDelete(certificate: Certificate) {
    closeForms()
    notice.value = ''
    deleting.value = certificate.id
    await focusField('certificate-delete-confirm')
}

async function confirmDelete() {
    if (busy.value || !deleting.value) return
    busy.value = true
    error.value = ''
    try {
        await certsApi.remove(deleting.value)
        closeForms()
        notice.value = t('certs.certificates.delete.done')
    } catch (cause) {
        if (await handleError(cause)) return
        if (
            cause instanceof AuthApiError &&
            cause.code === 'CERTIFICATE_NOT_FOUND'
        )
            deleting.value = null
    } finally {
        busy.value = false
    }
    await load()
}

onMounted(() => {
    void load()
})
// Hidden in the page cache: the EAB key leaves memory, polling stops.
useCachedPage({
    pause: () => {
        clearTimeout(refreshTimer)
        clearSecrets()
    },
    resume: () => void load(true),
})
onBeforeUnmount(() => {
    unmounted = true
    clearTimeout(refreshTimer)
    clearSecrets()
})
</script>

<template>
    <div>
        <PageIntro
            :eyebrow="t('common.shell.groups.security')"
            :title="t('common.routes.certificates')"
            :description="t('certs.certificates.description')"
        >
            <ElButton
                type="primary"
                size="large"
                :disabled="requesting || busy"
                @click="openRequest"
                ><Plus class="leading-icon" aria-hidden="true" />{{
                    t('certs.certificates.request')
                }}</ElButton
            >
        </PageIntro>

        <section class="plain-panel" aria-labelledby="certificates-title">
            <div class="panel-heading">
                <h2 id="certificates-title">
                    {{ t('certs.certificates.listTitle') }}
                </h2>
                <ElTag
                    type="info"
                    effect="plain"
                    data-testid="certificates-summary"
                    >{{ summary }}</ElTag
                >
            </div>

            <Reveal>
                <form
                    v-if="requesting"
                    class="node-form certs-form auth-form"
                    :aria-busy="busy"
                    novalidate
                    @keydown.esc="requestRequest"
                    @submit.prevent="submit"
                >
                    <div>
                        <h3>{{ t('certs.certificates.request') }}</h3>
                        <p class="auth-description">
                            {{ t('certs.certificates.form.description') }}
                        </p>
                    </div>
                    <div class="form-field">
                        <label for="certificate-domains">{{
                            t('certs.certificates.form.domains')
                        }}</label>
                        <ElInput
                            id="certificate-domains"
                            v-model="form.domains"
                            type="textarea"
                            :rows="3"
                            spellcheck="false"
                            autocapitalize="off"
                            autocomplete="off"
                            :disabled="busy"
                            placeholder="example.com&#10;*.example.com"
                        />
                        <small>{{
                            t('certs.certificates.form.domainsHint')
                        }}</small>
                    </div>
                    <div class="form-field">
                        <label for="certificate-credential">{{
                            t('certs.certificates.form.credential')
                        }}</label>
                        <ElSelect
                            id="certificate-credential"
                            v-model="form.credentialId"
                            :placeholder="
                                t(
                                    'certs.certificates.form.credentialPlaceholder',
                                )
                            "
                            :disabled="busy || !credentials?.length"
                        >
                            <ElOption
                                v-for="credential in credentials ?? []"
                                :key="credential.id"
                                :value="credential.id"
                                :label="
                                    credentialLabel(
                                        credential.name,
                                        credential.provider,
                                    )
                                "
                                :data-credential-id="credential.id"
                            />
                        </ElSelect>
                        <small v-if="credentials && !credentials.length"
                            >{{ t('certs.certificates.form.noCredentials') }}
                            <RouterLink to="/dns-providers">{{
                                t('certs.certificates.form.addCredential')
                            }}</RouterLink></small
                        >
                    </div>
                    <fieldset class="form-field choice-group">
                        <legend id="certificate-directory-label">
                            {{ t('certs.certificates.form.directory') }}
                        </legend>
                        <ElRadioGroup
                            v-model="form.directory"
                            :disabled="busy"
                            aria-labelledby="certificate-directory-label"
                        >
                            <ElRadio
                                v-for="option in directoryOptions"
                                :key="option.kind"
                                :value="option.kind"
                                >{{
                                    t(
                                        `certs.certificates.directories.${option.label}`,
                                    )
                                }}</ElRadio
                            >
                        </ElRadioGroup>
                    </fieldset>
                    <template v-if="form.directory === 'custom'">
                        <div class="form-field">
                            <label for="certificate-directory-url">{{
                                t('certs.certificates.form.directoryUrl')
                            }}</label>
                            <ElInput
                                id="certificate-directory-url"
                                v-model="form.directoryUrl"
                                type="url"
                                inputmode="url"
                                maxlength="2048"
                                autocomplete="off"
                                spellcheck="false"
                                placeholder="https://"
                                :disabled="busy"
                            />
                            <small>{{
                                t('certs.certificates.form.directoryUrlHint')
                            }}</small>
                        </div>
                        <fieldset class="certs-subgroup" :disabled="busy">
                            <legend>
                                {{ t('certs.certificates.form.eabTitle') }}
                            </legend>
                            <p class="field-note">
                                {{ t('certs.certificates.form.eabHint') }}
                            </p>
                            <div class="form-field">
                                <label for="certificate-eab-kid">{{
                                    t('certs.certificates.form.eabKeyId')
                                }}</label>
                                <ElInput
                                    id="certificate-eab-kid"
                                    v-model="form.eabKeyId"
                                    type="text"
                                    maxlength="256"
                                    autocomplete="off"
                                    spellcheck="false"
                                />
                            </div>
                            <div class="form-field">
                                <label for="certificate-eab-hmac">{{
                                    t('certs.certificates.form.eabHmacKey')
                                }}</label>
                                <ElInput
                                    id="certificate-eab-hmac"
                                    v-model="form.eabHmacKey"
                                    type="password"
                                    maxlength="512"
                                    autocomplete="off"
                                    spellcheck="false"
                                />
                            </div>
                        </fieldset>
                    </template>
                    <div class="form-field">
                        <label for="certificate-email">{{
                            t('certs.certificates.form.email')
                        }}</label>
                        <ElInput
                            id="certificate-email"
                            v-model="form.email"
                            type="email"
                            maxlength="254"
                            autocomplete="email"
                            :disabled="busy"
                        />
                        <small>{{
                            t('certs.certificates.form.emailHint')
                        }}</small>
                    </div>
                    <p class="field-note">
                        {{ t('certs.certificates.form.terms') }}
                    </p>
                    <div v-if="!requestConfirming" class="form-actions">
                        <ElButton
                            type="primary"
                            native-type="submit"
                            :loading="busy"
                            >{{
                                t('certs.certificates.form.submit')
                            }}</ElButton><ElButton
                            :disabled="busy"
                            @click="requestRequest"
                            >{{ t('common.actions.cancel') }}</ElButton
                        >
                    </div>
                    <DiscardPrompt
                        v-else
                        @keep="requestKeep"
                        @discard="requestDiscard"
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
                    certificates
                        ? t('certs.certificates.unavailable.stale')
                        : t('certs.certificates.unavailable.empty')
                }}
            </p>

            <div v-if="certificates && certificates.length" class="table-frame">
                <table
                    class="data-table"
                    :aria-label="t('certs.certificates.listLabel')"
                >
                    <thead>
                        <tr>
                            <th>
                                {{ t('certs.certificates.columns.domains') }}
                            </th>
                            <th>
                                {{ t('certs.certificates.columns.status') }}
                            </th>
                            <th>{{ t('certs.certificates.meta.issuer') }}</th>
                            <th>{{ t('certs.certificates.meta.notAfter') }}</th>
                            <th>{{ t('certs.certificates.meta.renewAt') }}</th>
                            <th>
                                {{ t('certs.certificates.columns.actions') }}
                            </th>
                        </tr>
                    </thead>
                    <tbody>
                        <template
                            v-for="certificate in certificates"
                            :key="certificate.id"
                        >
                            <tr :data-certificate-id="certificate.id">
                                <td>
                                    <div class="node-name">
                                        <span
                                            class="node-dot"
                                            :class="`is-${state(certificate).tone}`"
                                            aria-hidden="true"
                                        ></span>
                                        <div>
                                            <strong
                                                >{{
                                                    domainPreview(certificate)
                                                }}
                                                <span
                                                    v-if="
                                                        hasMoreDomains(
                                                            certificate,
                                                        )
                                                    "
                                                    class="cert-more"
                                                    >{{
                                                        t(
                                                            'certs.certificates.moreDomains',
                                                            {
                                                                count: formatNumber(
                                                                    certificate
                                                                        .domains
                                                                        .length,
                                                                ),
                                                            },
                                                            certificate.domains
                                                                .length,
                                                        )
                                                    }}</span
                                                ></strong
                                            >
                                            <small
                                                >{{
                                                    directoryLabel(certificate)
                                                }}
                                                ·
                                                {{
                                                    credentialLabel(
                                                        certificate.dnsCredentialName,
                                                        certificate.dnsCredentialProvider,
                                                    )
                                                }}</small
                                            >
                                            <p
                                                v-if="certificate.lastError"
                                                class="cert-error"
                                                data-testid="certificate-error"
                                            >
                                                {{
                                                    t(
                                                        'certs.certificates.lastError',
                                                        {
                                                            time: formatDateTime(
                                                                certificate
                                                                    .lastError
                                                                    .at,
                                                            ),
                                                            message:
                                                                issueMessage(
                                                                    certificate
                                                                        .lastError
                                                                        .code,
                                                                ),
                                                        },
                                                    )
                                                }}
                                                <code
                                                    v-if="
                                                        certificate.lastError
                                                            .detail
                                                    "
                                                    >{{
                                                        certificate.lastError
                                                            .detail
                                                    }}</code
                                                >
                                            </p>
                                        </div>
                                    </div>
                                </td>
                                <td>
                                    <span class="cert-tags">
                                        <ElTag
                                            :type="state(certificate).tag"
                                            data-testid="certificate-status"
                                            >{{
                                                t(
                                                    `certs.certificates.state.${certificate.status}`,
                                                )
                                            }}</ElTag><ElTag
                                            v-if="
                                                certificate.issuing &&
                                                certificate.status !== 'pending'
                                            "
                                            type="warning"
                                            >{{
                                                t(
                                                    'certs.certificates.state.renewing',
                                                )
                                            }}</ElTag
                                        >
                                    </span>
                                </td>
                                <td class="cell-mono">
                                    {{
                                        certificate.issuer ??
                                        t('certs.certificates.meta.none')
                                    }}
                                </td>
                                <td class="cell-mono">
                                    {{
                                        certificate.notAfter
                                            ? formatDate(certificate.notAfter)
                                            : t('certs.certificates.meta.none')
                                    }}
                                </td>
                                <td class="cell-mono">
                                    {{
                                        certificate.notAfter &&
                                        certificate.renewAt
                                            ? formatDate(certificate.renewAt)
                                            : t('certs.certificates.meta.none')
                                    }}
                                </td>
                                <td>
                                    <div class="row-actions">
                                        <ElButton
                                            size="small"
                                            :disabled="
                                                busy || certificate.issuing
                                            "
                                            @click="reissue(certificate)"
                                            >{{
                                                t('certs.certificates.reissue')
                                            }}</ElButton><ElButton
                                            v-if="certificate.notAfter"
                                            size="small"
                                            :disabled="busy"
                                            @click="download(certificate)"
                                            >{{
                                                t('certs.certificates.download')
                                            }}</ElButton
                                        ><ElButton
                                            v-if="deleting !== certificate.id"
                                            size="small"
                                            :disabled="busy"
                                            @click="openDelete(certificate)"
                                            >{{
                                                t(
                                                    'certs.certificates.delete.open',
                                                )
                                            }}</ElButton
                                        >
                                    </div>
                                </td>
                            </tr>
                            <Reveal>
                                <tr
                                    v-if="deleting === certificate.id"
                                    class="row-form"
                                >
                                    <td colspan="6">
                                        <p class="auth-description">
                                            {{
                                                t(
                                                    'certs.certificates.delete.description',
                                                )
                                            }}
                                        </p>
                                        <div class="form-actions">
                                            <ElButton
                                                id="certificate-delete-confirm"
                                                type="danger"
                                                :loading="busy"
                                                @click="confirmDelete"
                                                >{{
                                                    t(
                                                        'certs.certificates.delete.submit',
                                                    )
                                                }}</ElButton><ElButton
                                                :disabled="busy"
                                                @click="closeForms"
                                                >{{
                                                    t('common.actions.cancel')
                                                }}</ElButton
                                            >
                                        </div>
                                    </td>
                                </tr>
                            </Reveal>
                        </template>
                    </tbody>
                </table>
            </div>

            <div v-else-if="certificates" class="empty-state">
                <span class="empty-icon"><DocumentChecked /></span>
                <h3>{{ t('certs.certificates.empty.title') }}</h3>
                <p>
                    {{ t('certs.certificates.empty.first') }}<br />{{
                        t('certs.certificates.empty.second')
                    }}
                </p>
            </div>
        </section>
        <p class="inline-note">{{ t('certs.certificates.note') }}</p>
    </div>
</template>
