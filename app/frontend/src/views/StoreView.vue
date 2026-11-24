<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import PageIntro from '@/components/PageIntro.vue'
import Reveal from '@/components/Reveal.vue'
import PasswordField from '@/components/PasswordField.vue'
import AuthFeedback from '@/components/AuthFeedback.vue'
import { AuthApiError, authErrorMessage } from '@/api/auth'
import { appsApi, type AppPort, type StoreApp } from '@/api/apps'
import { useAppsStore } from '@/stores/apps'
import { useSessionExpiry } from '@/composables/useSessionExpiry'
import { useCatalogText } from '@/composables/useCatalogText'
import { formatNumber } from '@/i18n/format'
import { useCachedPage } from '@/composables/useCachedPage'

const { t } = useI18n()
const catalogText = useCatalogText()
const redirectIfExpired = useSessionExpiry()
const store = useAppsStore()
const unavailable = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
// The app whose uninstall form is open; the password lives only here.
const uninstalling = ref<StoreApp | null>(null)
const password = ref('')

const summary = computed(() => {
    if (unavailable.value && !store.apps) return t('common.status.unavailable')
    if (!store.apps) return t('common.status.loading')
    return t('apps.store.installedCount', {
        count: formatNumber(store.installed.length),
    })
})

function state(app: StoreApp) {
    if (!app.installed)
        return {
            label: t('apps.store.state.notInstalled'),
            tag: 'info',
        } as const
    return app.installed.configured
        ? ({ label: t('apps.store.state.installed'), tag: 'success' } as const)
        : ({
              label: t('apps.store.state.needsSettings'),
              tag: 'warning',
          } as const)
}

function portText(port: AppPort) {
    const transport = port.transport.toUpperCase()
    return port.port === null
        ? `${transport} · ${t('apps.store.randomPort')}`
        : `${transport} ${port.port}`
}

function clearSecrets() {
    password.value = ''
}

function closeForms() {
    uninstalling.value = null
    password.value = ''
    error.value = ''
}

async function load(background = false) {
    try {
        await store.load({ background })
        unavailable.value = false
    } catch (cause) {
        if (await redirectIfExpired(cause, clearSecrets)) return
        // Keep the last real catalog on a failed refresh; never invent one.
        unavailable.value = true
    }
}

async function install(app: StoreApp) {
    if (busy.value) return
    closeForms()
    notice.value = ''
    busy.value = true
    try {
        store.update(await appsApi.install(app.id))
        notice.value = t('apps.store.installedNotice', {
            name: catalogText(app.name, app.id),
        })
    } catch (cause) {
        if (await redirectIfExpired(cause, clearSecrets)) return
        error.value = authErrorMessage(cause)
        await load()
    } finally {
        busy.value = false
    }
}

async function openUninstall(app: StoreApp) {
    closeForms()
    notice.value = ''
    uninstalling.value = app
    await nextTick()
    document.getElementById('app-uninstall-password')?.focus()
}

async function submitUninstall() {
    const app = uninstalling.value
    if (busy.value || !app) return
    error.value = ''
    if (!password.value) {
        error.value = t('apps.uninstall.missingPassword')
        return
    }
    busy.value = true
    try {
        await appsApi.uninstall(app.id, password.value)
        closeForms()
        store.markUninstalled(app.id)
        notice.value = t('apps.uninstall.done', {
            name: catalogText(app.name, app.id),
        })
        await load()
    } catch (cause) {
        password.value = ''
        if (await redirectIfExpired(cause, clearSecrets)) return
        error.value = authErrorMessage(cause)
        if (
            cause instanceof AuthApiError &&
            ['APP_NOT_INSTALLED', 'APP_DEPLOYED', 'APP_NOT_FOUND'].includes(
                cause.code,
            )
        ) {
            uninstalling.value = null
            await load()
        } else {
            // Disabled inputs cannot take focus; re-enable the form first.
            busy.value = false
            await nextTick()
            document.getElementById('app-uninstall-password')?.focus()
        }
    } finally {
        busy.value = false
    }
}

