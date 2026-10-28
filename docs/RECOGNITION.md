# 路径元数据提取

范围为 **绝对路径 → 规则链 → Metadata + Step[]**。核心不读取文件、不探测媒体、不访问 TMDB。网络匹配和元数据后续使用仍由原有调用方负责。

## 代码结构

- `backend/internal/recognition/model.go`：Rule、Metadata、Step 和输入来源、目标枚举。
- `input.go`：路径文本规范化、边界、输入来源和 ID 标签保护。
- `engine.go`：配置编译、顺序执行、类型转换、覆盖决策、统一步骤记录。
- `builtin.go`、`defaults.go`：单目标内置处理器目录和默认规则链。
- `value.go`：字段校验和转换；未知值不猜测类型。
- `backend/internal/app/extraction*.go`：当前链原子替换、SQLite 轨迹持久化、接口与旧结果适配。
- `frontend/src/features/extraction/`：规则编辑器、试跑、元数据与步骤展示、历史页。
- `tests/backend/recognition/`、`tests/frontend/recognition/`：本功能测试，包括从原解析器迁来的路径回归样例；整个项目统一使用根目录 [tests/](../tests/README.md)。

`scanner` 只保留文件遍历及媒体探测，不再维护解析入口和规则。旧数据结构只在应用层适配。正式应用调用当前配置的执行器，每个文件开始时获取一次实例；保存规则不改变正在执行的实例。

## Rule

每条规则只有一个 `target`。公共字段为 `id`、`name`、`description?`、`target`、`source`、`order`、`enabled`、`kind`、`config`、`overwrite`、`examples`。`target` 取 `tmdbId/title/mediaType/year/season/episodes/releaseTag`。`source` 取 `filename/cleanFilename/parentDirectory/directories/relativePath/absolutePath`。文件名来源不含扩展名，路径来源保留扩展名。目录来源近到远、不包含边界目录；相对路径以边界为根。绝对路径来源显式提供规范化的完整路径。

正则使用 Go RE2 语法，`config={pattern,capture}` 指定命名捕获组，或使用 `{pattern,value}` 指定 JSON 固定值，二者互斥。内置规则使用 `{handler,params?}`，处理器目录返回支持目标、来源和参数默认值；处理器不匹配目标时拒绝保存。年份处理器参数 `min/max` 默认为 1900/2099。

```json
{"id":"season-sxe","name":"SxxExx 季号","target":"season","source":"filename","order":10,"enabled":true,"kind":"regex","config":{"pattern":"(?i)S(?P<value>\\d{1,3})E\\d{1,4}","capture":"value"},"overwrite":false,"examples":[{"path":"/Show.S02E03.mkv","expected":2}]}
```

规则按非负 `order` 升序执行，拒绝重复 ID、顺序、无效正则、捕获组和参数。全部验证成功后保存即替换当前链，不引入规则版本或任务绑定。空链有效，输出全未知。

## Metadata 与执行语义

```json
{"tmdbId":null,"title":"Show","mediaType":"tv","year":null,"season":2,"episodes":[3],"releaseTag":null}
```

单值未知为 `null`；集数未知为 `[]`。季号 `0` 明确表示特别篇，媒体类型只接受 `movie/tv/null`。电影和剧集共用 `title`。捕获的数字会转换成整数，季号支持中文数字，集数支持单集、数组以及 `1-3,5` 的范围形式；非法值和重复集数被拒绝。

先命中优先；后续规则仍匹配并记录提取值，但仅在未知或配置 `overwrite:true` 时写入。错误不删除已有结果，继续后续规则。禁用、无来源、未命中、冲突、覆盖均留下记录。TMDB 标签中的数字不能被任何规则作为季号或集数；默认链先屏蔽 ID 标签再解释季集。因此 Matrix 的 `tmdb-603` 只产生 ID，不产生第 603 集。

每个 Step 包含当时的完整 Rule 快照（`ruleConfig`）、目标、实际尝试文本与捕获、命中文本、提取值、前后值、是否采用、原因、时间和耗时。状态为 `disabled/not_matched/matched/skipped/error`。`matched` 不等于已采用，需看 `applied`。单条规则出错记录 `error`；取消后剩余规则记录 `skipped`，保留已经完成的字段和步骤。

## 使用与存储

自动识别页提供“识别规则”“识别调试”“执行历史”入口。编辑器修改先应用到编辑链，再试跑或保存；支持拖动、键盘可操作的上移下移按钮、启停、删除、恢复默认和样例测试。试跑可以输入不存在的路径，边界默认使用源目录（路径在其内时），否则 `/`。试跑不写配置、工作区、整理记录、预览或历史轨迹。

正式提取以文件为单位写入 `extraction_traces`，每完成一条规则即写入 `extraction_steps`。文件行只返回 `traceId`；详情按需读取，历史每页 20 条。已结束轨迹可以手动清理，关联步骤一并删除；不自动清理。旧工作区未补写轨迹，显示“无规则执行轨迹”。服务重启将仍在运行的提取标记为 `interrupted`，保留完成步骤；重试使用当前规则链。历史规则快照不依赖当前链。

接口见 [API](API.md)。执行 `make check` 验证类型检查、后端竞态测试与前端构建。
