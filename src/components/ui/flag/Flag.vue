<script setup lang="ts">
import { cn } from 'cn'
import { type HTMLAttributes, computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { countryHint, flagOf } from '@/lib/format'

// A country as its flag; the hint and screen readers name it: «DE — Германия». A code with no flag stays a code.
const props = defineProps<{ code: string; class?: HTMLAttributes['class'] }>()
const { locale } = useI18n()
const flag = computed(() => flagOf(props.code))
const hint = computed(() => countryHint(props.code, locale.value))
</script>

<template>
  <span
    v-if="code"
    role="img"
    :title="hint"
    :aria-label="hint"
    :class="cn('inline-block leading-none', !flag && 'font-mono text-[11px]', props.class)"
    data-testid="flag"
    >{{ flag || code.toUpperCase() }}</span
  >
</template>
