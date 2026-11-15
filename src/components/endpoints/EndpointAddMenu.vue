<script setup lang="ts">
import { ChevronDown, Copy, Plus } from '@lucide/vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Menu, MenuItem, MenuSeparator } from '@/components/ui'
import { serverTitle } from '@/lib/format'
import { useCoreStore } from '@/stores/core'

// «Add»: a new OpenVPN or OpenConnect endpoint typed in, or a copy of a profile of those types from any group; the copy
// keeps no tie to the profile.
const emit = defineEmits<{ add: [type: string]; copy: [serverId: string] }>()
const core = useCoreStore()
const { t } = useI18n()

const profiles = computed(() =>
  core.servers
    .filter((s) => s.protocol === 'openvpn' || s.protocol === 'openconnect')
    .map((s) => ({ id: s.id, name: serverTitle(s.name, s.country), type: s.protocol === 'openvpn' ? 'OpenVPN' : 'OpenConnect', group: core.groups.find((g) => g.id === s.groupId)?.name ?? '' })),
)
</script>

<template>
  <Menu min-width="320px">
    <template #trigger>
      <Button size="sm" data-testid="endpoint-add"><Plus />{{ t('endpoints.add') }}<ChevronDown class="size-3.5" /></Button>
    </template>
    <MenuItem data-testid="endpoint-add-openvpn" @select="emit('add', 'openvpn')"><Plus />OpenVPN</MenuItem>
    <MenuItem data-testid="endpoint-add-openconnect" @select="emit('add', 'openconnect')"><Plus />OpenConnect</MenuItem>
    <MenuSeparator />
    <div class="px-2.5 pt-1 pb-1 text-[11.5px] text-t3">{{ t('endpoints.copyOf') }}</div>
    <MenuItem v-for="p in profiles" :key="p.id" data-testid="endpoint-add-copy" @select="emit('copy', p.id)">
      <Copy />{{ p.name }}<span class="text-t3">· {{ p.type }} · {{ p.group }}</span>
    </MenuItem>
    <div v-if="profiles.length === 0" class="px-2.5 pb-1.5 text-xs text-t3">{{ t('endpoints.noProfiles') }}</div>
  </Menu>
</template>
