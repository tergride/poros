<script setup lang="ts">
import { Check, Copy, Globe, Info, LoaderCircle, RefreshCw, TriangleAlert, X } from '@lucide/vue'
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { useClock } from '@/composables/useClock'
import { Button, Dialog, Flag } from '@/components/ui'
import { countryName, formatAgo } from '@/lib/format'
import type { IPInfo } from '@/platform/core-types'
import { LeakFound, LeakNone, MethodNetworkCheck } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

const open = defineModel<boolean>('open', { default: false })
const core = useCoreStore()
const { t, locale } = useI18n()
const now = useClock(30_000)

const connected = computed(() => core.running)
const checkOn = computed(() => core.settings?.checkIp ?? false)

function check(): void {
  if (checkOn.value && !core.network.checking) void core.core().call(MethodNetworkCheck).catch(() => undefined)
}

// Each opening checks again unless a check is fresh: the window shows what is now.
watch(open, (shown) => {
  if (shown && Date.now() - core.network.checkedAt > 60_000) check()
})

const place = (info: IPInfo) => [countryName(info.countryCode, locale.value), info.city].filter(Boolean).join(', ')
const addresses = (info: IPInfo) => [info.ip, info.ipv6].filter(Boolean)
const sides = computed(() => [
  { id: 'without' as const, info: core.network.direct },
  { id: 'through' as const, info: core.network.proxy },
])

async function copy(): Promise<void> {
  const n = core.network
  const line = (side: string, info: IPInfo) => `${side}: ${addresses(info).join(' ')} ${place(info)} ${info.isp}`
  const lines = [line(t('ip.without'), n.direct)]
  if (addresses(n.proxy).length) lines.push(line(t('ip.through'), n.proxy))
  for (const server of n.dns) lines.push(`DNS: ${server.ip} ${server.countryCode} ${server.isp}`)
  await navigator.clipboard.writeText(lines.join('\n'))
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('ip.title')" class="w-[600px]">
    <template #icon><Globe /></template>
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="open = false"><X /></Button>
    </template>
    <div class="grid gap-3.5">
      <p v-if="!checkOn" class="text-t2">{{ t('ip.off') }}</p>
      <div class="grid grid-cols-2 gap-2.5">
        <section
          v-for="side in sides"
          :key="side.id"
          class="grid min-w-0 content-start gap-[5px] rounded-xl border border-line bg-s3 px-3.5 py-3"
          :data-testid="`ip-${side.id}`"
        >
          <h3 class="text-xs font-medium text-t3">{{ t(`ip.${side.id}`) }}</h3>
          <template v-if="side.id === 'through' && !connected">
            <span class="text-xs text-t3">{{ t('ip.connectFirst') }}</span>
          </template>
          <template v-else-if="addresses(side.info).length">
            <!-- An IPv6 address is long: it wraps inside the card after a colon, and the second address is smaller. -->
            <b
              v-for="(address, index) in addresses(side.info)"
              :key="address"
              :class="[
                'font-mono leading-snug font-medium [overflow-wrap:anywhere] select-text',
                index === 0 ? 'text-[17px]' : 'text-[12.5px] text-t2',
              ]"
              data-testid="ip-address"
              ><template v-for="(part, i) in address.split(':')" :key="i"
                ><template v-if="i > 0">:<wbr /></template>{{ part }}</template
              ></b
            >
            <span class="flex items-center gap-2 text-[13px]">
              <Flag :code="side.info.countryCode" class="text-[17px]" />
              {{ place(side.info) }}
            </span>
            <span class="text-xs text-t3">{{ side.info.isp }}</span>
          </template>
          <span v-else class="text-xs text-t3">{{ core.network.checking ? t('ip.checking') : t('ip.unknown') }}</span>
        </section>
      </div>
      <div>
        <h3 class="mb-1.5 text-xs font-medium text-t3">{{ t('ip.dns') }}</h3>
        <div v-if="core.network.dns.length" class="overflow-hidden rounded-[10px] border border-line">
          <div
            v-for="server in core.network.dns"
            :key="server.ip"
            class="grid min-h-[38px] grid-cols-[minmax(0,1.2fr)_22px_minmax(0,1fr)_auto] items-center gap-2.5 border-t border-line px-3 py-1.5 text-[13px] first:border-t-0"
          >
            <span class="font-mono text-[12.5px] [overflow-wrap:anywhere] select-text">{{ server.ip }}</span>
            <Flag :code="server.countryCode" class="text-[16px]" />
            <span class="truncate" :title="server.isp">{{ server.isp }}</span>
            <span v-if="server.home" class="rounded-md bg-s4 px-2 text-[11.5px] leading-[22px] text-warn">{{ t('ip.home') }}</span>
            <span v-else />
          </div>
        </div>
      </div>
      <div class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-3" data-testid="leak-verdict">
        <template v-if="!connected"><Info class="size-4 flex-none text-ok" />{{ t('ip.noConnection') }}</template>
        <template v-else-if="core.network.checking"><LoaderCircle class="size-4 flex-none animate-spin text-blue" />{{ t('ip.checking') }}</template>
        <template v-else-if="core.network.leak === LeakFound">
          <TriangleAlert class="size-4 flex-none text-warn" />
          <span><b class="font-medium">{{ t('ip.leak') }}</b> {{ t('ip.leakText') }}</span>
        </template>
        <template v-else-if="core.network.leak === LeakNone">
          <Check class="size-4 flex-none text-ok" />
          <span><b class="font-medium">{{ t('ip.noLeak') }}</b> {{ t('ip.noLeakText') }}</span>
        </template>
        <template v-else><Info class="size-4 flex-none text-t3" />{{ core.network.errorText || t('ip.unknownLeak') }}</template>
      </div>
      <p class="text-xs text-t3">
        {{ t('ip.note') }}
        <template v-if="core.network.checkedAt">{{ t('ip.checked', { ago: formatAgo(core.network.checkedAt, now, locale) }) }}</template>
      </p>
    </div>
    <template #footer>
      <Button class="sp" :disabled="!addresses(core.network.direct).length" @click="copy">
        <Copy />{{ t('common.copy') }}
      </Button>
      <Button :disabled="!checkOn || core.network.checking" @click="check"><RefreshCw />{{ t('ip.recheck') }}</Button>
    </template>
  </Dialog>
</template>
