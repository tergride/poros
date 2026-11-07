<script setup lang="ts">
import { Check, ClipboardPaste, Info, LoaderCircle } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Input } from '@/components/ui'
import { useFailure } from '@/composables/useReason'
import type { SubscriptionUpdate } from '@/platform/core-types'
import { MethodServersList, MethodSubscriptionsAdd } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// The link field of the wizard and of the add dialog: it adds the subscription as soon as the link is in and says
// what came of it. A server is chosen if there was none.
// manual leaves the adding to the caller's button instead of doing it on paste and Enter.
const props = defineProps<{ large?: boolean; name?: string; manual?: boolean }>()
const emit = defineEmits<{ added: [update: SubscriptionUpdate] }>()

const core = useCoreStore()
const describe = useFailure()
const { t } = useI18n()
const link = ref('')
const busy = ref(false)
const added = ref<SubscriptionUpdate | null>(null)
const failure = ref('')
// The link tried last: leaving the field does not try it twice. Enter or Add try a failed one again.
let last = ''
let lastFailed = false

const servers = computed(() => (added.value ? t('servers.count', added.value.subscription.servers) : ''))

async function add(again = true): Promise<void> {
  const text = link.value.trim()
  if (!text || busy.value || (text === last && !(again && lastFailed))) return
  last = text
  lastFailed = false
  busy.value = true
  failure.value = ''
  added.value = null
  try {
    const update = await core.core().call(MethodSubscriptionsAdd, { url: text, name: props.name ?? '' })
    added.value = update
    if (!core.server) {
      const first = (await core.core().call(MethodServersList, { subscriptionId: update.subscription.id }))[0]
      if (first) await core.updateSettings({ server: first.id })
    }
    emit('added', update)
  } catch (error) {
    lastFailed = true
    failure.value = describe(error)
  } finally {
    busy.value = false
  }
}

async function paste(): Promise<void> {
  link.value = (await navigator.clipboard.readText()).trim()
  if (!props.manual) await add()
}

/** Puts a link in from outside, such as a poros:// link. */
function fill(text: string): void {
  link.value = text
}

defineExpose({ add, fill, busy, added })
</script>

<template>
  <div class="grid gap-3">
    <div class="flex gap-2">
      <Input
        v-model="link"
        mono
        :class="large ? 'h-[46px] text-sm' : ''"
        :aria-label="t('wizard.subscription.label')"
        placeholder="https://…"
        @keydown.enter="add()"
        @blur="manual || add(false)"
      />
      <Button :size="large ? 'lg' : 'default'" @click="paste"><ClipboardPaste />{{ t('common.paste') }}</Button>
    </div>
    <div v-if="busy" class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-3">
      <LoaderCircle class="size-4 animate-spin text-blue" /><span>{{ t('wizard.subscription.checking') }}</span>
    </div>
    <div v-else-if="added" class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-3" data-testid="link-found">
      <Check class="size-4 text-ok" />
      <span>{{ t('wizard.subscription.found', { servers, name: added.subscription.name }) }}</span>
    </div>
    <div v-else-if="failure" class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-3" data-testid="link-failure">
      <Info class="size-4 flex-none text-warn" /><span>{{ failure }}</span>
    </div>
  </div>
</template>
