# Task 3.2 — Linux 双架构运行包与容器部署边界

遵循[开发流程](docs/development-workflow.md)，范围 authority 为已提交的
[Slice 3](SLICE.md)。Task 3.1 已经独立 B PASS；本 Task 只补齐 RO-4 和 RO-6 的容器
构建、挂载、停止／重启与共享锁证据，不重复实现调度或改变单轮业务语义。

```text
Task ID: 3.2
Parent Slice: 3
Status: PASS
Task Base: 8fc3a3ad8a720e9ca4c832967fdf39874581beff
Contract Checkpoint: ee5001646d4239755606c28b61f340d795ee7014
Difficulty: High
Test Level: T4
Planner / Reviewer: B
```

## Goal

提供可重复构建的 Linux amd64／arm64 `CGO_ENABLED=0` 产品镜像与 Compose 运行配置，
并以专用隔离容器证明非特权运行、环境 token 注入、配置只读、输出／SQLite／sidecar／
临时写入／固定锁目录挂载及正常停止、重启、共享输出竞争行为。不得把单容器私有
`/tmp` 的同名锁描述为跨容器互斥。

## 实际缺口与范围

- 真实候选位于 Task 3.1 的完整实现范围 `ee5001646d4239755606c28b61f340d795ee7014..b18caba6fd786bfe589f6ac29c2a2e779a5a1628`；本地 Go 全量 race／test、amd64／arm64 CGO=0 交叉构建、testhooks 构建及旧 sync／serve 隔离回归已按匹配输入复用。纯完成报告后继 `8fc3a3ad8a720e9ca4c832967fdf39874581beff` 不含可执行输入变更。
- 当前仓库只有 `dev/compose.yaml` 开发夹具，没有产品 `Dockerfile`、发布 Compose／运行包或容器生命周期验收器。当前锁固定为 `/tmp/strmgen-locks/sync.lock`；容器只有显式共享同一个锁目录对象且使用同一有效 UID／GID 才能声明共享输出互斥。
- Task 3.1 证据明确 Docker 镜像／Compose／挂载验证 NOT RUN。不得修改开发 OpenList 夹具以外的服务，不得挂载或触碰生产媒体、OpenList、Jellyfin、代理或其数据。
- 允许修改产品 `Dockerfile`、`.dockerignore`、产品 Compose／部署文档、必要构建脚本与 `scripts/run_container_integration.py`、对应 Python 测试、`dev/environment-spec.json`、README、架构 CURRENT、开发环境说明、测试矩阵和进度。不得改调度、Reconciler、锁命名空间、数据库协议或稳定需求；若包设计无法满足 Slice 合同，停止并交 A，不弱化验收。

## Developer 实施范围

### 1. 可复现镜像

1. 用仓库 `.go-version` 和依赖锁文件中的既有 module 构建 Linux `amd64`、`arm64`；两架构均显式 `CGO_ENABLED=0`，不修改 Go／Python 依赖版本，不把 SDK、源码仓库、凭证、测试夹具或 testhooks 放进运行镜像。
2. 产品镜像默认运行 `strmgen serve --config <只读配置>`，支持显式覆盖命令用于 `sync`；最终运行层使用非 root 身份，包含 HTTPS 所需 CA 根证书，镜像内不放 token、真实配置、宿主输出或 SQLite 数据。
3. 固定或可追溯记录基础镜像身份与构建目标；镜像标签／OCI 产物与源码完整 SHA 对应。不得声称交叉构建等于 ARM64 实机运行。

### 2. Compose 与挂载合同

1. Compose 使用环境变量注入 `OPENLIST_TOKEN`，不写入镜像、YAML、仓库 `.env` 示例、命令行参数或日志。配置只读挂载；输出、数据库状态及 SQLite sidecar、临时写目录、`/tmp/strmgen-locks` 锁目录分别明确挂载职责、权限、备份和恢复边界。
2. 以非特权 UID／GID 运行；配置说明主机预创建 bind mount 的属主／权限要求。不得使用 privileged、Docker socket、host PID／network、生产宿主路径或不必要 capability。
3. 提供单实例可运行 Compose。若提供共享输出的多容器示例，实例必须把完全相同的锁目录对象挂入固定容器路径，并使用一致 UID／GID；否则明确禁止多个实例写同一输出。不能用 Compose 项目内各自私有临时目录冒充共享锁。
4. 正常 SIGTERM／stop 必须给足退出宽限并让 `serve` 返回非零 cancelled 后释放 flock；强制 kill／宿主断电仍按现有文件／DB 恢复规则说明，不承诺跨文件事务或自动认领未归属文件。

