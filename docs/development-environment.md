# 开发环境与交接

环境文件仅写本仓库 `artifacts/dev-env/`；当前宿主为 Debian 12／Linux amd64，Python 3.11.2、Git、Docker 和 Compose 已存在。没有使用同机生产 Jellyfin 或配套项目的运行配置。Slice 1 已 PASS / CLOSED，Task 1.1／1.2 均有独立 B PASS，完整关闭合同及证据保留于 `930782035615512be811a7b8bcc8416050f0c12f:SLICE.md`。当前 Slice 2 已 ACTIVE，无活动实施 Task；Mirror 清理仍未实现，环境 smoke 不代表新 Slice 通过。状态见 [SLICE.md](../SLICE.md) 和 [TASK.md](../TASK.md)。

## 安装与检查

从仓库根目录执行：

```bash
python3 scripts/development_environment.py install
python3 scripts/development_environment.py install-cc
python3 scripts/development_environment.py doctor
bash scripts/go-local version
python3 scripts/development_environment.py smoke
python3 scripts/quality.py
```

Go 固定 1.27.2，`.go-version`、go.mod 与环境规范保持一致。首次安装校验官方归档 SHA-256，安全解压至 `artifacts/dev-env/sdk/go1.27.2` 并检查版本；已有同版本安装复用。wrapper 禁止自动切换工具链，隔离 GOPATH／GOCACHE／GOMODCACHE，默认 CGO=0，不修改用户 Go 配置。安装器在 `artifacts/dev-env` 建立独立 go.mod，避免 Go 根模块 `./...` 遍历 SDK 和 smoke 源码。

Task 1.1 固定 `gopkg.in/yaml.v3 v3.0.1` 与 `modernc.org/sqlite v1.60.1`；SQLite 驱动为纯 Go，支持 CGO=0 的 Linux amd64／arm64 构建，完整依赖记录在仓库 `go.sum`。`quality.py` 在产品包存在时运行 Go 格式检查、vet、测试、构建及 Python 回归。

`install-cc` 安装环境规范中固定版本及 SHA-256 的 Zig `cc`（`artifacts/dev-env/cc/bin/cc`），仅用于 race／CGO 构建，不是产品运行时依赖。Task 1.1 的 race 门槛命令为：

```bash
CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 -json ./...
```

离线安装可复制目标平台对应的同一官方 ZIP，再运行：

```bash
python3 scripts/development_environment.py install --sdk-archive /path/to/v0.0.1-go1.27.2.linux-amd64.zip
python3 scripts/development_environment.py install-cc --cc-archive /path/to/zig-x86_64-linux-0.14.1.tar.xz
```

`smoke` 在忽略目录生成标准库 Go 样例，检查 JSON Int64／net/http／slog，并分别构建 Linux amd64／arm64；ARM64 产物没有在本机执行。smoke 不是产品功能验证。

## CLI、路径和权限

构建本机二进制并先预览：

```bash
mkdir -p artifacts/slice-1/bin
bash scripts/go-local build -trimpath -o artifacts/slice-1/bin/strmgen-amd64 ./cmd/strmgen
export OPENLIST_TOKEN=...   # 必须与配置 openlist.token_env 一致
./artifacts/slice-1/bin/strmgen-amd64 sync --config config.yaml --dry-run
./artifacts/slice-1/bin/strmgen-amd64 sync --config config.yaml
```

相对 `sources[].output` 与 `database.path` 都按**启动工作目录**解析，而不是配置文件目录。启动目录、配置文件以及 token 环境变量必须正确；日志不显示 token、完整远端根或数据库／输出的私有完整路径。生产二进制仅支持一次性 `sync`／`sync --dry-run`；`serve` 与清理尚未实现，`cleanup.enabled: true` 会在同步写入前被拒绝。

