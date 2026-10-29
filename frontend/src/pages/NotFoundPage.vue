<script setup lang="ts">
import { RouterLink } from 'vue-router'
import PageHeader from '../components/ui/PageHeader.vue'
</script>
<template>
  <section>
    <PageHeader
      title="页面不存在"
      eyebrow="404"
      description="这个地址没有对应的页面，可以从侧边导航继续整理。"
    /><RouterLink class="button primary" :to="{ name: 'workbench' }"
      >返回整理工作台</RouterLink
    >
  </section>
</template>
