# JoyCurator 技术交接文档

本文描述当前仓库中的实现，供接手开发、定位问题和继续迭代使用。开发阶段只维护当前接口与数据模型，不保留旧接口、字段适配、迁移或降级分支；格式不符直接报错，不自动删除现有数据。范围包含路径提取、自动识别、手动整理、执行队列，以及本次拆分后的前端路由与查询管理。

配套文档：[后端模块与调用链](BACKEND.md)、[启动与使用](../README.md)、[接口字段](API.md)、[规则与脚本规范](RECOGNITION.md)、[前端目录与路由](FRONTEND.md)、[测试入口](../tests/README.md)。接口的完整请求格式以 `API.md` 和相应处理函数为准。

## 1. 先理解三个不同的业务环节

```mermaid
flowchart LR
  P[文件绝对路径文本] --> E[recognition 规则链]
  E --> M[Metadata + Step 数组]
  M --> A[app 匹配 TMDB 与校验季集]
  A --> W[识别工作区与人工修正]
  W --> V[整理预览]
  V --> J[确认后创建整理任务]
  J --> O[organizer 执行文件操作]
```

1. **路径提取**：从文本提取 ID、标题、年份、季集等；不读取文件，不访问 TMDB。代码在 `internal/recognition`。
2. **作品识别**：消费提取结果，查询 TMDB、判断候选、校验季集、形成可整理或待处理状态。代码在 `internal/matching/`。
3. **整理与刮削**：整理预览直接使用工作区快照，检查源文件与目标冲突，只执行视频／字幕复制、移动或硬链接；刮削另行查询 TMDB 并生成 NFO／图片。代码在 `organizer/preview.go`、`organizer/jobs.go`、`organizer/batch.go`、`organizer/fast.go` 和 `organizer`。

这三个环节的边界需要保持。比如“某个标题对应哪个 TMDB 作品”属于作品识别，“S02E03 提取为季 2、集 3”属于路径提取，“最终保存在哪个目录”属于命名与整理。

**两个容易混淆的命名：**

- `internal/recognition` 是纯路径元数据规则引擎；`matching/` 负责作品匹配，异步工作区由 app 协调。
- `internal/rules` 是目标目录分类、命名模板规则；不是路径提取规则库。修改提取规则应查看 `internal/recognition`。

## 2. 仓库与部署结构

```text
backend/
  cmd/joycurator/main.go       启动与退出
  internal/
    app/                      装配、HTTP、工作区流程、认证与设置接入
    recognition/              路径规则、Metadata、调试、轨迹
    matching/                 TMDB 匹配、候选与季集策略
    organizer/                Run、队列、文件操作、记录与回退
    model/                    共用文件、作品、设置与操作 DTO
    infra/tmdb/               TMDB 请求、响应转换和缓存端口
    infra/sqlite/             SQL、事务和持久化
    infra/fsops/              授权文件操作
    infra/scanner/            文件遍历
    rules/metadata/trace/     命名、NFO 与事件等现有工具能力
    auth/webui/               认证与前端资源接入
frontend/src/
  app/                        路由、登录、Query、导航、共享运行状态
  layouts/                    登录后的统一应用外壳
  pages/                      独立路由页面
  features/                   按业务封装的状态逻辑和组件
  components/                 通用 UI、导航、任务组件、文件浏览器
  api.ts                      统一请求及前端 DTO
  recognition.ts              自动识别分组、修正等纯函数
  style.css                   现有视觉基础与通用业务样式
tests/                        所有测试统一集中保存
docs/                         功能规范与交接文档
local/                        本地数据、缓存、构建产物、备份与验证材料
```

后端为 Go 服务，SQLite 使用 `modernc.org/sqlite`，前端为 Vue 3 + TypeScript + Vite。发布构建将前端静态资源嵌入 Go 可执行文件；开发时前后端分别启动。

当前前端使用 Vue Router、TanStack Vue Query 与通用布局组件，构建只使用当前源码。

## 3. 后端启动与职责划分

启动链：

```text
main.main
  → 读取 JOYCURATOR_* 环境变量，准备源／目标目录
  → app.Open(dataDir, sourceRoot, targetRoot, secure)
    → 创建授权文件系统 fsops.FS
    → 打开 joycurator.db、启用 WAL / busy_timeout / foreign_keys
    → 新库事务建表并写入首次配置／当前格式标识；已有库先校验完整 schema
    → sqlite.Open：队列一致性校验与事务中断恢复
    → 创建 organizer.Coordinator
    → recognition.New：读取并编译当前链
    → 装配设置、目录、识别、预览与执行服务
    → 校验设置
    → 校验 execution_queue 引用；原 running 任务及运行中识别标为 interrupted
    → organizer.Start 按持久化序号恢复 queued，启动整理／刮削两个 worker
  → App.Handler(webui.Files())
  → http.Server.ListenAndServe
```

