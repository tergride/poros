<script setup lang="ts">
import { cn } from 'cn'
import type { HTMLAttributes } from 'vue'

const props = defineProps<{ mono?: boolean; class?: HTMLAttributes['class'] }>()
const model = defineModel<string>({ default: '' })
</script>

<template>
  <input
    v-model="model"
    :class="
      cn(
        'h-9 w-full min-w-0 rounded-lg border border-line2 bg-s0 px-[11px] text-[13px] text-t1 outline-none placeholder:text-t3 focus:border-blue focus:shadow-[0_0_0_3px_color-mix(in_srgb,var(--blue)_20%,transparent)]',
        mono && 'font-mono text-[12.5px]',
        props.class,
      )
    "
  />
</template>
