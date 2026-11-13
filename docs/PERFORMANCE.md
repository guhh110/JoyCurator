# 架构重构性能记录

2026-11-02，本地 Apple M4、macOS arm64，Go 1.24.2 环境。重构前使用 HEAD 的隔离源码副本，重构后使用当前源码；两者均执行相同的固定样本，连续三次 `-benchtime=1x`，下表取中位数。初始化配置、创建文件、填充历史不计入计时。

| 样本 | 重构前 | 重构后 | 耗时降低 | 累计内存分配 B/op |
| --- | ---: | ---: | ---: | ---: |
| 2000 文件规则提取及历史写入 | 2865.62 ms | 2108.72 ms | 26.4% | 2191.119 MB → 2030.895 MB |
| 2000 文件完整工作区识别 | 3886.53 ms | 3128.71 ms | 19.5% | 3567.298 MB → 3333.357 MB |
| 1000 文件整理预览 | 416.07 ms | 389.46 ms | 6.4% | 32.493 MB → 29.300 MB |
| 历史列表 + 批次摘要 | 132.73 ms | 2.24 ms | 98.3% | 6.269 MB → 0.072 MB |

## 样本与测量范围

- `BenchmarkRecognition2000`：2000 个固定路径，运行默认七条脚本并保存完整历史。没有媒体读取或 TMDB 请求。识别轨迹由每文件九次独立提交变为运行摘要一次、完成事务一次，即每批 18000 → 4000 次提交；该计数来自明确的写入边界，不含初始建库和其他业务保存。
- `BenchmarkWorkspace2000`：2000 个小型本地视频，从目录遍历、规则提取到作品匹配、季集校验和工作区保存。TMDB 使用固定 Provider，两侧均为每批三次调用（详情、季、集），公网请求为零。逐文件校验和工作区进度保存保留。
- `BenchmarkPreview1000`：1000 个已确认的剧集文件，生成视频和同名字幕预览并检查冲突及空间；不请求 TMDB、不复制视频。
- `BenchmarkRunHistory`：预置 1000 条记录，每条 100 项操作；每轮读取一页 20 条记录及一个 10 条记录的批次。重构前读取完整 JSON 再丢弃操作，重构后使用 SQL 摘要投影及用途、批次索引。

B/op 是一次完整样本的累计分配，不是峰值常驻内存。样本使用本地小文件和模拟上游，不代表 NAS、大型媒体复制或公网 TMDB 的端到端耗时；三次测量用于回归比较，不作为跨设备性能保证。

## 回归入口

```sh
python3 tests/run_backend.py test -run '^$' -bench 'BenchmarkRecognition2000|BenchmarkWorkspace2000|BenchmarkPreview1000|BenchmarkRunHistory' -benchtime=1x -count=3 ./internal/infra/sqlite ./internal/app
```

基准集中位于 `tests/backend/app/performance_test.go` 和 `tests/backend/infra/sqlite/performance_test.go`。前端另外通过请求计数与虚拟时钟验证：活动摘要统一通过 `/queue` 发现，完整详情按需请求，任务完成后停止轮询，空闲记录页没有周期请求，分页搜索切换不保留上一组结果，页面销毁会取消未完成的查询和预览。


## 队列工作区状态投影

多批队列增加 `BenchmarkFileExecutionProjection1000`：一个运行任务、1000 个视频操作和映射，读取文件关联任务及锁定／完成状态。SQL 一次物化视频状态再关联映射，避免对每个文件重复展开整组操作。固定本地样本单次测得 13.26 ms/op、647168 B/op、30053 allocs/op（相同 Apple M4／Go 环境，`-benchtime=1x`，不含建库与写入）；此数据仅记录投影成本，不构成与上一版的统计比较，也不包含媒体复制。

```sh
python3 tests/run_backend.py test -run '^$' -bench BenchmarkFileExecutionProjection1000 -benchtime=1x ./internal/infra/sqlite
```

## 全面关系化存储样本

2026-11-02，Apple M4／macOS arm64／Go 1.24.2，当前关系表实现，连续三次 `-benchtime=1x` 取中位数。样本初始化仍不计入时间，使用隔离临时库、固定 Provider 和小型本地文件，不接触已有媒体数据。上文表格是较早架构重构阶段的记录，不能代替本轮实际结果。

| 样本 | 当前耗时 | 累计分配 B/op |
| --- | ---: | ---: |
| 2000 文件提取与完整历史写入 | 7928.94 ms | 2096.571 MB |
| 2000 文件完整工作区识别 | 10335.08 ms | 3026.518 MB |
| 1000 文件预览 | 389.45 ms | 33.428 MB |
| 历史列表与批次摘要 | 0.184 ms | 55,576 B |
| 1000 文件执行状态投影 | 1.902 ms | 726,800 B |
| 单操作检查点，任务共 10 项 | 0.211 ms | 6,232 B |
| 单操作检查点，任务共 1000 项 | 0.264 ms | 6,232 B |
| 更新十条工作区条目，总共 10 条 | 0.937 ms | 31,368 B |
| 更新十条工作区条目，总共 2000 条 | 1.101 ms | 31,368 B |

单操作和十条工作区检查点的分配量不随总条目数增长；触发器回归同时保证未变化的操作／文件行不会被更新或删除。操作检查点包含摘要及指定操作，工作区检查点包含摘要及指定十条文件，两个样本都包含事务提交。文件状态投影直接关联操作与映射表，历史列表只读取摘要列。作品、规则、尝试、事件与有序集合全部拆表后，完整历史写入和端到端识别成本高于上文的 JSON 快照样本，这是当前实现的性能代价。