onMounted(() => void load())
useCachedPage({ pause: clearSecrets, resume: () => void load(true) })
onBeforeUnmount(clearSecrets)
</script>

<template>
    <div>
        <PageIntro
            :eyebrow="t('common.shell.groups.software')"
            :title="t('common.routes.store')"
            :description="t('apps.store.description')"
        />

        <section class="plain-panel store-panel" aria-labelledby="store-title">
            <div class="panel-heading">
                <h2 id="store-title">{{ t('apps.store.listTitle') }}</h2>
                <ElTag
                    :type="store.installed.length ? 'success' : 'info'"
                    effect="plain"
                    data-testid="store-summary"
                    >{{ summary }}</ElTag
                >
            </div>
            <p class="store-note">{{ t('apps.store.builtIn') }}</p>

            <AuthFeedback :message="error" class="security-feedback" />
            <AuthFeedback
                :message="notice"
                tone="notice"
                class="security-feedback"
            />

            <p v-if="unavailable" class="nodes-unavailable" role="status">
                {{
                    store.apps
                        ? t('apps.store.unavailable.stale')
                        : t('apps.store.unavailable.empty')
                }}
            </p>

            <ul
                v-if="store.apps"
                class="app-list"
                :aria-label="t('apps.store.listLabel')"
            >
                <li
                    v-for="app in store.apps"
                    :key="app.id"
                    class="app-item"
                    :data-app-id="app.id"
                >
                    <div class="app-heading">
                        <div class="app-title">
                            <h3>{{ catalogText(app.name, app.id) }}</h3>
                            <small>{{
                                t('apps.store.version', {
                                    version: app.version,
                                })
                            }}</small>
                        </div>
                        <ElTag :type="state(app).tag" effect="plain">{{
                            state(app).label
                        }}</ElTag>
                    </div>
                    <p class="app-description">
                        {{ catalogText(app.description) }}
                    </p>
                    <dl class="app-meta">
                        <div>
                            <dt>{{ t('apps.store.protocols') }}</dt>
                            <dd>
                                <span
                                    v-for="protocol in app.protocols"
                                    :key="protocol.id"
                                    class="app-chip"
                                    >{{ protocol.name }} ·
                                    {{ protocol.transport.toUpperCase() }}</span
                                >
                            </dd>
                        </div>
                        <div>
                            <dt>{{ t('apps.store.ports') }}</dt>
                            <dd>
                                <span
                                    v-for="port in app.ports"
                                    :key="port.setting"
                                    class="app-chip"
                                    >{{ portText(port) }}</span
                                >
                            </dd>
                        </div>
                        <div class="app-image">
                            <dt>{{ t('apps.store.image') }}</dt>
                            <dd>
                                <code v-if="app.image.reference">{{
                                    app.image.reference
                                }}</code>
                                <span v-else>{{
                                    t('apps.store.localBuild', {
                                        path: app.image.build,
                                    })
                                }}</span>
                            </dd>
                        </div>
                    </dl>
                    <Reveal>
                        <div
                            v-if="uninstalling?.id !== app.id"
                            class="node-action"
                        >
                            <ElButton
                                v-if="!app.installed"
                                :disabled="busy"
                                @click="install(app)"
                                >{{ t('apps.store.install') }}</ElButton
                            >
                            <template v-else>
                                <RouterLink
                                    :to="`/apps/${app.id}`"
                                    class="app-manage-link"
                                    >{{
                                        t('apps.store.manage')
                                    }}</RouterLink><ElButton
                                    :disabled="busy"
                                    @click="openUninstall(app)"
                                    >{{ t('apps.uninstall.open') }}</ElButton
                                >
                            </template>
                        </div>
                        <form
                            v-else
                            class="node-revoke security-form auth-form"
                            :aria-busy="busy"
                            @submit.prevent="submitUninstall"
                        >
                            <p class="auth-description">
                                {{ t('apps.uninstall.description') }}
                            </p>
                            <PasswordField
                                id="app-uninstall-password"
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
                                        t('apps.uninstall.submit')
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
        </section>
    </div>
</template>
