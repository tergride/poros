<script setup lang="ts">
import { SwitchRoot, SwitchThumb } from 'reka-ui'
import { inject, useAttrs } from 'vue'

import { rowLabel } from './label'

defineProps<{ disabled?: boolean }>()
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
    class="relative h-[22px] w-[38px] flex-none cursor-pointer rounded-full bg-s5 transition-colors outline-none data-[state=checked]:bg-act disabled:cursor-not-allowed disabled:opacity-45"
  >
    <SwitchThumb
      class="block size-4 translate-x-[3px] rounded-full bg-t2 transition-transform data-[state=checked]:translate-x-[19px] data-[state=checked]:bg-white"
    />
  </SwitchRoot>
</template>
