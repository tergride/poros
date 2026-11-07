<script setup lang="ts">
import { DropdownMenuContent, DropdownMenuPortal, DropdownMenuRoot, DropdownMenuTrigger } from 'reka-ui'

// A menu of the mockups opened from a button; the trigger slot is the button.
defineProps<{ align?: 'start' | 'end'; minWidth?: string }>()
</script>

<template>
  <DropdownMenuRoot :modal="false">
    <DropdownMenuTrigger as-child><slot name="trigger" /></DropdownMenuTrigger>
    <DropdownMenuPortal>
      <DropdownMenuContent
        :align="align ?? 'start'"
        :side-offset="6"
        :style="{ minWidth: minWidth ?? '276px' }"
        class="z-50 rounded-[10px] border border-line2 bg-s3 p-1.5 shadow-[0_18px_44px_rgba(0,0,0,.55)] outline-none"
      >
        <slot />
      </DropdownMenuContent>
    </DropdownMenuPortal>
  </DropdownMenuRoot>
</template>
