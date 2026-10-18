# STRM Generator Go

OpenList 目录到本地 STRM 的 Go 生成器，配套 [jellyfin-strm-probe](https://github.com/kentinya/jellyfin-strm-probe)。开发依据为原样保留的 [V4.3 方案](<STRM Generator Go.md>)。

**Slice 1「安全的单轮生成闭环」与 Slice 2「安全目录镜像与清理恢复」均已 PASS / CLOSED；Slice 3「定时与部署」当前为 FIX REQUIRED。** Slice 2 的 Base 为 `930782035615512be811a7b8bcc8416050f0c12f`，Implementation Head 为 `d0a0f6db5447f38467c67612ed7173d77e6bea27`；Task 2.1／2.2 均有独立 B PASS，A 已审查完整范围并接受三轮缺失、源根／目标 Get 重核、本地安全清理、移动收敛、dry-run 和正常失败恢复。Slice 3 的 Base 为 `ca9f91e10a0fc6c28679b5827f453e632eb9854a`，Implementation Head 为 `2131b54bcec1cd43769d239a30456b4a8607f4aa`；Task 3.1／3.2 均有独立 B PASS，但 A 最终审查发现 sync 零间隔兼容回归和 Compose 渲染暴露 token 两项 P1 阻断，未关闭 Slice。分层证据和后续动作见 [当前合同](SLICE.md)。

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

用户最终选择的清理规则：源根未成功读取为目录时保留且不增加缺失次数；源根可读时，对需判断缺失的已拥有路径 Get 未读到即计缺失，不再区分同样响应背后的原因。默认三次完整轮及删除前同规则重核，保留归属／输出边界检查；所有列表 refresh:false。此前刷新列表证明及请求预算方案已撤回，详见[需求](docs/requirements.md#简化删除判定2026-10-18-用户最终决定)。

## 构建与运行

```bash
mkdir -p artifacts/slice-1/bin
bash scripts/go-local build -trimpath -o artifacts/slice-1/bin/strmgen-amd64 ./cmd/strmgen
export OPENLIST_TOKEN=...   # 来自 openlist.token_env 指定变量
./artifacts/slice-1/bin/strmgen-amd64 sync --config config.yaml --dry-run
./artifacts/slice-1/bin/strmgen-amd64 sync --config config.yaml
./artifacts/slice-1/bin/strmgen-amd64 serve --config config.yaml
```

成功退出 0；配置／参数／不支持命令退出 2；API、文件、数据库、冲突或忙碌退出 1；`serve` 被 SIGINT／SIGTERM 取消时以非零 cancelled 结果退出。`sync` 与 `sync --dry-run` 保持一次性行为，按 URL／SHA-1／Size 规则更新已拥有文件；`cleanup.enabled` 默认 false，显式 true 且 `sync.mirror` 开启时启用完整轮缺失确认与本地安全删除。`serve` 首轮立即执行，之后每 `sync.interval_minutes` 分钟（缺省 60，必须为正整数）运行一轮；手动 `sync` 与 `serve` 共用同一固定锁，锁忙的 tick 只报告 `busy-skip` 并在后续 tick 继续；单轮 API／文件／数据库失败记录脱敏摘要后继续下一 tick；信号到达时当前轮按既有取消边界结束、不提交不完整轮并释放锁。调度摘要写 stdout，脱敏诊断写 stderr，均不含 token、签名 URL 或私有完整路径。Linux amd64／arm64 产品镜像与 Compose 运行包已由 Task 3.2 实现，详见 [部署与运行包](docs/deployment.md)；真实服务兼容、播放、性能与 ARM64 实机尚未验收。

隔离 CLI 验收（自建动态端口假服务；故障二进制仅供测试）：

```bash
bash scripts/go-local build -tags strmgen_testhooks -trimpath \
  -o artifacts/slice-1/bin/strmgen-testhooks-amd64 ./cmd/strmgen
RUN_DIR="$(mktemp -d artifacts/validation-evidence/slice-1-local-XXXXXX)"
python3 scripts/run_isolated_sync_integration.py --binary artifacts/slice-1/bin/strmgen-amd64 \
  --fault-binary artifacts/slice-1/bin/strmgen-testhooks-amd64 \
  --scenario all --output-dir "$RUN_DIR"
```

`serve` 调度隔离验收（发布二进制验证首轮与取消，testhooks 二进制缩短调度间隔）：

```bash
SERVE_RUN_DIR="$(mktemp -d artifacts/validation-evidence/serve-local-XXXXXX)"
python3 scripts/run_serve_integration.py --binary artifacts/slice-1/bin/strmgen-amd64 \
  --fault-binary artifacts/slice-1/bin/strmgen-testhooks-amd64 \
  --output-dir "$SERVE_RUN_DIR"
```

部署镜像与容器验收（本机 amd64 运行、arm64 OCI manifest 核对）：

```bash
docker buildx build --platform linux/amd64 --load --tag strmgen:local .
docker buildx build --platform linux/arm64 \
  --output type=oci,dest=artifacts/task-3.2/oci/strmgen-arm64.oci.tar .
CONTAINER_RUN_DIR="$(mktemp -d artifacts/validation-evidence/container-local-XXXXXX)"
python3 scripts/run_container_integration.py --image strmgen:local \
  --arm64-oci artifacts/task-3.2/oci/strmgen-arm64.oci.tar \
  --output-dir "$CONTAINER_RUN_DIR"
```

镜像构建、Compose 挂载、非特权运行、token 环境注入、正常停止／重启及双容器共享
锁竞争细节见 [部署与运行包](docs/deployment.md)。

启动无需 Docker 的 OpenList 假服务：

```bash
python3 scripts/mock_openlist.py
```

仅监听 `127.0.0.1:15244`，公开假 token 为 `strmgen-fixture-only`，夹具查询方法与可选 Compose 见环境文档。目标配置见 [config.example.yaml](config.example.yaml)，其中 `serve` 与 `sync.interval_minutes` 由当前 Slice 3 实现。真实服务兼容、媒体播放及性能验收均未运行。

## 安全操作与恢复

- **正常写入边界：** 生成器写输出／暂存／DB，Jellyfin／插件等消费者只读；所有生成器共享同一锁对象，跨容器私有 /tmp 的同名路径不能协调共享输出。避免其他程序同时改写，可选专用身份或只读挂载；不要求新增 UID／权限管理能力或消费者 OS 拒写实验。不承诺无损抵御任意绕过锁的外部修改。已拥有可再生成 STRM 的可识别偏差与正常中断，按既有比较规则和恢复流程在后续成功同步中收敛，不保证下一次必然成功。详见 [需求 SW-1..SW-6](docs/requirements.md)。

- 相对配置中的 `sources[].output` 与 `database.path` 按启动工作目录解析，不按配置文件所在目录解析。输出路径的现有祖先必须是实际目录而非符号链接；真实同步需要对输出目录有目录遍历、建目录和创建文件的权限，读取既有 STRM 还需要读权限。数据库父目录需可访问并可写；token 必须存在于配置指定的环境变量中。
- 单轮锁使用固定运行时位置 `/tmp/strmgen-locks/sync.lock`，目录需由当前有效 UID 所有且不可被组／其他用户写入。所有受支持 CLI 同步共享该 `flock`；环境变量、TMPDIR、数据库或源配置不能另选锁空间。锁文件会保留以避免 inode 替换；进程正常退出、SIGTERM 或 SIGKILL 后内核释放锁，不要在运行时手动删除锁文件。
- SQLite 是文件归属的证据。数据库丢失后，已有 STRM 不会自动认领；同步会把没有数据库归属记录的目标作为冲突保留。数据库损坏／不可用会在媒体输出前失败。恢复时优先还原可信备份；若无法还原，先由管理员独立核验 STRM 与业务归属，不能用扫描结果自动重建所有权。
- 人工维护建议先停止生成器并备份 DB、输出及残留；普通重试或已拥有 STRM 的可识别内容变化，不一律要求换新输出／新状态。真正未归属文件和无法安全解释的冲突先保留排查；不盲删暂存或伪造归属，数据库丢失后仍没有自动再认领功能。不要在生产路径试验恢复。
- 发布或归属记录中断可能留下已发布但未登记的 STRM；再次运行会将其视为未归属冲突，不覆盖、不登记、不删除。失败前已成功发布并登记的文件可以保留；失败源不会提交完整轮次。
- 清理默认关闭；`cleanup.enabled: true` 且 `sync.mirror: true` 时，源根可读取的完整成功轮对未观察到的已拥有精确路径用 Get 判定，连续三次不同完整轮达到阈值后才具备删除资格，删除前按相同源根／目标 Get 规则重核。只删除可靠归属、精确映射、输出边界内且仍为 regular STRM 的自身目标；目录、符号链接、特殊文件、未知暂存与失配记录保留。dry-run 只列符合已提交阈值的候选，不写文件、库、计数或轮次。清理本地失败会停止该源后续删除并保留已发生操作的报告；重启后按当前源根／Get 重核与可靠本地事实收敛。
- 已拥有更新使用确定性 `.update.tmp` 暂存；正常中断留下的唯一可识别暂存可在真实同步中恢复，再按当前远端信息比较。dry-run 只报告残留冲突；无归属、多个残留或无法解释的路径／类型冲突仍保留。
- 正常失败会尽力移除同目录 `.strmgen-*.tmp`；SIGKILL、系统崩溃或突然断电可能遗留该临时文件。dry-run 读取 WAL 数据库时会在数据库父目录创建并正常清理私有 `.strmgen-ro-*` 快照目录，异常中断也可能残留。SQLite 的 `-wal`／`-shm`／`-journal` 文件由 SQLite 管理；确认相关进程退出前不得手工删除。对遗留项目先核实进程、目录和文件归属，不对未知文件做批量清理。

更多安装、权限、夹具和证据说明见 [开发环境与交接](docs/development-environment.md)。
