<script setup lang="ts">
import { Info } from '@lucide/vue'

import { vTooltip } from '@/components/ui'

// The text of a setting laid out other than as an OptionRow: its title, the short hint under it, and a note on how it
// works that shows while the pointer rests on the text or the keyboard focuses it, as an OptionRow's does.
defineProps<{ title: string; hint?: string; note: string }>()
</script>

<template>
  <span v-tooltip="note" class="cursor-help rounded-md" tabindex="0" data-testid="option-text">
    <span class="inline-flex items-center gap-1.5">{{ title }}<Info class="size-3.5 text-t3" /></span>
    <template v-if="hint"><br /><span class="text-xs text-t3">{{ hint }}</span></template>
  </span>
</template>
