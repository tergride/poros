<script setup lang="ts">
import { Info, ShieldAlert } from '@lucide/vue'
import { provide, useId } from 'vue'

import { rowLabel } from '@/components/ui/switch'
import { Tooltip } from '@/components/ui'

// One line of a settings card: what it is, a hint, and its control on the right. A note on how it works shows while
// the pointer rests on the line's text or the keyboard focuses it, so that a long explanation does not crowd the
// card. The control is named by the title, so that a screen reader says what it switches. In a narrow window a wide
// control goes under the text rather than pressing it into a column of single words.
defineProps<{ title: string; hint?: string; note?: string; shield?: boolean }>()
const id = useId()
provide(rowLabel, id)
</script>

<template>
  <div class="flex flex-wrap items-center gap-3.5 border-t border-line py-2.5 first:border-t-0">
    <Tooltip :text="note ?? ''">
      <div class="min-w-[180px] flex-1" :class="note && 'cursor-help rounded-md'" :tabindex="note ? 0 : undefined" data-testid="option-text">
        <strong :id="id" class="flex items-center gap-1.5 font-medium"
          >{{ title }}<Info v-if="note" class="size-3.5 text-t3" /><ShieldAlert v-if="shield" class="size-3.5 text-warn"
        /></strong>
        <span v-if="hint" class="text-xs text-t3">{{ hint }}</span>
      </div>
    </Tooltip>
    <div class="flex flex-none items-center gap-1.5"><slot /></div>
  </div>
</template>
