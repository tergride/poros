<script setup lang="ts">
import { ContextMenuContent, ContextMenuPortal, ContextMenuRoot, ContextMenuTrigger } from 'reka-ui'

// The menu of the right button, drawn as the menus of the mockups; the trigger slot is what it opens over.
defineProps<{ minWidth?: string }>()
defineEmits<{ open: [] }>()
</script>

<template>
  <ContextMenuRoot :modal="false" @update:open="(open) => open && $emit('open')">
    <ContextMenuTrigger as-child><slot name="trigger" /></ContextMenuTrigger>
    <ContextMenuPortal>
      <ContextMenuContent
        :style="{ minWidth: minWidth ?? '260px' }"
        class="z-50 max-h-[calc(100vh-24px)] overflow-auto rounded-[10px] border border-line2 bg-s3 p-1.5 shadow-[0_18px_44px_rgba(0,0,0,.55)] outline-none"
      >
        <slot />
      </ContextMenuContent>
    </ContextMenuPortal>
  </ContextMenuRoot>
</template>
