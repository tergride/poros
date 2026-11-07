<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import OptionRow from '@/components/settings/OptionRow.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Switch } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'
import { useCoreStore } from '@/stores/core'

const core = useCoreStore()
const { t } = useI18n()
const killSwitch = useSetting('killSwitch')
const allowLan = useSetting('allowLan')
const checkIp = useSetting('checkIp')
const ipv6 = useSetting('ipv6')
const reality = useSetting('realityOnXray')
</script>

<template>
  <h2 class="mb-1 font-disp text-[19px] font-normal">{{ t('settings.nav.conn') }}</h2>
  <p class="mb-4 max-w-[640px] text-t2">{{ t('settings.conn.lead') }}</p>
  <SettingsCard>
    <OptionRow :title="t('settings.conn.killSwitch')" :hint="t('settings.conn.killSwitchHint')" :shield="!core.status.elevated">
      <Switch v-model="killSwitch" />
    </OptionRow>
    <OptionRow :title="t('settings.conn.allowLan')" :hint="t('settings.conn.allowLanHint')"><Switch v-model="allowLan" /></OptionRow>
    <OptionRow :title="t('settings.conn.checkIp')" :hint="t('settings.conn.checkIpHint')"><Switch v-model="checkIp" /></OptionRow>
    <OptionRow :title="t('settings.conn.ipv6')" :hint="t('settings.conn.ipv6Hint')"><Switch v-model="ipv6" /></OptionRow>
    <OptionRow :title="t('settings.conn.reality')" :hint="t('settings.conn.realityHint')"><Switch v-model="reality" /></OptionRow>
  </SettingsCard>
</template>
