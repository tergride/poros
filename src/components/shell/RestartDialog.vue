<script setup lang="ts">
import { RefreshCw } from '@lucide/vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useRestartNeeded } from '@/composables/useRestartNeeded'

// Opened from the chip in the title bar: the settings are saved, and the running connection takes them only after a
// restart the user asks for. Cancel leaves the chip.
const restart = useRestartNeeded()
const attempt = useAttempt()
const { t } = useI18n()
</script>

<template>
  <Dialog v-model:open="restart.dialogOpen" :title="t('restart.title')">
    <template #icon><RefreshCw class="text-blue" /></template>
    <p class="text-t2" data-testid="restart-text">{{ t('restart.text') }}</p>
    <template #footer>
      <Button variant="ghost" class="sp" @click="restart.dialogOpen = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" data-testid="restart-apply" @click="attempt(() => restart.apply())">
        <RefreshCw />{{ t('restart.apply') }}
      </Button>
    </template>
  </Dialog>
</template>
