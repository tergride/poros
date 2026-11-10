<script setup lang="ts">
import { Download, RefreshCw } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import OptionRow from '@/components/settings/OptionRow.vue'
import { Button, Switch } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useClock } from '@/composables/useClock'
import { useReason } from '@/composables/useReason'
import { useSetting } from '@/composables/useSetting'
import { useUpdates } from '@/composables/useUpdates'
import { formatAgo } from '@/lib/format'
import { UpdateChecking, UpdateDownloading, UpdateReady } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// The rows of «Help» about updates: this build, the check, and checking by itself. title names the first row.
const props = defineProps<{ title?: string }>()
const core = useCoreStore()
const updates = useUpdates()
const { t, locale } = useI18n()
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
</script>

<template>
  <OptionRow :title="props.title ?? t('settings.help.updates')" :hint="updateHint" data-testid="updates">
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
</template>
