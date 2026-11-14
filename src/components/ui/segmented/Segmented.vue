<script setup lang="ts">
import { RadioGroupItem, RadioGroupRoot } from 'reka-ui'

// A choice of a few values in one row, as the .seg of the mockups. Disabled here rather than by a disabled fieldset
// around it, which the group does not see: Tab would stop on the group itself, with nothing to show.
defineProps<{ options: { value: string; label: string }[]; ariaLabel?: string; disabled?: boolean }>()
const model = defineModel<string>({ required: true })
</script>

<template>
  <RadioGroupRoot
    v-model="model"
    orientation="horizontal"
    :disabled="disabled"
    :aria-label="ariaLabel"
    class="inline-flex gap-0.5 rounded-[10px] border border-line2 bg-s0 p-[3px]"
  >
    <RadioGroupItem
      v-for="option in options"
      :key="option.value"
      :value="option.value"
      :data-testid="`segment-${option.value}`"
      class="inline-flex h-7 cursor-pointer items-center justify-center rounded-[7px] px-3 text-[13px] font-medium text-t2 outline-none hover:text-t1 focus-visible:ring-2 focus-visible:ring-blue data-[state=checked]:bg-s5 data-[state=checked]:text-white"
    >
      {{ option.label }}
    </RadioGroupItem>
  </RadioGroupRoot>
</template>
