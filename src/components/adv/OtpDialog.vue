<script setup lang="ts">
import { ChevronRight, Plus } from '@lucide/vue'
import { watchDebounced } from '@vueuse/core'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog, Input, Segmented, Select } from '@/components/ui'
import { useOtpProblem } from '@/composables/useOtpProblem'
import { useFailure } from '@/composables/useReason'
import { isCoreError } from '@/platform'
import {
  MethodOTPGet,
  MethodOTPParse,
  MethodOTPSave,
  OTPKindHOTP,
  OTPKindTOTP,
  OTPMaxDigits,
  OTPMinDigits,
  OTPSHA1,
  OTPSHA256,
  OTPSHA512,
} from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// A new code from an otpauth:// link or a bare secret, which the core reads as it is typed; or an entry to change.
// What a link does not say keeps the values most services use, under «More».
const open = defineModel<boolean>('open', { default: false })
const props = defineProps<{ editId: string }>()

const core = useCoreStore()
const { t } = useI18n()
const problemText = useOtpProblem()
const failure = useFailure()

const link = ref('')
const secret = ref('')
const name = ref('')
const account = ref('')
const kind = ref<string>(OTPKindTOTP)
const algorithm = ref<string>(OTPSHA1)
const digits = ref(6)
const period = ref('30')
const counter = ref('0')
const more = ref(false)
const error = ref('')
const saving = ref(false)
// A name typed by hand stays when the link changes.
let named = false

const editing = computed(() => props.editId !== '')
const kinds = computed(() => [
  { value: OTPKindTOTP, label: t('adv.tools.totp') },
  { value: OTPKindHOTP, label: t('adv.tools.hotpChoice') },
])
const algorithms = [OTPSHA1, OTPSHA256, OTPSHA512].map((a) => ({ value: a, label: a.replace('SHA', 'SHA-') }))
const digitChoices = Array.from({ length: OTPMaxDigits - OTPMinDigits + 1 }, (_, i) => ({ value: OTPMinDigits + i, label: String(OTPMinDigits + i) }))

function reset(): void {
  link.value = secret.value = name.value = account.value = error.value = ''
  kind.value = OTPKindTOTP
  algorithm.value = OTPSHA1
  digits.value = 6
  period.value = '30'
  counter.value = '0'
  more.value = false
  named = false
}

watch(open, async (now) => {
  if (!now) return
  reset()
  if (!editing.value) return
  try {
    const entry = await core.core().call(MethodOTPGet, { id: props.editId })
    secret.value = entry.secret
    name.value = entry.name
    account.value = entry.account
    kind.value = entry.kind
    algorithm.value = entry.algorithm
    digits.value = entry.digits
    period.value = String(entry.period || 30)
    counter.value = String(entry.counter)
    more.value = true
  } catch (e) {
    error.value = failure(e)
  }
})

watchDebounced(
  link,
  async (text) => {
    if (editing.value) return
    secret.value = ''
    error.value = ''
    if (!text.trim()) return
    const parsed = await core.core().call(MethodOTPParse, { text })
    if (parsed.problems.length > 0) {
      // One line here: its number says nothing.
      error.value = problemText({ ...parsed.problems[0]!, line: 0 })
      return
    }
    if (parsed.entries.length !== 1) {
      error.value = t('adv.tools.many')
      return
    }
    const entry = parsed.entries[0]!
    secret.value = entry.secret
    if (!named) {
      name.value = entry.name
      account.value = entry.account
    }
    kind.value = entry.kind
    algorithm.value = entry.algorithm
    digits.value = entry.digits
    period.value = String(entry.period || 30)
    counter.value = String(entry.counter)
  },
  { debounce: 150 },
)

function typedName(value: string): void {
  name.value = value
  named = true
}

const ready = computed(() => secret.value.trim() !== '' && (editing.value || error.value === '') && !saving.value)

