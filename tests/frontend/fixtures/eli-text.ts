import { createApp, defineComponent, h, ref } from 'vue'
import EliText from '../../../frontend/src/components/ui/EliText.vue'
import Select from '../../../frontend/src/components/ui/Select.vue'
import Modal from '../../../frontend/src/components/Modal.vue'
import '../../../frontend/src/style.css'

const longPath = '/media/collections/series/2024/高码率珍藏版/Season 01/Show.S01E00.2160p.mkv'
const windowsPath = 'C:\\Media\\Collections\\剧集\\العربية\\Season 01\\Show [2024] (S01E00).mkv'
createApp(defineComponent({ setup() {
  const narrow = ref(true), short = ref(false), start = ref(true), modal = ref(false)
  const selected = ref(longPath)
  const options = [longPath, windowsPath].map(value => ({ value, label: value }))
  const row = (label: string, text: string, ellipsis: 'start' | 'end') => h('section', { style: 'display:grid;gap:8px;margin:20px 0;min-width:0' }, [
    h('strong', { style: 'font-size:12px' }, label),
    h(EliText, { text, ellipsis, style: 'width:100%;padding:10px;border:1px solid #e4e3ed;border-radius:6px;background:white', 'data-testid': label }),
  ])
  return () => h('main', { style: 'min-height:100vh;padding:28px;background:#f8f7fb' }, [
    h('h1', { style: 'font-size:20px;margin-bottom:16px' }, 'EliText · 路径省略'),
    h('div', { style: 'display:flex;gap:8px;flex-wrap:wrap' }, [
      h('button', { class: 'button', onClick: () => { narrow.value = !narrow.value } }, '切换宽度'),
      h('button', { class: 'button', onClick: () => { short.value = !short.value } }, '切换长短路径'),
      h('button', { class: 'button', onClick: () => { start.value = !start.value } }, '切换省略方向'),
      h('button', { class: 'button', onClick: () => { modal.value = true } }, '弹窗检查'),
    ]),
    h('div', { style: `width:${narrow.value ? '310px' : '100%'};max-width:100%;margin-top:20px` }, [
      row('前省略 · 保留文件名', short.value ? '/a.mkv' : longPath, start.value ? 'start' : 'end'),
      row('后省略 · 保留开头', longPath, 'end'),
      row('混合文字路径', windowsPath, 'start'),
      h(Select, { modelValue: selected.value, 'onUpdate:modelValue': value => { selected.value = value }, label: '路径来源', options, ellipsis: 'start', style: 'width:100%' }),
    ]),
    modal.value ? h(Modal, { title: '路径弹窗', onClose: () => { modal.value = false } }, { default: () => [
      row('弹窗内完整路径', longPath, 'start'),
      h(Select, { modelValue: selected.value, 'onUpdate:modelValue': value => { selected.value = value }, label: '弹窗路径来源', options, ellipsis: 'start', style: 'width:100%' }),
    ] }) : null,
  ])
} })).mount('#app')
