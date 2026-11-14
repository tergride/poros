<script setup lang="ts">
import { Check, ClipboardPaste, Info, LoaderCircle, QrCode } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Input } from '@/components/ui'
import { useFailure } from '@/composables/useReason'
import { useScanScreen } from '@/composables/useScanScreen'
import { isCoreError } from '@/platform'
import type { ServersAdded, SubscriptionUpdate } from '@/platform/core-types'
import {
  GroupManual,
  MethodServersAdd,
  MethodServersList,
  MethodSubscriptionsAdd,
  ReasonSingleServer,
} from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// The link field of the wizard and of the add dialog: it adds the subscription as soon as the link is in and says
// what came of it. A server is chosen if there was none.
// manual leaves the adding to the caller's button instead of doing it on paste and Enter; scan reads the link from a QR
// code on the screen. servers adds a link to one server as a profile, to the first group of the user's or a new «My
// servers», instead of refusing it; singleHint is what the refusal says otherwise.
const props = defineProps<{
  large?: boolean
  name?: string
  manual?: boolean
  scan?: boolean
  servers?: boolean
  singleHint?: string
}>()
const emit = defineEmits<{ added: [update: SubscriptionUpdate]; server: [added: ServersAdded] }>()

const core = useCoreStore()
const describe = useFailure()
const scanScreen = useScanScreen()
const { t } = useI18n()
const link = ref('')
const busy = ref(false)
const added = ref<SubscriptionUpdate | null>(null)
const addedServer = ref<ServersAdded | null>(null)
const failure = ref('')
// The link tried last: leaving the field does not try it twice. Enter or Add try a failed one again.
let last = ''
let lastFailed = false

const servers = computed(() => (added.value ? t('servers.count', added.value.group.servers) : ''))

async function add(again = true): Promise<void> {
  const text = link.value.trim()
  if (!text || busy.value || (text === last && !(again && lastFailed))) return
  last = text
  lastFailed = false
  busy.value = true
  failure.value = ''
  added.value = null
  addedServer.value = null
  try {
    const update = await core.core().call(MethodSubscriptionsAdd, { url: text, name: props.name ?? '' })
    added.value = update
    if (!core.server) {
      const first = (await core.core().call(MethodServersList, { groupId: update.group.id }))[0]
      if (first) await core.updateSettings({ server: first.id })
    }
    emit('added', update)
  } catch (error) {
    const single = isCoreError(error) && error.data?.reason === ReasonSingleServer
    try {
      if (!single || !props.servers) throw error
      await addServer(text)
    } catch (refused) {
      lastFailed = true
      failure.value = single && !props.servers && props.singleHint ? props.singleHint : describe(refused)
    }
  } finally {
    busy.value = false
  }
}

async function addServer(text: string): Promise<void> {
  const own = core.groups.find((g) => g.kind === GroupManual)
  const result = await core
    .core()
    .call(MethodServersAdd, own ? { groupId: own.id, newGroup: '', text } : { groupId: '', newGroup: t('profiles.myServers'), text })
  addedServer.value = result
  if (!core.server && result.ids[0]) await core.updateSettings({ server: result.ids[0] })
  emit('server', result)
}

async function paste(): Promise<void> {
  link.value = (await navigator.clipboard.readText()).trim()
  if (!props.manual) await add()
}

// The first link of the QR codes on the screen, as if pasted.
async function fromScreen(): Promise<void> {
  if (busy.value) return
  failure.value = ''
  try {
    const found = (await scanScreen()).find((text) => /^[a-z][\w+.-]*:\/\//i.test(text.trim()))
    if (!found) {
      failure.value = t('qr.notFoundWizard')
      return
    }
    link.value = found.trim()
    if (!props.manual) await add()
  } catch (error) {
    failure.value = describe(error)
  }
}

/** Puts a link in from outside, such as a poros:// link. */
function fill(text: string): void {
  link.value = text
}

defineExpose({ add, fill, busy, added, addedServer })
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
      <Button
        v-if="scan"
        :size="large ? 'lg' : 'default'"
        :class="large ? 'w-[46px] px-0' : 'w-9 px-0'"
        :disabled="busy || !core.ready"
        :aria-label="t('qr.scanScreen')"
        :title="t('qr.scanScreen')"
        data-testid="link-scan"
        @click="fromScreen"
      >
        <QrCode />
      </Button>
    </div>
    <div v-if="busy" class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-3">
      <LoaderCircle class="size-4 animate-spin text-blue" /><span>{{ t('wizard.subscription.checking') }}</span>
    </div>
    <div v-else-if="added" class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-3" data-testid="link-found">
      <Check class="size-4 text-ok" />
      <span>{{ t('wizard.subscription.found', { servers, name: added.group.name }) }}</span>
    </div>
    <div v-else-if="addedServer" class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-3" data-testid="link-server">
      <Check class="size-4 text-ok" />
      <span>{{ t('wizard.subscription.server', { group: addedServer.group.name }) }}</span>
    </div>
    <div v-else-if="failure" class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-3" data-testid="link-failure">
      <Info class="size-4 flex-none text-warn" /><span>{{ failure }}</span>
    </div>
  </div>
</template>
