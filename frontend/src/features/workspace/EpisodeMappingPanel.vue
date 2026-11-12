<script setup lang="ts">
import { computed, watch } from 'vue'
import { ArrowRight, Check, CircleAlert } from '@lucide/vue'
import Button from '../../components/ui/Button.vue'
import Input from '../../components/ui/Input.vue'
import Field from '../../components/ui/Field.vue'
import Select from '../../components/ui/Select.vue'
import SegmentedControl from '../../components/ui/SegmentedControl.vue'
import EliText from '../../components/ui/EliText.vue'
import VirtualList from '../../components/ui/VirtualList.vue'
import SidePanel from '../../components/ui/SidePanel.vue'
import { digitParts } from './confirmation'
import { notify } from '../../app/toast'
import type { ConfirmationGroup } from './useConfirmationGroup'
const props = defineProps<{ state: ConfirmationGroup; disabled: boolean }>()
const emit = defineEmits<{ close: [] }>()
const s = props.state
const { mode, samplePath, chosen, sourceId, source, sample, mappingKey, candidates, offset, mappingPreview } = s
const valid = computed(() => mappingPreview.value.filter(row => !row.error).length)
const parts = computed(() => digitParts(source.value?.text || ''))
watch([mode, () => sample.value?.file.path, sourceId], () => { mappingKey.value = '' })
const previewKey = (value: typeof mappingPreview.value[number]) => value.row.file.path
function apply() {
  s.applyMapping()
  notify(`已应用 ${valid.value} 个文件${valid.value < chosen.value.length ? `，${chosen.value.length - valid.value} 个需修正` : ''}`, 'success')
  emit('close')
}
</script>
<template>
  <SidePanel :open="true" title="调整集数" body-class="confirm-mapping-body" @update:open="value => { if (!value) emit('close') }">
    <section class="confirm-mapping-panel" aria-label="批量调整集数">
      <div class="confirm-mapping-settings" role="region" aria-label="映射设置" tabindex="0">
        <div class="confirm-mapping-toolbar">
          <SegmentedControl v-model="mode" label="集数来源" :disabled="disabled" :options="[{value:'suggestion',label:'识别建议'},{value:'digits',label:'原文取数'},{value:'candidate',label:'标注候选'}]" />
        </div>
        <div v-if="mode !== 'suggestion'" class="confirm-mapping-fields" :class="{'has-candidates':mode === 'candidate'}">
          <Field label="样本"><Select :model-value="sample?.file.path || ''" @update:model-value="samplePath = $event" label="取数样本" ellipsis="start" size="sm" :disabled="disabled" :options="chosen.map(row => ({value:row.file.path,label:row.file.path}))" /></Field>
          <Field v-if="mode === 'digits'" label="来源"><Select v-model="sourceId" label="原文来源" size="sm" :disabled="disabled" :options="(sample?.view.document.sources || []).map(value => ({value:value.id,label:value.depth === 0 ? '文件名' : `上 ${value.depth} 层目录`}))" /></Field>
          <Field v-else label="候选"><Select v-model="mappingKey" label="集数候选" placeholder="选择候选" ellipsis="end" size="sm" :disabled="disabled" :options="candidates.map(value => ({value:value.id,label:`${JSON.stringify(value.value)} · ${value.reason || value.ruleId}`,disabled:value.status !== 'valid'}))" /></Field>
        </div>
        <div v-if="mode === 'digits' && source" class="confirm-raw-source" role="region" aria-label="点击原文数字" tabindex="0"><template v-for="(part,index) in parts" :key="part.index"><span>{{ source.text.slice(index ? parts[index-1]!.end : 0,part.start) }}</span><Button size="sm" :variant="mappingKey === String(part.index) ? 'primary' : 'default'" :aria-pressed="mappingKey === String(part.index)" :aria-label="`取数字 ${part.text}`" :disabled="disabled" @click="mappingKey = String(part.index)">{{ part.text }}</Button><span v-if="index === parts.length - 1">{{ source.text.slice(part.end) }}</span></template><span v-if="!parts.length">{{ source.text }}</span></div>
      </div>
      <section class="confirm-mapping-preview" aria-label="映射预览">
        <header><strong>映射预览</strong><span class="confirm-mapping-count">{{ valid }}/{{ chosen.length }} 个可取数</span><Field class="confirm-mapping-offset" label="偏移" for="episode-offset"><Input id="episode-offset" v-model="offset" size="sm" inputmode="numeric" aria-label="集数偏移" :disabled="disabled" /></Field></header>
        <VirtualList class="confirm-mapping-list" :items="mappingPreview" :item-key="previewKey" label="文件映射结果">
          <template #default="{ item: value }"><div class="confirm-mapping-row"><EliText :text="value.row.file.path" ellipsis="start" /><span class="confirm-taken">{{ value.error ? '—' : value.original }}</span><ArrowRight :size="13" /><span :class="{'confirm-mapping-error':value.error}" class="confirm-mapped"><template v-if="value.error"><CircleAlert :size="13" />{{ value.error }}</template><template v-else>{{ value.text }}</template></span></div></template>
          <template #empty><p class="confirm-mapping-empty">请选择文件</p></template>
        </VirtualList>
      </section>
    </section>
    <template #footer><span>{{ chosen.length }} 个文件</span><Button @click="emit('close')">取消</Button><Button variant="primary" :disabled="disabled || !s.canApplyMapping.value" @click="apply"><Check :size="14" />应用到表格</Button></template>
  </SidePanel>
</template>
