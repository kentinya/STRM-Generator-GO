# 部署与运行包

本文件说明 STRM Generator 的 Linux amd64／arm64 产品镜像与 Compose 运行方式，区分
**一次性**与**常驻**运行、状态与恢复边界。命令与路径以仓库根目录为基准。当前
证据层：本机 amd64 容器实测通过；arm64 只验证 OCI manifest／digest 与二进制
ELF 架构；ARM64 实机与正式发布验收在原 Slice 中为 NOT RUN。现有发布工作流见下节。
Slice 1–4 PASS / CLOSED，Slice 4
已验收固定 10000 文件生成、少量同步、Jellyfin 原始扫描与插件完整决定；所需环境／媒体／固定包
见 [资产清单](https://github.com/kentinya/STRM-Generator-GO/blob/4d16846722f31aa9615494b90df715cf4b5bb6df/docs/slice-4-assets.md)。4.4／4.5／4.6均独立B PASS，
[A最终审查](https://github.com/kentinya/STRM-Generator-GO/blob/4d16846722f31aa9615494b90df715cf4b5bb6df/docs/slice-4-a-final-review.md)已接受RO-1..6；这些结果来自分层原始证据，不能用容器smoke替代。

## 自动发布工作流

以下说明仓库已配置的工作流，实际发布结果以 GitHub Actions、Release 和镜像仓库记录为准。

| 产物 | 触发与配置 | 工作流 |
|---|---|---|
| Linux amd64／arm64 二进制 | 推送 `v*` 标签；从 `.go-version` 安装 Go，校验依赖，使用 CGO=0 构建。生成 `strmgen_<标签>_linux_<架构>.tar.gz`，每包包含 `strmgen` 和 `config.example.yaml`，另附 `SHA256SUMS.txt`，上传 GitHub Release | [release.yml](../.github/workflows/release.yml) |
| Linux amd64／arm64 镜像 | 推送 `main` 或手动运行；需配置 `DOCKERHUB_USERNAME`／`DOCKERHUB_TOKEN` secrets，推送 `<用户名>/strm-generator-go:latest` 与 `:sha-<完整提交 SHA>` | [docker-publish.yml](../.github/workflows/docker-publish.yml) |
| 仓库质量检查 | push／pull_request；固定 SDK／C 工具链、双架构 smoke、race、治理与工具回归 | [quality.yml](../.github/workflows/quality.yml) |

二进制使用示例配置和下方相同的运行时 token／路径规则。使用已有镜像时，将
`STRMGEN_IMAGE` 设置为实际发布的镜像引用；默认 Compose 仍使用本地 `strmgen:local`。
部署需要固定版本时使用对应提交标签或 digest。发布工作流的存在不扩大原合同的 ARM64 原生运行、
插件探测成功或性能验收结论。

## 构建

产品镜像使用 [Dockerfile](../Dockerfile) 构建。基础镜像按 digest 固定
（`golang:1.27.2` 构建层、`alpine:3.20` 运行层），Go 版本来自仓库
[.go-version](../.go-version)，依赖来自 `go.mod`／`go.sum` 并在构建时
`go mod verify`。构建阶段在 `$BUILDPLATFORM` 上以 `CGO_ENABLED=0` 交叉编译目标
架构；运行层不执行任何架构相关命令，只复制 CA 根证书、非特权账号文件和静态
二进制。

```bash
mkdir -p artifacts/task-3.2/oci
# 本机 amd64 镜像（可加载运行）
docker buildx build --platform linux/amd64 --load --tag strmgen:local .

# arm64 OCI 产物（不加载，供分发与 manifest 核对）
docker buildx build --platform linux/arm64 \
  --output type=oci,dest=artifacts/task-3.2/oci/strmgen-arm64.oci.tar .
```

- 两个架构均显式 `CGO_ENABLED=0`，产物为静态二进制。
- 镜像默认命令是 `serve --config /etc/strmgen/config.yaml`；用显式命令覆盖为
  `sync` 时复用同一数据挂载。
- 运行镜像不含源码、测试夹具、`strmgen_testhooks` 构建、token、真实配置、宿主
  输出或 SQLite 状态。
- 交叉构建**不等于** ARM64 实机运行；后者标为 NOT RUN。

## Compose 运行

```bash
cp deploy/config.example.yaml deploy/config.yaml   # 编辑远端／输出映射
mkdir -p deploy/data/output deploy/data/state deploy/data/locks
sudo chown -R "$(id -u):$(id -g)" deploy/data

export OPENLIST_TOKEN=...        # 只放环境变量或 secret store
export STRMGEN_UID="$(id -u)"
export STRMGEN_GID="$(id -g)"
docker compose -f deploy/compose.yaml up -d
docker compose -f deploy/compose.yaml logs -f
docker compose -f deploy/compose.yaml stop -t 60
```

[deploy/compose.yaml](../deploy/compose.yaml) 的关键约束：

| 项 | 合同 |
|---|---|
| token | 只通过运行时环境 `OPENLIST_TOKEN` 注入（Compose 透传声明 `OPENLIST_TOKEN:`）；`compose.yaml`、镜像层、仓库 `.env` 示例都不含真实 token。采集脱敏渲染时用 `docker compose config --no-interpolate`，不要把带宿主值的普通 `config` 输出写入日志或证据 |
| 配置 | `STRMGEN_CONFIG` 以 `:ro` 只读挂载到 `/etc/strmgen/config.yaml` |
| 输出 | `STRMGEN_OUTPUT` → `/output`，生成器拥有的 STRM 树 |
| 数据库 | `STRMGEN_STATE` → `/state`，含 SQLite 主库及 `-wal`／`-shm`／`-journal` sidecar 与 dry-run 快照 |
| 临时写 | 容器内 `tmpfs:/tmp`（`noexec,nosuid,nodev,size=64m`）提供通用临时空间；STRM 同目录暂存写在 `/output`，数据库私有只读快照写在 `/state` |
| 锁 | `STRMGEN_LOCK_DIR` → `/tmp/strmgen-locks`，固定锁命名空间 |
| 身份 | `user: STRMGEN_UID:STRMGEN_GID`，默认镜像非特权账号 `10001:10001` |
| 权限 | `read_only: true`、`cap_drop: ALL`、`no-new-privileges`、`pids_limit`、`mem_limit`；不使用 privileged／Docker socket／host PID／host network |
| 停止 | `stop_grace_period: 60s`，让当前轮按取消语义结束并释放 flock |

宿主 bind mount 的属主／权限要求：`STRMGEN_OUTPUT`、`STRMGEN_STATE`、
`STRMGEN_LOCK_DIR` 必须由 `STRMGEN_UID`／`STRMGEN_GID` 拥有且可写；锁目录不能由组／
其他用户写入。配置目录只读。程序在现有祖先上要求目录遍历与建目录权限，并拒绝
符号链接祖先。

## 一次性与常驻运行

```bash
# 一次性（默认由镜像 ENTRYPOINT 覆盖为 sync）
docker compose -f deploy/compose.yaml run --rm strmgen \
  sync --config /etc/strmgen/config.yaml
docker compose -f deploy/compose.yaml run --rm strmgen \
  sync --config /etc/strmgen/config.yaml --dry-run

# 常驻
docker compose -f deploy/compose.yaml up -d
```

`serve` 首轮立即执行，之后每 `sync.interval_minutes` 分钟（缺省 60）触发；与手动
`sync` 共用同一固定锁，锁忙的 tick 报告 `busy-skip`，单轮失败后继续下一 tick。
SIGINT／SIGTERM 取消当前轮并以非零退出，不提交不完整轮、缺失计数或删除资格。

`sync` 与 `sync --dry-run` 不调度，因此 `sync.interval_minutes: 0` 仍是合法配置；
`serve` 在任何输出／数据库动作前拒绝零、负数或分钟到 `time.Duration` 溢出的间隔。

`deploy/compose.yaml` 用 Compose 透传声明 `OPENLIST_TOKEN:` 注入 token。注意：
带宿主值的普通 `docker compose config` 会解析并打印该值
（这是 `environment:` 注入的既有行为），因此**不要**把普通 `config` 输出写进日志或
证据；需要脱敏渲染时使用 `docker compose config --no-interpolate`，它只输出
`OPENLIST_TOKEN: null`。仓库、镜像与 `--no-interpolate` 渲染都不含真实 token。

## 状态、备份与恢复

- **持久状态**：SQLite 是文件归属与完整轮次的唯一证据，位于 `/state`。它必须
  在容器重启后保留；丢失后不会自动认领已有 STRM。
- **备份**：先停止生成器，再备份 `/state`（含 sidecar）与 `/output`。恢复时优先
  还原可信备份。
- **sidecar**：`-wal`／`-shm`／`-journal` 由 SQLite 管理，进程仍可能运行时不得手工
  删除。
- **临时写**：`/tmp` 为容器私有 tmpfs，不承载持久状态；同目录 `.strmgen-*.tmp`
  与 `.strmgen-ro-*` 残留按既有规则核实，不盲目批量清理。
- **正常停止**：SIGTERM 后 `serve` 返回非零 cancelled 并释放 flock；重启或手动
  `sync` 按 Slice 1／2 的恢复与最终一致性收敛。
- **强制 kill／宿主断电**：沿用现有文件／DB 恢复规则；不承诺跨文件事务，不自动
  认领未归属文件。

## 多实例与共享锁

同一输出根只能由一个生成器写入。需要多个容器写同一输出时，必须把**完全相同**的
宿主锁目录对象挂到固定的 `/tmp/strmgen-locks`，并使用一致的 `STRMGEN_UID`／
`STRMGEN_GID`：

```bash
STRMGEN_LOCK_DIR=/srv/strmgen/shared-locks STRMGEN_OUTPUT=/srv/strmgen/output ...
```

否则必须显式禁止多实例写同一输出。**容器各自私有的 `/tmp` 不能协调共享输出**：
默认 Compose 的 `/tmp` 是每个容器独立的 tmpfs，其中的同名锁路径不构成跨容器互斥。
锁目录挂载到 `read_only` 文件系统时仍可写（bind mount 不受 `read_only: true` 限制），
但必须由运行身份拥有。

## 边界与 NOT RUN

Slice 3 已接受镜像构建、最小隔离运行、挂载与停止／重启；这些历史验收不代表发布工作流已成功运行。
Slice 4 的外部服务／固定万文件证据依 [合同](../SLICE.md) 单列，已由B独立验收及A最终接受；Compose
的 `mem_limit: 512m` 是部署配置，不是 512 MB 性能验收门槛。客户端播放、代理及更高
规模压力已退出当前 V4.4 验收。

ARM64 原生实机、正式发布、Kubernetes／systemd 专属控制器、health／metrics、配置热
重载和自动备份未验收且不属于当前合同。真实服务仅使用 artifacts 下的专用隔离实例，
不得以交叉构建、环境 smoke 或合成夹具替代实际证据。


Slice 4 已有隔离 S3/RustFS、OpenList、Jellyfin 和三个辅助脚本按
[资产清单](https://github.com/kentinya/STRM-Generator-GO/blob/4d16846722f31aa9615494b90df715cf4b5bb6df/docs/slice-4-assets.md) 记录于原开发机，未随 Git 克隆提供。复核时不要求重新部署环境或为验证而停止
运行实例；按需恢复／停止使用已有脚本，禁止删持久数据。产品发布 Compose 与该隔离
环境的 Compose 用途分开，不能混用生产或产品部署配置来接管隔离实例。
