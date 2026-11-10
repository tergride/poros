<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Switch } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'

// «Security» of the advanced mode: which certificates Xray trusts, and the certificate check of servers that do not
// skip it themselves. Changes wait for «Save».
const { t } = useI18n()
const systemCerts = useSetting('xraySystemCerts')
const insecure = useSetting('insecureDefault')
</script>

<template>
  <h2 class="mb-4 font-disp text-[19px] font-normal">{{ t('settings.nav.security') }}</h2>
  <SettingsCard :title="t('settings.security.protection')">
    <label class="flex items-start gap-3 pt-2.5">
      <Switch v-model="systemCerts" :aria-label="t('settings.security.systemCerts')" data-testid="xray-system-certs" />
      <span>{{ t('settings.security.systemCerts') }}<br /><span class="text-xs text-t3">{{ t('settings.security.systemCertsHint') }}</span></span>
    </label>
    <label class="flex items-start gap-3 pt-3 pb-1">
      <Switch v-model="insecure" :aria-label="t('settings.security.insecure')" data-testid="insecure-default" />
      <span>{{ t('settings.security.insecure') }}<br /><span class="text-xs text-t3">{{ t('settings.security.insecureHint') }}</span></span>
    </label>
  </SettingsCard>
</template>
