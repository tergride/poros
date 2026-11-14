<script setup lang="ts">
import { computed, useId } from 'vue'

import type { Traffic } from '@/platform/core-types'

// The speed of the last minute as two lines: download on the brand gradient, upload under it. The highest second
// fills the height, so that a quiet minute still shows its shape.
const props = defineProps<{ samples: Traffic[]; slots?: number }>()
const gradient = useId()
const width = 600
const height = 100
const slots = computed(() => props.slots ?? 60)
const top = computed(() => Math.max(1024, ...props.samples.map((s) => Math.max(s.download, s.upload))))

function line(value: (s: Traffic) => number): string {
  const samples = props.samples.slice(-slots.value)
  const offset = slots.value - samples.length
  const step = width / Math.max(1, slots.value - 1)
  return samples
    .map((s, i) => `${((offset + i) * step).toFixed(1)},${(height - 4 - (value(s) / top.value) * (height - 8)).toFixed(1)}`)
    .join(' ')
}
const download = computed(() => line((s) => s.download))
const upload = computed(() => line((s) => s.upload))
</script>

<template>
  <svg :viewBox="`0 0 ${width} ${height}`" preserveAspectRatio="none" class="block h-full w-full" aria-hidden="true">
    <defs>
      <linearGradient :id="gradient" x1="0" x2="1" y1="0" y2="0">
        <stop offset="0" stop-color="var(--blue)" />
        <stop offset="1" stop-color="var(--violet)" />
      </linearGradient>
    </defs>
    <polyline v-if="samples.length > 1" :points="upload" fill="none" stroke="var(--ok)" stroke-width="2" vector-effect="non-scaling-stroke" />
    <polyline
      v-if="samples.length > 1"
      :points="download"
      fill="none"
      :stroke="`url(#${gradient})`"
      stroke-width="2"
      vector-effect="non-scaling-stroke"
    />
  </svg>
</template>
