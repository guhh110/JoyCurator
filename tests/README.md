# 测试目录

项目测试文件统一保存在这里，按运行环境和业务包分类：

```text
tests/
├── backend/          # Go 测试，子目录对应 backend/internal/ 中的包
│   ├── app/
│   ├── infra/        # sqlite、tmdb、scanner
│   ├── matching/
│   ├── metadata/
│   ├── organizer/
│   ├── recognition/
│   ├── rules/
│   ├── trace/
│   └── webui/
├── frontend/         # Vitest 测试
│   ├── app/          # 路由、导航、查询缓存与登录隔离
│   └── recognition/
├── scripts/          # Python 脚本测试
└── run_backend.py    # Go 测试入口
```

在仓库根目录执行：

```sh
make check           # 全部检查、测试与前端构建
make test-backend    # Go 竞态测试
make test-frontend   # 前端测试
make test-scripts    # Python 脚本测试
```

Go 测试保留原包名，通过 Go 自带的 `-overlay` 在单次检查期间映射回对应包，因此仍能验证包内私有方法，不需要向业务代码增加测试接口，也不会复制测试文件到源码目录。映射文件临时存放于 `local/tmp/`，退出时清理。直接运行 `go test` 不会加载集中存放的测试，请使用上述入口。

可以传递标准 Go 参数来筛选包、用例或构建标签，例如：

```sh
python3 tests/run_backend.py test -run TestOrder ./internal/recognition
python3 tests/run_backend.py test -tags production ./internal/webui ./internal/app
```

前端继续使用 `pnpm -C frontend test`，Vitest 已配置为只从 `tests/frontend/` 发现测试。Python 测试可用 `python3 -m unittest discover -s tests/scripts -p '*_test.py'` 执行。

新增测试放入相应子目录，沿用 `*_test.go`、`*.test.ts`、`*_test.py` 命名；不要在业务源码旁新增测试文件。

默认识别规则的分类路径样本库位于 [backend/recognition/testdata/](backend/recognition/testdata/README.md)：目前 97 条通过样本、3 条明确记录的待修复样本。每条都包含路径、解析边界、完整元数据期望及预期错误；不会自动覆盖用户已保存的规则。执行 `python3 tests/run_backend.py test -run 'TestRecognition(PathCorpus|KnownGaps|CorpusFormat)' -v ./internal/recognition` 查看样本级结果。

架构约束按 recognition、matching、organizer 三模块检查。预览缓存、识别事务回滚、摘要读取、连接与限速共享及前端空闲停止轮询均有集中回归测试。

队列测试集中于 `backend/organizer/queue_test.go`、`coordinator_test.go`、`backend/infra/sqlite/queue_test.go` 与 `backend/app/queue_test.go`、`scrape_queue_test.go`。可控 worker 覆盖多批 FIFO、失败继续、等待／运行取消、批次隔离、队尾重试、双通道重叠、第一集登记后立即刮削、撤销资源等待及重启续跑；SQLite 触发器验证入队、完成与视频映射登记的事务回滚。工作区测试覆盖稀疏修正、锁定文件拒绝、并行导入及执行不增加编辑 revision；HTTP 测试覆盖队列和分页批次的新响应格式。前端 runtime 与工作区测试覆盖双通道发现、进度缓存失效、停止空闲轮询和后台刷新时的编辑草稿。

固定性能样本：2000 文件规则提取和历史写入、1000 文件工作区预览、1000 条历史（每条 100 项操作）的列表和批次摘要查询。运行：

```sh
python3 tests/run_backend.py test -run '^$' -bench 'BenchmarkRecognition2000|BenchmarkDirectoryDecisions2000|BenchmarkWorkspace2000|BenchmarkRunHistory|BenchmarkPreview1000' -benchtime=1x ./internal/infra/sqlite ./internal/app ./internal/recognition
```

队列工作区另有 `BenchmarkFileExecutionProjection1000`，测量 1000 文件任务状态投影。基准不访问公网，不操作已有用户数据，设置及样本创建不计入计时。结果与测量边界见 [性能记录](../docs/PERFORMANCE.md)。

存储关系化回归位于 `backend/infra/sqlite/relational_test.go`：覆盖独立作品／规则快照、NULL 与特别篇、有序集合、递归撤销与 BLOB、空标注列表的流水线重启、严格动态 JSON、旧格式／损坏结构拒绝且不改写、跨记录恢复回滚，以及触发器保护未变化条目。`BenchmarkRunCheckpoint`（10／1000 操作）和 `BenchmarkWorkspaceCheckpoint`（10／2000 条目，更新固定十条）测量增量写入成本。


增补回归：`backend/infra/fsops/replace_test.go` 验证 Linux／macOS 独占覆盖及目标、暂存、hold 冲突；`backend/organizer/undo_identity_test.go` 验证等长外部替换、检查点期间变化和 NFO 覆盖撤销；`backend/infra/sqlite/trace_batch_test.go` 覆盖跨 SQL 批次顺序、独立规则快照及末尾事件失败回滚。`maintenance_test.go` 与 `app/maintenance_routes_test.go` 覆盖清理预览、固定截止、终态筛选、外键子表与缓存共同回滚、revision 不变及登录／同源约束。`run_filter_test.go` 覆盖筛选后分页、用途隔离及字面搜索；前端对应测试位于 `frontend/app/maintenance.test.ts`、`records.test.ts` 和 `recordPresentation.test.ts`。


选择性重跑测试位于 `backend/app/reidentify_test.go`，覆盖工作区 revision、原子提交失败、未选条目禁止写入、人工特别篇映射与作品快照、旧历史独立、取消／匹配失败重试及关闭重启后范围保留。`backend/app/scrape_options_test.go` 覆盖内容过滤、仅 NFO 不查询作品图片、只补缺失保留外部文件、未受影响映射不变、全跳过拒绝空任务及撤销。前端范围选择与选项请求／过期响应测试分别在 `frontend/recognition/reidentify.test.ts` 和 `frontend/scrape/queries.test.ts`。
