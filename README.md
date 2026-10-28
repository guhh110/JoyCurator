# JoyCurator

JoyCurator 是面向个人媒体收藏的文件整理工具，使用 TMDB 元数据，为电影和剧集生成规范目录、文件名及配套刮削文件。

**添加文件 → 选择作品 → 修正季集 → 预览路径 → 确认整理。**

## 功能

- 手动搜索并绑定 TMDB 作品，支持电影、剧集、特别篇和合并集。
- 自定义命名模板与分类规则，执行前预览目标路径、空间需求和文件冲突。
- 支持复制、移动和硬链接，不覆盖已有视频或自动删除重复版本。
- 生成 NFO、下载海报和背景图，关联整理已有同名字幕。
- 查看逐文件操作记录，支持取消、失败重试、中断后重试和整批撤销。
- 桌面双栏对照、手机分区操作，固定导航，页面与列表独立滚动。
- 采用单管理员账号管理。

项目不包含媒体播放、转码、字幕下载或下载器联动。

## 单文件部署

支持以下平台：

| 系统 | 架构 | 文件名后缀 |
| --- | --- | --- |
| Linux | x86_64 | `linux_amd64` |
| Linux | ARM64 | `linux_arm64` |
| macOS | Intel | `darwin_amd64` |
| macOS | Apple Silicon | `darwin_arm64` |

