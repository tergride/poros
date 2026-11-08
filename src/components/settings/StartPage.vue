<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import OptionRow from '@/components/settings/OptionRow.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Button, Switch } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useConnect } from '@/composables/useConnect'
import { useSetting, useSettingAction } from '@/composables/useSetting'
import { useSettingsDraft } from '@/composables/useSettingsDraft'
import { type UrlScheme, urlScheme } from '@/platform'

const { t } = useI18n()
const attempt = useAttempt()
const connect = useConnect()
const autostart = useSetting('autostart')
const autoConnect = useSetting('autoConnect')
const startMinimized = useSetting('startMinimized')
const closeToTray = useSetting('closeToTray')
const runAsAdmin = useSetting('runAsAdmin')

// Turning it on asks for rights first: the sign-in task is made by a core that has them.
const adminAutostart = useSettingAction('adminAutostart', (on) => connect.setAdminAutostart(on))

// The switch shows what Windows holds and changes it at once.
const scheme = ref<UrlScheme | null>(null)
onMounted(async () => (scheme.value = await urlScheme.get()))
const schemeOn = computed({
  get: () => scheme.value?.owner === 'this',
  set: (on: boolean) => void attempt(async () => (scheme.value = await urlScheme.set(on))),
})
// The advanced mode tells what Windows holds and offers the one action that changes it.
const advanced = useSettingsDraft() !== null
const schemeState = computed(() =>
  !scheme.value
    ? ''
    : scheme.value.owner === 'this'
      ? t('settings.start.urlSchemeThis')
      : scheme.value.owner === 'other'
        ? t('settings.start.urlSchemeOtherCopy', { program: scheme.value.program })
        : t('settings.start.urlSchemeNone'),
)
const schemeHint = computed(() =>
  scheme.value?.owner === 'other' ? t('settings.start.urlSchemeOther', { program: scheme.value.program }) : '',
)
</script>

<template>
  <h2 class="mb-1 font-disp text-[19px] font-normal">{{ t('settings.nav.start') }}</h2>
  <p class="mb-4 max-w-[640px] text-t2">{{ t('settings.start.lead') }}</p>
  <SettingsCard>
    <!-- What happens at launch stays together: the start with Windows, its rights, how it shows, the connection. -->
    <OptionRow :title="t('wizard.start.autostart')"><Switch v-model="autostart" data-testid="autostart" /></OptionRow>
    <OptionRow
      :title="t('settings.start.adminAutostart')"
      :hint="autostart ? t('settings.start.adminAutostartHint') : t('settings.start.autostartOnly')"
    >
      <Switch v-model="adminAutostart" :disabled="!autostart" data-testid="admin-autostart" />
    </OptionRow>
    <!-- Only the start with Windows comes minimized: a start by hand always shows the window. -->
    <OptionRow
      :title="t('settings.start.startMinimized')"
      :hint="autostart ? t('settings.start.startMinimizedHint') : t('settings.start.autostartOnly')"
    >
      <Switch v-model="startMinimized" :disabled="!autostart" />
    </OptionRow>
    <OptionRow :title="t('settings.start.runAsAdmin')" :hint="t('settings.start.runAsAdminHint')">
      <Switch v-model="runAsAdmin" data-testid="run-as-admin" />
    </OptionRow>
    <OptionRow :title="t('wizard.start.autoConnect')"><Switch v-model="autoConnect" /></OptionRow>
    <OptionRow v-if="advanced" :title="t('settings.start.urlSchemeTitle')" :hint="schemeState">
      <Button size="sm" :disabled="!scheme" data-testid="url-scheme-action" @click="schemeOn = !schemeOn">
        {{ schemeOn ? t('settings.start.urlSchemeRemove') : t('settings.start.urlSchemeInstall') }}
      </Button>
    </OptionRow>
    <OptionRow v-else :title="t('wizard.start.urlScheme')" :hint="schemeHint">
      <Switch v-model="schemeOn" :disabled="!scheme" data-testid="url-scheme" />
    </OptionRow>
    <OptionRow :title="t('settings.start.closeToTray')" :hint="t('settings.start.closeToTrayHint')">
      <Switch v-model="closeToTray" />
    </OptionRow>
  </SettingsCard>
</template>
