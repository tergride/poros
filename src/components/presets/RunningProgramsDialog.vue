<script setup lang="ts">
import { AppWindow, LoaderCircle, RotateCw, Search } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Banner, Button, Dialog, Input, vTooltip } from '@/components/ui'
import { useFailure } from '@/composables/useReason'
import { programsFor, programValue } from '@/lib/rules'
import type { RunningProgram } from '@/platform/core-types'
import { CondProcessPath, MethodProgramsRunning } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// The programs running now, for the value of a rule by program: a click gives its file name or its path, as the kind
// asks. The core reads them anew at each opening and by «Refresh», those of the system after the user's. Enter in the
// search takes the first one shown; its default is kept from the button the focus goes back to, which would open the
// window again.
const open = defineModel<boolean>('open', { default: false })
const props = defineProps<{ kind: string }>()
const emit = defineEmits<{ pick: [value: string] }>()
const core = useCoreStore()
const failure = useFailure()
const { t } = useI18n()

const programs = ref<RunningProgram[]>([])
const loading = ref(false)
const problem = ref('')
const query = ref('')

async function load(): Promise<void> {
  if (loading.value) return
  loading.value = true
  problem.value = ''
  try {
    programs.value = await core.core().call(MethodProgramsRunning)
  } catch (error) {
    problem.value = failure(error)
  } finally {
    loading.value = false
  }
}

watch(
  open,
  (now) => {
    if (!now) return
    query.value = ''
    void load()
  },
  { immediate: true },
)

const shown = computed(() => programsFor(programs.value, props.kind, query.value))
const groups = computed(() =>
  [
    { key: 'own', programs: shown.value.filter((p) => !p.system) },
    { key: 'system', programs: shown.value.filter((p) => p.system) },
  ].filter((g) => g.programs.length > 0),
)

function pick(program: RunningProgram | undefined): void {
  if (!program) return
  emit('pick', programValue(props.kind, program))
  open.value = false
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('rules.programs.title')" class="h-[600px] w-[600px]">
    <template #icon><AppWindow /></template>
    <!-- The search stays over the list as it scrolls. -->
    <div class="sticky top-0 z-[1] -mt-1 bg-s2 pt-1 pb-1">
      <div class="flex gap-2">
        <div class="relative min-w-0 flex-1">
          <Search class="absolute top-2.5 left-[11px] size-4 text-t3" />
          <Input
            v-model="query"
            class="pl-[33px]"
            :placeholder="t('rules.programs.search')"
            :aria-label="t('rules.programs.search')"
            data-testid="programs-search"
            @keydown.enter.prevent="pick(shown[0])"
          />
        </div>
        <Button size="icon" :aria-label="t('rules.programs.refresh')" v-tooltip="t('rules.programs.refresh')" data-testid="programs-refresh" @click="load">
          <RotateCw :class="loading && 'animate-spin'" />
        </Button>
      </div>
      <p class="mt-2 text-xs text-t3">{{ t(kind === CondProcessPath ? 'rules.programs.hintPath' : 'rules.programs.hintName') }}</p>
    </div>
    <Banner v-if="problem" class="mt-2">{{ problem }}</Banner>
    <p v-else-if="loading && programs.length === 0" class="flex items-center justify-center gap-2 py-6 text-t3">
      <LoaderCircle class="size-4 animate-spin" />{{ t('rules.programs.loading') }}
    </p>
    <p v-else-if="groups.length === 0" class="py-6 text-center text-t3" data-testid="programs-nothing">{{ t('rules.programs.nothing') }}</p>
    <template v-for="group in groups" :key="group.key">
      <div class="px-2.5 pt-2.5 pb-1 text-[11.5px] text-t3">{{ t(`rules.programs.${group.key}`) }}</div>
      <button
        v-for="program in group.programs"
        :key="`${program.path}:${program.name}`"
        type="button"
        class="flex min-h-[34px] w-full cursor-pointer flex-col justify-center rounded-md px-2.5 py-1.5 text-left hover:bg-s4"
        data-testid="program"
        @click="pick(program)"
      >
        <span class="truncate text-[13px] text-t1">{{ program.name }}</span>
        <span v-if="program.path" class="truncate font-mono text-[11.5px] text-t3" v-tooltip.cut="program.path">{{ program.path }}</span>
      </button>
    </template>
    <template #footer>
      <Button variant="ghost" class="ml-auto" @click="open = false">{{ t('common.cancel') }}</Button>
    </template>
  </Dialog>
</template>