从 [Releases](https://github.com/guhh110/JoyCurator/releases) 下载对应程序，放入可写的安装目录并重命名为 `joycurator`。发布程序已包含前端页面、Go 服务和 SQLite 驱动，无需安装 Go、Node.js 或数据库服务。Windows 暂不支持。

```sh
chmod +x joycurator
./joycurator
```

打开 **http://localhost:8081**，按页面提示创建管理员。首次启动会在程序所在目录创建 `local/data/` 保存数据库，以及 `local/source/`、`local/target/` 作为默认源目录和目标目录。

可通过 `./joycurator --version` 查看版本，使用 Release 附带的 `SHA256SUMS` 核对下载文件。macOS 如提示程序无法打开，可在系统设置中检查并允许运行。

ffprobe 为可选依赖。需要识别分辨率、编码和时长时，请安装 FFmpeg，并确保 `ffprobe` 在 PATH 中；未安装时仍可进行文件整理。

## Docker Compose 部署

镜像支持 Linux amd64 和 arm64，包含 ffprobe、CA 证书和时区数据：

```text
ghcr.io/guhh110/joycurator:latest
```

发布包和镜像随版本发布生成，可用版本以 [Releases](https://github.com/guhh110/JoyCurator/releases) 为准。

将 [deploy/compose.yaml](deploy/compose.yaml) 保存为部署目录中的 `compose.yaml`，在该目录执行：

```sh
mkdir -p data source target
export JOYCURATOR_UID=$(id -u)
export JOYCURATOR_GID=$(id -g)
docker compose pull
docker compose up -d
```

访问 `http://服务器IP:8081`；若修改了 `WEB_PORT`，使用对应的端口。默认目录映射如下：

| 宿主机目录 | 容器内目录 | 用途 |
| --- | --- | --- |
| `${DATA_DIR:-./data}` | `/data` | 数据库、配置和操作记录 |
| `${SOURCE_DIR:-./source}` | `/source` | 待整理的视频及字幕 |
| `${TARGET_DIR:-./target}` | `/target` | 整理结果及刮削文件 |

容器内端口固定为 `8081`，通过 `WEB_PORT` 调整对外端口。可以在部署目录的 `.env` 中设置宿主机路径和端口，例如：

```dotenv
WEB_PORT=8081
DATA_DIR=./data
SOURCE_DIR=/srv/media/incoming
TARGET_DIR=/srv/media/library
```

应用内使用容器路径：源目录 `/source`、目标目录 `/target`，数据库保存在 `/data`。电影与剧集没有独立挂载，其子目录由分类规则生成。

需要硬链接时，将 [deploy/compose.shared.yaml](deploy/compose.shared.yaml) 保存为部署目录中的 `compose.shared.yaml`，使用共同父目录挂载：

```sh
mkdir -p data media/source media/target
docker compose -f compose.shared.yaml up -d
```

此示例将 `${MEDIA_DIR:-./media}` 一次挂载到 `/media`，程序的源、目标目录分别为 `/media/source` 和 `/media/target`。它们仍需位于同一文件系统；两个独立挂载即使来自同一磁盘，也可能无法互相创建硬链接。

容器默认使用 UID/GID `1000:1000`；可通过 `JOYCURATOR_UID`、`JOYCURATOR_GID` 调整为媒体目录所有者的身份，挂载目录需允许该用户读写。首次启动时会创建默认目录；已有挂载目录的权限需由宿主机管理。需要长期保留 UID/GID 设置时，将它们写入部署目录的 `.env`。需要固定版本时，在该文件中设置 `JOYCURATOR_VERSION` 为已发布的镜像版本号。

## 首次配置与使用

1. 创建管理员账号，密码长度为 12–72 字节。
2. 在设置中填写 TMDB **API Read Access Token**，按需设置网络代理。
3. 配置已授权范围内的源目录和目标目录，并检查分类规则。
4. 添加文件或文件夹，选择对应作品，检查并修正季集信息。
5. 生成预览，处理冲突后确认执行；进度和操作结果可在整理记录中查看。

浏览器中的目录均位于运行 JoyCurator 的设备上；从手机访问时，同样操作服务端文件。

源目录和目标目录需预先存在，启动时会创建默认目录。作品、分类和季目录会在整理时自动创建。默认使用复制，也支持移动和硬链接。

分类规则按顺序匹配，首个命中的规则生成目标目录下的相对路径。默认一级分类为“电影”和“电视剧”，例如：

```text
目标目录/
├── 电影/
│   ├── 华语电影/作品目录/
│   └── 动画电影/作品目录/
└── 电视剧/
    ├── 儿童/作品目录/Season 01/
    ├── 国产剧/作品目录/Season 01/
    ├── 国漫/作品目录/Season 01/
    ├── 欧美剧/作品目录/Season 01/
    ├── 日番/作品目录/Season 01/
    ├── 日韩剧/作品目录/Season 01/
    └── 其他剧集/作品目录/Season 01/
```

电视剧默认按以下顺序匹配：

| 分类 | 条件 |
| --- | --- |
| 儿童 | 类型包含儿童 / Kids |
| 国漫 | 动画类型，且地区为 CN、HK、TW 或 SG |
| 日番 | 动画类型，且地区为 JP |
| 国产剧 | 地区为 CN、HK、TW 或 SG |
| 欧美剧 | 地区为 US、GB、FR、DE、IT、ES、CA、AU 或 NZ |
| 日韩剧 | 地区为 JP 或 KR |
| 其他剧集 | 未命中以上规则 |

分类子目录可自定义，使用 `/` 创建层级；不允许绝对路径、空路径段或 `..`。扫描和预览只能使用源目录内的文件，目标目录会被排除。源和目标不能相同，源目录也不能位于目标目录内部。

目录扫描会递归导入视频，跳过隐藏文件、符号链接和已配置的目标目录。每次最多导入 200 个路径、2000 个视频，单次整理最多 1000 个视频。

剧集整理当前季中能够完整对应的文件；特别篇季号为 `0`，合并集支持 `1-3`、`1,3,5` 等写法。电影整理当前工作区中的全部来源文件，文件名筛选仅影响显示。预览有效期为 15 分钟，配置变更、文件变化或服务重启后需重新生成。

## 环境变量

以下为独立程序的默认配置，Compose 示例已覆盖容器内的监听地址和目录。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `JOYCURATOR_ADDR` | `127.0.0.1:8081` | HTTP 监听地址 |
| `JOYCURATOR_DATA_DIR` | 程序目录下的 `local/data` | SQLite 数据目录 |
| `JOYCURATOR_SOURCE_DIR` | 程序目录下的 `local/source` | 源目录及其授权范围 |
| `JOYCURATOR_TARGET_DIR` | 程序目录下的 `local/target` | 目标目录及其授权范围 |
| `JOYCURATOR_SECURE_COOKIE` | `false` | 通过 HTTPS 访问时设置为 `true` |

需要局域网访问时，可将监听地址设置为 `0.0.0.0:8081`。使用 HTTPS 反向代理时，代理应保留原始 Host，并启用安全 Cookie。长期运行可使用 systemd 等进程管理工具。

## 数据与隐私

账号、设置、TMDB 缓存和整理记录保存在 SQLite 中。管理员密码使用 bcrypt 哈希，会话凭据使用 SHA256 哈希；TMDB Token 不返回前端，也不写入浏览器存储。

**TMDB Token 在数据库中以明文保存，因此数据库及备份应作为敏感文件保管。** 浏览器会保存当前工作区及文件路径，退出登录时清空。

程序没有遥测或媒体上传接口。TMDB 查询会发送搜索词及作品、季集 ID，海报和背景图通过 TMDB 图片服务获取。

升级前停止服务并备份数据目录。单文件部署替换程序即可；Compose 部署可执行：

```sh
docker compose pull
docker compose up -d
```

保留数据库和媒体挂载目录。撤销操作会检查目标文件是否被修改或替换，以及原始路径是否被占用，遇到冲突会停止对应操作。

更改源目录、目标目录或容器挂载时，已有媒体不会自动搬迁。历史任务仍引用原始绝对路径，重试或撤销前需确保这些路径仍存在且处于授权范围内。

## 源码开发与构建

技术栈为 Go、SQLite、Vue 3 和 TypeScript。

开发依赖：Go 1.24+、Node.js 22.19+、pnpm 11.28.5、Python 3。ffprobe 可选。

```sh
git clone https://github.com/guhh110/JoyCurator.git
cd JoyCurator
make install
```

分别在两个终端启动后端和前端：

```sh
make backend
```

```sh
make frontend
```

开发页面位于 **http://localhost:5175**，API 默认监听 `127.0.0.1:8081`，由 Vite 代理。

```sh
make check                         # 检查、测试与前端构建
make build                         # 构建内嵌前端的单文件程序
./local/bin/joycurator
make release VERSION=v0.0.1       # 交叉编译四个平台的程序与校验文件
```

单平台程序输出到 `local/bin/`，多平台产物输出到 `local/releases/`。使用 `make backend` 开发时，数据目录位于项目的 `local/` 下。编译后的程序遵循单文件部署规则，在程序所在目录下创建 `local/`，也可通过环境变量指定目录。

接口说明见 [docs/API.md](docs/API.md)，自动构建与发布流程见 [GitHub Actions](.github/workflows/release.yml)。
