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
| `GET /runs?page=` | 整理批次，固定每页 20 条，返回 `{items,total,page}` |
| `GET /runs/{id}` | 批次详情、逐文件状态和错误 |
| `POST /runs/{id}/cancel` | 取消运行中的任务 |
| `POST /runs/{id}/retry` | 重试失败、取消或中断的整理 |
| `POST /runs/{id}/undo` | 整批撤销；撤销中断后再次调用以继续 |

扫描文件结构含 `path`、`size`、`mtime`、`season`、`episodes` 和 `probe`。`mtime` 为纳秒时间戳字符串，避免 JavaScript 数值精度损失；未知季号为 `-1`，预览前由用户选择的季补全。

预览返回 `id`、`rows`、`required`、`issues`。仅无阻塞问题的预览可执行；需要跳过冲突文件时，去掉对应文件后重新请求预览。前端不提交文件操作指令。

任务状态为 `running`、`completed`、`failed`、`cancelled`、`interrupted`、`undone`。前端轮询详情获取进度；服务重启后运行中的批次标为中断，操作的临时路径、文件身份与校验值仍持久化，便于重试或撤销。

每次只运行一个整理或撤销任务。浏览器保存工作区；服务器内存只保存短期预览，SQLite 保存管理员、会话、配置、TMDB 缓存和整理批次。

## 目录与分类

设置使用 `sourceRoot`、`targetRoot` 两个绝对路径。容器默认分别为 `/source`、`/target`，数据库路径为 `/data`。返回的 `roots` 表示启动时授权的目录范围；设置只能选择该范围内已存在的目录。源与目标不能相同，源也不能位于目标目录内部。

分类规则的 `name` 是目标目录下的相对子目录，允许多级路径，例如 `电影/华语电影`、`电视剧/国产剧`。规则按顺序匹配，首个命中规则的路径填入 `{category}`；最终文件路径统一以 `targetRoot` 为根。禁止绝对路径、`.`、`..`、空路径段和非法目录名。

扫描及预览均校验来源必须位于 `sourceRoot` 内且不在 `targetRoot` 内。显式提交目标文件也会被拒绝。`scope=roots` 只供设置目录浏览，不会放宽扫描或预览权限。