退出时 HTTP 服务先停止接收请求，随后 `App.Close` 停止出队，取消整理、刮削与识别的活动任务，等待保存并保留未开始队列，关闭 TMDB 空闲连接和数据库。

`App` 装配三个业务模块与 infra，并接入 HTTP、设置、认证和工作区流程；当前链属于 recognition，预览与执行统一属于 organizer，任务与后台生命周期由 organizer.Coordinator 管理。业务模块不直接依赖 SQL／HTTP，具体边界与调用链见 [后端架构](BACKEND.md)。

数据库仅设置一个打开连接。不要在尚未关闭的查询结果集内再执行依赖同连接的写入；读取后关闭 rows，再修改记录。并发控制和任务取消要遵循`organizer.Coordinator` 的准入锁、等待计数和 context 生命周期。

## 4. 路径规则链的稳定模型

### 4.1 Rule：单条规则只能写一个字段

| 字段 | 职责 |
| --- | --- |
| `id` | 稳定标识，链内唯一 |
| `name` / `description` | 管理与调试展示 |
| `target` | `tmdbId/title/mediaType/year/season/episodes/releaseTag` 之一 |
| `source` | 本条规则读取哪类路径文本 |
| `order` | 非负顺序值，升序执行，不允许重复 |
| `enabled` | 禁用仍生成步骤记录 |
| `kind` | 当前接受 `script` 或 `regex` |
| `config` | 源码／正则／参数／捕获或固定值／可选清理源码 |
| `overwrite` | 默认 false；已有值时是否允许覆盖 |
| `examples` | 路径、边界和本规则预期提取值 |

来源可选 `filename`、`cleanFilename`、`parentDirectory`、`directories`、`relativePath`、`absolutePath`。文件名不含扩展名；路径来源保留扩展名。逐级父目录按近到远尝试，不包含边界目录，也不会越过边界。

### 4.2 Metadata：只放结果

```json
{
  "tmdbId": null,
  "title": "Show",
  "mediaType": "tv",
  "year": null,
  "season": 2,
  "episodes": [3],
  "releaseTag": null
}
```

单值未知为 `null`，集数未知为 `[]`。`season: 0` 是特别篇；电影与剧集共用 `title`，`mediaType` 只接受 `movie/tv/null`。接口、存储和内部业务统一使用 Metadata，没有 Parsed 转换；用于整理的可编辑季集映射与原始 Metadata 分开。

### 4.3 Step：说明每条规则做了什么

每条规则都会产生 Step，包含：规则 ID／名称／顺序、完整 Rule 快照、目标字段、尝试文本、命中文本与捕获、提出的值、前后值、采用标志、原因、时间和耗时。逐目录尝试放在 `attempts`，脚本细节放在 `attempts[].trace`。

`status` 为 `disabled/not_matched/matched/skipped/error`。**命中不等于采用**：若前面的规则已写入该字段且未允许覆盖，后续规则仍匹配、记录 proposed value，但 `applied:false`。

## 5. 提取执行器的调用链与行为

核心入口：

```go
engine, err := recognition.Compile(rules)
result, err := engine.Extract(ctx, recognition.Input{Path: path, Root: root}, observer)
```

`Compile` 全量验证规则 ID、顺序、目标、来源、类型、源码或正则配置等，然后生成不可变执行实例。`Rules()` 返回配置快照，调用方不应修改引擎内部状态。

`Extract` 的执行顺序：

```text
prepare(Input)：验证 /… 绝对路径、文本规范化、确认边界
  → 初始化全未知 Metadata
  → 按 order 逐条执行
    → 创建 Step，记录目标 before
    → 判断取消、禁用、来源不存在
    → 按来源逐个尝试文本
    → 执行来源清理（如需）、ID 标签保护、脚本或正则
    → 将提出值按 target 转换并验证合法性
    → 未知字段补全；已有字段按 overwrite 决策
    → 记录 after、applied、reason、耗时
    → 深复制 Step，加入结果并通知 observer
  → 返回 Metadata + Step[]
```

