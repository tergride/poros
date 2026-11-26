<script setup lang="ts">
import { Info, TriangleAlert } from '@lucide/vue'

// A line above the content with what went wrong, or what can be done now, and its buttons. Where the text would get
// too narrow beside them, the buttons go under it.
defineProps<{ tone?: 'warn' | 'info' }>()
</script>

<template>
  <div
    :role="tone === 'info' ? 'status' : 'alert'"
    :class="[
      'flex flex-wrap items-center gap-2.5 rounded-[10px] border px-3 py-2.5 text-[13px]',
      tone === 'info' ? 'border-line2 bg-s3' : 'border-warn/35 bg-warn/10',
    ]"
  >
    <slot name="icon">
      <Info v-if="tone === 'info'" class="size-4 flex-none text-blue" />
      <TriangleAlert v-else class="size-4 flex-none text-warn" />
    </slot>
    <span class="min-w-0 flex-[1_1_200px]"><slot /></span>
    <div v-if="$slots.actions" class="ml-auto flex items-center gap-2.5"><slot name="actions" /></div>
  </div>
</template>
