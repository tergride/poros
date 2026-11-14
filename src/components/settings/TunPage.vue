<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Button, Input, Select, Switch, Textarea } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'
import { TunBypassDefault, TunIPv4Default, TunIPv6Default } from '@/platform/core-types'

// The TUN of the advanced mode: the adapter, its addresses and the ranges it keeps out. Changes wait for «Save» and
// come with the next connection.
const { t } = useI18n()
const mtu = useSetting('tunMtu')
const strictRoute = useSetting('tunStrictRoute')
const routing = useSetting('tunRouting')
const ipv4 = useSetting('tunIpv4')
const ipv6 = useSetting('tunIpv6')
const bypass = useSetting('tunBypass')

const mtus = computed(() => {
  const known = [9000, 1500, 1420]
  return [...(known.includes(mtu.value) ? [] : [mtu.value]), ...known].map((value) => ({ value: String(value), label: String(value) }))
})
const mtuText = computed({ get: () => String(mtu.value), set: (text: string) => (mtu.value = Number(text)) })

function defaultAddresses(): void {
  ipv4.value = TunIPv4Default
  ipv6.value = TunIPv6Default
}
</script>

<template>
  <h2 class="mb-4 font-disp text-[19px] font-normal">{{ t('settings.nav.tun') }}</h2>
  <SettingsCard :title="t('settings.tun.adapter')">
    <div class="grid grid-cols-2 items-end gap-x-4 gap-y-3 pt-2.5">
      <div class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">MTU</span>
        <Select v-model="mtuText" :options="mtus" class="w-full" aria-label="MTU" data-testid="tun-mtu" />
      </div>
      <label class="flex h-9 items-center gap-3">
        <Switch v-model="strictRoute" aria-label="Strict Route" data-testid="tun-strict" />Strict Route
      </label>
    </div>
    <label class="flex items-start gap-3 pt-3 pb-1">
      <Switch v-model="routing" aria-label="TUN Routing" data-testid="tun-routing" />
      <span>TUN Routing<br /><span class="text-xs text-t3">{{ t('settings.tun.routingHint') }}</span></span>
    </label>
  </SettingsCard>
  <SettingsCard :title="t('settings.tun.addresses')">
    <div class="grid grid-cols-2 gap-x-4 gap-y-3 pt-2.5">
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">IPv4 CIDR</span>
        <Input v-model="ipv4" mono data-testid="tun-ipv4" />
      </label>
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">IPv6 CIDR</span>
        <Input v-model="ipv6" mono data-testid="tun-ipv6" />
      </label>
    </div>
    <div class="pt-2.5 pb-1"><Button size="sm" data-testid="tun-addresses-default" @click="defaultAddresses">{{ t('settings.tun.addressesDefault') }}</Button></div>
  </SettingsCard>
  <SettingsCard :title="t('settings.tun.bypass')">
    <label class="grid gap-[5px] pt-2.5">
      <span class="text-[12.5px] text-t2">{{ t('settings.tun.ranges') }}</span>
      <Textarea v-model="bypass" mono class="h-[120px] resize-y" :aria-label="t('settings.tun.ranges')" data-testid="tun-bypass" />
      <span class="text-xs text-t3">{{ t('settings.tun.bypassHint') }}</span>
    </label>
    <div class="pt-2.5 pb-1"><Button size="sm" data-testid="tun-bypass-default" @click="bypass = TunBypassDefault">{{ t('settings.tun.bypassDefault') }}</Button></div>
  </SettingsCard>
</template>
