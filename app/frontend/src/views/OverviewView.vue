<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { I18nT, useI18n } from 'vue-i18n'
import ArrowRight from '~icons/lucide/arrow-right'
import PageIntro from '@/components/PageIntro.vue'
import { nodesApi, type VpnNode } from '@/api/nodes'
import { certsApi, type Certificate } from '@/api/certs'
import { useAuthStore } from '@/stores/auth'
import { useAppsStore } from '@/stores/apps'
import { useSessionExpiry } from '@/composables/useSessionExpiry'
import { formatDate, formatNumber, formatSince } from '@/i18n/format'
import { useCachedPage } from '@/composables/useCachedPage'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const apps = useAppsStore()
const redirectIfExpired = useSessionExpiry()
const nodes = ref<VpnNode[] | null>(null)
const certificates = ref<Certificate[] | null>(null)
const nodesUnavailable = ref(false)
const certificatesUnavailable = ref(false)
const now = Date.now()

// Revoked nodes no longer belong to the network.
const current = computed(
    () => nodes.value?.filter((node) => node.status !== 'revoked') ?? [],
)
const online = computed(() => current.value.filter((node) => node.online))
// A state in words until there are nodes to count.
const nodeState = computed(() => {
    if (nodesUnavailable.value) return t('common.status.unavailable')
    if (!nodes.value) return t('common.status.loading')
    if (!current.value.length) return t('common.status.notConnected')
    return undefined
})
const valid = computed(
    () => certificates.value?.filter((item) => item.status === 'valid') ?? [],
)
// The certificates that expire first, soonest at the top.
const expiring = computed(() =>
    valid.value
        .flatMap((item) =>
            item.notAfter
                ? [
                      {
                          id: item.id,
                          domain: item.domains[0] ?? '',
                          notAfter: item.notAfter,
                      },
                  ]
                : [],
        )
        .sort((a, b) => Date.parse(a.notAfter) - Date.parse(b.notAfter))
        .slice(0, 4),
)
const installedCount = computed(() => apps.installed.length)
const catalogCount = computed(() => apps.apps?.length ?? 0)

function state(node: VpnNode) {
    if (node.status === 'pending')
        return { tag: 'warning', key: 'pending' } as const
    return node.online
        ? ({ tag: 'success', key: 'online' } as const)
        : ({ tag: 'info', key: 'offline' } as const)
}

async function load() {
    const [nodeResult, certificateResult] = await Promise.allSettled([
        nodesApi.list({ background: true }),
        certsApi.listCertificates({ background: true }),
    ])
    for (const result of [nodeResult, certificateResult])
        if (
            result.status === 'rejected' &&
            (await redirectIfExpired(result.reason))
        )
            return
    nodesUnavailable.value = nodeResult.status === 'rejected'
    if (nodeResult.status === 'fulfilled') nodes.value = nodeResult.value
    certificatesUnavailable.value = certificateResult.status === 'rejected'
    if (certificateResult.status === 'fulfilled')
        certificates.value = certificateResult.value
}

onMounted(() => void load())
useCachedPage({ pause: () => undefined, resume: () => void load() })
</script>

