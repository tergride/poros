<script setup lang="ts">
import { TriangleAlert, Upload, X } from '@lucide/vue'
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Banner, Button, Dialog, RadioGroup, RadioItem } from '@/components/ui'
import { useFailure } from '@/composables/useReason'
import { appControls } from '@/platform'
import type { Endpoint } from '@/platform/core-types'
import { MethodEndpointsExport } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// The file of an endpoint, with the data for signing in or without it; a file without it names what it left out, so
// that its import asks for exactly that. The code of OTP is of this computer and never goes in.
const open = defineModel<boolean>('open', { default: false })
const props = defineProps<{ endpoint: Endpoint | null }>()
const core = useCoreStore()
const ui = useUiStore()
const failureOf = useFailure()
const { t } = useI18n()
const signIn = ref('without')
const failure = ref('')
const busy = ref(false)

watch(open, (now) => {
  if (!now) return
  signIn.value = 'without'
  failure.value = ''
})

async function save(): Promise<void> {
  if (!props.endpoint || busy.value) return
  busy.value = true
  failure.value = ''
  try {
    const file = await core.core().call(MethodEndpointsExport, { id: props.endpoint.id, signIn: signIn.value === 'with' })
    const where = await appControls.saveFile(file.name, t('endpoints.fileKind'), file.text)
    if (where === null) return
    open.value = false
    ui.notice = { tone: 'info', text: t('endpoints.saved', { path: where }) }
  } catch (error) {
    failure.value = failureOf(error)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('endpoints.exportTitle', { name: endpoint?.name ?? '' })" class="w-[560px]">
    <template #icon><Upload /></template>
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="open = false"><X /></Button>
    </template>
    <div class="grid gap-3" data-testid="endpoint-export">
      <RadioGroup v-model="signIn" class="grid gap-1">
        <label v-for="k in ['without', 'with'] as const" :key="k" class="grid cursor-pointer grid-cols-[20px_1fr] items-center gap-3 border-t border-line py-2.5 first:border-t-0">
          <RadioItem :value="k" :aria-label="t(`endpoints.${k}SignIn`)" :data-testid="`export-${k}`" />
          <span>
            <strong class="block font-medium">{{ t(`endpoints.${k}SignIn`) }}</strong>
            <span class="text-xs text-t3">{{ t(`endpoints.${k}SignInText`) }}</span>
          </span>
        </label>
      </RadioGroup>
      <div v-if="signIn === 'with'" class="flex items-start gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-2.5" role="alert" data-testid="export-warning">
        <TriangleAlert class="mt-0.5 size-4 flex-none text-warn" /><span>{{ t('endpoints.withSignInWarn') }}</span>
      </div>
      <p class="text-xs text-t3">{{ t('endpoints.otpNote') }}</p>
      <Banner v-if="failure">{{ failure }}</Banner>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="open = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="busy || !endpoint" data-testid="export-save" @click="save"><Upload />{{ t('endpoints.saveFile') }}</Button>
    </template>
  </Dialog>
</template>
