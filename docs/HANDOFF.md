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
3. **整理执行**：根据已确认的作品与映射生成预览，检查冲突，执行复制／移动／硬链接、字幕和刮削产物。代码在 `planning/preview.go`、`organizer/jobs.go`、`organizer/batch.go` 和 `organizer`。

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
    planning/                 Plan 生成、修改、删除、提交
    organizer/                Run、队列、文件操作、记录与回退
    model/                    共用文件、作品、设置与操作 DTO
    infra/tmdb/               TMDB 请求、响应转换和缓存端口
    infra/sqlite/             SQL、事务和持久化
    infra/fsops/              授权文件操作
    infra/scanner/            文件遍历和媒体探测
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

`local/archive/legacy-20261027/source/frontend` 是本次参考的旧前端备份，不参与当前构建。采用了其中的 Vue Router、TanStack Vue Query、布局与通用组件组织思路，没有把备份目录作为运行依赖。

## 3. 后端启动与职责划分

启动链：

```text
main.main
  → 读取 JOYCURATOR_* 环境变量，准备源／目标目录
  → app.Open(dataDir, sourceRoot, targetRoot, secure)
    → 创建授权文件系统 fsops.FS
    → 打开 joycurator.db、启用 WAL / busy_timeout / foreign_keys
    → 建表、写入首次默认设置
    → sqlite.Open：原有表与历史中断恢复
    → 创建 organizer.Coordinator
    → recognition.New：读取并编译当前链
    → 装配设置、目录、识别、规划、执行与 HTTP 服务
    → 校验设置
    → 将遗留 running / queued 整理任务及运行中识别标为 interrupted
  → App.Handler(webui.Files())
  → http.Server.ListenAndServe
```

退出时 HTTP 服务先停止接收请求，随后 `App.Close` 取消当前整理、识别与批次，等待后台工作结束，再关闭数据库。

`App` 装配四个业务模块与 infra，并接入 HTTP、设置、认证和工作区流程；当前链属于 recognition，预览属于 planning，任务与后台生命周期由 organizer.Coordinator 管理。业务模块不直接依赖 SQL／HTTP，具体边界与调用链见 [后端架构](BACKEND.md)。

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
  → recognition 的规则锁内通过 Store 保存 extraction_rules
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
  → Engine.Extract(observer)
      每完成一条 Step → 插入 extraction_steps
  → 保存最终 metadata、status、finishedAt、error
  → 返回 Result 与 traceId
```

手动 `/scan` 和自动识别都会调用 `ExtractFile`；手动扫描还会做媒体探测，自动识别阶段不探测。消费方的无持久化适配入口是 `recognition.Service.Parse`。

工作区文件行只携带 `traceId`，历史列表分页读取摘要，选中详情才获取全部步骤。每步含完整规则及源码快照，当前规则改名、修改或删除不影响旧记录。重启将遗留 running 轨迹标记 interrupted。清理仅允许已结束轨迹，关联 steps 通过外键级联删除，不删除工作区结果。

注意：轨迹 completed 表示执行器完成整条链，不代表没有单条规则 error，也不代表 TMDB 匹配成功。定位时同时检查 Step 和工作区状态。

## 8. 自动识别、匹配与人工修正

```text
AutomaticRecognitionPage → useAutomaticRecognition.add
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

## 9. 手动整理与预览调用链

```text
WorkbenchPage → useWorkbench.add → POST /scan
  → scanner.Walk → ExtractFile → scanner.Probe
  → 浏览器保存文件列表
搜索作品 → GET /search → 选择 Entry
  → 剧集再 GET /seasons、GET /episodes → 修正映射
预览 → POST /preview → Planning.Preview → buildPreview
  → 校验源路径和文件身份、读取设置与 TMDB 元数据
  → rules.Target / ShowRoot 渲染分类与命名
  → 生成视频／字幕／NFO／图片操作
  → 校验冲突、磁盘需求等 → storePlan
确认 → POST /execute {id} → Planning.Submit → Organizer.Execute → launch → work
  → organizer.Engine.Run，逐步持久化操作状态
```

`buildPreview` 是手动与批量共用的规划入口。前端只提交作品、映射和模式，不能提交可信文件操作指令。预览在内存中短期保存（15 分钟），包含配置版本及服务端生成的 ops；服务重启后须重新预览。

`organizer` 按操作处理视频、字幕和刮削产物，通过 `fsops` 完成受限目录内的读写、暂存与发布。操作记录保留身份、哈希、暂存路径及进度，供恢复和撤销使用；不要绕开这层直接移动媒体。

## 10. 多作品批次与执行互斥

```text
POST /batches/preview {recognitionId,revision,mode}
  → Planning.BatchPreview：仅收集已选 ready 文件，按作品分组
  → 每组 buildPreview
  → 合并全部 ops，检查跨作品重复目标与合计空间
  → 保存统一 Plan（绑定工作区 revision 与设置 version）
POST /batches/execute {id}
  → Planning.Submit → Organizer.ExecuteBatch：重验预览、revision、设置、来源与目标
  → 事务写入每作品一条 queued Run，带 batchId
  → 占用批次执行槽 → runBatch 顺序调用 work
```

批次不是单独的数据库表，`batchId` 将 `runs` 中的作品记录关联起来，`readBatch` 重建批次视图。每部作品启动前重新校验配置版本、授权来源和操作条件；某部失败继续下一部。

`organizer.Coordinator` 管当前单任务和整个队列的互斥占用，避免两部作品之间的空隙被其他执行抢占。手动执行、重试、撤销和另一批次共享执行限制；队列结束前不允许设置变更。

