<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Check from '~icons/lucide/check'
import { usePreferencesStore } from '@/stores/preferences'
import { useModeToggle } from '@/composables/useModeToggle'
import {
    glassDefaults,
    pageTransitions,
    palettes,
    sidebarWidth,
    visions,
    type ColorMode,
} from '@/theme/palettes'
import { browserLocale, isLocale, localeName, supportedLocales } from '@/i18n'
import { formatNumber } from '@/i18n/format'
import DisabledReason from '@/components/DisabledReason.vue'

// Shared by the preferences drawer and the preferences page; every change
// applies at once and is saved in this browser only.
const props = withDefaults(
    defineProps<{ idPrefix: string; sectionHeading?: 'h2' | 'h3' }>(),
    { sectionHeading: 'h2' },
)
const itemHeading = computed(() =>
    props.sectionHeading === 'h2' ? 'h3' : 'h4',
)
const { t } = useI18n()
const preferences = usePreferencesStore()
const id = (name: string) => `${props.idPrefix}-${name}`

// 'auto' follows the browser language and leaves storage untouched.
const language = computed({
    get: () => preferences.locale ?? 'auto',
    set: (value: string) => {
        preferences.locale = isLocale(value) ? value : undefined
    },
})
const paletteSchemes = ['light', 'dark'] as const
const { setMode } = useModeToggle()
let modePress: PointerEvent | undefined
const selectedMode = computed({
    get: () => preferences.mode,
    set: (value: ColorMode) => {
        setMode(value, modePress)
        modePress = undefined
    },
})
const modeOptions = computed(() =>
    ([...paletteSchemes, 'system'] as const).map((value) => ({
        value,
        label: t(`appearance.mode.${value}`),
    })),
)
const visionOptions = computed(() =>
    visions.map((value) => ({
        value,
        label: t(`appearance.vision.${value}`),
    })),
)
const widthText = computed(() =>
    preferences.sidebarWidth === undefined
        ? t('appearance.layout.sidebarWidth.auto', {
              width: formatNumber(sidebarWidth.default),
          })
        : t('appearance.layout.sidebarWidth.fixed', {
              width: formatNumber(preferences.sidebarWidth),
          }),
)

function resetGlass() {
    preferences.glassTint = glassDefaults.tint
    preferences.glassStrength = glassDefaults.strength
}

const switches = [
    'navMultiple',
    'tabBar',
    'progressBar',
    'pageCache',
    'reducedMotion',
] as const
</script>

