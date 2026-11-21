<script setup lang="ts">
import { Check, Eye, RefreshCw } from '@lucide/vue'
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import { Button, Dialog } from '@/components/ui'
import { useClock } from '@/composables/useClock'
import { useFailure } from '@/composables/useReason'
import { formatAgo } from '@/lib/format'
import { MethodSubscriptionsUpdate } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// A poros://addsub/ link to a subscription Poros has already. The add dialog would only refuse it; this one updates
// the subscription or shows it in the list of the mode, and both end at its group.
const core = useCoreStore()
const ui = useUiStore()
const router = useRouter()
const reason = useFailure()
const { t, locale } = useI18n()
const now = useClock(60_000)
const busy = ref(false)
const failure = ref('')

const group = computed(() => core.groups.find((g) => g.id === ui.knownSubscription))
const open = computed({
  get: () => ui.knownSubscription !== '',
  set: (shown: boolean) => {
    if (!shown) ui.knownSubscription = ''
  },
})

watch(open, () => (failure.value = ''))
// Removed meanwhile: there is nothing left to offer, and the next link must not wait for this dialog.
watch(group, (found) => {
  if (!found && ui.knownSubscription) ui.knownSubscription = ''
})

const summary = computed(() => {
  const g = group.value
  if (!g) return ''
  const servers = t('servers.count', g.servers)
  if (!g.updatedAt) return t('list.groupNever', { servers })
  return t('list.group', { servers, when: formatAgo(g.updatedAt, now.value, locale.value) })
})

async function show(id: string): Promise<void> {
  ui.knownSubscription = ''
  ui.focusGroup = id
  await router.push(core.settings?.uiMode === 'advanced' ? '/adv/profiles' : '/main')
  // A notice belongs to the screen it came from: the one set after this lands on the new screen.
  await nextTick()
}

async function update(): Promise<void> {
  const g = group.value
  if (!g || busy.value) return
  busy.value = true
  failure.value = ''
  try {
    const result = await core.core().call(MethodSubscriptionsUpdate, { id: g.id })
    await show(g.id)
    if (core.settings?.showChanges) {
      ui.notice = { tone: 'info', text: t('list.updated', { added: result.added, removed: result.removed, changed: result.changed }) }
    }
  } catch (error) {
    failure.value = reason(error)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('knownSubscription.title')">
    <div class="grid gap-3">
      <p class="text-t2">{{ t('knownSubscription.text', { name: group?.name ?? '' }) }}</p>
      <div class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-3" data-testid="known-summary">
        <Check class="size-4 flex-none text-ok" /><span>{{ summary }}</span>
      </div>
      <p v-if="failure" class="text-xs text-bad" data-testid="known-failure">{{ failure }}</p>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="open = false">{{ t('common.close') }}</Button>
      <Button :disabled="busy" data-testid="known-show" @click="group && show(group.id)"><Eye />{{ t('knownSubscription.show') }}</Button>
      <Button variant="primary" :disabled="busy" data-testid="known-update" @click="update">
        <RefreshCw :class="busy && 'animate-spin'" />{{ t('knownSubscription.update') }}
      </Button>
    </template>
  </Dialog>
</template>