工作区基准保持原来的 5 ms 轮询频率、2000 个文件和每批三次 Provider 调用，仅将基准等待上限改为 60 秒；普通功能测试仍使用 5 秒上限。该轮询频率高于产品的约 1.2 秒，会把反复组装完整工作区的读取成本计入端到端样本。各业务对象的多表读取使用事务快照，写入在事务内复用固定 SQL 的预编译语句。

```sh
python3 tests/run_backend.py test -run '^$' -bench 'BenchmarkRecognition2000|BenchmarkWorkspace2000|BenchmarkRunHistory|BenchmarkPreview1000|BenchmarkFileExecutionProjection1000|BenchmarkRunCheckpoint|BenchmarkWorkspaceCheckpoint' -benchtime=1x -count=3 ./internal/infra/sqlite ./internal/app
```


## 轨迹批量插入优化

2026-11-03，同一台 Apple M4／macOS arm64／Go 1.24.2，优化前后各运行三次原有 `BenchmarkRecognition2000`（`-benchtime=1x`），取中位数，保留相同默认规则、2000 路径与完整轨迹，不访问公网或媒体内容。

| 样本 | 优化前 | 优化后 | 变化 |
| --- | ---: | ---: | ---: |
| 2000 文件提取与完整历史写入 | 7396.98 ms | 5048.67 ms | 耗时降低 31.7% |
| 累计分配 B/op | 2,102,069,096 | 2,150,579,480 | 增加 2.3% |
| 分配次数 allocs/op | 28,470,418 | 26,658,163 | 减少 6.4% |

完成事务中不再为全新步骤及其子记录执行 DELETE／UPSERT；按表分组插入步骤、规则快照、样例、输入、捕获、尝试与脚本事件，每条 SQL 最多 900 个绑定参数。父表先于子表写入，最终 Metadata 与状态仍和所有子表原子提交；末尾事件触发器失败回归验证跨批次整体回滚。批量缓冲以单文件为单位，累计分配略增，未测峰值内存；该优化不减少历史内容，不用于文件操作检查点。

同时复跑当前 `BenchmarkWorkspace2000` 三次，中位数 7720.42 ms/op、3,070,876,672 B/op，仍为每批 3 次模拟 Provider 调用、5 ms 轮询。没有同期重测工作区优化前值，因此不据较早表格计算该样本收益。高频轮询及完整工作区读取仍计入端到端成本，本机小文件样本不代表 NAS 或公网耗时。

## 复制与目录导入优化

2026-11-13，Apple M4／macOS arm64／Go 1.24.2，修改前后各执行三次 `-benchtime=1x`，下表取中位数。识别修改前的源码通过隔离 Go overlay 加载；复制基准在同一次运行中比较原有带取消检查的 32 KiB 读取路径与当前实现。所有样本只使用临时文件和数据库。

| 样本 | 修改前 | 修改后 | 耗时降低 |
| --- | ---: | ---: | ---: |
| 64 MiB 复制，包含目标文件 Sync | 187.35 ms | 169.84 ms | 9.3% |
| 2000 文件提取与完整历史写入 | 4571.32 ms | 4271.51 ms | 6.6% |
| 2000 文件完整工作区识别 | 5528.36 ms | 5220.12 ms | 5.6% |

规则提取的累计分配中位数从 5,468,431,952 降到 4,860,454,232 B/op，减少约 11.1%；完整工作区识别从 5,126,182,872 降到 4,576,101,800 B/op，减少约 10.7%。它们是累计分配，并非峰值内存。当前工作区样本仍采用 5 ms 轮询；由于新文件只生成待确认候选，修改前后均为每批一次模拟 Provider 详情查询，不包含公网耗时，与较早样本中的三次调用不同。

复制不再用通用 Reader 包装文件后交给 `io.Copy`。Linux 优先按 8 MiB 块调用 `copy_file_range`；跨文件系统、内核或文件系统不支持时，使用可复用的 1 MiB 缓冲，其余平台也使用该缓冲。每块间检查取消，写完仍执行 Sync，再发布目标；跨盘移动与跨盘撤销使用同一传输实现。同盘移动继续直接重命名。上表仅测量 macOS 缓冲路径，Linux 内核路径已做交叉编译检查，尚无本机运行或性能数据。

目录导入复用首次递归扫描得到的路径和文件属性，省去提取目录上下文时的第二次遍历；单文件导入仍读取父目录和后代视频，重新识别仍获取新快照。脚本输入冻结与入口程序改为共用预编译结果，冻结嵌套输入时减少临时函数和无用递归调用，每次规则执行仍创建独立运行时。识别结果、规则、完整历史、取消和撤销语义保持不变。上述本地小文件与 64 MiB 样本不能代表 NAS、大型视频或公网 TMDB 的实际表现。

```sh
python3 tests/run_backend.py test -run '^$' -bench 'BenchmarkRecognition2000|BenchmarkWorkspace2000|BenchmarkDurableCopy64MiB' -benchtime=1x -count=3 ./internal/infra/fsops ./internal/infra/sqlite ./internal/app
```

## 离线候选与格式 6 快照

2026-11-13，Apple M4／macOS arm64／Go 1.24.2，保留上节复制与目录扫描优化。下列模式各执行三次 `-benchtime=1x`，使用隔离临时库，不与其他测试或基准并行运行。中位数分别按各指标计算：

