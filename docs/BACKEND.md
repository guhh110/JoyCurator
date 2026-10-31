# 后端架构：四个业务模块 + Infra

程序按一条数据流组织，不采用完整 DDD，也不为每项职责新增独立包：

```mermaid
flowchart LR
  Path[文件路径] --> Recognition[recognition 路径识别]
  Recognition --> Metadata[Metadata + Step 数组]
  Metadata --> Matching[matching 作品匹配]
  Matching --> Planning[planning 整理规划]
  Planning --> Organizer[organizer 整理执行]
  Organizer --> Run[Run 执行记录]
```

## 目录职责

```text
internal/
  app/                  装配、HTTP、工作区流程、认证与设置接入
  recognition/          规则链、Metadata、调试和轨迹用例
  matching/             作品查询、候选决策、季集校验
  planning/             计划生成、管理、校验与确认提交
  organizer/            执行、队列、记录、重试、取消、撤销
  model/                共用文件、作品、设置及操作 DTO
  infra/
    tmdb/               HTTP、代理、限速、重试、响应转换
    sqlite/             SQL、事务、认证、配置、缓存、工作区与记录
    fsops/              授权文件访问、身份、磁盘检查、安全发布
    scanner/            遍历、视频扩展名、ffprobe 探测
  rules/                命名和分类的纯计算能力
  metadata/             NFO 等产物内容生成
  auth/                 密码与会话逻辑，使用存储接口
  trace/                完整识别调试的事件记录器
  webui/                开发与生产静态资源接入
```

`recognition.Metadata` 和规则模型属于识别；`planning.Plan` 属于规划；`organizer.Run/Batch` 属于执行。跨模块确实共用的文件、作品、设置与操作值放在 `model`，不再单独拆 catalog、configuration、organization 等模型包。

`app` 可以协调一个跨模块用例，例如扫描后识别再匹配并保存工作区；它不维护第二套识别算法、命名算法或文件执行状态机。HTTP 按文件分组，避免再增加一个 transport 层。

## 依赖原则

- `app.Open` 创建基础设施和四个服务，通过构造函数注入依赖。
- recognition 和 matching 不导入 infra；识别和候选判断可以直接给数据测试。
- 业务代码不直接使用 SQLite、HTTP 客户端或第三方响应结构。需要存储或 TMDB 时使用消费方定义的接口。
- planning 和 organizer 可以使用现有授权文件与扫描能力，不为每个底层类型增加接口。
- planning 不导入 organizer，通过提交回调与文件验证接口协作，避免循环依赖。
- infra 可以实现业务需要的接口，但不执行候选判定、命名规划或任务调度。
- HTTP 文件不直接执行 SQL、遍历媒体或调用文件操作。

这些约束在 `tests/scripts/architecture_test.py` 中检查。不要再次增加同职责的 store/adapters/execution/extraction 等平行包。

## 启动和关闭

```text
main → app.Open
  → fsops.New：启动授权范围
  → sqlite.Open：WAL、单连接、原有表及历史中断恢复
  → organizer.NewCoordinator
  → recognition.New：读取并编译当前规则链
  → ConfigService / FileService / WorkspaceService
  → matching.New / planning.New / organizer.New
  → HTTP Dependencies
main → App.Handler → NewHTTP → 分组接口与中间件
退出 → App.Close
  → Coordinator.Close：关闭准入、取消执行和识别、等待 worker
  → SQLite.Close
```

关闭不持锁等待 worker，先允许它们保存中断状态，再关闭数据库。`App.Close` 使用 `sync.Once`，可重复调用。SQLite 保留一个打开连接；读取 rows 后须关闭，再做依赖同连接的其他操作。

## 1. recognition：路径告诉了我们什么

输入绝对路径和解析边界，输出统一的 Metadata 与完整步骤。自动、手动和调试均使用同一套执行器，不查询 TMDB，不决定目标路径。

```text
普通扫描／自动导入 → Service.ExtractFile
  → 获取一次当前 Engine
  → Store.StartTrace
  → Engine.Extract，observer 每步 Store.AppendStep
  → Store.FinishTrace
  → Result + traceId

调试 → Service.Test → 同一 Engine.Extract(observer=nil)
规划中读取路径 → Service.Parse → 同一 Engine.Extract(observer=nil)
```

`Metadata` 未知单值为 null、未知集数为 []，季号 0 为特别篇。先命中优先、显式覆盖、ID 标签保护、脚本限制和逐步可观测性保持不变。详见 [规则与脚本规范](RECOGNITION.md)。

保存先全量编译，再在规则锁内持久化并替换当前引擎。保存失败不替换当前链；同一次调用不切换实例，下一文件使用最新链。正式轨迹增量保存，试跑不写工作区、历史或计划。

## 2. matching：对应哪一部作品

`Resolver` 负责通过 Provider 查询、memo 去重、年份回退与过程记录，再调用纯策略 `DecideMatch/CompareCandidate`。季号和集数检查同样有只接收给定数据的纯函数。

```text
Metadata → 查询作品候选 → 纯候选策略 → Entry / candidates / state / reason
                              ↓
                       查询季集、校验映射
```

明确 ID 优先；跨类型 ID 重复需消歧。模糊、同名、年份冲突和回退搜索需确认。缺少季号仅在唯一常规季且集数有效时补全。人工选定作品仍使用同一季集校验，不另写手动逻辑。

