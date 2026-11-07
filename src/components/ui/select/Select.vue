<script setup lang="ts" generic="T extends string | number">
import { cn } from 'cn'
import type { HTMLAttributes } from 'vue'

// A native select, as in the mockups: Windows draws the list, so it fits the system.
const props = defineProps<{ options: { value: T; label: string }[]; class?: HTMLAttributes['class'] }>()
const model = defineModel<T>({ required: true })
</script>

<template>
  <select
    v-model="model"
    :class="
      cn(
        'h-9 w-[220px] min-w-0 cursor-pointer rounded-lg border border-line2 bg-s0 pr-1.5 pl-[11px] text-[13px] text-t1 outline-none focus:border-blue [&>option]:bg-s2',
        props.class,
      )
    "
  >
    <option v-for="option in options" :key="String(option.value)" :value="option.value">{{ option.label }}</option>
  </select>
</template>
