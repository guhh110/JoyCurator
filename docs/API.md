# API

开发阶段只维护当前接口与数据模型，不保留旧接口、字段别名、旧数据迁移或双格式读写。格式不符直接报错，不自动重建数据。


接口前缀 `/api`，JSON 请求和响应。错误结构为 `{"message":"原因"}`。除健康检查、会话状态、初始化与登录外均需 HttpOnly 会话 Cookie。写请求检查同源，反向代理需保留原始 Host。

| 方法及路径 | 用途 |
| --- | --- |
| `GET /health` | HTTP、数据库健康检查 |
| `GET /session` | `{initialized,username}`；未登录时用户名为空 |
| `POST /setup` | `{username,password}`，首次创建管理员并登录 |
| `POST /login` | `{username,password}`，登录 |
| `POST /logout` | 注销会话 |
| `GET /settings` | 设置、授权 `roots`、凭据存在标志 `hasToken`；不返回 Token |
| `PUT /settings` | 设置及当前 `version`，可选 `tmdbToken`；版本过期、识别未结束或任一通道运行／排队时拒绝保存 |
| `POST /maintenance/preview` | 预览已结束识别历史及过期 TMDB 缓存的清理数量，返回 `{request,traces,cache}` |
| `POST /maintenance/cleanup` | 提交确认的清理条件，同一事务删除所选历史及其全部子表和过期缓存，返回实际数量 `{traces,cache}` |
| `GET /browse?path=` | 默认仅浏览已配置源目录；目录设置选择器可用 `scope=roots` 浏览授权根目录，跳过隐藏文件和符号链接 |
| `GET /search?type=&q=&year=` | 搜索 TMDB，`type` 为 `movie` 或 `tv` |
| `GET /seasons?id=` | 作品的季列表 |
| `GET /episodes?id=&season=` | 指定季的集列表，季号 0 为特别篇 |
| `GET /scrape/works?q=&cursor=` | 刮削作品概览，按类型和 TMDB ID 稳定排序，每次最多 50 部；返回 `{items:[{type,tmdbId,representative,total,pending,failed,completed}],nextCursor}`。搜索匹配目标路径或 TMDB ID；不逐个检查视频是否可用 |
| `GET /scrape/works/{kind}/{id}` | 指定电影或剧集的全部已登记目标视频及实时可用状态，返回 `MediaMapping[]`；临时状态含 `queued/running/unavailable`，不扫描旧媒体 |
| `POST /scrape/preview` | `{paths,content,onlyMissing}`，仅接受已登记且可用的同一作品，单次 1–1000 个目标视频，返回产物、空间与冲突 |
| `POST /scrape/execute` | `{id}`，确认刮削预览；提交时复核所选映射与文件，返回持久化 `queued` Run 摘要 |
| `GET /recognitions` | 最近的识别工作区，无记录时返回 `null` |
| `POST /recognitions` | `{paths,id?}`，创建异步识别任务；提供 `id` 时补充现有工作区并保留修正 |
| `GET /extraction/rules` | 当前链、默认链、完整脚本模板目录、目标及来源枚举 |
| `PUT /extraction/rules` | `{rules}`，全量验证并原子替换当前链 |
| `POST /extraction/test` | `{path,root?,rules?}`，试跑保存链或提供的编辑链，返回 `{path,root,metadata,steps}` |
| `POST /extraction/examples` | `{rules}`，返回各规则样例的 `{ruleId,path,expected,actual,passed,error}` 数组；`error` 无错误时为空字符串，整条链任一规则执行报错均判为失败，即使 `actual` 与 `expected` 都为 `null` |
| `GET /extraction/traces?page=&path=&workspaceId=` | 每页 20 条执行摘要，返回 `{items,total,page}`；路径为精确筛选 |
| `GET /extraction/traces/{id}` | `{trace,metadata,steps}`，含执行时规则快照 |
| `DELETE /extraction/traces/{id}` | 手动清理已结束轨迹及步骤，不修改工作区结果 |
| `GET /recognitions/{id}` | 识别进度、候选、匹配依据、文件分组及选中状态 |
| `PUT /recognitions/{id}` | `{revision,items}`，只提交变更文件，保存人工修正并校验作品与季集；拒绝过期版本及已锁定／完成文件 |
| `POST /recognitions/{id}/cancel` | 取消识别，已完成结果保留 |
| `POST /recognitions/{id}/reidentify` | `{revision,paths}`，对 1–2000 个已有文件按当前规则重新识别，保留人工修正，拒绝过期 revision、已移出及已提交文件 |
| `POST /recognitions/{id}/retry` | 重试失败与未完成项，保留已处理文件的人工修正 |
| `GET /queue` | `{revision,active,recognizing,organize,scrape}`；每通道返回 `{current,queued,queuedBatches,waitReason}`，`current` 为活动 Run 摘要或 `null`，等待时显示原因 |
| `GET /queue/items?purpose=&page=` | `purpose=organize/scrape`，按实际出队顺序返回等待任务，固定每页 20 条 `{items,total,page}`；不含运行中任务 |
| `POST /batches/preview` | `{recognitionId,revision,mode,duplicatePolicy}`，预览选中文件，统一检查跨作品目标冲突及空间；重复策略必填 `error/skip/overwrite` |
| `POST /batches/execute` | `{id}`，确认批量预览，持久化逐作品队列，返回 `{id,runs,active,state,position}` |
| `GET /batches?page=` | 分页批次列表，固定每页 20 批 `{items,total,page}`，空列表为 `[]` |
| `GET /batches/{id}` | `{id,runs,active,state,position}`，持久化任务推导状态及队列批次位置；`position=0` 表示已无队列项 |
| `POST /batches/{id}/cancel` | 取消当前作品及未启动队列项，已完成记录保留 |
| `GET /runs?page=&search=&state=` | 整理任务摘要，作品名称包含搜索及任务状态筛选，先筛选后分页，固定每页 20 条 `{items,total,page}` |
| `GET /runs/{id}` | 整理任务详情、逐文件状态和错误 |
| `POST /runs/{id}/cancel` | 取消排队中或运行中的任务 |
| `POST /runs/{id}/retry` | 重试失败、取消或中断的整理 |
| `POST /runs/{id}/undo` | 该作品逆序撤销；撤销中断后再次调用以继续 |

