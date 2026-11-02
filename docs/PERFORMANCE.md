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
