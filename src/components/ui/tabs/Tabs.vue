<script setup lang="ts">
import { TabsRoot } from 'reka-ui'

// The root only holds the choice: its list and panels can sit apart, such as in the head and the body of a dialog.
const model = defineModel<string>({ required: true })
</script>

<template>
  <TabsRoot v-model="model"><slot /></TabsRoot>
</template>
