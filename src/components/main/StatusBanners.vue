<script setup lang="ts">
import { Download, KeyRound, RefreshCw, ScrollText, SquarePen, X } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import CoreBanner from '@/components/main/CoreBanner.vue'
import { Banner, Button } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useConnect } from '@/composables/useConnect'
import { useProfileActions } from '@/composables/useProfileActions'
import { sentence, useFailure, useReason } from '@/composables/useReason'
import { useUpdates } from '@/composables/useUpdates'
import { GroupManual, ReasonKillSwitchFailed, ReasonKillSwitchNeedsElevation, ReasonKillSwitchTaken, ReasonSecretsLost } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// The lines over the servers of the simple mode and under the connection bar of the advanced one: the core, a failed
// TUN, TLS spoofing or kill switch, the connection's failure, an update, and what the last action did.
const core = useCoreStore()
const ui = useUiStore()
const router = useRouter()
const connect = useConnect()
const profiles = useProfileActions()
const updates = useUpdates()
const reason = useReason()
const failure = useFailure()
const attempt = useAttempt()
const { t } = useI18n()

// The connection's failure stays until the next start, or until the user closes it.
const errorClosed = ref('')

// The offer tells what the restart takes away for a moment and what comes back by itself.
const updateText = computed(() =>
  [
    t('updates.ready', { version: core.updates.version }),
    core.killSwitch.on ? t('updates.killSwitch') : '',
    core.running ? t('updates.reconnect') : '',
  ]
    .filter(Boolean)
    .join(' '),
)

async function installUpdate(): Promise<void> {
  try {
    await updates.install()
  } catch (error) {
    ui.notice = { tone: 'warn', text: t('updates.installFailed', { reason: reason(error) }) }
  }
}

// The kill switch has banners of its own; a failed start for its sake is told there, not twice.
const killSwitchReasons = [ReasonKillSwitchNeedsElevation, ReasonKillSwitchFailed, ReasonKillSwitchTaken]
const killSwitchBroken = computed(() => core.killSwitch.error === ReasonKillSwitchFailed)
// Another Poros on the computer holds the block of Windows: rights would not help, only its own Stop.
const killSwitchTaken = computed(() => core.killSwitch.error === ReasonKillSwitchTaken)
const connectionError = computed(
  () =>
    !!core.connection.error &&
    !connect.tunFailed &&
    !connect.spoofFailed &&
    !killSwitchReasons.includes(core.connection.error) &&
    errorClosed.value !== `${core.connection.error}:${core.connection.since}:${core.connection.errorText}`,
)
const connectionErrorText = computed(() =>
  failure({ code: 1, message: core.connection.errorText, data: { reason: core.connection.error } }),
)
function closeConnectionError(): void {
  errorClosed.value = `${core.connection.error}:${core.connection.since}:${core.connection.errorText}`
}

// A start refused for secrets saved on another computer leads to where they are typed in again: the link of the
// chosen server's subscription, the editor of the advanced mode, and a backup for the rest.
const advanced = computed(() => core.settings?.uiMode === 'advanced')
const secretsLost = computed(() => core.connection.error === ReasonSecretsLost)
const lostServer = computed(() => {
  const server = core.servers.find((s) => s.id === core.settings?.server)
  return server?.need?.length ? server : null
})
const lostGroup = computed(() => {
  const group = core.groups.find((g) => g.id === lostServer.value?.groupId)
  return group && group.kind !== GroupManual && group.need?.length ? group : null
})
</script>

<template>
  <CoreBanner />
  <Banner v-if="connect.tunFailed" data-testid="tun-failed">
    {{ t('admin.failed') }}
    <template #actions>
      <Button size="sm" @click="attempt(() => connect.elevateAndConnect())">{{ t('admin.restart') }}</Button>
      <Button size="sm" @click="attempt(() => connect.useSystemInstead())">{{ t('admin.useSystemShort') }}</Button>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="connect.tunFailed = false"><X /></Button>
    </template>
  </Banner>
  <Banner v-if="connect.spoofFailed && !connect.tunFailed" data-testid="spoof-failed">
    {{ t('admin.spoofFailed') }}
    <template #actions>
      <Button size="sm" @click="attempt(() => connect.elevateForSpoof())">{{ t('admin.restart') }}</Button>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="connect.spoofFailed = false"><X /></Button>
    </template>
  </Banner>
  <Banner v-if="connect.killSwitchFailed && !connect.tunFailed" data-testid="kill-switch-failed">
    {{ t('admin.killSwitchFailed') }}
    <template #actions>
      <Button size="sm" @click="attempt(() => connect.elevateForKillSwitch())">{{ t('admin.restart') }}</Button>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="connect.killSwitchFailed = false"><X /></Button>
    </template>
  </Banner>
  <Banner v-if="killSwitchBroken" data-testid="kill-switch-broken">
    {{ t('killSwitch.broken') }}
    <template #actions>
      <Button size="sm" @click="ui.logs = true">{{ t('menu.logs') }}</Button>
    </template>
  </Banner>
  <Banner v-if="killSwitchTaken" data-testid="kill-switch-taken">
    {{ sentence(t('reason.kill_switch_taken')) }}
  </Banner>
  <Banner v-if="connectionError" data-testid="connection-error">
    {{ connectionErrorText }}
    <template #actions>
      <template v-if="secretsLost">
        <Button v-if="lostGroup" size="sm" data-testid="lost-link-again" @click="ui.linkAgain = lostGroup.id"><KeyRound />{{ t('list.linkAgain') }}</Button>
        <Button v-else-if="lostServer && advanced" size="sm" data-testid="lost-edit" @click="profiles.edit(lostServer.id)">
          <SquarePen />{{ t('profiles.editLost') }}
        </Button>
        <Button size="sm" data-testid="lost-backup" @click="router.push(advanced ? '/adv/settings/backup' : '/settings/backup')">
          <Download />{{ t('settings.backup.restore') }}
        </Button>
      </template>
      <Button v-else size="sm" @click="ui.logs = true"><ScrollText />{{ t('menu.logs') }}</Button>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="closeConnectionError()"><X /></Button>
    </template>
  </Banner>
  <Banner v-if="updates.offer" tone="info" data-testid="update-ready">
    <template #icon><Download class="size-4 flex-none text-blue" /></template>
    {{ updateText }}
    <template #actions>
      <Button size="sm" variant="primary" @click="installUpdate()"><RefreshCw />{{ t('updates.install') }}</Button>
      <Button size="sm" variant="ghost" @click="updates.putOff()">{{ t('common.later') }}</Button>
    </template>
  </Banner>
  <Banner v-if="updates.failure" data-testid="update-failed">
    {{ t(`updates.failed.${core.updates.installError}`, { version: core.updates.installVersion || '?' }) }}
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="updates.failureClosed = true"><X /></Button>
    </template>
  </Banner>
  <Banner v-if="ui.notice" :tone="ui.notice.tone" data-testid="notice">
    {{ ui.notice.text }}
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="ui.notice = null"><X /></Button>
    </template>
  </Banner>
</template>
