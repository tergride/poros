<script setup lang="ts">
import { Download, Gauge, LoaderCircle, Plus, RefreshCw, ScrollText, Search, X, Zap } from '@lucide/vue'
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import mark from '@/assets/img/mark.png'
import CoreBanner from '@/components/main/CoreBanner.vue'
import ServerGroup from '@/components/main/ServerGroup.vue'
import SubscriptionLink from '@/components/main/SubscriptionLink.vue'
import { Banner, Button, Dialog, Input } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useConnect } from '@/composables/useConnect'
import { useFailure, useReason } from '@/composables/useReason'
import { useUpdates } from '@/composables/useUpdates'
import type { Group } from '@/platform/core-types'
import {
  GroupManual,
  MethodGroupsRemove,
  MethodGroupsRename,
  MethodSubscriptionsUpdate,
  ReasonKillSwitchFailed,
  ReasonKillSwitchNeedsElevation,
} from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

const core = useCoreStore()
const ui = useUiStore()
const connect = useConnect()
const updates = useUpdates()
const reason = useReason()
const failure = useFailure()
const attempt = useAttempt()
const { t } = useI18n()

const query = ref('')
const closed = reactive(new Set<string>())
const updating = reactive(new Set<string>())
// The connection's failure stays until the next start, or until the user closes it.
const errorClosed = ref('')

// The offer tells what the restart takes away for a moment and what comes back by itself.
const updateText = computed(() =>
  [
    t('updates.ready', { version: core.updates.version }),
    core.killSwitch.on ? t('updates.killSwitch') : '',
    core.running ? t('updates.reconnect') : '',
  ]
    .filter(Boolean)
    .join(' '),
)

async function installUpdate(): Promise<void> {
  try {
    await updates.install()
  } catch (error) {
    ui.notice = { tone: 'warn', text: t('updates.installFailed', { reason: reason(error) }) }
  }
}

// The kill switch has banners of its own; a failed start for its sake is told there, not twice.
const killSwitchReasons = [ReasonKillSwitchNeedsElevation, ReasonKillSwitchFailed]
const killSwitchBroken = computed(() => core.killSwitch.error === ReasonKillSwitchFailed)
const connectionError = computed(
  () =>
    !!core.connection.error &&
    !connect.tunFailed &&
    !killSwitchReasons.includes(core.connection.error) &&
    errorClosed.value !== `${core.connection.error}:${core.connection.since}:${core.connection.errorText}`,
)
const connectionErrorText = computed(() =>
  failure({ code: 1, message: core.connection.errorText, data: { reason: core.connection.error } }),
)
function closeConnectionError(): void {
  errorClosed.value = `${core.connection.error}:${core.connection.since}:${core.connection.errorText}`
}

const groups = computed(() => {
  const needle = query.value.trim().toLowerCase()
  return core.groups
    .map((group) => ({
      group,
      servers: core.servers.filter((s) => s.groupId === group.id && (!needle || s.name.toLowerCase().includes(needle))),
    }))
    .filter((group) => !needle || group.servers.length > 0)
})

async function update(subscription: Group): Promise<void> {
  updating.add(subscription.id)
  ui.notice = null
  try {
    const result = await core.core().call(MethodSubscriptionsUpdate, { id: subscription.id })
    if (core.settings?.showChanges) {
      ui.notice = { tone: 'info', text: t('list.updated', { added: result.added, removed: result.removed, changed: result.changed }) }
    }
  } catch (error) {
    ui.notice = { tone: 'warn', text: t('list.failed', { name: subscription.name, reason: reason(error) }) }
  } finally {
    updating.delete(subscription.id)
  }
}

function copyLink(subscription: Group): Promise<void> {
  return attempt(async () => {
    await navigator.clipboard.writeText(subscription.url)
    ui.notice = { tone: 'info', text: t('list.copied') }
  })
}

const renaming = ref<Group | null>(null)
const newName = ref('')
// An empty name takes a subscription back to the provider's title; a group of the user's needs one.
const renamingManual = computed(() => renaming.value?.kind === GroupManual)
function startRename(group: Group): void {
  renaming.value = group
  newName.value = group.name
}
function rename(): Promise<void> {
  const group = renaming.value
  if (!group || (renamingManual.value && !newName.value.trim())) return Promise.resolve()
  renaming.value = null
  return attempt(() => core.core().call(MethodGroupsRename, { id: group.id, name: newName.value }))
}

const removing = ref<Group | null>(null)
function remove(): Promise<void> {
  const group = removing.value
  removing.value = null
  if (!group) return Promise.resolve()
  return attempt(() => core.core().call(MethodGroupsRemove, { id: group.id }))
}

function toggle(id: string): void {
  if (closed.has(id)) closed.delete(id)
  else closed.add(id)
}
</script>