| 模式 | 耗时中位数 | 累计分配 B/op | 分配次数 allocs/op | 写入或输出量 |
| --- | ---: | ---: | ---: | --- |
| LightImport：本地候选生成与关系快照落库 | 3961.44 ms | 4,685,836,336 | 80,146,117 | 78,034 次 SQL 行变更；DB + WAL 净增长 45,299,944 B |
| FullDebug：完整过程与 JSON 响应编码 | 4249.20 ms | 4,783,379,328 | 82,504,571 | 0 次 SQL 行变更；响应 21,593,709 B |
| Workspace2000：扫描至工作区保存完成 | 3775.89 ms | 3,820,937,848 | 66,088,477 | Provider 调用 0 次 |

`LightImport` 与 `FullDebug` 使用完全相同的 2000 个路径：`/source/Show/Season 1/Show.S01E%04d.1080p.mkv`，序号为 0–1999，默认规则及更新目标也相同。两者调用同一执行核心，但输出目的不同：

- `LightImport` 计入本地分词、标注、目录建议、规则快照、运行摘要和关系快照的事务提交；结束后读回快照的验证不计入耗时和分配。没有逐规则 Step、Attempt、前后值或脚本事件存储。
- `FullDebug` 计入完整步骤和事件收集、结果 JSON 编码，使用计数字节的 writer，不持久化普通历史。响应中的时间及耗时字段使三次编码字节数略有差异。它不是原有“完整历史落库”模式，不能与 LightImport 当作同一个持久化基准计算收益。
- SQL 行变更来自 `total_changes()` 的差值，统计本轮逻辑写入；DB + WAL 字节为文件净增长，包含索引等存储开销，不等于累计物理磁盘 I/O，也不与调试响应字节直接比较。累计分配不是峰值常驻内存。

三次原始结果如下；LightImport 每次均为 78,034 次 SQL 行变更、45,299,944 B 净增长，FullDebug 每次均无 SQL 写入：

| 模式 | 次序 | 耗时 ms | B/op | allocs/op | 调试响应 B |
| --- | ---: | ---: | ---: | ---: | ---: |
| LightImport | 1 | 3961.441917 | 4,684,251,496 | 80,145,546 | — |
| LightImport | 2 | 3913.969916 | 4,687,787,240 | 80,147,048 | — |
| LightImport | 3 | 4154.393167 | 4,685,836,336 | 80,146,117 | — |
| FullDebug | 1 | 3730.655250 | 4,783,379,328 | 82,504,877 | 21,593,709 |
| FullDebug | 2 | 4255.355000 | 4,782,080,392 | 82,504,505 | 21,593,633 |
| FullDebug | 3 | 4249.198500 | 4,783,500,200 | 82,504,571 | 21,593,746 |
| Workspace2000 | 1 | 3791.885334 | 3,820,377,960 | 66,087,436 | — |
| Workspace2000 | 2 | 3774.851750 | 3,820,937,848 | 66,089,351 | — |
| Workspace2000 | 3 | 3775.893791 | 3,821,285,504 | 66,088,477 | — |

工作区样本仍为 2000 个本地小视频、5 ms 轮询，包含目录扫描、轻量识别、快照落库、工作区读取与最终保存。文件名为 `Show [tmdb-2].S01E%04d.mkv`，序号为 1–2000；与规则基准分别报告。三次均验证所有新文件待确认、未选择且 Provider 调用为零。上节工作区记录为 5220.12 ms、4,576,101,800 B/op、一次模拟 Provider 详情调用；本轮记录较低，但属于历史批次比较，没有重新构造同步的旧实现对照。所有本地规则仍须执行，不能把删除联网及完整历史落库理解为跳过规则处理，也不能由本机样本推断 NAS 或公网等待耗时。

```sh
python3 tests/run_backend.py test ./internal/infra/sqlite -run '^$' -bench '^BenchmarkRecognition2000$' -benchtime=1x -count=3
python3 tests/run_backend.py test ./internal/app -run '^$' -bench '^BenchmarkWorkspace2000$' -benchtime=1x -count=3
```

### 纯识别与保存成本拆分

同日追加两个独立模式，仍用上述相同的 2000 路径及默认规则，各执行三次。`PureRecognition` 只调用轻量 `Engine.Snapshot`，不扫描文件、不访问数据库、不输出完整调试步骤；`SnapshotWrite` 在计时前生成快照，只测运行摘要创建与快照／终态事务提交。这两个独立样本用于定位成本，不能要求其耗时之和精确等于另一轮 LightImport。

| 模式 | 三次耗时 ms | 耗时中位数 | 平均每文件 | 累计分配中位数 B/op |
| --- | --- | ---: | ---: | ---: |
| PureRecognition | 3297.838709 / 3323.539084 / 3320.489500 | 3320.49 ms | 1.660 ms | 4,564,753,112 |
| SnapshotWrite | 477.140459 / 472.422958 / 824.607375 | 477.14 ms | 0.239 ms | 118,836,024 |

纯识别三次分配次数分别为 79,418,971／79,422,661／79,422,239 allocs/op；快照保存为 724,754／724,738／724,717 allocs/op。分配量仍是累计值，纯识别并非同时驻留 4.56 GB 内存。

