# 路径元数据提取

范围为 **绝对路径 → 规则链 → Metadata + Step[]**。核心不读取文件、不探测媒体、不访问 TMDB。网络匹配和元数据后续使用仍由原有调用方负责。

## 代码结构

- `backend/internal/recognition/model.go`：Rule、Metadata、Step 和输入来源、目标枚举。
- `input.go`：路径文本规范化、边界、输入来源和 ID 标签保护。
- `engine.go`：配置编译、顺序执行、类型转换、覆盖决策、统一步骤记录。
- `scripts/*.js`：7 个字段的默认脚本（每个字段一个）与文件名清理脚本的完整源码。
- `catalog.go`、`defaults.go`：可编辑的脚本模板与默认链；不包含提取算法。
- `script.go`：JavaScript 执行器、通用正则能力、日志、行号、取消与执行限制。
- `value.go`：字段校验和转换；未知值不猜测类型。
- `backend/internal/recognition/service.go`、`history.go`、`trial.go`：当前链管理、轨迹、试跑；SQL 在 `infra/sqlite/`，接口在 `app/http_extraction.go`。
- `frontend/src/features/extraction/`：规则编辑器、试跑、元数据与步骤展示、历史页。
- `tests/backend/recognition/`、`tests/frontend/recognition/`：本功能测试，包括从原解析器迁来的路径回归样例；整个项目统一使用根目录 [tests/](../tests/README.md)。

`scanner` 只保留文件遍历及媒体探测，不再维护解析入口和规则。接口、工作区存储、命名与匹配统一消费 Metadata，不保留另一套解析 DTO。正式应用调用当前配置的执行器，每个文件开始时获取一次实例；保存规则不改变正在执行的实例。

## Rule

每条规则只有一个 `target`。公共字段为 `id`、`name`、`description?`、`target`、`source`、`order`、`enabled`、`kind`、`config`、`overwrite`、`examples`。`target` 取 `tmdbId/title/mediaType/year/season/episodes/releaseTag`。`source` 取 `filename/cleanFilename/parentDirectory/directories/relativePath/absolutePath`。文件名来源不含扩展名，路径来源保留扩展名。目录来源近到远、不包含边界目录；相对路径以边界为根。绝对路径来源显式提供规范化的完整路径。

正则使用 Go RE2 语法，`config={pattern,capture}` 指定命名捕获组，或使用 `{pattern,value}` 指定 JSON 固定值，二者互斥。脚本规则使用 `kind:"script"`、`config:{code,params?}`；`code` 是完整 JavaScript 源码，`params` 是可选 JSON 对象。规则目录提供完整脚本模板，添加后可以直接编辑代码和目标，不会调用隐藏处理器。

```json
{"id":"season-sxe","name":"SxxExx 季号","target":"season","source":"filename","order":10,"enabled":true,"kind":"regex","config":{"pattern":"(?i)S(?P<value>\\d{1,3})E\\d{1,4}","capture":"value"},"overwrite":false,"examples":[{"path":"/Show.S02E03.mkv","expected":2}]}
```

规则按非负 `order` 升序执行，拒绝重复 ID、顺序、无效正则、捕获组和参数。全部验证成功后保存即替换当前链，不引入规则版本或任务绑定。空链有效，输出全未知。

## 默认规则

默认链每个字段一条规则，共 7 条，来源均为 `relativePath`：脚本自己读取 `ctx.filename` 与 `ctx.directories`，在一个脚本内按优先级尝试多种写法，先命中优先。某种写法出错时记录原因并继续下一种，全部未命中且有错误才报告错误。

| 顺序 | 规则 | 尝试顺序 |
| --- | --- | --- |
| 10 | TMDB ID | 文件名 → 目录（近到远）中的 `tmdb-603` |
| 20 | 季号 | SxxExx → 1x03 → 独立 Sxx → 季目录（Season 2、第二季、Specials） |
| 30 | 集数 | SxxExx（含合并集）→ 1x03 → 纯集号文件名（需父目录）→ 尾部集号 |
| 40 | 媒体类型 | 季号或集数已识别即为 `tv`，不推断 `movie` |
| 50 | 片名 | 文件名；只剩集号时改用目录名，跳过季目录 |
| 60 | 年份 | 文件名 → 目录中片名之后的四位年份；参数 `min/max` |
| 70 | 发布标签 | 文件名末尾 `-GROUP` |

媒体类型排在季集之后，直接读取 `ctx.metadata.season / episodes`，不重复匹配文件名。用户仍可添加自定义规则，按 `order` 插入链中任意位置；默认规则也可编辑、停用或删除。

