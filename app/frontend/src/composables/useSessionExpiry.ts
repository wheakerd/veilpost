import { useRoute, useRouter } from 'vue-router'
import { AuthApiError } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { safeReturnPath } from '@/router/returnPath'

// Returns true after sending the user to sign in again when the server
// reports that the full session has expired. cleanup runs before leaving so
// temporary secrets never block or outlive the redirect.
export function useSessionExpiry() {
    const auth = useAuthStore()
    const route = useRoute()
    const router = useRouter()

    return async function redirectIfExpired(
        cause: unknown,
        cleanup?: () => void,
    ) {
        if (!(
            cause instanceof AuthApiError && cause.code === 'SESSION_EXPIRED'
        ))
            return false
        cleanup?.()
        auth.clear()
        await router.replace({
            path: '/login',
            query: {
                redirect: safeReturnPath(route.fullPath),
                reason: 'expired',
            },
        })
        return true
    }
}