不自动回头执行规则。单条规则运行或转换失败记录 error，保留已有字段并继续；取消后剩余规则记录 skipped，最终返回取消错误。observer 保存失败则停止调用，不能假装轨迹完整落库。

`tmdb-603` 等 ID 标签在季号、集数输入及相关脚本上下文中受保护，不允许再次把标签数字当季集。Matrix 场景应得到 ID 603、年份 1999，不能产生第 603 集。媒体类型没有提取依据就保持未知，再由消费方进行跨类型查找和消歧。

## 6. 脚本规则与可观测性

JavaScript 引擎使用 goja，每次调用创建独立 Runtime，不提供 Node.js、网络和文件 API。仅支持同步入口，拒绝 Promise 输出。原生 RegExp 需要主动 trace；默认脚本使用可追踪的 RE2 辅助函数。250 ms 是执行中断时限，不是进程级内存或所有原生操作的硬隔离。旧语言源码不自动转换，也不自动重置用户数据。

默认规则每个字段一条，提取算法全部位于 `recognition/scripts/*.js`（一个字段一个脚本，内部按优先级尝试多种写法）；`catalog.go` 和 `defaults.go` 负责登记模板、来源和默认顺序。没有运行时 builtin 处理器分支。

脚本定义同步 `function extract(ctx)`，返回单个值／集数列表、`null`，或 `{value,matchedText?,captures?}`。上下文经 JSON 复制并递归冻结，包含当前文本、文件名、目录、source、metadata、params 等。规则不能直接修改公共 Metadata。

允许的通用能力为 JavaScript 标准字符串、数组及正则操作、Go RE2 正则函数 `reSearch/reFindAll/reSub` 及 `trace/print`。不提供文件、网络、模块加载、系统命令和环境变量访问。

日志层次：

- 执行器自动记录每条规则的输入、结果、字段采用或拒绝。
- 脚本环境自动记录 RE2 辅助函数调用、返回／错误以及源码行列号。
- `trace(message, data)` 或 `print` 展示作者认为重要的中间值。
- 普通变量赋值不会自动逐行记录。要解释算法分支，应在脚本关键处补 `trace`。
- `cleanFilename` 的清理也是脚本，使用 `config.sourceCode`，步骤快照保留实际源码。

当前限制：单个脚本 32 KiB、250 ms 执行时限与 256 层调用栈；每脚本最多保留 128 条过程事件（超出有省略提示）；单次正则结果最多 512 项。脚本超限作为该规则 error，不抹掉此前轨迹。

规则只维护当前 script/regex 模型，不提供旧处理器迁移、旧接口或双格式读写。已有的当前链由用户编辑保存，不被默认脚本自动覆盖。

## 7. 当前链保存、试跑与正式轨迹

### 保存

```text
RecognitionRules 编辑草稿 → PUT /api/extraction/rules
  → recognition.Service.Save
  → recognition.Compile：先全量验证
  → recognition 的规则锁内通过 Store 保存 rules 与 rule_examples
  → 替换 recognition.Service 的当前 Engine
  → 前端失效并重新读取 rules Query
```

保存失败不替换原链。不维护版本、草稿发布或任务绑定。每个文件开始提取时通过 `recognition.Service.Current()` 获取一次实例；本次调用内不切换。批量任务的后续文件可以使用刚保存的新链。

### 规则工作台前端

- `useRuleDraft.ts`：唯一页面草稿、稳定界面 key、原始 JSON、全链校验、选中状态与保存指纹。试跑和保存均从同一草稿生成 Rule[]，不再有“应用到编辑链”步骤。
- 各业务页统一使用 `components/ui/PageHeader.vue`：标题和操作为第一行，说明或流程为第二行；英文副标题桌面内联、手机隐藏，不再单独占一行。手机工作流页将操作／步骤放第二行，说明让位给操作。
- `RuleFieldTable.vue`：规则列表，按字段顺序分组；每个字段可新增脚本、正则或模板规则，规则行可启停、同字段内上移下移、删除，点击进入编辑页。界面 key 不使用可编辑的规则 ID。
- `RuleEditor.vue`：脚本／基本配置／样例标签，直接编辑共享草稿；无效 JSON 也保留。`ScriptCode.vue` 根据执行事件行号选中代码。
- `RecognitionRules.vue`：规则列表页（`/recognition/rules`），管理整条链：排序、启停、新增、删除、恢复默认、运行全部样例与保存；有未保存修改时离开需确认。`RuleEdit.vue`：单条规则编辑页（`/recognition/rules/:ruleId`，`new` 表示新建），代码与试跑在同一页，试跑和保存都使用「已生效的链 + 这条草稿」。`DebugPage.vue` 显示每个字段的结果和决定它的规则，可跳到编辑页。`ExtractionResult.vue` 提供 Metadata／执行过程／JSON 视图、按 `ruleId` 过滤单条规则及定位事件。
- 切换规则、排序和手机标签保留草稿；离开整个页面仍按现有约定丢弃未保存草稿，不增加本地持久化。

