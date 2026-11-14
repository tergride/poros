<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Input, Select, Switch } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'

// «Protocols and bypass»: what profiles that leave a choice «As by default» take — the multiplex, split and forged
// TLS handshakes, the uTLS fingerprint — and the QUIC of the servers that run on it. Changes wait for «Save».
const { t } = useI18n()
const mux = useSetting('muxDefault')
const muxProtocol = useSetting('muxProtocol')
const muxConcurrency = useSetting('muxConcurrency')
const fragment = useSetting('fragmentDefault')
const fingerprint = useSetting('fingerprintDefault')
const spoof = useSetting('spoofDefault')
const spoofSni = useSetting('spoofSni')
const spoofMethod = useSetting('spoofMethod')
const idleTimeout = useSetting('quicIdleTimeout')
const keepAlive = useSetting('quicKeepAlive')
const streamWindow = useSetting('quicStreamWindow')
const connectionWindow = useSetting('quicConnectionWindow')
const maxStreams = useSetting('quicMaxStreams')
const packetSize = useSetting('quicInitialPacketSize')
const noPathMtu = useSetting('quicDisablePmtud')

// A number as typed: empty is 0, the core's own; the core refuses what is out of range at «Save».
function whole(model: { value: number }) {
  return computed({
    get: () => (model.value ? String(model.value) : ''),
    set: (text: string) => {
      const value = Number(text.trim())
      if (Number.isInteger(value)) model.value = value
    },
  })
}
const concurrencyText = whole(muxConcurrency)
const maxStreamsText = whole(maxStreams)
const packetSizeText = whole(packetSize)

const protocols = ['h2mux', 'smux', 'yamux'].map((value) => ({ value, label: value }))
const fingerprints = computed(() => [
  { value: '', label: t('settings.protocols.noFingerprint') },
  ...['chrome', 'firefox', 'edge', 'safari', '360', 'qq', 'ios', 'android', 'random', 'randomized'].map((value) => ({ value, label: value })),
])
const methods = computed(() => [
  { value: '', label: t('settings.protocols.coreMethod') },
  ...['wrong-sequence', 'wrong-checksum', 'wrong-ack', 'wrong-md5', 'wrong-timestamp'].map((value) => ({ value, label: value })),
])
</script>

<template>
  <h2 class="mb-4 font-disp text-[19px] font-normal">{{ t('settings.nav.protocols') }}</h2>
  <SettingsCard :title="t('settings.protocols.mux')">
    <div class="grid grid-cols-2 items-end gap-x-4 gap-y-3 pt-2.5 pb-1">
      <label class="flex h-9 items-center gap-3"><Switch v-model="mux" :aria-label="t('settings.protocols.muxOn')" data-testid="mux-default" />{{ t('settings.protocols.muxOn') }}</label>
      <div class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.protocols.protocol') }}</span>
        <Select v-model="muxProtocol" :options="protocols" class="w-full" :aria-label="t('settings.protocols.protocol')" />
      </div>
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">Concurrency</span>
        <Input v-model="concurrencyText" mono inputmode="numeric" data-testid="mux-concurrency" />
      </label>
    </div>
  </SettingsCard>
  <SettingsCard :title="t('settings.protocols.fragment')">
    <label class="flex items-start gap-3 pt-2.5 pb-1">
      <Switch v-model="fragment" :aria-label="t('settings.protocols.byDefault')" data-testid="fragment-default" />
      <span>{{ t('settings.protocols.byDefault') }}<br /><span class="text-xs text-t3">{{ t('settings.protocols.fragmentHint') }}</span></span>
    </label>
  </SettingsCard>
  <SettingsCard title="TLS">
    <div class="grid grid-cols-2 gap-x-4 pt-2.5 pb-1">
      <div class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.protocols.fingerprint') }}</span>
        <Select v-model="fingerprint" :options="fingerprints" class="w-full" :aria-label="t('settings.protocols.fingerprint')" data-testid="fingerprint-default" />
      </div>
    </div>
  </SettingsCard>
  <SettingsCard :title="t('settings.protocols.spoof')">
    <div class="grid grid-cols-2 gap-x-4 gap-y-3 pt-2.5">
      <label class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">Spoof SNI</span>
        <Input v-model="spoofSni" mono placeholder="www.example.com" data-testid="spoof-sni" />
      </label>
      <div class="grid gap-[5px]">
        <span class="text-[12.5px] text-t2">{{ t('settings.protocols.method') }}</span>
        <Select v-model="spoofMethod" :options="methods" class="w-full" :aria-label="t('settings.protocols.method')" />
      </div>
    </div>
    <label class="flex items-start gap-3 pt-3 pb-1">
      <Switch v-model="spoof" :aria-label="t('settings.protocols.byDefault')" data-testid="spoof-default" />
      <span>{{ t('settings.protocols.byDefault') }}<br /><span class="text-xs text-t3">{{ t('settings.protocols.spoofHint') }}</span></span>
    </label>
  </SettingsCard>
  <SettingsCard :title="t('settings.protocols.quic')">
    <p class="mt-1.5 text-xs text-t3">{{ t('settings.protocols.quicHint') }}</p>
    <div class="grid grid-cols-2 gap-x-4 gap-y-3 pt-2.5 pb-1">
      <label class="grid gap-[5px]"><span class="text-[12.5px] text-t2">Idle Timeout</span><Input v-model="idleTimeout" mono placeholder="30s" /></label>
      <label class="grid gap-[5px]"><span class="text-[12.5px] text-t2">Keep Alive Period</span><Input v-model="keepAlive" mono placeholder="10s" /></label>
      <label class="grid gap-[5px]"><span class="text-[12.5px] text-t2">Stream Receive Window</span><Input v-model="streamWindow" mono placeholder="8MB" /></label>
      <label class="grid gap-[5px]"><span class="text-[12.5px] text-t2">Connection Receive Window</span><Input v-model="connectionWindow" mono placeholder="20MB" /></label>
      <label class="grid gap-[5px]"><span class="text-[12.5px] text-t2">Max Concurrent Streams</span><Input v-model="maxStreamsText" mono inputmode="numeric" /></label>
      <label class="grid gap-[5px]"><span class="text-[12.5px] text-t2">Initial Packet Size</span><Input v-model="packetSizeText" mono inputmode="numeric" placeholder="1252" /></label>
    </div>
    <label class="flex h-9 items-center gap-3 pb-1"><Switch v-model="noPathMtu" aria-label="Disable Path MTU Discovery" />Disable Path MTU Discovery</label>
  </SettingsCard>
</template>
