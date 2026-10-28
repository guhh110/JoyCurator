# API

接口前缀 `/api`，JSON 请求和响应。错误结构为 `{"message":"原因"}`。除健康检查、会话状态、初始化与登录外均需 HttpOnly 会话 Cookie。写请求检查同源，反向代理需保留原始 Host。

| 方法及路径 | 用途 |
| --- | --- |
| `GET /health` | HTTP、数据库健康检查 |
| `GET /session` | `{initialized,username}`；未登录时用户名为空 |
| `POST /setup` | `{username,password}`，首次创建管理员并登录 |
| `POST /login` | `{username,password}`，登录 |
| `POST /logout` | 注销会话 |
| `GET /settings` | 设置、授权 `roots`、凭据存在标志 `hasToken`；不返回 Token |
| `PUT /settings` | 设置及当前 `version`，可选 `tmdbToken`；版本过期拒绝保存 |
| `GET /browse?path=` | 默认仅浏览已配置源目录；目录设置选择器可用 `scope=roots` 浏览授权根目录，跳过隐藏文件和符号链接 |
| `POST /scan` | `{paths}`，递归扫描，返回视频数组 |
| `GET /search?type=&q=&year=` | 搜索 TMDB，`type` 为 `movie` 或 `tv` |
| `GET /seasons?id=` | 作品的季列表 |
| `GET /episodes?id=&season=` | 指定季的集列表，季号 0 为特别篇 |
| `POST /preview` | `{type,tmdbId,mode,files}`，创建后端预览 |
| `POST /execute` | `{id}`，执行已确认的预览，返回任务 |
| `GET /recognitions` | 最近的识别工作区，无记录时返回 `null` |
| `POST /recognitions` | `{paths,id?}`，创建异步识别任务；提供 `id` 时补充现有工作区并保留修正 |
| `GET /extraction/rules` | 当前链、默认链、完整脚本模板目录、目标及来源枚举 |
| `PUT /extraction/rules` | `{rules}`，全量验证并原子替换当前链 |
| `POST /extraction/test` | `{path,root?,rules?}`，试跑保存链或提供的编辑链，返回 `{path,root,metadata,steps}` |
| `POST /extraction/examples` | `{rules}`，返回各规则样例的 `{ruleId,path,expected,actual,passed}` 数组 |
| `GET /extraction/traces?page=&path=&workspaceId=` | 每页 20 条执行摘要，返回 `{items,total,page}`；路径为精确筛选 |
| `GET /extraction/traces/{id}` | `{trace,metadata,steps}`，含执行时规则快照 |
| `DELETE /extraction/traces/{id}` | 手动清理已结束轨迹及步骤，不修改工作区结果 |
| `POST /recognitions/debug` | `{path,mode,root?}`，只模拟单文件识别，返回结果及逐步追踪，不创建任务 |
| `GET /recognitions/{id}` | 识别进度、候选、匹配依据、文件分组及选中状态 |
| `PUT /recognitions/{id}` | `{revision,items}`，保存人工修正并校验作品与季集；拒绝过期版本 |
| `POST /recognitions/{id}/cancel` | 取消识别，已完成结果保留 |
| `POST /recognitions/{id}/retry` | 重试失败与未完成项，保留已处理文件的人工修正 |
| `POST /batches/preview` | `{recognitionId,revision,mode}`，预览选中文件，统一检查跨作品目标冲突及空间 |
| `POST /batches/execute` | `{id}`，确认批量预览，持久化逐作品队列，返回 `{id,runs,active}` |
| `GET /batches` | 最近的自动整理批次，无记录时返回 `null` |
| `GET /batches/{id}` | `{id,runs,active}`，逐作品状态与批次是否仍在执行 |
| `POST /batches/{id}/cancel` | 取消当前作品及未启动队列项，已完成记录保留 |
| `GET /runs?page=` | 整理批次，固定每页 20 条，返回 `{items,total,page}` |
| `GET /runs/{id}` | 批次详情、逐文件状态和错误 |
| `POST /runs/{id}/cancel` | 取消运行中的任务 |
| `POST /runs/{id}/retry` | 重试失败、取消或中断的整理 |
| `POST /runs/{id}/undo` | 整批撤销；撤销中断后再次调用以继续 |

