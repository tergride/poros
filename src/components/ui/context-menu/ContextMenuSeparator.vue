<script setup lang="ts">
import { ContextMenuLabel, ContextMenuSeparator } from 'reka-ui'

// A separator, or with text a label over the items after it.
defineProps<{ label?: string }>()
</script>

<template>
  <ContextMenuLabel v-if="label" class="px-2.5 pt-1.5 pb-1 text-[11.5px] text-t3">{{ label }}</ContextMenuLabel>
  <ContextMenuSeparator v-else class="my-[5px] h-px bg-line" />
</template>
