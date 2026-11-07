<script setup lang="ts">
import { DropdownMenuItem } from 'reka-ui'

defineProps<{ danger?: boolean; shortcut?: string }>()
defineEmits<{ select: [] }>()
</script>

<template>
  <DropdownMenuItem
    :class="[
      'flex min-h-[34px] cursor-pointer items-center gap-2.5 rounded-md px-2.5 text-[13px] outline-none data-[highlighted]:bg-s5 [&>svg]:size-4',
      danger ? 'text-bad [&>svg]:text-bad' : 'text-t1 [&>svg]:text-t2',
    ]"
    @select="$emit('select')"
  >
    <slot />
    <span v-if="shortcut" class="ml-auto text-[11.5px] text-t3">{{ shortcut }}</span>
  </DropdownMenuItem>
</template>
