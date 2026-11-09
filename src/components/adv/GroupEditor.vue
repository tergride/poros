<script setup lang="ts">
import { Pencil } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Banner, Button, Checkbox, Dialog, Input, Select, Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui'
import { useProfileActions } from '@/composables/useProfileActions'
import { useReason } from '@/composables/useReason'
import { useRestartNeeded } from '@/composables/useRestartNeeded'
import type { Group, GroupSettings } from '@/platform/core-types'
import { GroupManual, MethodGroupsConfigure, MethodGroupsRename } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// A group as a whole: its name and link, the proxies around its servers, how a subscription asks and updates.
const props = defineProps<{ group: Group | null }>()
const emit = defineEmits<{ close: [] }>()

const core = useCoreStore()
const actions = useProfileActions()
const reason = useReason()
const restart = useRestartNeeded()
const { t } = useI18n()

const tab = ref('base')
const name = ref('')
const url = ref('')
const settings = ref<GroupSettings | null>(null)
const failure = ref('')
const saving = ref(false)
watch(
  () => props.group,
  (group) => {
    if (!group) return
    tab.value = 'base'
    name.value = group.name
    url.value = group.url
    settings.value = { ...group.settings }
    failure.value = ''
  },
  { immediate: true },
)
const manual = computed(() => props.group?.kind === GroupManual)

// The proxies are servers of the other groups, by name and group.
const proxies = computed(() => [
  { value: '', label: t('groupEditor.none') },
  ...core.servers
    .filter((s) => s.groupId !== props.group?.id && s.protocol !== 'chain' && s.protocol !== 'direct')
    .map((s) => ({ value: s.id, label: `${s.name} · ${core.groups.find((g) => g.id === s.groupId)?.name ?? ''}` })),
])
const versions = (choices: string[]) => [
  { value: '', label: t('groupEditor.asSettings') },
  // The keys of the versions hold no dots: vue-i18n reads a dot as a level of the key.
  ...choices.map((value) => ({ value, label: t(`groupEditor.version.${value.replace('.', '_')}`) })),
]
const deviceChoices = computed(() => [
  { value: '', label: t('groupEditor.asSettings') },
  { value: 'on', label: t('groupEditor.on') },
  { value: 'off', label: t('groupEditor.off') },
])

// How often, as one choice: the settings of all subscriptions, the provider, a few fixed intervals, one of the
// user's, or never.
const fixedHours = [1, 6, 24]
const frequencies = computed(() => [
  { value: '', label: t('groupEditor.frequency.default') },
  { value: 'provider', label: t('groupEditor.frequency.provider') },
  ...fixedHours.map((h) => ({ value: String(h), label: t(`groupEditor.frequency.h${h}`) })),
  { value: 'custom', label: t('groupEditor.frequency.custom') },
  { value: 'never', label: t('groupEditor.frequency.never') },
])
const customHours = ref('12')
const hoursOf = (text: string) => Math.min(168, Math.max(1, Math.round(Number(text)) || 1))
const frequency = computed({
  get: () => {
    const s = settings.value
    if (!s || s.update === '') return ''
    if (s.update !== 'hours') return s.update
    return fixedHours.includes(s.updateHours) ? String(s.updateHours) : 'custom'
  },
  set: (choice: string) => {
    const s = settings.value
    if (!s) return
    if (choice === '' || choice === 'provider' || choice === 'never') {
      s.update = choice
      s.updateHours = 0
    } else {
      s.update = 'hours'
      s.updateHours = choice === 'custom' ? hoursOf(customHours.value) : Number(choice)
    }
  },
})
watch(customHours, (hours) => {
  if (frequency.value === 'custom' && settings.value) settings.value.updateHours = hoursOf(hours)
})
// The actions by a test's results need the test.
watch(
  () => settings.value?.test,
  (test) => {
    if (!test && settings.value) settings.value.removeUnavailable = settings.value.sortByLatency = false
  },
)

const groupIds = computed(() => core.servers.filter((s) => s.groupId === props.group?.id).map((s) => s.id))

