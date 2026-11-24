<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { I18nT, useI18n } from 'vue-i18n'
import Connection from '~icons/lucide/server'
import Copy from '~icons/lucide/copy'
import Plus from '~icons/lucide/plus'
import PageIntro from '@/components/PageIntro.vue'
import Reveal from '@/components/Reveal.vue'
import PasswordField from '@/components/PasswordField.vue'
import AuthFeedback from '@/components/AuthFeedback.vue'
import { AuthApiError, authErrorMessage } from '@/api/auth'
import { nodesApi, type VpnNode } from '@/api/nodes'
import { useSessionExpiry } from '@/composables/useSessionExpiry'
import { formatDateTime, formatNumber, formatSince } from '@/i18n/format'
import { useCachedPage } from '@/composables/useCachedPage'
import DiscardPrompt from '@/components/DiscardPrompt.vue'
import { useDiscardGuard } from '@/composables/useDiscardGuard'

const refreshInterval = 15_000

const { t } = useI18n()
const redirectIfExpired = useSessionExpiry()
const nodes = ref<VpnNode[] | null>(null)
const unavailable = ref(false)
const creating = ref(false)
const name = ref('')
const password = ref('')
type NodeAction = 'reissue' | 'revoke' | 'delete'
// The node action form open below a node, if any; each needs the password.
const action = ref<{ node: VpnNode; kind: NodeAction } | null>(null)
const actionType = {
    reissue: 'primary',
    revoke: 'danger',
    delete: 'danger',
} as const
const busy = ref(false)
const error = ref('')
const notice = ref('')
// The one-time enrollment token lives only here, in memory, while shown.
const enrollment = ref<{
    nodeName: string
    token: string
    expiresAt: string
} | null>(null)
const now = ref(Date.now())
let refreshTimer: ReturnType<typeof setInterval> | undefined
let clockTimer: ReturnType<typeof setInterval> | undefined

const enrollCommand = computed(
    () =>
        `veilpost-agent enroll --server ${window.location.origin} --token-stdin`,
)
const onlineCount = computed(
    () => nodes.value?.filter((node) => node.online).length ?? 0,
)
const summary = computed(() => {
    if (unavailable.value && !nodes.value) return t('common.status.unavailable')
    if (!nodes.value) return t('common.status.loading')
    if (!nodes.value.length) return t('common.status.notConnected')
    return t('nodes.onlineCount', { count: formatNumber(onlineCount.value) })
})

function state(node: VpnNode) {
    if (node.status === 'revoked')
        return {
            label: t('nodes.state.revoked'),
            tag: 'danger',
            tone: 'revoked',
        } as const
    if (node.status === 'pending')
        return node.enrollmentExpiresAt &&
            Date.parse(node.enrollmentExpiresAt) > now.value
            ? ({
                  label: t('nodes.state.pending'),
                  tag: 'warning',
                  tone: 'pending',
              } as const)
            : ({
                  label: t('nodes.state.expired'),
                  tag: 'info',
                  tone: 'expired',
              } as const)
    return node.online
        ? ({
              label: t('nodes.state.online'),
              tag: 'success',
              tone: 'online',
          } as const)
        : ({
              label: t('nodes.state.offline'),
              tag: 'info',
              tone: 'offline',
          } as const)
}

function detail(node: VpnNode) {
    if (node.status === 'revoked')
        return node.revokedAt
            ? t('nodes.detail.revokedAt', {
                  time: formatDateTime(node.revokedAt),
              })
            : t('nodes.state.revoked')
    if (node.status === 'pending')
        return state(node).tone === 'pending' && node.enrollmentExpiresAt
            ? t('nodes.detail.tokenValidUntil', {
                  time: formatDateTime(node.enrollmentExpiresAt),
              })
            : t('nodes.detail.tokenExpired')
    return node.lastSeenAt
        ? t('nodes.detail.lastSeen', {
              time: formatSince(node.lastSeenAt, now.value),
          })
        : t('nodes.detail.neverSeen')
}

function clearSecrets() {
    password.value = ''
    enrollment.value = null
}

function closeForms() {
    creating.value = false
    action.value = null
    name.value = ''
    password.value = ''
    error.value = ''
}

// Unsaved edits ask before Cancel or Escape drops them.
const {
    confirming: createConfirming,
    request: createRequest,
    keep: createKeep,
    discard: createDiscard,
} = useDiscardGuard({
    open: () => creating.value && 'create',
    signature: () => [name.value, password.value !== ''].join('|'),
    close: closeForms,
})

async function handleError(cause: unknown) {
    if (await redirectIfExpired(cause, clearSecrets)) return true
    error.value = authErrorMessage(cause)
    return false
}

