<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Input, Switch } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'

// «Log» of the advanced mode: how many of the last lines the log screen shows, and the statistics of connections.
// Changes wait for «Save».
const { t } = useI18n()
const lines = useSetting('logLines')
const stats = useSetting('stats')

// A number as typed; the core refuses what is out of range at «Save».
const linesText = computed({
  get: () => (lines.value ? String(lines.value) : ''),
  set: (text: string) => {
    const value = Number(text.trim())
    if (Number.isInteger(value)) lines.value = value
  },
})
</script>

<template>
  <h2 class="mb-4 font-disp text-[19px] font-normal">{{ t('settings.nav.logging') }}</h2>
  <SettingsCard :title="t('settings.logging.screen')">
    <label class="grid max-w-[346px] gap-[5px] pt-2.5 pb-1">
      <span class="text-[12.5px] text-t2">{{ t('settings.logging.lines') }}</span>
      <Input v-model="linesText" mono inputmode="numeric" data-testid="log-lines" />
      <span class="text-xs text-t3">{{ t('settings.logging.linesHint') }}</span>
    </label>
  </SettingsCard>
  <SettingsCard :title="t('settings.logging.statsTitle')">
    <label class="flex items-start gap-3 pt-2.5 pb-1">
      <Switch v-model="stats" :aria-label="t('settings.logging.stats')" data-testid="stats" />
      <span>{{ t('settings.logging.stats') }}<br /><span class="text-xs text-t3">{{ t('settings.logging.statsHint') }}</span></span>
    </label>
  </SettingsCard>
</template>