另外对 PureRecognition 单次采集 CPU 与分配剖析，耗时 3306.96 ms。分配采样中约 97% 位于 `executeScript` 调用树；其中输入 JSON 解析及递归冻结约占总采样分配的 26%，实际脚本调用约占 64%。`goja.New` 本身的直接调用树分配不足 1%，所以不能只凭运行时数量认定初始化就是主要成本。CPU 剖析也显示明显的 GC 扫描成本；多线程采样时间不能直接当作端到端耗时占比。

默认流程为每文件一次分词、五次标注，整树一次目录建议，即 2000 文件共 12,001 次独立脚本调用。各次调用重新构建输入对象，执行 JSON 解析、冻结、脚本中的数组／对象操作，以及结果转换与校验。当前纯识别的主要优化方向是降低这部分对象和输入重建成本，并保持规则、文件之间的隔离及脚本契约；删除 TMDB 调用和完整历史写入还不足以消除这些成本。1 ms／文件可以作为本固定样本的优化目标，当前实现尚未达到，不能作为任意自定义脚本和目录上下文的固定保证。

```sh
python3 tests/run_backend.py test ./internal/infra/sqlite -run '^$' -bench '^BenchmarkRecognition2000/(PureRecognition|SnapshotWrite)$' -benchtime=1x -count=3
```


## 保留脚本的执行器与默认脚本优化

2026-11-13，同一台 Apple M4／macOS arm64／Go 1.24.2。新执行器的普通执行最多 `min(4, GOMAXPROCS, 文件数)` 路；旧执行器在 GOMAXPROCS=4 时仍串行，调试始终串行。改造前使用本轮修改前保留的 recognition 源码（Go overlay），其余离线识别、关系快照与工作区代码相同。基线与新执行器均使用同一套基准计时边界，连续三次 `-benchtime=1x -count=3 -cpu=1,4`；不同测试、构建及性能样本不同时运行。首次冻结基线与最终同仪器重测均保留，以下比较采用最终重测。

- PureRecognition、SnapshotWrite、LightImport、FullDebug 完全相同的 2000 个固定路径：`/source/Show/Season 1/Show.S01E%04d.1080p.mkv`，0–1999。Engine 编译与规则准备不计入执行计时。
- 既有脚本为仓库冻结 fixture `tests/backend/recognition/testdata/execution-baseline-pipeline.json`；新执行器在该源码上直接执行，未恢复默认。新版默认为当前可编辑 JS 模板。SnapshotWrite 只计预先生成结果的 StartTrace／FinishTrace 事务；LightImport 包括识别与落库，读回验证不计时。FullDebug 包括串行完整步骤和 JSON 编码，不写数据库。
- Workspace2000 包括扫描、本地识别、快照及工作区保存，保留原 5 ms 轮询和 2000 个小视频；文件名 `Show [tmdb-2].S01E%04d.mkv`，1–2000。文件创建、建库与配置保存不计时，Provider 调用均为零。物理临时根目录由测试框架随机生成，其余路径结构相同；既有脚本通过 `JOY_BENCH_RULES=saved` 运行同一个 BenchmarkWorkspace2000，避免更长的测试名改变路径长度。另测的长测试名样本仅保留原始数据，不用于此表比较。临时目录统一为仓库 local/tmp 的规范绝对路径，避免 macOS /var 与 /private/var 别名影响文件身份校验。
- 每列单独取三次中位数。B/op、allocs/op 是整批累计分配；DB+WAL 是净增长，SQL 行数是 total_changes()，均不代表累计物理磁盘 I/O。FullDebug 响应含规则源码、时间及耗时，新模板源码长度变化会改变响应量。

### 纯识别与单线程分配

| 执行器／规则 | GOMAXPROCS | 耗时中位数 | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| 改造前／既有脚本 | 1 | 4889.89 ms | 4,529,483,352 | 79,402,556 |
| 改造前／既有脚本 | 4 | 3437.92 ms | 4,562,493,448 | 79,422,924 |
| 新执行器／既有脚本 | 1 | 3448.61 ms | 3,667,084,856 | 52,596,429 |
| 新执行器／既有脚本 | 4 | 1309.28 ms | 3,668,574,408 | 52,597,232 |
| 新执行器／新版默认 | 1 | 2866.40 ms | 2,933,790,960 | 43,443,824 |
| 新执行器／新版默认 | 4 | 1371.67 ms | 2,934,810,992 | 43,444,645 |

新执行器／既有脚本：4 worker 中位数 1.309 s，满足 ≤2 s；单线程累计分配减少 19.0%，分配次数减少 33.8%。

新执行器／新版默认：4 worker 中位数 1.372 s，满足 ≤2 s；单线程累计分配减少 35.2%，分配次数减少 45.3%。

4 worker 的 batch 耗时除以 2000 是吞吐均值，并非单文件延迟。本轮单线程三次耗时明显波动，尤其后两次常比首次更慢；保留全部值，不把单次最快值当结论，也不将 B/op 当峰值内存。新版模板进一步减少分配，但 4 worker 耗时与既有脚本的差异处于本轮样本波动范围，不能据此宣称模板额外加速。

### 保存、调试与工作区（GOMAXPROCS=4）