真实同步须能遍历现有输出祖先，对目标输出位置有创建目录和文件的权限，并能读取已拥有 STRM；数据库父目录需允许创建／读写 SQLite 文件。现有符号链接和非目录路径组件会被拒绝，不能以软链接把数据库、输出根或其父目录映射到其他位置。预览不创建输出或数据库；如果使用已有 WAL 库，程序在数据库父目录建立私有只读快照并正常清理。

所有 CLI 轮次共用固定 `/tmp/strmgen-locks/sync.lock`。锁目录必须是当前有效 UID 所有，并且不能由组／其他用户写入；创建该目录还需要对 `/tmp` 有常规权限。`STRMGEN_LOCK_DIR`、`TMPDIR`、数据库路径或源配置不改变产品锁命名空间。锁文件会保留以保证锁 inode 稳定；进程正常结束、SIGTERM 或 SIGKILL 后由内核释放锁，不要删除可能正被使用的锁文件。锁路径不安全或不可用时，同步在触碰产品数据库／输出前失败。

数据库是文件归属与完整轮次的唯一持久证据。数据库丢失、损坏或无法访问时，不能自动认领输出目录中的既有 STRM；丢失 DB 后再次同步会把无记录文件报告为未归属冲突并保留，损坏／不可读 DB 会先失败。恢复应优先使用可信备份；如无法恢复，管理员必须逐项核实文件归属，不能让工具自动重建所有权或据此删除未知文件。失败轮不会提交完整轮次；已成功发布并登记的文件可在重试时跳过，已发布但登记失败的文件仍是未归属冲突。

同目录 `.strmgen-*.tmp` 正常错误路径会尽力清理；SIGKILL、断电或系统崩溃可能遗留临时 STRM 写文件。WAL 模式 dry-run 的 `.strmgen-ro-*` 快照目录也可能在进程未正常关闭时残留。SQLite `-wal`／`-shm`／`-journal` sidecar 由 SQLite 管理，不应在数据库进程仍可能运行时手工删除。对任何残留先确认进程状态、路径、所有者及内容来源；不对未知文件做盲目批量删除。当前不实现 Mirror 清理。

## 隔离 OpenList 夹具

```bash
python3 scripts/mock_openlist.py
```

默认只监听 `127.0.0.1:15244`，Ctrl-C 停止。无数据库、无外部网络请求、无媒体下载，使用公开假 token `strmgen-fixture-only`。请求示例：

```bash
curl --fail http://127.0.0.1:15244/health
curl --fail http://127.0.0.1:15244/api/fs/list \
  -H 'Authorization: strmgen-fixture-only' \
  -H 'Content-Type: application/json' \
  -d '{"path":"/Movies","page":1,"per_page":2,"refresh":false}'
```

夹具数据位于 [openlist.json](../dev/fixtures/openlist.json)，覆盖分页、目录、中文特殊名、有效／缺失／无效 SHA-1 与大 Size。list/get 支持合成 `/error`（HTTP 200/code 500）、`/forbidden`（HTTP 200/code 403）、`/offline`（HTTP 503）和不存在路径等测试情形；这不表示真实 OpenList 对所有这些响应有相同语义。

get.raw_url 故意携带假签名，用于验证生成器不复制该地址。哈希及文件均为合成元数据，夹具没有媒体字节或 `/d/` 播放端点，不能用于播放或真实内容哈希验证，也不覆盖真实限流、token 续期、驱动缓存及全部 API 字段。

目标 [配置示例](../config.example.yaml) 使用不可解析的示例 public_url，输出与 DB 均落在本地 artifacts 下。真实配置应作为本地文件管理，只设置专用临时输出／DB 做隔离联调，不能把生产目录替换进夹具。

## 可选 Docker Compose 夹具

先停止原生夹具，避免端口冲突；冲突时直接失败，不停止占用端口的其他服务。

```bash
docker compose -f dev/compose.yaml config --quiet
docker compose -f dev/compose.yaml up -d --wait
docker compose -f dev/compose.yaml ps
docker compose -f dev/compose.yaml stop
```

