<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Input, Select } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'
import { SpeedTestCountry, SpeedTestDownload, SpeedTestFull, SpeedTestSimple, SpeedTestUpload } from '@/platform/core-types'

// The tests of servers: what the delay test asks for, how long it waits and how many servers it tests at once, and
// what a speed test measures. The core checks the values on «Save».
const { t } = useI18n()
const latencyUrl = useSetting('latencyUrl')
const timeout = useSetting('latencyTimeout')
const concurrency = useSetting('latencyConcurrency')
const speedMode = useSetting('speedTestMode')
const speedUrl = useSetting('speedTestUrl')

// A number as typed: what is no whole number keeps the value there was.
function whole(model: { value: number }) {
  return computed({
    get: () => String(model.value),
    set: (text: string) => {
      const value = Number(text.trim())
      if (text.trim() !== '' && Number.isInteger(value)) model.value = value
    },
  })
}
const timeoutText = whole(timeout)
const concurrencyText = whole(concurrency)
const modes = computed(() =>
  [SpeedTestFull, SpeedTestDownload, SpeedTestUpload, SpeedTestSimple, SpeedTestCountry].map((value) => ({ value, label: t(`settings.tests.modes.${value}`) })),
)
</script>

<template>
  <h2 class="mb-4 font-disp text-[19px] font-normal">{{ t('settings.nav.tests') }}</h2>
  <SettingsCard :title="t('settings.tests.latency')">
    <div class="grid grid-cols-2 gap-x-4 gap-y-3 pt-2.5 pb-1">
      <label class="col-span-2 grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.tests.url') }}</span>
        <Input v-model="latencyUrl" mono placeholder="http://cp.cloudflare.com/" data-testid="latency-url" />
      </label>
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.tests.timeout') }}</span>
        <Input v-model="timeoutText" mono inputmode="numeric" data-testid="latency-timeout" />
      </label>
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.tests.concurrency') }}</span>
        <Input v-model="concurrencyText" mono inputmode="numeric" data-testid="latency-concurrency" />
      </label>
    </div>
  </SettingsCard>
  <SettingsCard :title="t('settings.tests.speed')">
    <div class="grid grid-cols-2 gap-x-4 gap-y-3 pt-2.5 pb-1">
      <div class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.tests.mode') }}</span>
        <Select v-model="speedMode" :options="modes" class="w-full" :aria-label="t('settings.tests.mode')" data-testid="speed-mode" />
      </div>
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.tests.speedUrl') }}</span>
        <Input v-model="speedUrl" mono :disabled="speedMode !== SpeedTestSimple" data-testid="speed-url" />
      </label>
    </div>
  </SettingsCard>
</template>
