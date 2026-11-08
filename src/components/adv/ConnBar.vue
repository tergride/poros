<script setup lang="ts">
import { ArrowDown, ArrowUp, Lock, Play, ShieldAlert, Square, X } from '@lucide/vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import { Button, Flag, Select, Switch } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useConnect } from '@/composables/useConnect'
import { useConnectionView } from '@/composables/useConnectionView'
import { latencyTone } from '@/lib/format'
import { ConnectionStopping } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// The connection bar of the advanced mode: what the portal of the simple mode says, in one row over every section.
defineEmits<{ ip: [] }>()

const core = useCoreStore()
const connect = useConnect()
const attempt = useAttempt()
const router = useRouter()
const { t, te } = useI18n()
const { state, on, busy, group, blocked, full, held, blockedIdle, cancellable, stateText, address, shownIp, ipText, latencyText, speed, tun, system, serverName } =
  useConnectionView()

// The presets by name, and a last choice that opens them.
const morePresets = '\u0000more'
const presetOptions = computed(() => [
  ...core.presets.map((p) => ({
    value: p.id,
    label: p.builtin && te(`presets.name.${p.id}`) ? t(`presets.name.${p.id}`) : p.name,
  })),
  { value: morePresets, label: t('adv.presetsMore') },
])
const preset = computed({
  get: () => core.settings?.preset ?? '',
  set: (id: string) => {
    if (id === morePresets) void router.push('/adv/routing')
    else void attempt(() => connect.choosePreset(id))
  },
})
const lockTitle = computed(() =>
  held.value ? t(full.value ? 'portal.heldFullTitle' : 'portal.heldTitle') : blocked.value ? t('portal.lockTitle') : undefined,
)
</script>

<template>
  <div class="flex flex-none items-center gap-3 border-b border-line bg-s0 px-3.5 py-2.5" data-testid="connbar" :data-state="state">
    <div :title="lockTitle" class="flex-none">
      <Button v-if="on || held" :disabled="!core.ready" data-testid="stop" @click="attempt(() => connect.stop())">
        <Square class="size-4 fill-current" />{{ t('portal.stop') }}
        <Lock v-if="blocked" class="size-3.5 opacity-75" data-testid="kill-switch-lock" />
      </Button>
      <Button v-else-if="cancellable" :disabled="!core.ready" data-testid="cancel" @click="attempt(() => connect.stop())">
        <X class="size-4" />{{ t('portal.cancel') }}
      </Button>
      <Button
        v-else
        variant="primary"
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

    <div class="grid min-w-0 flex-1 gap-0.5">
      <strong class="flex min-w-0 items-center gap-2 text-[13.5px] font-medium">
        <span
          :class="['size-2 flex-none rounded-full', on ? 'bg-ok' : held || blockedIdle ? 'bg-warn' : busy ? 'bg-blue' : 'bg-t3']"
        />
        <span class="truncate" :title="on ? serverName : stateText" data-testid="connbar-state">{{ on ? serverName : stateText }}</span>
      </strong>
      <small class="truncate text-xs text-t3">
        <template v-if="!on">{{ t('adv.selected', { name: serverName }) }}</template>
        <template v-else-if="group">{{ group.name }}</template>
        <template v-if="ipText">
          ·
          <button
            type="button"
            class="cursor-pointer text-t2 underline decoration-t3 decoration-dotted underline-offset-[3px] hover:text-t1"
            data-testid="connbar-ip"
            @click="$emit('ip')"
          >
            {{ ipText }}
          </button>
          <template v-if="shownIp && address.countryCode"> · <Flag :code="address.countryCode" /></template>
        </template>
        <template v-if="on && core.server && latencyText(core.server.latency)">
          ·
          <span :class="{ 'text-ok': latencyTone(core.server.latency) === 'good' }">{{ latencyText(core.server.latency) }}</span>
        </template>
      </small>
    </div>

    <div v-if="on" class="flex flex-none gap-3 font-mono text-xs text-t2">
      <span class="inline-flex items-center gap-1"><ArrowDown class="size-3.5" />{{ speed(core.traffic?.download ?? 0) }}</span>
      <span class="inline-flex items-center gap-1"><ArrowUp class="size-3.5" />{{ speed(core.traffic?.upload ?? 0) }}</span>
    </div>

    <Select v-model="preset" :options="presetOptions" class="w-[170px] flex-none" :aria-label="t('portal.preset')" />
    <label class="flex flex-none cursor-pointer items-center gap-2 text-[13px] font-semibold">
      <Switch v-model="tun" :disabled="!core.ready" data-testid="tun" />TUN<ShieldAlert v-if="!core.status.elevated" class="size-3.5 text-warn" />
    </label>
    <label class="flex flex-none cursor-pointer items-center gap-2 text-[13px] font-semibold">
      <Switch v-model="system" :disabled="!core.ready" data-testid="system" />System
    </label>
  </div>
</template>
