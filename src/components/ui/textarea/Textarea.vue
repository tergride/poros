<script setup lang="ts">
import { cn } from 'cn'
import type { HTMLAttributes } from 'vue'

const props = defineProps<{ mono?: boolean; class?: HTMLAttributes['class'] }>()
const model = defineModel<string>({ default: '' })
</script>

<template>
  <textarea
    v-model="model"
    :class="
      cn(
        'min-h-[76px] w-full rounded-lg border border-line2 bg-s0 px-[11px] py-[9px] text-[13px] text-t1 outline-none select-text placeholder:text-t3 focus:border-blue focus:shadow-[0_0_0_3px_color-mix(in_srgb,var(--blue)_20%,transparent)]',
        mono && 'font-mono text-[12.5px]',
        props.class,
      )
    "
  />
</template>
