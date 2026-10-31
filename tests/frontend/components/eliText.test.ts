import { readFileSync } from 'node:fs'
import { h } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { describe, expect, it } from 'vitest'
import EliText from '../../../frontend/src/components/ui/EliText.vue'
import Select from '../../../frontend/src/components/ui/Select.vue'

describe('EliText ellipsis direction', () => {
  it('defaults to end ellipsis without changing the complete text', async () => {
    const text = '/media/剧集/Season 1/Show.S01E00.mkv'
    const html = await renderToString(h(EliText, { text }))
    expect(html).toContain('data-ellipsis="end"')
    expect(html).toContain(`<bdi dir="ltr">${text}</bdi>`)
  })
  it.each(['start', 'end'] as const)('supports %s ellipsis and preserves mixed scripts and punctuation', async ellipsis => {
    const text = 'C:\\媒体\\العربية\\Show [2024] (S01E00).mkv'
    const html = await renderToString(h(EliText, { text, ellipsis, side: 'bottom' }))
    expect(html).toContain(`data-ellipsis="${ellipsis}"`)
    expect(html).toContain(`<bdi dir="ltr">${text}</bdi>`)
    expect(html).not.toContain('tabindex="0"')
  })
  it('renders path selections with start ellipsis without adding a nested tab stop', async () => {
    const path = '/media/collections/long-directory/Season 1'
    const html = await renderToString(h(Select, { modelValue: path, options: [{ value: path, label: path }], label: '来源', ellipsis: 'start' }))
    expect(html).toContain('role="combobox"')
    expect(html).toContain('data-ellipsis="start"')
    expect(html).toContain(`<bdi dir="ltr">${path}</bdi>`)
    expect(html).not.toMatch(/class="eli-text-content"[^>]*tabindex=/)
  })
  it('keeps ordinary select labels unchanged', async () => {
    const html = await renderToString(h(Select, { modelValue: '1', options: [{ value: '1', label: '第一季' }], label: '季度' }))
    expect(html).toContain('aria-label="季度"')
    expect(html).not.toContain('eli-text-content')
  })
  it('keeps the placeholder for an empty path select', async () => {
    const html = await renderToString(h(Select, { options: [], label: '来源', placeholder: '选择来源', ellipsis: 'start' }))
    expect(html).toContain('选择来源')
    expect(html).not.toContain('eli-text-content')
  })
  it('uses start ellipsis at all dedicated path display sites', () => {
    const paths = [
      'pages/AutomaticRecognitionPage.vue', 'pages/SettingsPage.vue', 'components/jobs/RunDetails.vue',
      'features/automatic/EpisodeOffsetDialog.vue', 'features/extraction/ExtractionHistory.vue',
      'features/extraction/ExtractionHistoryDetail.vue', 'features/extraction/RuleExamples.vue',
      'features/extraction/ExtractionResult.vue', 'pages/ScrapePage.vue',
    ]
    for (const path of paths) {
      const source = readFileSync(new URL(`../../../frontend/src/${path}`, import.meta.url), 'utf8')
      const displays = source.match(/<EliText\b[^>]*\/>/g) ?? []
      expect(displays.length, path).toBeGreaterThan(0)
      for (const display of displays) expect(display, path).toContain('ellipsis="start"')
    }
  })
})
