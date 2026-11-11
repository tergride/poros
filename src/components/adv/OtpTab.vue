<script setup lang="ts">
import { ArrowDown, ArrowUp, Check, Copy, Download, MoreHorizontal, Plus, RefreshCw, SquarePen, Trash2 } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import OtpDialog from '@/components/adv/OtpDialog.vue'
import OtpImportDialog from '@/components/adv/OtpImportDialog.vue'
import { Button, Dialog, Menu, MenuItem, MenuSeparator } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { useClock } from '@/composables/useClock'
import { groupCode, hiddenCode, secondsLeft } from '@/lib/otp'
import { appControls } from '@/platform'
import { MethodOTPCode, MethodOTPList, MethodOTPMove, MethodOTPRemove, OTPKindHOTP, type OTPCode } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// «Tools → OTP»: the codes of the user's sign-ins. A TOTP code counts down and the list asks the core again when one
// runs out; a HOTP code shows on «Next code», since each one moves the counter of the server.
const core = useCoreStore()
const attempt = useAttempt()
const { t } = useI18n()
const now = useClock(1000)

const adding = ref(false)
const importing = ref(false)
const editId = ref('')
const editing = ref(false)
const removing = ref<OTPCode | null>(null)
const copied = ref('')
// The HOTP codes taken here, until the list is left.
const shown = ref<Record<string, string>>({})

const codes = computed(() => core.otp)
watch(now, async (time) => {
  if (!core.ready || !codes.value.some((c) => c.until > 0 && c.until <= time)) return
  core.otp = await core.core().call(MethodOTPList).catch(() => core.otp)
})

const title = (c: OTPCode) => c.name || c.account || t('adv.tools.unnamed')
const subtitle = (c: OTPCode) => [c.name ? c.account : '', c.kind === OTPKindHOTP ? t('adv.tools.hotp') : ''].filter(Boolean).join(' · ')
const codeOf = (c: OTPCode) => (c.kind === OTPKindHOTP ? (shown.value[c.id] ?? '') : c.code)

async function next(c: OTPCode): Promise<void> {
  await attempt(async () => {
    const code = await core.core().call(MethodOTPCode, { id: c.id })
    shown.value = { ...shown.value, [c.id]: code.code }
  })
}

async function copy(c: OTPCode): Promise<void> {
  const code = codeOf(c)
  if (!code) return
  await attempt(async () => {
    await appControls.copy(code)
    copied.value = c.id
    setTimeout(() => {
      if (copied.value === c.id) copied.value = ''
    }, 1500)
  })
}

function edit(c: OTPCode): void {
  editId.value = c.id
  editing.value = true
}

function add(): void {
  editId.value = ''
  adding.value = true
}

const move = (c: OTPCode, offset: number) => attempt(() => core.core().call(MethodOTPMove, { id: c.id, offset }))

async function remove(): Promise<void> {
  const c = removing.value
  removing.value = null
  if (c) await attempt(() => core.core().call(MethodOTPRemove, { id: c.id }))
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="flex flex-none items-center gap-2 border-b border-line px-3.5 py-2.5">
      <Button variant="primary" size="sm" data-testid="otp-add" @click="add()"><Plus />{{ t('common.add') }}</Button>
      <Button size="sm" data-testid="otp-import-open" @click="importing = true"><Download />{{ t('adv.tools.import') }}</Button>
    </div>
    <div class="min-h-0 flex-1 overflow-auto px-3.5 py-3.5">
      <section class="rounded-xl border border-line bg-s2 px-4" data-testid="otp-list">
        <div v-if="codes.length === 0" class="grid gap-1 py-4" data-testid="otp-empty">
          <strong class="font-medium">{{ t('adv.tools.emptyTitle') }}</strong>
          <span class="text-xs text-t3">{{ t('adv.tools.emptyText') }}</span>
        </div>
        <div
          v-for="(c, i) in codes"
          :key="c.id"
          class="grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-3 border-t border-line py-2.5 first:border-t-0"
          data-testid="otp-row"
        >
          <div class="min-w-0">
            <div class="truncate">{{ title(c) }}</div>
            <span v-if="subtitle(c)" class="block truncate text-xs text-t3">{{ subtitle(c) }}</span>
          </div>
          <b :class="['font-mono text-[19px] font-medium tracking-[.08em]', !codeOf(c) && 'text-t3']" data-testid="otp-code">
            {{ codeOf(c) ? groupCode(codeOf(c)) : hiddenCode(c.digits) }}
          </b>
          <div class="flex items-center gap-1.5">
            <Button v-if="c.kind === OTPKindHOTP" size="sm" data-testid="otp-next" @click="next(c)"><RefreshCw />{{ t('adv.tools.next') }}</Button>
            <span v-else class="inline-flex h-[22px] min-w-[44px] items-center justify-center rounded-md bg-s4 px-2 text-[11.5px] text-t2" data-testid="otp-left">
              {{ t('adv.tools.seconds', { n: secondsLeft(c.until, now) }) }}
            </span>
            <Button variant="ghost" size="icon-sm" :disabled="!codeOf(c)" :aria-label="t('adv.tools.copy')" data-testid="otp-copy" @click="copy(c)">
              <Check v-if="copied === c.id" class="text-ok" /><Copy v-else />
            </Button>
            <Menu align="end" min-width="220px">
              <template #trigger>
                <Button variant="ghost" size="icon-sm" :aria-label="t('adv.tools.actions')" data-testid="otp-menu"><MoreHorizontal /></Button>
              </template>
              <MenuItem :disabled="!codeOf(c)" @select="copy(c)"><Copy />{{ t('adv.tools.copy') }}</MenuItem>
              <MenuItem data-testid="otp-edit" @select="edit(c)"><SquarePen />{{ t('adv.tools.edit') }}</MenuItem>
              <MenuSeparator />
              <MenuItem :disabled="i === 0" data-testid="otp-up" @select="move(c, -1)"><ArrowUp />{{ t('adv.tools.up') }}</MenuItem>
              <MenuItem :disabled="i === codes.length - 1" @select="move(c, 1)"><ArrowDown />{{ t('adv.tools.down') }}</MenuItem>
              <MenuSeparator />
              <MenuItem danger data-testid="otp-remove" @select="removing = c"><Trash2 />{{ t('common.remove') }}</MenuItem>
            </Menu>
          </div>
        </div>
      </section>
      <p class="mt-2.5 text-xs leading-relaxed text-t3">{{ t('adv.tools.note') }}</p>
    </div>

    <OtpDialog v-model:open="adding" edit-id="" />
    <OtpDialog v-model:open="editing" :edit-id="editId" />
    <OtpImportDialog v-model:open="importing" />
    <Dialog :open="removing !== null" :title="t('adv.tools.removeTitle')" @update:open="(v: boolean) => !v && (removing = null)">
      <p class="text-t2">{{ t('adv.tools.removeText', { name: removing ? title(removing) : '' }) }}</p>
      <template #footer>
        <Button variant="ghost" class="sp" @click="removing = null">{{ t('common.cancel') }}</Button>
        <Button variant="danger" data-testid="otp-remove-confirm" @click="remove()">{{ t('common.remove') }}</Button>
      </template>
    </Dialog>
  </div>
</template>
