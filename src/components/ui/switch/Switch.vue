<script setup lang="ts">
import { SwitchRoot, SwitchThumb } from 'reka-ui'

defineProps<{ disabled?: boolean }>()
const model = defineModel<boolean>({ default: false })
</script>

<template>
  <SwitchRoot
    v-model="model"
    :disabled="disabled"
    class="relative h-[22px] w-[38px] flex-none cursor-pointer rounded-full bg-s5 transition-colors outline-none data-[state=checked]:bg-act disabled:cursor-not-allowed disabled:opacity-45"
  >
    <SwitchThumb
      class="block size-4 translate-x-[3px] rounded-full bg-t2 transition-transform data-[state=checked]:translate-x-[19px] data-[state=checked]:bg-white"
    />
  </SwitchRoot>
</template>
