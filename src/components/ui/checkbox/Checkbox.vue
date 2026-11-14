<script setup lang="ts">
import { Check } from '@lucide/vue'
import { CheckboxIndicator, CheckboxRoot } from 'reka-ui'

defineProps<{ disabled?: boolean }>()
const model = defineModel<boolean>({ default: false })
</script>

<template>
  <CheckboxRoot
    v-model="model"
    :disabled="disabled"
    class="grid size-5 flex-none cursor-pointer place-items-center rounded-md border-[1.5px] border-line2 bg-s0 data-[state=checked]:border-act data-[state=checked]:bg-act"
  >
    <CheckboxIndicator><Check class="size-3.5 stroke-[3] text-white" /></CheckboxIndicator>
  </CheckboxRoot>
</template>