## 脚本规则

脚本使用 JavaScript，由纯 Go 的 [goja](https://github.com/dop251/goja) 执行，定义一个 `extract(ctx)` 函数。脚本不能加载模块、访问文件、网络、系统命令或环境变量。每条规则仍只返回一个目标字段；不能在脚本中修改公共元数据。

```js
function extract(ctx) {
  trace("开始检查季号", ctx.text);
  const match = reSearch("(?i)S(\\d{1,3})E\\d{1,4}", ctx.text);
  if (!match) {
    trace("未找到 SxxExx 标记");
    return null;
  }
  const season = Number(match.groups[1]);
  trace("提取季号", season);
  return {value: season, matchedText: match.text};
}
```

- `ctx.text`：本次来源文本；逐级目录规则每次得到一个目录名，仍按近到远尝试。
- `ctx.filename`：不含扩展名的文件名；`ctx.directories`：边界内的目录名数组；`ctx.hasParent`：是否有边界内父目录。
- `ctx.source`：当前来源枚举；`ctx.metadata`：当前已提取元数据的只读对象；`ctx.params`：规则参数的只读对象。参数使用 JSON 数据。
- 返回标量或集数列表，未命中返回 `null`；也可返回 `{value,matchedText?,captures?}`，其中 `captures` 只能是字符串字典。其他结果字段会被拒绝。

自动追踪的辅助函数为 `reSearch(pattern,text)`、`reFindAll(pattern,text)`、`reSub(pattern,text,replacement)` 与 `trace(message,data?)`；另可用 JS 字符串、数组、数字及原生 RegExp 操作。这些辅助函数使用 Go RE2 语法；原生 JS RegExp 使用 JavaScript 语法。命中对象包含 `text`、`groups`（含索引 0 的完整匹配）、`captures`（命名捕获）、`start/end`（UTF-16 索引，与 JS slice 一致）；未命中为 `null`，`reFindAll` 返回列表。替换文本中的捕获引用使用 `$1` 或 `${name}`。

每次调用自动记录脚本输入、返回值或错误；正则调用自动记录输入、表达式和结果；`trace` 与 `print` 记录说明或中间值。每条事件携带 `phase`（`input/source/extract`）、`line`、`column`、`function`、`operation`、`message` 和可选 `data`，存放在 `Step.attempts[].trace`。界面点击事件行号可定位到当时脚本的对应行。普通 JS 变量赋值不会自动记录，需要时用 `trace` 展示。

`cleanFilename` 来源也改用脚本，代码位于 `config.sourceCode`：省略时填充默认清理脚本，并写入步骤快照；编辑器可查看或修改。清理脚本同样定义 `extract(ctx)`，但必须返回字符串，日志阶段为 `source`。普通脚本规则中的清理和判断全部直接保存在自己的源码中。

每次执行使用独立的解释器全局状态。单个脚本限制为 32 KiB、250 ms 执行时限与 256 层调用栈；每个脚本至多保留 128 条过程事件（超出后增加一条省略提示），单次正则匹配最多 512 项；返回值最多 12 层、单个集合 1024 项、字符串 32 KiB，总遍历预算 64 KiB。超限或运行出错会保留此前日志、记录错误并继续后续规则；请求取消会中止运行并跳过剩余规则。输入中的 ID 标签在季号、集数脚本的文本、文件名及目录上下文中统一屏蔽，此时 `ctx.metadata["tmdbId"]` 也为 `null`，保护过程本身留下记录。

开发阶段只维护当前 script/regex 规则。不存在 builtin 迁移或处理器字段；配置不符合当前模型直接报错。规则源码变化通过编辑并保存当前链生效，不自动覆盖用户配置。

### JavaScript 运行边界

- 必须声明同步的 `function extract(ctx)`，单个普通参数，不接受 async 或 generator 入口。未命中返回 null（undefined 同样表示未命中），不接受 Promise 结果。
- 每次调用创建独立 goja Runtime；编译结果可复用，运行时、全局变量和原型不共享。传入数据经 JSON 复制后递归冻结，严格模式下修改输入会报错。
- 不提供 Node.js、npm、require、fetch、文件、数据库或环境变量访问。输入与输出之间不暴露宿主 Go 对象。来源清理也使用同一 JS 执行器。
- 原生 RegExp 调用和普通变量赋值不会自动产生子步骤；需要解释时调用 trace。默认规则使用自动追踪的 RE2 辅助函数，保留查询表达式、输入和匹配结果。
- 250 ms 中断用于 JS 执行，不是进程级内存隔离，也无法保证抢占所有原生操作。规则是管理员维护的代码；不应将此环境作为任意不可信代码的隔离服务。复杂正则优先使用 RE2 辅助函数。
- 仅维护 JavaScript 源码，不保留旧语言引擎、转换器或数据迁移。已有当前规则含旧语言源码时加载会报错，需显式更新开发配置；应用不会自动覆盖或清空已有数据。

## Metadata 与执行语义

```json
{"tmdbId":null,"title":"Show","mediaType":"tv","year":null,"season":2,"episodes":[3],"releaseTag":null}
```

单值未知为 `null`；集数未知为 `[]`。季号 `0` 明确表示特别篇，媒体类型只接受 `movie/tv/null`。电影和剧集共用 `title`。捕获的数字会转换成整数，季号支持中文数字，集数支持单集、数组以及 `1-3,5` 的范围形式；非法值和重复集数被拒绝。

先命中优先；后续规则仍匹配并记录提取值，但仅在未知或配置 `overwrite:true` 时写入。错误不删除已有结果，继续后续规则。禁用、无来源、未命中、冲突、覆盖均留下记录。TMDB 标签中的数字不能被任何规则作为季号或集数；默认链先屏蔽 ID 标签再解释季集。因此 Matrix 的 `tmdb-603` 只产生 ID，不产生第 603 集。

每个 Step 包含当时的完整 Rule 快照（`ruleConfig`）、目标、实际尝试文本与捕获、命中文本、提取值、前后值、是否采用、原因、时间和耗时。状态为 `disabled/not_matched/matched/skipped/error`。`matched` 不等于已采用，需看 `applied`。单条规则出错记录 `error`；取消后剩余规则记录 `skipped`，保留已经完成的字段和步骤。

## 使用与存储

自动识别页提供“识别规则”“识别调试”“执行历史”入口，三者是独立页面。**规则列表**按字段排列整条链，执行顺序按字段固定：TMDB ID → 季号 → 集数 → 媒体类型 → 片名 → 年份 → 发布标签，后面的字段可读取前面字段的结果；排序只在同一字段内用上移下移调整，`order` 按此自动重新编号，载入的链若不是这个顺序会整理后提示保存。列表页负责启停、删除、新增入口、恢复默认、运行全部样例和保存，有未保存修改时离开需确认。**规则编辑页**一次只编辑一条规则：代码在前，基本／高级／测试样例默认折叠；无效 JSON 保留并提示，修正前禁止保存。试跑与保存都使用当前已生效的链并替换这条草稿，试跑结果即保存后的效果；执行步骤行号可定位到代码，离开前有未保存修改需确认。**识别调试页**用已生效的链解析一条路径，显示每个字段的结果、决定它的规则和完整执行过程，点击规则名跳到编辑页。试跑可以输入不存在的路径，边界默认使用源目录（路径在其内时），否则 `/`。试跑不写配置、工作区、整理记录、预览或历史轨迹。

正式提取以文件为单位写入 `extraction_traces`，每完成一条规则即写入 `extraction_steps`。文件行只返回 `traceId`；详情按需读取，历史每页 20 条。已结束轨迹可以手动清理，关联步骤一并删除；不自动清理。轨迹被清理后，工作区文件显示无执行轨迹。服务重启将仍在运行的提取标记为 `interrupted`，保留完成步骤；重试使用当前规则链。历史规则快照不依赖当前链。

接口见 [API](API.md)。执行 `make check` 验证类型检查、后端竞态测试与前端构建。

## 规则回归样本

默认脚本的整链回归样本集中在 `tests/backend/recognition/testdata/`，包含 97 条通过路径与 3 条待修复路径。每条显式提供解析边界、全部元数据字段、预期错误及判断理由；覆盖原 31 条路径以及误识别、目录优先级、发布标签和非法季集等情况。待修复样本不改写为当前错误输出，只容许已声明的差异，修复后需转入通过样本库。维护与运行方式见 [样本库说明](../tests/backend/recognition/testdata/README.md)。

这份样本库暂不进入产品配置或页面。页面规则 `examples` 仍验证对应规则的提取值，而非最终完整元数据；运行中整条链任一规则 `error` 都使该样例失败，即使预期与实际同为 `null`。接口通过 `error` 返回规则 ID 和原因，界面在失败样例中展示，禁止把执行故障当成正常未命中。

集号下限为 0：E00、中文第零集及包含 0 的合并集都是有效提取值，集号范围为 0–9999，最多 1000 集且不可重复；未知集数仍用空数组表示。匹配与整理预览还会校验该季对应集号是否在 TMDB 中存在，E0 不绕过此校验。
