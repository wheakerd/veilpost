<script setup lang="ts">
import {
    computed,
    nextTick,
    onBeforeUnmount,
    reactive,
    ref,
    toRef,
    watch,
} from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import PackageSearch from '~icons/lucide/package-search'
import PageIntro from '@/components/PageIntro.vue'
import PasswordField from '@/components/PasswordField.vue'
import AuthFeedback from '@/components/AuthFeedback.vue'
import LoadingState from '@/components/LoadingState.vue'
import { AuthApiError, authErrorMessage } from '@/api/auth'
import {
    appsApi,
    appIdPattern,
    type AppSetting,
    type AppSettings,
    type SettingValue,
    type StoreApp,
} from '@/api/apps'
import { useAppsStore } from '@/stores/apps'
import { useSessionExpiry } from '@/composables/useSessionExpiry'
import { useCatalogText } from '@/composables/useCatalogText'
import { formatDateTime } from '@/i18n/format'
import { useCachedPage } from '@/composables/useCachedPage'

const props = defineProps<{ id: string }>()
const { t } = useI18n()
const catalogText = useCatalogText()
const redirectIfExpired = useSessionExpiry()
const store = useAppsStore()
const id = toRef(props, 'id')
const phase = ref<
    'loading' | 'ready' | 'missing' | 'notInstalled' | 'unavailable'
>('loading')
const app = ref<StoreApp | null>(null)
const settings = ref<AppSettings | null>(null)
// Form text for non-sensitive settings (booleans stay booleans).
const values = reactive<Record<string, string | boolean>>({})
// Sensitive values typed into the form. They stay in this component until
// they are sent and are cleared after saving, on leaving and on expiry.
const secrets = reactive<Record<string, string>>({})
const clearing = reactive<Record<string, boolean>>({})
const invalidField = ref('')
const busy = ref(false)
const error = ref('')
const notice = ref('')
const uninstalling = ref(false)
const password = ref('')

const name = computed(() =>
    app.value ? catalogText(app.value.name, app.value.id) : id.value,
)

function reset<T>(record: Record<string, T>) {
    for (const key of Object.keys(record)) Reflect.deleteProperty(record, key)
}

function clearSecrets() {
    reset(secrets)
    reset(clearing)
    password.value = ''
}

function fill(current: StoreApp, loaded: AppSettings) {
    reset(values)
    for (const setting of current.settings) {
        if (setting.sensitive) continue
        const value = loaded.values[setting.key]
        values[setting.key] =
            setting.type === 'boolean'
                ? value === true
                : value === null
                  ? ''
                  : String(value)
    }
}

async function load() {
    phase.value = 'loading'
    clearSecrets()
    uninstalling.value = false
    invalidField.value = ''
    error.value = ''
    settings.value = null
    const requested = id.value
    try {
        const list = await store.load()
        if (requested !== id.value) return
        const current = list.find((item) => item.id === requested)
        app.value = current ?? null
        if (!current) {
            phase.value = 'missing'
            return
        }
        if (!current.installed) {
            phase.value = 'notInstalled'
            return
        }
        const loaded = await appsApi.settings(current)
        if (requested !== id.value) return
        settings.value = loaded
        fill(current, loaded)
        phase.value = 'ready'
    } catch (cause) {
        if (await redirectIfExpired(cause, clearSecrets)) return
        if (
            cause instanceof AuthApiError &&
            cause.code === 'APP_NOT_INSTALLED'
        ) {
            store.markUninstalled(requested)
            phase.value = 'notInstalled'
            return
        }
        phase.value = 'unavailable'
    }
}

watch(
    id,
    () => {
        notice.value = ''
        if (!appIdPattern.test(id.value)) {
            app.value = null
            phase.value = 'missing'
            return
        }
        void load()
    },
    { immediate: true },
)

function required(setting: AppSetting) {
    const condition = setting.requiredWhen
    if (!condition) return setting.required
    return values[condition.setting] === condition.equals
}

function inputValue(setting: AppSetting): SettingValue {
    const value = values[setting.key]
    if (typeof value === 'boolean') return value
    const text = (value ?? '').trim()
    if (!text) return null
    // Numbers the browser can read are sent as numbers; anything else goes
    // to the server as typed, which rejects it with INVALID_SETTING.
    if (
        (setting.type === 'port' || setting.type === 'integer') &&
        /^-?\d+$/.test(text)
    )
        return Number(text)
    return text
}

