<script setup lang="ts">
import { ChevronRight, Layers, Lock, Play, Settings, ShieldAlert, Square, X } from '@lucide/vue'
import { computed } from 'vue'
import { I18nT, useI18n } from 'vue-i18n'

import portalArt from '@/assets/img/portal.jpg'
import { Button, Flag, Switch } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useConnect } from '@/composables/useConnect'
import { useConnectionView } from '@/composables/useConnectionView'
import { latencyTone } from '@/lib/format'
import { localProxyHost, localProxyPort } from '@/platform/core'
import { ConnectionStopping } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

defineEmits<{ ip: []; proxy: [] }>()

const core = useCoreStore()
const connect = useConnect()
const attempt = useAttempt()
const { t } = useI18n()
const {
  state,
  on,
  busy,
  group,
  empty,
  blocked,
  full,
  held,
  blockedIdle,
  cancellable,
  stateText,
  address,
  shownIp,
  ipText,
  latencyText,
  speed,
  presetName,
  tun,
  system,
  exitCountry,
  serverName,
} = useConnectionView()

// Without TUN and System the hint names the local proxy; its address opens what it is and how to use it.
const hint = computed(() => {
  const s = core.settings
  if (!s) return ''
  if (s.tun && s.systemProxy) return t('portal.hint.both')
  if (s.tun) return t('portal.hint.tun')
  if (s.systemProxy) return t('portal.hint.system')
  return ''
})
const proxyAddress = `${localProxyHost}:${localProxyPort}`
</script>

