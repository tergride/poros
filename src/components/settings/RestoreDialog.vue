<script setup lang="ts">
import { Download } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Checkbox, Dialog } from '@/components/ui'
import { useFailure } from '@/composables/useReason'
import { type BackupInfo, MethodBackupRestore } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// What the chosen backup holds, before anything is replaced: the advanced mode picks the parts, the simple one takes
// all. The core checks the file whole before it changes anything.
const open = defineModel<boolean>('open', { default: false })
const props = defineProps<{ file: string; text: string; info: BackupInfo | null; advanced: boolean }>()
const emit = defineEmits<{ restored: [info: BackupInfo] }>()

const core = useCoreStore()
const failure = useFailure()
const { t, locale } = useI18n()
const chosen = ref<string[]>([])
const error = ref('')
const busy = ref(false)
watch(open, (now) => {
  if (!now) return
  chosen.value = [...(props.info?.parts ?? [])]
  error.value = ''
})

const date = computed(() =>
  props.info ? new Date(props.info.created).toLocaleString(locale.value, { day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '',
)
function amount(part: string): string {
  const i = props.info
  if (!i) return ''
  switch (part) {
    case 'profiles':
      return t('settings.backup.amount.profiles', { groups: i.groups, servers: i.servers })
    case 'routing':
      return t('settings.backup.amount.routing', { n: i.presets })
    case 'otp':
      return t('settings.backup.amount.otp', { n: i.codes })
    default:
      return t('settings.backup.amount.settings')
  }
}
function toggle(part: string, on: boolean): void {
  chosen.value = on ? [...chosen.value, part] : chosen.value.filter((p) => p !== part)
}

async function restore(): Promise<void> {
  busy.value = true
  error.value = ''
  try {
    const info = await core.core().call(MethodBackupRestore, { text: props.text, parts: props.advanced ? chosen.value : [] })
    open.value = false
    emit('restored', info)
  } catch (e) {
    error.value = failure(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('settings.backup.dialogTitle')" class="w-[560px]">
    <template #icon><Download /></template>
    <div class="grid gap-3" data-testid="restore-dialog">
      <p class="text-t2">{{ t('settings.backup.from', { file, date, version: info?.version || '—' }) }}</p>
      <div v-if="advanced" class="grid gap-2">
        <label
          v-for="part in info?.parts ?? []"
          :key="part"
          class="flex cursor-pointer items-start gap-3.5 rounded-xl border border-line bg-s2 px-3.5 py-3 hover:border-line2"
          :data-testid="`restore-part-${part}`"
        >
          <Checkbox :model-value="chosen.includes(part)" @update:model-value="(v: boolean) => toggle(part, v)" />
          <span class="grid gap-0.5">
            <strong class="font-medium">{{ t(`settings.backup.part.${part}`) }}</strong>
            <span class="text-xs text-t3">{{ amount(part) }}</span>
          </span>
        </label>
      </div>
      <div v-else class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 rounded-[10px] border border-line bg-s2 px-3.5 py-3">
        <template v-for="part in info?.parts ?? []" :key="part">
          <span class="text-t3">{{ t(`settings.backup.part.${part}`) }}</span><span>{{ amount(part) }}</span>
        </template>
      </div>
      <p class="text-xs text-t3">
        {{ t(advanced ? 'settings.backup.replaceChosen' : 'settings.backup.replaceAll') }}{{ core.running ? ` ${t('settings.backup.stops')}` : '' }}
      </p>
      <p v-if="error" class="rounded-[10px] border border-line bg-s1 px-3.5 py-2.5 text-[13px] text-warn" data-testid="restore-error">{{ error }}</p>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="open = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="busy || (advanced && chosen.length === 0)" data-testid="restore-confirm" @click="restore()">
        <Download />{{ t('settings.backup.restoreButton') }}
      </Button>
    </template>
  </Dialog>
</template>
