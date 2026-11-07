<script setup lang="ts">
import { RadioGroupRoot } from 'reka-ui'

const model = defineModel<string>({ required: true })
</script>

<template>
  <RadioGroupRoot v-model="model"><slot /></RadioGroupRoot>
</template>