### 试跑

```text
DebugPage / 规则编辑器试跑 → useExtraction
  → POST /api/extraction/test {path,root?,rules?}
  → 选当前引擎，或临时编译提供的编辑链
  → Engine.Extract(observer=nil)
  → 返回 metadata + steps
```

支持不存在的模拟路径。省略 root 时，源目录内路径使用源目录，否则 `/`；显式 root 必须为输入文件的父级目录。`rules` 省略表示当前链，`[]` 表示空链。试跑不写规则配置、工作区、轨迹、整理记录或预览，不访问 TMDB。前端 90 秒上限，取消／离开页面终止请求并忽略过期响应。规则工作台额外按整条草稿、路径和边界的指纹标记过期结果，不将旧轨迹标记到修改后的规则。

样例接口会运行整条链，再比较对应规则 Step 的 `extractedValue`，因此样例验证的是该规则提出的值，不一定是最终 Metadata（后续覆盖可能改变最终结果）。

### 正式轨迹

```text
recognition.Service.ExtractFile(path,root,workspace)
  → 获取当前 Engine
  → 插入 extraction_traces(status=running)
  → Engine.Extract：内存收集完整步骤
  → 单个事务按子表批量插入步骤、规则快照和事件，并保存最终 metadata、status、finishedAt、error
  → 返回 Result 与 traceId
```

工作区导入统一调用 `ExtractFile`，不探测媒体；独立 `/scan` 已移除。消费方的无持久化适配入口是 `recognition.Service.Parse`。

工作区文件行只携带 `traceId`，历史列表分页读取摘要，选中详情才获取全部步骤。每步含完整规则及源码快照，当前规则改名、修改或删除不影响旧记录。重启将遗留 running 轨迹标记 interrupted。清理仅允许已结束轨迹，关联 steps 通过外键级联删除，不删除工作区结果。

注意：轨迹 completed 表示执行器完成整条链，不代表没有单条规则 error，也不代表 TMDB 匹配成功。定位时同时检查 Step 和工作区状态。

## 8. 自动识别、匹配与人工修正

```text
AutomaticRecognitionPage → useWorkspace.start
  → POST /api/recognitions {paths,id?}
  → WorkspaceService.Start：校验目录／数量、初始化或补充工作区
  → 保存 Recognition，建立后台 context
  → recognize
    → scanner.Walk（不 Probe）
    → 对新文件 ExtractFile → Metadata → 初始 query 分组
    → Resolver.Match → DecideMatch：作品候选匹配与同批查询去重
    → Resolver.Validate → 季集纯策略：季集有效性
    → 按 type + TMDB ID 合并作品，同剧多季同组
    → 自动选择 ready 且未移出的文件
    → 分批保存工作区进度
  → 前端 TanStack Query 轮询
```

关键入口：`app/workspace.go` 管任务准入，`app/recognition_job.go` 管异步流程，`app/workspace_edits.go` 管修正；`matching/match.go`、`validate.go` 管查询协调，`policy.go` 管不依赖网络的判断。TMDB 客户端负责请求、响应缓存与重试，规则引擎不参与网络。

匹配策略：明确 ID 优先；类型不明时查电影和剧集，ID 在两类中都存在也需消歧。标题／原名标准化精确匹配、已知年份一致、无同等候选才自动采用；模糊、同名、年份冲突与去年份回退进入 review。没有搜索结果与网络／凭据错误分开处理。缺季号只在唯一常规季且集数有效时补全；不自动换算跨季连续集号。

状态分两层：

| 对象 | 状态 |
| --- | --- |
| 工作区任务 | running、completed、failed、cancelled、interrupted |
| 文件识别 | pending、ready、review、unmatched、failed |

工作区 completed 只表示本轮处理完成，可以仍有 review、unmatched 或 failed 文件。用户可以先选 ready 部分进入整理。

