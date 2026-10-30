# 路径识别样本库

这是默认规则源码的回归样本，不是用户配置。运行测试不会读取媒体文件、调用 TMDB、保存规则或清理数据。

## 当前覆盖

- `paths.json`：97 条通过样本，包括原 `paths_test.go` 的全部 31 条路径；逐项比较全部 7 个元数据字段及执行错误。
- `known-gaps.json`：3 条待修复样本，保留期望结果和问题说明，不把当前错误输出写成正确答案，也不计入识别通过数。
- 覆盖电影、剧集、动漫、中文数字、TMDB ID、防画质误识别、年份、目录上下文、解析边界、字段冲突、发布标签、非法输入值。
- 这些是人工设计的命名样本，不代表真实媒体库识别准确率；后续应持续加入脱敏后的真实失败路径。

## 样本格式

每条记录必须包含：

```json
{
  "id": "example-sxe",
  "category": "tv",
  "path": "/source/Show.S02E03.mkv",
  "root": "/source",
  "reason": "明确季集标记同时提供季号与集数。",
  "expected": {
    "tmdbId": null,
    "title": "Show",
    "mediaType": "tv",
    "year": null,
    "season": 2,
    "episodes": [3],
    "releaseTag": null
  },
  "errors": {}
}
```

`expected` 必须完整，不省略未知字段；单值未知为 `null`，集数未知为 `[]`。不要把季号 0 或集号 0 当成未知。`root` 必须显式填写，即使是 `/`；相同路径在不同边界下可以有不同期望。

`errors` 是「规则 ID → 预期错误信息片段」字典。正常样本使用 `{}`，任何意外执行错误都会失败。非法集号等样本应同时断言剩余有效元数据和明确错误，不能把报错与正常未命中混为一谈。

## 待修复问题

1. `release-case`：发布组 `Group_1` 末尾数字被识别为第 1 集，媒体类型随之误判为 `tv`。
2. `invalid-descending-range`：`S01E05-E03` 倒序范围被尾部集号兜底识别成第 3 集。
3. `invalid-too-many-episodes`：`S01E01-E1001` 超大范围被尾部集号兜底识别成第 1001 集。

待修复记录用 `sample` 保存同格式样本，额外提供 `issue` 和 `differences`（允许仍不符合期望的字段名，执行错误用 `errors`）。测试不跳过它们：出现额外差异会失败；全部修好后也会提示失败，要求将该样本移入 `paths.json`、删除对应待修复记录。使用 `-v` 可看到当前差异。

## 当前策略与含糊命名

- 默认不从“没有季集”推断电影类型。
- 文件名优先于目录；目录近到远；多个年份取首个有片名前缀的合法候选。
- `Film.2024.1080p-123` 的纯数字后缀按当前尾部集号策略识别为 123，但不视为发布组。若要改变这项策略，先评估数字片名、动漫尾部集号等相关样本，不自动从当前输出生成新期望。
- 无法确定含义的真实路径，先记录并明确预期，不为了增加数量擅自猜测。

## 运行与维护

从仓库根目录运行：

```sh
# 通过样本：全字段与错误检查
python3 tests/run_backend.py test -run '^TestRecognitionPathCorpus$' -v ./internal/recognition

# 单类或单个样本，便于改规则后快速验证
python3 tests/run_backend.py test -run 'TestRecognitionPathCorpus/ids/' -v ./internal/recognition
python3 tests/run_backend.py test -run 'TestRecognitionKnownGaps/invalid-descending-range' -v ./internal/recognition

# 全部识别测试，包括样本接口、运行时、取消和竞态
python3 tests/run_backend.py test -race ./internal/recognition
```

流程：先添加有理由的期望 → 运行确认差异 → 修改规则 → 运行完整样本库 → 将修好的待修复样本移入通过库。不要用执行器批量生成期望，也不要静默跳过失败样本。

本轮未接入页面批量样本库。页面「运行全部样例」仍运行当前规则各自的 `examples`（比较该规则提取值），不是此处的整链全元数据样本库；但样例运行中整条链任一规则报错现在都算失败，并显示具体规则和原因。
