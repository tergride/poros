<script setup lang="ts">
import { ChevronDown, Plus, Server } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Button, Dialog, Menu, MenuItem, MenuSeparator } from '@/components/ui'
import { type EndpointView, usePresetDraft } from '@/composables/usePresetDraft'
import { protocolName } from '@/lib/format'
import { ActionEndpoint, ActionProxy } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// The endpoints of an advanced preset: OpenVPN and OpenConnect profiles of any group that connect alongside the server.
// The networks their servers announce go through them, and rules send them more. One that rules send traffic to goes
// only after asking: those rules then send it through the server.
const core = useCoreStore()
const { draft, endpointOf } = usePresetDraft()
const { t } = useI18n()

const endpoints = computed<EndpointView[]>(() => (draft.value?.endpoints ?? []).map(endpointOf))
const choices = computed(() =>
  core.servers
    .filter((s) => s.protocol === 'openvpn' || s.protocol === 'openconnect')
    .map((s) => ({
      id: s.id,
      name: s.name,
      about: `${protocolName(s.protocol, t)} · ${core.groups.find((g) => g.id === s.groupId)?.name ?? ''}`,
      added: draft.value?.endpoints.includes(s.id) ?? false,
    })),
)

const removing = ref<EndpointView | null>(null)
const usedBy = (id: string) => draft.value?.rules.filter((r) => r.action === ActionEndpoint && r.endpoint === id).length ?? 0

function add(id: string): void {
  if (draft.value && !draft.value.endpoints.includes(id)) draft.value.endpoints = [...draft.value.endpoints, id]
}

function remove(endpoint: EndpointView, asked = false): void {
  if (!draft.value) return
  if (!asked && usedBy(endpoint.id) > 0) {
    removing.value = endpoint
    return
  }
  removing.value = null
  draft.value.rules = draft.value.rules.map((r) => (r.action === ActionEndpoint && r.endpoint === endpoint.id ? { ...r, action: ActionProxy, endpoint: '' } : r))
  draft.value.endpoints = draft.value.endpoints.filter((id) => id !== endpoint.id)
}
</script>

<template>
  <template v-if="draft">
    <p class="mb-2.5 text-xs text-t3">{{ t('endpoints.hint') }}</p>
    <SettingsCard data-testid="endpoints">
      <div v-for="endpoint in endpoints" :key="endpoint.id" class="flex items-center gap-3.5 border-t border-line py-2.5 first:border-t-0" data-testid="endpoint-row">
        <div class="min-w-0 flex-1">
          <strong :class="['block truncate font-medium', endpoint.gone && 'text-t3']">{{ endpoint.name }}</strong>
          <span class="text-xs text-t3">{{ endpoint.gone ? t('endpoints.goneHint') : `${endpoint.protocol} · ${endpoint.group}` }}</span>
        </div>
        <Button size="sm" variant="danger" data-testid="endpoint-remove" @click="remove(endpoint)">{{ t('common.remove') }}</Button>
      </div>
      <p v-if="endpoints.length === 0" class="py-2.5 text-xs text-t3">{{ t('endpoints.none') }}</p>
    </SettingsCard>
    <Menu min-width="320px">
      <template #trigger>
        <Button size="sm" data-testid="endpoint-add"><Plus />{{ t('common.add') }}<ChevronDown class="size-3.5" /></Button>
      </template>
      <MenuSeparator :label="t('endpoints.profiles')" />
      <MenuItem v-for="choice in choices" :key="choice.id" :disabled="choice.added" @select="add(choice.id)">
        <Server /><span class="truncate">{{ choice.name }}</span><span class="truncate text-t3">· {{ choice.about }}</span>
      </MenuItem>
      <MenuItem v-if="choices.length === 0" disabled>{{ t('endpoints.noProfiles') }}</MenuItem>
    </Menu>
    <Dialog :open="removing !== null" :title="t('endpoints.removeTitle')" @update:open="(o) => !o && (removing = null)">
      <p>{{ t('endpoints.removeText', { name: removing?.name ?? '', n: usedBy(removing?.id ?? '') }, usedBy(removing?.id ?? '')) }}</p>
      <template #footer>
        <Button variant="ghost" class="sp" @click="removing = null">{{ t('common.cancel') }}</Button>
        <Button variant="danger" data-testid="endpoint-remove-confirm" @click="removing && remove(removing, true)">{{ t('common.remove') }}</Button>
      </template>
    </Dialog>
  </template>
</template>
