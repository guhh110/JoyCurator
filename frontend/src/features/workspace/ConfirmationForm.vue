<script setup lang="ts">
import { Search, MoreHorizontal, FolderOpen, CircleAlert } from '@lucide/vue'
import EliText from '../../components/ui/EliText.vue'
import Button from '../../components/ui/Button.vue'
import Input from '../../components/ui/Input.vue'
import SegmentedControl from '../../components/ui/SegmentedControl.vue'
import ActionMenu from '../../components/ui/ActionMenu.vue'
import ActionMenuItem from '../../components/ui/ActionMenuItem.vue'
import ConfirmationWorkGroup from './ConfirmationWorkGroup.vue'
import type { Workspace } from './useWorkspace'
const props = defineProps<{ workspace: Workspace }>()
const w = props.workspace, s = w.confirmation
const { busy, locked, currentFiles, directory, status } = w
const { stale, loading, groups, session } = s
</script>
<template>
  <section class="flow-card confirm-main" aria-label="确认作品与季集">
    <header class="confirm-scope-toolbar">
      <div class="flow-current-path"><FolderOpen :size="16" /><EliText :text="directory || '全部目录'" ellipsis="start" /></div>
      <SegmentedControl :model-value="status" label="确认状态" :options="[{value:'pending',label:'待确认'},{value:'confirmed',label:'已确认'}]" :disabled="busy" @update:model-value="status = $event === 'confirmed' ? 'confirmed' : 'pending'" />
      <Input v-model="w.query.value" type="search" aria-label="搜索文件" placeholder="搜索文件" :disabled="busy"><template #icon><Search :size="14" /></template></Input>
      <ActionMenu align="end" :disabled="locked"><template #trigger><Button variant="ghost" size="sm" icon aria-label="目录更多操作"><MoreHorizontal :size="18" /></Button></template><ActionMenuItem :disabled="!currentFiles.length" @select="w.reidentification.open(currentFiles)">重新识别筛选结果</ActionMenuItem><ActionMenuItem @select="w.reloadDraft">重载草稿</ActionMenuItem></ActionMenu>
    </header>
    <div v-if="stale && s.open.value" class="confirm-stale"><CircleAlert :size="15" /><span>工作区已更新</span><Button size="sm" :disabled="busy" @click="w.reloadDraft">重新载入</Button></div>
    <ConfirmationWorkGroup v-for="group in groups" :key="`${session}:${group.key}`" :workspace="w" :group-key="group.key" />
    <div v-if="loading && !groups.length" class="flow-loading">正在读取文件与候选…</div>
    <p v-else-if="!currentFiles.length" class="flow-empty">{{ status === 'pending' ? '没有符合筛选条件的待确认文件' : '没有符合筛选条件的已确认文件' }}</p>
    <footer class="flow-footer confirm-footer"><Button :disabled="locked || !w.workGroups.value.length" @click="w.goStep(3)">前往预览（{{ w.ready.value.length }}）</Button></footer>
  </section>
</template>

<style src="./confirmation.css"></style>
