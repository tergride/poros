<script setup lang="ts">
import { Download, RefreshCw, ScrollText, Zap } from '@lucide/vue'
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import OptionRow from '@/components/settings/OptionRow.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Button } from '@/components/ui'
import { useConnect } from '@/composables/useConnect'
import { appInfo } from '@/platform'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

const core = useCoreStore()
const ui = useUiStore()
const connect = useConnect()
const router = useRouter()
const { t } = useI18n()
const version = ref('')
const resetting = ref(false)

onMounted(async () => (version.value = (await appInfo()).version))

/** A new core with the same rights, and the connection back if there was one. */
async function reset(): Promise<void> {
  const wasRunning = core.running
  resetting.value = true
  try {
    await (core.status.elevated ? core.core().elevate() : core.core().restart())
    await new Promise<void>((resolve) => {
      const stop = watch(
        () => core.status.state === 'ready' && core.loaded,
        (ready) => {
          if (ready) {
            stop()
            resolve()
          }
        },
      )
    })
    if (wasRunning) await connect.start()
  } finally {
    resetting.value = false
  }
}
</script>

<template>
  <h2 class="mb-1 font-disp text-[19px] font-normal">{{ t('settings.nav.help') }}</h2>
  <p class="mb-4 max-w-[640px] text-t2">{{ t('settings.help.lead') }}</p>
  <SettingsCard>
    <OptionRow :title="t('settings.help.wizard')" :hint="t('settings.help.wizardHint')">
      <Button @click="router.push('/wizard/1')"><Zap />{{ t('settings.help.run') }}</Button>
    </OptionRow>
    <OptionRow :title="t('settings.help.logs')" :hint="t('settings.help.logsHint')">
      <Button @click="ui.logs = true"><ScrollText />{{ t('settings.help.open') }}</Button>
    </OptionRow>
    <OptionRow :title="t('settings.help.reset')" :hint="t('settings.help.resetHint')">
      <Button :disabled="resetting" @click="reset"><RefreshCw :class="resetting && 'animate-spin'" />{{ t('settings.help.resetButton') }}</Button>
    </OptionRow>
    <OptionRow :title="t('settings.help.updates')" :hint="version && t('settings.help.version', { version })">
      <Button disabled><Download />{{ t('settings.help.check') }}</Button>
    </OptionRow>
  </SettingsCard>
</template>