function settingLabel(key: string) {
    const setting = app.value?.settings.find((item) => item.key === key)
    return setting ? catalogText(setting.label, key) : key
}

async function save() {
    const current = app.value
    if (busy.value || !current) return
    error.value = ''
    notice.value = ''
    invalidField.value = ''
    const update = {
        values: {} as Record<string, SettingValue>,
        secrets: {} as Record<string, string | null>,
    }
    for (const setting of current.settings) {
        if (!setting.sensitive) update.values[setting.key] = inputValue(setting)
        else if (clearing[setting.key]) update.secrets[setting.key] = null
        else {
            const secret = secrets[setting.key]
            if (secret) update.secrets[setting.key] = secret
        }
    }
    busy.value = true
    try {
        const saved = await appsApi.updateSettings(current, update)
        clearSecrets()
        settings.value = saved
        fill(current, saved)
        notice.value = t('apps.app.settings.saved')
        // Refresh the configured state and ports shown in the store.
        void store.load({ background: true }).catch(() => undefined)
    } catch (cause) {
        if (await redirectIfExpired(cause, clearSecrets)) return
        if (
            cause instanceof AuthApiError &&
            cause.field &&
            current.settings.some((setting) => setting.key === cause.field)
        ) {
            invalidField.value = cause.field
            error.value = t(
                cause.code === 'SETTING_REQUIRED'
                    ? 'apps.app.settings.requiredField'
                    : 'apps.app.settings.invalidField',
                { field: settingLabel(cause.field) },
            )
            // Disabled inputs cannot take focus; re-enable the form first.
            busy.value = false
            await nextTick()
            document.getElementById(`setting-${cause.field}`)?.focus()
            return
        }
        error.value = authErrorMessage(cause)
        if (
            cause instanceof AuthApiError &&
            cause.code === 'APP_NOT_INSTALLED'
        ) {
            store.markUninstalled(current.id)
            phase.value = 'notInstalled'
        }
    } finally {
        busy.value = false
    }
}

async function openUninstall() {
    error.value = ''
    notice.value = ''
    uninstalling.value = true
    await nextTick()
    document.getElementById('app-uninstall-password')?.focus()
}

function closeUninstall() {
    uninstalling.value = false
    password.value = ''
    error.value = ''
}

async function submitUninstall() {
    const current = app.value
    if (busy.value || !current) return
    error.value = ''
    if (!password.value) {
        error.value = t('apps.uninstall.missingPassword')
        return
    }
    busy.value = true
    try {
        await appsApi.uninstall(current.id, password.value)
        clearSecrets()
        uninstalling.value = false
        settings.value = null
        store.markUninstalled(current.id)
        phase.value = 'notInstalled'
        notice.value = t('apps.uninstall.done', { name: name.value })
        void store.load({ background: true }).catch(() => undefined)
    } catch (cause) {
        password.value = ''
        if (await redirectIfExpired(cause, clearSecrets)) return
        error.value = authErrorMessage(cause)
        if (
            cause instanceof AuthApiError &&
            cause.code === 'APP_NOT_INSTALLED'
        ) {
            store.markUninstalled(current.id)
            phase.value = 'notInstalled'
            return
        }
        busy.value = false
        await nextTick()
        document.getElementById('app-uninstall-password')?.focus()
    } finally {
        busy.value = false
    }
}

// Hidden in the page cache: sensitive values and the password leave memory;
// the form keeps its other values.
useCachedPage({ pause: clearSecrets, resume: () => undefined })
onBeforeUnmount(clearSecrets)
</script>