人工修正通过 `PUT /recognitions/{id}` 提交完整 edits 和 revision。服务端保留扫描得到的文件身份、校验作品和季集、拒绝旧 revision；前端不应自行把未校验项目标为 ready。重试保留已处理及人工修正项，对待重试文件调用当前链；旧轨迹继续保留。

## 9. 整理工作区与预览调用链

```text
AutomaticRecognitionPage → useWorkspace.start → POST /recognitions
  → 自动识别并分组 → 服务端保存工作区
组内搜索作品 → GET /search → 选择 Entry → PUT /recognitions/{id}
  → 剧集 GET /seasons、GET /episodes → 单集对照、关联文件、修正季集
  → 单集对照内“查看季度”只切换视图，不修改工作区
  → 批量修正：目标季度与集数偏移输入框（原始识别基准，不累加）；当前映射与修正后编号旁显示相对原始识别的偏移量徽章
  → 独立勾选、预览并确认后保存，由服务端校验；不自动执行整理
预览 → POST /batches/preview → buildPreview（按作品）
  → 校验源路径和工作区版本、读取已确认的作品详情与季集快照（不请求 TMDB）
  → rules.Target / ShowRoot 渲染分类与命名
  → 整理仅生成视频／字幕操作并登记目标视频映射；独立刮削预览生成 NFO／图片操作
  → 校验冲突、磁盘需求等 → PreviewCache
确认 → POST /batches/execute {id} → Run 与队列同事务写入 → 返回排队回执
  → organizer.drain 按序出队 → work
  → organizer.Engine.Run，逐步持久化操作状态
```

`BatchPreview` 是单部与多部作品共用的规划入口，各组调用 `buildPreview`。前端提交工作区 ID、修订号、重复策略和模式，不能提交作品详情或可信文件操作指令。两类预览在共享缓存中短期保存（15 分钟，合计最多 32 份，满额逐出最早预览），包含配置版本及服务端生成的 ops；服务重启后须重新预览。

`organizer` 将整理的视频／字幕和独立刮削的 NFO／图片分开记录，通过 `fsops` 完成受限目录内的读写、暂存与发布。视频／字幕采用无全文件哈希的快速路径；异常中断若状态不明，保留现场待人工确认。整理撤销核对持久化的目标身份与大小，发现外部替换或大小变化则停止；不扫描视频内容，因此不能识别同一文件的等长修改。刮削仍保持内容与身份校验。覆盖没有备份，不能恢复旧内容。

## 10. 多批队列、并行刮削与文件锁定

```text
POST /batches/preview {recognitionId,revision,mode,duplicatePolicy}
  → Organizer.BatchPreview：只收集可编辑且已选 ready 文件，按作品分组
  → buildPreview → 检查全部目标、合计空间与已排队任务的预留
  → PreviewCache：绑定 workspace revision、settings version、源／目标身份
POST /batches/execute {id}
  → 校验预览与文件，锁内复核快照
  → Enqueue 事务写入逐作品 Run 和 execution_queue 序号 → 立即返回
organize drain → 一次取得全部访问占用 → 再校验配置、授权、文件与空间
  → work / Engine：逐阶段持久化
  → 视频 completed 并核验身份 → SaveRunMappings 同事务保存检查点＋映射
  → 释放该视频写占用 → scrape drain 可立即消费；整理继续后续视频
```

`Coordinator` 持有 organize/scrape 各一个 worker，整理批次连续 FIFO；单部失败、取消或完成均推进后续任务。新建、重试和撤销都进入对应通道，重试追加队尾，不按 Run 的历史创建时间排序。批次由 `runs.batchId` 关联，不新增批次表；持久化顺序与任务引用由 `execution_queue` 管理。

跨通道的读写冲突进入等待，启动时一次获得全部所需资源，不持有部分锁，`GET /queue` 显示等待原因。整理覆盖／撤销不能破坏刮削正在读的视频；共享图片和 NFO 撤销仍检查其他记录引用。已排队／运行的整理来源和全部目标不能重复提交；已入队任务不受 15 分钟预览有效期限制。

工作区只提交变更文件，revision 检查继续保留。`FileExecutions` 按 Run 投影文件关联任务、锁定、完成与人工核对状态；后台执行不保存工作区或增加 revision，识别增量保存也不持久化该投影。未提交文件继续导入、修正、选择和预览；已提交文件不能修改或重复提交，移出包含锁定文件的来源被拒绝。已登记完成视频退出整理选择；失败／取消后尚未完成的视频释放编辑，状态不明的视频保留锁定。整理最后保存或重试不会把同一身份视频的已完成刮削状态重置为待办。

