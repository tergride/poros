<script setup lang="ts">
import { Check, Download, Globe, Info, X } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import OptionRow from '@/components/settings/OptionRow.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Button, Dialog, Input, Segmented, Select, Switch, Textarea } from '@/components/ui'
import { useFailure } from '@/composables/useReason'
import { useSetting } from '@/composables/useSetting'
import type { WARPDevice } from '@/platform/core-types'
import { MethodWARPRegister, WARPHTTP2, WARPHTTP3, WARPModeMASQUE, WARPModeWireGuard } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// WARP of the routing, in a RoutingDraftPage: the exit of what rules send «Through WARP», behind the server, so that
// sites see an address of Cloudflare. Its keys are data rather than a behavior, so they stay editable while it is off;
// each mode keeps a device of its own. A device comes from Cloudflare after its terms are accepted, or is typed in.
const core = useCoreStore()
const failureOf = useFailure()
const { t, locale } = useI18n()
const on = useSetting('warp')
const mode = useSetting('warpMode')
const wireGuard = useSetting('warpWireGuard')
const masque = useSetting('warpMasque')
const hosts = useSetting('warpHosts')

const isMasque = computed(() => mode.value === WARPModeMASQUE)
const device = computed(() => (isMasque.value ? masque.value : wireGuard.value))
function change(field: keyof WARPDevice, value: string): void {
  const next = { ...device.value, [field]: value }
  if (isMasque.value) masque.value = next
  else wireGuard.value = next
}
const field = (name: keyof WARPDevice) => computed({ get: () => String(device.value[name]), set: (value: string) => change(name, value) })
const endpoint = field('endpoint')
const privateKey = field('privateKey')
const peerKey = field('peerKey')
const addresses = field('addresses')
const reserved = field('reserved')
const serverName = field('serverName')
const http = field('http')

const modes = [
  { value: WARPModeWireGuard, label: 'WireGuard' },
  { value: WARPModeMASQUE, label: 'MASQUE' },
]
const versions = computed(() => [
  { value: '', label: t('routing.warp.httpBoth') },
  { value: WARPHTTP3, label: t('routing.warp.http3') },
  { value: WARPHTTP2, label: t('routing.warp.http2') },
])
const empty = computed(() => !device.value.endpoint && !device.value.privateKey && !device.value.peerKey && !device.value.addresses)
const registered = computed(() =>
  device.value.registeredAt ? new Intl.DateTimeFormat(locale.value, { day: 'numeric', month: 'long', year: 'numeric' }).format(device.value.registeredAt) : '',
)

// Registration: Cloudflare's terms first, then a new device of the mode, which the core keeps at once.
const termsOpen = ref(false)
const registering = ref(false)
const failure = ref('')
watch(termsOpen, () => (failure.value = ''))
async function register(): Promise<void> {
  registering.value = true
  failure.value = ''
  try {
    const saved = await core.core().call(MethodWARPRegister, { mode: mode.value })
    // The kept device replaces what the page had of it.
    if (isMasque.value) masque.value = saved.warpMasque
    else wireGuard.value = saved.warpWireGuard
    termsOpen.value = false
  } catch (error) {
    failure.value = failureOf(error)
  } finally {
    registering.value = false
  }
}

const hostsOpen = ref(false)
const hostsText = ref('')
watch(hostsOpen, (shown) => {
  if (shown) hostsText.value = hosts.value
})
function saveHosts(): void {
  hosts.value = hostsText.value.trim()
  hostsOpen.value = false
}
</script>

