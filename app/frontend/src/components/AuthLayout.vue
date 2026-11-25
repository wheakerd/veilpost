<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import LabGlobe from '@/components/LabGlobe.vue'
import { usePreferencesStore } from '@/stores/preferences'
import { useModeToggle } from '@/composables/useModeToggle'
import { currentLocale, messages, supportedLocales, type Locale } from '@/i18n'
import { formatClock } from '@/i18n/format'
import { enteringConsole } from '@/composables/useSignInExit'

const { t } = useI18n()
const route = useRoute()
const preferences = usePreferencesStore()
const mode = useModeToggle()
const version = __APP_VERSION__

function chooseLocale(locale: Locale) {
    preferences.locale = locale
}

const now = ref(Date.now())
let clock: ReturnType<typeof setInterval> | undefined
onMounted(() => {
    clock = setInterval(() => {
        now.value = Date.now()
    }, 1000)
})
onBeforeUnmount(() => {
    clearInterval(clock)
    enteringConsole.value = false
})
</script>

<template>
    <div class="auth-shell" :class="{ 'is-entering': enteringConsole }">
        <LabGlobe />
        <span class="frame-corner top-left" aria-hidden="true"></span>
        <span class="frame-corner top-right" aria-hidden="true"></span>
        <span class="frame-corner bottom-left" aria-hidden="true"></span>
        <span class="frame-corner bottom-right" aria-hidden="true"></span>
        <header class="auth-top">
            <div class="auth-brand">
                <img src="/favicon.svg" width="28" height="28" alt="" />
                <strong>{{ t('common.app.name') }}</strong>
                <p class="readout">
                    {{ t('common.shell.version', { version }) }} ·
                    {{ t('auth.layout.meta') }} ·
                    <time>{{ formatClock(now) }}</time>
                </p>
            </div>
            <div class="auth-tools">
                <div
                    class="segmented"
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
                        {{ messages[locale].common.language.short }}
                    </button>
                </div>
                <button
                    type="button"
                    class="icon-button framed"
                    :aria-label="mode.label.value"
                    :title="mode.label.value"
                    @click="mode.toggle"
                >
                    <component :is="mode.icon.value" aria-hidden="true" />
                </button>
            </div>
        </header>
        <main id="main-content" class="auth-main" tabindex="-1">
            <div class="auth-card" :inert="enteringConsole">
                <span class="hazard-strip" aria-hidden="true"></span>
                <slot />
            </div>
            <p class="auth-entering readout" role="status">
                {{ enteringConsole ? t('auth.layout.entering') : '' }}
            </p>
        </main>
        <footer class="auth-bottom">
            <span class="readout"
                ><span class="live-dot" aria-hidden="true"></span>
                {{
                    route.name === 'verify'
                        ? t('auth.layout.statusVerify')
                        : t('auth.layout.status')
                }}</span
            >
            <span class="readout">{{ t('auth.layout.footer') }}</span>
        </footer>
    </div>
</template>