| 模式 | 执行器／规则 | 耗时中位数 | B/op | allocs/op | 写入／响应 |
| --- | --- | ---: | ---: | ---: | --- |
| SnapshotWrite | 改造前／既有脚本 | 639.90 ms | 118,831,000 | 724,714 | 18,729,400 B 净增长；78,034 SQL 行 |
| SnapshotWrite | 新执行器／既有脚本 | 483.50 ms | 118,830,248 | 724,715 | 18,729,400 B 净增长；78,034 SQL 行 |
| SnapshotWrite | 新执行器／新版默认 | 497.76 ms | 118,829,520 | 724,708 | 18,729,400 B 净增长；78,034 SQL 行 |
| LightImport | 改造前／既有脚本 | 4276.39 ms | 4,682,286,584 | 80,146,482 | 45,299,912 B 净增长；78,034 SQL 行 |
| LightImport | 新执行器／既有脚本 | 1882.13 ms | 3,789,922,560 | 53,321,781 | 45,299,912 B 净增长；78,034 SQL 行 |
| LightImport | 新执行器／新版默认 | 1778.92 ms | 3,055,923,272 | 44,169,047 | 45,299,912 B 净增长；78,034 SQL 行 |
| FullDebug | 改造前／既有脚本 | 4174.35 ms | 4,773,152,352 | 82,501,787 | 21,593,626 B 响应；0 SQL 行 |
| FullDebug | 新执行器／既有脚本 | 2874.68 ms | 3,914,969,712 | 55,758,462 | 21,593,546 B 响应；0 SQL 行 |
| FullDebug | 新执行器／新版默认 | 2348.80 ms | 3,179,705,600 | 46,605,058 | 21,595,368 B 响应；0 SQL 行 |
| Workspace2000 | 改造前／既有脚本 | 3719.43 ms | 3,810,566,656 | 66,080,534 | 41,983,144 B 净增长；0 Provider |
| Workspace2000 | 新执行器／既有脚本 | 2121.64 ms | 3,036,117,840 | 43,089,876 | 41,983,144 B 净增长；0 Provider |
| Workspace2000 | 新执行器／新版默认 | 1876.66 ms | 2,507,149,288 | 36,503,987 | 41,983,144 B 净增长；0 Provider |

快照行数与同路径样本的数据库净增长保持一致。SnapshotWrite 没有改动存储算法，耗时差异属于本轮三次波动；LightImport、FullDebug、Workspace2000 均无明显耗时回退。工作区的读回轮询成本仍计入，不能把它与纯识别等同。所有模式的单线程原始值也在下方保留。

### 独立进程峰值内存

每个模式、每套规则启动三个独立测试进程，均 `-cpu=4 -count=1 -benchtime=1x`。使用 Python `os.wait4(pid, 0)` 的 `ru_maxrss`（Darwin 单位为字节）取得每个子进程的最大常驻内存；它与 `/usr/bin/time -l` 使用的子进程资源统计含义相同。本环境 time 的附加 sysctl 查询受限，改用 wait4 直接读取。下表包括进程启动、规则编译、建库、样本／预计算结果准备和验证，因此是整个独立进程的峰值，不能解释为计时区间的对象存活量。1 MB = 1,000,000 B。

| 模式 | 执行器／规则 | 三次最大 RSS（B） | 中位数 |
| --- | --- | --- | ---: |
| PureRecognition | 改造前／既有脚本 | 258,441,216, 262,701,056, 267,845,632 | 262.70 MB |
| PureRecognition | 新执行器／既有脚本 | 216,956,928, 198,967,296, 220,233,728 | 216.96 MB |
| PureRecognition | 新执行器／新版默认 | 242,155,520, 234,143,744, 234,160,128 | 234.16 MB |
| SnapshotWrite | 改造前／既有脚本 | 306,233,344, 316,653,568, 282,886,144 | 306.23 MB |
| SnapshotWrite | 新执行器／既有脚本 | 233,865,216, 274,874,368, 245,743,616 | 245.74 MB |
| SnapshotWrite | 新执行器／新版默认 | 303,710,208, 264,093,696, 282,411,008 | 282.41 MB |
| LightImport | 改造前／既有脚本 | 280,051,712, 275,120,128, 285,278,208 | 280.05 MB |
| LightImport | 新执行器／既有脚本 | 243,187,712, 287,686,656, 251,543,552 | 251.54 MB |
| LightImport | 新执行器／新版默认 | 282,558,464, 290,455,552, 281,149,440 | 282.56 MB |
| FullDebug | 改造前／既有脚本 | 409,075,712, 466,206,720, 421,134,336 | 421.13 MB |
| FullDebug | 新执行器／既有脚本 | 352,960,512, 399,130,624, 398,114,816 | 398.11 MB |
| FullDebug | 新执行器／新版默认 | 419,463,168, 416,169,984, 401,866,752 | 416.17 MB |
| Workspace2000 | 改造前／既有脚本 | 227,360,768, 238,026,752, 235,732,992 | 235.73 MB |
| Workspace2000 | 新执行器／既有脚本 | 219,774,976, 229,949,440, 209,059,840 | 219.77 MB |
| Workspace2000 | 新执行器／新版默认 | 243,810,304, 235,896,832, 265,977,856 | 243.81 MB |

新版默认的 LightImport 与 Workspace2000 最大 RSS 中位数相较原基线分别增加约 0.9% 和 3.4%，三次范围存在重叠；其余模式低于原基线。累计分配减少并不保证整个进程的最大 RSS 等比例降低，此处也包含准备与回收阶段，不据三次样本确定堆行为的成因。

### 复跑与原始数据

