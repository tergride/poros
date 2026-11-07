<script setup lang="ts">
import { Download, RefreshCw, ScrollText, Zap } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import OptionRow from '@/components/settings/OptionRow.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Button, Switch } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useClock } from '@/composables/useClock'
import { useConnect } from '@/composables/useConnect'
import { useReason } from '@/composables/useReason'
import { useSetting } from '@/composables/useSetting'
import { useUpdates } from '@/composables/useUpdates'
import { formatAgo } from '@/lib/format'
import { UpdateChecking, UpdateDownloading, UpdateReady } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

const core = useCoreStore()
const ui = useUiStore()
const connect = useConnect()
const updates = useUpdates()
const router = useRouter()
const { t, locale } = useI18n()
const resetting = ref(false)
const updateCheck = useSetting('updateCheck')
const now = useClock(60_000)
const reason = useReason()
const attempt = useAttempt()
// Why "Restart and update" did not hand over to the updater; Poros runs on.
const installFailure = ref('')

async function install(): Promise<void> {
  installFailure.value = ''
  try {
    await updates.install()
  } catch (error) {
    installFailure.value = t('updates.installFailed', { reason: reason(error) })
  }
}

// The mockup's line under "Updates": what this build is and what the last check found.
const updateHint = computed(() => {
  const state = core.updates
  if (installFailure.value) return installFailure.value
  const current = state.current
  if (!current) return t('settings.help.local')
  if (!state.available) return t('settings.help.noUpdater', { version: current })
  if (state.state === UpdateChecking) return t('settings.help.checking')
  if (state.state === UpdateDownloading) return t('settings.help.downloading', { version: state.version })
  if (state.state === UpdateReady) return t('settings.help.ready', { version: state.version })
  if (state.error) return t('settings.help.checkFailed', { version: current })
  if (state.checkedAt && updates.checkedByHand) return t('settings.help.latest', { version: current })
  if (state.checkedAt) {
    return t('settings.help.checked', { version: current, when: formatAgo(state.checkedAt, now.value, locale.value) })
  }
  return t('settings.help.version', { version: current })
})
const busy = computed(() => core.updates.state === UpdateChecking || core.updates.state === UpdateDownloading)

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
      <Button :disabled="resetting" @click="attempt(reset)"><RefreshCw :class="resetting && 'animate-spin'" />{{ t('settings.help.resetButton') }}</Button>
    </OptionRow>
    <OptionRow :title="t('settings.help.updates')" :hint="updateHint" data-testid="updates">
      <Button v-if="core.updates.state === UpdateReady" variant="primary" @click="install()">
        <RefreshCw />{{ t('updates.install') }}
      </Button>
      <Button v-else :disabled="!core.updates.available || busy" @click="attempt(() => updates.check())">
        <Download />{{ t('settings.help.check') }}
      </Button>
    </OptionRow>
    <OptionRow :title="t('settings.help.autoCheck')" :hint="t('settings.help.autoCheckHint')">
      <Switch v-model="updateCheck" />
    </OptionRow>
  </SettingsCard>
</template>
