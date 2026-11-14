<script setup lang="ts">
import { TriangleAlert } from '@lucide/vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog } from '@/components/ui'
import { usePresetDraft } from '@/composables/usePresetDraft'

// Making a simple preset advanced keeps its rules but cannot be undone: an advanced preset has no simple form. The
// change waits for saving, as every other one.
const { draft, toAdvancedOpen } = usePresetDraft()
const { t } = useI18n()

function confirm(): void {
  if (draft.value) draft.value.advanced = true
  toAdvancedOpen.value = false
}
</script>

<template>
  <Dialog v-model:open="toAdvancedOpen" :title="t('toAdvanced.title')">
    <template #icon><TriangleAlert class="text-warn" /></template>
    <div class="grid gap-2">
      <p class="text-t2">{{ t('toAdvanced.text', { name: draft?.name.trim() ?? '' }) }}</p>
      <div class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-2.5 text-warn">
        <TriangleAlert class="size-4 flex-none" /><span>{{ t('toAdvanced.warn') }}</span>
      </div>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="toAdvancedOpen = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" data-testid="to-advanced-confirm" @click="confirm">{{ t('toAdvanced.button') }}</Button>
    </template>
  </Dialog>
</template>
