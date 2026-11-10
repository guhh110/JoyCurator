<script setup lang="ts">
import { useRouter } from 'vue-router'
import PageHeader from '../../components/ui/PageHeader.vue'
import ExtractionResult from '../../features/extraction/ExtractionResult.vue'
import { useExtraction } from '../../features/extraction/useExtraction'
import type { ScriptEvent } from '../../features/extraction/model'
const router = useRouter(),
  { path, root, busy, result, run, cancel } = useExtraction()
function edit(ruleId: string, event?: ScriptEvent) {
  void router.push({
    name: 'recognition-rule-edit',
    params: { ruleId },
    query: event?.line ? { line: event.line } : {},
  })
}
</script>
<template>
  <section class="debug-page">
    <PageHeader title="识别调试"
      ><template #actions
        ><button
          class="button"
          @click="router.push({ name: 'automatic-recognition' })"
        >
          返回整理工作区
        </button></template
      ></PageHeader
    >
    <form class="pane debug-form" @submit.prevent="run()">
      <label for="debug-paths">目录内文件路径（每行一个）</label
      ><textarea
        id="debug-paths"
        v-model="path"
        rows="6"
        placeholder="/source/Show/Season 2/01.mkv&#10;/source/Show/Season 2/02.mkv"
        spellcheck="false"
        required
      /><label for="debug-root">解析边界目录</label
      ><input id="debug-root" v-model="root" placeholder="留空使用源目录" />
      <p class="tiny muted">
        仅解析输入文本；不读取文件或访问
        TMDB。请提供完整目录的路径，以便比较片段。
      </p>
      <div class="actions">
        <button v-if="busy" class="button" type="button" @click="cancel()">
          取消</button
        ><button class="button primary" :disabled="busy">
          {{ busy ? '调试中…' : '开始调试' }}
        </button>
      </div>
    </form>
    <ExtractionResult v-if="result" :result="result" editable @locate="edit" />
  </section>
</template>
<style scoped>
.debug-page {
  display: grid;
  gap: 20px;
  min-width: 0;
}
.debug-form {
  padding: 24px;
  display: grid;
  gap: 12px;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
textarea {
  font-family: monospace;
}
</style>