### 3. 隔离容器证据与文档

1. 新增 `scripts/run_container_integration.py` 和必要 Python 单元测试，使用本地合成 OpenList、`artifacts/` 下的独立输出／状态／配置／锁／临时目录和新构建镜像。脚本按镜像 digest／OCI 身份和源码 SHA 验证输入，设置硬超时并在 finally 中仅清理自己创建的容器、网络、卷与目录。
2. 检查镜像／Compose 静态配置、非 root `id`、只读配置挂载、token 环境注入且不在镜像层、容器内 source／output／DB／lock/temp 对应挂载、数据库文件及 sidecar 留存于持久挂载。禁止输出真实环境、token 或私有完整路径。
3. 容器启动 `serve` 后证明首轮交付；正常 stop 后 `cancelled` 非零退出且锁可重获；重启后只按既有 round／ownership 恢复并跳过稳定文件。使用两个并发容器与同一输出／同一锁挂载，在可控 API barrier 下证明只有一个进入 Reconciler，另一个 busy-skip；释放后后续轮可以运行。若隔离 Docker 引擎／buildx／目标架构不可用，应记录 BLOCKED 及原始错误，不能改用纯文本或主机测试声称容器 PASS。
4. 文档说明 amd64／arm64 构建与实际执行证据区别、生产配置准备、非 root bind 权限、token 管理、备份／还原 SQLite 主库和 sidecar、共享锁挂载、正常停止／重启，以及真实 OpenList、插件、Jellyfin／代理、ARM64 实机、规模与正式发布仍 NOT RUN。

## 验收条件

| ID | 可核对结果 |
|---|---|
| AC-1 可追溯构建 | amd64 本机可加载运行镜像与 arm64 OCI 产物均由匹配源码／依赖构建、`CGO_ENABLED=0`；运行镜像不含源码／测试钩子／凭证；构建身份、架构、digest 可核对。 |
| AC-2 最小权限与秘密 | 容器以非 root 执行；token 只来自运行时环境；镜像 config／layer、Compose 渲染文件、stdout／stderr、证据索引均无 token；配置以只读方式挂载。 |
| AC-3 数据边界 | 输出根、DB 及 SQLite sidecar、临时写区和固定锁目录映射到专用持久／临时挂载；运行中数据库／STRM 位于预期宿主 artifacts 目录，无越界写入。 |
| AC-4 生命周期恢复 | 首轮立即运行；Compose/Docker 正常 stop 观察 cancelled 非零、资源退出和锁释放；同一挂载重启只产生预期完整 round，稳定输出不改写、ownership 不重复。 |
| AC-5 多实例互斥 | 同一输出与相同锁目录的两个独立容器争用时最多一个进入 Reconciler；另一方 busy-skip 且不提交轮次；释放后可重试。不同锁目录不得声称对同一输出安全。 |
| AC-6 全范围与分层证据 | Task 3.1 已匹配的 Go／serve／sync 证据可复用并记录身份；容器新增隔离证据和完整 T4 候选门禁通过；真实外部播放、插件解析、Jellyfin／代理、ARM64 实机、规模、正式发布仍明确 NOT RUN。 |

## 必要测试

### Developer 开发／修正反馈

先核对 Task 身份并执行 `python3 scripts/check_governance.py`，再按修改执行：

```bash
python3 -m unittest discover -s tests -p 'test_run_container_integration.py' -v
docker buildx build --platform linux/amd64 --load --tag strmgen:task-3.2-amd64 .
python3 scripts/run_container_integration.py --image strmgen:task-3.2-amd64 --output-dir artifacts/validation-evidence/task-3.2-dev/container
STRMGEN_UID=1001 STRMGEN_GID=1001 OPENLIST_TOKEN=strmgen-fixture-only docker compose -f deploy/compose.yaml config --quiet
```

构建／Compose 文件路径以 Developer 实际提交为准；不得将不存在的场景报告为通过。开发期间只跑受影响包／脚本和新增隔离场景，不反复重跑全量 T4。

### 最终检查点门槛

候选前先建唯一证据目录 `artifacts/validation-evidence/task-3.2-final-<id>/`，保留完整源码 SHA、未提交输入摘要、Go／Python／Docker／Compose／buildx 版本、OCI digest、镜像构建参数、夹具／配置身份、精确命令／cwd、退出码／耗时、容器状态与挂载快照、stdout／stderr、首次失败与重跑；所有 secret 用合成值并脱敏。执行：

