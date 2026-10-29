# 测试目录

项目测试文件统一保存在这里，按运行环境和业务包分类：

```text
tests/
├── backend/          # Go 测试，子目录对应 backend/internal/ 中的包
│   ├── app/
│   ├── metadata/
│   ├── organizer/
│   ├── recognition/
│   ├── rules/
│   ├── scanner/
│   ├── tmdb/
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