// Nodes that turned online since the last refresh send out one ring.
const arriving = ref(new Set<string>())
let arrivalTimer: ReturnType<typeof setTimeout> | undefined
function markArrivals(before: VpnNode[] | null, after: VpnNode[]) {
    if (!before) return
    const previous = new Map(before.map((node) => [node.id, state(node).tone]))
    const turned = after
        .filter(
            (node) =>
                previous.has(node.id) &&
                previous.get(node.id) !== 'online' &&
                state(node).tone === 'online',
        )
        .map((node) => node.id)
    if (!turned.length) return
    arriving.value = new Set(turned)
    clearTimeout(arrivalTimer)
    arrivalTimer = setTimeout(() => {
        arriving.value = new Set()
    }, 700)
}

async function load(background = false) {
    try {
        const before = nodes.value
        nodes.value = await nodesApi.list({ background })
        markArrivals(before, nodes.value)
        unavailable.value = false
    } catch (cause) {
        if (await redirectIfExpired(cause, clearSecrets)) return
        // Keep the last real list on a failed background refresh; never invent one.
        unavailable.value = true
    }
}

async function openCreate() {
    closeForms()
    notice.value = ''
    creating.value = true
    await nextTick()
    document.getElementById('node-name')?.focus()
}

async function create() {
    if (busy.value) return
    error.value = ''
    if (!name.value.trim() || !password.value) {
        error.value = t('nodes.form.missing')
        return
    }
    busy.value = true
    try {
        await showEnrollment(
            await nodesApi.create(name.value.trim(), password.value),
        )
    } catch (cause) {
        password.value = ''
        if (await handleError(cause)) return
        await nextTick()
        document
            .getElementById(
                cause instanceof AuthApiError && cause.code === 'INVALID_NAME'
                    ? 'node-name'
                    : 'node-password',
            )
            ?.focus()
    } finally {
        busy.value = false
    }
}

async function showEnrollment(result: {
    node: VpnNode
    enrollment: { token: string; expiresAt: string }
}) {
    enrollment.value = {
        nodeName: result.node.name,
        token: result.enrollment.token,
        expiresAt: result.enrollment.expiresAt,
    }
    closeForms()
    await load()
    await nextTick()
    document.getElementById('enrollment-title')?.focus()
}

function finishEnrollment() {
    const nodeName = enrollment.value?.nodeName
    enrollment.value = null
    notice.value = nodeName
        ? t('nodes.enrollment.hidden', { name: nodeName })
        : ''
}

async function openAction(node: VpnNode, kind: NodeAction) {
    closeForms()
    notice.value = ''
    action.value = { node, kind }
    await nextTick()
    document.getElementById('node-action-password')?.focus()
}

async function submitAction() {
    if (busy.value || !action.value) return
    const { node, kind } = action.value
    error.value = ''
    if (!password.value) {
        error.value = t('nodes.form.missingPassword')
        return
    }
    busy.value = true
    try {
        if (kind === 'reissue') {
            enrollment.value = null
            await showEnrollment(
                await nodesApi.reissue(node.id, password.value),
            )
            return
        }
        if (kind === 'revoke') {
            const revoked = await nodesApi.revoke(node.id, password.value)
            notice.value = t('nodes.revoke.done', { name: revoked.name })
        } else {
            await nodesApi.remove(node.id, password.value)
            notice.value = t('nodes.delete.done', { name: node.name })
        }
        closeForms()
        await load()
    } catch (cause) {
        password.value = ''
        if (await handleError(cause)) return
        if (
            cause instanceof AuthApiError &&
            ['NODE_NOT_FOUND', 'NODE_NOT_PENDING', 'NODE_NOT_REVOKED'].includes(
                cause.code,
            )
        ) {
            closeForms()
            error.value = authErrorMessage(cause)
            await load()
        }
    } finally {
        busy.value = false
    }
}

async function copy(value: string) {
    try {
        await navigator.clipboard.writeText(value)
        ElMessage.success({
            message: t('common.actions.copied'),
            grouping: true,
        })
    } catch {
        ElMessage.error({
            message: t('common.actions.copyFailed'),
            grouping: true,
        })
    }
}

function startTimers() {
    refreshTimer = setInterval(() => {
        if (!document.hidden) void load(true)
    }, refreshInterval)
    clockTimer = setInterval(() => {
        now.value = Date.now()
        if (
            enrollment.value &&
            Date.parse(enrollment.value.expiresAt) <= now.value
        ) {
            enrollment.value = null
            notice.value = t('nodes.enrollment.expired')
        }
    }, 1000)
}

function stopTimers() {
    clearInterval(refreshTimer)
    clearInterval(clockTimer)
}

