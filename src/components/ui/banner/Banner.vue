<script setup lang="ts">
import { Info, TriangleAlert } from '@lucide/vue'

// A line above the content with what went wrong, or what can be done now, and its buttons.
defineProps<{ tone?: 'warn' | 'info' }>()
</script>

<template>
  <div
    :role="tone === 'info' ? 'status' : 'alert'"
    :class="[
      'flex items-center gap-2.5 rounded-[10px] border px-3 py-2.5 text-[13px]',
      tone === 'info' ? 'border-line2 bg-s3' : 'border-warn/35 bg-warn/10',
    ]"
  >
    <Info v-if="tone === 'info'" class="size-4 flex-none text-blue" />
    <TriangleAlert v-else class="size-4 flex-none text-warn" />
    <span class="min-w-0 flex-1"><slot /></span>
    <slot name="actions" />
  </div>
</template>