<template>
    <div>
        <PageIntro
            :eyebrow="t('common.shell.groups.workspace')"
            :title="t('common.routes.overview')"
            :description="t('overview.description')"
        >
            <ElButton type="primary" @click="router.push('/nodes')"
                >{{ t('overview.viewNodes') }}
                <ArrowRight class="button-icon" aria-hidden="true"
            /></ElButton>
        </PageIntro>

        <section class="stat-row" :aria-label="t('overview.summary')">
            <div class="stat">
                <span>{{ t('overview.stats.nodes') }}</span>
                <strong
                    v-if="nodeState"
                    class="is-word"
                    data-testid="overview-nodes"
                    >{{ nodeState }}</strong
                >
                <I18nT
                    v-else
                    keypath="overview.online"
                    tag="strong"
                    class="is-ratio"
                    data-testid="overview-nodes"
                >
                    <template #online
                        ><span class="stat-figure">{{
                            formatNumber(online.length)
                        }}</span></template
                    >
                    <template #total>{{
                        formatNumber(current.length)
                    }}</template>
                </I18nT>
                <small>{{
                    t('overview.stats.nodesDetail', {
                        pending: formatNumber(
                            current.filter((node) => node.status === 'pending')
                                .length,
                        ),
                    })
                }}</small>
            </div>
            <div class="stat">
                <span>{{ t('overview.stats.apps') }}</span>
                <I18nT
                    v-if="apps.apps"
                    keypath="overview.ofTotal"
                    tag="strong"
                    class="is-ratio"
                >
                    <template #count
                        ><span class="stat-figure">{{
                            formatNumber(installedCount)
                        }}</span></template
                    >
                    <template #total>{{ formatNumber(catalogCount) }}</template>
                </I18nT>
                <strong v-else class="is-word">{{
                    t('common.status.loading')
                }}</strong>
                <small>{{ t('overview.stats.appsDetail') }}</small>
            </div>
            <div class="stat">
                <span>{{ t('overview.stats.certificates') }}</span>
                <strong
                    :class="{
                        'is-word': certificatesUnavailable || !certificates,
                    }"
                    >{{
                        certificatesUnavailable
                            ? t('common.status.unavailable')
                            : certificates
                              ? formatNumber(valid.length)
                              : t('common.status.loading')
                    }}</strong
                >
                <small>{{
                    expiring[0]
                        ? t('overview.stats.nextExpiry', {
                              date: formatDate(expiring[0].notAfter),
                          })
                        : t('overview.stats.noExpiry')
                }}</small>
            </div>
            <div class="stat">
                <span>{{ t('overview.stats.twoFactor') }}</span>
                <strong class="is-word">{{
                    auth.user?.twoFactorEnabled
                        ? t('overview.stats.enabled')
                        : t('overview.stats.disabled')
                }}</strong>
                <small
                    ><RouterLink to="/settings/security">{{
                        t('overview.stats.manage')
                    }}</RouterLink></small
                >
            </div>
        </section>

        <div class="overview-grid">
            <section class="plain-panel" aria-labelledby="node-status-title">
                <div class="panel-heading">
                    <h2 id="node-status-title">
                        {{ t('overview.nodes.title') }}
                    </h2>
                    <RouterLink class="panel-link" to="/nodes">{{
                        t('overview.nodes.all')
                    }}</RouterLink>
                </div>
                <div v-if="current.length" class="table-frame">
                    <table class="data-table">
                        <thead>
                            <tr>
                                <th>{{ t('overview.nodes.name') }}</th>
                                <th>{{ t('overview.nodes.status') }}</th>
                                <th>{{ t('overview.nodes.lastSeen') }}</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr
                                v-for="node in current.slice(0, 6)"
                                :key="node.id"
                            >
                                <td>{{ node.name }}</td>
                                <td>
                                    <ElTag :type="state(node).tag">{{
                                        t(
                                            `overview.nodes.states.${state(node).key}`,
                                        )
                                    }}</ElTag>
                                </td>
                                <td class="cell-mono">
                                    {{
                                        node.lastSeenAt
                                            ? formatSince(node.lastSeenAt, now)
                                            : '—'
                                    }}
                                </td>
                            </tr>
                        </tbody>
                    </table>
                </div>
                <p v-else class="panel-empty">
                    {{
                        nodesUnavailable
                            ? t('overview.nodes.unavailable')
                            : t('overview.nodes.empty')
                    }}
                </p>
            </section>

            <section class="plain-panel" aria-labelledby="expiry-title">
                <div class="panel-heading">
                    <h2 id="expiry-title">{{ t('overview.expiry.title') }}</h2>
                    <RouterLink class="panel-link" to="/certificates">{{
                        t('overview.expiry.all')
                    }}</RouterLink>
                </div>
                <ul v-if="expiring.length" class="expiry-list">
                    <li v-for="item in expiring" :key="item.id">
                        <span>{{ item.domain }}</span>
                        <span class="readout">{{
                            formatDate(item.notAfter)
                        }}</span>
                    </li>
                </ul>
                <p v-else class="panel-empty">
                    {{
                        certificatesUnavailable
                            ? t('overview.expiry.unavailable')
                            : t('overview.expiry.empty')
                    }}
                </p>
            </section>
        </div>
    </div>
</template>
