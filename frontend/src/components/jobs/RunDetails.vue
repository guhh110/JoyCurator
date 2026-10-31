<script setup lang="ts">
import { computed } from 'vue'
import { Undo2 } from '@lucide/vue'
import Modal from '../Modal.vue'
import QueryState from '../ui/QueryState.vue'
import EliText from '../ui/EliText.vue'
import { useRuntime } from '../../app/runtime'
import {
  date,
  kindNames,
  modeNames,
  states,
} from '../../features/records/presentation'
const runtime = useRuntime()
const {
  detailId,
  detailTitle,
  detailQuery,
  undoing,
  executing: running,
  actionBusy: busy,
  recordAction,
} = runtime
const detail = computed(() => detailQuery.data.value)
</script>
<template>
  <Modal
    v-if="detailId"
    :title="detail?.title || detailTitle"
    wide
    @close="detailId = ''"
    ><QueryState
      :loading="detailQuery.isPending.value"
      :error="detailQuery.error.value"
      @retry="detailQuery.refetch()"
      ><template v-if="detail"
        ><div class="preview-summary">
          <span class="state-pill" :class="detail.state">{{
            states[detail.state]
          }}</span
          ><span>{{ modeNames[detail.mode] }}</span
          ><span>{{ date(detail.created) }}</span
          ><span>{{ detail.completed }}/{{ detail.total }} 项</span>
        </div>
        <p v-if="detail.error" class="notice error preserve-lines">
          {{ detail.error }}
        </p>
        <div class="operation-list">
          <article v-for="(op, index) in detail.ops" :key="index">
            <span class="operation-kind">{{ kindNames[op.kind] }}</span>
            <div>
              <code><EliText :text="op.target" ellipsis="start" /></code>
              <p v-if="op.error" class="danger">{{ op.error }}</p>
            </div>
            <span
              class="tiny"
              :class="op.status === 'completed' ? 'green' : 'muted'"
              >{{ states[op.status] || op.status }}</span
            >
          </article>
        </div></template
      ></QueryState
    ><template v-if="detail" #footer
      ><span class="muted tiny">文件变化时会停止操作</span>
      <div class="footer-buttons">
        <button
          v-if="
            ['failed', 'interrupted', 'cancelled'].includes(detail.state) &&
            !detail.undo
          "
          class="button"
          :disabled="busy || running"
          @click="recordAction(detail, 'retry')"
        >
          重试未完成项</button
        ><button
          v-if="!['undone', 'running', 'queued'].includes(detail.state)"
          class="button"
          :disabled="busy || running"
          @click="undoing = detail"
        >
          <Undo2 :size="15" />{{ detail.undo ? '继续撤销' : `撤销本次${detail.purpose === 'scrape' ? '刮削' : '整理'}` }}
        </button>
      </div></template
    ></Modal
  ><Modal
    v-if="undoing"
    :title="`撤销这次${undoing.purpose === 'scrape' ? '刮削' : '整理'}？`"
    :locked="busy"
    @close="undoing = null"
    ><p>将撤销「{{ undoing.title }}」的文件操作。</p>
    <p class="muted">
      {{
        undoing.mode === 'move'
          ? '视频和字幕将恢复到原路径。原路径被占用时停止对应操作。'
          : '将移除本次创建的目标文件，来源文件保持原样。'
      }}目标内容被修改时会停止，不会删除被替换的文件。
      覆盖过的旧文件没有备份，撤销不会恢复其旧内容。
    </p>
    <template #footer
      ><button class="button" @click="undoing = null">保留整理结果</button
      ><button
        class="button primary"
        :disabled="busy"
        @click="recordAction(undoing!, 'undo')"
      >
        确认撤销
      </button></template
    ></Modal
  >
</template>
