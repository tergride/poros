<script setup lang="ts">
import { DropdownMenuLabel, DropdownMenuSeparator } from 'reka-ui'

// A separator, or with text a label over the items after it.
defineProps<{ label?: string }>()
</script>

<template>
  <DropdownMenuLabel v-if="label" class="px-2.5 pt-1.5 pb-1 text-[11.5px] text-t3">{{ label }}</DropdownMenuLabel>
  <DropdownMenuSeparator v-else class="my-[5px] h-px bg-line" />
</template>
