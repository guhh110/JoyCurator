# JoyCurator

面向个人媒体收藏的本地整理工具。Go + SQLite 后端，Vue 3 前端，元数据来自 TMDB。

**添加文件 → 选择作品 → 修正季集 → 预览路径 → 确认整理。**

正式源码位于根目录的 `frontend/` 与 `backend/`。原 `rewrite/` 已提升为正式版本；旧版源码、数据库及测试媒体归档于本机的 `local/archive/legacy-20261027/`，不参与构建、不提交 Git。旧数据库不直接导入新版本。

## 本地开发

需要 Go 1.24+、Node.js 22.19+、pnpm 11.28.5。构建与检查还需要 Python 3（仅标准库）。ffprobe 可选；缺失时仍能整理，分辨率、编码等探测字段为空。

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

`make check` 运行 Go vet、竞态测试、前端单元测试、类型检查和生产构建。`make build` 将前端嵌入 Go 二进制，输出 `local/bin/joycurator`，部署时只需复制这一个文件，不需要携带前端目录。开发命令仍然使用 Vite，不依赖内嵌资源。

## 单文件部署

从 [Releases](https://github.com/guhh110/JoyCurator/releases) 下载对应系统和架构的程序，例如 Linux x86_64 使用 `joycurator_v0.1.0_linux_amd64`，Apple Silicon 使用 `darwin_arm64`。当前支持 Linux/macOS 的 amd64、arm64；Windows 暂不支持。

把下载的程序放入独立、可写的安装目录，重命名为 `joycurator`：

```sh
chmod +x joycurator
./joycurator --version
./joycurator
```

访问 **http://localhost:8081** 创建管理员。程序已包含页面、API 和 SQLite 驱动，无需安装 Go、Node.js、pnpm 或 SQLite。ffprobe 可选；安装 FFmpeg 并将 ffprobe 放入 PATH 后可获取分辨率、编码等信息。

默认在**程序所在目录**创建可见的 `local/data/` 和 `local/media/{inbox,movies,tv}/`，从其他工作目录启动也使用同一位置。本仓库的 `local/bin/joycurator` 特别使用项目已有的 `local/`。更新前停止服务、备份整个 `local/data/`，替换程序后重新启动；保留媒体与数据目录。SHA256 校验值随 Release 提供，macOS 下载后如受系统安全策略拦截，需在系统设置中允许该程序运行。

需要通过局域网访问时设置 `JOYCURATOR_ADDR=0.0.0.0:8081`；可使用上表的环境变量指定数据位置和授权媒体根目录。访问 HTTPS 反向代理时设置 `JOYCURATOR_SECURE_COOKIE=true`，代理需保留原始 Host。长期运行可交给 systemd 或其他进程管理器，使用有媒体读写权限的普通用户。

## Docker 部署（服务器）

开发电脑不需要 Docker。镜像包含单文件程序、CA 证书、时区数据和 ffprobe，支持 Linux amd64/arm64。

将 [deploy/compose.yaml](deploy/compose.yaml) 放入服务器上的独立部署目录，在该目录执行：

```sh
mkdir -p data media/inbox media/movies media/tv
# 让容器使用当前用户访问挂载目录；NAS 可改成实际媒体所有者的 UID/GID。
export JOYCURATOR_UID=$(id -u)
export JOYCURATOR_GID=$(id -g)
docker compose pull
docker compose up -d
```

访问 `http://服务器IP:8081`。SQLite 和凭据持久化在宿主机 `data/`，媒体位于 `media/`。容器内授权根目录为 `/media`；添加文件、设置目标路径时使用 `/media/inbox`、`/media/movies`、`/media/tv`。已有媒体可将 `./media:/media` 改为实际绝对路径挂载；源和目标应位于同一文件系统以使用硬链接。

默认使用镜像的 `latest` 稳定版；可在部署目录的 `.env` 设置 `JOYCURATOR_VERSION=0.1.0` 固定版本。容器默认 UID/GID 为 1000，挂载目录需允许相应用户读写；不要用公开权限解决写入问题。升级时备份 `data/`，再运行 `docker compose pull && docker compose up -d`。首次发布后 GHCR 包需设为 Public 才能匿名拉取，私有包需先登录 GHCR。

## 发布版本

[GitHub Actions](.github/workflows/release.yml) 在推送 main、提交 PR 或手动触发时运行检查、构建四个平台的二进制，并分别使用原生 Linux amd64/arm64 runner 构建容器、检查启动、页面和 ffprobe；这些触发不会发布版本。

确认代码已推送后，创建版本标签触发正式发布：

```sh
git tag v0.1.0
git push origin v0.1.0
```

检查通过后，工作流自动创建 GitHub Release，上传四个单文件程序和 `SHA256SUMS`，同时发布 `ghcr.io/guhh110/joycurator:0.1.0`。稳定版本更新 `latest`，带连字符的预发布版本（例如 `v0.2.0-rc.1`）不更新它。程序版本可通过 `--version` 查看。

镜像发布使用仓库自带 `GITHUB_TOKEN` 的 `packages: write` 权限，Release 使用 `contents: write`，无需配置 Docker Hub 密钥。仓库需允许运行 Actions 和相应权限；首次发布后在 GitHub Packages 的包设置中选择 Public。构建与发布日志均在仓库 Actions 页面查看。

也可本地执行 `make release VERSION=v0.1.0` 生成同样的四个平台程序及校验文件，输出到 `local/releases/`；仅构建，不联网发布、不运行 Docker。此目标额外需要 Python 3 生成校验文件。生产构建使用 `production` 标签，必须先构建前端并复制内嵌资源，推荐通过 Makefile 完成，不要直接用普通 `go build` 制作发布包。

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

## 隐私与发布边界

- `local/` 下的 SQLite、媒体、归档、日志、截图及缓存不提交 Git，也不进入 Docker 构建上下文。数据库、备份、环境文件和私钥即使误放到源码目录，也有额外排除规则。不要用 `git add -f` 绕过这些规则。
- Docker 上下文仅开放构建必需的源码与清单；最终镜像只复制编译后的单文件程序，不复制开发电脑的数据库、媒体或配置。GitHub Actions 使用云端全新检出，不读取开发电脑的 `local/`。
- 发布上传明确限定为四个平台程序和 `SHA256SUMS`，不会将整个 `local/` 或发布目录打包上传。前端禁用 public 目录自动复制和 source map；程序只内嵌构建的首页及 assets。
- `python3 scripts/check-release.py --assets` 检查已跟踪文件及前端产物中的私有文件、SQLite 内容、常见密钥格式和个人绝对路径；发现问题只输出文件和规则，不输出敏感值。构建、检查和 CI 会自动执行此检查。规则扫描不能保证识别所有形式的秘密，新增凭据应通过后端配置输入。
- TMDB Token 不返回前端、不写入浏览器存储、不出现在上游请求错误中。管理员密码以 bcrypt 哈希保存，会话凭据在数据库中保存 SHA256 哈希。**TMDB Token 在本地数据库中以明文保存**，因此数据库及备份属于敏感文件，不要公开上传或分享。
- 程序没有遥测或媒体上传接口。联网行为用于 TMDB 元数据查询和图片下载；搜索词、作品/季集 ID 会发送给 TMDB，前端海报也直接请求 TMDB 图片域名。浏览器本地存储会保存工作区文件路径，退出登录时清空，不存储 TMDB Token 或密码。