```sh
python3 tests/run_backend.py test -run '^$' -bench '^BenchmarkRecognition(2000|SavedRules2000)$' -benchtime=1x -count=3 -cpu=1,4 ./internal/infra/sqlite
python3 tests/run_backend.py test -run '^$' -bench '^BenchmarkWorkspace2000$' -benchtime=1x -count=3 -cpu=1,4 ./internal/app
JOY_BENCH_RULES=saved python3 tests/run_backend.py test -run '^$' -bench '^BenchmarkWorkspace2000$' -benchtime=1x -count=3 -cpu=1,4 ./internal/app
```

make check 通过：脚本测试、隐私检查、Go vet、完整后端 race、201 个前端测试及构建。冻结结果对比覆盖 106 组／115 文件，比较旧源码与新默认在 1／4 worker 的完整候选、建议和引用，并与串行调试一致；另有原生冻结输入、参数与全局隔离、有界结果读取、并行诊断顺序、进度失败、分词失败与取消退出测试。HTTP、快照和格式 6 不变；已有规则源码不自动覆盖。

既有规则 fixture SHA-256：`2b0474c5e88ddbe2568f9bc5e5af07081eb7e9d84f4368f38fec964898ebd5bd`。

[完整测量输出与逐进程资源统计](performance/recognition-execution-2026-11-13.txt) 包括两次基线、全部模式与每次 wait4 结果；以下同时保留本轮最终对比的全部三次基准值。

<details>
<summary>全部基准原始值</summary>

