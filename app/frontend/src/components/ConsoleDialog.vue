<script setup lang="ts">
import { useTemplateRef, watch } from 'vue'

const props = defineProps<{
    title: string
    heading?: string
    beforeClose?: (done: () => void) => void
}>()
const open = defineModel<boolean>({ required: true })
const emit = defineEmits<{ close: [] }>()
const content = useTemplateRef<HTMLDivElement>('content')

watch(
    () => props.heading,
    () => {
        if (content.value) content.value.scrollTop = 0
    },
)
</script>

<template>
    <ElDialog
        v-model="open"
        class="console-dialog"
        :class="{ 'has-sidebar': $slots.sidebar }"
        :title="title"
        :width="
            $slots.sidebar
                ? 'min(64rem, calc(100vw - 1rem))'
                : 'min(32rem, calc(100vw - 1rem))'
        "
        :before-close="beforeClose"
        append-to-body
        align-center
        destroy-on-close
        @close="emit('close')"
    >
        <template #header>
            <span class="visually-hidden">{{ title }}</span>
        </template>
        <div class="console-dialog-layout">
            <aside v-if="$slots.sidebar" class="console-dialog-sidebar">
                <slot name="sidebar" />
            </aside>
            <div class="console-dialog-main">
                <h2 class="console-dialog-heading">{{ heading ?? title }}</h2>
                <div ref="content" class="console-dialog-content">
                    <slot />
                </div>
            </div>
        </div>
    </ElDialog>
</template>

<style lang="scss">
:root .console-dialog.el-dialog {
    --el-dialog-padding-primary: 0;
    overflow: hidden;
    border-radius: var(--app-radius-card);
    background-color: color-mix(
        in srgb,
        var(--app-raised) 62%,
        var(--app-sheet-fill-strong)
    );

    .el-dialog__header {
        position: absolute;
        inset-block-start: 0;
        inset-inline-end: 0;
        z-index: 2;
        padding: 0;
        margin: 0;
    }

    .el-dialog__headerbtn {
        inset-block-start: 1.25rem;
        inset-inline-end: 1.25rem;
        display: grid;
        place-items: center;
        width: 2rem;
        height: 2rem;
        border: 1px solid var(--app-border-strong);
        border-radius: var(--app-radius-capsule);
        background: var(--app-raised);
        font-size: 1rem;

        .el-dialog__close {
            color: var(--app-text);
        }

        &:hover {
            background: var(--app-primary-soft);
        }
    }

    .el-dialog__body {
        padding: 0;
        color: var(--app-text);
    }
}

.console-dialog-layout {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    max-block-size: calc(100dvh - 1rem);
}

.console-dialog.has-sidebar .console-dialog-layout {
    grid-template-columns: minmax(0, 16rem) minmax(0, 1fr);
    block-size: min(42.5rem, calc(100dvh - 1rem));
}

.console-dialog-sidebar,
.console-dialog-main,
.console-dialog-content {
    min-inline-size: 0;
    min-block-size: 0;
}

.console-dialog-sidebar {
    overflow: auto;
    overscroll-behavior: contain;
    border-inline-end: 1px solid var(--app-border);
    background: color-mix(in srgb, var(--app-primary) 14%, var(--app-surface));
}

.console-dialog-main {
    display: flex;
    flex-direction: column;
}

.console-dialog-heading {
    flex-shrink: 0;
    margin: 0;
    padding: 1.5rem 4rem 1.25rem 1.5rem;
    color: var(--app-text);
    font-size: 1.125rem;
    font-weight: 600;
    line-height: 1.35;
    overflow-wrap: anywhere;
}

.console-dialog-content {
    flex: 1;
    padding: 0 1.5rem 1.5rem;
    overflow: auto;
    overscroll-behavior: contain;
}

@media (max-width: 47.5rem) {
    :root .console-dialog.el-dialog .el-dialog__headerbtn {
        inset-block-start: 0.75rem;
        inset-inline-end: 0.75rem;
    }

    .console-dialog.has-sidebar .console-dialog-layout {
        grid-template-columns: minmax(0, 1fr);
        grid-template-rows: auto minmax(0, 1fr);
        block-size: min(40rem, calc(100dvh - 1rem));
    }

    .console-dialog-sidebar {
        max-block-size: 50dvh;
        border-inline-end: 0;
        border-block-end: 1px solid var(--app-border);
    }

    .console-dialog-heading {
        padding: 1rem 3.5rem 0.9rem 1rem;
    }

    .console-dialog-content {
        padding: 0 1rem 1rem;
    }
}
</style>
