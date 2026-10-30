import type { Target } from './model'

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

export const scriptFunctions: readonly ScriptFunctionReference[] = [
  {
    name: 'reSearch', signature: '(pattern: string, text: string) → Match | null',
    summary: '查找第一个匹配，自动记录执行过程。',
    description: '使用 Go RE2 语法。未命中返回 null；Match 包含 text、groups（索引 0 为完整匹配）、captures（命名捕获）和 start/end（UTF-16 索引）。',
    snippet: 'reSearch(${pattern}, ${text})',
    example: { usage: '适用字段：年份', code: `function extract(ctx) {
  const match = reSearch("(?P<year>19[0-9]{2}|20[0-9]{2})", ctx.text);
  return match ? Number(match.captures.year) : null;
}` },
  },
  {
    name: 'reFindAll', signature: '(pattern: string, text: string) → Match[]',
    summary: '查找全部匹配，自动记录执行过程。',
    description: '使用 Go RE2 语法。未命中返回空数组；每个 Match 包含 text、groups、captures、start/end，最多 512 项，超限报错。',
    snippet: 'reFindAll(${pattern}, ${text})',
    example: { usage: '适用字段：集数（如 E03E04）', code: `function extract(ctx) {
  const matches = reFindAll("(?i)E(?P<episode>[0-9]{1,4})", ctx.text);
  const episodes = matches.map(match => Number(match.captures.episode));
  return episodes.length ? Array.from(new Set(episodes)) : null;
}` },
  },
  {
    name: 'reSub', signature: '(pattern: string, text: string, replacement: string) → string',
    summary: '替换全部匹配，自动记录执行过程。',
    description: '使用 Go RE2 语法。捕获组替换使用 $1 或 ${name}；未命中时返回原文本。',
    snippet: 'reSub(${pattern}, ${text}, ${replacement})',
    example: { usage: '适用字段：片名（仅演示分隔符清理）', code: `function extract(ctx) {
  const title = reSub("[._]+", ctx.filename, " ").trim();
  return title || null;
}` },
  },
  {
    name: 'trace', signature: '(message: string, data?: unknown) → undefined',
    summary: '记录说明、中间数据及所在代码行。',
    description: '不改变提取结果。data 使用可序列化的 JSON 数据；在试跑的执行过程中查看日志。',
    snippet: 'trace(${message}, ${data})',
    example: { usage: '适用字段：任意（仅演示日志，不提取值）', code: `function extract(ctx) {
  trace("当前输入", { text: ctx.text, filename: ctx.filename });
  return null;
}` },
  },
  {
    name: 'print', signature: '(...values: unknown[]) → undefined',
    summary: '将参数转成文本写入执行轨迹。',
    description: '多个参数以空格连接，附带所在代码行；需要查看对象结构时使用 trace。',
    snippet: 'print(${value})',
    example: { usage: '适用字段：任意（仅演示日志，不提取值）', code: `function extract(ctx) {
  print("来源", ctx.source, "文本", ctx.text);
  return null;
}` },
  },
]

export const scriptContextFields: readonly ScriptFieldReference[] = [
  { name: 'text', type: 'string', summary: '当前来源文本', description: '本次规则选择的来源文本。逐级父目录来源每次传入一个目录名，按近到远尝试；来源清理脚本接收待清理文件名。' },
  { name: 'filename', type: 'string', summary: '不含扩展名的文件名', description: '不随当前来源的选择而改变；文件名清理不会改写此字段。' },
  { name: 'directories', type: 'string[]', summary: '边界内目录，近到远', description: '不包含解析边界目录本身；无边界内父目录时为空数组。' },
  { name: 'hasParent', type: 'boolean', summary: '边界内是否有父目录', description: '等价于 ctx.directories.length > 0。' },
  { name: 'source', type: 'string', summary: '当前规则来源类型', description: 'filename / cleanFilename / parentDirectory / directories / relativePath / absolutePath。' },
  { name: 'metadata', type: 'Metadata', summary: '已提取的字段', description: '前面规则已提取的元数据；未知单值为 null，未知集数为空数组。不能通过修改它写入结果。' },
  { name: 'params', type: 'object', summary: '本规则参数', description: '高级设置中的参数 JSON，未配置时为空对象。不适合点访问的键请使用 ctx.params["参数名"]。' },
]

