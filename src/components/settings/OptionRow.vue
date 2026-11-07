<script setup lang="ts">
import { ShieldAlert } from '@lucide/vue'

// One line of a settings card: what it is, a hint, and its control on the right.
defineProps<{ title: string; hint?: string; shield?: boolean }>()
</script>

<template>
  <div class="flex items-center gap-3.5 border-t border-line py-2.5 first:border-t-0">
    <div class="min-w-0 flex-1">
      <strong class="flex items-center gap-1.5 font-medium">{{ title }}<ShieldAlert v-if="shield" class="size-3.5 text-warn" /></strong>
      <span v-if="hint" class="text-xs text-t3">{{ hint }}</span>
    </div>
    <div class="flex flex-none items-center gap-1.5"><slot /></div>
  </div>
</template>