<template>
  <main class="flex min-w-0 flex-1 flex-col bg-s1">
    <div v-if="core.groups.length === 0" class="flex flex-1 flex-col items-center justify-center gap-3.5 p-10 text-center">
      <div class="w-full max-w-[520px] text-left empty:hidden"><CoreBanner /></div>
      <img :src="mark" alt="" class="h-16 w-auto opacity-90" />
      <h2 class="font-disp text-xl font-normal">{{ t('list.emptyTitle') }}</h2>
      <p class="max-w-[400px] text-t2">{{ t('list.emptyText') }}</p>
      <div class="w-full max-w-[520px] text-left"><SubscriptionLink large /></div>
      <RouterLink to="/wizard/1" class="inline-flex items-center gap-2 text-t2 hover:text-t1">
        <Zap class="size-4" />{{ t('list.runWizard') }}
      </RouterLink>
    </div>

    <template v-else>
      <!-- A narrow window keeps the buttons' icons and gives the search its room. -->
      <div class="flex gap-2 border-b border-line px-4 py-3.5">
        <div class="relative min-w-[150px] flex-1">
          <Search class="absolute top-2.5 left-[11px] size-4 text-t3" />
          <Input v-model="query" class="pl-[33px]" :placeholder="t('list.search')" :aria-label="t('list.search')" />
        </div>
        <Button :disabled="core.testing || !core.ready" @click="attempt(() => core.testServers())">
          <LoaderCircle v-if="core.testing" class="animate-spin" /><Gauge v-else />
          <span class="max-[1079px]:sr-only">{{ core.testing ? t('list.testing') : t('list.test') }}</span>
        </Button>
        <Button @click="ui.addSubscription = true">
          <Plus /><span class="max-[1079px]:sr-only">{{ t('list.subscription') }}</span>
        </Button>
      </div>
      <div class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto px-4 pt-3 pb-4">
        <CoreBanner />
        <Banner v-if="connect.tunFailed" data-testid="tun-failed">
          {{ t('admin.failed') }}
          <template #actions>
            <Button size="sm" @click="attempt(() => connect.elevateAndConnect())">{{ t('admin.restart') }}</Button>
            <Button size="sm" @click="attempt(() => connect.useSystemInstead())">{{ t('admin.useSystemShort') }}</Button>
            <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="connect.tunFailed = false"><X /></Button>
          </template>
        </Banner>
        <Banner v-if="connect.killSwitchFailed && !connect.tunFailed" data-testid="kill-switch-failed">
          {{ t('admin.killSwitchFailed') }}
          <template #actions>
            <Button size="sm" @click="attempt(() => connect.elevateForKillSwitch())">{{ t('admin.restart') }}</Button>
            <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="connect.killSwitchFailed = false"><X /></Button>
          </template>
        </Banner>
        <Banner v-if="killSwitchBroken" data-testid="kill-switch-broken">
          {{ t('killSwitch.broken') }}
          <template #actions>
            <Button size="sm" @click="ui.logs = true">{{ t('menu.logs') }}</Button>
          </template>
        </Banner>
        <Banner v-if="connectionError" data-testid="connection-error">
          {{ connectionErrorText }}
          <template #actions>
            <Button size="sm" @click="ui.logs = true"><ScrollText />{{ t('menu.logs') }}</Button>
            <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="closeConnectionError()"><X /></Button>
          </template>
        </Banner>
        <Banner v-if="updates.offer" tone="info" data-testid="update-ready">
          <template #icon><Download class="size-4 flex-none text-blue" /></template>
          {{ updateText }}
          <template #actions>
            <Button size="sm" variant="primary" @click="installUpdate()"><RefreshCw />{{ t('updates.install') }}</Button>
            <Button size="sm" variant="ghost" @click="updates.putOff()">{{ t('common.later') }}</Button>
          </template>
        </Banner>
        <Banner v-if="updates.failure" data-testid="update-failed">
          {{ t(`updates.failed.${core.updates.installError}`, { version: core.updates.installVersion || '?' }) }}
          <template #actions>
            <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="updates.failureClosed = true"><X /></Button>
          </template>
        </Banner>
        <Banner v-if="ui.notice" :tone="ui.notice.tone" data-testid="notice">
          {{ ui.notice.text }}
          <template #actions>
            <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="ui.notice = null"><X /></Button>
          </template>
        </Banner>
        <ServerGroup
          v-for="group in groups"
          :key="group.group.id"
          :group="group.group"
          :servers="group.servers"
          :selected="core.settings?.server ?? ''"
          :open="!closed.has(group.group.id) || query !== ''"
          :updating="updating.has(group.group.id)"
          @toggle="toggle(group.group.id)"
          @pick="(id) => attempt(() => connect.chooseServer(id))"
          @update="update(group.group)"
          @test="attempt(() => core.testServers(group.group.id))"
          @copy-link="copyLink(group.group)"
          @rename="startRename(group.group)"
          @remove="removing = group.group"
        />
        <p v-if="groups.length === 0" class="py-6 text-center text-t3">{{ t('list.nothingFound') }}</p>
        <button
          type="button"
          class="flex h-[42px] flex-none cursor-pointer items-center justify-center gap-2 rounded-xl border border-dashed border-line2 text-t2 hover:border-blue hover:text-t1"
          @click="ui.addSubscription = true"
        >
          <Plus class="size-4" />{{ t('list.addAnother') }}
        </button>
      </div>
    </template>

    <Dialog
      :open="renaming !== null"
      :title="t(renamingManual ? 'list.renameGroupTitle' : 'list.renameTitle')"
      @update:open="(o) => !o && (renaming = null)"
    >
      <Input
        v-model="newName"
        :placeholder="t(renamingManual ? 'list.groupNamePlaceholder' : 'list.renamePlaceholder')"
        @keydown.enter="rename"
      />
      <template #footer>
        <Button variant="ghost" class="sp" @click="renaming = null">{{ t('common.cancel') }}</Button>
        <Button variant="primary" :disabled="renamingManual && !newName.trim()" @click="rename">{{ t('common.save') }}</Button>
      </template>
    </Dialog>

    <Dialog
      :open="removing !== null"
      :title="t(removing?.kind === GroupManual ? 'list.removeGroupTitle' : 'list.removeTitle')"
      @update:open="(o) => !o && (removing = null)"
    >
      <p class="text-t2">{{ t('list.removeText', { name: removing?.name ?? '' }) }}</p>
      <template #footer>
        <Button variant="ghost" class="sp" @click="removing = null">{{ t('common.cancel') }}</Button>
        <Button variant="danger" @click="remove">{{ t('common.remove') }}</Button>
      </template>
    </Dialog>
  </main>
</template>
