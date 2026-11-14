<script setup lang="ts">
import { RefreshCw } from '@lucide/vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog } from '@/components/ui'
import { useUpdates } from '@/composables/useUpdates'
import { useCoreStore } from '@/stores/core'

// "Restart and update" while a connection runs: the restart drops it, and the traffic goes directly until Poros is
// back and connects again. Without a connection the update installs without this question.
const core = useCoreStore()
const updates = useUpdates()
const { t } = useI18n()
</script>

<template>
  <Dialog :open="updates.confirming" :title="t('updates.confirmTitle')" @update:open="(open) => open || updates.confirm(false)">
    <template #icon><RefreshCw class="text-blue" /></template>
    <div class="grid gap-2">
      <p class="text-t2" data-testid="update-confirm-text">{{ t('updates.confirmText', { version: core.updates.version }) }}</p>
      <p v-if="core.killSwitch.on" class="text-xs text-t3">{{ t('updates.confirmKillSwitch') }}</p>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="updates.confirm(false)">{{ t('common.cancel') }}</Button>
      <Button variant="primary" data-testid="update-confirm" @click="updates.confirm(true)">
        <RefreshCw />{{ t('updates.install') }}
      </Button>
    </template>
  </Dialog>
</template>
