<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ArrowRight, Check, CircleAlert } from '@lucide/vue'
import Button from '../../components/ui/Button.vue'
import Input from '../../components/ui/Input.vue'
import Field from '../../components/ui/Field.vue'
import Select from '../../components/ui/Select.vue'
import SegmentedControl from '../../components/ui/SegmentedControl.vue'
import EliText from '../../components/ui/EliText.vue'
import Pagination from '../../components/ui/Pagination.vue'
import SidePanel from '../../components/ui/SidePanel.vue'
import { digitParts } from './confirmation'
import { notify } from '../../app/toast'
import type { useConfirmation } from './useConfirmation'
const props = defineProps<{ state: ReturnType<typeof useConfirmation>; disabled: boolean }>()
const emit = defineEmits<{ close: [] }>()
const s = props.state
const { mode, samplePath, chosen, sourceId, source, sample, mappingKey, candidates, offset, mappingPreview } = s
const page = ref(1)
const visible = computed(() => mappingPreview.value.slice((page.value - 1) * 5, page.value * 5))
const valid = computed(() => mappingPreview.value.filter(row => !row.error).length)
const parts = computed(() => digitParts(source.value?.text || ''))
watch([mode, samplePath, sourceId], () => { mappingKey.value = ''; page.value = 1 })
function apply() {
  s.applyMapping()
  notify(`已应用 ${valid.value} 个文件${valid.value < chosen.value.length ? `，${chosen.value.length - valid.value} 个需修正` : ''}`, 'success')
  emit('close')
}
</script>
<template>
  <SidePanel :open="true" title="调整集数" @update:open="value => { if (!value) emit('close') }">
    <section class="confirm-tool-panel" aria-label="批量调整集数">
    <SegmentedControl v-model="mode" label="集数来源" :disabled="disabled" :options="[{value:'suggestion',label:'识别建议'},{value:'digits',label:'原文取数'},{value:'candidate',label:'标注候选'}]" />
    <div v-if="mode !== 'suggestion'" class="confirm-mapping-fields">
      <Field label="样本文件"><Select v-model="samplePath" label="取数样本" ellipsis="start" :disabled="disabled" :placeholder="sample?.file.path" :options="chosen.map(row => ({value:row.file.path,label:row.file.path}))" /></Field>
      <Field v-if="mode === 'digits'" label="原文来源"><Select v-model="sourceId" label="原文来源" :disabled="disabled" :options="(sample?.view.document.sources || []).map(value => ({value:value.id,label:value.depth === 0 ? '文件名' : `上 ${value.depth} 层目录`}))" /></Field>
      <Field v-else label="集数候选"><Select v-model="mappingKey" label="集数候选" placeholder="选择候选" :disabled="disabled" :options="candidates.map(value => ({value:value.id,label:`${JSON.stringify(value.value)} · ${value.reason || value.ruleId}`,disabled:value.status !== 'valid'}))" /></Field>
    </div>
    <div v-if="mode === 'digits' && source" class="confirm-raw-source" aria-label="点击原文数字"><template v-for="(part,index) in parts" :key="part.index"><span>{{ source.text.slice(index ? parts[index-1]!.end : 0,part.start) }}</span><Button size="sm" :variant="mappingKey === String(part.index) ? 'primary' : 'default'" :aria-pressed="mappingKey === String(part.index)" :aria-label="`取数字 ${part.text}`" :disabled="disabled" @click="mappingKey = String(part.index)">{{ part.text }}</Button><span v-if="index === parts.length - 1">{{ source.text.slice(part.end) }}</span></template><span v-if="!parts.length">{{ source.text }}</span></div>
    <Field class="confirm-mapping-offset" label="集数偏移" for="episode-offset"><Input id="episode-offset" v-model="offset" inputmode="numeric" aria-label="集数偏移" :disabled="disabled" /></Field>
    <div class="confirm-mapping-preview">
      <header><strong>映射预览</strong><span>{{ valid }}/{{ chosen.length }} 个可取数</span></header>
      <div v-for="value in visible" :key="value.row.file.path" class="confirm-mapping-row"><EliText :text="value.row.file.path" ellipsis="start" /><span class="confirm-taken">{{ value.error ? '—' : value.original }}</span><ArrowRight :size="13" /><span :class="{'confirm-mapping-error':value.error}" class="confirm-mapped"><template v-if="value.error"><CircleAlert :size="13" />{{ value.error }}</template><template v-else>{{ value.text }}</template></span></div>
      <Pagination v-if="chosen.length > 5" :page="page" :total="chosen.length" :page-size="5" @change="page = $event" />
    </div>
    </section>
    <template #footer><span>{{ chosen.length }} 个文件</span><Button @click="emit('close')">取消</Button><Button variant="primary" :disabled="disabled || !s.canApplyMapping.value" @click="apply"><Check :size="14" />应用到表格</Button></template>
  </SidePanel>
</template>