```bash
python3 scripts/check_governance.py
bash scripts/go-local mod download
bash scripts/go-local mod verify
python3 scripts/quality.py
CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 -json ./...
bash scripts/go-local test -count=1 -json ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 bash scripts/go-local build -trimpath -o artifacts/task-3.2/bin/strmgen-amd64 ./cmd/strmgen
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 bash scripts/go-local build -trimpath -o artifacts/task-3.2/bin/strmgen-arm64 ./cmd/strmgen
bash scripts/go-local build -tags strmgen_testhooks -trimpath -o artifacts/task-3.2/bin/strmgen-testhooks-amd64 ./cmd/strmgen
python3 -m unittest discover -s tests -p 'test_*.py' -v
docker buildx build --platform linux/amd64 --load --tag strmgen:task-3.2-amd64 .
docker buildx build --platform linux/arm64 --output type=oci,dest=artifacts/task-3.2/strmgen-arm64.oci.tar .
STRMGEN_UID=1001 STRMGEN_GID=1001 OPENLIST_TOKEN=strmgen-fixture-only docker compose -f deploy/compose.yaml config --quiet
python3 scripts/run_container_integration.py --image strmgen:task-3.2-amd64 --output-dir "$TASK_EVIDENCE/container"
python3 scripts/run_serve_integration.py --binary artifacts/task-3.2/bin/strmgen-amd64 --fault-binary artifacts/task-3.2/bin/strmgen-testhooks-amd64 --output-dir "$TASK_EVIDENCE/serve-regression"
python3 scripts/run_isolated_sync_integration.py --binary artifacts/task-3.2/bin/strmgen-amd64 --fault-binary artifacts/task-3.2/bin/strmgen-testhooks-amd64 --scenario all --output-dir "$TASK_EVIDENCE/sync-regression"
git diff --check
```

固定以本机加载的 amd64 镜像执行容器集成，以 arm64 OCI manifest／digest 证明第二目标产物；不能降级为只 build 当前主机架构。不得调用生产配置或公开发布镜像。ARM64 实机仍 NOT RUN。

### B 独立最终门禁

B 审查 Task Base..Implementation Head，核对 Dockerfile、Compose、构建／运行器和完整 RO 对应。B 独立从新构建的发布镜像复现：

1. 镜像非 root 身份、token 只读运行时注入且不泄露、配置只读、输出／DB／temp／固定锁挂载的实际宿主路径与权限。
2. 容器内首次 `serve`、正常停止的取消码／round 边界、持久状态重启后的稳定 skip 和锁释放。
3. 同输出与同一共享锁对象的双容器竞争；独立容器不共享锁时不作正确性主张。检查忙碌容器未提交 round／修改 STRM。
4. 镜像 manifest 含且仅核对目标 amd64／arm64；读取构建日志／digest，独立 `docker image inspect` 与运行 amd64 镜像。ARM64 实机、真实服务和外部播放不属于可复用的容器证据。

可复用 Task 3.1 的 Go 全量、race、双架构二进制构建及 sync／serve 证据，前提是逐项确认完整源码、命令、配置、夹具、工具链和二进制身份相同；容器变更后不机械重跑 slice Final，而须运行受影响的部署隔离证据和 Task 3.2 指定全量候选门禁。任何 Docker／Compose 输入、脚本、源码或镜像输入在最终候选后变化，按影响重跑；产品可执行输入改变时 T4 全量门槛重跑。

### 证据与重跑

Developer 报告记录 AC／RO 映射、真实候选 SHA、镜像 digest／架构／运行身份、输入／夹具、容器生命周期、挂载和 SQL／文件断言、每项耗时／exit code、首次失败及重跑，以及 NOT RUN。证据仅本机 `artifacts/`，不宣称远端可读。B 未独立 PASS 前 Task 保持 READY FOR B REVIEW；本 Task PASS 后 B 重评 RO-1..RO-6，只有全部满足时才做 Slice Final 和 Closure Packet。

## 非目标

不新增业务入口／调度逻辑／锁配置／数据库协议／清理状态、不实现编排平台或 health／metrics 服务、不写容器自动备份／自动清理生产数据、不 push 镜像或发布 registry、不对 ARM64 实机、OpenList 兼容、插件解析、Jellyfin／代理、真实播放、性能作超出证据的结论。

