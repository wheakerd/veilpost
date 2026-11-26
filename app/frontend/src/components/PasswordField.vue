<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Eye from '~icons/lucide/eye'
import EyeOff from '~icons/lucide/eye-off'

defineProps<{
    id: string
    label?: string
    disabled?: boolean
    placeholder?: string
    autocomplete?: 'current-password' | 'new-password'
}>()
const model = defineModel<string>({ required: true })
const visible = ref(false)
const capsLock = ref(false)
const { t } = useI18n()
// The label row reports Caps Lock while the field is in use.
function readCapsLock(event: Event) {
    if (event instanceof KeyboardEvent || event instanceof MouseEvent)
        capsLock.value = event.getModifierState('CapsLock')
}
</script>

<template>
    <div class="form-field">
        <div class="field-label-row">
            <label :for="id"
                ><span>{{ label ?? t('common.password.label') }}</span></label
            >
            <span
                v-if="capsLock"
                :id="`${id}-caps`"
                class="field-status"
                role="status"
                >{{ t('common.password.capsLock') }}</span
            >
        </div>
        <ElInput
            :id="id"
            v-model="model"
            class="password-field"
            :name="id"
            :type="visible ? 'text' : 'password'"
            :autocomplete="autocomplete ?? 'current-password'"
            :placeholder="placeholder"
            required
            maxlength="256"
            :disabled="disabled"
            :aria-describedby="capsLock ? `${id}-caps` : undefined"
            @keydown="readCapsLock"
            @keyup="readCapsLock"
            @mousedown="readCapsLock"
            @blur="capsLock = false"
        >
            <template v-if="$slots.prefix" #prefix
                ><slot name="prefix"
            /></template>
            <template #suffix>
                <button
                    type="button"
                    class="password-toggle"
                    :aria-label="
                        visible
                            ? t('common.password.hideLabel')
                            : t('common.password.showLabel')
                    "
                    :aria-pressed="visible"
                    :aria-controls="id"
                    :disabled="disabled"
                    @click="visible = !visible"
                >
                    <component
                        :is="visible ? EyeOff : Eye"
                        aria-hidden="true"
                    />
                </button>
            </template>
        </ElInput>
    </div>
</template>
