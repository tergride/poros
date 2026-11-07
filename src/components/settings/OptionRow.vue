<script setup lang="ts">
import { ShieldAlert } from '@lucide/vue'
import { provide, useId } from 'vue'

import { rowLabel } from '@/components/ui/switch'

// One line of a settings card: what it is, a hint, a note on how it works, and its control on the right. The
// control is named by the title, so that a screen reader says what it switches.
defineProps<{ title: string; hint?: string; note?: string; shield?: boolean }>()
const id = useId()
provide(rowLabel, id)
</script>

<template>
  <div class="flex items-center gap-3.5 border-t border-line py-2.5 first:border-t-0">
    <div class="min-w-0 flex-1">
      <strong :id="id" class="flex items-center gap-1.5 font-medium">{{ title }}<ShieldAlert v-if="shield" class="size-3.5 text-warn" /></strong>
      <span v-if="hint" class="text-xs text-t3">{{ hint }}</span>
      <p v-if="note" class="mt-1 max-w-[560px] text-[11.5px] text-t3/80" data-testid="option-note">{{ note }}</p>
    </div>
    <div class="flex flex-none items-center gap-1.5"><slot /></div>
  </div>
</template>
