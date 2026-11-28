<script setup lang="ts">
import { Check } from '@lucide/vue'
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'

import { Button, Dialog } from '@/components/ui'
import { provideSettingsPage } from '@/composables/useSettingsBlock'

// A tab of the routing made of settings, «General» and WARP: as on the settings pages, a switch or a list applies at
// once, a card of typed values on its «Apply», and leaving with one not applied asks first.
const { t } = useI18n()
const settingsPage = provideSettingsPage()

// The question before leaving with changes not applied: the answer resolves the navigation it holds.
const asking = ref<((go: boolean) => void) | null>(null)
function ask(): Promise<boolean> | boolean {
  if (!settingsPage.dirty) return true
  return new Promise((resolve) => (asking.value = resolve))
}
function answer(choice: 'apply' | 'leave' | 'stay'): void {
  const resolve = asking.value
  asking.value = null
  if (!resolve) return
  if (choice === 'leave') {
    settingsPage.discard()
    resolve(true)
  } else if (choice === 'apply') {
    void settingsPage.apply().then(resolve)
  } else resolve(false)
}
onBeforeRouteLeave(ask)
onBeforeRouteUpdate(ask)
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="min-h-0 flex-1 overflow-auto px-7 pt-4 pb-7"><slot /></div>
    <Dialog :open="asking !== null" :title="t('adv.unapplied.title')" @update:open="(o) => !o && answer('stay')">
      <p class="text-t2">{{ t('adv.unapplied.text') }}</p>
      <template #footer>
        <Button variant="ghost" class="sp" data-testid="unsaved-leave" @click="answer('leave')">{{ t('adv.unsaved.leave') }}</Button>
        <Button variant="primary" data-testid="unsaved-apply" @click="answer('apply')"><Check />{{ t('settings.block.apply') }}</Button>
      </template>
    </Dialog>
  </div>
</template>