export const scriptMetadataFields: readonly ScriptFieldReference[] = [
  { name: 'tmdbId', type: 'number | null', summary: 'TMDB ID', description: '季号和集数脚本中此值会被屏蔽为 null，防止将 ID 识别为季集号。' },
  { name: 'title', type: 'string | null', summary: '片名', description: '电影和剧集共用此字段。' },
  { name: 'mediaType', type: '"movie" | "tv" | null', summary: '媒体类型', description: 'movie 为电影，tv 为剧集；未知时为 null。' },
  { name: 'year', type: 'number | null', summary: '年份', description: '已提取的年份；未知时为 null。' },
  { name: 'season', type: 'number | null', summary: '季号，0 为特别篇', description: '0 是有效季号；判断是否已知请使用 !== null，而不是判断真假。' },
  { name: 'episodes', type: 'number[]', summary: '集数', description: '未知时为空数组；一个文件可以对应多集。' },
  { name: 'releaseTag', type: 'string | null', summary: '发布标签', description: '已提取的发布标签；未知时为 null。' },
]

export const scriptEntry = {
  name: 'extract', signature: '(ctx) → 当前字段值 | null',
  description: '同步入口，只接受一个普通参数。返回本字段值或 null，也可返回 { value, matchedText?, captures? }；来源清理脚本必须返回字符串。',
  snippet: 'function extract(ctx) {\n  ${return null;}\n}',
}

export const scriptReturnTypes: Record<Target, { type: string; description: string; example: string }> = {
  tmdbId: { type: 'number | string', description: '1–999999999 的整数，也接受整数字符串。', example: '603' },
  title: { type: 'string', description: '非空片名，自动去除两端空白。', example: '"示例片名"' },
  mediaType: { type: '"movie" | "tv"', description: '只接受 movie（电影）或 tv（剧集）。', example: '"tv"' },
  year: { type: 'number | string', description: '1–9999 的整数，也接受整数字符串。', example: '2024' },
  season: { type: 'number | string', description: '0–999 的整数；0 为特别篇。也接受整数字符串及支持的中文数字写法（如「二」「十二」）。', example: '2' },
  episodes: { type: 'number[] | number | string', description: '推荐返回整数数组；也接受单集或范围字符串（如 "0-3,5"）。每集为 0–9999，不可重复，最多 1000 集；未命中返回 null，不要返回空数组。', example: '[3, 4]' },
  releaseTag: { type: 'string', description: '非空发布标签，自动去除两端空白。', example: '"GROUP"' },
}

export const scriptRuntimeNotes = [
  { title: '入口与只读输入', text: '必须定义同步的 function extract(ctx)，且只有一个普通参数。不支持 async、generator 或 Promise 返回值。ctx 及其嵌套内容只读，修改会报错；每次执行的全局状态相互独立。' },
  { title: '两种正则语法', text: 'reSearch / reFindAll / reSub 使用 Go RE2 语法，支持 (?i) 忽略大小写，不支持前后查找和反向引用。原生 RegExp 使用 JavaScript 语法。字符串中的反斜杠需要转义，例如 "\\\\d+"。' },
  { title: '执行轨迹', text: '输入、返回值、错误及三个正则辅助函数的调用会自动记录。原生 RegExp、字符串操作及变量赋值不会自动记录；用 trace / print 补充说明，在试跑「查看执行过程」中查看。' },
  { title: '可用能力', text: '可使用 JS 字符串、数组、Math、JSON、RegExp 等标准能力。不提供浏览器或 Node.js 环境，不能加载模块、访问网络、文件、系统命令或环境变量；没有 fetch、require、window、document。' },
  { title: '执行限制', text: '单个脚本最多 32 KiB，执行时限 250 ms，调用栈最多 256 层；最多保留 128 条过程事件。reFindAll 最多匹配 512 项；正则参数和替换结果也有大小限制，超限报错。' },
  { title: '返回数据与错误', text: '返回值必须为 JSON 数据；最多嵌套 12 层，单个集合最多 1024 项，字符串最多 32 KiB，总遍历预算 64 KiB。不接受循环引用、函数或非有限数值。运行出错会保留此前日志并继续后续规则；请求取消会中止执行。' },
  { title: '季集保护', text: '季号、集数脚本输入中的 ID 标签会被屏蔽，包括文本、文件名、目录及 ctx.metadata.tmdbId，避免将 TMDB ID 误识别为季集号。' },
]