扫描文件结构含 `path`、`size`、`mtime`、`season`、`episodes` 和 `probe`。`mtime` 为纳秒时间戳字符串，避免 JavaScript 数值精度损失；未知季号为 `-1`，预览前由用户选择的季补全。

预览返回 `id`、`rows`、`required`、`issues`。仅无阻塞问题的预览可执行；需要跳过冲突文件时，去掉对应文件后重新请求预览。前端不提交文件操作指令。

任务状态为 `queued`、`running`、`completed`、`failed`、`cancelled`、`interrupted`、`undone`。前端轮询详情获取进度；服务重启后运行中的批次标为中断，操作的临时路径、文件身份与校验值仍持久化，便于重试或撤销。

每次只运行一个整理或撤销任务。自动整理队列占用执行槽直到整批结束，期间禁止其他执行及设置变更；单部作品失败后继续下一部。每部作品的记录带可选 `batchId`，可独立重试、撤销。重启后运行中和排队中的记录均标为中断，不自动执行。

手动工作区保存在浏览器；自动识别工作区和修正、队列、整理记录保存在 SQLite，预览仅短期保存在内存。

## 自动识别工作区

识别任务返回 `id`、`revision`、`state`、`paths`、`items`、`completed`、`total`、`error`、`created`。任务状态为 `running`、`completed`、`failed`、`cancelled`、`interrupted`。每个文件在扫描字段之外包含可选 `traceId`（旧结果无轨迹）、`parsed`、`group`、`entry`、`candidates`、`state`、`reason`、`selected`、`removed`；文件状态为 `pending`、`ready`、`review`、`unmatched`、`failed`。只有 `ready` 的选中文件可进入预览。

保存修正需提交完整文件列表；每项为 `{path,group,type,tmdbId,season,episodes,selected,removed}`。大小和修改时间由服务端保留，不接受前端修改。`tmdbId:0` 表示尚未选定作品。服务端校验季集，并自动取消无效文件的选择。预览绑定工作区版本，任何后续保存均要求重新预览。

识别不运行媒体探测；目录推断止于当前 `sourceRoot`。同批搜索去重、限制请求速率并复用 TMDB 缓存；年份回退及模糊候选需确认。缺少季号时，仅在唯一常规季且集数有效时自动补全。 TMDB ID 标记在提取后不再参与集数识别；同一 ID 在电影和剧集类型中均存在时，仅在片名／原名及已知年份唯一匹配时自动消歧，否则保留待确认。

## 路径元数据规则链

新调试页使用 `POST /extraction/test`，只解析路径文本，不读取媒体文件、不访问 TMDB，无需 Token。支持不存在的路径；`root` 必须为其父级绝对目录，省略时路径在源目录内采用源目录，否则 `/`。`rules` 省略时使用当前链，显式 `[]` 表示空链。试跑与样例不创建工作区、预览、整理记录或历史轨迹。前端 90 秒超时，取消或离开页面时中止请求并忽略过期响应。

`GET /extraction/rules` 返回 `{rules,defaults,catalog,targets,sources,sourceScripts,scriptLanguage}`，`scriptLanguage` 为 `starlark`；模板为 `{id,name,target,description,sources,config:{code,params?}}`，源码可直接编辑。规则类型为 `script` 或 `regex`；脚本配置为 `{code,params?,sourceCode?}`，正则配置仍为 `{pattern,capture|value,sourceCode?}`。`sourceCode` 仅用于 `cleanFilename` 来源的清理过程。旧内置配置启动时迁移，不接受新建 `builtin`。

