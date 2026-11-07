<script setup lang="ts">
import { cn } from 'cn'
import { TabsContent } from 'reka-ui'
import type { HTMLAttributes } from 'vue'

const props = defineProps<{ value: string; class?: HTMLAttributes['class'] }>()
</script>

<template>
  <TabsContent :value="props.value" :class="cn('outline-none', props.class)"><slot /></TabsContent>
</template>
