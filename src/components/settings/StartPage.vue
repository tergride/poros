<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import OptionRow from '@/components/settings/OptionRow.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Switch } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'

const { t } = useI18n()
const autostart = useSetting('autostart')
const autoConnect = useSetting('autoConnect')
const urlScheme = useSetting('urlScheme')
const startMinimized = useSetting('startMinimized')
const closeToTray = useSetting('closeToTray')
</script>

<template>
  <h2 class="mb-1 font-disp text-[19px] font-normal">{{ t('settings.nav.start') }}</h2>
  <p class="mb-4 max-w-[640px] text-t2">{{ t('settings.start.lead') }}</p>
  <SettingsCard>
    <OptionRow :title="t('wizard.start.autostart')"><Switch v-model="autostart" /></OptionRow>
    <OptionRow :title="t('wizard.start.autoConnect')"><Switch v-model="autoConnect" /></OptionRow>
    <OptionRow :title="t('wizard.start.urlScheme')"><Switch v-model="urlScheme" /></OptionRow>
    <OptionRow :title="t('settings.start.startMinimized')" :hint="t('settings.start.startMinimizedHint')">
      <Switch v-model="startMinimized" />
    </OptionRow>
    <OptionRow :title="t('settings.start.closeToTray')" :hint="t('settings.start.closeToTrayHint')">
      <Switch v-model="closeToTray" />
    </OptionRow>
  </SettingsCard>
</template>
