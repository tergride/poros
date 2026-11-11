<script setup lang="ts">
import {
  ChevronDown,
  ClipboardPaste,
  Copy,
  Download,
  FileUp,
  Folder,
  FolderInput,
  Gauge,
  Globe,
  Link,
  LoaderCircle,
  MoreHorizontal,
  Pencil,
  Play,
  Plus,
  QrCode,
  Radar,
  RefreshCw,
  RotateCcw,
  ScanLine,
  Search,
  Share2,
  ShieldAlert,
  Square,
  SquarePen,
  Trash2,
  TriangleAlert,
  X,
  Zap,
} from '@lucide/vue'
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import GroupEditor from '@/components/adv/GroupEditor.vue'
import NewGroupDialog from '@/components/adv/NewGroupDialog.vue'
import QrDialog from '@/components/adv/QrDialog.vue'
import SubscriptionQuota from '@/components/main/SubscriptionQuota.vue'
import { Button, ContextMenu, ContextMenuItem, ContextMenuSeparator, Dialog, Flag, Input, Menu, MenuItem, MenuSeparator, Select, Textarea } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useClock } from '@/composables/useClock'
import { useConnect } from '@/composables/useConnect'
import { useProfileActions } from '@/composables/useProfileActions'
import { useReason } from '@/composables/useReason'
import { useScanScreen } from '@/composables/useScanScreen'
import { useSelection } from '@/composables/useSelection'
import { formatAgo, formatBytes, latencyTone, protocolName, serverTitle } from '@/lib/format'
import type { Group, Server } from '@/platform/core-types'
import { GroupManual, MethodGroupsRemove, MethodServersClean, MethodServersLinks, MethodSubscriptionsUpdate } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// The profiles of the advanced mode: the groups on the left, the servers of one in a table selected as a list of
// Windows, with a menu on the right button and a bar for several rows.
const core = useCoreStore()
const ui = useUiStore()
const connect = useConnect()
const attempt = useAttempt()
const reason = useReason()
const actions = useProfileActions()
const scanScreen = useScanScreen()
const { t, locale } = useI18n()
const now = useClock(60_000)

const groupId = ref('')
const group = computed<Group | null>(() => core.groups.find((g) => g.id === groupId.value) ?? core.groups[0] ?? null)
watch(
  () => core.groups.map((g) => g.id),
  (ids) => {
    if (!ids.includes(groupId.value)) groupId.value = ids[0] ?? ''
  },
  { immediate: true },
)
const manual = computed(() => group.value?.kind === GroupManual)
const count = (id: string) => core.servers.filter((s) => s.groupId === id).length

const query = ref('')
const rows = computed(() => {
  const needle = query.value.trim().toLowerCase()
  return core.servers.filter(
    (s) => s.groupId === group.value?.id && (!needle || s.name.toLowerCase().includes(needle) || s.host.toLowerCase().includes(needle)),
  )
})
const selection = useSelection(computed(() => rows.value.map((s) => s.id)))
// A profile the search picked: its group, the row selected and in sight.
watch(
  () => ui.focusServer,
  async (id) => {
    const server = id ? core.servers.find((s) => s.id === id) : undefined
    if (!server) return
    ui.focusServer = ''
    groupId.value = server.groupId
    query.value = ''
    await nextTick()
    selection.click(id)
    document.querySelector(`[data-server-id="${id}"]`)?.scrollIntoView({ block: 'nearest' })
  },
  { immediate: true },
)
const picked = computed(() => rows.value.filter((s) => selection.isSelected(s.id)))
const one = computed(() => (picked.value.length === 1 ? picked.value[0]! : null))
watch(groupId, () => selection.clear())

const running = (s: Server) => core.running && core.settings?.server === s.id
const bytes = (value: number) => formatBytes(value, locale.value, (unit) => t(`units.${unit}`))
const toneClass = { good: 'text-ok', mid: 'text-warn', bad: 'text-bad', none: 'text-t3' }
function result(s: Server): string {
  const parts: string[] = []
  if (s.latency > 0) parts.push(t('ping.ms', { n: s.latency }))
  if (s.latency < 0) parts.push(t('ping.none'))
  if (s.speedDown) parts.push(`↓ ${s.speedDown}`)
  if (s.speedUp) parts.push(`↑ ${s.speedUp}`)
  if (s.exitIp) parts.push(s.exitIp)
  return parts.join(' · ')
}

