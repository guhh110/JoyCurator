<script setup lang="ts">
import Button from '../components/ui/Button.vue'
import { onBeforeRouteLeave } from 'vue-router'
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Check, ChevronRight, FolderOpen, Folders } from '@lucide/vue'
import FileBrowser from '../components/FileBrowser.vue'
import QueryState from '../components/ui/QueryState.vue'
import EliText from '../components/ui/EliText.vue'
import SourceStep from '../features/workspace/SourceStep.vue'
import ConfirmationForm from '../features/workspace/ConfirmationForm.vue'
import PreviewStep from '../features/workspace/PreviewStep.vue'
import ReidentifyDialog from '../features/workspace/ReidentifyDialog.vue'
import { useWorkspace } from '../features/workspace/useWorkspace'
import '../features/workspace/workflow.css'
const props = withDefaults(defineProps<{ purpose?: 'organize' | 'scrape' }>(), { purpose: 'organize' })
const w = useWorkspace(props.purpose)
const { step, busy, locked, recognizing, browser, task, taskQuery, loading, directories, directory } = w
const directoriesOpen = ref(false)
const steps = ['添加文件', '确认', '预览']
defineExpose({ openBrowser: w.openBrowser, canLeave: w.discardDraft })
onBeforeRouteLeave(() => w.discardDraft())
function beforeUnload(event: BeforeUnloadEvent) { if (w.confirmation.dirty.value) { event.preventDefault(); event.returnValue = '' } }
onMounted(() => window.addEventListener('beforeunload', beforeUnload))
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
async function chooseDirectory(path: string) { await w.chooseDirectory(path); directoriesOpen.value = false }
</script>
<template>
  <section class="recognition-flow" :aria-label="purpose === 'scrape' ? '目录刮削' : '整理工作区'">
    <header class="flow-heading"><h1>{{ purpose === 'scrape' ? '目录刮削' : '整理' }}</h1><RouterLink :to="purpose === 'scrape' ? '/scrape/records' : '/records'">查看队列</RouterLink></header>
    <nav class="flow-steps" aria-label="操作步骤"><template v-for="(label,index) in steps" :key="label"><ChevronRight v-if="index" :size="16" aria-hidden="true" /><Button variant="ghost" class="flow-step" :aria-current="step === index + 1 ? 'step' : undefined" :disabled="locked || (index === 1 && !w.files.value.length) || (index === 2 && !w.workGroups.value.length)" @click="w.goStep((index + 1) as 1 | 2 | 3)"><span><Check v-if="index === 0 && task?.state === 'completed'" :size="14" /><template v-else>{{ index + 1 }}</template></span>{{ label }}</Button></template></nav>
    <QueryState v-if="taskQuery.error.value" :error="taskQuery.error.value" @retry="w.poll" />
    <div v-if="loading && !task" class="flow-card flow-empty">读取工作区…</div>
    <SourceStep v-else-if="step === 1 || recognizing" :workspace="w" />
    <div v-else-if="step === 2" class="flow-confirm-shell">
      <Button class="flow-mobile-directory" :aria-expanded="directoriesOpen" @click="directoriesOpen = !directoriesOpen"><FolderOpen :size="16" /><EliText :text="directory || '全部目录'" ellipsis="start" /><ChevronRight :size="15" /></Button>
      <aside class="flow-card flow-directories" :class="{open:directoriesOpen}" aria-label="目录导航">
        <header class="flow-section-head"><h2>目录</h2><span class="muted">{{ directories.length }}</span></header>
        <div>
          <Button variant="ghost" :aria-current="!directory ? 'true' : undefined" :disabled="locked || w.confirmation.loading.value" @click="chooseDirectory('')"><Folders :size="16" /><EliText text="全部" ellipsis="start" /><span>{{ w.files.value.length }}</span></Button>
          <Button variant="ghost" v-for="group in directories" :key="group.id" :aria-current="directory === group.id ? 'true' : undefined" :disabled="locked || w.confirmation.loading.value" @click="chooseDirectory(group.id)"><FolderOpen :size="16" /><EliText :text="group.id" ellipsis="start" /><span>{{ group.files.length }}</span></Button>
        </div>
      </aside>
      <ConfirmationForm :workspace="w" />
    </div>
    <PreviewStep v-else :workspace="w" />
    <ReidentifyDialog :state="w.reidentification" :busy="busy" />
    <FileBrowser v-if="browser" :directory="purpose === 'scrape'" :scope="purpose === 'scrape' ? 'target' : undefined" confirm-label="添加" @close="browser = false" @select="w.addPaths" />
  </section>
</template>
