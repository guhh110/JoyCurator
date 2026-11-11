<script setup lang="ts">
import Button from '../../components/ui/Button.vue'
import Progress from '../../components/ui/Progress.vue'
import { FolderOpen, Plus, Trash2, LoaderCircle } from '@lucide/vue'
import EliText from '../../components/ui/EliText.vue'
import type { Workspace } from './useWorkspace'
const props = defineProps<{ workspace: Workspace }>()
const w = props.workspace
const { task, pendingPaths, sources, recognizing, busy, locked, files } = w
</script>
<template>
  <section class="flow-card source-step" aria-label="添加文件">
    <header class="flow-section-head"><h2>{{ w.purpose === 'scrape' ? '选择 Target 目录' : '添加文件或目录' }}</h2><Button  :disabled="locked" @click="w.openBrowser"><Plus :size="16" />{{ w.purpose === 'scrape' ? '选择目录' : '添加' }}</Button></header>
    <div v-if="!sources.length && !pendingPaths.length" class="flow-empty"><FolderOpen :size="32" /><p>{{ w.purpose === 'scrape' ? '选择需要原地刮削的目录' : '选择需要整理的视频或目录' }}</p></div>
    <ul class="flow-source-list">
      <li v-for="source in sources" :key="source.path"><FolderOpen :size="18" /><EliText :text="source.path" ellipsis="start" /><span class="muted">{{ source.files }} 个视频</span><Button variant="ghost" size="sm" icon :aria-label="`移除来源 ${source.path}`" :disabled="locked" @click="w.removeSource(source.path)"><Trash2 :size="15" /></Button></li>
      <li v-for="path in pendingPaths" :key="path"><FolderOpen :size="18" /><EliText :text="path" ellipsis="start" /><span class="muted">待识别</span><Button variant="ghost" size="sm" icon :aria-label="`移除待识别来源 ${path}`" :disabled="locked" @click="pendingPaths = pendingPaths.filter(value => value !== path)"><Trash2 :size="15" /></Button></li>
    </ul>
    <div v-if="recognizing" class="flow-progress" role="status"><LoaderCircle class="spin" :size="18" /><span>正在识别 {{ task?.completed }}/{{ task?.total || '扫描中' }}</span><Progress :value="task?.completed || 0" :max="task?.total || 1" /><Button variant="ghost" size="sm" :disabled="busy" @click="w.cancelRecognition">取消</Button></div>
    <div v-else-if="task && (task.error || task.state !== 'completed' || task.items.some(file => file.state === 'failed'))" class="flow-scan-error"><span>{{ task.error || '部分文件未完成识别' }}</span><Button variant="ghost" size="sm" :disabled="busy" @click="w.retry">重试未完成项</Button></div>
    <p v-else-if="task && !files.length && !pendingPaths.length" class="flow-empty">没有找到视频文件</p>
    <footer class="flow-footer"><span class="muted">{{ files.length }} 个视频</span><Button v-if="pendingPaths.length" variant="primary" :disabled="locked" @click="w.scanSources">开始识别</Button><Button v-else variant="primary" :disabled="locked || !files.length" @click="w.goStep(2)">继续确认</Button></footer>
  </section>
</template>