工作区文件结构含 `path`、`size`、`mtime`、`metadata`、`season` 和 `episodes`。metadata 是规则直接提取的原始结果，季集映射可由用户另行修正。`mtime` 为纳秒时间戳字符串，避免 JavaScript 数值精度损失；未知季号为 `-1`，预览前由用户选择的季补全。

预览返回 `id`、`rows`、`required`、`issues`。仅无阻塞问题的预览可执行；需要跳过冲突文件时，去掉对应文件后重新请求预览。前端不提交文件操作指令。

任务状态为 `queued`、`running`、`completed`、`failed`、`cancelled`、`interrupted`、`undone`。前端轮询详情获取进度；服务重启后运行中的批次标为中断，操作的临时路径、文件身份与校验值仍持久化，便于重试或撤销。

整理与刮削各有一个执行通道，可同时运行。整理批次按提交顺序逐作品执行；失败、取消及完成都会推进队列。新任务、重试和撤销统一持久化入队，重试追加队尾。跨通道访问冲突等待资源，不持有部分访问锁。每部作品记录带可选 `batchId`，可独立取消、重试、撤销。关闭停止出队并等待当前任务保存；重启将原运行项标为 `interrupted`，已持久化等待项保留原序、复核后继续。队列数据不一致直接报错，不自动修复。

工作区和修正、队列、整理记录保存在 SQLite；浏览器仅维护编辑草稿与视图状态，活动任务以 `/queue` 为准。两类预览共享内存缓存，15 分钟有效、合计最多 32 份，满额逐出最早预览，设置保存后全部失效。有效期仅约束提交；Run 与队列位置同事务写入后立即返回回执，等待中的任务不再受预览有效期限制，开始时重验配置、授权、文件与空间。

## 自动识别工作区

识别任务返回 `id`、`revision`、`state`、`paths`、`items`、`completed`、`total`、`error`、`created`。任务状态为 `running`、`completed`、`failed`、`cancelled`、`interrupted`。每个文件在扫描字段之外包含可选 `traceId`、`metadata`、`group`、`entry`、`candidates`、`state`、`reason`、`selected`、`removed`；文件状态为 `pending`、`reidentifying`、`ready`、`review`、`unmatched`、`failed`。只有 `ready` 的选中文件可进入预览。

