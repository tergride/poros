<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import AdminDialog from '@/components/main/AdminDialog.vue'
import OptionRow from '@/components/settings/OptionRow.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Segmented, Switch } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useConnect } from '@/composables/useConnect'
import { useSetting } from '@/composables/useSetting'
import { KillSwitchAlways, KillSwitchConnected, KillSwitchOff } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

const core = useCoreStore()
const connect = useConnect()
const attempt = useAttempt()
const { t } = useI18n()
const allowLan = useSetting('allowLan')
const checkIp = useSetting('checkIp')
const ipv6 = useSetting('ipv6')
const reality = useSetting('realityOnXray')

const modes = computed(() => [
  { value: KillSwitchOff, label: t('settings.conn.killSwitchOff') },
  { value: KillSwitchConnected, label: t('settings.conn.killSwitchConnected') },
  { value: KillSwitchAlways, label: t('settings.conn.killSwitchAlways') },
])
const killSwitch = computed({
  get: () => core.settings?.killSwitch ?? KillSwitchOff,
  set: (mode: string) => void attempt(() => connect.setKillSwitch(mode)),
})
</script>

<template>
  <h2 class="mb-1 font-disp text-[19px] font-normal">{{ t('settings.nav.conn') }}</h2>
  <p class="mb-4 max-w-[640px] text-t2">{{ t('settings.conn.lead') }}</p>
  <SettingsCard>
    <OptionRow
      :title="t('settings.conn.killSwitch')"
      :hint="t(`settings.conn.killSwitchHint.${killSwitch}`)"
      :note="t('settings.conn.killSwitchNote')"
      :shield="!core.status.elevated"
      data-testid="kill-switch"
    >
      <Segmented v-model="killSwitch" :options="modes" :aria-label="t('settings.conn.killSwitch')" />
    </OptionRow>
    <OptionRow :title="t('settings.conn.allowLan')" :hint="t('settings.conn.allowLanHint')"><Switch v-model="allowLan" /></OptionRow>
    <OptionRow :title="t('settings.conn.checkIp')" :hint="t('settings.conn.checkIpHint')"><Switch v-model="checkIp" /></OptionRow>
    <OptionRow :title="t('settings.conn.ipv6')" :hint="t('settings.conn.ipv6Hint')"><Switch v-model="ipv6" /></OptionRow>
    <OptionRow :title="t('settings.conn.reality')" :hint="t('settings.conn.realityHint')"><Switch v-model="reality" /></OptionRow>
  </SettingsCard>
  <AdminDialog />
</template>