<template>
  <aside
    class="relative flex w-[384px] flex-none flex-col justify-end gap-3.5 overflow-hidden border-r border-line bg-s0 p-[22px]"
    data-testid="portal"
    :data-state="state"
  >
    <div
      :class="[
        'absolute inset-x-0 top-0 h-[380px] bg-cover bg-top transition-[filter] duration-700',
        on ? '' : busy ? 'brightness-[.7] saturate-[.8]' : 'brightness-[.42] saturate-[.45]',
      ]"
      :style="{ backgroundImage: `url(${portalArt})` }"
    >
      <div class="absolute inset-0 bg-[linear-gradient(to_bottom,transparent_40%,var(--s0)_98%)]" />
    </div>

    <div
      :class="['relative flex items-center gap-2 text-[13px]', on ? 'text-ok' : held || blockedIdle ? 'text-warn' : 'text-t2']"
      data-testid="portal-state"
    >
      <span
        :class="[
          'size-2 rounded-full',
          on
            ? 'bg-ok shadow-[0_0_0_4px_color-mix(in_srgb,var(--ok)_16%,transparent)]'
            : held || blockedIdle
              ? 'bg-warn'
              : busy
                ? 'bg-blue'
                : 'bg-t3',
        ]"
      />
      {{ stateText }}
    </div>

    <div class="relative grid gap-1">
      <!-- Providers pack notes into names; three lines keep the buttons in place, the whole name is in the hint. -->
      <h1 class="line-clamp-3 font-disp text-[19px] leading-snug font-normal" :title="serverName" data-testid="portal-server">
        {{ serverName }}
      </h1>
      <span class="text-xs text-t3">
        <template v-if="empty">{{ t('portal.addFirst') }}</template>
        <template v-else>
          <button
            v-if="ipText"
            type="button"
            class="cursor-pointer break-all text-t2 underline decoration-t3 decoration-dotted underline-offset-[3px] hover:text-t1"
            data-testid="portal-ip"
            @click="$emit('ip')"
          >
            {{ ipText }}
          </button>
          <template v-if="ipText && shownIp && address.countryCode">
            · <Flag :code="address.countryCode" class="text-[13px]" />
          </template>
          <template v-if="!on && group"><template v-if="ipText"> · </template>{{ group.name }}</template>
          <template v-if="core.server && latencyText(core.server.latency)">
            ·
            <span :class="{ 'text-ok': latencyTone(core.server.latency) === 'good' }">{{ latencyText(core.server.latency) }}</span>
          </template>
        </template>
      </span>
    </div>

    <div v-if="on" class="relative grid grid-cols-3 gap-2">
      <div class="rounded-[10px] border border-line bg-s2 px-2.5 py-[9px]">
        <span class="mb-0.5 block text-[11.5px] text-t3">{{ t('portal.download') }}</span>
        <b class="font-mono text-[13px] font-medium">{{ speed(core.traffic?.download ?? 0) }}</b>
      </div>
      <div class="rounded-[10px] border border-line bg-s2 px-2.5 py-[9px]">
        <span class="mb-0.5 block text-[11.5px] text-t3">{{ t('portal.upload') }}</span>
        <b class="font-mono text-[13px] font-medium">{{ speed(core.traffic?.upload ?? 0) }}</b>
      </div>
      <div class="rounded-[10px] border border-line bg-s2 px-2.5 py-[9px]">
        <span class="mb-0.5 block text-[11.5px] text-t3">{{ t('portal.exit') }}</span>
        <Flag v-if="exitCountry" :code="exitCountry" class="text-[17px] leading-[20px]" />
        <b v-else class="font-mono text-[13px] font-medium">—</b>
      </div>
    </div>

    <div
      class="relative flex gap-2.5"
      :title="held ? t(full ? 'portal.heldFullTitle' : 'portal.heldTitle') : blocked ? t('portal.lockTitle') : undefined"
    >
      <Button
        v-if="on || held"
        size="start"
        class="flex-1"
        :disabled="!core.ready"
        data-testid="stop"
        @click="attempt(() => connect.stop())"
      >
        <Square class="size-4 fill-current" />{{ t('portal.stop') }}
        <Lock v-if="blocked" class="size-3.5 opacity-75" data-testid="kill-switch-lock" />
      </Button>
      <Button
        v-else-if="cancellable"
        size="start"
        class="flex-1"
        :disabled="!core.ready"
        data-testid="cancel"
        @click="attempt(() => connect.stop())"
      >
        <X class="size-4" />{{ t('portal.cancel') }}
      </Button>
      <Button
        v-else
        variant="primary"
        size="start"
        class="flex-1"
        :disabled="!core.server || busy || !core.ready"
        data-testid="start"
        @click="attempt(() => connect.start())"
      >
        <Play class="size-4 fill-current" />{{
          state === ConnectionStopping ? t('portal.disconnecting') : busy ? t('portal.connecting') : t('portal.start')
        }}
        <Lock v-if="blocked" class="size-3.5 opacity-75" data-testid="kill-switch-lock" />
      </Button>
    </div>

    <div class="relative grid grid-cols-2 gap-2">
      <label
        :class="[
          'flex h-11 cursor-pointer items-center gap-2.5 rounded-[10px] border px-3 hover:border-line2',
          tun ? 'border-line2 bg-s3' : 'border-line bg-s2',
        ]"
      >
        <Switch v-model="tun" :disabled="!core.ready" data-testid="tun" />
        <b class="flex items-center gap-1.5 text-[13px] font-semibold">
          TUN<ShieldAlert v-if="!core.status.elevated" class="size-3.5 text-warn" />
        </b>
      </label>
      <label
        :class="[
          'flex h-11 cursor-pointer items-center gap-2.5 rounded-[10px] border px-3 hover:border-line2',
          system ? 'border-line2 bg-s3' : 'border-line bg-s2',
        ]"
      >
        <Switch v-model="system" :disabled="!core.ready" data-testid="system" />
        <b class="text-[13px] font-semibold">System</b>
      </label>
    </div>
    <p v-if="hint || !core.settings" class="relative text-xs text-t3">{{ hint }}</p>
    <I18nT v-else keypath="portal.hint.none" tag="p" scope="global" class="relative text-xs text-t3">
      <template #address>
        <button
          type="button"
          class="cursor-pointer font-mono text-t2 underline decoration-t3 decoration-dotted underline-offset-[3px] hover:text-t1"
          data-testid="portal-proxy"
          @click="$emit('proxy')"
        >
          {{ proxyAddress }}
        </button>
      </template>
    </I18nT>

    <div class="relative flex gap-2">
      <RouterLink
        to="/settings/presets"
        class="flex h-[42px] min-w-0 flex-1 items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3 text-[13px] text-t2 hover:border-line2"
      >
        <Layers class="size-4 flex-none" /><span class="truncate">{{ t('portal.preset') }} <b class="font-medium text-t1">{{ presetName }}</b></span>
        <ChevronRight class="ml-auto size-4 flex-none" />
      </RouterLink>
      <RouterLink
        to="/settings/conn"
        class="grid size-[42px] flex-none place-items-center rounded-[10px] border border-line bg-s2 text-t2 hover:border-line2 hover:text-t1"
        :title="t('portal.settings')"
        :aria-label="t('portal.settings')"
        data-testid="portal-settings"
      >
        <Settings class="size-4" />
      </RouterLink>
    </div>
  </aside>
</template>