保存修正只提交变更文件，保留 revision 检查；每项为 `{path,group,type,tmdbId,season,episodes,selected,removed}`。大小和修改时间由服务端保留，不接受前端修改。`tmdbId:0` 表示尚未选定作品。服务端校验季集，并自动取消无效文件的选择。预览绑定工作区版本，人工保存或识别增量保存后要求重新预览。执行进度不修改编辑 revision，不使无关预览过期。

工作区文件可带 `execution:{runId,state,locked,completed,manualReview}`，由持久化任务投影而来，不接受客户端写入。已提交文件不可修改作品、季集、分组或选择，也不可重复提交；完成并登记的视频不再参与整理选择。失败／取消后未完成文件释放编辑限制，状态不明文件继续锁定、需人工核对。移出来源接口 `POST /recognitions/{id}/sources/remove {path,revision}` 拒绝含锁定文件的来源并报告任务 ID。

视频完成安全落盘及身份核验后，操作检查点与媒体映射同事务提交，随后即能手动选择刮削。跳过已有视频也先核验并登记；尚在发布或状态不明的视频不能刮削。整理最终保存、取消及重试不会重置同一身份视频的已完成刮削状态。

识别不运行媒体探测；目录推断止于当前 `sourceRoot`。同批搜索去重、限制请求速率并复用 TMDB 缓存；年份回退及模糊候选需确认。缺少季号时，仅在唯一常规季且集数有效时自动补全。 TMDB ID 标记在提取后不再参与集数识别；同一 ID 在电影和剧集类型中均存在时，仅在片名／原名及已知年份唯一匹配时自动消歧，否则保留待确认。

## 路径元数据规则链

新调试页使用 `POST /extraction/test`，只解析路径文本，不读取媒体文件、不访问 TMDB，无需 Token。支持不存在的路径；`root` 必须为其父级绝对目录，省略时路径在源目录内采用源目录，否则 `/`。`rules` 省略时使用当前链，显式 `[]` 表示空链。试跑与样例不创建工作区、预览、整理记录或历史轨迹。前端 90 秒超时，取消或离开页面时中止请求并忽略过期响应。

`GET /extraction/rules` 返回 `{rules,defaults,catalog,targets,sources,sourceScripts,scriptLanguage}`，`scriptLanguage` 为 `javascript`；模板为 `{id,name,target,description,sources,config:{code,params?}}`，源码可直接编辑。规则类型为 `script` 或 `regex`；脚本配置为 `{code,params?,sourceCode?}`，正则配置仍为 `{pattern,capture|value,sourceCode?}`。`sourceCode` 仅用于 `cleanFilename` 来源的清理过程。仅接受当前 script/regex 配置，不支持处理器字段或 builtin 类型。

`Step.attempts[].trace` 保存脚本的正则调用、`trace/print` 中间值、阶段和源码行列号。脚本语法、参数结构错误在保存或试跑时返回 HTTP 400；运行错误和超限作为该规则的 `error` 步骤保留日志，继续执行后续规则。历史详情中的配置与源码均来自当次快照。

`Metadata` 的单值字段未知为 `null`，集数未知为 `[]`；`season:0` 为特别篇。`Step` 保存每条规则的配置、输入尝试、命中与捕获、提出的值、前后值、采用标志、原因、时间和耗时；`matched` 状态不等于已采用。规则模型及执行语义详见 [路径元数据提取](RECOGNITION.md)。当前链保存无效时保留原链；单次调用只使用开始时获取的链。正式轨迹逐文件集中写入步骤和最终摘要，长期保存；正常取消保存已产生步骤，强制退出时当前文件可能只有摘要，重启标为中断；删除轨迹不会修改原识别结果。

## 目录与分类

设置使用 `sourceRoot`、`targetRoot` 两个绝对路径。容器默认分别为 `/source`、`/target`，数据库路径为 `/data`。返回的 `roots` 表示启动时授权的目录范围；设置只能选择该范围内已存在的目录。源与目标不能相同，源也不能位于目标目录内部。