const updatedText = computed(() => {
  const g = group.value
  if (!g || g.kind === GroupManual) return t('profiles.manualText')
  const host = (() => {
    try {
      return new URL(g.url).hostname
    } catch {
      return g.url
    }
  })()
  return g.updatedAt ? `${host} · ${t('profiles.updated', { when: formatAgo(g.updatedAt, now.value, locale.value) })}` : host
})

const updating = ref(false)
async function updateGroup(g: Group): Promise<void> {
  updating.value = true
  try {
    const r = await core.core().call(MethodSubscriptionsUpdate, { id: g.id })
    if (core.settings?.showChanges) ui.notice = { tone: 'info', text: t('list.updated', { added: r.added, removed: r.removed, changed: r.changed }) }
  } catch (error) {
    ui.notice = { tone: 'warn', text: t('list.failed', { name: g.name, reason: reason(error) }) }
  } finally {
    updating.value = false
  }
}
async function updateAll(): Promise<void> {
  for (const g of core.groups.filter((g) => g.kind !== GroupManual)) await updateGroup(g)
}

function clean(kind: 'unavailable' | 'duplicates' | 'insecure'): Promise<void> {
  return attempt(async () => {
    const { ids } = await core.core().call(MethodServersClean, {
      groupId: group.value?.id ?? '',
      unavailable: kind === 'unavailable',
      duplicates: kind === 'duplicates',
      insecure: kind === 'insecure',
    })
    ui.notice = { tone: 'info', text: t('profiles.cleaned', ids.length) }
  })
}

// The rows: a click as in Windows, a double click connects, the right button keeps the selection or takes the row.
function onRowClick(event: MouseEvent, s: Server): void {
  selection.click(s.id, { ctrl: event.ctrlKey || event.metaKey, shift: event.shiftKey })
}
function onKey(event: KeyboardEvent): void {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'a') {
    event.preventDefault()
    selection.selectAll()
  }
}

// Dialogs.
const editing = ref<Group | null>(null)
const newOpen = ref(false)
const importOpen = ref(false)
const importText = ref('')
const removingProfiles = ref<string[]>([])
const removingGroup = ref<Group | null>(null)
const moving = ref<string[]>([])
const moveTarget = ref('')
const moveNewName = ref('')
const newChoice = '\u0000new'
const manualGroups = computed(() => [
  ...core.groups.filter((g) => g.kind === GroupManual && g.id !== group.value?.id).map((g) => ({ value: g.id, label: g.name })),
  { value: newChoice, label: t('profiles.newGroupChoice') },
])
function startMove(ids: string[]): void {
  moving.value = ids
  moveTarget.value = manualGroups.value[0]!.value
  moveNewName.value = ''
}
async function move(): Promise<void> {
  const ids = moving.value
  moving.value = []
  let target = moveTarget.value
  if (target === newChoice) {
    const name = moveNewName.value.trim() || t('profiles.myServers')
    await actions.newGroup(name)
    target = core.groups.find((g) => g.kind === GroupManual && g.name === name)?.id ?? ''
    if (!target) return
  }
  await actions.moveTo(ids, target)
}

async function fromClipboard(): Promise<void> {
  const text = await navigator.clipboard.readText().catch(() => '')
  if (!text.trim()) {
    ui.notice = { tone: 'warn', text: t('profiles.clipboardEmpty') }
    return
  }
  await addFrom(text)
}
// The links of the QR codes on the screen go as from the clipboard.
const scanning = ref(false)
async function fromScreen(): Promise<void> {
  scanning.value = true
  try {
    await attempt(async () => {
      const texts = await scanScreen()
      if (texts.length === 0) ui.notice = { tone: 'warn', text: t('qr.notFound') }
      else await addFrom(texts.join('\n'))
    })
  } finally {
    scanning.value = false
  }
}

