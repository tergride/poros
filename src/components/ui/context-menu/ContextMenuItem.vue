<script setup lang="ts">
import { ContextMenuItem } from 'reka-ui'

defineProps<{ danger?: boolean; disabled?: boolean }>()
defineEmits<{ select: [] }>()
</script>

<template>
  <ContextMenuItem
    :disabled="disabled"
    :class="[
      'flex min-h-[32px] cursor-pointer items-center gap-2.5 rounded-md px-2.5 text-[13px] outline-none data-[disabled]:cursor-default data-[disabled]:opacity-45 data-[highlighted]:bg-s5 [&>svg]:size-4',
      danger ? 'text-bad [&>svg]:text-bad' : 'text-t1 [&>svg]:text-t2',
    ]"
    @select="$emit('select')"
  >
    <slot />
  </ContextMenuItem>
</template>
