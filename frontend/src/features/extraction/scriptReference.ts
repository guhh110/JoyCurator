export interface ScriptFieldReference {
  name: string
  type: string
  summary: string
  description: string
}
export interface ScriptFunctionReference {
  name: string
  signature: string
  summary: string
  description: string
  snippet: string
  example: { usage: string; code: string }
}
export const scriptContextFields: readonly ScriptFieldReference[] = [
  {
    name: 'file',
    type: 'Document',
    summary: '当前文件',
    description:
      '分词与标注使用；包含 fileId、path、directory；片段等数组单独提供。',
  },
  {
    name: 'sources',
    type: 'TextSource[]',
    summary: '原始文本来源',
    description:
      '文件名不含扩展名；父目录近到远，不包含解析边界。id、kind、text、depth；用于回查原文，默认标注使用 tokens。',
  },
  {
    name: 'tokens',
    type: 'Token[]',
    summary: '结构片段',
    description:
      '标注输入，包含 id、sourceId、parentId、kind、text、start、end。text/number 是完整词，separator 是分隔符，group 保留括号层级。第3集、HD1080P 不拆开；H.264 为相邻词。位置为 UTF-16。',
  },
  {
    name: 'candidates',
    type: 'Candidate[]',
    summary: '已有候选',
    description:
      '已有标注，保留冲突与无效解释及词内原文范围。片名标注最后执行并排除其他标注的范围，不包含最终元数据。',
  },
  {
    name: 'contextRoot',
    type: 'string',
    summary: '上下文目录',
    description: '裁决输入，完整目录树根。',
  },
  {
    name: 'sourceBoundary',
    type: 'string',
    summary: '解析边界',
    description: '裁决输入，来源文本不包含此目录及其父级。',
  },
  {
    name: 'tree',
    type: 'TreeNode[]',
    summary: '目录树',
    description: '裁决输入，节点含 path、parent、fileIds。',
  },
  {
    name: 'files',
    type: 'Document[]',
    summary: '全部上下文文件',
    description:
      '裁决输入，含每个文件的 sources、tokens、candidates；包含仅提供证据的非目标文件。',
  },
  {
    name: 'targetFileIds',
    type: 'string[]',
    summary: '待裁决目标',
    description: '裁决须逐个返回结果；不得遗漏、重复或增加其他文件。',
  },
  {
    name: 'params',
    type: 'object',
    summary: '规则参数',
    description: '当前规则配置中的 JSON 参数。',
  },
]
export const scriptMetadataFields: readonly ScriptFieldReference[] = []
export const scriptFunctions: readonly ScriptFunctionReference[] = [
  {
    name: 'reSearch',
    signature: '(pattern: string, text: string) → Match | null',
    summary: '查找第一个匹配。',
    description:
      'Go RE2 语法；Match 含 text、groups、captures、start、end；位置为 UTF-16，自动记录日志。',
    snippet: 'reSearch(${pattern}, ${text})',
    example: {
      usage: '匹配词语',
      code: 'const word = ctx.tokens.find(t => t.kind === "text")\nconst match = word && reSearch("S([0-9]+)", word.text)',
    },
  },
  {
    name: 'reFindAll',
    signature: '(pattern: string, text: string) → Match[]',
    summary: '查找全部匹配。',
    description: 'Go RE2 语法；最多 512 项，自动记录日志。',
    snippet: 'reFindAll(${pattern}, ${text})',
    example: {
      usage: '匹配词语',
      code: 'const matches = ctx.tokens.filter(t => t.kind === "number").flatMap(t => reFindAll("[0-9]+", t.text))',
    },
  },
  {
    name: 'reSub',
    signature: '(pattern: string, text: string, replacement: string) → string',
    summary: '替换匹配文本。',
    description:
      'Go RE2 语法；替换文本支持 $1、${name}。候选范围仍需指向原文。',
    snippet: 'reSub(${pattern}, ${text}, ${replacement})',
    example: {
      usage: '临时文本',
      code: 'const text = reSub("^0+", ctx.tokens.find(t => t.kind === "number")?.text ?? "", "")',
    },
  },
  {
    name: 'trace',
    signature: '(message: string, data?: JSON) → void',
    summary: '记录判定依据。',
    description: '自动附带阶段和源码行列。',
    snippet: 'trace(${message}, ${data})',
    example: {
      usage: '诊断',
      code: 'trace("候选数量", ctx.candidates.length)',
    },
  },
  {
    name: 'print',
    signature: '(...values: unknown[]) → void',
    summary: '记录文字。',
    description: '参数转换为文本后写入日志。',
    snippet: 'print(${message})',
    example: { usage: '诊断', code: 'print("开始标注")' },
  },
]
export const scriptEntry = {
  name: 'extract',
  description: '同步阶段入口；输入只读，返回数组。',
  snippet: 'function extract(${ctx}) {\n  return []\n}',
}
