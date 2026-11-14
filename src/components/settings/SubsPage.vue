<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import OptionRow from '@/components/settings/OptionRow.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Input, Select, Switch } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'
import { useSettingsDraft } from '@/composables/useSettingsDraft'
import { useCoreStore } from '@/stores/core'

// «Subscriptions»: how they update. The advanced mode adds how they are asked for: the User-Agent, the versions of
// TLS and HTTP, and what providers are told about the device.
const { t } = useI18n()
const core = useCoreStore()
const advanced = useSettingsDraft() !== null
const autoUpdate = useSetting('subscriptionAutoUpdate')
const hours = useSetting('subscriptionUpdateHours')
const viaProxy = useSetting('subscriptionViaProxy')
const showChanges = useSetting('showChanges')
const deviceInfo = useSetting('sendDeviceInfo')
const userAgent = useSetting('subscriptionUserAgent')
const tls = useSetting('subscriptionTls')
const http = useSetting('subscriptionHttp')
const deviceData = useSetting('deviceData')

const intervals = computed(() => [
  { value: 0, label: t('settings.subs.provider') },
  { value: 1, label: t('settings.subs.hourly') },
  { value: 6, label: t('settings.subs.every6') },
  { value: 24, label: t('settings.subs.daily') },
])
const tlsVersions = computed(() => [
  { value: '', label: t('settings.subs.auto') },
  { value: '1.2', label: 'TLS 1.2' },
  { value: '1.3', label: 'TLS 1.3' },
])
const httpVersions = computed(() => [
  { value: '', label: t('settings.subs.auto') },
  { value: '1.1', label: 'HTTP/1.1' },
])
const ownAgent = computed(() => (core.updates.current ? `Poros/${core.updates.current}` : 'Poros'))
</script>

<template>
  <h2 :class="['font-disp text-[19px] font-normal', advanced ? 'mb-4' : 'mb-1']">{{ t('settings.nav.subs') }}</h2>
  <p v-if="!advanced" class="mb-4 max-w-[640px] text-t2">{{ t('settings.subs.lead') }}</p>
  <SettingsCard :title="advanced ? t('settings.subs.update') : undefined">
    <OptionRow :title="t('settings.subs.autoUpdate')"><Switch v-model="autoUpdate" /></OptionRow>
    <OptionRow :title="t('settings.subs.interval')">
      <Select v-model="hours" :options="intervals" :disabled="!autoUpdate" :aria-label="t('settings.subs.interval')" />
    </OptionRow>
    <OptionRow :title="t('settings.subs.viaProxy')" :hint="t('settings.subs.viaProxyHint')"><Switch v-model="viaProxy" /></OptionRow>
    <OptionRow :title="t('settings.subs.showChanges')" :hint="t('settings.subs.showChangesHint')"><Switch v-model="showChanges" /></OptionRow>
    <OptionRow :title="t('settings.subs.deviceInfo')" :hint="t('settings.subs.deviceInfoHint')"><Switch v-model="deviceInfo" /></OptionRow>
  </SettingsCard>
  <SettingsCard v-if="advanced" :title="t('settings.subs.request')">
    <div class="grid grid-cols-2 gap-x-4 gap-y-3 pt-2.5 pb-1">
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">User Agent</span>
        <Input v-model="userAgent" :placeholder="ownAgent" spellcheck="false" data-testid="sub-user-agent" />
      </label>
      <div class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.subs.tls') }}</span>
        <Select v-model="tls" :options="tlsVersions" class="w-full" :aria-label="t('settings.subs.tls')" />
      </div>
      <div class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.subs.http') }}</span>
        <Select v-model="http" :options="httpVersions" class="w-full" :aria-label="t('settings.subs.http')" />
      </div>
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.subs.deviceData') }}</span>
        <Input v-model="deviceData" mono placeholder="hwid=…, os=…, osVersion=…, model=…" spellcheck="false" data-testid="sub-device-data" />
      </label>
    </div>
    <p class="pb-1 text-xs text-t3">{{ t('settings.subs.deviceDataHint') }}</p>
  </SettingsCard>
</template>
