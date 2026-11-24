<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Lock from '~icons/lucide/lock'
import Palette from '~icons/lucide/palette'
import ChevronRight from '~icons/lucide/chevron-right'
import LogOut from '~icons/lucide/log-out'
import AuthFeedback from '@/components/AuthFeedback.vue'
import { authErrorMessage } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'

// The account centre: who is signed in, the account's pages, and signing out.
const open = defineModel<boolean>({ required: true })
const { t } = useI18n()
const auth = useAuthStore()
const router = useRouter()
const signingOut = ref(false)
const error = ref('')
const initial = computed(
    () => auth.user?.username.slice(0, 1).toUpperCase() ?? '',
)

async function signOut() {
    if (signingOut.value) return
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
    <ElDialog
        v-model="open"
        class="account-center"
        :title="t('common.shell.account.title')"
        width="min(24rem, calc(100vw - 2rem))"
        append-to-body
        align-center
        @closed="error = ''"
    >
        <div v-if="auth.user" class="account-card">
            <span class="account-avatar is-large" aria-hidden="true">{{
                initial
            }}</span>
            <div class="account-card-text">
                <strong>{{ auth.user.username }}</strong>
                <small>{{
                    auth.user.twoFactorEnabled
                        ? t('common.shell.account.twoFactorOn')
                        : t('common.shell.account.twoFactorOff')
                }}</small>
            </div>
        </div>
        <nav
            class="account-links"
            :aria-label="t('common.shell.account.title')"
        >
            <RouterLink
                to="/settings/security"
                class="account-link"
                @click="open = false"
            >
                <Lock aria-hidden="true" />
                <span>{{ t('common.routes.security') }}</span>
                <ChevronRight class="account-link-chevron" aria-hidden="true" />
            </RouterLink>
            <RouterLink
                to="/settings/appearance"
                class="account-link"
                @click="open = false"
            >
                <Palette aria-hidden="true" />
                <span>{{ t('common.routes.appearance') }}</span>
                <ChevronRight class="account-link-chevron" aria-hidden="true" />
            </RouterLink>
        </nav>
        <AuthFeedback :message="error" />
        <template #footer>
            <ElButton type="danger" :loading="signingOut" @click="signOut"
                ><LogOut
                    v-if="!signingOut"
                    class="leading-icon"
                    aria-hidden="true"
                />{{ t('common.shell.signOut') }}</ElButton
            >
        </template>
    </ElDialog>
</template>
