<script setup lang="ts">
import { computed } from 'vue'
import Button from '../../components/ui/Button.vue'
import Progress from '../../components/ui/Progress.vue'
import { FolderOpen, Plus, Trash2, LoaderCircle } from '@lucide/vue'
import EliText from '../../components/ui/EliText.vue'
import type { Workspace } from './useWorkspace'
const props = defineProps<{ workspace: Workspace }>()
const w = props.workspace
const { task, pendingPaths, sources, recognizing, busy, locked, files } = w
const visibleSources = computed(() => w.purpose === 'organize' && task.value?.state === 'completed' && !files.value.length && pendingPaths.value.length ? [] : sources.value)
const allCompleted = computed(() => !!task.value?.items.length && task.value.items.every(file => file.execution?.completed))
const emptyMessage = computed(() => {
  if (allCompleted.value) return w.purpose === 'scrape' ? '已完成刮削，可选择新的目录' : '已完成整理，可添加视频或目录开始新一轮'
  if (sources.value.length) return '没有找到视频文件'
  return w.purpose === 'scrape' ? '选择需要原地刮削的目录' : '选择需要整理的视频或目录'
})
</script>
<template>
  <section class="flow-card source-step" aria-label="添加文件">
    <header class="flow-section-head"><h2>{{ w.purpose === 'scrape' ? '选择 Target 目录' : '添加文件或目录' }}</h2><Button  :disabled="locked" @click="w.openBrowser"><Plus :size="16" />{{ w.purpose === 'scrape' ? '选择目录' : '添加' }}</Button></header>
    <ul class="flow-source-list">
      <li v-for="source in visibleSources" :key="source.path"><FolderOpen :size="18" /><EliText :text="source.path" ellipsis="start" /><span class="muted">{{ source.completed && !source.files ? '已完成' : `${source.files} 个视频` }}</span><Button variant="ghost" size="sm" icon :aria-label="`移除来源 ${source.path}`" :disabled="locked" @click="w.removeSource(source.path)"><Trash2 :size="15" /></Button></li>
      <li v-for="path in pendingPaths" :key="path"><FolderOpen :size="18" /><EliText :text="path" ellipsis="start" /><span class="muted">待识别</span><Button variant="ghost" size="sm" icon :aria-label="`移除待识别来源 ${path}`" :disabled="locked" @click="pendingPaths = pendingPaths.filter(value => value !== path)"><Trash2 :size="15" /></Button></li>
    </ul>
    <div v-if="recognizing" class="flow-progress" role="status"><LoaderCircle class="spin" :size="18" /><span>正在识别 {{ task?.completed }}/{{ task?.total || '扫描中' }}</span><Progress :value="task?.completed || 0" :max="task?.total || 1" /><Button variant="ghost" size="sm" :disabled="busy" @click="w.cancelRecognition">取消</Button></div>
    <div v-else-if="task && (task.error || task.state !== 'completed' || task.items.some(file => file.state === 'failed'))" class="flow-scan-error"><span>{{ task.error || '部分文件未完成识别' }}</span><Button variant="ghost" size="sm" :disabled="busy" @click="w.retry">重试未完成项</Button></div>
    <div v-else-if="!pendingPaths.length && !files.length" class="flow-empty"><FolderOpen :size="32" /><p>{{ emptyMessage }}</p></div>
    <footer class="flow-footer"><span class="muted">{{ files.length }} 个视频</span><Button v-if="pendingPaths.length" variant="primary" :disabled="locked" @click="w.scanSources">开始识别</Button><Button v-else variant="primary" :disabled="locked || !files.length" @click="w.goStep(2)">继续确认</Button></footer>
  </section>
</template>