分类规则的 `name` 是目标目录下的相对子目录，允许多级路径，例如 `电影/华语电影`、`电视剧/国产剧`。规则按顺序匹配，首个命中规则的路径填入 `{category}`；最终文件路径统一以 `targetRoot` 为根。禁止绝对路径、`.`、`..`、空路径段和非法目录名。

扫描及预览均校验来源必须位于 `sourceRoot` 内且不在 `targetRoot` 内。显式提交目标文件也会被拒绝。`scope=roots` 只供设置目录浏览，不会放宽扫描或预览权限。

单部与多部整理共用工作区和批次入口。独立扫描、单作品预览执行入口已移除。记录列表及批次状态不包含 `ops` 和 `mappings`；单条详情按需返回完整记录。刮削记录对应 `GET /scrape/runs`、`GET /scrape/runs/{id}` 及 `POST /scrape/runs/{id}/cancel|retry|undo`，与整理记录按用途隔离。


## 记录筛选与数据维护

`GET /runs` 和 `GET /scrape/runs` 均接受 `search`（作品名称包含搜索，去掉两端空格，ASCII 大小写不敏感，`%/_` 按字面匹配）、`state`（省略或 `queued/running/completed/failed/cancelled/interrupted/undone`）及 `page`。无效状态报错。总数与当前页使用同一数据库快照，不加载操作内容或映射快照；按稳定创建序号倒序，重试不会改变历史位置。

数据维护请求为 `{before,traces,cache,cacheBefore}`。`traces/cache` 至少一项为 true；清理历史时 `before` 必须为不晚于当前时间的 RFC3339 时间，只清理在此时刻之前结束的 completed/failed/cancelled/interrupted 轨迹。运行项、无结束时间及边界时刻记录保留。`cacheBefore` 为 Unix 秒，0 或省略时由服务端确定当前时间；只删除过期时间小于等于该值的缓存，不接受未来时间。前端提交预览返回的 `request`，固定已复核的截止时间；预览不锁定数据，实际数量以清理响应为准。

清理需要登录和同源写请求。先预览、再确认；不改变工作区结果及其 revision，不删除整理／刮削 Run、当前规则或媒体文件，不执行 VACUUM。历史清理后，工作区历史链接可能提示已清理。任何子表或缓存删除失败都会整体回滚，不报告成功；数据库已释放页面可供后续写入复用，文件体积不保证立即缩小。


## 选择性重新识别与刮削选项

`POST /recognitions/{id}/reidentify` 的 `paths` 必须来自当前工作区且无重复；整个请求在提交前检查 revision、路径授权及文件可用性，任一项无效则整体拒绝。提交事务只更新指定条目及工作区摘要，成功后才注册后台任务。`reidentifying` 标记随条目持久化；工作区变为 running，取消／关闭后保留未完成标记，通过现有 retry 入口继续，不重扫其他来源。存在未完成重新识别时不允许再提交新的重跑或补充来源。

成功重跑写入新的原始 Metadata 和 traceId，保留旧轨迹。未人工编辑的文件按当前提取结果重新匹配；Edited 文件保留作品快照、分组、季集及整理选择，重新校验无效时取消选择。未指定文件及已提交文件不修改。匹配失败仍保留重跑标记，工作区报告 failed，可继续重试。

刮削 `content` 必填且只能为 `all/nfo/images`；`onlyMissing` 为布尔值，默认 false。返回的 `ScrapePlan` 新增 `content/onlyMissing/skipped`，其中 `artifacts` 只含本次计划生成的文件，`skipped` 为按路径排序的同名已有普通文件。不选 NFO 时不生成 NFO 内容，不选图片时不查询作品图片候选。已有目录或符号链接仍作为冲突，不作为可保留产物。

“只补缺失”不验证已有产物内容，也不自动登记这些文件的归属。预览后目标被外部新增时沿用严格提交／执行校验，拒绝覆盖；由其他本系统任务生成的相同且未变化产物仍可复用。没有任何操作的预览不能执行；不包含实际操作归属的视频不进入 Run 映射快照，既有刮削状态不改变。重试、撤销和重启直接使用已持久化操作快照，不重新扩展刮削内容。完成状态仅针对本次选择的产物。
