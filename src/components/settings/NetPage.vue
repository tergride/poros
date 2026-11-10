<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Input, Select, Switch } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'

// «Network» of the advanced mode: the certificate check of Poros's own requests, and a clock of the core's own from
// an NTP server for when the system's is wrong. Changes wait for «Save».
const { t } = useI18n()
const ignoreTls = useSetting('ignoreTlsErrors')
const ntp = useSetting('ntp')
const server = useSetting('ntpServer')
const port = useSetting('ntpPort')
const interval = useSetting('ntpInterval')
const outbound = useSetting('ntpOutbound')

// A number as typed; the core refuses what is out of range at «Save».
function whole(model: { value: number }) {
  return computed({
    get: () => (model.value ? String(model.value) : ''),
    set: (text: string) => {
      const value = Number(text.trim())
      if (Number.isInteger(value)) model.value = value
    },
  })
}
const portText = whole(port)
const intervalText = whole(interval)

const outbounds = computed(() => [
  { value: 'direct', label: t('settings.net.direct') },
  { value: 'proxy', label: t('settings.net.proxy') },
])
</script>

<template>
  <h2 class="mb-4 font-disp text-[19px] font-normal">{{ t('settings.nav.net') }}</h2>
  <SettingsCard :title="t('settings.net.general')">
    <label class="flex items-start gap-3 pt-2.5 pb-1">
      <Switch v-model="ignoreTls" :aria-label="t('settings.net.ignoreTls')" data-testid="ignore-tls" />
      <span>{{ t('settings.net.ignoreTls') }}<br /><span class="text-xs text-t3">{{ t('settings.net.ignoreTlsHint') }}</span></span>
    </label>
  </SettingsCard>
  <SettingsCard title="NTP">
    <label class="flex items-start gap-3 pt-2.5">
      <Switch v-model="ntp" :aria-label="t('settings.net.ntpOn')" data-testid="ntp" />
      <span>{{ t('settings.net.ntpOn') }}<br /><span class="text-xs text-t3">{{ t('settings.net.ntpHint') }}</span></span>
    </label>
    <div class="grid grid-cols-2 gap-x-4 gap-y-3 pt-3 pb-1">
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.net.server') }}</span>
        <Input v-model="server" mono :disabled="!ntp" spellcheck="false" data-testid="ntp-server" />
      </label>
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.net.port') }}</span>
        <Input v-model="portText" mono inputmode="numeric" :disabled="!ntp" data-testid="ntp-port" />
      </label>
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.net.interval') }}</span>
        <Input v-model="intervalText" mono inputmode="numeric" :disabled="!ntp" data-testid="ntp-interval" />
      </label>
      <div class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.net.outbound') }}</span>
        <Select v-model="outbound" :options="outbounds" class="w-full" :disabled="!ntp" :aria-label="t('settings.net.outbound')" />
      </div>
    </div>
  </SettingsCard>
</template>