async function save(): Promise<void> {
  if (!ready.value) return
  saving.value = true
  error.value = ''
  try {
    await core.core().call(MethodOTPSave, {
      id: props.editId,
      name: name.value,
      account: account.value,
      secret: secret.value,
      kind: kind.value,
      algorithm: algorithm.value,
      digits: digits.value,
      period: kind.value === OTPKindTOTP ? Number(period.value) : 0,
      counter: kind.value === OTPKindHOTP ? Number(counter.value) : 0,
    })
    open.value = false
  } catch (e) {
    error.value = isCoreError(e) && e.data?.problem ? problemText(e.data.problem) : failure(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog v-model:open="open" :title="t(editing ? 'adv.tools.editTitle' : 'adv.tools.newTitle')" class="w-[620px]">
    <div class="grid gap-3" data-testid="otp-dialog">
      <div class="grid grid-cols-2 gap-x-4 gap-y-3">
        <label v-if="editing" class="col-span-2 grid gap-1.5">
          <span class="text-xs text-t3">{{ t('adv.tools.secret') }}</span>
          <Input v-model="secret" type="password" mono autocomplete="off" spellcheck="false" data-testid="otp-secret" />
          <span class="text-xs text-t3">{{ t('adv.tools.secretHint') }}</span>
        </label>
        <label v-else class="col-span-2 grid gap-1.5">
          <span class="text-xs text-t3">{{ t('adv.tools.linkOrSecret') }}</span>
          <Input v-model="link" mono autocomplete="off" spellcheck="false" :placeholder="t('adv.tools.linkPlaceholder')" data-testid="otp-link" />
          <span class="text-xs text-t3">{{ t('adv.tools.linkHint') }}</span>
        </label>
        <label class="grid gap-1.5">
          <span class="text-xs text-t3">{{ t('adv.tools.name') }}</span>
          <Input :model-value="name" :placeholder="t('adv.tools.namePlaceholder')" data-testid="otp-name" @update:model-value="typedName" />
        </label>
        <label class="grid gap-1.5">
          <span class="text-xs text-t3">{{ t('adv.tools.account') }}</span>
          <Input v-model="account" :placeholder="t('adv.tools.optional')" data-testid="otp-account" />
        </label>
      </div>
      <div class="border-t border-line pt-1.5">
        <button
          type="button"
          class="flex min-h-[34px] cursor-pointer items-center gap-1.5 text-[13px] text-t2 hover:text-t1"
          :aria-expanded="more"
          data-testid="otp-more"
          @click="more = !more"
        >
          <ChevronRight :class="['size-3.5 transition-transform', more && 'rotate-90']" />{{ t('adv.tools.more') }}
        </button>
        <div v-if="more" class="grid gap-3 pt-1.5">
          <div class="grid gap-1.5">
            <span class="text-xs text-t3">{{ t('adv.tools.kind') }}</span>
            <Segmented v-model="kind" :options="kinds" :aria-label="t('adv.tools.kind')" class="justify-self-start" />
          </div>
          <div class="grid grid-cols-3 gap-4">
            <label class="grid gap-1.5">
              <span class="text-xs text-t3">{{ t('adv.tools.algorithm') }}</span>
              <Select v-model="algorithm" :options="algorithms" :aria-label="t('adv.tools.algorithm')" class="w-full" />
            </label>
            <label class="grid gap-1.5">
              <span class="text-xs text-t3">{{ t('adv.tools.digits') }}</span>
              <Select v-model="digits" :options="digitChoices" :aria-label="t('adv.tools.digits')" class="w-full" />
            </label>
            <label v-if="kind === OTPKindHOTP" class="grid gap-1.5">
              <span class="text-xs text-t3">{{ t('adv.tools.counter') }}</span>
              <Input v-model="counter" mono inputmode="numeric" data-testid="otp-counter" />
            </label>
            <label v-else class="grid gap-1.5">
              <span class="text-xs text-t3">{{ t('adv.tools.period') }}</span>
              <Input v-model="period" mono inputmode="numeric" data-testid="otp-period" />
            </label>
          </div>
          <span class="text-xs text-t3">{{ t('adv.tools.moreHint') }}</span>
        </div>
      </div>
      <p v-if="error" class="rounded-[10px] border border-line bg-s1 px-3.5 py-2.5 text-[13px] text-warn" data-testid="otp-error">{{ error }}</p>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="open = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="!ready" data-testid="otp-save" @click="save()">
        <template v-if="editing">{{ t('common.save') }}</template>
        <template v-else><Plus />{{ t('common.add') }}</template>
      </Button>
    </template>
  </Dialog>
</template>
