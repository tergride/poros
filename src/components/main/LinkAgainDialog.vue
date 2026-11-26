<script setup lang="ts">
import { KeyRound, RefreshCw } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog, Input } from '@/components/ui'
import { useFailure } from '@/composables/useReason'
import { MethodGroupsConfigure, MethodSubscriptionsUpdate } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// A subscription whose link was saved on another computer: the link typed in again, then an update, which brings the
// servers back with their passwords and keys. An update that fails keeps the link and says why.
const core = useCoreStore()
const ui = useUiStore()
const describe = useFailure()
const { t } = useI18n()
const link = ref('')
const busy = ref(false)
const failure = ref('')

const group = computed(() => core.groups.find((g) => g.id === ui.linkAgain))
const open = computed({
  get: () => ui.linkAgain !== '',
  set: (shown: boolean) => {
    if (!shown) ui.linkAgain = ''
  },
})
watch(open, () => {
  link.value = ''
  failure.value = ''
})
// Removed meanwhile: there is nothing left to give a link to.
watch(group, (found) => {
  if (!found && ui.linkAgain) ui.linkAgain = ''
})

async function save(): Promise<void> {
  const g = group.value
  const url = link.value.trim()
  if (!g || !url || busy.value) return
  busy.value = true
  failure.value = ''
  try {
    await core.core().call(MethodGroupsConfigure, { id: g.id, url, settings: g.settings })
    const result = await core.core().call(MethodSubscriptionsUpdate, { id: g.id })
    ui.linkAgain = ''
    if (core.settings?.showChanges) {
      ui.notice = { tone: 'info', text: t('list.updated', { added: result.added, removed: result.removed, changed: result.changed }) }
    }
  } catch (error) {
    failure.value = describe(error)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('linkAgain.title')">
    <template #icon><KeyRound /></template>
    <div class="grid gap-3">
      <p class="text-t2">{{ t('linkAgain.text', { name: group?.name ?? '' }) }}</p>
      <Input v-model="link" mono placeholder="https://…" :aria-label="t('linkAgain.title')" data-testid="link-again-url" @keydown.enter="save()" />
      <p v-if="failure" class="text-xs text-bad" data-testid="link-again-failure">{{ failure }}</p>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="open = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="busy || !link.trim()" data-testid="link-again-save" @click="save()">
        <RefreshCw :class="busy && 'animate-spin'" />{{ t('linkAgain.save') }}
      </Button>
    </template>
  </Dialog>
</template>
