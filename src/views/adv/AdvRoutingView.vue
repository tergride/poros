<script setup lang="ts">
import PresetsPage from '@/components/settings/PresetsPage.vue'
import { useCoreStore } from '@/stores/core'

const core = useCoreStore()
</script>

<template>
  <!-- The presets as in the simple mode, until the editor of rules takes their place. -->
  <div class="min-w-0 flex-1 overflow-auto px-7 pt-[22px] pb-7">
    <PresetsPage v-if="core.settings" />
  </div>
</template>
