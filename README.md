# STRM Generator Go

OpenList 目录到本地 STRM 的 Go 生成器，配套 [jellyfin-strm-probe](https://github.com/kentinya/jellyfin-strm-probe)。开发依据为原样保留的 [V4.3 方案](<STRM Generator Go.md>)。

**Slice 1「安全的单轮生成闭环」已 ACTIVE，Base 固定为开发准备提交 `f90f9560e55b07fd2d6a20e8bac43741c39176c4`；[Task 1.1](TASK.md)「首次生成、归属登记与安全重复运行」已实现为 Developer 检查点，待 B 审查。**

从 [AGENTS.md](AGENTS.md)、[开发流程](docs/development-workflow.md)、[环境说明](docs/development-environment.md) 开始，再读 [需求](docs/requirements.md)、[架构](docs/architecture.md)、[路线图](docs/roadmap.md)、[当前 Slice](SLICE.md) 和 [Task](TASK.md)。[进度](docs/progress.md) 区分已验证环境与未运行产品验收。

仓库根目录执行（Python 3.11+、Git；SDK 与 C 工具链安装需要 HTTPS）：

```bash
python3 scripts/development_environment.py install
python3 scripts/development_environment.py install-cc
python3 scripts/development_environment.py doctor
bash scripts/go-local version
python3 scripts/development_environment.py smoke
python3 scripts/check_governance.py
python3 -m unittest discover -s tests -v
```

无 HEAD 时使用 `--bootstrap`；建立首个提交后使用常规治理检查。`smoke` 只验证编译器、标准库、Linux amd64／arm64 交叉编译，不生成产品二进制。module 暂为本地名 `strm-generator-go`，发布仓库确定后显式修改；没有配置配套项目为本仓库远程。

## 构建与运行

```bash
bash scripts/go-local build -trimpath -o artifacts/task-1.1/bin/strmgen-amd64 ./cmd/strmgen
export OPENLIST_TOKEN=...   # 来自 openlist.token_env 指定变量
./artifacts/task-1.1/bin/strmgen-amd64 sync --config config.yaml --dry-run
./artifacts/task-1.1/bin/strmgen-amd64 sync --config config.yaml
```

成功退出 0；配置／参数／不支持命令退出 2；API、文件、数据库、冲突或忙碌退出 1。当前只支持 `sync` 与 `sync --dry-run`；`serve` 与清理尚未实现，`cleanup.enabled: true` 会被明确拒绝。已拥有文件需要更新时保留原文件并非零退出（内容替换由后续 Task 承接）。

隔离端到端验收（自建动态端口假服务，不接触共享夹具或生产）：

```bash
python3 scripts/run_isolated_sync_integration.py --binary artifacts/task-1.1/bin/strmgen-amd64 \
  --scenario all --output-dir artifacts/validation-evidence/task-1.1-final
```

启动无需 Docker 的 OpenList 假服务：

```bash
python3 scripts/mock_openlist.py
```

仅监听 `127.0.0.1:15244`，公开假 token 为 `strmgen-fixture-only`，夹具查询方法与可选 Compose 见环境文档。目标配置见 [config.example.yaml](config.example.yaml)，其中 `serve`／定时仍是计划中的命令。真实服务兼容、媒体播放及性能验收均未运行。
