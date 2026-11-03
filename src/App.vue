<script setup lang="ts">
import TitleBar from '@/components/shell/TitleBar.vue'
</script>

<template>
  <div class="flex h-full flex-col">
    <TitleBar />
    <main class="min-h-0 flex-1">
      <RouterView />
    </main>
  </div>
</template>
