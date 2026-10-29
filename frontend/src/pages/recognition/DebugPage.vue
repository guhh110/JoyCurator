<script setup lang="ts">
import { useExtraction } from '../../features/extraction/useExtraction'
import ExtractionResult from '../../features/extraction/ExtractionResult.vue'
import { RouterLink } from 'vue-router'
import PageHeader from '../../components/ui/PageHeader.vue'
const { path, root, busy, error, notice, result, run, cancel } = useExtraction()
</script>
<template>
  <section class="debug-page" aria-label="识别调试页面">
    <PageHeader
      title="识别调试"
      eyebrow="PATH METADATA"
      description="绝对路径 → 当前规则链 → 文件元数据"
      ><template #actions
        ><RouterLink class="button" :to="{ name: 'automatic-recognition' }"
          >返回识别工作区</RouterLink
        ></template
      ></PageHeader
    >
    <form class="pane debug-form" @submit.prevent="run()">
      <label for="debug-path">完整文件路径</label
      ><input
        id="debug-path"
        v-model="path"
        required
        placeholder="/media/Show/Season 02/03.mkv"
        autocomplete="off"
        spellcheck="false"
        :disabled="busy"
      /><label for="debug-root">解析边界目录（可选）</label
      ><input
        id="debug-root"
        v-model="root"
        placeholder="源目录内默认源目录，否则使用 /"
        autocomplete="off"
        :disabled="busy"
      />
      <p class="muted tiny">
        支持不存在的模拟路径；只解析文本，不读取文件、不访问
        TMDB，不保存执行轨迹。
      </p>
      <div class="actions">
        <button v-if="busy" type="button" class="button" @click="cancel">
          取消请求</button
        ><button class="button primary" :disabled="busy">
          {{ busy ? '执行中…' : '开始调试' }}
        </button>
      </div>
    </form>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <ExtractionResult v-if="result" :result="result" />
  </section>
</template>
<style scoped>
.debug-page {
  display: grid;
  gap: 20px;
  min-width: 0;
}
.heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.heading h1 {
  margin: 8px 0;
}
.debug-form {
  padding: 24px;
}
.debug-form label:not(:first-child) {
  margin-top: 18px;
}
.debug-form p {
  line-height: 1.8;
}
.actions {
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  margin-top: 20px;
}
@media (max-width: 600px) {
  .heading {
    align-items: flex-start;
    flex-direction: column;
  }
  .debug-form {
    padding: 18px;
  }
}
</style>
