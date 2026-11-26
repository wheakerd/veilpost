<script setup lang="ts">
import { onBeforeUnmount, useTemplateRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { animate, motion } from 'motion-v'
import { useInterfaceMotion } from '@/composables/useInterfaceMotion'
import { usePreferencesStore } from '@/stores/preferences'
import { useModeToggle } from '@/composables/useModeToggle'
import { currentLocale, messages, supportedLocales, type Locale } from '@/i18n'
import { enteringConsole } from '@/composables/useSignInExit'

const { t } = useI18n()
const route = useRoute()
const preferences = usePreferencesStore()
const mode = useModeToggle()
const { reduceMotion } = useInterfaceMotion()
const panel = useTemplateRef<HTMLDivElement>('panel')
const version = __APP_VERSION__

watch(
    [panel, () => route.path, reduceMotion],
    ([element, path, reduced], previous, onCleanup) => {
        if (
            !element ||
            reduced ||
            (element === previous[0] && path === previous[1])
        )
            return
        const entry = animate(
            element,
            { opacity: [0, 1], y: [24, 0] },
            { duration: 0.32, ease: 'easeOut' },
        )
        onCleanup(() => {
            entry.complete()
        })
    },
    { flush: 'post' },
)

function chooseLocale(locale: Locale) {
    preferences.locale = locale
}

onBeforeUnmount(() => {
    enteringConsole.value = false
})
</script>

<template>
    <div class="auth-shell" :class="{ 'is-entering': enteringConsole }">
        <header class="auth-top">
            <div class="auth-brand">
                <img src="/favicon.svg" width="28" height="28" alt="" />
                <strong>{{ t('common.app.name') }}</strong>
            </div>
            <div class="auth-tools">
                <div
                    class="auth-languages"
                    role="group"
                    :aria-label="t('common.shell.language')"
                >
                    <button
                        v-for="locale in supportedLocales"
                        :key="locale"
                        type="button"
                        :lang="locale"
                        :aria-label="messages[locale].common.language.self"
                        :aria-pressed="currentLocale === locale"
                        @click="chooseLocale(locale)"
                    >
                        <motion.span
                            v-if="currentLocale === locale"
                            class="auth-language-selection"
                            :layout-id="
                                reduceMotion
                                    ? undefined
                                    : 'auth-language-selection'
                            "
                            :transition="{ duration: 0.22, ease: 'easeOut' }"
                            aria-hidden="true"
                        />
                        <span class="auth-language-label">{{
                            messages[locale].common.language.short
                        }}</span>
                    </button>
                </div>
                <button
                    type="button"
                    class="icon-button auth-mode"
                    :aria-label="mode.label.value"
                    :title="mode.label.value"
                    @click="mode.toggle"
                >
                    <component :is="mode.icon.value" aria-hidden="true" />
                </button>
            </div>
        </header>
        <main id="main-content" class="auth-main" tabindex="-1">
            <div ref="panel" class="auth-panel">
                <div class="auth-access">
                    <div class="auth-card" :inert="enteringConsole">
                        <slot />
                    </div>
                    <p class="auth-entering" role="status">
                        {{ enteringConsole ? t('auth.layout.entering') : '' }}
                    </p>
                </div>
                <div class="auth-art" aria-hidden="true">
                    <div class="auth-art-mark">
                        <img src="/favicon.svg" width="48" height="48" alt="" />
                    </div>
                </div>
            </div>
        </main>
        <footer class="auth-bottom">
            <p>{{ t('auth.layout.footer') }}</p>
            <p>{{ t('common.shell.version', { version }) }}</p>
        </footer>
    </div>
</template>
