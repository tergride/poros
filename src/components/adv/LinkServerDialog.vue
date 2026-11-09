<script setup lang="ts">
import { Plus, Server, SquarePen } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog, Flag, Input, Select } from '@/components/ui'
import { useProfileActions } from '@/composables/useProfileActions'
import { protocolName, serverTitle } from '@/lib/format'
import { GroupManual } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// A link from a browser or a messenger to one server: the advanced mode adds it as a profile, but only after a yes,
// to a group of the user's, «My servers» unless the user picks another or a new one.
const core = useCoreStore()
const ui = useUiStore()
const actions = useProfileActions()
const { t } = useI18n()

const newGroup = '\u0000new'
const groups = computed(() => [
  ...core.groups.filter((g) => g.kind === GroupManual).map((g) => ({ value: g.id, label: g.name })),
  { value: newGroup, label: t('profiles.newGroupChoice') },
])
const target = ref('')
const groupName = ref('')
watch(
  () => ui.linkServer,
  (link) => {
    if (!link) return
    target.value = groups.value[0]!.value
    groupName.value = groups.value.length > 1 ? '' : t('profiles.myServers')
  },
)
const server = computed(() => ui.linkServer?.server)

// The group the server goes to: a group of the user's, or a new one of the name typed.
const targetOf = () =>
  target.value === newGroup
    ? { groupId: '', newGroup: groupName.value.trim() || t('profiles.myServers') }
    : { groupId: target.value, newGroup: '' }

function editFirst(): void {
  const link = ui.linkServer
  if (!link?.profile) return
  ui.linkServer = null
  ui.profileEdit = { id: '', profile: link.profile, target: targetOf() }
}

async function add(): Promise<void> {
  const link = ui.linkServer
  if (!link) return
  ui.linkServer = null
  if (target.value === newGroup) {
    await actions.addText(link.link, null, groupName.value.trim() || t('profiles.myServers'))
  } else {
    await actions.addText(link.link, core.groups.find((g) => g.id === target.value) ?? null)
  }
}
</script>

<template>
  <Dialog :open="ui.linkServer !== null" :title="t('profiles.linkTitle')" @update:open="(open) => !open && (ui.linkServer = null)">
    <template #icon><Server /></template>
    <div v-if="server" class="grid gap-3">
      <p class="text-t2">{{ t('profiles.linkText') }}</p>
      <div class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 rounded-[10px] border border-line bg-s1 px-3.5 py-3">
        <span class="text-t3">{{ t('profiles.type') }}</span><span>{{ protocolName(server.protocol, t) }}</span>
        <span class="text-t3">{{ t('profiles.name') }}</span>
        <span class="flex items-center gap-2"><Flag v-if="server.country" :code="server.country" />{{ serverTitle(server.name, server.country) }}</span>
        <span class="text-t3">{{ t('profiles.address') }}</span><span class="font-mono">{{ server.host }}:{{ server.port }}</span>
      </div>
      <label class="grid gap-1.5">
        <span class="text-xs text-t3">{{ t('profiles.group') }}</span>
        <Select v-model="target" :options="groups" :aria-label="t('profiles.group')" data-testid="link-group" />
      </label>
      <Input v-if="target === newGroup" v-model="groupName" :placeholder="t('list.groupNamePlaceholder')" data-testid="link-new-group" />
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" :disabled="!ui.linkServer?.profile" data-testid="link-edit" @click="editFirst()"><SquarePen />{{ t('profiles.editFirst') }}</Button>
      <Button variant="ghost" @click="ui.linkServer = null">{{ t('common.cancel') }}</Button>
      <Button variant="primary" data-testid="link-add" @click="add()"><Plus />{{ t('common.add') }}</Button>
    </template>
  </Dialog>
</template>
