<script setup lang="ts">
import { Download, Gauge, LoaderCircle, Plus, RefreshCw, Search, X, Zap } from '@lucide/vue'
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import mark from '@/assets/img/mark.png'
import ServerGroup from '@/components/main/ServerGroup.vue'
import SubscriptionLink from '@/components/main/SubscriptionLink.vue'
import { Banner, Button, Dialog, Input } from '@/components/ui'
import { useConnect } from '@/composables/useConnect'
import { useReason } from '@/composables/useReason'
import { useUpdates } from '@/composables/useUpdates'
import type { Subscription } from '@/platform/core-types'
import {
  MethodSubscriptionsRemove,
  MethodSubscriptionsRename,
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
const { t } = useI18n()

const query = ref('')
const closed = reactive(new Set<string>())
const updating = reactive(new Set<string>())
const notice = ref<{ tone: 'info' | 'warn'; text: string } | null>(null)

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
    notice.value = { tone: 'warn', text: t('updates.installFailed', { reason: reason(error) }) }
  }
}

// The kill switch has banners of its own; a failed start for its sake is told there, not twice.
const killSwitchReasons = [ReasonKillSwitchNeedsElevation, ReasonKillSwitchFailed]
const killSwitchBroken = computed(() => core.killSwitch.error === ReasonKillSwitchFailed)
const connectionError = computed(
  () => !!core.connection.error && !connect.tunFailed && !killSwitchReasons.includes(core.connection.error),
)

const groups = computed(() => {
  const needle = query.value.trim().toLowerCase()
  return core.subscriptions
    .map((subscription) => ({
      subscription,
      servers: core.servers.filter(
        (s) => s.subscriptionId === subscription.id && (!needle || s.name.toLowerCase().includes(needle)),
      ),
    }))
    .filter((group) => !needle || group.servers.length > 0)
})

async function update(subscription: Subscription): Promise<void> {
  updating.add(subscription.id)
  notice.value = null
  try {
    const result = await core.core().call(MethodSubscriptionsUpdate, { id: subscription.id })
    if (core.settings?.showChanges) {
      notice.value = { tone: 'info', text: t('list.updated', { added: result.added, removed: result.removed, changed: result.changed }) }
    }
  } catch (error) {
    notice.value = { tone: 'warn', text: t('list.failed', { reason: reason(error) }) }
  } finally {
    updating.delete(subscription.id)
  }
}

async function copyLink(subscription: Subscription): Promise<void> {
  await navigator.clipboard.writeText(subscription.url)
}

const renaming = ref<Subscription | null>(null)
const newName = ref('')
function startRename(subscription: Subscription): void {
  renaming.value = subscription
  newName.value = subscription.name
}
async function rename(): Promise<void> {
  if (!renaming.value) return
  await core.core().call(MethodSubscriptionsRename, { id: renaming.value.id, name: newName.value })
  renaming.value = null
}

const removing = ref<Subscription | null>(null)
async function remove(): Promise<void> {
  if (!removing.value) return
  await core.core().call(MethodSubscriptionsRemove, { id: removing.value.id })
  removing.value = null
}

function toggle(id: string): void {
  if (closed.has(id)) closed.delete(id)
  else closed.add(id)
}
</script>

<template>
  <main class="flex min-w-0 flex-1 flex-col bg-s1">
    <div v-if="core.subscriptions.length === 0" class="flex flex-1 flex-col items-center justify-center gap-3.5 p-10 text-center">
      <img :src="mark" alt="" class="h-16 w-auto opacity-90" />
      <h2 class="font-disp text-xl font-normal">{{ t('list.emptyTitle') }}</h2>
      <p class="max-w-[400px] text-t2">{{ t('list.emptyText') }}</p>
      <div class="w-full max-w-[520px] text-left"><SubscriptionLink large /></div>
      <RouterLink to="/wizard/1" class="inline-flex items-center gap-2 text-t2 hover:text-t1">
        <Zap class="size-4" />{{ t('list.runWizard') }}
      </RouterLink>
    </div>

    <template v-else>
      <div class="flex gap-2 border-b border-line px-4 py-3.5">
        <div class="relative min-w-0 flex-1">
          <Search class="absolute top-2.5 left-[11px] size-4 text-t3" />
          <Input v-model="query" class="pl-[33px]" :placeholder="t('list.search')" :aria-label="t('list.search')" />
        </div>
        <Button :disabled="core.testing" @click="core.testServers()">
          <LoaderCircle v-if="core.testing" class="animate-spin" /><Gauge v-else />
          {{ core.testing ? t('list.testing') : t('list.test') }}
        </Button>
        <Button @click="ui.addSubscription = true"><Plus />{{ t('list.subscription') }}</Button>
      </div>
      <div class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto px-4 pt-3 pb-4">
        <Banner v-if="connect.tunFailed" data-testid="tun-failed">
          {{ t('admin.failed') }}
          <template #actions>
            <Button size="sm" @click="connect.elevateAndConnect()">{{ t('admin.restart') }}</Button>
            <Button size="sm" @click="connect.useSystemInstead()">{{ t('admin.useSystemShort') }}</Button>
            <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="connect.tunFailed = false"><X /></Button>
          </template>
        </Banner>
        <Banner v-if="connect.killSwitchFailed && !connect.tunFailed" data-testid="kill-switch-failed">
          {{ t('admin.killSwitchFailed') }}
          <template #actions>
            <Button size="sm" @click="connect.elevateForKillSwitch()">{{ t('admin.restart') }}</Button>
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
          {{ reason({ code: 1, message: core.connection.errorText, data: { reason: core.connection.error } }) }}
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
        <Banner v-if="notice" :tone="notice.tone">
          {{ notice.text }}
          <template #actions>
            <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="notice = null"><X /></Button>
          </template>
        </Banner>
        <ServerGroup
          v-for="group in groups"
          :key="group.subscription.id"
          :subscription="group.subscription"
          :servers="group.servers"
          :selected="core.settings?.server ?? ''"
          :open="!closed.has(group.subscription.id) || query !== ''"
          :updating="updating.has(group.subscription.id)"
          @toggle="toggle(group.subscription.id)"
          @pick="(id) => connect.chooseServer(id)"
          @update="update(group.subscription)"
          @test="core.testServers(group.subscription.id)"
          @copy-link="copyLink(group.subscription)"
          @rename="startRename(group.subscription)"
          @remove="removing = group.subscription"
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

    <Dialog :open="renaming !== null" :title="t('list.renameTitle')" @update:open="(o) => !o && (renaming = null)">
      <Input v-model="newName" :placeholder="t('list.renamePlaceholder')" @keydown.enter="rename" />
      <template #footer>
        <Button variant="ghost" class="sp" @click="renaming = null">{{ t('common.cancel') }}</Button>
        <Button variant="primary" @click="rename">{{ t('common.save') }}</Button>
      </template>
    </Dialog>

    <Dialog :open="removing !== null" :title="t('list.removeTitle')" @update:open="(o) => !o && (removing = null)">
      <p class="text-t2">{{ t('list.removeText', { name: removing?.name ?? '' }) }}</p>
      <template #footer>
        <Button variant="ghost" class="sp" @click="removing = null">{{ t('common.cancel') }}</Button>
        <Button variant="danger" @click="remove">{{ t('common.remove') }}</Button>
      </template>
    </Dialog>
  </main>
</template>