<template>
  <SettingsCard :title="t('routing.warp.title')">
    <OptionRow :title="t('routing.warp.use')" :hint="t('routing.warp.useHint')"><Switch v-model="on" data-testid="warp-on" /></OptionRow>
  </SettingsCard>
  <SettingsCard :title="t('routing.warp.config')">
    <div class="pt-2.5">
      <div v-if="registered" class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-2.5" data-testid="warp-state">
        <Check class="size-4 flex-none text-ok" />{{ t('routing.warp.registered', { date: registered }) }}
      </div>
      <div v-else-if="empty" class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-2.5" data-testid="warp-state">
        <Info class="size-4 flex-none text-warn" />{{ t('routing.warp.none') }}
      </div>
    </div>
    <div class="grid gap-[5px] pt-3">
      <span class="text-[12.5px] text-t2">{{ t('routing.warp.mode') }}</span>
      <div><Segmented v-model="mode" :options="modes" :aria-label="t('routing.warp.mode')" data-testid="warp-mode" /></div>
      <span class="text-xs text-t3">{{ t('routing.warp.modeHint') }}</span>
    </div>
    <div class="grid grid-cols-2 gap-x-4 gap-y-3 pt-3 pb-1">
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">Endpoint</span>
        <Input v-model="endpoint" mono :placeholder="isMasque ? '162.159.198.2:443' : '162.159.192.1:2408'" data-testid="warp-endpoint" />
      </label>
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('routing.warp.privateKey') }}</span>
        <Input v-model="privateKey" mono data-testid="warp-private-key" />
      </label>
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('routing.warp.peerKey') }}</span>
        <Input v-model="peerKey" mono />
      </label>
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('routing.warp.addresses') }}</span>
        <Input v-model="addresses" mono placeholder="172.16.0.2/32, 2606:4700:110::2/128" />
      </label>
      <template v-if="isMasque">
        <label class="grid gap-[5px]">
          <span class="text-[12.5px] text-t2">SNI</span>
          <Input v-model="serverName" mono placeholder="consumer-masque.cloudflareclient.com" />
        </label>
        <div class="grid gap-[5px]">
          <span class="text-[12.5px] text-t2">{{ t('routing.warp.http') }}</span>
          <Select v-model="http" :options="versions" class="w-full" :aria-label="t('routing.warp.http')" />
        </div>
      </template>
      <label v-else class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">Reserved</span>
        <Input v-model="reserved" mono placeholder="12, 85, 201" />
      </label>
    </div>
    <div class="flex items-center gap-2 pt-1 pb-2.5">
      <Button size="sm" data-testid="warp-register" @click="termsOpen = true"><Download />{{ t('routing.warp.register') }}</Button>
      <Button size="sm" data-testid="warp-hosts" @click="hostsOpen = true">{{ t('routing.warp.hosts') }}</Button>
    </div>
  </SettingsCard>

  <Dialog v-model:open="termsOpen" :title="t('routing.warp.termsTitle')">
    <template #icon><Globe /></template>
    <div class="grid gap-2 text-t2">
      <p>{{ t(empty ? 'routing.warp.termsText' : 'routing.warp.termsTextReplace') }}</p>
      <p>{{ t('routing.warp.terms') }} <span class="font-mono text-[12.5px] select-text">https://www.cloudflare.com/application/terms/</span></p>
      <p v-if="failure" class="text-xs text-bad" data-testid="warp-failure">{{ failure }}</p>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="termsOpen = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="registering" data-testid="warp-accept" @click="register"><Download />{{ t('routing.warp.accept') }}</Button>
    </template>
  </Dialog>
  <Dialog v-model:open="hostsOpen" :title="t('routing.warp.hostsTitle')">
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="hostsOpen = false"><X /></Button>
    </template>
    <div class="grid gap-2">
      <Textarea v-model="hostsText" mono class="h-[120px] resize-none" placeholder="api.cloudflareclient.com" :aria-label="t('routing.warp.hostsTitle')" data-testid="warp-hosts-text" />
      <p class="text-xs text-t3">{{ t('routing.warp.hostsHint') }}</p>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="hostsOpen = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" data-testid="warp-hosts-done" @click="saveHosts">{{ t('common.done') }}</Button>
    </template>
  </Dialog>
</template>
