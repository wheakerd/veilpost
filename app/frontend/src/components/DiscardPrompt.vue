<script setup lang="ts">
import { onMounted, ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'

// Replaces a form's actions while it asks whether to drop unsaved edits.
const emit = defineEmits<{ keep: []; discard: [] }>()
const { t } = useI18n()
const id = useId()
const keepButton = ref<{ $el: HTMLElement }>()
onMounted(() => {
    keepButton.value?.$el.focus()
})
</script>

<template>
    <div
        class="discard-prompt"
        role="alertdialog"
        :aria-labelledby="id"
        @keydown.esc.stop.prevent="emit('keep')"
    >
        <p :id="id">{{ t('common.discard.message') }}</p>
        <div class="form-actions">
            <ElButton type="danger" @click="emit('discard')">{{
                t('common.discard.confirm')
            }}</ElButton><ElButton ref="keepButton" @click="emit('keep')">{{
                t('common.discard.keep')
            }}</ElButton>
        </div>
    </div>
</template>
