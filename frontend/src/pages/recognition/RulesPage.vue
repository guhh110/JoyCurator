<script setup lang="ts">
import { useRouter } from 'vue-router'
import RecognitionRules from '../../features/extraction/RecognitionRules.vue'
const router = useRouter()
</script>
<template>
  <RecognitionRules @back="router.push({ name: 'automatic-recognition' })" />
</template>