专用 project `strm-generator-dev`，仅发布本机 15244；只读挂载夹具脚本和数据、非 root、固定 image digest、128 MB 限制。不挂载生产路径、Docker socket 或 Jellyfin 数据。停止保留项目资源，不提供全局清理。另一工作副本应通过 `-p` 指定独立 project 和调整端口，避免共享同名项目。

固定镜像当前仅在 amd64 运行验证，ARM64 主机可使用原生 Python 夹具；不能把 Go 的 ARM64 编译结论当作该镜像支持证明。后续真实 OpenList／Jellyfin 集成须另建固定隔离环境，不启动或接管同机生产容器。

## T4 验证与 Git 证据

Slice 1 已 PASS / CLOSED，最终门槛及原始证据索引保留于 `930782035615512be811a7b8bcc8416050f0c12f:SLICE.md`：治理、`mod download`／`mod verify`、`quality.py`、race、Linux amd64／arm64 CGO=0 构建、隔离 CLI `--scenario all` 和 `git diff --check`。完整历史 Task 计划保留于 `315cc6fd22798c4c085f990fb2d223ce313811cf:TASK.md`。驱动的每个 `--output-dir` 必须位于仓库 `artifacts/validation-evidence/` 下且为新空目录；旧候选及首次失败证据不可覆盖。完整故障场景还需传 `--fault-binary`，该二进制以 `-tags strmgen_testhooks` 构建，示例见 README。hook 仅用于确定性发布／归属／staging 写入／交换及 round 前中断；测试二进制不可用于生产。发布二进制不含 hook 环境开关，并用于实际重启验证。

Slice 2 的目标 T4 与缺失确认真实 OpenList 实证见当前合同和测试矩阵，准确命令及夹具由 B 在激活提交后规划；已有 CLI 驱动尚不覆盖 Mirror 删除，不把 Slice 1 的 9/9 直接作为清理证据。只新建隔离服务并使用 artifacts 下专用媒体／输出／状态，不接管生产 OpenList 或 Jellyfin。

验证证据在本机 `artifacts/validation-evidence/`，可能被 Git 忽略且不可远程读取；索引应记录源码／二进制哈希、命令／cwd、依赖和夹具身份、计数、退出码、首次失败及重跑。合成夹具不代表真实 OpenList、插件解析、Jellyfin／代理或播放通过。测试输出、交叉编译和环境 smoke 均不得替代这些独立证据。

本仓库原先无 Git；用户提供身份后只在本仓库配置 Git 身份，开发准备文件已提交在本地 main。未配置远程、未推送。当前 Slice／Task 生命周期与审查结论以 [SLICE.md](../SLICE.md) 和 [TASK.md](../TASK.md) 为准；旧验证和审查内容保留为历史记录，不覆盖新候选证据。

## Task 2.1 真实 OpenList 缺失确认实证（简化 Get 规则）