// A profile's link as a QR code: the same link as «Copy links».
const qr = ref<{ title: string; link: string; chip: string } | null>(null)
const qrOpen = computed({ get: () => qr.value !== null, set: (v: boolean) => !v && (qr.value = null) })
async function showQr(server: Server): Promise<void> {
  await attempt(async () => {
    const [link] = (await core.core().call(MethodServersLinks, { ids: [server.id] })).links
    qr.value = { title: serverTitle(server.name, server.country), link: link ?? '', chip: protocolName(server.protocol, t) }
  })
}

// A subscription's link goes to the dialog of subscriptions, anything else to the servers of a group.
async function addFrom(text: string): Promise<void> {
  const trimmed = text.trim()
  if (/^https?:\/\/\S+$/i.test(trimmed) && !trimmed.includes('\n')) {
    ui.addPrefill = { url: trimmed, name: '', check: true }
    ui.addSubscription = true
    return
  }
  await actions.addText(trimmed, group.value)
}
const fileInput = ref<HTMLInputElement | null>(null)
async function fromFiles(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement
  for (const file of input.files ?? []) await actions.addText(await file.text(), group.value)
  input.value = ''
}
async function importEntered(): Promise<void> {
  const text = importText.value
  importOpen.value = false
  importText.value = ''
  if (text.trim()) await addFrom(text)
}
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1">
    <aside class="flex w-[200px] flex-none flex-col border-r border-line bg-s0">
      <div class="flex items-center justify-between px-3 pt-3 pb-2 text-[11.5px] font-medium tracking-wide text-t3 uppercase">
        <span>{{ t('profiles.groups') }}</span>
        <Button variant="ghost" size="icon-sm" :aria-label="t('profiles.newTitle')" @click="newOpen = true"><Plus /></Button>
      </div>
      <div class="flex min-h-0 flex-1 flex-col gap-0.5 overflow-auto px-2">
        <button
          v-for="g in core.groups"
          :key="g.id"
          type="button"
          :class="[
            'flex min-h-9 cursor-pointer items-center gap-2 rounded-lg px-2.5 text-left text-[13.5px]',
            g.id === group?.id ? 'bg-s4 text-white' : 'text-t2 hover:bg-s3 hover:text-t1',
          ]"
          data-testid="group-item"
          @click="groupId = g.id"
        >
          <Folder v-if="g.kind === GroupManual" class="size-4 flex-none" /><Link v-else class="size-4 flex-none" />
          <span class="min-w-0 flex-1 truncate">{{ g.name }}</span>
          <span class="text-xs text-t3">{{ count(g.id) }}</span>
        </button>
      </div>
      <div class="p-2"><Button size="sm" class="w-full" data-testid="new-group" @click="newOpen = true"><Plus />{{ t('profiles.newButton') }}</Button></div>
    </aside>

    <section class="flex min-h-0 min-w-0 flex-1 flex-col">
      <template v-if="group">
        <div class="flex flex-none items-center gap-3 border-b border-line px-4 py-3">
          <div class="min-w-0 flex-1">
            <h2 class="truncate font-disp text-[17px] font-normal" data-testid="group-title">{{ group.name }}</h2>
            <small class="block truncate text-xs text-t3">{{ updatedText }}</small>
          </div>
          <!-- The header of a subscription: its quota and its end date, as in the simple mode. -->
          <SubscriptionQuota v-if="!manual" :group="group" class="w-[230px]" />
          <Button v-if="!manual" size="sm" :disabled="updating || !core.ready" @click="updateGroup(group)">
            <RefreshCw :class="updating && 'animate-spin'" />{{ t('profiles.update') }}
          </Button>
          <Button variant="ghost" size="icon-sm" :aria-label="t('profiles.editGroup')" data-testid="edit-group" @click="editing = group"><Pencil /></Button>
          <Menu align="end">
            <template #trigger>
              <Button variant="ghost" size="icon-sm" :aria-label="t('profiles.groupActions')"><MoreHorizontal /></Button>
            </template>
            <MenuItem v-if="!manual" @select="updateGroup(group)"><RefreshCw />{{ t('profiles.update') }}</MenuItem>
            <MenuItem @select="attempt(() => core.testServers(group!.id))"><Gauge />{{ t('list.test') }}</MenuItem>
            <MenuItem v-if="!manual" @select="attempt(() => actions.copyText(group!.url))"><Copy />{{ t('list.copyLink') }}</MenuItem>
            <MenuItem @select="editing = group"><SquarePen />{{ t('profiles.editGroupMenu') }}</MenuItem>
            <MenuSeparator />
            <MenuItem @select="updateAll()"><Radar />{{ t('profiles.updateAll') }}</MenuItem>
            <MenuSeparator />
            <MenuItem danger @select="removingGroup = group"><Trash2 />{{ t(manual ? 'list.removeGroup' : 'list.removeSubscription') }}</MenuItem>
          </Menu>
        </div>

        <div class="flex flex-none items-center gap-2 px-4 py-2.5">
          <div class="relative min-w-[150px] flex-1">
            <Search class="absolute top-2 left-[11px] size-4 text-t3" />
            <Input v-model="query" class="h-8 pl-[33px]" :placeholder="t('profiles.search')" :aria-label="t('profiles.search')" />
          </div>
          <Button variant="primary" size="sm" data-testid="new-profile" @click="actions.newProfile(group)"><Plus />{{ t('profiles.newProfile') }}</Button>
          <Menu align="end">
            <template #trigger>
              <Button size="sm"><Download />{{ t('profiles.import') }}<ChevronDown class="size-3.5" /></Button>
            </template>
            <MenuItem @select="fromClipboard()"><ClipboardPaste />{{ t('profiles.fromClipboard') }}</MenuItem>
            <MenuItem @select="fileInput?.click()"><FileUp />{{ t('profiles.fromFiles') }}</MenuItem>
            <MenuItem :disabled="scanning || !core.ready" data-testid="import-scan" @select="fromScreen()"><ScanLine />{{ t('qr.scan') }}</MenuItem>
            <MenuItem @select="importOpen = true"><SquarePen />{{ t('profiles.enterLink') }}</MenuItem>
          </Menu>
          <Menu align="end">
            <template #trigger>
              <Button size="sm"><LoaderCircle v-if="core.testing || core.speedTesting || core.ipTesting" class="animate-spin" /><Gauge v-else />{{ t('profiles.tests') }}<ChevronDown class="size-3.5" /></Button>
            </template>
            <MenuItem @select="attempt(() => core.testServers(group!.id))"><Gauge />{{ t('profiles.testLatencyGroup') }}</MenuItem>
            <MenuItem @select="attempt(() => core.speedTestServers(group!.id))"><Zap />{{ t('profiles.testSpeedGroup') }}</MenuItem>
            <MenuItem @select="attempt(() => core.ipTestServers(group!.id))"><Globe />{{ t('profiles.testIpGroup') }}</MenuItem>
            <MenuItem @select="actions.resolve(rows.map((s) => s.id))"><Search />{{ t('profiles.resolveGroup') }}</MenuItem>
            <MenuSeparator />
            <MenuItem @select="actions.clearResults(rows.map((s) => s.id))"><X />{{ t('profiles.clearResults') }}</MenuItem>
            <MenuItem @select="attempt(() => core.stopTests())"><Square />{{ t('profiles.stopTests') }}</MenuItem>
          </Menu>
          <Menu align="end">
            <template #trigger>
              <Button size="sm"><Trash2 />{{ t('profiles.clean') }}<ChevronDown class="size-3.5" /></Button>
            </template>
            <MenuItem @select="clean('unavailable')"><Trash2 />{{ t('profiles.cleanUnavailable') }}</MenuItem>
            <MenuItem @select="clean('duplicates')"><Copy />{{ t('profiles.cleanDuplicates') }}</MenuItem>
            <MenuItem @select="clean('insecure')"><TriangleAlert />{{ t('profiles.cleanInsecure') }}</MenuItem>
          </Menu>
          <input ref="fileInput" type="file" multiple class="hidden" accept=".txt,.json,.yaml,.yml,.conf,.ovpn" @change="fromFiles" />
        </div>

        <ContextMenu min-width="270px">
          <template #trigger>
            <div class="min-h-0 flex-1 overflow-auto px-4 pb-3 outline-none" tabindex="0" @keydown="onKey">
              <table class="w-full border-collapse text-[13px]" aria-multiselectable="true" data-testid="profiles-table">
                <thead class="sticky top-0 bg-s1 text-left text-xs text-t3">
                  <tr>
                    <th class="px-2.5 py-2 font-medium">{{ t('profiles.type') }}</th>
                    <th class="px-2.5 py-2 font-medium">{{ t('profiles.address') }}</th>
                    <th class="px-2.5 py-2 font-medium">{{ t('profiles.name') }}</th>
                    <th class="px-2.5 py-2 font-medium">{{ t('profiles.result') }}</th>
                    <th class="px-2.5 py-2 text-right font-medium">{{ t('profiles.traffic') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="s in rows"
                    :key="s.id"
                    :aria-selected="selection.isSelected(s.id)"
                    :class="[
                      'cursor-default border-t border-line select-none',
                      selection.isSelected(s.id) ? 'bg-s4' : 'hover:bg-s2',
                      running(s) && 'text-ok',
                    ]"
                    data-testid="profile-row"
                    :data-server-id="s.id"
                    @click="onRowClick($event, s)"
                    @dblclick="actions.start(s.id)"
                    @contextmenu="selection.contextOn(s.id)"
                  >
                    <td class="px-2.5 py-2 whitespace-nowrap">
                      {{ protocolName(s.protocol, t) }}
                      <span v-if="s.security" class="ml-1 inline-flex items-center gap-1 text-xs text-warn" :title="t(`profiles.security.${s.security}`)">
                        <ShieldAlert class="size-3.5" />{{ t(`profiles.securityShort.${s.security}`) }}
                      </span>
                    </td>
                    <td class="px-2.5 py-2 font-mono text-xs text-t2">
                      <template v-if="s.host">{{ s.host }}:{{ s.port }}</template><span v-else class="text-t3">—</span>
                    </td>
                    <td class="px-2.5 py-2">
                      <span class="flex min-w-0 items-center gap-2">
                        <Play v-if="running(s)" class="size-3.5 flex-none fill-current" />
                        <Flag v-if="s.country" :code="s.country" />
                        <span class="truncate">{{ serverTitle(s.name, s.country) }}</span>
                      </span>
                    </td>
                    <td :class="['px-2.5 py-2 whitespace-nowrap', toneClass[latencyTone(s.latency)]]">{{ result(s) || '—' }}</td>
                    <td class="px-2.5 py-2 text-right font-mono text-xs whitespace-nowrap text-t2">
                      {{ s.upload + s.download > 0 ? bytes(s.upload + s.download) : '—' }}
                    </td>
                  </tr>
                </tbody>
              </table>
              <p v-if="rows.length === 0" class="py-8 text-center text-t3">{{ query ? t('list.nothingFound') : t('profiles.empty') }}</p>
            </div>
          </template>
          <template v-if="picked.length > 1">
            <ContextMenuSeparator :label="t('profiles.selectedCount', { n: picked.length })" />
            <ContextMenuItem @select="attempt(() => core.testServers('', selection.selected.value))"><Gauge />{{ t('profiles.testLatency') }}</ContextMenuItem>
            <ContextMenuItem @select="attempt(() => core.speedTestServers('', selection.selected.value))"><Zap />{{ t('profiles.testSpeed') }}</ContextMenuItem>
            <ContextMenuItem @select="attempt(() => core.ipTestServers('', selection.selected.value))"><Globe />{{ t('profiles.testIp') }}</ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem @select="actions.copyLinks(selection.selected.value)"><Link />{{ t('profiles.copyLinks') }}</ContextMenuItem>
            <ContextMenuItem @select="actions.exportConfigs(selection.selected.value)"><Download />{{ t('profiles.exportConfigs') }}</ContextMenuItem>
            <ContextMenuItem @select="actions.clone(selection.selected.value)"><Copy />{{ t('profiles.clone') }}</ContextMenuItem>
            <ContextMenuItem @select="startMove(selection.selected.value)"><FolderInput />{{ t('profiles.moveTo') }}</ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem @select="actions.clearResults(selection.selected.value)"><X />{{ t('profiles.clearResults') }}</ContextMenuItem>
            <ContextMenuItem @select="actions.resetTraffic(selection.selected.value)"><RotateCcw />{{ t('profiles.resetTraffic') }}</ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem danger @select="removingProfiles = selection.selected.value"><Trash2 />{{ t('common.remove') }}</ContextMenuItem>
          </template>
          <template v-else-if="one">
            <ContextMenuItem :disabled="!core.ready" @select="actions.start(one!.id)"><Play />{{ t('profiles.start') }}</ContextMenuItem>
            <ContextMenuItem :disabled="!running(one)" @select="attempt(() => connect.stop())"><Square />{{ t('portal.stop') }}</ContextMenuItem>
            <ContextMenuItem data-testid="edit-profile" @select="actions.edit(one!.id)"><SquarePen />{{ t('profiles.edit') }}</ContextMenuItem>
            <ContextMenuItem @select="actions.clone([one!.id])"><Copy />{{ t('profiles.clone') }}</ContextMenuItem>
            <ContextMenuSeparator :label="t('profiles.share')" />
            <ContextMenuItem data-testid="profile-qr" @select="showQr(one!)"><QrCode />{{ t('qr.menu') }}</ContextMenuItem>
            <ContextMenuItem @select="actions.exportConfigs([one!.id])"><Download />{{ t('profiles.exportConfig') }}</ContextMenuItem>
            <ContextMenuItem @select="actions.copyLinks([one!.id])"><Link />{{ t('profiles.copyLinks') }}</ContextMenuItem>
            <ContextMenuItem @select="actions.copyDeepLinks([one!.id])"><Share2 />{{ t('profiles.copyDeepLink') }}</ContextMenuItem>
            <ContextMenuItem @select="actions.copyResult(one!)"><ClipboardPaste />{{ t('profiles.copyResult') }}</ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem @select="attempt(() => core.testServers('', [one!.id]))"><Gauge />{{ t('profiles.testLatency') }}</ContextMenuItem>
            <ContextMenuItem @select="attempt(() => core.speedTestServers('', [one!.id]))"><Zap />{{ t('profiles.testSpeed') }}</ContextMenuItem>
            <ContextMenuItem @select="attempt(() => core.ipTestServers('', [one!.id]))"><Globe />{{ t('profiles.testIp') }}</ContextMenuItem>
            <ContextMenuItem :disabled="!one.host" @select="actions.resolve([one!.id])"><Search />{{ t('profiles.resolve') }}</ContextMenuItem>
            <ContextMenuItem @select="actions.clearResults([one!.id])"><X />{{ t('profiles.clearResult') }}</ContextMenuItem>
            <ContextMenuItem @select="actions.resetTraffic([one!.id])"><RotateCcw />{{ t('profiles.resetTraffic') }}</ContextMenuItem>
            <ContextMenuItem @select="startMove([one!.id])"><FolderInput />{{ t('profiles.moveTo') }}</ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem danger @select="removingProfiles = [one!.id]"><Trash2 />{{ t('common.remove') }}</ContextMenuItem>
          </template>
        </ContextMenu>

        <div v-if="picked.length > 1" class="flex flex-none flex-wrap items-center gap-2 border-t border-line bg-s2 px-4 py-2" data-testid="bulk-bar">
          <span class="mr-auto text-t2">{{ t('profiles.selectedCount', { n: picked.length }) }}</span>
          <Button size="sm" @click="attempt(() => core.testServers('', selection.selected.value))"><Gauge />{{ t('profiles.latency') }}</Button>
          <Button size="sm" @click="attempt(() => core.speedTestServers('', selection.selected.value))"><Zap />{{ t('profiles.speed') }}</Button>
          <Button size="sm" @click="actions.copyLinks(selection.selected.value)"><Share2 />{{ t('profiles.shareShort') }}</Button>
          <Button size="sm" @click="actions.clone(selection.selected.value)"><Copy />{{ t('profiles.clone') }}</Button>
          <Button size="sm" @click="startMove(selection.selected.value)"><FolderInput />{{ t('profiles.toGroup') }}</Button>
          <Button size="sm" variant="danger" @click="removingProfiles = selection.selected.value"><Trash2 />{{ t('common.remove') }}</Button>
        </div>
      </template>
      <div v-else class="grid flex-1 place-items-center p-10 text-center text-t2">
        <div class="grid justify-items-center gap-3">
          <p>{{ t('profiles.noGroups') }}</p>
          <Button variant="primary" @click="newOpen = true"><Plus />{{ t('profiles.newButton') }}</Button>
        </div>
      </div>
    </section>

    <GroupEditor :group="editing" @close="editing = null" />
    <NewGroupDialog v-model:open="newOpen" />

    <Dialog v-model:open="importOpen" :title="t('profiles.enterLinkTitle')" class="w-[620px]">
      <template #icon><SquarePen /></template>
      <Textarea v-model="importText" mono class="h-[220px] resize-none" :placeholder="t('profiles.enterLinkPlaceholder')" data-testid="import-text" />
      <template #footer>
        <Button variant="ghost" class="sp" @click="importOpen = false">{{ t('common.cancel') }}</Button>
        <Button variant="primary" :disabled="!importText.trim()" data-testid="import-add" @click="importEntered()"><Plus />{{ t('common.add') }}</Button>
      </template>
    </Dialog>

    <Dialog :open="moving.length > 0" :title="t('profiles.moveTitle', moving.length)" @update:open="(open) => !open && (moving = [])">
      <template #icon><FolderInput /></template>
      <div class="grid gap-3">
        <p class="text-t2">{{ t('profiles.moveText') }}</p>
        <Select v-model="moveTarget" :options="manualGroups" :aria-label="t('profiles.group')" />
        <Input v-if="moveTarget === newChoice" v-model="moveNewName" :placeholder="t('list.groupNamePlaceholder')" />
      </div>
      <template #footer>
        <Button variant="ghost" class="sp" @click="moving = []">{{ t('common.cancel') }}</Button>
        <Button variant="primary" data-testid="move-apply" @click="move()"><FolderInput />{{ t('profiles.moveApply') }}</Button>
      </template>
    </Dialog>

    <Dialog :open="removingProfiles.length > 0" :title="t('profiles.removeTitle', removingProfiles.length)" @update:open="(open) => !open && (removingProfiles = [])">
      <p class="text-t2">{{ t(manual ? 'profiles.removeText' : 'profiles.removeTextSubscription') }}</p>
      <template #footer>
        <Button variant="ghost" class="sp" @click="removingProfiles = []">{{ t('common.cancel') }}</Button>
        <Button
          variant="danger"
          data-testid="remove-profiles"
          @click="
            () => {
              const ids = removingProfiles
              removingProfiles = []
              void actions.remove(ids)
            }
          "
        >
          {{ t('common.remove') }}
        </Button>
      </template>
    </Dialog>

    <Dialog
      :open="removingGroup !== null"
      :title="t(removingGroup?.kind === GroupManual ? 'list.removeGroupTitle' : 'list.removeTitle')"
      @update:open="(open) => !open && (removingGroup = null)"
    >
      <p class="text-t2">{{ t('list.removeText', { name: removingGroup?.name ?? '' }) }}</p>
      <template #footer>
        <Button variant="ghost" class="sp" @click="removingGroup = null">{{ t('common.cancel') }}</Button>
        <Button
          variant="danger"
          @click="
            () => {
              const g = removingGroup
              removingGroup = null
              if (g) void attempt(() => core.core().call(MethodGroupsRemove, { id: g.id }))
            }
          "
        >
          {{ t('common.remove') }}
        </Button>
      </template>
    </Dialog>
    <QrDialog v-model:open="qrOpen" :title="qr?.title ?? ''" :link="qr?.link ?? ''" :chip="qr?.chip" :note="t('qr.profileNote')" />
  </div>
</template>
