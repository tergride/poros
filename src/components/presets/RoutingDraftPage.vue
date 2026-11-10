<script setup lang="ts">
import { Check } from '@lucide/vue'
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'

import { Button, Dialog } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { provideSettingsDraft } from '@/composables/useSettingsDraft'

// A tab of the routing made of settings, «General» and WARP: the page below changes a draft, which waits for «Save»,
// and leaving with a change asks first, as on the settings pages.
const attempt = useAttempt()
const { t } = useI18n()
const draft = provideSettingsDraft()

const saving = ref(false)
async function save(): Promise<boolean> {
  saving.value = true
  try {
    await draft.save()
    return true
  } catch (error) {
    await attempt(() => Promise.reject(error))
    return false
  } finally {
    saving.value = false
  }
}

// The question before leaving with changes: the answer resolves the navigation it holds.
const asking = ref<((go: boolean) => void) | null>(null)
function ask(): Promise<boolean> | boolean {
  if (!draft.dirty.value) return true
  return new Promise((resolve) => (asking.value = resolve))
}
function answer(choice: 'save' | 'leave' | 'stay'): void {
  const resolve = asking.value
  asking.value = null
  if (!resolve) return
  if (choice === 'leave') {
    draft.discard()
    resolve(true)
  } else if (choice === 'save') {
    void save().then(resolve)
  } else resolve(false)
}
onBeforeRouteLeave(ask)
onBeforeRouteUpdate(ask)
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="min-h-0 flex-1 overflow-auto px-7 pt-4 pb-7"><slot /></div>
    <div v-if="draft.dirty.value" class="flex flex-none items-center gap-3 border-t border-line bg-s2 px-7 py-2.5" role="status" data-testid="savebar">
      <span class="flex-1 text-t2">{{ t('adv.settings.unsaved') }}</span>
      <Button variant="primary" size="sm" :disabled="saving" data-testid="routing-save" @click="save()"><Check />{{ t('common.save') }}</Button>
    </div>
    <Dialog :open="asking !== null" :title="t('adv.unsaved.title')" @update:open="(o) => !o && answer('stay')">
      <p class="text-t2">{{ t('adv.unsaved.text') }}</p>
      <template #footer>
        <Button variant="ghost" class="sp" data-testid="unsaved-leave" @click="answer('leave')">{{ t('adv.unsaved.leave') }}</Button>
        <Button variant="primary" data-testid="unsaved-save" @click="answer('save')"><Check />{{ t('common.save') }}</Button>
      </template>
    </Dialog>
  </div>
</template>
