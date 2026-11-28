<script setup lang="ts">
import { Check, Copy, Eraser, FolderOpen, RefreshCw, ScrollText, Zap } from '@lucide/vue'
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import CleanupDialog from '@/components/settings/CleanupDialog.vue'
import OptionRow from '@/components/settings/OptionRow.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import UpdateRows from '@/components/settings/UpdateRows.vue'
import { Button, Switch } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useResetConnection } from '@/composables/useResetConnection'
import { useSetting } from '@/composables/useSetting'
import { useSettingsPage } from '@/composables/useSettingsBlock'
import { appControls } from '@/platform'
import { MethodAppDebugInfo } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// «Help»: what to do when something does not work, updates, and the removal of what Poros left outside its folder before it is deleted. The
// advanced mode gives each its card and adds the debug information for a report, the data folder and beta versions.
const core = useCoreStore()
const ui = useUiStore()
const router = useRouter()
const { t } = useI18n()
const advanced = useSettingsPage() !== null
const { reset, resetting } = useResetConnection()
const debugLog = useSetting('debugLog')
const beta = useSetting('updateBeta')
const attempt = useAttempt()
const openLogs = () => attempt(() => appControls.openLogs())
const openData = () => attempt(() => appControls.openData())
const copied = ref(false)
const cleaning = ref(false)

async function copyDebugInfo(): Promise<void> {
  await navigator.clipboard.writeText(await core.core().call(MethodAppDebugInfo))
  copied.value = true
  setTimeout(() => (copied.value = false), 2000)
}
</script>

<template>
  <h2 :class="['font-disp text-[19px] font-normal', advanced ? 'mb-4' : 'mb-1']">{{ t('settings.nav.help') }}</h2>
  <p v-if="!advanced" class="mb-4 max-w-[640px] text-t2">{{ t('settings.help.lead') }}</p>
  <SettingsCard :title="advanced ? t('settings.help.trouble') : undefined">
    <OptionRow :title="t('settings.help.wizard')" :hint="t('settings.help.wizardHint')">
      <Button @click="router.push('/wizard/1')"><Zap />{{ t('settings.help.run') }}</Button>
    </OptionRow>
    <OptionRow :title="t('settings.help.logs')" :hint="t('settings.help.logsHint')">
      <Button @click="ui.logs = true"><ScrollText />{{ t('settings.help.open') }}</Button>
      <Button @click="openLogs"><FolderOpen />{{ t('settings.help.logsFolder') }}</Button>
    </OptionRow>
    <OptionRow :title="t('settings.help.debugLog')" :hint="t('settings.help.debugLogHint')">
      <Switch v-model="debugLog" data-testid="debug-log" />
    </OptionRow>
    <OptionRow :title="t('settings.help.reset')" :hint="t('settings.help.resetHint')">
      <Button :disabled="resetting" @click="attempt(reset)"><RefreshCw :class="resetting && 'animate-spin'" />{{ t('settings.help.resetButton') }}</Button>
    </OptionRow>
    <template v-if="advanced">
      <OptionRow :title="t('settings.help.debugInfo')" :hint="t('settings.help.debugInfoHint')">
        <Button data-testid="debug-info" @click="attempt(copyDebugInfo)">
          <Check v-if="copied" /><Copy v-else />{{ t(copied ? 'settings.help.copied' : 'settings.help.copy') }}
        </Button>
      </OptionRow>
      <OptionRow :title="t('settings.help.dataFolder')" :hint="t('settings.help.dataFolderHint')">
        <Button @click="openData"><FolderOpen />{{ t('settings.help.open') }}</Button>
      </OptionRow>
    </template>
    <UpdateRows v-else />
  </SettingsCard>
  <SettingsCard v-if="advanced" :title="t('settings.help.updatesTitle')">
    <UpdateRows :title="t('settings.help.poros')" />
    <OptionRow :title="t('settings.help.beta')" :hint="t('settings.help.betaHint')">
      <Switch v-model="beta" data-testid="update-beta" />
    </OptionRow>
  </SettingsCard>
  <SettingsCard :title="advanced ? t('settings.help.cleanupTitle') : undefined">
    <OptionRow :title="t('settings.help.cleanup')" :hint="t('settings.help.cleanupHint')">
      <Button data-testid="cleanup-open" @click="cleaning = true"><Eraser />{{ t('settings.help.cleanupButton') }}</Button>
    </OptionRow>
  </SettingsCard>
  <CleanupDialog v-model:open="cleaning" />
</template>