async function save(): Promise<void> {
  const group = props.group
  const next = settings.value
  if (!group || !next) return
  saving.value = true
  failure.value = ''
  try {
    if (name.value.trim() !== group.name) await core.core().call(MethodGroupsRename, { id: group.id, name: name.value })
    // A group of the user's takes only the address and the proxies.
    const sent = manual.value
      ? { ...group.settings, serverAddress: next.serverAddress, frontProxy: next.frontProxy, landingProxy: next.landingProxy }
      : next
    await core.core().call(MethodGroupsConfigure, { id: group.id, url: manual.value ? '' : url.value.trim(), settings: sent })
    // The address and the proxies of a group are part of a connection through its servers.
    const before = group.settings
    if (next.serverAddress !== before.serverAddress || next.frontProxy !== before.frontProxy || next.landingProxy !== before.landingProxy) {
      groupIds.value.forEach((id) => restart.serverSaved(id))
    }
    emit('close')
  } catch (error) {
    failure.value = reason(error)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Tabs v-model="tab">
    <Dialog :open="group !== null" :title="t('groupEditor.title')" class="h-[560px] w-[720px]" @update:open="(open) => !open && emit('close')">
      <template #icon><Pencil /></template>
      <template #tabs>
        <TabsList class="px-3">
          <TabsTrigger value="base">{{ t('groupEditor.tab.base') }}</TabsTrigger>
          <TabsTrigger value="request">{{ t(manual ? 'groupEditor.tab.address' : 'groupEditor.tab.request') }}</TabsTrigger>
          <TabsTrigger v-if="!manual" value="update">{{ t('groupEditor.tab.update') }}</TabsTrigger>
        </TabsList>
      </template>
      <template v-if="settings">
        <TabsContent value="base" class="grid grid-cols-2 gap-3 pt-3">
          <label class="grid gap-1.5">
            <span class="text-xs text-t3">{{ t('groupEditor.name') }}</span>
            <Input v-model="name" data-testid="group-name" />
          </label>
          <label v-if="!manual" class="col-span-2 grid gap-1.5">
            <span class="text-xs text-t3">{{ t('groupEditor.url') }}</span>
            <Input v-model="url" class="font-mono" data-testid="group-url" />
          </label>
          <label class="grid gap-1.5">
            <span class="text-xs text-t3">{{ t('groupEditor.landing') }}</span>
            <Select v-model="settings.landingProxy" :options="proxies" :aria-label="t('groupEditor.landing')" />
          </label>
          <label class="grid gap-1.5">
            <span class="text-xs text-t3">{{ t('groupEditor.front') }}</span>
            <Select v-model="settings.frontProxy" :options="proxies" :aria-label="t('groupEditor.front')" />
          </label>
          <p class="col-span-2 text-xs text-t3">{{ t('groupEditor.proxiesHint') }}</p>
          <div class="col-span-2 flex flex-wrap gap-2">
            <Button size="sm" :disabled="groupIds.length === 0" @click="actions.copyLinks(groupIds)">{{ t('groupEditor.copyLinks') }}</Button>
            <Button size="sm" :disabled="groupIds.length === 0" @click="actions.copyDeepLinks(groupIds)">{{ t('groupEditor.copyDeepLinks') }}</Button>
          </div>
        </TabsContent>
        <TabsContent value="request" class="grid grid-cols-2 gap-3 pt-3">
          <label class="col-span-2 grid gap-1.5">
            <span class="text-xs text-t3">{{ t('groupEditor.address') }}</span>
            <Input v-model="settings.serverAddress" class="font-mono" :placeholder="t('groupEditor.addressPlaceholder')" data-testid="group-address" />
            <span class="text-xs text-t3">{{ t('groupEditor.addressHint') }}</span>
          </label>
          <template v-if="!manual">
            <label class="col-span-2 grid gap-1.5">
              <span class="text-xs text-t3">User Agent</span>
              <Input v-model="settings.userAgent" :placeholder="t('groupEditor.asSettings')" />
            </label>
            <label class="grid gap-1.5">
              <span class="text-xs text-t3">{{ t('groupEditor.tls') }}</span>
              <Select v-model="settings.tls" :options="versions(['auto', '1.2', '1.3'])" :aria-label="t('groupEditor.tls')" />
            </label>
            <label class="grid gap-1.5">
              <span class="text-xs text-t3">{{ t('groupEditor.http') }}</span>
              <Select v-model="settings.http" :options="versions(['auto', '1.1'])" :aria-label="t('groupEditor.http')" />
            </label>
            <label class="grid gap-1.5">
              <span class="text-xs text-t3">{{ t('groupEditor.deviceInfo') }}</span>
              <Select v-model="settings.deviceInfo" :options="deviceChoices" :aria-label="t('groupEditor.deviceInfo')" />
            </label>
            <label class="grid gap-1.5">
              <span class="text-xs text-t3">HWID</span>
              <Input v-model="settings.deviceId" class="font-mono" :placeholder="t('groupEditor.poros')" />
            </label>
            <label class="grid gap-1.5">
              <span class="text-xs text-t3">OS</span>
              <Input v-model="settings.os" :placeholder="t('groupEditor.poros')" />
            </label>
            <label class="grid gap-1.5">
              <span class="text-xs text-t3">OS Version</span>
              <Input v-model="settings.osVersion" :placeholder="t('groupEditor.poros')" />
            </label>
            <label class="grid gap-1.5">
              <span class="text-xs text-t3">Device Model</span>
              <Input v-model="settings.deviceModel" />
            </label>
          </template>
        </TabsContent>
        <TabsContent v-if="!manual" value="update" class="grid gap-3 pt-3">
          <div class="flex items-end gap-3">
            <label class="grid flex-1 gap-1.5">
              <span class="text-xs text-t3">{{ t('groupEditor.frequency.title') }}</span>
              <Select v-model="frequency" :options="frequencies" :aria-label="t('groupEditor.frequency.title')" />
            </label>
            <label v-if="frequency === 'custom'" class="grid w-[140px] gap-1.5">
              <span class="text-xs text-t3">{{ t('groupEditor.frequency.hours') }}</span>
              <Input v-model="customHours" type="number" min="1" max="168" />
            </label>
          </div>
          <label class="flex items-start gap-2.5">
            <Checkbox v-model="settings.keepWorking" /><span>{{ t('groupEditor.keepWorking') }}<br /><span class="text-xs text-t3">{{ t('groupEditor.keepWorkingHint') }}</span></span>
          </label>
          <label class="flex items-center gap-2.5"><Checkbox v-model="settings.removeDuplicates" />{{ t('groupEditor.removeDuplicates') }}</label>
          <label class="flex items-center gap-2.5"><Checkbox v-model="settings.removeInsecure" />{{ t('groupEditor.removeInsecure') }}</label>
          <label class="flex items-center gap-2.5"><Checkbox v-model="settings.test" data-testid="group-test" />{{ t('groupEditor.test') }}</label>
          <label :class="['flex items-start gap-2.5 pl-7', !settings.test && 'opacity-55']">
            <Checkbox v-model="settings.removeUnavailable" :disabled="!settings.test" /><span>{{ t('groupEditor.removeUnavailable') }}<br /><span class="text-xs text-t3">{{ t('groupEditor.byTest') }}</span></span>
          </label>
          <label :class="['flex items-start gap-2.5 pl-7', !settings.test && 'opacity-55']">
            <Checkbox v-model="settings.sortByLatency" :disabled="!settings.test" /><span>{{ t('groupEditor.sortByLatency') }}<br /><span class="text-xs text-t3">{{ t('groupEditor.byTest') }}</span></span>
          </label>
        </TabsContent>
      </template>
      <Banner v-if="failure" class="mt-3" data-testid="group-failure">{{ failure }}</Banner>
      <template #footer>
        <Button variant="ghost" class="sp" @click="emit('close')">{{ t('common.cancel') }}</Button>
        <Button variant="primary" :disabled="saving || (manual && !name.trim())" data-testid="group-save" @click="save()">{{ t('common.save') }}</Button>
      </template>
    </Dialog>
  </Tabs>
</template>
