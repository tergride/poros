<script setup lang="ts">
import { Check, Copy, Globe, Info, LoaderCircle, RefreshCw, TriangleAlert, X } from '@lucide/vue'
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { useClock } from '@/composables/useClock'
import { Button, Dialog } from '@/components/ui'
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

async function copy(): Promise<void> {
  const n = core.network
  const lines = [`${t('ip.without')}: ${n.direct.ip} ${place(n.direct)} ${n.direct.isp}`]
  if (n.proxy.ip) lines.push(`${t('ip.through')}: ${n.proxy.ip} ${place(n.proxy)} ${n.proxy.isp}`)
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
          v-for="side in ['without', 'through'] as const"
          :key="side"
          class="grid content-start gap-[5px] rounded-xl border border-line bg-s3 px-3.5 py-3"
        >
          <h3 class="text-xs font-medium text-t3">{{ t(`ip.${side}`) }}</h3>
          <template v-if="side === 'through' && !connected">
            <span class="text-xs text-t3">{{ t('ip.connectFirst') }}</span>
          </template>
          <template v-else-if="(side === 'without' ? core.network.direct : core.network.proxy).ip">
            <b class="font-mono text-[17px] leading-snug font-medium select-text">{{ (side === 'without' ? core.network.direct : core.network.proxy).ip }}</b>
            <span class="flex items-center gap-2 text-[13px]">
              <span class="inline-grid h-[22px] w-[30px] place-items-center rounded-[5px] bg-s4 font-mono text-[11px]">
                {{ (side === 'without' ? core.network.direct : core.network.proxy).countryCode }}
              </span>
              {{ place(side === 'without' ? core.network.direct : core.network.proxy) }}
            </span>
            <span class="text-xs text-t3">{{ (side === 'without' ? core.network.direct : core.network.proxy).isp }}</span>
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
            class="grid h-[38px] grid-cols-[150px_34px_1fr_auto] items-center gap-2.5 border-t border-line px-3 text-[13px] first:border-t-0"
          >
            <span class="font-mono text-[12.5px] select-text">{{ server.ip }}</span>
            <span class="inline-grid h-[22px] w-[30px] place-items-center rounded-[5px] bg-s4 font-mono text-[11px]">{{ server.countryCode }}</span>
            <span class="truncate">{{ server.isp }}</span>
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
      <Button class="sp" :disabled="!core.network.direct.ip" @click="copy">
        <Copy />{{ t('common.copy') }}
      </Button>
      <Button :disabled="!checkOn || core.network.checking" @click="check"><RefreshCw />{{ t('ip.recheck') }}</Button>
    </template>
  </Dialog>
</template>
