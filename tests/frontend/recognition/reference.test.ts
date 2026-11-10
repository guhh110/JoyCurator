import { h } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { describe, it, expect } from 'vitest'
import ScriptReference from '../../../frontend/src/features/extraction/ScriptReference.vue'
import {
  scriptFunctions,
  scriptContextFields,
} from '../../../frontend/src/features/extraction/scriptReference'
describe('stage script contracts', () => {
  it.each(['tokenize', 'annotate', 'resolve'] as const)(
    'documents %s without legacy returns',
    async (stage) => {
      const html = await renderToString(
        h(ScriptReference, { label: '脚本', editorId: 'code', stage }),
      )
      expect(html).toContain('function extract(ctx)')
      expect(html).toContain('UTF-16')
      expect(html).toContain('250 ms')
      expect(html).toContain('64 MiB')
      expect(html).not.toContain('matchedText')
      expect(html).not.toContain('overwrite')
      expect(html).not.toContain(' open')
      if (stage === 'resolve') expect(html).toContain('跨文件引用')
      if (stage === 'annotate') expect(html).toContain('strength')
      if (stage === 'tokenize') expect(html).toContain('parentId')
    },
  )
  it('completion metadata describes current interfaces', () => {
    expect(scriptFunctions.map((f) => f.name)).toEqual([
      'reSearch',
      'reFindAll',
      'reSub',
      'trace',
      'print',
    ])
    expect(scriptContextFields.map((f) => f.name)).toContain('targetFileIds')
    expect(scriptContextFields.map((f) => f.name)).not.toContain('metadata')
  })
})
