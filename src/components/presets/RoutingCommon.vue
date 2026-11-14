<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Select } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'
import { AddressFirstIPv4, AddressFirstIPv6 } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// «General» of the routing: which address of a site with both to take first, and the preset in use, the same choice
// as in the connection bar. It lives in a RoutingDraftPage, whose «Save» keeps it.
const core = useCoreStore()
const { t, te } = useI18n()
const addressFirst = useSetting('addressFirst')
const preset = useSetting('preset')

const families = computed(() => [
  { value: '', label: t('routing.common.auto') },
  { value: AddressFirstIPv4, label: 'IPv4' },
  { value: AddressFirstIPv6, label: 'IPv6' },
])
const presets = computed(() =>
  core.presets.map((p) => ({ value: p.id, label: p.builtin && te(`presets.name.${p.id}`) ? t(`presets.name.${p.id}`) : p.name })),
)
</script>

<template>
  <SettingsCard :title="t('routing.common.title')">
    <div class="grid grid-cols-2 items-start gap-x-4 gap-y-3 pt-2.5 pb-1">
      <div class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('routing.common.addressFirst') }}</span>
        <Select
          v-model="addressFirst"
          :options="families"
          :disabled="!core.settings?.ipv6"
          class="w-full"
          :aria-label="t('routing.common.addressFirst')"
          data-testid="address-first"
        />
        <span class="text-xs text-t3">{{ t('routing.common.addressFirstHint') }}</span>
      </div>
      <div class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('routing.common.preset') }}</span>
        <Select v-model="preset" :options="presets" class="w-full" :aria-label="t('routing.common.preset')" data-testid="common-preset" />
        <span class="text-xs text-t3">{{ t('routing.common.presetHint') }}</span>
      </div>
    </div>
  </SettingsCard>
</template>