`matching.Item`、工作区接口和 SQLite 统一保存原始 `metadata`，作品 Entry 与校验后的季集分开，不用 TMDB 结果覆盖路径提取依据。扫描接口同样返回 Metadata，不存在 parsed 字段或双格式序列化。

`matching.Service` 提供手动搜索、季和集查询。现有工作区模型保留在 matching；异步流程、人工修正提交与页面工作区协调放在 app。

## 工作区流程：自动和手动共用业务能力

```text
自动：扫描 → recognition → matching → 人工确认不确定项 → planning → organizer
手动：扫描 → recognition → 搜索／选择作品 → 同一校验 → planning → organizer
```

`WorkspaceService.Start` 在协调器锁内检查准入、保存工作区并注册 worker；worker 不探测媒体，逐文件调用识别后匹配，增量保存进度。手动扫描保留原有媒体探测行为。

人工修正在锁外联网校验，提交时锁内重新检查 revision，避免覆盖其他页面的修改。执行完成通过注入的 `FinishRun` 回调移除工作区中已完成视频，执行模块不依赖工作区实现。

## 3. planning：应该整理成什么样

输入确认后的作品、映射、文件信息与设置，仅生成视频及同名字幕的整理清单和待登记的目标视频映射。`Preview` 和 `BatchPreview` 共用 `buildPreview`；多作品合并后再检查跨作品目标与合计空间。刮削预览从已登记的目标视频生成独立 NFO/图片清单；不扫描旧媒体或未整理的来源文件。

- `Get`：读取当前计划。
- `Edit`：重新生成并校验单作品预览，生成新 ID，旧确认失效；失败保留旧预览。
- 批次编辑：仍通过现有工作区修正和 revision，再调用 `BatchPreview`；不维护第二套批次编辑输入。
- `Delete`：只移除未提交的内存预览，不影响文件或执行记录。
- `Submit`：确认并交给注入的 organizer，保留既有 Run/Batch 回执。

计划仍在内存，15 分钟有效；保留原有最多 32 份缓存策略。重启后须重新预览。设置保存失效全部预览，工作区修改通过 revision 阻止旧批次执行。

## 4. organizer：实际操作与恢复

organizer 在已有 Engine 基础上统一管理队列、当前任务、取消、记录、重试与撤销：

```text
Planning.Submit → Execute / ExecuteBatch
  → 协调器锁内读取计划、检查版本、revision、来源和目标
  → 保存 Run，或事务写入整批 queued Runs
  → 保留执行槽并启动 worker
  → Engine.Run / Undo → infra/fsops
  → 每步持久化进度、身份、哈希和暂存信息
```

`Coordinator` 是 running、batch、取消函数、后台登记、closed 与等待计数的唯一所有者。批次在作品切换间隙仍占用执行槽；每部开始前重验来源、目标和设置版本，单部失败继续下一部。批次取消传播到当前任务及未开始项；关闭标为 interrupted。

设置保存与执行准入共用锁，避免检查空闲后被另一请求抢占。保留原有准入规则，不额外收紧识别期间已允许的操作；识别使用开始时的设置快照。

整理与刮削分别记录和撤销，共用后台执行互斥。重试使用持久化操作，不重新识别或命名。撤销逆序进行，仍被其他有效刮削记录引用的共享产物保留；无备份覆盖的旧内容不能恢复。取消或中断后可继续。删除预览不能撤销文件，清理提取轨迹也不能删除执行记录。

## 数据与安全

开发阶段只维护当前表结构、接口与 JSON 格式：管理员、会话、配置、TMDB 缓存、工作区、规则、轨迹、步骤及 runs。格式变更直接同步调用方；不提供旧数据迁移、旧接口或字段适配。读取不支持的字段直接报错，不自动重写或删除数据。批次由 Run.batchId 关联，不增加批次表。正常响应缓存仍为 24 小时，轨迹不自动清理。

启动将未完成 Run、工作区和轨迹标为 interrupted，不自动恢复文件操作。规则仅接受 script/regex，不存在 builtin 处理器或转换器。任务中断恢复属于当前业务生命周期，不是版本迁移。

Token 不返回前端、不写入调试轨迹；认证和同源检查保留。路径试跑只处理文本，正式导入、预览、执行仍进行授权与文件身份检查。

## 测试与接手入口

全部测试在根目录 `tests/`，使用 `tests/run_backend.py` 的 Go overlay 加载。直接 `go test` 不包含集中测试。

- recognition：路径回归、脚本、规则保存、轨迹、取消及调用一致性。
- matching：纯候选／季集策略、模拟 TMDB、Metadata 接口与存储契约。
- planning 与 app：预览编辑删除、旧确认失效、真实装配、来源与空间、手动／批量流程及 HTTP。
- organizer：执行竞争、批次间隙占用、失败继续、取消、独立重试撤销、关闭等待。
- infra/sqlite：当前数据校验、事务回滚、分页、快照、清理、缓存与重启中断。
- scripts：依赖边界和发布隐私检查。

执行 `make check`、`make build`、`python3 tests/run_backend.py test -tags production ./internal/webui ./internal/app`。接手建议按 recognition → matching → planning → organizer 阅读，随后追 app 的工作区流程和 infra 实现。
