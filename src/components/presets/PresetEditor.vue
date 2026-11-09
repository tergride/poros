<script setup lang="ts">
import { Plus, TextAlignStart } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import DefaultCard from '@/components/presets/DefaultCard.vue'
import DnsChoice from '@/components/presets/DnsChoice.vue'
import EndpointList from '@/components/presets/EndpointList.vue'
import RuleDialog from '@/components/presets/RuleDialog.vue'
import RuleList from '@/components/presets/RuleList.vue'
import ToAdvancedDialog from '@/components/presets/ToAdvancedDialog.vue'
import PresetListsDialog from '@/components/settings/PresetListsDialog.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui'
import { usePresetDraft } from '@/composables/usePresetDraft'
import type { PresetRule } from '@/platform/core-types'
import { DNSLocal } from '@/platform/core-types'

// The editor of a preset under its name, the same in both modes: its kind, not the mode, decides what it has. A simple
// preset is one page: where traffic goes by default, the rules and the remote DNS. An advanced one has tabs: the rules
// with more kinds, the endpoints and both DNS servers. A built-in preset shows as a simple one under a lock.
// listsButton puts «Edit as a list» over a simple preset, for a host that has no place for it of its own.
defineProps<{ listsButton?: boolean }>()
const { draft, listsOpen, ruleOpen } = usePresetDraft()
const { t } = useI18n()
const tab = ref('list')

const remoteChoices = computed(() => [
  { value: '', label: t('presetEdit.dnsDefault') },
  { value: 'https://1.1.1.1/dns-query', label: 'Cloudflare' },
  { value: 'https://8.8.8.8/dns-query', label: 'Google' },
  { value: 'https://9.9.9.9/dns-query', label: 'Quad9' },
])
const directChoices = computed(() => [
  { value: '', label: t('presetEdit.dnsDefault') },
  { value: DNSLocal, label: t('presetEdit.dnsSystem') },
])

// The lists window replaces the rules it has lines for; the preset is saved as before.
function applyLists(rules: PresetRule[]): void {
  if (draft.value) draft.value.rules = rules
}
</script>

<template>
  <template v-if="draft">
    <template v-if="!draft.advanced">
      <div v-if="listsButton && !draft.builtin" class="mb-2.5 flex justify-end">
        <Button size="sm" data-testid="preset-lists" @click="listsOpen = true"><TextAlignStart />{{ t('presetLists.open') }}</Button>
      </div>
      <DefaultCard />
      <RuleList :advanced="false" />
      <SettingsCard :title="t('presetEdit.dns')" class="max-w-[520px]">
        <DnsChoice v-model="draft.dns" :title="t('presetEdit.dnsRemote')" :hint="t('presetEdit.dnsRemoteHint')" :choices="remoteChoices" :disabled="draft.builtin" />
      </SettingsCard>
    </template>
    <Tabs v-else v-model="tab">
      <TabsList class="-mx-7 mb-3.5 px-5">
        <TabsTrigger value="list" data-testid="preset-tab-list">{{ t('presetEdit.tabRules') }}</TabsTrigger>
        <TabsTrigger value="endpoints" data-testid="preset-tab-endpoints">{{ t('presetEdit.tabEndpoints') }}</TabsTrigger>
        <TabsTrigger value="dns" data-testid="preset-tab-dns">{{ t('presetEdit.tabDns') }}</TabsTrigger>
      </TabsList>
      <TabsContent value="list">
        <div class="mb-2.5 flex justify-end gap-2">
          <Button size="sm" data-testid="rule-new" @click="ruleOpen = -1"><Plus />{{ t('rules.withConditions') }}</Button>
          <Button size="sm" data-testid="preset-lists" @click="listsOpen = true"><TextAlignStart />{{ t('presetLists.open') }}</Button>
        </div>
        <DefaultCard />
        <RuleList advanced />
      </TabsContent>
      <TabsContent value="endpoints">
        <EndpointList />
      </TabsContent>
      <TabsContent value="dns">
        <SettingsCard :title="t('presetEdit.dnsServers')" class="max-w-[640px]">
          <DnsChoice v-model="draft.dns" :title="t('presetEdit.dnsRemote')" :hint="t('presetEdit.dnsRemoteHint')" :choices="remoteChoices" />
          <DnsChoice v-model="draft.directDns" :title="t('presetEdit.dnsDirect')" :hint="t('presetEdit.dnsDirectHint')" :choices="directChoices" />
        </SettingsCard>
      </TabsContent>
    </Tabs>
    <PresetListsDialog v-model:open="listsOpen" :name="draft.name" :advanced="draft.advanced" :rules="draft.rules" @apply="applyLists" />
    <RuleDialog />
    <ToAdvancedDialog />
  </template>
</template>