批次取消只作用于该批当前与未开始项；单条取消支持等待与运行态。关闭停止出队并等待活动任务保存；重启将原 running 置为 interrupted 并移出队列，不自动重试，原 queued 按序重验并续跑。队列引用或状态不一致明确报错，不自动重建。设置在任一通道活动、排队或识别未结束时禁止保存。

## 11. 数据保存在哪里

| 内容 | 存储 | 说明 |
| --- | --- | --- |
| 管理员与会话 | SQLite administrators / sessions | 密码 bcrypt、会话 Token 存 SHA256 哈希 |
| 设置与 TMDB 凭据 | SQLite config | 对外只返回凭据存在标志 |
| TMDB 响应 | SQLite tmdb_cache | 带过期时间，正常响应缓存 24 小时 |
| 当前提取规则链 | SQLite rule_chain / rules / rule_examples | 关系表，保存即替换，空链重启不填充 |
| 提取执行与步骤 | SQLite extraction_traces / extraction_steps | 长期保留，人工清理 |
| 自动识别工作区 | SQLite recognitions / workspace_items 等 | 摘要、路径、季集和作品快照拆表，含 revision 与 traceId |
| 整理／刮削／撤销记录 | SQLite runs / operations / run_mappings 等 | 摘要、检查点与映射快照拆表，操作内容使用 BLOB |
| 执行顺序与任务引用 | SQLite execution_queue | 自增 position、唯一 run_id、purpose；与 Run 原子入队 |
| 已登记媒体 | SQLite media_mappings | 视频身份、映射归属和刮削状态；逐视频原子登记 |
| 尚未执行的预览 | organizer.PreviewCache 内存 | 过期或重启失效 |
| 双通道活动摘要 | `/queue` ＋ TanStack Query | 挂载／聚焦从服务端发现，不保存浏览器任务 ID |
| 查询响应缓存 | TanStack Query 内存 | 按 key 隔离，退出清空 |
| 规则编辑草稿、调试结果 | 页面内存 | 未保存编辑离开规则页会丢失 |

备份数据库应停止服务后复制，或使用 SQLite 一致性备份方案；运行中随意只复制主文件可能遗漏 WAL 内容。恢复前同时确认启动授权目录和已保存路径仍适用。

## 12. 前端架构、页面与调用关系

```text
main.ts
  → 创建 Vue App
  → 安装 VueQueryPlugin(queryClient)
  → 安装 Vue Router
  → App.vue：路由出口／初始加载状态
    → /login：LoginPage
    → AppLayout：provideRuntime
      → AppSidebar / MobileMenu
      → RouterView + KeepAlive
      → TaskToast / RunDetails
```

路由配置集中在 `app/routes.ts`，登录守卫在 `routerGuards.ts`。未登录进入保护页面时跳转登录并保留站内返回地址，`safeRedirect` 拒绝跨站地址和登录循环。生产 HTTP 对缺失的无扩展名前端页面 GET/HEAD 回退入口 HTML；API 和缺失资源保持 404。

| 导航组 | 页面 | 路径与业务封装 |
| --- | --- | --- |
| 工作区 | 整理工作区 | `/recognition` → AutomaticRecognitionPage + useWorkspace |
| 记录 | 整理记录 | `/records` → RecordsPage |
| 记录 | 识别历史列表 | `/recognition/history` → HistoryPage + ExtractionHistory |
| 记录 | 识别历史详情 | `/recognition/history/:traceId` → HistoryDetailPage + ExtractionHistoryDetail |
| 配置 | 识别规则 | `/recognition/rules` → RulesPage + RecognitionRules |
| 配置 | 识别调试 | `/recognition/debug` → DebugPage + useExtraction |
| 配置 | 设置 | `/settings` → SettingsPage + useSettingsForm |

三个识别工具页是独立路由，不嵌在自动识别工作区的视图分支中。历史详情 ID、筛选路径和分页保留在 URL；设置分区使用 query 参数。支持刷新、前进后退和直接访问。

`AppSidebar` 使用 Reka Accordion，按组折叠；`MobileMenu` 使用 Reka Dialog，提供焦点限制与 Escape 关闭。页面切换关闭移动菜单和临时任务详情。

工作台、自动识别工作区、设置页通过 KeepAlive 保留编辑状态，离开时关闭临时弹窗；布局持有任务观察，因此切换页面不停止后台任务进度。规则／历史／调试页面按路由挂载与卸载，调试离开即取消。

