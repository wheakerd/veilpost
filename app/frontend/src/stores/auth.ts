import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { authApi, authErrorMessage, type AuthSession } from '@/api/auth'

export const useAuthStore = defineStore('auth', () => {
    const session = ref<AuthSession>({ status: 'anonymous' })
    const serviceError = ref('')
    const setupRequired = computed(
        () =>
            session.value.status === 'anonymous' &&
            session.value.setupRequired === true,
    )
    const user = computed(() =>
        session.value.status === 'authenticated' ? session.value.user : null,
    )
    const challengeExpiresAt = computed(() =>
        session.value.status === 'mfa_required'
            ? session.value.expiresAt
            : null,
    )
    let restoring: Promise<void> | undefined
    // When the server last confirmed the session; 0 forces the next restore.
    let confirmedAt = 0

    function clear() {
        session.value = { status: 'anonymous' }
        confirmedAt = 0
    }

    function accept(nextSession: AuthSession) {
        session.value = nextSession
        serviceError.value = ''
        confirmedAt = Date.now()
    }

    // maxAge skips the request when a signed-in session was confirmed recently;
    // the backend still authorizes every request on its own.
    async function restore(maxAge = 0) {
        if (
            maxAge > 0 &&
            session.value.status === 'authenticated' &&
            Date.now() - confirmedAt < maxAge
        )
            return
        if (restoring) return restoring
        restoring = (async () => {
            try {
                accept(await authApi.session())
                if (
                    challengeExpiresAt.value &&
                    Date.parse(challengeExpiresAt.value) <= Date.now()
                ) {
                    clear()
                }
            } catch (error) {
                clear()
                serviceError.value = authErrorMessage(error)
            }
        })()
        try {
            await restoring
        } finally {
            restoring = undefined
        }
    }

    async function logout() {
        await authApi.logout()
        clear()
    }

    return {
        session,
        user,
        challengeExpiresAt,
        serviceError,
        setupRequired,
        accept,
        clear,
        restore,
        logout,
    }
})
