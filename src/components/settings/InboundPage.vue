<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Input, Switch } from '@/components/ui'
import { useSettingsBlock } from '@/composables/useSettingsBlock'
import { localProxyPort } from '@/platform/core'

// The local proxy of the advanced mode: its port, and a name and password programs must give, which come on with
// their switch. The system proxy of Windows passes no password, so the core gives it a port of its own while one is
// asked for.
const { t } = useI18n()
const portBlock = useSettingsBlock()
const port = portBlock.setting('proxyPort')
const authBlock = useSettingsBlock()
const auth = authBlock.setting('proxyAuth')
const username = authBlock.setting('proxyUsername')
const password = authBlock.setting('proxyPassword')

const portText = computed({
  get: () => (port.value ? String(port.value) : ''),
  set: (text: string) => {
    const value = Number(text.trim())
    port.value = text.trim() === '' ? 0 : Number.isInteger(value) ? value : port.value
  },
})
</script>

<template>
  <h2 class="mb-4 font-disp text-[19px] font-normal">{{ t('settings.nav.inbound') }}</h2>
  <SettingsCard :title="t('settings.inbound.port')" :block="portBlock">
    <label class="grid max-w-[346px] gap-[5px] pt-2.5 pb-1">
      <span class="text-[12.5px] text-t2">{{ t('settings.inbound.portLabel') }}</span>
      <Input v-model="portText" mono inputmode="numeric" :placeholder="String(localProxyPort)" data-testid="proxy-port" />
      <span class="text-xs text-t3">{{ t('settings.inbound.portHint') }}</span>
    </label>
  </SettingsCard>
  <SettingsCard :title="t('settings.inbound.auth')" :block="authBlock">
    <label class="flex items-start gap-3 pt-2.5">
      <Switch v-model="auth" :aria-label="t('settings.inbound.authOn')" data-testid="proxy-auth" />
      <span>{{ t('settings.inbound.authOn') }}<br /><span class="text-xs text-t3">{{ t('settings.inbound.authHint') }}</span></span>
    </label>
    <div class="grid grid-cols-2 gap-x-4 gap-y-3 pt-3 pb-1">
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.inbound.username') }}</span>
        <Input v-model="username" :disabled="!auth" autocomplete="off" data-testid="proxy-username" />
      </label>
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.inbound.password') }}</span>
        <Input v-model="password" type="password" :disabled="!auth" autocomplete="new-password" data-testid="proxy-password" />
      </label>
    </div>
  </SettingsCard>
</template>
