<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import OptionRow from '@/components/settings/OptionRow.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Switch } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'
import { type UrlScheme, urlScheme } from '@/platform'

const { t } = useI18n()
const autostart = useSetting('autostart')
const autoConnect = useSetting('autoConnect')
const startMinimized = useSetting('startMinimized')
const closeToTray = useSetting('closeToTray')

// The switch shows what Windows holds and changes it at once.
const scheme = ref<UrlScheme | null>(null)
onMounted(async () => (scheme.value = await urlScheme.get()))
const schemeOn = computed({
  get: () => scheme.value?.owner === 'this',
  set: (on: boolean) => void urlScheme.set(on).then((next) => (scheme.value = next)),
})
const schemeHint = computed(() =>
  scheme.value?.owner === 'other' ? t('settings.start.urlSchemeOther', { program: scheme.value.program }) : '',
)
</script>

<template>
  <h2 class="mb-1 font-disp text-[19px] font-normal">{{ t('settings.nav.start') }}</h2>
  <p class="mb-4 max-w-[640px] text-t2">{{ t('settings.start.lead') }}</p>
  <SettingsCard>
    <OptionRow :title="t('wizard.start.autostart')"><Switch v-model="autostart" /></OptionRow>
    <OptionRow :title="t('wizard.start.autoConnect')"><Switch v-model="autoConnect" /></OptionRow>
    <OptionRow :title="t('wizard.start.urlScheme')" :hint="schemeHint">
      <Switch v-model="schemeOn" :disabled="!scheme" data-testid="url-scheme" />
    </OptionRow>
    <OptionRow :title="t('settings.start.startMinimized')" :hint="t('settings.start.startMinimizedHint')">
      <Switch v-model="startMinimized" />
    </OptionRow>
    <OptionRow :title="t('settings.start.closeToTray')" :hint="t('settings.start.closeToTrayHint')">
      <Switch v-model="closeToTray" />
    </OptionRow>
  </SettingsCard>
</template>