onMounted(() => {
    void load()
    startTimers()
})
// Hidden in the page cache: the enrollment token and password leave memory.
useCachedPage({
    pause: () => {
        stopTimers()
        clearSecrets()
    },
    resume: () => {
        now.value = Date.now()
        void load(true)
        startTimers()
    },
})
onBeforeUnmount(() => {
    stopTimers()
    clearSecrets()
    clearTimeout(arrivalTimer)
})
</script>

<template>
    <div>
        <PageIntro
            :eyebrow="t('common.shell.groups.workspace')"
            :title="t('common.routes.nodes')"
            :description="t('nodes.description')"
        >
            <ElButton
                type="primary"
                size="large"
                :disabled="creating || busy"
                @click="openCreate"
                ><Plus class="leading-icon" aria-hidden="true" />{{
                    t('nodes.create')
                }}</ElButton
            >
        </PageIntro>

        <section class="plain-panel nodes-panel" aria-labelledby="nodes-title">
            <div class="panel-heading">
                <h2 id="nodes-title">{{ t('nodes.listTitle') }}</h2>
                <ElTag
                    :type="onlineCount ? 'success' : 'info'"
                    effect="plain"
                    data-testid="nodes-summary"
                    >{{ summary }}</ElTag
                >
            </div>

            <Reveal>
                <form
                    v-if="creating"
                    class="node-form security-form auth-form"
                    :aria-busy="busy"
                    @keydown.esc="createRequest"
                    @submit.prevent="create"
                >
                    <div>
                        <h3>{{ t('nodes.create') }}</h3>
                        <p class="auth-description">
                            {{ t('nodes.form.description') }}
                        </p>
                    </div>
                    <div class="form-field">
                        <label for="node-name">{{
                            t('nodes.form.name')
                        }}</label><ElInput
                            id="node-name"
                            v-model="name"
                            type="text"
                            maxlength="64"
                            autocomplete="off"
                            required
                            :disabled="busy"
                        />
                        <small>{{ t('nodes.form.nameHint') }}</small>
                    </div>
                    <PasswordField
                        id="node-password"
                        v-model="password"
                        :label="t('common.password.current')"
                        :disabled="busy"
                    />
                    <div v-if="!createConfirming" class="form-actions">
                        <ElButton
                            type="primary"
                            native-type="submit"
                            :loading="busy"
                            >{{ t('nodes.form.submit') }}</ElButton><ElButton
                            :disabled="busy"
                            @click="createRequest"
                            >{{ t('common.actions.cancel') }}</ElButton
                        >
                    </div>
                    <DiscardPrompt
                        v-else
                        @keep="createKeep"
                        @discard="createDiscard"
                    />
                </form>
            </Reveal>

            <Reveal>
                <div
                    v-if="enrollment"
                    class="enrollment-content"
                    aria-labelledby="enrollment-title"
                >
                    <h3 id="enrollment-title" tabindex="-1">
                        {{
                            t('nodes.enrollment.title', {
                                name: enrollment.nodeName,
                            })
                        }}
                    </h3>
                    <p>
                        {{
                            t('nodes.enrollment.instructions', {
                                time: formatDateTime(enrollment.expiresAt),
                            })
                        }}
                    </p>
                    <div class="enrollment-field">
                        <span>{{ t('nodes.enrollment.command') }}</span>
                        <code data-testid="enroll-command">{{
                            enrollCommand
                        }}</code>
                        <ElButton size="small" @click="copy(enrollCommand)">{{
                            t('nodes.enrollment.copyCommand')
                        }}</ElButton>
                    </div>
                    <div class="enrollment-field">
                        <span>{{ t('nodes.enrollment.token') }}</span>
                        <code data-testid="enroll-token">{{
                            enrollment.token
                        }}</code>
                        <ElButton
                            size="small"
                            @click="copy(enrollment.token)"
                            >{{ t('nodes.enrollment.copyToken') }}</ElButton
                        >
                    </div>
                    <I18nT
                        keypath="nodes.enrollment.note"
                        tag="p"
                        class="field-note"
                    >
                        <template #stateDir
                            ><code>/var/lib/veilpost-agent</code></template
                        >
                        <template #runCommand
                            ><code>veilpost-agent run</code></template
                        >
                    </I18nT>
                    <ElButton type="primary" @click="finishEnrollment">{{
                        t('nodes.enrollment.finish')
                    }}</ElButton>
                </div>
            </Reveal>

            <AuthFeedback :message="error" class="security-feedback" />
            <AuthFeedback
                :message="notice"
                tone="notice"
                class="security-feedback"
            />

            <p v-if="unavailable" class="nodes-unavailable" role="status">
                {{
                    nodes
                        ? t('nodes.unavailable.stale')
                        : t('nodes.unavailable.empty')
                }}
            </p>

            <div v-if="nodes && nodes.length" class="table-frame">
                <table class="data-table" :aria-label="t('nodes.listLabel')">
                    <thead>
                        <tr>
                            <th>{{ t('nodes.columns.name') }}</th>
                            <th>{{ t('nodes.columns.status') }}</th>
                            <th>{{ t('nodes.columns.id') }}</th>
                            <th>{{ t('nodes.meta.agentVersion') }}</th>
                            <th>{{ t('nodes.meta.createdAt') }}</th>
                            <th>{{ t('nodes.columns.actions') }}</th>
                        </tr>
                    </thead>
                    <tbody>
                        <template v-for="node in nodes" :key="node.id">
                            <tr :data-node-id="node.id">
                                <td>
                                    <div class="node-name">
                                        <span
                                            class="node-dot"
                                            :class="[
                                                `is-${state(node).tone}`,
                                                {
                                                    'is-arriving': arriving.has(
                                                        node.id,
                                                    ),
                                                },
                                            ]"
                                            aria-hidden="true"
                                        ></span>
                                        <div>
                                            <strong>{{ node.name }}</strong>
                                            <small>{{ detail(node) }}</small>
                                        </div>
                                    </div>
                                </td>
                                <td>
                                    <ElTag :type="state(node).tag">{{
                                        state(node).label
                                    }}</ElTag>
                                </td>
                                <td class="cell-mono">
                                    {{ node.id
                                    }}<button
                                        type="button"
                                        class="copy-button"
                                        :aria-label="
                                            t('nodes.copyId', {
                                                name: node.name,
                                            })
                                        "
                                        @click="copy(node.id)"
                                    >
                                        <Copy aria-hidden="true" />
                                    </button>
                                </td>
                                <td class="cell-mono">
                                    {{
                                        node.agentVersion ??
                                        t('nodes.meta.unknown')
                                    }}
                                </td>
                                <td class="cell-mono">
                                    {{ formatDateTime(node.createdAt) }}
                                </td>
                                <td>
                                    <div
                                        v-if="action?.node.id !== node.id"
                                        class="row-actions"
                                    >
                                        <ElButton
                                            v-if="node.status === 'pending'"
                                            size="small"
                                            :disabled="busy"
                                            @click="openAction(node, 'reissue')"
                                            >{{
                                                t('nodes.reissue.open')
                                            }}</ElButton><ElButton
                                            v-if="node.status !== 'revoked'"
                                            size="small"
                                            :disabled="busy"
                                            @click="openAction(node, 'revoke')"
                                            >{{
                                                t('nodes.revoke.open')
                                            }}</ElButton
                                        ><ElButton
                                            v-else
                                            size="small"
                                            :disabled="busy"
                                            @click="openAction(node, 'delete')"
                                            >{{
                                                t('nodes.delete.open')
                                            }}</ElButton
                                        >
                                    </div>
                                </td>
                            </tr>
                            <Reveal>
                                <tr
                                    v-if="action?.node.id === node.id"
                                    class="row-form"
                                >
                                    <td colspan="6">
                                        <form
                                            class="node-revoke security-form auth-form"
                                            :aria-busy="busy"
                                            @submit.prevent="submitAction"
                                        >
                                            <p class="auth-description">
                                                {{
                                                    t(
                                                        `nodes.${action.kind}.description`,
                                                    )
                                                }}
                                            </p>
                                            <PasswordField
                                                id="node-action-password"
                                                v-model="password"
                                                :label="
                                                    t('common.password.current')
                                                "
                                                :disabled="busy"
                                            />
                                            <div class="form-actions">
                                                <ElButton
                                                    :type="
                                                        actionType[action.kind]
                                                    "
                                                    native-type="submit"
                                                    :loading="busy"
                                                    >{{
                                                        t(
                                                            `nodes.${action.kind}.submit`,
                                                        )
                                                    }}</ElButton><ElButton
                                                    :disabled="busy"
                                                    @click="closeForms"
                                                    >{{
                                                        t(
                                                            'common.actions.cancel',
                                                        )
                                                    }}</ElButton
                                                >
                                            </div>
                                        </form>
                                    </td>
                                </tr>
                            </Reveal>
                        </template>
                    </tbody>
                </table>
            </div>

            <div v-else-if="nodes" class="empty-state">
                <span class="empty-icon"><Connection /></span>
                <h3>{{ t('nodes.empty.title') }}</h3>
                <p>
                    {{ t('nodes.empty.first') }}<br />{{
                        t('nodes.empty.second')
                    }}
                </p>
            </div>

            <div class="store-link-row">
                <span>{{ t('nodes.protocols') }}</span><RouterLink
                    to="/store"
                    >{{ t('nodes.storeLink') }}</RouterLink
                >
            </div>
        </section>
        <p class="inline-note">{{ t('nodes.note') }}</p>
    </div>
</template>
