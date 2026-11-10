# 路径识别流水线

核心输入是应用提供的目录快照，不读取媒体文件、不访问文件系统或 TMDB。流程为：

```mermaid
flowchart LR
  A[完整目录树的视频路径] --> B[分词脚本]
  B --> C[词语与相邻词标注]
  C --> T[剩余区段片名标注]
  T --> D[目录级元数据裁决脚本]
  D --> E[每个目标文件的 Metadata 与依据]
  E --> F[matching 作品匹配]
```

配置只有一种格式：`{tokenizer, annotators, resolver}`。分词和裁决各一条启用的脚本；标注可启停、增删，支持脚本或正则；普通标注先执行、片名标注最后执行，两组内分别按 `order` 排序。同一棵树开始执行时固定一份配置快照。保存失败不会替换正在生效的配置。

## 结构与阶段边界

三个阶段均使用同步入口 `function extract(ctx)`，通过返回 JSON 数据提供结果。输入递归冻结。分词与标注不能写最终 Metadata。

分词输入 `ctx.file={fileId,path,directory}`、`ctx.sources[]`、`ctx.params`。来源包含 `id/kind/text/depth`；文件名不含扩展名，父目录近到远，不包含解析边界本身及其父级。分词返回：

```js
[{id: 't0', sourceId: 'name', parentId: null, kind: 'text', text: 'Show', start: 0, end: 4}]
```

`kind` 为 `text/number/separator/group`。保留原文、分组、数字、分隔符与 UTF-16 位置，不预设“是否季号／集号”。同层不得重叠，顶层须覆盖全部原文，范围不得切开代理对。默认脚本将连续汉字（含辅助平面汉字）、英文字母 `A–Z/a–z` 和数字 `0–9` 组成一个词，其他字符作为分隔点。纯数字词使用 `number`，其他词使用 `text`。`第3集`、`HD1080P`、`S01E03` 保持完整，`H.264` 则为两个词；分隔符保留原文，括号保留嵌套分组。规则脚本可修改。

标注输入 `ctx.file`、`sources`、`tokens`、`candidates`、`params`。`source=filename/directories/all` 决定可见来源；已有候选只包含完整落在所选来源内的项。规则 `target` 指定候选身份，`order` 是唯一非负整数。脚本返回：

```js
function extract(ctx) {
  return ctx.tokens.filter(t => t.kind === 'text').flatMap(token =>
    reFindAll('(?i)S([0-9]+)', token.text).map(match => ({
      value: Number(match.groups[1]),
      spans: [{sourceId: token.sourceId, start: token.start + match.start,
        end: token.start + match.end}],
      strength: 'explicit', status: 'valid', reason: 'S 季标记'
    })))
}
```

宿主填写候选 `id/ruleId/role`。`strength` 为 `explicit/contextual/heuristic`；`status` 为 `valid/invalid`，均须填写依据。冲突与非法显式写法保留为候选，不抛弃其位置。候选身份包含七个最终字段，以及 `resolution/videoCodec/audioCodec/bitDepth/dynamicRange/language/subtitleLanguage/checksum`。默认标注读取词语或同来源同组的相邻词，利用分隔符关系识别 `H.264`、`tmdb-123`、合并集范围等；支持正文中的 `第N集／话`、无父目录的显式中文集号和 `HD1080P`。技术标签、发布组、TMDB 标记先标注，用于保护数字和片名。一个词可有多个标签，标注可覆盖词内子范围，词本身不被改写或删除。宿主拒绝把 TMDB ID 范围用作合法季集候选。

正则规则使用 Go RE2，配置为 `{pattern,capture}`（命名捕获组）或 `{pattern,value}`（固定 JSON 值）。正则仅对所选来源的 `text/number` 词逐个匹配，不匹配分隔符或整组文本，跨词格式使用脚本。每次命中生成显式候选，位置映射回来源原文；捕获值由对应身份转换、校验。不存在来源清理、覆盖开关或字段直接写入接口。

默认片名标注排除其他候选占用的范围（包括推测候选和无效元信息标记），从连续剩余区段生成候选，不再在首个季集或年份处整体截断。相邻词统一用一个空格连接，标点不进入片名；已标注范围和括号组边界会分开候选，不跨来源拼接。同一来源有可用括号外正文时使用正文，否则按括号组分别提取；未标注的数字片名仍可保留。未增加类别、版本说明、来源署名等身份，因此未知说明文字仍可能成为片名候选。

裁决输入 `ctx.contextRoot/sourceBoundary/tree/files/targetFileIds/params`。`files[]` 提供整个上下文目录及子目录中每个文件的来源、片段、候选；`tree[]` 节点为 `path/parent/fileIds`。返回目标文件结果数组：

