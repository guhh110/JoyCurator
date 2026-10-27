# JoyCurator

面向个人媒体收藏的本地整理工具。Go + SQLite 后端，Vue 3 前端，元数据来自 TMDB。

**添加文件 → 选择作品 → 修正季集 → 预览路径 → 确认整理。**

正式源码位于根目录的 `frontend/` 与 `backend/`。原 `rewrite/` 已提升为正式版本；旧版源码、数据库及测试媒体归档于本机的 `local/archive/legacy-20261027/`，不参与构建、不提交 Git。旧数据库不直接导入新版本。

## 本地开发

需要 Go 1.24+、Node.js 22.19+、pnpm 11.28.5。ffprobe 可选；缺失时仍能整理，分辨率、编码等探测字段为空。

首次安装：

```sh
cd frontend
pnpm install --frozen-lockfile
```

分别在两个终端启动：

```sh
cd backend
go run ./cmd/joycurator
```

```sh
cd frontend
pnpm dev
```

访问 **http://localhost:5175**。API 默认监听 `127.0.0.1:8081`，由 Vite 同源代理；生产构建也可直接通过 Go 服务访问。开发不需要 Docker。

首次进入创建管理员，密码为 12–72 字节；在设置页填写 TMDB **API Read Access Token**。凭据只保存在后端，留空保留已保存的 Token，勾选清除才会删除。

也可以在项目根目录运行 `make install`、`make backend`、`make frontend`；保留 `make dev-backend`、`make dev-frontend` 别名。Makefile 将 Go 缓存与临时文件集中到项目 `local/`。

## 测试媒体与配置

默认授权 `local/media/`，首次启动会创建：

```text
local/media/
├── inbox/     # 在这里手动创建文件夹、放入视频及字幕
├── movies/    # 电影目标目录
└── tv/        # 剧集目标目录
```

在工作台添加文件或文件夹，目录会递归展开为视频，跳过隐藏文件、符号链接及目标目录。每次最多导入 200 项、2000 个视频，单次整理最多 1000 个视频。

| 环境变量 | 默认值 | 用途 |
| --- | --- | --- |
| `JOYCURATOR_ADDR` | `127.0.0.1:8081` | HTTP 地址 |
| `JOYCURATOR_DATA_DIR` | 项目 `local/data` | SQLite 数据目录 |
| `JOYCURATOR_MEDIA_ROOTS` | 项目 `local/media` | 授权根目录，macOS/Linux 用冒号分隔 |
| `JOYCURATOR_SECURE_COOKIE` | `false` | HTTPS 部署启用安全 Cookie |

目标根目录需预先存在，下层分类、作品和季目录自动创建。默认复制，也可选择移动或硬链接。剧集整理当前季可完整对上的文件；电影整理当前工作区全部来源，文件名筛选只影响显示。特别篇季号为 `0`；合并集支持 `1-3`、`1,3,5`。

## 构建与检查

```sh
make check
make build
./local/bin/joycurator
```

`make check` 运行 Go vet、竞态测试、前端单元测试、类型检查和生产构建。前端产物在 `local/build/frontend/`，Go 二进制在 `local/bin/`；运行构建产物时需一并保留前端产物。二进制在此布局下会定位到同级 `local/`，不依赖终端当前目录。

## 功能与数据

- 整理工作台、整理记录、设置三个视图；桌面双列对照，手机分区切换。
- 手动搜索 TMDB、逐文件季集修正、命名模板与按顺序匹配的分类规则。
- 后端生成并保存预览，执行仅提交预览 ID。预览有效期 15 分钟；设置变更、文件变化或重启后需重新生成。
- 复制、移动、硬链接，带文件校验、防覆盖及授权目录限制；不自动删除重复版本。
- NFO、海报、背景图及同名字幕全部纳入预览；共享产物去重，已有产物保留，视频和字幕重名时报冲突。
- 单任务串行运行，轮询进度，支持取消、失败重试、中断恢复和整批撤销。目标被替换、修改或原路径被占用时停止对应操作。
- SQLite 保存账号、设置、TMDB 缓存和整理记录；操作状态先持久化，再改动文件。浏览器保存当前工作区，刷新保留，退出清空。

运行数据、日志、媒体、构建产物和缓存统一放在可见的 `local/` 下，不提交 Git。若直接运行 Go 命令，也可先在项目根目录设置缓存路径：

```sh
export GOMODCACHE="$PWD/local/cache/go-mod"
export GOCACHE="$PWD/local/cache/go-build"
export GOTMPDIR="$PWD/local/tmp"
export TMPDIR="$PWD/local/tmp"
```

接口见 [docs/API.md](docs/API.md)。文件安全、解析、TMDB、命名和 NFO 模块沿用原有测试；业务编排与 UI 已重新实现。
