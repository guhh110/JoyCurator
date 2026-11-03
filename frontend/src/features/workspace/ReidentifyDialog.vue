<script setup lang="ts">
import Modal from '../../components/Modal.vue'
import EliText from '../../components/ui/EliText.vue'
import { canReidentify, type useReidentification } from './useReidentification'
defineProps<{ state: ReturnType<typeof useReidentification>; busy: boolean }>()
</script>
<template>
  <Modal v-if="state.context.value" title="重新识别文件" :locked="busy" @close="state.close">
    <p>使用当前保存的识别规则重新提取并匹配。人工指定的作品、分组和季集映射会保留；已提交或整理完成的文件不能重跑。</p>
    <div class="rerun-toolbar"><span>已选 {{ state.selected.value.length }}/{{ state.eligible.value.length }} 个文件</span><button class="text-button" :disabled="busy" @click="state.selectAll(true)">全选可用</button><button class="text-button" :disabled="busy" @click="state.selectAll(false)">清空</button></div>
    <div class="rerun-files">
      <label v-for="file in state.files.value" :key="file.path" class="rerun-file">
        <input v-model="state.selected.value" type="checkbox" :value="file.path" :disabled="busy || !canReidentify(file)" />
        <EliText :text="file.path" ellipsis="start" />
        <span v-if="!canReidentify(file)" class="muted tiny">不可重跑</span>
      </label>
    </div>
    <template #footer><button class="button" :disabled="busy" @click="state.close">取消</button><button class="button primary" :disabled="busy || !state.selected.value.length" @click="state.submit">重新识别 {{ state.selected.value.length }} 个文件</button></template>
  </Modal>
</template>
<style scoped>
.rerun-toolbar { display:flex; flex-wrap:wrap; align-items:center; gap:12px; margin:16px 0; font-size:12px; }
.rerun-toolbar span { margin-right:auto; }
.rerun-files { max-height:45vh; overflow:auto; }
.rerun-file { display:flex; align-items:center; gap:10px; padding:10px 0; border-bottom:1px solid var(--line); min-width:0; margin:0; }
.rerun-file input { width:auto; flex-shrink:0; }
.rerun-file .eli-text { flex:1; min-width:0; }
.rerun-file > span { flex-shrink:0; }
</style>
