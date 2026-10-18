# STRM Generator Go

OpenList 目录到本地 STRM 的 Go 生成器，配套 [jellyfin-strm-probe](https://github.com/kentinya/jellyfin-strm-probe)。开发依据为原样保留的 [V4.3 方案](<STRM Generator Go.md>)。

**Slice 1「安全的单轮生成闭环」已 ACTIVE，Base 固定为开发准备提交 `f90f9560e55b07fd2d6a20e8bac43741c39176c4`。[Task 1.1](TASK.md) 有可执行实现；B 对前一候选返回 FIX REQUIRED。当前状态、真实 Head 与证据以 `TASK.md` 为准；实现不等于 B 接受或 Slice PASS。**

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
RUN_DIR="$(mktemp -d artifacts/validation-evidence/task-1.1-local-XXXXXX)"
python3 scripts/run_isolated_sync_integration.py --binary artifacts/task-1.1/bin/strmgen-amd64 \
  --scenario all --output-dir "$RUN_DIR"
```

启动无需 Docker 的 OpenList 假服务：

```bash
python3 scripts/mock_openlist.py
```

仅监听 `127.0.0.1:15244`，公开假 token 为 `strmgen-fixture-only`，夹具查询方法与可选 Compose 见环境文档。目标配置见 [config.example.yaml](config.example.yaml)，其中 `serve`／定时仍是计划中的命令。真实服务兼容、媒体播放及性能验收均未运行。

## 安全操作与恢复

- 相对配置中的 `sources[].output` 与 `database.path` 按启动工作目录解析，不按配置文件所在目录解析。输出路径的现有祖先必须是实际目录而非符号链接；真实同步需要对输出目录有目录遍历、建目录和创建文件的权限，读取既有 STRM 还需要读权限。数据库父目录需可访问并可写；token 必须存在于配置指定的环境变量中。
- 单轮锁使用固定运行时位置 `/tmp/strmgen-locks/sync.lock`，目录需由当前有效 UID 所有且不可被组／其他用户写入。所有受支持 CLI 同步共享该 `flock`；环境变量、TMPDIR、数据库或源配置不能另选锁空间。锁文件会保留以避免 inode 替换；进程正常退出、SIGTERM 或 SIGKILL 后内核释放锁，不要在运行时手动删除锁文件。
- SQLite 是文件归属的证据。数据库丢失后，已有 STRM 不会自动认领；同步会把没有数据库归属记录的目标作为冲突保留。数据库损坏／不可用会在媒体输出前失败。恢复时优先还原可信备份；若无法还原，先由管理员独立核验 STRM 与业务归属，不能用扫描结果自动重建所有权。
- 发布或归属记录中断可能留下已发布但未登记的 STRM；再次运行会将其视为未归属冲突，不覆盖、不登记、不删除。失败前已成功发布并登记的文件可以保留；失败源不会提交完整轮次。当前 Task 不实现删除，未知文件不得作为“清理残留”盲目删除。
- 正常失败会尽力移除同目录 `.strmgen-*.tmp`；SIGKILL、系统崩溃或突然断电可能遗留该临时文件。dry-run 读取 WAL 数据库时会在数据库父目录创建并正常清理私有 `.strmgen-ro-*` 快照目录，异常中断也可能残留。SQLite 的 `-wal`／`-shm`／`-journal` 文件由 SQLite 管理；确认相关进程退出前不得手工删除。对遗留项目先核实进程、目录和文件归属，不对未知文件做批量清理。

更多安装、权限、夹具和证据说明见 [开发环境与交接](docs/development-environment.md)。