```js
[{fileId: 'file-0', metadata: {
  tmdbId: null, title: 'Show', mediaType: 'tv', year: null,
  season: 2, episodes: [3], releaseTag: null
}, decisions: {
  title: {candidates: [{fileId:'file-0', candidateId:'title:0'}], reason:'片名候选'},
  season: {candidates: [{fileId:'file-0', candidateId:'season:0'}], reason:'显式季号'},
  episodes: {candidates: [{fileId:'file-0', candidateId:'episodes:0'}], reason:'显式集号'},
  mediaType: {candidates: [{fileId:'file-0', candidateId:'season:0'}], reason:'季号提供剧集依据'}
}}]
```

必须覆盖全部目标，不能增加或重复文件。Metadata 必须提供完整七个字段，未知标量为 `null`、未知集数为 `[]`，0 是有效季集号。所有非空字段需要依据；引用可跨文件，但必须指向当前树中有效候选。最终路径由宿主填写。

默认裁决优先显式依据与较近来源；同一作品目录内比较括号数字的固定位置与变化位置，忽略已标注的技术标签变化，支持缺集和重复发布，不按排序位置生成集号。无效显式季集格式阻止较弱解释接管。多作品、多季分别保留各自依据；不因缺少季集推断电影。

## 目录范围与更新

应用先完成扫描再执行三阶段。导入目录时纳入其全部后代视频；导入单个文件时读取父目录整棵树作为上下文，只更新导入文件。重叠根折叠到外层，互不相交的根分别裁决。隐藏文件、符号链接和目标目录沿用 scanner 的排除规则；每棵树超过 2000 个视频直接报错，不截断。

选择性重新识别读取相同的完整上下文，仅检查点写入选中行。未选中、已提交文件和人工决定保持原业务语义；取消、中断后的重新识别标记继续用于重试。下游 matching、organizer 消费普通 Metadata。

## 运行与历史

- 每条源码最多 32 KiB；分词与标注每次 250 ms，裁决 5 s。每文件最多 1024 片段、分组深度 16；每条标注最多 512 候选、每文件最多 1024 候选。
- 普通阶段输入／输出 4 MiB，裁决输入 64 MiB、输出 16 MiB；返回 JSON 的深度最多 12、集合最多 10000、字符串最多 32 KiB。领域限制另行校验。
- 可用 `trace/print/reSearch/reFindAll/reSub`，事件保留阶段、源码行列和诊断。每次调用最多 128 条事件及一条截断说明。输入输出自动日志只保存摘要，结果记录在阶段输出中。
- 脚本没有模块加载、网络、文件、系统命令或环境变量接口。goja 的执行中断不等于进程级内存隔离。
- 分词或裁决失败终止该树；标注失败继续收集诊断，整树不更新元数据。取消停止后续执行，保留已完成轨迹。observer 失败立即停止。

正式执行每棵树共享一个 traceId；目录上下文与树仅保存一次，每文件保存来源、分词与候选输出；每条规则源码只保存一次。阶段输出可重建包括跨文件引用在内的历史结果，不依赖当前规则。结束时步骤与状态同事务提交；服务重启将遗留运行摘要标记中断，不重置用户数据。单文件服务入口仅是相同流水线的便捷包装。

默认脚本更新不自动覆盖已保存的规则；通过“恢复默认规则 → 保存”应用新默认。

存储格式为 2。旧格式直接拒绝，应用不会迁移、删除或重建已有数据库。验证与基准使用独立临时目录。

## 编辑与验证

规则页按“分词／候选标注／元数据裁决”排列；普通标注与片名标注各自在组内排序；片名始终最后执行。新增规则或修改身份时进入对应组。单规则页试跑和保存使用“当前流水线＋草稿”。调试输入多行虚拟路径，不访问文件系统；结果按文件展示元数据、候选、原文范围、分词和跨文件依据。历史使用同样的结果组件。

规则样例使用 `{paths,root,targets,expected}`，运行完整流水线。分词／标注比较该规则按文件排列的输出数组；裁决比较完整文件结果。任何执行错误使样例失败。保存前必须通过当前草稿的全部样例。

根目录 tests/ 包含 100 个全部通过的路径样本，以及按词分隔、词内标注、正文中文集号、连续剩余片名区段、用户长路径、截图、目录比较、多作品、多季、多发布、选择性更新、边界、运行限制、持久化与重启测试。`make check` 运行完整验证；`BenchmarkRecognition2000` 测量整树执行及轨迹持久化，并检查历史可完整重建。