## 前一 Task 3.1 B Review Result

```text
Reviewed: 2026-10-19 by B
Reviewed Range: ee5001646d4239755606c28b61f340d795ee7014..b18caba6fd786bfe589f6ac29c2a2e779a5a1628
Post-review report checkpoint: 8fc3a3ad8a720e9ca4c832967fdf39874581beff (documentation only)
Decision: PASS
Task 3.1 Outcomes: AC-1..AC-6 PASS; implementation evidence matched candidate b18caba and recorded binary hashes
Independent replay: PASS; `artifacts/validation-evidence/task-3.1-b-review/sigint/` records a fresh SIGINT during an in-flight isolated round, exit=1/cancelled, no round or ownership committed, then post-signal sync acquired the shared lock and completed
Full gates: PASS by matching evidence at artifacts/validation-evidence/task-3.1-final-bgG1Ej/INDEX.md; no full T4 rerun
Slice Required Outcomes: RO-1 PASS; RO-2 PASS; RO-3 PASS; RO-4 PENDING Task 3.2; RO-5 PASS; RO-6 PARTIAL (scheduler evidence PASS, container build/mount/lifecycle evidence pending Task 3.2)
Slice Required Outcomes all satisfied: NO
Outstanding: real OpenList, plugin parsing, Jellyfin/proxy, Docker image/Compose, ARM64 runtime, scale and formal publication NOT RUN
Next: Developer may implement planned Task 3.2 after its planning checkpoint is committed
```

## Developer Completion Report

- **Changed Files / Implemented：**
  - `Dockerfile`（新增）：`golang:1.27.2` 构建层与 `alpine:3.20` 运行层均按 digest 固定；构建层在
    `$BUILDPLATFORM` 上以 `CGO_ENABLED=0` 交叉编译 `$TARGETOS/$TARGETARCH`，并在构建期
    `go mod verify`；`prep` 层只在构建平台生成非特权账号与默认目录；运行层不执行架构相关
    命令，仅复制 CA 根、账号文件与静态二进制，默认非 root `10001:10001`、`serve` 默认命令。
  - `.dockerignore`（新增）：只允许 `go.mod`／`go.sum`／`cmd/`／`internal/` 进入构建上下文。
  - `deploy/compose.yaml`（新增）：运行时环境注入 `OPENLIST_TOKEN`；配置只读挂载；输出／
    SQLite 状态／固定锁目录分别挂载；容器私有 `/tmp` tmpfs；非 root、`read_only`、
    `cap_drop: ALL`、`no-new-privileges`、`pids_limit`、`mem_limit`、`stop_grace_period: 60s`。
  - `deploy/config.example.yaml`、`deploy/.env.example`（新增）：容器路径合同与仅夹具用假 token 示例。
  - `scripts/run_container_integration.py`（新增）：隔离容器验收驱动（6 场景）。
  - `tests/test_run_container_integration.py`（新增）：驱动单元测试 13 项。
  - `docs/deployment.md`（新增）：构建、挂载、权限、备份、共享锁与 NOT RUN 边界。
  - `README.md`、`docs/architecture.md`、`docs/development-environment.md`、`docs/test-matrix.md`、
    `docs/progress.md`、`dev/environment-spec.json`：状态与交付事实同步。
  - 未修改 `cmd/`、`internal/` 的 Go 产品源码、调度、Reconciler、锁命名空间、数据库协议或
    稳定需求；`git diff b18caba..HEAD -- cmd internal` 为空。

