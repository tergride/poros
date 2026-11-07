<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import OptionRow from '@/components/settings/OptionRow.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Select, Switch } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'

const { t } = useI18n()
const autoUpdate = useSetting('subscriptionAutoUpdate')
const hours = useSetting('subscriptionUpdateHours')
const viaProxy = useSetting('subscriptionViaProxy')
const showChanges = useSetting('showChanges')
const deviceInfo = useSetting('sendDeviceInfo')

const intervals = computed(() => [
  { value: 0, label: t('settings.subs.provider') },
  { value: 1, label: t('settings.subs.hourly') },
  { value: 6, label: t('settings.subs.every6') },
  { value: 24, label: t('settings.subs.daily') },
])
</script>

<template>
  <h2 class="mb-1 font-disp text-[19px] font-normal">{{ t('settings.nav.subs') }}</h2>
  <p class="mb-4 max-w-[640px] text-t2">{{ t('settings.subs.lead') }}</p>
  <SettingsCard>
    <OptionRow :title="t('settings.subs.autoUpdate')"><Switch v-model="autoUpdate" /></OptionRow>
    <OptionRow :title="t('settings.subs.interval')">
      <Select v-model="hours" :options="intervals" :disabled="!autoUpdate" :aria-label="t('settings.subs.interval')" />
    </OptionRow>
    <OptionRow :title="t('settings.subs.viaProxy')" :hint="t('settings.subs.viaProxyHint')"><Switch v-model="viaProxy" /></OptionRow>
    <OptionRow :title="t('settings.subs.showChanges')" :hint="t('settings.subs.showChangesHint')"><Switch v-model="showChanges" /></OptionRow>
    <OptionRow :title="t('settings.subs.deviceInfo')" :hint="t('settings.subs.deviceInfoHint')"><Switch v-model="deviceInfo" /></OptionRow>
  </SettingsCard>
</template>
