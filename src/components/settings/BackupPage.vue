<script setup lang="ts">
import { Download, TriangleAlert, Upload, X } from '@lucide/vue'
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

import OptionRow from '@/components/settings/OptionRow.vue'
import RestoreDialog from '@/components/settings/RestoreDialog.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Banner, Button, Switch } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useFailure } from '@/composables/useReason'
import { useSettingsDraft } from '@/composables/useSettingsDraft'
import { appControls } from '@/platform'
import { type BackupInfo, BackupOTP, BackupProfiles, BackupRouting, BackupSettings, MethodBackupCreate, MethodBackupInspect } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// «Backup»: the data of Poros in one file and back. The simple mode keeps all of it; the advanced one picks the
// parts to keep and to put back. The file holds the keys of the servers as they are.
const core = useCoreStore()
const ui = useUiStore()
const attempt = useAttempt()
const failure = useFailure()
const { t, locale } = useI18n()
const advanced = useSettingsDraft() !== null

const parts = [BackupProfiles, BackupRouting, BackupSettings, BackupOTP]
const chosen = ref<string[]>([...parts])
const toggle = (part: string, on: boolean) => (chosen.value = on ? [...chosen.value, part] : chosen.value.filter((p) => p !== part))

async function create(): Promise<void> {
  await attempt(async () => {
    const file = await core.core().call(MethodBackupCreate, { parts: advanced ? chosen.value : parts })
    const path = await appControls.saveFile(file.name, t('settings.backup.kind'), file.text)
    if (path) ui.notice = { tone: 'info', text: t('settings.backup.saved', { path }) }
  })
}

// The chosen file: what it holds, then the dialog that restores it.
const fileInput = ref<HTMLInputElement | null>(null)
const reading = ref<{ name: string; text: string; info: BackupInfo } | null>(null)
const dialog = ref(false)
async function read(): Promise<void> {
  const file = fileInput.value?.files?.[0]
  if (fileInput.value) fileInput.value.value = ''
  if (!file) return
  try {
    const text = await file.text()
    const info = await core.core().call(MethodBackupInspect, { text })
    reading.value = { name: file.name, text, info }
    dialog.value = true
  } catch (error) {
    ui.notice = { tone: 'warn', text: failure(error) }
  }
}

const restored = ref('')
function done(info: BackupInfo): void {
  restored.value = t('settings.backup.restored', { date: new Date(info.created).toLocaleDateString(locale.value, { day: 'numeric', month: 'long', year: 'numeric' }) })
}
</script>

<template>
  <h2 :class="['font-disp text-[19px] font-normal', advanced ? 'mb-4' : 'mb-1']">{{ t('settings.nav.backup') }}</h2>
  <p v-if="!advanced" class="mb-4 max-w-[640px] text-t2">{{ t('settings.backup.lead') }}</p>
  <Banner v-if="restored" tone="info" class="mb-3" data-testid="backup-restored">
    {{ restored }}
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="restored = ''"><X /></Button>
    </template>
  </Banner>
  <template v-if="advanced">
    <SettingsCard :title="t('settings.backup.createTitle')">
      <div class="grid grid-cols-2 gap-x-4 gap-y-3 pt-2.5 pb-1">
        <label v-for="part in parts" :key="part" class="flex items-start gap-3" :data-testid="`backup-part-${part}`">
          <Switch :model-value="chosen.includes(part)" :aria-label="t(`settings.backup.part.${part}`)" @update:model-value="(v: boolean) => toggle(part, v)" />
          <span>{{ t(`settings.backup.part.${part}`) }}<br /><span class="text-xs text-t3">{{ t(`settings.backup.partHint.${part}`) }}</span></span>
        </label>
      </div>
      <p class="my-2.5 flex items-center gap-2.5 rounded-[10px] border border-line bg-s1 px-3.5 py-2.5 text-[13px]">
        <TriangleAlert class="size-4 flex-none text-warn" />{{ t('settings.backup.secret') }}
      </p>
      <Button size="sm" :disabled="chosen.length === 0 || !core.ready" data-testid="backup-create" @click="create()">
        <Upload />{{ t('settings.backup.createMore') }}
      </Button>
    </SettingsCard>
    <SettingsCard :title="t('settings.backup.restoreTitle')">
      <OptionRow :title="t('settings.backup.restore')" :hint="t('settings.backup.restoreChosenHint')">
        <Button size="sm" :disabled="!core.ready" data-testid="backup-choose" @click="fileInput?.click()"><Download />{{ t('settings.backup.chooseMore') }}</Button>
      </OptionRow>
    </SettingsCard>
  </template>
  <SettingsCard v-else>
    <OptionRow :title="t('settings.backup.create')" :hint="t('settings.backup.createHint')">
      <Button :disabled="!core.ready" data-testid="backup-create" @click="create()"><Upload />{{ t('settings.backup.createButton') }}</Button>
    </OptionRow>
    <OptionRow :title="t('settings.backup.restore')" :hint="t('settings.backup.restoreHint')">
      <Button :disabled="!core.ready" data-testid="backup-choose" @click="fileInput?.click()"><Download />{{ t('settings.backup.choose') }}</Button>
    </OptionRow>
  </SettingsCard>
  <input ref="fileInput" type="file" class="hidden" accept=".json" data-testid="backup-file" @change="read" />
  <RestoreDialog
    v-model:open="dialog"
    :file="reading?.name ?? ''"
    :text="reading?.text ?? ''"
    :info="reading?.info ?? null"
    :advanced="advanced"
    @restored="done"
  />
</template>