- **Tests and Results / Evidence：** 最终候选 SHA `2131b54bcec1cd43769d239a30456b4a8607f4aa`；
  证据目录 `artifacts/validation-evidence/task-3.2-final-20261019-1792350581/`（本机 Git 忽略）。
  - 治理 `PASS`；`mod download`／`mod verify` exit 0；`quality.py` exit 0。
  - `test -count=1 -json ./...`：290 test pass／0 fail／0 skip，exit 0。
  - `test -race -count=1 -json ./...`（CGO＋固定 Zig cc）：290 test pass／0 fail／0 skip，exit 0。
  - Linux amd64／arm64 `CGO_ENABLED=0` 构建与 testhooks 构建 exit 0；二进制 SHA-256 见
    `binaries-sha256.txt`。
  - Python 全量 110 项 exit 0。
  - amd64 镜像 `--load` 构建 exit 0，image id `sha256:1e6d2dfe…`，`amd64/linux`；arm64
    `--output type=oci` 构建 exit 0，OCI SHA-256 `ef3941fa…`，manifest `sha256:53caa292…`，
    config `arm64/linux`、user `10001:10001`，层内 `/usr/local/bin/strmgen` ELF machine 183。
  - 隔离容器驱动 6/6 PASS：`image_static`（非 root、默认命令、镜像层无 token／testhooks
    标记）、`minimal_run`（挂载 RW／RO、token 注入、4 STRM＋1 round＋sidecar）、
    `serve_lifecycle`（首轮、stop exit 1、重启后 round 3、稳定输出不改写）、
    `cross_container_busy`（同输出同锁双容器，contender exit 1 且未改 DB／轮次，释放后 skip）、
    `compose_lifecycle`（Compose up／stop exit 1）、`arm64_oci`。
  - `run_serve_integration.py` 6/6 PASS；`run_isolated_sync_integration.py --scenario all`
    11/11 PASS（复用与 Task 3.1 相同的 Go 源码，二进制为本次候选重建）。
  - Compose 渲染 `config --quiet`／`--no-interpolate` exit 0，仓库文件与渲染均不含真实 token；
    运行容器 `mount` 显示配置 `ro`、输出／状态／锁为独立挂载。
  - `git diff --check`、`git show --format= --check HEAD` exit 0；发布二进制 `strings` 无
    `STRMGEN_TEST*`／`strmgen_testhooks`。
  - 首次失败与重跑：容器驱动开发期发现默认 CMD 为 `serve` 导致一次性场景超时、合成服务
    未接受运行 token 导致源根不可读、`ARG TARGETARCH=amd64` 默认值覆盖 BuildKit 平台参数
    导致 arm64 OCI 内为 x86-64 二进制；三处均在实现内修正后重跑通过。原始失败记录见
    `docs/progress.md` 历史与开发证据目录。

- **Decisions / Risks / Remaining Required Outcomes：**
  - 镜像基础层按 digest 固定，运行层不执行架构相关命令，故 arm64 无需 QEMU 即可在 amd64
    宿主构建；代价是账号／目录以数据方式从 `prep` 层复制。
  - `dev/environment-spec.json` 的 `required_files` 已加入本 Task 新增文件；未改动 Go 版本、
    依赖、原方案身份或既有文件集合。
  - 风险：arm64 只有 OCI manifest／ELF 证据，**ARM64 实机 NOT RUN**；容器 `/tmp` 为每容器
    私有 tmpfs，文档明确它不构成跨容器互斥，共享输出必须显式共享同一锁目录对象。
  - 本 Task 覆盖 RO-4 与部署相关 RO-6；RO-1／RO-2／RO-3／RO-5 与调度部分 RO-6 由 Task 3.1
    PASS 覆盖。真实 OpenList、无签名 `/d/`、固定插件解析、Jellyfin／代理、ARM64 实机、规模
    与正式发布仍 NOT RUN。

- **Head SHA：** `2131b54bcec1cd43769d239a30456b4a8607f4aa`

- **B Review Result：** PASS（独立 B 审查见下方；Developer 未签发独立审查结论）

## B Review Result

```text
Reviewed: 2026-10-19 by B
Reviewed Range: 8fc3a3ad8a720e9ca4c832967fdf39874581beff..2131b54bcec1cd43769d239a30456b4a8607f4aa
Decision: PASS
AC-1..AC-6: PASS
Independent key replay: PASS; rebuilt amd64 image `strmgen:task-3.2-b-review-2131` passed image/static,
minimal sync, serve stop/restart, shared-lock busy-skip, Compose lifecycle and arm64 OCI scenarios 6/6.
Full candidate gates: PASS; governance, mod verify, quality, Go test/race, amd64/arm64/testhooks
builds, Python 110 tests, Compose checks, container 6/6, serve 6/6, sync 11/11 and diff check.
Evidence: artifacts/validation-evidence/task-3.2-final-20261019-1792350581/INDEX.md and
artifacts/validation-evidence/task-3.2-b-review-20261019/.
Slice Required Outcomes all satisfied: YES (RO-1..RO-6; Task 3.1 evidence reused where inputs matched)
NOT RUN: ARM64 real hardware, real OpenList, unsigned `/d/` playback, plugin parsing, Jellyfin/proxy,
scale and formal publication.
Next: B submits Slice 3 Closure Packet; A performs final review
```
