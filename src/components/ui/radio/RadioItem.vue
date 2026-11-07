<script setup lang="ts">
import { RadioGroupItem } from 'reka-ui'

defineProps<{ value: string }>()
</script>

<template>
  <RadioGroupItem
    :value="value"
    class="size-[18px] flex-none cursor-pointer rounded-full border-[1.5px] border-line2 outline-none data-[state=checked]:border-[5px] data-[state=checked]:border-blue"
  />
</template>
