# STRM Generator Go

OpenList 目录到本地 STRM 的 Go 生成器，配套 [jellyfin-strm-probe](https://github.com/kentinya/jellyfin-strm-probe)。开发依据为原样保留的 [V4.3 方案](<STRM Generator Go.md>)。

**当前是开发准备基线：环境工具、隔离 API 夹具、流程与合同已建立；同步程序尚未实现，所有产品 Slice 均为 PLANNED。**

从 [AGENTS.md](AGENTS.md)、[开发流程](docs/development-workflow.md)、[环境说明](docs/development-environment.md) 开始，再读 [需求](docs/requirements.md)、[架构](docs/architecture.md)、[路线图](docs/roadmap.md)、[当前 Slice](SLICE.md) 和 [Task](TASK.md)。[进度](docs/progress.md) 区分已验证环境与未运行产品验收。

仓库根目录执行（Python 3.11+、Git；SDK 安装需要 HTTPS）：

```bash
python3 scripts/development_environment.py install
python3 scripts/development_environment.py doctor
bash scripts/go-local version
python3 scripts/development_environment.py smoke
python3 scripts/check_governance.py
python3 -m unittest discover -s tests -v
```

无 HEAD 时使用 `--bootstrap`；建立首个提交后使用常规治理检查。`smoke` 只验证编译器、标准库、Linux amd64／arm64 交叉编译，不生成产品二进制。module 暂为本地名 `strm-generator-go`，发布仓库确定后显式修改；没有配置配套项目为本仓库远程。

启动无需 Docker 的 OpenList 假服务：

```bash
python3 scripts/mock_openlist.py
```

仅监听 `127.0.0.1:15244`，公开假 token 为 `strmgen-fixture-only`，夹具查询方法与可选 Compose 见环境文档。目标配置见 [config.example.yaml](config.example.yaml)，其中 `sync`／`serve` 仍是计划中的命令。真实服务兼容、媒体播放及性能验收均未运行。
