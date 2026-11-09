<script setup lang="ts">
import { SwitchRoot, SwitchThumb } from 'reka-ui'
import { inject, useAttrs } from 'vue'

import { rowLabel } from './label'

// sm is the switch of a row of a list, such as a rule of a preset.
defineProps<{ disabled?: boolean; size?: 'sm' }>()
const model = defineModel<boolean>({ default: false })
// In a settings row the title names the switch, unless the caller gives a name of its own.
const label = inject(rowLabel, undefined)
const attrs = useAttrs()
</script>

<template>
  <SwitchRoot
    v-model="model"
    :disabled="disabled"
    :aria-labelledby="attrs['aria-label'] || attrs['aria-labelledby'] ? undefined : label"
    :class="[
      'relative flex-none cursor-pointer rounded-full bg-s5 transition-colors outline-none data-[state=checked]:bg-act disabled:cursor-not-allowed disabled:opacity-45',
      size === 'sm' ? 'h-[18px] w-[30px]' : 'h-[22px] w-[38px]',
    ]"
  >
    <SwitchThumb
      :class="[
        'block translate-x-[3px] rounded-full bg-t2 transition-transform data-[state=checked]:bg-white',
        size === 'sm' ? 'size-3 data-[state=checked]:translate-x-[15px]' : 'size-4 data-[state=checked]:translate-x-[19px]',
      ]"
    />
  </SwitchRoot>
</template>
