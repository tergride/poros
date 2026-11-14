<script setup lang="ts">
import { Info } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import DnsChoice from '@/components/presets/DnsChoice.vue'
import DnsCacheDialog from '@/components/settings/DnsCacheDialog.vue'
import DnsObjectDialog from '@/components/settings/DnsObjectDialog.vue'
import OptionRow from '@/components/settings/OptionRow.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Button, Segmented, Switch, Textarea } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'
import { useSettingsDraft } from '@/composables/useSettingsDraft'
import { DNSRoutingAuto, DNSRoutingDirect, DNSRoutingProxy } from '@/platform/core-types'

// The DNS of every preset with «By default», and which way names go: as their traffic, all through the server or all
// past it. The advanced mode adds the hosts file, ready answers, the cache and an own dns object of sing-box, which
// replaces the rest of the page and the DNS of presets. Long explanations show on hover.
const { t } = useI18n()
const advanced = useSettingsDraft() !== null
const remote = useSetting('dnsRemote')
const direct = useSetting('dnsDirect')
const routing = useSetting('dnsRouting')
const fakeIp = useSetting('fakeIp')
const hosts = useSetting('dnsHosts')
const answers = useSetting('dnsAnswers')
const object = useSetting('dnsObject')
const cacheOpen = ref(false)
const objectOpen = ref(false)

const remoteChoices = [
  { value: '', label: 'Google' },
  { value: 'https://1.1.1.1/dns-query', label: 'Cloudflare' },
  { value: 'https://9.9.9.9/dns-query', label: 'Quad9' },
]
const directChoices = computed(() => [{ value: '', label: t('presetEdit.dnsSystem') }])
const ways = computed(() => [
  { value: DNSRoutingAuto, label: t('settings.dns.routingAuto') },
  { value: DNSRoutingProxy, label: t('settings.dns.routingProxy') },
  { value: DNSRoutingDirect, label: t('settings.dns.routingDirect') },
])
</script>

<template>
  <h2 class="mb-1 font-disp text-[19px] font-normal">{{ t('settings.nav.dns') }}</h2>
  <p class="mb-4 max-w-[640px] text-t2">{{ t('settings.dns.lead') }}</p>
  <div v-if="object" class="mb-3 flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-2.5" role="status" data-testid="dns-object-set">
    <Info class="size-4 flex-none text-warn" />
    <span class="flex-1">{{ t(advanced ? 'settings.dns.objectSet' : 'settings.dns.objectSetSimple') }}</span>
    <Button v-if="advanced" size="sm" @click="objectOpen = true">{{ t('settings.dns.change') }}</Button>
  </div>
  <fieldset :disabled="!!object" :class="['m-0 min-w-0 border-0 p-0', object && 'opacity-55']">
    <SettingsCard :title="t('settings.dns.servers')">
      <p class="mt-1.5 mb-0.5 text-xs text-t3">{{ t('settings.dns.serversHint') }}</p>
      <DnsChoice v-model="remote" lazy :title="t('presetEdit.dnsRemote')" :hint="t('settings.dns.remoteHint')" :choices="remoteChoices" />
      <DnsChoice v-model="direct" lazy :title="t('presetEdit.dnsDirect')" :hint="t('settings.dns.directHint')" :choices="directChoices" />
    </SettingsCard>
    <SettingsCard :title="t('settings.dns.how')">
      <OptionRow :title="t('settings.dns.routing')" :hint="t('settings.dns.routingHint')" :note="t('settings.dns.routingNote')">
        <Segmented v-model="routing" :options="ways" :disabled="!!object" :aria-label="t('settings.dns.routing')" data-testid="dns-routing" />
      </OptionRow>
      <OptionRow :title="t('settings.dns.fakeIp')" :hint="t('settings.dns.fakeIpHint')" :note="t('settings.dns.fakeIpNote')">
        <Switch v-model="fakeIp" data-testid="fake-ip" />
      </OptionRow>
    </SettingsCard>
  </fieldset>
  <template v-if="advanced">
    <SettingsCard :title="t('settings.dns.hostsTitle')">
      <OptionRow :title="t('settings.dns.hosts')" :hint="t('settings.dns.hostsHint')"><Switch v-model="hosts" /></OptionRow>
    </SettingsCard>
    <SettingsCard :title="t('settings.dns.answers')">
      <div class="grid gap-1 pt-2 pb-2.5">
        <Textarea v-model="answers" mono class="h-[84px] resize-y" placeholder="nas.home 192.168.1.20" :aria-label="t('settings.dns.answers')" data-testid="dns-answers" />
        <span class="text-xs text-t3">{{ t('settings.dns.answersHint') }}</span>
      </div>
    </SettingsCard>
    <SettingsCard :title="t('settings.dns.more')">
      <OptionRow :title="t('settings.dns.cache')" :hint="t('settings.dns.cacheHint')">
        <Button size="sm" data-testid="dns-cache" @click="cacheOpen = true">{{ t('settings.dns.change') }}</Button>
      </OptionRow>
      <OptionRow :title="t('settings.dns.object')" :hint="t('settings.dns.objectHint')">
        <Button size="sm" data-testid="dns-object" @click="objectOpen = true">{{ t(object ? 'settings.dns.change' : 'settings.dns.set') }}</Button>
      </OptionRow>
    </SettingsCard>
    <DnsCacheDialog v-model:open="cacheOpen" />
    <DnsObjectDialog v-model:open="objectOpen" />
  </template>
</template>
