<script setup lang="ts">
import { cn } from 'cn'
import { TabsList } from 'reka-ui'
import type { HTMLAttributes } from 'vue'

const props = defineProps<{ class?: HTMLAttributes['class'] }>()
</script>

<template>
  <TabsList :class="cn('flex flex-none gap-0.5 border-b border-line px-3.5', props.class)"><slot /></TabsList>
</template>