### 查询、写入与缓存

- `app/query.ts`：唯一 QueryClient、默认策略和公共 keys。
- `app/runtime.ts`：登录后共享设置和双通道队列 Query、按需详情 Query、取消／重试／撤销操作。
- `api.ts`：带同源 Cookie 的 JSON 请求、ApiError、AbortSignal、401 事件、会话 epoch。
- 查询函数通过 `signal` 传给 API，读取按 key 去重；写入使用 mutation，成功后失效相关查询。
- GET 临时错误最多重试一次，4xx 不自动重试；写请求不自动重试。不要对整理确认增加自动重试。
- 运行中的任务约每 1.2 秒观察一次；识别页失活后停止该页的识别轮询，布局的双通道队列观察继续；空闲后所有任务与片库定时轮询停止。
- 可编辑规则、设置、工作区从 Query 数据复制为本地草稿，避免直接修改只读共享缓存。
- 自动识别草稿用工作区 ID 与 revision 防止旧查询覆盖新修正；任何新写请求都不能忽略 revision 处理。
- `clearSession` 取消查询、清空缓存与本地工作区，并使此前 API 响应失效。新请求不可绕过统一 API，否则会破坏旧会话隔离。

新增页面时在 routes 和 navigation 各登记一次，优先复用 `PageHeader`、`QueryState`、`Notices`、`Pagination`；业务状态放对应 feature，避免继续向 App.vue 堆逻辑。

## 13. 安全与限制

路径调试只验证文本，不验证文件存在、权限或真实授权。正式扫描／预览／执行使用 `fsops` 和 `sourceAllowed` 再校验，不能因试跑成功而跳过文件授权。

默认复制，不监控目录，不自动确认整理。单次导入 1–200 个路径、最多 2000 个视频，单次确认最多 1000 个视频。不做视频内容识别、下载器联动或动漫跨季自动映射。

登录之外的敏感接口沿用 Cookie 登录保护；写请求检查同源。Token、认证头、代理凭据与原始上游错误不能写入轨迹或返回前端。规则源码和路径是用户配置／数据，历史详情只向已登录用户提供。

识别调试统一使用 `POST /api/extraction/test`，只展示路径元数据规则链；不提供另一套 full/parse 调试接口。

## 14. 如何定位常见问题

| 现象 | 优先查看 | 排查顺序 |
| --- | --- | --- |
| 片名／季集提取错误 | 调试页、Step、scripts | 输入来源和边界 → 命中捕获 → 转换错误 → overwrite／applied |
| Matrix 被当成某一集 | ID 标签保护、集数脚本 | 是否使用当前链 → 是否把路径别处数字当集数 → Step 的实际输入 |
| ID 对应错误类型 | matching/match.go 与 policy.go | 路径类型依据 → 电影／剧集候选 → 标题／年份消歧 |
| 有候选却未自动采用 | 文件 reason 和候选比较 | 同名／年份冲突／回退／模糊匹配是否需要人工确认 |
| 页面切换后状态不对 | feature 草稿、runtime、query keys | KeepAlive → 查询启停 → revision → 缓存失效范围 |
| 页面刷新 404 | 后端根路由／反向代理 | 是否命中 SPA 回退 → API 与资源是否被误回退 |
| 整理预览过期 | Plan、配置 version、工作区 revision | 是否修改过设置／选择／映射 → 重新预览 |
| 整理或撤销失败 | Run.error、Ops | 每项 status／identity／checksum／暂存路径 → 独立重试或继续撤销 |
| 设置无法保存 | 执行互斥与 version | 队列是否仍活动 → 表单 version 是否过期 |
| 规则改了但历史没变化 | RuleConfig 快照 | 历史按当时配置解释；重新识别才产生新轨迹 |

后端日志负责服务级启动和异常，规则提取的业务解释以持久化 Step 为准。看到轨迹引用不存在，先确认是否旧数据无轨迹或已手工清理，不要自动补造历史。

## 15. 开发、检查与构建

在仓库根目录运行：

```sh
make install
make backend                 # 默认后端 127.0.0.1:8081
make frontend                # 另一个终端，Vite 开发服务
make check                   # 脚本检查、隐私检查、Go vet/race、前端测试与类型构建
make build                   # 内嵌静态资源，输出 local/bin/joycurator
python3 tests/run_backend.py test -tags production ./internal/webui ./internal/app
```

