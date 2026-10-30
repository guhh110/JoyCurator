<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterView, useRouter } from 'vue-router'
import QueryState from './components/ui/QueryState.vue'
import ConfirmDialog from './components/ui/ConfirmDialog.vue'
const router = useRouter(),
  ready = ref(false)
onMounted(async () => {
  await router.isReady()
  ready.value = true
})
</script>
<template>
  <RouterView />
  <ConfirmDialog />
  <div v-if="!ready" class="boot"><QueryState loading /></div>
</template>
