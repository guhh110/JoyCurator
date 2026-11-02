<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { CircleAlert } from '@lucide/vue'
import { bytes, name } from '../../api'
import Modal from '../../components/Modal.vue'
import Select from '../../components/ui/Select.vue'
import Tooltip from '../../components/ui/Tooltip.vue'
import EliText from '../../components/ui/EliText.vue'
import type { useWorkspacePreview } from './useWorkspacePreview'
const props = defineProps<{ state: ReturnType<typeof useWorkspacePreview> }>()
const { preview, previewPage, previewRows, duplicatePolicy, previewReady, prepare, changeDuplicatePolicy, execute } = props.state
const busy = computed(() => props.state.pending.value)
const modes: Record<string, string> = { copy: '复制', move: '移动', hardlink: '硬链接' }
// The same issue is often reported once per file; show it once with a count.
const previewIssues = computed(() => {
  const counts = new Map<string, number>()
  for (const issue of preview.value?.issues || [])
    if (!conflictPath(issue)) counts.set(issue, (counts.get(issue) || 0) + 1)
  return [...counts]
})
const previewRowsElement = ref<HTMLElement | null>(null)
watch(previewPage, async () => {
  await nextTick()
  previewRowsElement.value?.closest('.modal-body')?.scrollTo({ top: 0 })
})
function conflictPath(issue: string) {
  return /^(?:目标已存在|目标重复|作品之间目标重复)：(.+)$/.exec(issue)?.[1] || ''
}
function conflictTargets(issues: string[]) {
  return [...new Set(issues.map(conflictPath).filter(Boolean))]
}
</script>
<template>
  <Modal v-if="preview" title="确认多作品整理预览" wide :locked="busy" @close="preview = null"
    ><template #subtitle><p class="auto-preview-summary">{{ preview.groups.length }} 部作品 · {{ preview.rows.length }} 个视频 <span aria-hidden="true">·</span> {{ modes[preview.mode] }} · 需要 {{ bytes(preview.required) }}</p></template>
    <div v-if="previewIssues.length" class="notice error">
      <p v-for="[issue, count] in previewIssues" :key="issue">
        {{ issue }}<b v-if="count > 1" class="auto-issue-count"> × {{ count }}</b>
      </p>
      <p>关闭预览，调整选中文件或分组后重新预览。</p>
    </div>
    <div ref="previewRowsElement" class="auto-preview-rows">
      <article v-for="row in previewRows" :key="row.source">
        <strong :title="row.source">{{ name(row.source) }}</strong><span class="auto-preview-action" :class="row.action">{{ row.action === 'skip' ? '跳过重复' : row.action === 'overwrite' ? '直接覆盖' : '新增' }}</span>
        <p class="muted tiny auto-path"><EliText :text="row.source" ellipsis="start" /></p>
        <p class="auto-path auto-preview-target" :class="{ 'auto-row-issue': conflictTargets([...row.issues, ...preview.issues]).includes(row.target) }">
          <span aria-hidden="true">→</span><EliText :text="row.target" ellipsis="start" />
          <Tooltip v-if="conflictTargets([...row.issues, ...preview.issues]).includes(row.target)" content="已存在">
            <CircleAlert :size="16" tabindex="0" role="img" aria-label="已存在" />
          </Tooltip>
        </p>
        <p v-for="target in conflictTargets(row.issues).filter(target => target !== row.target)" :key="target" class="auto-path auto-preview-target auto-row-issue">
          <span aria-hidden="true">→</span><EliText :text="target" ellipsis="start" />
          <Tooltip content="已存在">
            <CircleAlert :size="16" tabindex="0" role="img" aria-label="已存在" />
          </Tooltip>
        </p>
        <p v-if="row.artifacts.length" class="muted tiny">
          {{ row.artifacts.join(' · ') }}
        </p>
        <p
          v-for="issue in new Set(row.issues.filter(issue => !conflictPath(issue)))"
          :key="issue"
          class="auto-row-issue tiny"
        >
          {{ issue }}
        </p>
      </article>
    </div>
    <div v-if="preview.rows.length > 30" class="auto-pagination">
      <button
        class="text-button"
        :disabled="previewPage === 1"
        @click="previewPage--"
      >
        上一页</button
      ><span>{{ previewPage }}/{{ Math.ceil(preview.rows.length / 30) }}</span
      ><button
        class="text-button"
        :disabled="previewPage * 30 >= preview.rows.length"
        @click="previewPage++"
      >
        下一页
      </button>
    </div>
    <template #footer
      ><div class="auto-duplicate-policy">
        <label for="duplicate-policy">重复文件处理</label>
        <Select id="duplicate-policy" :model-value="duplicatePolicy" label="重复文件处理" :disabled="busy"
          :options="[{ value: 'skip', label: '跳过重复' }, { value: 'overwrite', label: '覆盖重复' }]" @update:model-value="changeDuplicatePolicy" />
        <span class="auto-duplicate-count muted tiny" role="status">{{ busy ? '正在更新预览…' : preview.duplicatePolicy !== duplicatePolicy ? '预览未更新，请重试' : `跳过 ${preview.rows.filter(row => row.action === 'skip').length} · 覆盖 ${preview.rows.filter(row => row.action === 'overwrite').length}` }}</span>
        <button v-if="preview.duplicatePolicy !== duplicatePolicy && !busy" class="text-button" @click="prepare">重新预览</button>
      </div>
      <div class="auto-preview-confirm">
        <span v-if="duplicatePolicy === 'overwrite'" class="auto-overwrite-warning">覆盖已有文件后，撤销无法恢复原内容。</span>
        <button
        class="button primary"
        :disabled="!previewReady"
        @click="execute"
      >
        {{ preview.rows.every(row => row.action === 'skip') ? '登记刮削待办' : '确认整理' }}
      </button></div></template
    ></Modal
  >
</template>
