<script setup lang="ts">
import { cn } from 'cn'
import { DialogContent, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'
import type { HTMLAttributes } from 'vue'

// The dialogs of the mockups: a title row, a body that scrolls, and a footer of buttons.
const props = defineProps<{ title: string; class?: HTMLAttributes['class'] }>()
const open = defineModel<boolean>('open', { default: false })
</script>

<template>
  <DialogRoot v-model:open="open">
    <DialogPortal>
      <DialogOverlay class="fixed inset-0 z-40 bg-s0/75" />
      <DialogContent
        :aria-describedby="undefined"
        :class="
          cn(
            'fixed top-1/2 left-1/2 z-50 flex max-h-[calc(100%-48px)] w-[520px] max-w-[calc(100%-32px)] -translate-x-1/2 -translate-y-1/2 flex-col rounded-[14px] border border-line2 bg-s2 shadow-[0_24px_60px_rgba(0,0,0,.55)] outline-none',
            props.class,
          )
        "
      >
        <div class="flex items-center gap-2.5 px-5 pt-4 pb-3 [&>svg]:size-5">
          <slot name="icon" />
          <DialogTitle class="text-[17px] font-semibold">{{ title }}</DialogTitle>
          <div class="ml-auto flex items-center gap-2"><slot name="actions" /></div>
        </div>
        <div class="min-h-0 flex-1 overflow-auto px-5 pt-1 pb-[18px]"><slot /></div>
        <div v-if="$slots.footer" class="flex items-center gap-2 border-t border-line px-5 py-3 [&>.sp]:mr-auto">
          <slot name="footer" />
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