2026-10-18 用户最终改用源根／目标 Get 简化规则且所有列表 refresh:false。以下为按该
规则实施并重跑的真实服务实证；旧强刷候选的运行器、场景与结论保留在 Git 历史，不再
是本合同的执行清单。准确语义见[需求](requirements.md#简化删除判定2026-10-18-用户最终决定)。

本 Task 不开放清理，只验证远端“存在／未读到／无法判定”的一次判定能力。运行器
`scripts/run_openlist_absence_integration.py` 自建专用容器、驱动 `openlist_integration`
构建标签下的 Go 集成测试二进制，不在 Python 侧重写判断算法。

固定身份（可追溯到 `dev/environment-spec.json` 的 OpenList 源码 SHA）：

```text
image      openlistteam/openlist@sha256:11b490123d57f88c84ecda46285e1921869cf511da3e5bb4062f183e7e1200b7
revision   3b28270a4066ea52943a69ecddd850d83f7e8bdc (org.opencontainers.image.revision)
version    v4.2.7
```

运行器在启动前校验镜像的 revision／version 标签；不匹配即以 `BLOCKED` 非零退出，
不会漂移到 `latest`。它只创建／停止带 `strmgen.task=2.1` 标签的自有容器，从不
接管同名之外的既有容器。

```bash
mkdir -p artifacts/task-2.1/bin
bash scripts/go-local test -c -tags openlist_integration \
  -o artifacts/task-2.1/bin/openlist-integration.test ./internal/openlist
TASK_EVIDENCE="$(mktemp -d artifacts/validation-evidence/task-2.1-simple-get-final-XXXXXX)"
python3 scripts/run_openlist_absence_integration.py \
  --test-binary artifacts/task-2.1/bin/openlist-integration.test \
  --output-dir "$TASK_EVIDENCE/real-openlist"
```

`--output-dir` 必须在 `artifacts/validation-evidence/` 下且为新空目录；复用或越界一律
拒绝。`--scenario` 接受 `all` 或逗号分隔的子集，可用值必须与 `SCENARIOS` 一致，未知
名称直接报错。缺少 Docker、镜像标签不匹配、服务未就绪等前置条件不足时返回 3 并写
`summary.json` 的 `blocked` 字段，不静默跳过；Go 测试出现 `SKIP` 也按失败处理。

每个场景独占一个容器、数据目录与媒体目录：

| 场景 | 覆盖 |
|---|---|
| `exact_names` | 精确大小写、中文与 `#`／`%`／空格／引号、目录目标类型冲突、无写入副作用 |
| `pagination` | 250 项目录第二页以 `refresh:false` 读取；源根与目标仍只作两次 Get |
| `empty_directory` | 真实 `content:null,total:0` 形状被接受；目录目标为类型冲突 |
| `removed_parent` | 源内父目录删除按同一 Get 规则成为缺失观察；父挂载根可读而目标未读到同为缺失；禁用存储的根不可读则保留 |
| `read_only_identity` | 只读身份以 `refresh:false` 正常列表与判定，不依赖刷新权限 |
| `cached_driver` | WebDav 自挂载短 TTL：等待缓存自然更新，再验证删／增目标的重复读取一致 |
| `nested_mount_masking` | 父存储禁用后仍可读的虚拟挂载祖先按用户规则继续，目标未读到即缺失观察；无嵌套挂载的禁用存储根不可读 |
| `hidden_entries` | metadata `hide` 过滤使受限身份读不到，属用户接受的同码局限；不受限身份仍可读 |

容器以镜像自带的非特权 `openlist` 用户运行，因此运行器只对本次专用的数据／媒体
目录放开写权限。生成的 admin 密码只存在于进程内，不写入任何证据文件；`summary.json`
与各 `go-test.log` 记录镜像身份、夹具、请求结果与 Go 结论，不含 token、Cookie、签名
URL 或私有完整路径。停止容器后保留证据目录。

已实测的固定版本语义（源码对照见 [compatibility](compatibility.md)）：空目录序列化为
`content:null,total:0`；缺失对象、驱动故障与虚拟挂载视图都返回 HTTP 200 + 应用码 500；
嵌套挂载存在时，父驱动故障会让列表只剩虚拟项而不报错；`hide` 过滤会从列表中静默
移除条目且受限身份的直接读取也被拒绝；WebDav 自挂载的目录可读、文件 Get 在 1 分钟
TTL 自然到期后仍不能被读取。运行器准备、轮询与取证请求全部显式 `refresh:false`，
不使用强刷或清缓存捷径。

因为父挂载根在后备目录被删除后仍返回目录对象，而禁用存储的挂载根不再可读，`removed_parent`
场景同时保留“根可读 → 缺失观察”和“根不可读 → 保留”两种真实结果。`nested_mount_masking`
与 `hidden_entries` 记录的是用户明确接受的局限，不写成已安全辨别或已修复。

本 Task 不改变生产 CLI：`cleanup.enabled: true` 仍在任何输出／数据库变更前被拒绝，
`sync`／`sync --dry-run` 行为与 Slice 1 回归一致。`/d/` 播放、固定插件解析、
Jellyfin／代理、ARM64 实机与规模仍为 NOT RUN。
