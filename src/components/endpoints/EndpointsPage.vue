<script setup lang="ts">
import { ArrowDown, ArrowUp, Check, Copy, Import, KeyRound, MoreHorizontal, Plus, Server, TextAlignStart, Trash2, Upload } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'

import EndpointAddMenu from '@/components/endpoints/EndpointAddMenu.vue'
import EndpointConnection from '@/components/endpoints/EndpointConnection.vue'
import EndpointExportDialog from '@/components/endpoints/EndpointExportDialog.vue'
import EndpointFillDialog from '@/components/endpoints/EndpointFillDialog.vue'
import EndpointImportDialog from '@/components/endpoints/EndpointImportDialog.vue'
import RuleDialog from '@/components/presets/RuleDialog.vue'
import RuleList from '@/components/presets/RuleList.vue'
import PresetListsDialog from '@/components/settings/PresetListsDialog.vue'
import { Banner, Button, Dialog, Input, Menu, MenuItem, MenuSeparator, Switch, Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { provideEndpointDraft } from '@/composables/useEndpointDraft'
import { useProfileForms } from '@/composables/useProfileForms'
import { useRestartNeeded } from '@/composables/useRestartNeeded'
import type { Endpoint, PresetRule } from '@/platform/core-types'
import { MethodEndpointsMove, MethodEndpointsRemove, MethodServersGet } from '@/platform/core-types'
import type { Profile } from '@/platform/profile-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// «Routing → Endpoints»: OpenVPN and OpenConnect connections of their own beside the server, with any preset. The
// list is their order, which decides when the rules of two match the same traffic; the editor beside it has the
// rules and the connection. A change waits for «Save», and leaving with one asks first, as for the presets.
const core = useCoreStore()
const ui = useUiStore()
const route = useRoute()
const router = useRouter()
const restart = useRestartNeeded()
const attempt = useAttempt()
const { forms, load: loadForms } = useProfileForms()
const { t } = useI18n()
const editor = provideEndpointDraft()
const { entry, failure, dirty, listsOpen, ruleOpen } = editor
const tab = ref('rules')
const importing = ref(false)
const exporting = ref(false)
const filling = ref(false)
const removing = ref(false)
// A new endpoint is a draft without an ID until it is saved.
const making = ref(false)

const selected = computed(() => String(route.params.id || core.endpoints[0]?.id || ''))
watch(
  selected,
  (id) => {
    if (making.value) return
    if (id) void editor.load(id)
    else entry.value = null
  },
  { immediate: true },
)
const current = computed<Endpoint | undefined>(() => core.endpoints.find((e) => e.id === entry.value?.id))
const index = computed(() => core.endpoints.findIndex((e) => e.id === entry.value?.id))
const needList = computed(() => (entry.value?.need ?? []).map((name) => t(`endpoints.needOf.${name}`)).join(', '))
const on = computed({
  get: () => entry.value?.on ?? false,
  set: (next: boolean) => {
    if (entry.value) entry.value.on = next
  },
})

function freeName(base: string): string {
  const taken = (name: string) => core.endpoints.some((e) => e.name.toLowerCase() === name.toLowerCase())
  let name = base
  for (let n = 2; taken(name); n++) name = `${base} ${n}`
  return name
}

function begin(profile: Profile): void {
  making.value = true
  tab.value = 'connection'
  editor.start({ id: '', on: false, profile, rules: [], need: [] })
}

async function add(type: string): Promise<void> {
  await attempt(async () => {
    await loadForms()
    const fresh = forms.value?.new.find((n) => n.profile.type === type)?.profile ?? ({ type, [type]: {} } as unknown as Profile)
    const profile = JSON.parse(JSON.stringify(fresh)) as Profile
    begin({ ...profile, name: freeName(type === 'openvpn' ? 'OpenVPN' : 'OpenConnect') })
  })
}

async function copy(serverId: string): Promise<void> {
  await attempt(async () => {
    const { profile } = await core.core().call(MethodServersGet, { id: serverId })
    begin({ ...profile, name: freeName(profile.name?.trim() || profile.server) })
  })
}

function duplicate(): void {
  if (!entry.value) return
  making.value = true
  editor.duplicate(freeName(t('presetEdit.copyName', { name: entry.value.profile.name ?? '' })))
}

async function imported(endpoint: Endpoint): Promise<void> {
  ui.notice = { tone: 'info', text: t('endpoints.imported', { name: endpoint.name }) }
  making.value = false
  await router.push(`/adv/routing/endpoints/${endpoint.id}`)
  if (endpoint.need.length > 0 && (await editor.load(endpoint.id))) filling.value = true
}

const saving = ref(false)
async function save(): Promise<boolean> {
  saving.value = true
  try {
    const saved = await editor.save()
    if (!saved) return false
    restart.endpointChanged(saved.id, saved.on)
    if (making.value) {
      making.value = false
      await router.push(`/adv/routing/endpoints/${saved.id}`)
    }
    return true
  } finally {
    saving.value = false
  }
}

async function move(offset: number): Promise<void> {
  const id = entry.value?.id
  if (!id) return
  await attempt(async () => {
    await core.core().call(MethodEndpointsMove, { id, offset })
    restart.endpointChanged(id, false)
  })
}

async function remove(): Promise<void> {
  removing.value = false
  const id = entry.value?.id
  if (!id) return
  await attempt(async () => {
    await core.core().call(MethodEndpointsRemove, { id })
    restart.endpointChanged(id, false)
    entry.value = null
    await router.push('/adv/routing/endpoints')
  })
}

// The lists window replaces the rules it has lines for; the endpoint is saved as before.
function applyLists(rules: PresetRule[]): void {
  if (editor.rules.draft.value) editor.rules.draft.value.rules = rules
}

// The question before leaving with changes: the answer resolves the navigation it holds.
const asking = ref<((go: boolean) => void) | null>(null)
function ask(): Promise<boolean> | boolean {
  if (!dirty.value) return true
  return new Promise((resolve) => (asking.value = resolve))
}
function answer(choice: 'save' | 'leave' | 'stay'): void {
  const resolve = asking.value
  asking.value = null
  if (!resolve) return
  if (choice === 'leave') {
    editor.discard()
    making.value = false
    resolve(true)
  } else if (choice === 'save') {
    void save().then(resolve)
  } else resolve(false)
}
onBeforeRouteLeave(ask)
onBeforeRouteUpdate(ask)
</script>

<template>
  <div class="flex min-h-0 flex-1">
    <aside class="flex w-[230px] flex-none flex-col border-r border-line bg-s1">
      <div class="px-3.5 pt-3 pb-1 text-[12.5px] font-semibold">{{ t('endpoints.title') }}</div>
      <nav class="min-h-0 flex-1 overflow-auto px-2 pb-2">
        <RouterLink
          v-for="e in core.endpoints"
          :key="e.id"
          :to="`/adv/routing/endpoints/${e.id}`"
          :class="[
            'flex min-h-9 items-center gap-2.5 rounded-lg px-2.5 text-[13px]',
            e.id === entry?.id && !making ? 'bg-s4 text-white' : 'text-t2 hover:bg-s3 hover:text-t1',
          ]"
          data-testid="routing-endpoint"
        >
          <Server class="size-4 flex-none" />
          <span class="min-w-0 flex-1 truncate">{{ e.name }}</span>
          <KeyRound v-if="e.need.length > 0" class="size-3.5 flex-none text-warn" :aria-label="t('endpoints.needShort')" />
          <span v-else class="text-[11.5px] text-t3">{{ t(e.on ? 'endpoints.onShort' : 'endpoints.offShort') }}</span>
        </RouterLink>
      </nav>
      <div class="grid gap-1.5 p-2 [&>button]:w-full">
        <EndpointAddMenu @add="add" @copy="copy" />
        <Button size="sm" data-testid="endpoint-import" @click="importing = true"><Import />{{ t('endpoints.import') }}</Button>
      </div>
    </aside>

    <div v-if="entry" class="flex min-w-0 flex-1 flex-col">
      <div class="flex flex-none flex-wrap items-center gap-2 border-b border-line px-5 py-2.5">
        <Input v-model="entry.profile.name" class="h-[30px] w-[220px] shrink-0" :aria-label="t('presetEdit.name')" data-testid="endpoint-name" />
        <span class="inline-flex h-[22px] items-center rounded-md bg-s4 px-2 text-[11.5px] text-t2">{{ entry.profile.type === 'openvpn' ? 'OpenVPN' : 'OpenConnect' }}</span>
        <label class="ml-1.5 flex items-center gap-2 text-[13px] text-t2" :title="entry.need.length > 0 ? t('endpoints.needShort') : undefined">
          <Switch v-model="on" :disabled="entry.need.length > 0" :aria-label="t('endpoints.on')" data-testid="endpoint-on" />{{ t('endpoints.on') }}
        </label>
        <span class="flex-1" />
        <Button size="sm" :disabled="!current" data-testid="endpoint-export" @click="exporting = true"><Upload />{{ t('endpoints.export') }}</Button>
        <Menu align="end" min-width="220px">
          <template #trigger>
            <Button variant="ghost" size="icon-sm" :aria-label="t('common.more')" data-testid="endpoint-menu"><MoreHorizontal /></Button>
          </template>
          <MenuItem :disabled="!current || index <= 0" @select="move(-1)"><ArrowUp />{{ t('endpoints.up') }}</MenuItem>
          <MenuItem :disabled="!current || index >= core.endpoints.length - 1" @select="move(1)"><ArrowDown />{{ t('endpoints.down') }}</MenuItem>
          <MenuItem :disabled="!current" data-testid="endpoint-duplicate" @select="duplicate"><Copy />{{ t('endpoints.duplicate') }}</MenuItem>
          <MenuSeparator />
          <MenuItem danger :disabled="!current" data-testid="endpoint-remove" @select="removing = true"><Trash2 />{{ t('common.remove') }}</MenuItem>
        </Menu>
      </div>
      <div class="min-h-0 flex-1 overflow-auto px-7 pb-7">
        <Tabs v-model="tab">
          <TabsList class="-mx-7 mb-3.5 px-5">
            <TabsTrigger value="rules" data-testid="endpoint-tab-rules">{{ t('endpoints.tabRules') }}</TabsTrigger>
            <TabsTrigger value="connection" data-testid="endpoint-tab-connection">{{ t('endpoints.tabConnection') }}</TabsTrigger>
          </TabsList>
          <Banner v-if="failure" class="mb-3" data-testid="endpoint-failure">{{ failure }}</Banner>
          <div v-if="entry.need.length > 0" class="mb-3 flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-2.5" role="status" data-testid="endpoint-need">
            <KeyRound class="size-4 flex-none text-warn" /><span class="flex-1">{{ t('endpoints.needBanner', { list: needList }) }}</span>
            <Button size="sm" data-testid="endpoint-fill" @click="filling = true">{{ t('endpoints.fill') }}</Button>
          </div>
          <TabsContent value="rules">
            <div class="mb-2.5 flex justify-end gap-2">
              <Button size="sm" data-testid="rule-new" @click="ruleOpen = -1"><Plus />{{ t('rules.withConditions') }}</Button>
              <Button size="sm" data-testid="endpoint-lists" @click="listsOpen = true"><TextAlignStart />{{ t('presetLists.open') }}</Button>
            </div>
            <RuleList advanced />
          </TabsContent>
          <TabsContent value="connection">
            <EndpointConnection />
          </TabsContent>
        </Tabs>
      </div>
      <div v-if="dirty" class="flex flex-none items-center gap-3 border-t border-line bg-s2 px-7 py-2.5" role="status" data-testid="savebar">
        <span class="flex-1 text-t2">{{ t('endpoints.unsaved', { name: entry.profile.name?.trim() ?? '' }) }}</span>
        <Button variant="primary" size="sm" :disabled="saving" data-testid="endpoint-save" @click="save()"><Check />{{ t('common.save') }}</Button>
      </div>
    </div>
    <div v-else class="grid min-w-0 flex-1 content-center justify-items-center gap-3.5 p-10 text-center" data-testid="endpoints-empty">
      <Server class="size-10 text-t3" />
      <h2 class="font-disp text-[20px] font-normal">{{ t('endpoints.empty') }}</h2>
      <p class="max-w-[420px] text-t2">{{ t('endpoints.emptyText') }}</p>
      <div class="flex gap-2">
        <EndpointAddMenu @add="add" @copy="copy" />
        <Button size="sm" @click="importing = true"><Import />{{ t('endpoints.importFile') }}</Button>
      </div>
    </div>

    <PresetListsDialog
      v-if="editor.rules.draft.value"
      v-model:open="listsOpen"
      :name="entry?.profile.name ?? ''"
      advanced
      endpoint
      :rules="editor.rules.draft.value.rules"
      @apply="applyLists"
    />
    <RuleDialog />
    <EndpointImportDialog v-model:open="importing" @added="imported" />
    <EndpointExportDialog v-model:open="exporting" :endpoint="current ?? null" />
    <EndpointFillDialog v-model:open="filling" />
    <Dialog :open="removing" :title="t('endpoints.removeTitle')" @update:open="(o) => !o && (removing = false)">
      <p class="text-t2">{{ t('endpoints.removeText', { name: entry?.profile.name ?? '' }) }}</p>
      <template #footer>
        <Button variant="ghost" class="sp" @click="removing = false">{{ t('common.cancel') }}</Button>
        <Button variant="danger" data-testid="endpoint-remove-confirm" @click="remove">{{ t('common.remove') }}</Button>
      </template>
    </Dialog>
    <Dialog :open="asking !== null" :title="t('adv.unsaved.title')" @update:open="(o) => !o && answer('stay')">
      <p class="text-t2">{{ t('adv.unsaved.text') }}</p>
      <template #footer>
        <Button variant="ghost" class="sp" data-testid="unsaved-leave" @click="answer('leave')">{{ t('adv.unsaved.leave') }}</Button>
        <Button variant="primary" data-testid="unsaved-save" @click="answer('save')"><Check />{{ t('common.save') }}</Button>
      </template>
    </Dialog>
  </div>
</template>
