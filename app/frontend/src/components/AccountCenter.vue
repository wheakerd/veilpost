<script setup lang="ts">
import { computed, onScopeDispose, ref, useTemplateRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import UserRound from '~icons/lucide/user-round'
import Lock from '~icons/lucide/lock'
import Monitor from '~icons/lucide/monitor'
import Palette from '~icons/lucide/palette'
import LogOut from '~icons/lucide/log-out'
import AccountAvatar from '@/components/AccountAvatar.vue'
import AccountSecurity from '@/components/AccountSecurity.vue'
import AuthFeedback from '@/components/AuthFeedback.vue'
import ConsoleDialog from '@/components/ConsoleDialog.vue'
import { authErrorMessage } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'

const open = defineModel<boolean>({ required: true })
const { t } = useI18n()
const auth = useAuthStore()
const router = useRouter()
const signingOut = ref(false)
const error = ref('')
const security = useTemplateRef<{ confirmLeave: () => Promise<boolean> }>(
    'security',
)

const sections = [
    {
        id: 'profile',
        label: 'common.shell.account.profile',
        icon: UserRound,
    },
    {
        id: 'security',
        label: 'common.shell.account.loginSecurity',
        icon: Lock,
    },
    { id: 'sessions', label: 'security.sessions.title', icon: Monitor },
] as const
type Section = (typeof sections)[number]['id']
const section = ref<Section>('profile')
const heading = computed(() =>
    t(
        (sections.find((item) => item.id === section.value) ?? sections[0])
            .label,
    ),
)

function reset() {
    section.value = 'profile'
    error.value = ''
}

watch(open, (visible) => {
    if (visible) reset()
})

async function canLeave() {
    return !signingOut.value && (await (security.value?.confirmLeave() ?? true))
}

onScopeDispose(router.beforeEach(() => (open.value ? canLeave() : true)))
watch(
    () => router.currentRoute.value.fullPath,
    () => {
        open.value = false
    },
)

function close(done: () => void) {
    void canLeave().then((allowed) => {
        if (allowed) done()
    })
}

async function select(next: Section) {
    if (next !== section.value && (await canLeave())) section.value = next
}

async function openAppearance() {
    if (!(await canLeave())) return
    open.value = false
    await router.push('/settings/appearance')
}

async function signOut() {
    if (!(await canLeave()) || signingOut.value) return
    signingOut.value = true
    error.value = ''
    try {
        await auth.logout()
        open.value = false
        await router.replace('/login')
    } catch (cause) {
        error.value = authErrorMessage(cause)
    } finally {
        signingOut.value = false
    }
}
</script>

<template>
    <ConsoleDialog
        v-model="open"
        :title="t('common.shell.account.title')"
        :heading="heading"
        :before-close="close"
        @close="reset"
    >
        <template #sidebar>
            <div class="account-sidebar">
                <div v-if="auth.user" class="account-identity">
                    <AccountAvatar :username="auth.user.username" />
                    <div class="account-identity-text">
                        <strong>{{ auth.user.username }}</strong>
                        <small>{{
                            auth.user.twoFactorEnabled
                                ? t('common.shell.account.twoFactorOn')
                                : t('common.shell.account.twoFactorOff')
                        }}</small>
                    </div>
                </div>
                <nav
                    class="account-navigation"
                    :aria-label="t('common.shell.account.title')"
                >
                    <ElButton
                        v-for="item in sections"
                        :key="item.id"
                        text
                        class="account-section-button"
                        :aria-current="section === item.id ? 'true' : undefined"
                        @click="select(item.id)"
                    >
                        <component :is="item.icon" aria-hidden="true" />
                        {{ t(item.label) }}
                    </ElButton>
                    <ElButton
                        text
                        class="account-section-button"
                        @click="openAppearance"
                    >
                        <Palette aria-hidden="true" />
                        {{ t('common.routes.appearance') }}
                    </ElButton>
                </nav>
                <div class="account-sidebar-footer">
                    <ElButton
                        text
                        type="danger"
                        class="account-section-button account-sign-out"
                        :loading="signingOut"
                        @click="signOut"
                    >
                        <LogOut v-if="!signingOut" aria-hidden="true" />
                        {{ t('common.shell.signOut') }}
                    </ElButton>
                </div>
            </div>
        </template>

        <AuthFeedback :message="error" class="account-feedback" />
        <dl v-if="section === 'profile' && auth.user" class="account-profile">
            <div>
                <dt>{{ t('auth.login.username') }}</dt>
                <dd>{{ auth.user.username }}</dd>
            </div>
        </dl>
        <AccountSecurity
            v-else-if="open && section !== 'profile'"
            :key="section"
            ref="security"
            :section="section"
        />
    </ConsoleDialog>
</template>

<style lang="scss">
.account-sidebar {
    display: flex;
    min-block-size: 100%;
    flex-direction: column;
    padding: var(--app-space-20) var(--app-space-14) var(--app-space-14);
}

.account-identity {
    display: flex;
    align-items: center;
    gap: var(--app-space-12);
    padding: var(--app-space-4) var(--app-space-8) var(--app-space-16);

    .account-avatar {
        flex: 0 0 auto;
        width: 2.75rem;
        height: 2.75rem;
        border-color: transparent;
        background: var(--app-primary);
        color: var(--app-on-primary);
        font-size: var(--app-font-size-heading);
    }
}

.account-identity-text {
    display: grid;
    min-inline-size: 0;
    gap: 0.1875rem;

    strong {
        font-size: var(--app-font-size-subheading);
        font-weight: var(--app-font-weight-semibold);
        line-height: 1.4;
        overflow-wrap: anywhere;
    }

    small {
        color: var(--app-text-secondary);
        font-size: var(--app-font-size-small);
        line-height: 1.35;
        overflow-wrap: anywhere;
    }
}

.account-navigation {
    display: grid;
    gap: var(--app-space-4);
}

:root .account-sidebar .account-section-button.el-button.is-text {
    justify-content: flex-start;
    width: 100%;
    min-height: 2.375rem;
    margin-inline-start: 0;
    padding: var(--app-space-8) var(--app-space-10);
    border-radius: var(--app-radius-control);
    color: var(--app-text);
    font-size: var(--app-font-size-body);
    text-align: start;

    > span {
        display: flex;
        align-items: center;
        gap: var(--app-space-10);
        min-inline-size: 0;
    }

    &:hover {
        background: var(--app-primary-soft);
    }

    &[aria-current='true'] {
        background: var(--app-account-selected-fill);
        color: var(--app-account-selected-ink);
        font-weight: var(--app-font-weight-semibold);
    }

    &.account-sign-out {
        color: var(--app-danger);

        &:hover {
            background: var(--app-danger-soft);
        }
    }
}

.account-sidebar-footer {
    margin-block-start: auto;
    padding-block-start: var(--app-space-8);
    border-block-start: var(--app-border-width) solid var(--app-border);
}

.account-profile {
    margin: 0;

    > div {
        display: grid;
        grid-template-columns: minmax(0, min(8rem, 30%)) minmax(0, 1fr);
        align-items: baseline;
        gap: var(--app-space-12);
        min-block-size: 3.25rem;
        padding: var(--app-space-16);
        border-radius: var(--app-radius-row);
        background: var(--app-profile-row-fill);
        font-size: var(--app-font-size-body);
        line-height: 1.45;
        overflow-wrap: anywhere;
    }

    dd {
        margin: 0;
        font-weight: var(--app-font-weight-medium);
    }
}

.account-feedback:not(:empty) {
    margin-block-end: var(--app-space-16);
}

@media (max-width: 47.5rem) {
    .account-sidebar {
        padding: var(--app-space-12);
    }

    .account-identity {
        padding-inline-end: 2.75rem;
        padding-block-end: var(--app-space-10);
    }

    .account-navigation {
        grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    .account-sidebar-footer {
        margin-block-start: var(--app-space-8);
        padding-block-start: var(--app-space-6);
    }
}
</style>