<template>
    <div class="preferences">
        <section class="preference-section" :aria-labelledby="id('palette')">
            <component :is="sectionHeading" :id="id('palette')">
                {{ t('appearance.palette.title') }}
            </component>
            <p :id="id('palette-preview')" class="preference-hint">
                {{ t('appearance.palette.preview') }}
            </p>
            <div
                class="palette-chips"
                role="radiogroup"
                :aria-labelledby="id('palette')"
                :aria-describedby="id('palette-preview')"
            >
                <label
                    v-for="palette in palettes"
                    :key="palette"
                    class="palette-chip"
                    :class="{ selected: preferences.palette === palette }"
                    :data-palette="palette"
                >
                    <input
                        v-model="preferences.palette"
                        class="visually-hidden"
                        type="radio"
                        :name="id('palette')"
                        :value="palette"
                    />
                    <span class="palette-heading">
                        <span class="palette-name">{{
                            t(`appearance.palettes.${palette}.name`)
                        }}</span>
                        <span class="palette-indicator" aria-hidden="true">
                            <Check v-if="preferences.palette === palette" />
                        </span>
                    </span>
                    <span class="palette-previews" aria-hidden="true">
                        <span
                            v-for="scheme in paletteSchemes"
                            :key="scheme"
                            class="palette-preview"
                            :class="`palette-preview--${scheme}`"
                        >
                            <span class="palette-primary"></span>
                            <span class="palette-secondary"></span>
                        </span>
                    </span>
                </label>
            </div>
        </section>

        <section class="preference-section" :aria-labelledby="id('mode')">
            <component :is="sectionHeading" :id="id('mode')">
                {{ t('appearance.mode.title') }}
            </component>
            <ElSegmented
                v-model="selectedMode"
                block
                :options="modeOptions"
                :aria-label="t('appearance.mode.title')"
                @pointerdown="modePress = $event"
                @keydown="modePress = undefined"
            />
        </section>

        <section class="preference-section" :aria-labelledby="id('vision')">
            <component :is="sectionHeading" :id="id('vision')">
                {{ t('appearance.vision.title') }}
            </component>
            <ElSegmented
                v-model="preferences.vision"
                block
                :options="visionOptions"
                :aria-label="t('appearance.vision.title')"
            />
            <p class="preference-hint">
                {{ t(`appearance.vision.hints.${preferences.vision}`) }}
            </p>
        </section>

        <section class="preference-section" :aria-labelledby="id('glass')">
            <component :is="sectionHeading" :id="id('glass')">
                {{ t('appearance.glass.title') }}
            </component>
            <div class="glass-row">
                <span class="glass-label">{{
                    t('appearance.glass.tint')
                }}</span>
                <span class="glass-end">{{
                    t('appearance.glass.tintMin')
                }}</span>
                <ElSlider
                    v-model="preferences.glassTint"
                    :show-tooltip="false"
                    :aria-label="t('appearance.glass.tint')"
                />
                <span class="glass-end">{{
                    t('appearance.glass.tintMax')
                }}</span>
            </div>
            <div class="glass-row">
                <span class="glass-label">{{
                    t('appearance.glass.strength')
                }}</span>
                <span class="glass-end">{{
                    t('appearance.glass.strengthMin')
                }}</span>
                <ElSlider
                    v-model="preferences.glassStrength"
                    :show-tooltip="false"
                    :aria-label="t('appearance.glass.strength')"
                />
                <span class="glass-end">{{
                    t('appearance.glass.strengthMax')
                }}</span>
            </div>
            <div class="preference-hint-row">
                <p class="preference-hint">
                    {{ t('appearance.glass.description') }}
                </p>
                <ElButton
                    size="small"
                    :disabled="
                        preferences.glassTint === glassDefaults.tint &&
                        preferences.glassStrength === glassDefaults.strength
                    "
                    @click="resetGlass"
                    >{{ t('appearance.glass.reset') }}</ElButton
                >
            </div>
        </section>

        <section class="preference-section" :aria-labelledby="id('transition')">
            <component :is="sectionHeading" :id="id('transition')">
                {{ t('appearance.transition.title') }}
            </component>
            <div
                class="transition-grid"
                role="radiogroup"
                :aria-labelledby="id('transition')"
            >
                <label
                    v-for="name in pageTransitions"
                    :key="name"
                    class="transition-option"
                    :class="[
                        `preview-${name}`,
                        { selected: preferences.pageTransition === name },
                    ]"
                >
                    <input
                        v-model="preferences.pageTransition"
                        class="visually-hidden"
                        type="radio"
                        :name="id('transition')"
                        :value="name"
                    />
                    <span class="transition-preview" aria-hidden="true">
                        <span class="preview-page">
                            <span class="preview-line wide"></span>
                            <span class="preview-block"></span>
                            <span class="preview-line"></span>
                        </span>
                    </span>
                    <span class="transition-name"
                        ><span class="transition-radio" aria-hidden="true"
                            ><Check
                        /></span>
                        {{ t(`appearance.transition.${name}`) }}</span
                    >
                </label>
            </div>
            <p class="preference-hint">
                {{ t('appearance.transition.description') }}
            </p>
        </section>

        <section
            class="preference-section preference-list"
            :aria-labelledby="id('layout')"
        >
            <component :is="sectionHeading" :id="id('layout')">
                {{ t('appearance.layout.title') }}
            </component>
            <div class="preference-item">
                <div>
                    <component :is="itemHeading">
                        {{ t('appearance.layout.sidebarWidth.title') }}
                    </component>
                    <p>{{ widthText }}</p>
                </div>
                <DisabledReason
                    :reason="
                        preferences.sidebarWidth === undefined &&
                        t('appearance.layout.sidebarWidth.atDefault')
                    "
                >
                    <ElButton
                        size="small"
                        :disabled="preferences.sidebarWidth === undefined"
                        @click="preferences.sidebarWidth = undefined"
                        >{{
                            t('appearance.layout.sidebarWidth.reset')
                        }}</ElButton
                    >
                </DisabledReason>
            </div>
            <div v-for="name in switches" :key="name" class="preference-item">
                <div>
                    <component :is="itemHeading">
                        {{ t(`appearance.layout.${name}.title`) }}
                    </component>
                    <p>{{ t(`appearance.layout.${name}.description`) }}</p>
                </div>
                <ElSwitch
                    v-model="preferences[name]"
                    :aria-label="t(`appearance.layout.${name}.title`)"
                />
            </div>
            <div class="preference-item wraps">
                <div>
                    <component :is="itemHeading">
                        {{ t('appearance.language.title') }}
                    </component>
                    <p>{{ t('appearance.language.description') }}</p>
                </div>
                <ElSelect
                    v-model="language"
                    class="language-select"
                    :aria-label="t('appearance.language.title')"
                >
                    <ElOption
                        value="auto"
                        :label="
                            t('appearance.language.auto', {
                                language: localeName(browserLocale),
                            })
                        "
                    />
                    <ElOption
                        v-for="locale in supportedLocales"
                        :key="locale"
                        :value="locale"
                        :label="localeName(locale)"
                        :lang="locale"
                    />
                </ElSelect>
            </div>
        </section>

        <footer class="preferences-footer">
            <span>{{ t('appearance.footer.note') }}</span>
            <ElButton @click="preferences.$reset()">{{
                t('appearance.footer.reset')
            }}</ElButton>
        </footer>
    </div>
</template>