测试全部集中在根目录 `tests/`，**不要在业务源码旁新增测试文件**。Go 包内测试通过 `tests/run_backend.py` 的 Go overlay 映射回包，直接 `go test` 不会加载这批集中测试。前端 Vitest 从 `tests/frontend/` 发现测试。

关键回归入口：

- `tests/backend/recognition/`：纯提取、脚本、目录边界、季集、Matrix、覆盖／取消／当前配置验证。
- `tests/backend/matching/` 与 `tests/backend/app/recognition*_test.go`：纯策略、查询协调、工作区、调试与修正。
- `tests/backend/app/batch_test.go`、`tests/backend/organizer/` 与 `tests/backend/infra/sqlite/`：跨作品冲突、空间、事务、队列、取消、单部失败继续、关闭与中断恢复。
- `tests/backend/app/spa_test.go`：生产前端路由回退、API 与资源 404。
- `tests/frontend/recognition/`：规则模型、调试取消、工作区行为与脚本展示。
- `tests/frontend/app/`：路由、登录跳转、侧边导航、Query 去重／取消／失效、会话隔离。

交付检查包含 `make check`、`make build` 及 production 构建标签测试。队列回归使用可控 worker 和隔离目录，覆盖多批 FIFO、失败继续、队尾重试、双通道重叠、逐视频登记、资源等待、文件级修正和重启续跑；不使用真实收藏数据。

## 16. 后续修改建议与当前取舍

1. 调整提取行为优先编辑脚本、补路径样例；只有运行环境能力不足时才修改 script.go。避免重新增加不可查看的领域处理器。
2. 新增 Metadata 字段要同步 target 枚举、转换校验、前端模型、编辑器与展示，以及所有消费方；不要在脚本返回任意字段字典。
3. TMDB 匹配策略继续留在 matching 模块，现阶段不属于可排序的路径提取链。新引擎的 Step 仅追踪路径提取，不能据此声称已持久化完整 TMDB 决策链。
4. 规则单步错误容错继续执行，最终结果可能是部分字段。需要严格拒绝某种结果时在消费边界明确处理，避免把规则运行 error 和网络 failed 混为一种状态。
5. 轨迹长期保留、没有自动清理，较大目录会持续增加数据库体积；支持逐条清理，也支持“设置 → 数据维护”预览后按结束时间批量清理历史，并清理过期 TMDB 缓存；运行中轨迹不参与。
6. 当前规则保存不做多编辑者版本冲突控制，后保存者替换当前链；工作区修正和设置则分别有 revision/version。
7. 规则编辑草稿只在页面内存中，离开后不恢复。需要保存草稿时应新增明确机制，不能复用“当前生效链”。
8. 批次执行和恢复信息持久化，预览不持久化；服务重启区分原运行与等待状态：运行项中断后需人工重试，等待项按序重验后自动续跑。
9. 前端已按路由与 feature 分开，既有大表单与业务样式仍有继续细分空间；新增逻辑应遵循现有职责，不回填到应用外壳。

建议接手顺序：先读本节总体链路，再读 `recognition/model.go → engine.go → scripts`，然后 `recognition → matching → organizer`，配合 `infra/sqlite` 和 `app/http*.go`，最后结合前端 `routes.ts → runtime.ts → 对应 feature` 追一条实际路径。

当前持久化模型已全面关系化，只有脚本动态值和上游缓存保留 JSON/BLOB。旧 JSON 表格式不能直接启动；先校验 schema 再恢复，明确报错，不重置或迁移用户数据。逐操作和工作区批次更新使用增量检查点；执行检查点与媒体登记同事务提交。模型所有者、构造依赖、摘要读取及事务语义以 [后端架构](BACKEND.md) 为准；性能样本见 [性能记录](PERFORMANCE.md)。


本轮新增入口：记录列表的 `search/state` 筛选在 SQL 分页前执行，列表总数与当前页使用同一读取事务；详情异常筛选只作用于已加载操作。数据维护由 `app/maintenance.go` 校验选项与截止时间，`infra/sqlite/maintenance.go` 事务执行预览或清理，HTTP 不直接访问存储。清理按 `finishedAt` 截止，保留边界时刻及运行记录；缓存使用预览时确定的 `cacheBefore`，执行时不自动向后移动。数量可能随并发识别变化，响应报告实际删除数量。子表级联删除失败时历史与缓存一起回滚，工作区结果、revision、历史引用及媒体文件不改写。