`Step.attempts[].trace` 保存脚本的正则调用、`trace/print` 中间值、阶段和源码行列号。脚本语法、参数结构错误在保存或试跑时返回 HTTP 400；运行错误和超限作为该规则的 `error` 步骤保留日志，继续执行后续规则。历史详情中的配置与源码均来自当次快照。

`Metadata` 的单值字段未知为 `null`，集数未知为 `[]`；`season:0` 为特别篇。`Step` 保存每条规则的配置、输入尝试、命中与捕获、提出的值、前后值、采用标志、原因、时间和耗时；`matched` 状态不等于已采用。规则模型及执行语义详见 [路径元数据提取](RECOGNITION.md)。当前链保存无效时保留原链；单次调用只使用开始时获取的链。正式轨迹长期保存，重启将运行中轨迹标为中断；删除轨迹不会修改原识别结果。

## 兼容识别调试接口

以下原接口继续保留，其路径解析阶段已复用当前规则执行器；TMDB 匹配仍由既有调用方负责。

`POST /recognitions/debug` 需要登录并通过同源检查，`mode` 为 `full`（含 TMDB）或 `parse`（不联网且无需 Token）。`path` 是以 `/` 开头的完整文件路径，可以不存在；不执行文件存在性、权限或内容探测。输入路径及可选 `root` 按文本规范化，`root` 必须是文件的父级目录。未指定边界时，源目录内路径使用 `sourceRoot`，其他路径使用 `/`。

返回 `{mode,input,parsed,entry,candidates,state,reason,season,episodes,steps,durationMs}`。`parsed` 是原始路径解析结果，顶层 `season`、`episodes` 是匹配校验后的结果。`state` 可为 `parsed`、`ready`、`review`、`unmatched`、`failed`；`ready` 仅表示模拟识别通过，不代表真实文件可执行整理。

`input` 包含原始及规范化路径、实际解析边界与来源、扩展名是否受支持、是否处于源／目标目录的文本范围；`fileExistence` 和 `fileAccess` 固定为 `unverified`。不受支持的扩展名、源目录外或目标目录内路径仍可调试，步骤中说明正式导入会跳过或拒绝的原因。

`steps` 按执行顺序返回 `{stage,rule,status,message,data?,elapsedMs}`，阶段为 `input`、`parse`、`tmdb`、`match`、`validate`、`result`。`elapsedMs` 为自调试开始的累计时间；TMDB 请求的 `data.durationMs` 为该请求耗时，同时记录缓存命中、请求尝试次数及候选比较依据。开启追踪不会改变正式识别器的决策。

输入错误返回 HTTP 400；缺少 Token、配置错误、TMDB 失败及季集失败保留已完成的步骤，以 HTTP 200 返回相应最终状态。服务端最多执行 90 秒；前端可主动取消，离开调试页时中止请求。完整模式只允许正常更新 TMDB 缓存，不修改配置、识别工作区、整理记录或预览。追踪不包含认证头、Token、代理配置、完整上游地址或原始错误响应。

## 目录与分类

设置使用 `sourceRoot`、`targetRoot` 两个绝对路径。容器默认分别为 `/source`、`/target`，数据库路径为 `/data`。返回的 `roots` 表示启动时授权的目录范围；设置只能选择该范围内已存在的目录。源与目标不能相同，源也不能位于目标目录内部。

分类规则的 `name` 是目标目录下的相对子目录，允许多级路径，例如 `电影/华语电影`、`电视剧/国产剧`。规则按顺序匹配，首个命中规则的路径填入 `{category}`；最终文件路径统一以 `targetRoot` 为根。禁止绝对路径、`.`、`..`、空路径段和非法目录名。

扫描及预览均校验来源必须位于 `sourceRoot` 内且不在 `targetRoot` 内。显式提交目标文件也会被拒绝。`scope=roots` 只供设置目录浏览，不会放宽扫描或预览权限。