批次取消作用于当前任务和尚未开始项；已完成记录保留。每部作品独立重试／撤销。撤销按操作逆序执行，公共 NFO／图片的共享所有权通过记录判断，不能盲删。重启后 queued/running 均标为 interrupted，不自动恢复执行。

## 11. 数据保存在哪里

| 内容 | 存储 | 说明 |
| --- | --- | --- |
| 管理员与会话 | SQLite administrators / sessions | 密码 bcrypt、会话 Token 存 SHA256 哈希 |
| 设置与 TMDB 凭据 | SQLite config | 对外只返回凭据存在标志 |
| TMDB 响应 | SQLite tmdb_cache | 带过期时间，正常响应缓存 24 小时 |
| 当前提取规则链 | SQLite extraction_rules | 单行 JSON，保存即替换 |
| 提取执行与步骤 | SQLite extraction_traces / extraction_steps | 长期保留，人工清理 |
| 自动识别工作区 | SQLite recognitions | JSON，含 revision、修正、候选、traceId |
| 整理／撤销记录与队列 | SQLite runs | JSON，操作恢复信息与可选 batchId |
| 尚未执行的预览 | planning.Service 内存 | 过期或重启失效 |
| 手动文件列表、作品、活动任务引用 | 浏览器 localStorage | 跨刷新恢复；退出清空 |
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
| 文件整理 | 手动工作台 | `/organize` → WorkbenchPage + useWorkbench |
| 文件整理 | 整理记录 | `/records` → RecordsPage |
| 自动识别 | 工作区 | `/recognition` → AutomaticRecognitionPage + useAutomaticRecognition |
| 自动识别 | 识别规则 | `/recognition/rules` → RulesPage + RecognitionRules |
| 自动识别 | 执行历史 | `/recognition/history/:traceId?` → HistoryPage + ExtractionHistory |
| 自动识别 | 识别调试 | `/recognition/debug` → DebugPage + useExtraction |
| 系统设置 | 设置 | `/settings` → SettingsPage + useSettingsForm |

三个识别工具页是独立路由，不嵌在自动识别工作区的视图分支中。历史详情 ID、筛选路径和分页保留在 URL；设置分区使用 query 参数。支持刷新、前进后退和直接访问。

`AppSidebar` 使用 Reka Accordion，按组折叠；`MobileMenu` 使用 Reka Dialog，提供焦点限制与 Escape 关闭。页面切换关闭移动菜单和临时任务详情。

工作台、自动识别工作区、设置页通过 KeepAlive 保留编辑状态，离开时关闭临时弹窗；布局持有任务观察，因此切换页面不停止后台任务进度。规则／历史／调试页面按路由挂载与卸载，调试离开即取消。

### 查询、写入与缓存

- `app/query.ts`：唯一 QueryClient、默认策略和公共 keys。
- `app/runtime.ts`：登录后共享设置 Query、当前批次轮询、活动手动任务及详情 Query、本地文件／作品、重试撤销操作。
- `api.ts`：带同源 Cookie 的 JSON 请求、ApiError、AbortSignal、401 事件、会话 epoch。
- 查询函数通过 `signal` 传给 API，读取按 key 去重；写入使用 mutation，成功后失效相关查询。
- GET 临时错误最多重试一次，4xx 不自动重试；写请求不自动重试。不要对整理确认增加自动重试。
- 运行中的任务约每 1.2 秒观察一次；识别页失活后停止该页的识别轮询，布局的批次／活动任务观察继续。
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

交付检查包含 `make check`（前端 9 个测试文件、47 个用例）、`make build` 及 production 构建标签测试；浏览器验证了三工具页、历史详情刷新、手机抽屉与宽度、手动导入／查询／作品映射及切页保留状态。浏览器验证使用隔离测试目录，没有用真实收藏执行整理。

## 16. 后续修改建议与当前取舍

1. 调整提取行为优先编辑脚本、补路径样例；只有运行环境能力不足时才修改 script.go。避免重新增加不可查看的领域处理器。
2. 新增 Metadata 字段要同步 target 枚举、转换校验、前端模型、编辑器与展示，以及所有消费方；不要在脚本返回任意字段字典。
3. TMDB 匹配策略继续留在 matching 模块，现阶段不属于可排序的路径提取链。新引擎的 Step 仅追踪路径提取，不能据此声称已持久化完整 TMDB 决策链。
4. 规则单步错误容错继续执行，最终结果可能是部分字段。需要严格拒绝某种结果时在消费边界明确处理，避免把规则运行 error 和网络 failed 混为一种状态。
5. 轨迹长期保留、没有自动清理，较大目录会持续增加数据库体积；当前只支持手动清理已结束记录。
6. 当前规则保存不做多编辑者版本冲突控制，后保存者替换当前链；工作区修正和设置则分别有 revision/version。
7. 规则编辑草稿只在页面内存中，离开后不恢复。需要保存草稿时应新增明确机制，不能复用“当前生效链”。
8. 批次执行和恢复信息持久化，预览不持久化；服务重启只标中断，不自动继续危险的文件操作。
9. 前端已按路由与 feature 分开，既有大表单与业务样式仍有继续细分空间；新增逻辑应遵循现有职责，不回填到应用外壳。

建议接手顺序：先读本节总体链路，再读 `recognition/model.go → engine.go → scripts`，然后 `recognition → matching → planning → organizer`，配合 `infra/sqlite` 和 `app/http*.go`，最后结合前端 `routes.ts → runtime.ts → 对应 feature` 追一条实际路径。
