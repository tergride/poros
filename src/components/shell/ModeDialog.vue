<script setup lang="ts">
import { AppWindow, Layers } from '@lucide/vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useUiMode } from '@/composables/useUiMode'
import { useUiStore } from '@/stores/ui'

// From the chip of the title bar: what the other mode holds before the move. The connection runs on through it.
const ui = useUiStore()
const attempt = useAttempt()
const { mode, switchTo } = useUiMode()
const { t } = useI18n()
const toAdvanced = computed(() => mode.value === 'simple')

async function move(): Promise<void> {
  ui.modeDialog = false
  await attempt(() => switchTo(toAdvanced.value ? 'advanced' : 'simple'))
}
</script>

<template>
  <Dialog v-model:open="ui.modeDialog" :title="t(toAdvanced ? 'mode.toAdvancedTitle' : 'mode.toSimpleTitle')">
    <template #icon><Layers v-if="toAdvanced" class="text-blue" /><AppWindow v-else class="text-blue" /></template>
    <div class="grid gap-2.5">
      <p class="text-t2" data-testid="mode-text">{{ t(toAdvanced ? 'mode.toAdvancedText' : 'mode.toSimpleText') }}</p>
      <p class="text-xs text-t3">{{ t('mode.note') }}</p>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="ui.modeDialog = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" data-testid="mode-switch" @click="move()">
        {{ t(toAdvanced ? 'mode.toAdvanced' : 'mode.toSimple') }}
      </Button>
    </template>
  </Dialog>
</template>