```text

[recognition-before-final-bench.txt]
goos: darwin
goarch: arm64
pkg: joycurator/internal/infra/sqlite
cpu: Apple M4
BenchmarkRecognition2000/PureRecognition           	       1	3629467791 ns/op	4564681768 B/op	79422665 allocs/op
BenchmarkRecognition2000/PureRecognition           	       1	4889890917 ns/op	4529483352 B/op	79402556 allocs/op
BenchmarkRecognition2000/PureRecognition           	       1	5114582666 ns/op	4525237912 B/op	79401863 allocs/op
BenchmarkRecognition2000/PureRecognition-4         	       1	3437917167 ns/op	4563289768 B/op	79423324 allocs/op
BenchmarkRecognition2000/PureRecognition-4         	       1	3457216375 ns/op	4562493448 B/op	79422924 allocs/op
BenchmarkRecognition2000/PureRecognition-4         	       1	3398028584 ns/op	4561870456 B/op	79422262 allocs/op
BenchmarkRecognition2000/SnapshotWrite             	       1	 479337667 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118831728 B/op	  724719 allocs/op
BenchmarkRecognition2000/SnapshotWrite             	       1	 560166458 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118828232 B/op	  724696 allocs/op
BenchmarkRecognition2000/SnapshotWrite             	       1	 528322916 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118828016 B/op	  724693 allocs/op
BenchmarkRecognition2000/SnapshotWrite-4           	       1	 675853833 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118831144 B/op	  724718 allocs/op
BenchmarkRecognition2000/SnapshotWrite-4           	       1	 639900500 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118830264 B/op	  724714 allocs/op
BenchmarkRecognition2000/SnapshotWrite-4           	       1	 537055250 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118831000 B/op	  724713 allocs/op
BenchmarkRecognition2000/LightImport               	       1	4524964334 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	4681917112 B/op	80146115 allocs/op
BenchmarkRecognition2000/LightImport               	       1	6339903084 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	4646452712 B/op	80127139 allocs/op
BenchmarkRecognition2000/LightImport               	       1	5999883208 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	4646450568 B/op	80127087 allocs/op
BenchmarkRecognition2000/LightImport-4             	       1	4276391000 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	4681011664 B/op	80145524 allocs/op
BenchmarkRecognition2000/LightImport-4             	       1	4229559084 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	4683877904 B/op	80147448 allocs/op
BenchmarkRecognition2000/LightImport-4             	       1	4898134833 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	4682286584 B/op	80146482 allocs/op
BenchmarkRecognition2000/FullDebug                 	       1	4149824875 ns/op	  21593676 response_bytes/op	         0 sqlite_rows/op	4778613288 B/op	82502773 allocs/op
BenchmarkRecognition2000/FullDebug                 	       1	6019551875 ns/op	  21593670 response_bytes/op	         0 sqlite_rows/op	4761116632 B/op	82494845 allocs/op
BenchmarkRecognition2000/FullDebug                 	       1	5159694667 ns/op	  21593716 response_bytes/op	         0 sqlite_rows/op	4748463016 B/op	82493974 allocs/op
BenchmarkRecognition2000/FullDebug-4               	       1	4174351250 ns/op	  21593605 response_bytes/op	         0 sqlite_rows/op	4773152352 B/op	82501884 allocs/op
BenchmarkRecognition2000/FullDebug-4               	       1	4495763083 ns/op	  21593626 response_bytes/op	         0 sqlite_rows/op	4775566864 B/op	82500956 allocs/op
BenchmarkRecognition2000/FullDebug-4               	       1	4110250917 ns/op	  21593700 response_bytes/op	         0 sqlite_rows/op	4772995352 B/op	82501787 allocs/op
PASS

[recognition-after-bench.txt]
goos: darwin
goarch: arm64
pkg: joycurator/internal/infra/sqlite
cpu: Apple M4
BenchmarkRecognition2000/PureRecognition           	       1	1099161250 ns/op	2934923072 B/op	43444847 allocs/op
BenchmarkRecognition2000/PureRecognition           	       1	2948362708 ns/op	2933790960 B/op	43443824 allocs/op
BenchmarkRecognition2000/PureRecognition           	       1	2866401959 ns/op	2933788776 B/op	43443808 allocs/op
BenchmarkRecognition2000/PureRecognition-4         	       1	1371671584 ns/op	2934810992 B/op	43444645 allocs/op
BenchmarkRecognition2000/PureRecognition-4         	       1	1187678958 ns/op	2934833744 B/op	43444524 allocs/op
BenchmarkRecognition2000/PureRecognition-4         	       1	1378151541 ns/op	2934674928 B/op	43444657 allocs/op
BenchmarkRecognition2000/SnapshotWrite             	       1	 490693542 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118829696 B/op	  724706 allocs/op
BenchmarkRecognition2000/SnapshotWrite             	       1	 539437417 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118828000 B/op	  724693 allocs/op
BenchmarkRecognition2000/SnapshotWrite             	       1	 531985250 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118827536 B/op	  724682 allocs/op
BenchmarkRecognition2000/SnapshotWrite-4           	       1	 497757458 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118828944 B/op	  724689 allocs/op
BenchmarkRecognition2000/SnapshotWrite-4           	       1	 479016750 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118829520 B/op	  724708 allocs/op
BenchmarkRecognition2000/SnapshotWrite-4           	       1	 515991542 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118830400 B/op	  724710 allocs/op
BenchmarkRecognition2000/LightImport               	       1	1844408041 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	3056025840 B/op	44169203 allocs/op
BenchmarkRecognition2000/LightImport               	       1	3562129459 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	3055044256 B/op	44168250 allocs/op
BenchmarkRecognition2000/LightImport               	       1	3511807417 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	3055044360 B/op	44168252 allocs/op
BenchmarkRecognition2000/LightImport-4             	       1	1635096917 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	3055923272 B/op	44169040 allocs/op
BenchmarkRecognition2000/LightImport-4             	       1	1778923001 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	3055771128 B/op	44169244 allocs/op
BenchmarkRecognition2000/LightImport-4             	       1	1800740958 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	3055966592 B/op	44169047 allocs/op
BenchmarkRecognition2000/FullDebug                 	       1	2560194500 ns/op	  21595400 response_bytes/op	         0 sqlite_rows/op	3179770032 B/op	46606145 allocs/op
BenchmarkRecognition2000/FullDebug                 	       1	3425871666 ns/op	  21595406 response_bytes/op	         0 sqlite_rows/op	3113256344 B/op	46600455 allocs/op
BenchmarkRecognition2000/FullDebug                 	       1	3147133042 ns/op	  21595311 response_bytes/op	         0 sqlite_rows/op	3113218528 B/op	46600438 allocs/op
BenchmarkRecognition2000/FullDebug-4               	       1	2347324500 ns/op	  21595387 response_bytes/op	         0 sqlite_rows/op	3178529824 B/op	46604432 allocs/op
BenchmarkRecognition2000/FullDebug-4               	       1	2348800083 ns/op	  21595296 response_bytes/op	         0 sqlite_rows/op	3179705600 B/op	46605187 allocs/op
BenchmarkRecognition2000/FullDebug-4               	       1	2387864667 ns/op	  21595368 response_bytes/op	         0 sqlite_rows/op	3191208352 B/op	46605058 allocs/op
BenchmarkRecognitionSavedRules2000/PureRecognition           	       1	1276632500 ns/op	3668677144 B/op	52597187 allocs/op
BenchmarkRecognitionSavedRules2000/PureRecognition           	       1	3448606000 ns/op	3667084856 B/op	52596429 allocs/op
BenchmarkRecognitionSavedRules2000/PureRecognition           	       1	3480921208 ns/op	3667084096 B/op	52596411 allocs/op
BenchmarkRecognitionSavedRules2000/PureRecognition-4         	       1	1278362416 ns/op	3668574408 B/op	52597232 allocs/op
BenchmarkRecognitionSavedRules2000/PureRecognition-4         	       1	1442143375 ns/op	3668803848 B/op	52597273 allocs/op
BenchmarkRecognitionSavedRules2000/PureRecognition-4         	       1	1309275042 ns/op	3668158456 B/op	52597099 allocs/op
BenchmarkRecognitionSavedRules2000/SnapshotWrite             	       1	 480300834 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118830056 B/op	  724710 allocs/op
BenchmarkRecognitionSavedRules2000/SnapshotWrite             	       1	 535746750 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118827600 B/op	  724684 allocs/op
BenchmarkRecognitionSavedRules2000/SnapshotWrite             	       1	 544936250 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118827984 B/op	  724692 allocs/op
BenchmarkRecognitionSavedRules2000/SnapshotWrite-4           	       1	 483496459 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118829520 B/op	  724708 allocs/op
BenchmarkRecognitionSavedRules2000/SnapshotWrite-4           	       1	 486853041 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118830312 B/op	  724715 allocs/op
BenchmarkRecognitionSavedRules2000/SnapshotWrite-4           	       1	 466196459 ns/op	  18729400 db_growth_bytes/op	     78034 sqlite_rows/op	118830248 B/op	  724716 allocs/op
BenchmarkRecognitionSavedRules2000/LightImport               	       1	1823376917 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	3790106856 B/op	53321768 allocs/op
BenchmarkRecognitionSavedRules2000/LightImport               	       1	4119602874 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	3788345576 B/op	53320953 allocs/op
BenchmarkRecognitionSavedRules2000/LightImport               	       1	4123032750 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	3788346792 B/op	53320981 allocs/op
BenchmarkRecognitionSavedRules2000/LightImport-4             	       1	1814856583 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	3789922560 B/op	53321781 allocs/op
BenchmarkRecognitionSavedRules2000/LightImport-4             	       1	2034809083 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	3789576112 B/op	53321810 allocs/op
BenchmarkRecognitionSavedRules2000/LightImport-4             	       1	1882132375 ns/op	  45299912 db_growth_bytes/op	     78034 sqlite_rows/op	3789964432 B/op	53321770 allocs/op
BenchmarkRecognitionSavedRules2000/FullDebug                 	       1	2803505500 ns/op	  21593619 response_bytes/op	         0 sqlite_rows/op	3927656768 B/op	55758885 allocs/op
BenchmarkRecognitionSavedRules2000/FullDebug                 	       1	3643804500 ns/op	  21593608 response_bytes/op	         0 sqlite_rows/op	3846519232 B/op	55752551 allocs/op
BenchmarkRecognitionSavedRules2000/FullDebug                 	       1	3707520166 ns/op	  21593487 response_bytes/op	         0 sqlite_rows/op	3846520216 B/op	55752562 allocs/op
BenchmarkRecognitionSavedRules2000/FullDebug-4               	       1	2927385042 ns/op	  21593479 response_bytes/op	         0 sqlite_rows/op	3925118072 B/op	55757529 allocs/op
BenchmarkRecognitionSavedRules2000/FullDebug-4               	       1	2874678500 ns/op	  21593546 response_bytes/op	         0 sqlite_rows/op	3914319824 B/op	55758503 allocs/op
BenchmarkRecognitionSavedRules2000/FullDebug-4               	       1	2823944042 ns/op	  21593625 response_bytes/op	         0 sqlite_rows/op	3914969712 B/op	55758462 allocs/op
PASS

[workspace-before-bench.txt]
goos: darwin
goarch: arm64
pkg: joycurator/internal/app
cpu: Apple M4
BenchmarkWorkspace2000     	       1	3749383708 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	3810636952 B/op	66077203 allocs/op
BenchmarkWorkspace2000     	       1	4902705750 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	3778819840 B/op	65858097 allocs/op
BenchmarkWorkspace2000     	       1	5025533000 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	3778839872 B/op	65861509 allocs/op
BenchmarkWorkspace2000-4   	       1	3719432584 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	3810566656 B/op	66080534 allocs/op
BenchmarkWorkspace2000-4   	       1	3718009167 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	3808127504 B/op	66074096 allocs/op
BenchmarkWorkspace2000-4   	       1	4180127459 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	3811850080 B/op	66115864 allocs/op
PASS

[workspace-after-bench.txt]
goos: darwin
goarch: arm64
pkg: joycurator/internal/app
cpu: Apple M4
BenchmarkWorkspace2000               	       1	1698982625 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	2507026848 B/op	36489981 allocs/op
BenchmarkWorkspace2000               	       1	3111951416 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	2508216952 B/op	36554058 allocs/op
BenchmarkWorkspace2000               	       1	3119939750 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	2508317816 B/op	36559917 allocs/op
BenchmarkWorkspace2000-4             	       1	1893164500 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	2507304464 B/op	36503987 allocs/op
BenchmarkWorkspace2000-4             	       1	1704499208 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	2506575600 B/op	36488687 allocs/op
BenchmarkWorkspace2000-4             	       1	1876656333 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	2507149288 B/op	36505289 allocs/op
BenchmarkWorkspaceSavedRules2000     	       1	1790275958 ns/op	  42262488 db_growth_bytes/op	         0 provider_calls/batch	3036416712 B/op	43073049 allocs/op
BenchmarkWorkspaceSavedRules2000     	       1	3549621209 ns/op	  42221408 db_growth_bytes/op	         0 provider_calls/batch	3038087616 B/op	43147252 allocs/op
BenchmarkWorkspaceSavedRules2000     	       1	3526232875 ns/op	  42262488 db_growth_bytes/op	         0 provider_calls/batch	3036129256 B/op	43082329 allocs/op
BenchmarkWorkspaceSavedRules2000-4   	       1	2032803125 ns/op	  42262488 db_growth_bytes/op	         0 provider_calls/batch	3036970728 B/op	43086454 allocs/op
BenchmarkWorkspaceSavedRules2000-4   	       1	1847689583 ns/op	  42262488 db_growth_bytes/op	         0 provider_calls/batch	3036710144 B/op	43074351 allocs/op
BenchmarkWorkspaceSavedRules2000-4   	       1	1817942500 ns/op	  42262488 db_growth_bytes/op	         0 provider_calls/batch	3036846144 B/op	43074261 allocs/op
PASS

[workspace-saved-bench.txt]
goos: darwin
goarch: arm64
pkg: joycurator/internal/app
cpu: Apple M4
BenchmarkWorkspace2000     	       1	1991197667 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	3036128368 B/op	43077300 allocs/op
BenchmarkWorkspace2000     	       1	4232490667 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	3034096328 B/op	43047953 allocs/op
BenchmarkWorkspace2000     	       1	4474591333 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	3035921440 B/op	43110280 allocs/op
BenchmarkWorkspace2000-4   	       1	2121642167 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	3036117840 B/op	43088760 allocs/op
BenchmarkWorkspace2000-4   	       1	2067543000 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	3035966200 B/op	43089876 allocs/op
BenchmarkWorkspace2000-4   	       1	2267411500 ns/op	  41983144 db_growth_bytes/op	         0 provider_calls/batch	3036299296 B/op	43105199 allocs/op
PASS
```

</details>