<template>
    <div>
        <div v-if="phase === 'loading'" class="app-loading">
            <LoadingState :title="t('common.status.loading')" />
        </div>

        <div
            v-else-if="phase === 'missing' || phase === 'notInstalled'"
            class="app-guidance plain-panel empty-state"
        >
            <span class="empty-icon" aria-hidden="true"><PackageSearch /></span>
            <h1>
                {{
                    phase === 'missing'
                        ? t('apps.app.notFound.title')
                        : t('apps.app.notInstalled.title', { name })
                }}
            </h1>
            <p>
                {{
                    phase === 'missing'
                        ? t('apps.app.notFound.description')
                        : t('apps.app.notInstalled.description')
                }}
            </p>
            <RouterLink to="/store" class="app-manage-link">{{
                t('apps.app.toStore')
            }}</RouterLink>
            <AuthFeedback :message="notice" tone="notice" />
        </div>

        <div
            v-else-if="phase === 'unavailable' || !app"
            class="plain-panel empty-state"
        >
            <p role="status">{{ t('apps.app.unavailable') }}</p>
        </div>

        <template v-else>
            <PageIntro
                :title="name"
                :description="catalogText(app.description)"
            />

            <section
                class="plain-panel app-panel"
                aria-labelledby="settings-title"
            >
                <div class="panel-heading app-settings-heading">
                    <h2 id="settings-title">
                        {{ t('apps.app.settings.title') }}
                    </h2>
                    <ElTag
                        v-if="settings"
                        class="app-status"
                        :type="settings.configured ? 'success' : 'warning'"
                        effect="plain"
                        data-testid="settings-state"
                        >{{
                            settings.configured
                                ? t('apps.app.settings.complete')
                                : t('apps.app.settings.incomplete')
                        }}</ElTag
                    >
                </div>
                <p class="store-note">
                    {{ t('apps.app.settings.description') }}
                </p>
                <p v-if="settings" class="store-note">
                    {{
                        t('apps.app.settings.updatedAt', {
                            time: formatDateTime(settings.updatedAt),
                        })
                    }}
                </p>

                <form
                    class="app-settings auth-form"
                    novalidate
                    :aria-busy="busy"
                    @submit.prevent="save"
                >
                    <div
                        v-for="setting in app.settings"
                        :key="setting.key"
                        class="form-field"
                    >
                        <label :for="`setting-${setting.key}`"
                            >{{ catalogText(setting.label, setting.key)
                            }}<span
                                v-if="required(setting)"
                                class="field-required"
                                >{{ t('apps.app.settings.required') }}</span
                            ></label
                        >
                        <template v-if="setting.sensitive && settings">
                            <div class="secret-state">
                                <ElTag
                                    :type="
                                        settings.secrets[setting.key]
                                            ? 'success'
                                            : 'info'
                                    "
                                    effect="plain"
                                    :data-testid="`secret-${setting.key}`"
                                    >{{
                                        settings.secrets[setting.key]
                                            ? t('apps.app.settings.secretSet')
                                            : t('apps.app.settings.secretUnset')
                                    }}</ElTag
                                >
                                <ElButton
                                    v-if="settings.secrets[setting.key]"
                                    size="small"
                                    :disabled="busy"
                                    @click="
                                        clearing[setting.key] =
                                            !clearing[setting.key]
                                    "
                                    >{{
                                        clearing[setting.key]
                                            ? t('apps.app.settings.secretUndo')
                                            : t('apps.app.settings.secretClear')
                                    }}</ElButton
                                >
                            </div>
                            <ElInput
                                :id="`setting-${setting.key}`"
                                v-model="secrets[setting.key]"
                                type="password"
                                autocomplete="new-password"
                                spellcheck="false"
                                :maxlength="setting.maxLength ?? undefined"
                                :placeholder="
                                    settings.secrets[setting.key]
                                        ? t('apps.app.settings.secretKeep')
                                        : ''
                                "
                                :disabled="busy || clearing[setting.key]"
                                :aria-invalid="invalidField === setting.key"
                                :aria-describedby="
                                    setting.hint
                                        ? `setting-${setting.key}-hint`
                                        : undefined
                                "
                            />
                            <small
                                v-if="clearing[setting.key]"
                                class="secret-clearing"
                                >{{
                                    t('apps.app.settings.secretClearing')
                                }}</small
                            >
                        </template>
                        <ElSelect
                            v-else-if="setting.type === 'enum'"
                            :id="`setting-${setting.key}`"
                            v-model="values[setting.key]"
                            class="app-select"
                            :disabled="busy"
                            :aria-invalid="invalidField === setting.key"
                            :aria-describedby="
                                setting.hint
                                    ? `setting-${setting.key}-hint`
                                    : undefined
                            "
                        >
                            <ElOption
                                v-for="option in setting.options"
                                :key="option.value"
                                :value="option.value"
                                :label="catalogText(option.label, option.value)"
                            />
                        </ElSelect>
                        <span
                            v-else-if="setting.type === 'boolean'"
                            class="app-toggle"
                        >
                            <ElSwitch
                                :id="`setting-${setting.key}`"
                                v-model="values[setting.key]"
                                :disabled="busy"
                                :aria-describedby="
                                    setting.hint
                                        ? `setting-${setting.key}-hint`
                                        : undefined
                                "
                            />
                        </span>
                        <ElInput
                            v-else
                            :id="`setting-${setting.key}`"
                            :model-value="String(values[setting.key] ?? '')"
                            type="text"
                            :inputmode="
                                setting.type === 'port' ||
                                setting.type === 'integer'
                                    ? 'numeric'
                                    : setting.type === 'url'
                                      ? 'url'
                                      : 'text'
                            "
                            autocomplete="off"
                            spellcheck="false"
                            :disabled="busy"
                            :aria-invalid="invalidField === setting.key"
                            :aria-describedby="
                                setting.hint
                                    ? `setting-${setting.key}-hint`
                                    : undefined
                            "
                            @update:model-value="values[setting.key] = $event"
                        />
                        <small
                            v-if="setting.hint"
                            :id="`setting-${setting.key}-hint`"
                            >{{ catalogText(setting.hint) }}</small
                        >
                    </div>
                    <AuthFeedback :message="uninstalling ? '' : error" />
                    <AuthFeedback :message="notice" tone="notice" />
                    <div class="form-actions">
                        <ElButton
                            type="primary"
                            native-type="submit"
                            :loading="busy"
                            >{{ t('apps.app.settings.save') }}</ElButton
                        >
                    </div>
                </form>
            </section>

            <section
                class="plain-panel app-panel"
                aria-labelledby="management-title"
            >
                <div class="panel-heading">
                    <h2 id="management-title">
                        {{ t('apps.app.management.title') }}
                    </h2>
                </div>
                <dl class="app-meta">
                    <div>
                        <dt>{{ t('apps.app.management.version') }}</dt>
                        <dd>{{ app.installed?.version ?? app.version }}</dd>
                    </div>
                    <div v-if="app.installed">
                        <dt>{{ t('apps.app.management.installedAt') }}</dt>
                        <dd>{{ formatDateTime(app.installed.installedAt) }}</dd>
                    </div>
                    <div class="app-image">
                        <dt>{{ t('apps.app.management.image') }}</dt>
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
                    <div v-if="app.image.platforms.length">
                        <dt>{{ t('apps.app.management.platforms') }}</dt>
                        <dd>{{ app.image.platforms.join(', ') }}</dd>
                    </div>
                    <div>
                        <dt>{{ t('apps.app.management.source') }}</dt>
                        <dd>
                            <a
                                :href="app.image.source"
                                target="_blank"
                                rel="noopener noreferrer"
                                >{{
                                    app.image.source.replace('https://', '')
                                }}</a
                            >
                        </dd>
                    </div>
                </dl>

                <div class="deploy-state" aria-labelledby="deploy-title">
                    <h3 id="deploy-title">
                        {{ t('apps.app.management.deployTitle') }}
                    </h3>
                    <p data-testid="deploy-unavailable">
                        {{ t('apps.app.management.deployUnavailable') }}
                    </p>
                    <ElButton disabled>{{
                        t('apps.app.management.deployAction')
                    }}</ElButton>
                </div>

                <div class="deploy-state">
                    <ElButton
                        v-if="!uninstalling"
                        :disabled="busy"
                        @click="openUninstall"
                        >{{ t('apps.uninstall.open') }}</ElButton
                    >
                    <form
                        v-else
                        class="security-form auth-form"
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
                        <AuthFeedback :message="error" />
                        <div class="form-actions">
                            <ElButton
                                type="danger"
                                native-type="submit"
                                :loading="busy"
                                >{{
                                    t('apps.uninstall.submit')
                                }}</ElButton><ElButton
                                :disabled="busy"
                                @click="closeUninstall"
                                >{{ t('common.actions.cancel') }}</ElButton
                            >
                        </div>
                    </form>
                </div>
            </section>
        </template>
    </div>
</template>
