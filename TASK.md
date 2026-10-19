# Task 4.1 — 目标 OpenList 到 Jellyfin 原生播放隔离闭环

遵循[开发流程](docs/development-workflow.md)，范围 authority 为[Slice 4](SLICE.md)。本 Task 是完整的隔离真实服务端到端兼容验收；不以夹具、源代码阅读或包构建替代实际链路。

```text
Task ID: 4.1
Parent Slice: 4
Status: PLANNED
Task Base: 527570df75ab469eb204d6a42b2d1b4eb07f71e6
Contract Checkpoint: 527570df75ab469eb204d6a42b2d1b4eb07f71e6
Difficulty: High
Test Level: T4
Planner / Reviewer: B
```

## Goal

在本仓库 `artifacts/` 下，用专用合成媒体和隔离服务，实证目标 OpenList 驱动的 list／get 到发布 CLI 生成稳定无签名 `/d/` STRM，再由从插件远程仓库固定 SHA 构建并部署的包解析；隔离 Jellyfin 原生扫描并经 Jellyfin 原生客户端实际播放。完整记录目标身份、每层结果与故障，不连接生产服务，不验证代理。

## 实际缺口与前置条件

- 当前 Go 仓库已有真实 OpenList `v4.2.7` 的隔离测试，但其测试挂载为 Local／WebDAV；Slice 4 合同明确不允许以此代替目标驱动兼容结论。已有 mock、CLI、Docker／Compose 和本地 T4 证据可以按可执行输入身份复用，但不代表本 Task 的目标 OpenList、插件解析或播放通过。
- `git ls-remote --symref https://github.com/kentinya/jellyfin-strm-probe.git HEAD` 在 2026-10-19 解析默认分支 `main`，完整 HEAD 为 `2afc6517fda5a7b4170ebea625619a552c47dabc`。这是本 Task 的插件源码与包来源身份；旧 SHA `1bdb79082c4945e126611f217200555d53c31367` 只作协议参考。不得改用本地配套仓库、旧预构建包或未记录补丁。
- 固定源码提交的仓库说明给出包入口 `python3 scripts/package_plugin.py --configuration Release --runtime linux-x64 --output-dir artifacts/releases`，SDK `10.0.401`。同一提交的 `dev/images.lock.json` 固定 Jellyfin `jellyfin/jellyfin@sha256:357724bf0ae27a672c7cbaa899db2d9abeb13dbd8657ccce750258a4c059d037`，插件要求 Jellyfin ABI `12.2.0.0`；测试媒体容器如使用该仓库锁定的镜像，使用 `python@sha256:0a310eeecf4e1f5a0743f9a6520c90c88d089c903ca5fd283f501e3a805f5f89`。来源：[`README.md`](https://github.com/kentinya/jellyfin-strm-probe/blob/2afc6517fda5a7b4170ebea625619a552c47dabc/README.md)、[`scripts/package_plugin.py`](https://github.com/kentinya/jellyfin-strm-probe/blob/2afc6517fda5a7b4170ebea625619a552c47dabc/scripts/package_plugin.py)、[`dev/images.lock.json`](https://github.com/kentinya/jellyfin-strm-probe/blob/2afc6517fda5a7b4170ebea625619a552c47dabc/dev/images.lock.json)。
- **尚缺目标 OpenList 驱动／测试挂载身份。** 开始任何外部服务准备前，用户须提供目标存储驱动／提供方、专用测试挂载及可准备合成对象的方式、隔离 OpenList API 与稳定公开 `/d/` 地址。还须确认该无签名地址可从隔离 Jellyfin 网络实际访问。用户不应把秘密贴入对话或提交文件；专用测试凭据只可经进程环境注入。不得从生产服务或配置推断这些信息。
- 若目标驱动、专用测试身份／挂载、隔离网络路由或稳定无签名 `/d/` 前提未能取得，Task 留在 `PLANNED` 或停止在当前阶段并记录 `BLOCKED`／`NOT RUN`；不得用 Local、WebDAV、mock API、生产服务、未知来源包或仅构建结果替代。此时不启动外部联调，也不签发 PASS。

## Developer 实施范围

1. **先做准入核对。** 在创建容器或调用目标服务前，核对用户提供的目标驱动／挂载、专用凭据注入方式、隔离 OpenList、稳定 `/d/` 地址和隔离 Jellyfin 网络方案。所有状态、配置、媒体、插件源码与产物、容器标记和证据仅放在本仓库 `artifacts/` 下。只启动、读取和清理本 Task 明确创建的资源。
2. **固定并构建远程插件。** 将远程仓库克隆到 `artifacts/task-4.1/plugin-source/`，checkout 到上方完整 SHA，断言 `git rev-parse HEAD` 一致且 tracked worktree clean。按该 checkout 自带流程构建并打包；在插件源码目录运行：

   ```bash
   python3 scripts/package_plugin.py --configuration Release --runtime linux-x64 --output-dir artifacts/releases
   ```

   记录 clone URL、分支／SHA、SDK、精确命令／cwd、构建输入清单、归档 sidecar 和最终插件 ZIP SHA-256。源码副本不作编辑；任何构建错误都如实保留，不得通过修改依赖、工具链或插件源码悄悄改变输入。
3. **真实目标 OpenList 到 STRM。** 用固定 OpenList 镜像 `openlistteam/openlist@sha256:11b490123d57f88c84ecda46285e1921869cf511da3e5bb4062f183e7e1200b7`（源码 `3b28270a4066ea52943a69ecddd850d83f7e8bdc`，v4.2.7）及用户指定的专用目标驱动／测试挂载运行。用候选 Go CLI 验证专用 API 认证、list 分页、get、hash／size 和稳定 URL。目录至少有 201 个可列举对象以穿过产品每页 200 项的边界，并在第二页实际包含媒体项；媒体名覆盖普通名及中文、空格、`#`、`%`。每个媒体对象使用固定合成内容；核对生成 STRM 的逐段编码、URL 前缀和实际 HTTP 返回字节与源内容相同。所有列表 `refresh:false`；STRM 不得含 `raw_url`、签名查询、token、Cookie 或临时 URL。若目标存储不支持稳定无签名 `/d/`，停止该场景并记录合同前提不满足，不绕过鉴权。
4. **部署实际包并观察插件行为。** 使用上方固定 Jellyfin 镜像 digest，建立全新隔离 Jellyfin 配置／库／用户；仅部署 Task 构建出的插件 ZIP，核对 Jellyfin 已加载的插件版本和安装文件与归档 manifest 对应。把 CLI 生成的原始 STRM 加入媒体库，使用 Jellyfin 原生扫描／支持的原生播放入口及客户端验证解析、必要主媒体流、实际播放会话与进度。插件决策只能从 Jellyfin／插件正常日志、播放结果和脱敏的媒体请求计数等产品可观察行为判断；不调用插件私有 API、不读取插件 SQLite、不以源码静态阅读或测试桩代替。
5. **覆盖变化及恢复。** 记录首轮、同输入重复同步、URL 改变、同 URL 内容／hash 改变、hash 不可比而有效 Size 改变、路径移动，以及 Jellyfin 后续扫描／播放的结果。按合同覆盖稳定 URL、URL 改变、同 URL／同有效 SHA-1／不同 Size、不同 SHA-1、hash 不可比时有效 Size 改变的插件决策矩阵。禁止通过伪造目标 OpenList 响应、修改插件源码／数据库或未标注地编辑生成 STRM 来制造通过证据。若同 SHA-1／不同 Size 矩阵行不能由目标提供真实可观察输入触发，应保留该行 `BLOCKED`／`NOT RUN` 和原因，交 A 协调，不得降级验收或声称整项通过。
6. **必要时实现本仓库测试驱动及范围内修复。** 当前 Go 仓库没有目标 OpenList 到原生 Jellyfin 播放的联合驱动；可在本仓库新增专用隔离 runner 和 Python 测试，复用远程 checkout 中受支持的原生 Web UI 播放观测工具，但必须确保其证据脱敏。runner 使用动态资源名、硬超时、`finally` 清理且只清理本 Task 创建的容器／网络／文件。只可修复隔离实测确认、且落在当前既有需求／架构内的本仓库产品缺陷。缺陷若要求修改 V4.3 协议、插件代码、外部服务或本 Slice 合同，停止并交 A；不修改配套仓库工作树。

## 验收条件

| ID | 可核对结果 |
|---|---|
| AC-1 输入与隔离（RO-1） | OpenList 源码／镜像、目标驱动／测试挂载、插件远程源码 SHA／归档 SHA-256、Jellyfin 镜像／版本、媒体、配置摘要及工具身份完整可追溯；容器和文件均归本 Task 所有且位于 `artifacts/`。无生产访问、无 `latest`、无真实媒体；凭据、Cookie、签名 URL 和私有完整路径未进入文件、stdout、日志或证据。任一泄露或触碰生产即阻断。 |
| AC-2 目标 OpenList 与输出（RO-2） | 目标驱动通过专用测试身份认证；CLI 实际遍历超过一页目录并在目标页处理媒体，验证 get、路径编码、可用元数据；生成 STRM 只含稳定无签名 `/d/` URL 和有效合同字段；通过该 URL 实际取回字节并与合成源逐字节一致。mock／Local／WebDAV 不能满足此 AC。 |
| AC-3 远程插件来源和构建（RO-1／RO-3） | 源码副本 `HEAD` 等于 `2afc6517fda5a7b4170ebea625619a552c47dabc`、tracked worktree clean；用锁定源码自己的打包流程生成归档，产物 SHA-256 与 manifest 可核对，隔离 Jellyfin 安装并加载的就是该归档中的构建。旧预构建包或源码补丁不通过。 |
| AC-4 原始 STRM 解析及决定（RO-3） | Jellyfin 对候选 CLI 原始 STRM 解析成功并观察到必要主媒体流；在真实运行插件上覆盖矩阵中的每项变化及探测／重用结果。仅 Go 测试、单测、接口 mock、内部 API／DB 查询、播放页面加载或请求 `/d/` 均不通过。不能合法触发的行记录 BLOCKED／NOT RUN，整 Task 不得 PASS。 |
| AC-5 原生库扫描和播放（RO-4） | Jellyfin 原生库扫描实际加载目标 STRM；经原生客户端／支持的原生入口播放合成媒体，观察到正确内容身份、必要流、活动播放会话与时间／进度推进。扫描、插件观察和真实播放分别有证据；代理不运行、不验证且不阻断。 |
| AC-6 重复、更新、移动和恢复（RO-5） | 首轮和稳定重复同步符合新增／跳过与不改写预期；URL／媒体变化、移动后 STRM 与 Jellyfin 后续状态按合同收敛；测试失败或服务暂时不可用时保留安全状态，恢复后重跑得到预期闭环。层级结果不得互相推断。 |
| AC-7 T4、独立审查与证据（RO-6） | 候选本地产品执行匹配输入的全量 T4；真实服务／插件／原生播放保留原始且脱敏证据。B 独立审查完整 Task Base..Implementation Head，至少独立复现目标 OpenList → 生成 STRM → 隔离 Jellyfin 原生扫描与一次实际播放，并核对关键决策证据。输入变更按影响重跑；任何缺失场景如实留为 BLOCKED／NOT RUN，不得判 Task PASS。 |

## 必要测试

### Developer 开发／修正反馈

先核对活动 Task 身份并运行 `python3 scripts/check_governance.py`。新增 runner 时先执行其对应 Python 单测；产品变更执行聚焦 Go 测试、`python3 scripts/quality.py` 和目标隔离场景。不得在准入前启动外部服务或尝试生产凭据。

插件取源／打包命令固定为：

```bash
mkdir -p artifacts/task-4.1
git clone https://github.com/kentinya/jellyfin-strm-probe.git artifacts/task-4.1/plugin-source
git -C artifacts/task-4.1/plugin-source checkout --detach 2afc6517fda5a7b4170ebea625619a552c47dabc
test "$(git -C artifacts/task-4.1/plugin-source rev-parse HEAD)" = 2afc6517fda5a7b4170ebea625619a552c47dabc
test -z "$(git -C artifacts/task-4.1/plugin-source status --porcelain --untracked-files=no)"
```

然后在 `artifacts/task-4.1/plugin-source/` 运行上方 `package_plugin.py` 命令。若开发 runner，其准确调用形式和参数随实现记录；禁止虚构尚未存在的命令或把 upstream 独立集成脚本默认视作本合同证明。

### 最终检查点门槛

在 `artifacts/validation-evidence/task-4.1-final-<run-id>/` 记录完整 Go 源码 SHA、插件源 SHA、归档及已安装文件摘要、OpenList／Jellyfin／媒体镜像 digest、驱动／挂载身份、脱敏配置摘要、OS／Go／Python／Docker／Compose／.NET／浏览器版本、确切命令与 cwd、退出码、计数、耗时、每场景结果、首次失败及重跑。证据和所有服务状态只留在本仓库 `artifacts/`。

按候选实际变化执行并保留：

```bash
python3 scripts/check_governance.py
bash scripts/go-local mod download
bash scripts/go-local mod verify
python3 scripts/quality.py
CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 ./...
bash scripts/go-local test -count=1 ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 bash scripts/go-local build -trimpath -o artifacts/task-4.1/bin/strmgen-amd64 ./cmd/strmgen
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 bash scripts/go-local build -tags strmgen_testhooks -trimpath -o artifacts/task-4.1/bin/strmgen-testhooks-amd64 ./cmd/strmgen
python3 -m unittest discover -s tests -p 'test_*.py' -v
python3 scripts/run_isolated_sync_integration.py --binary artifacts/task-4.1/bin/strmgen-amd64 --fault-binary artifacts/task-4.1/bin/strmgen-testhooks-amd64 --scenario all --output-dir "$TASK_EVIDENCE/local-sync"
python3 scripts/run_serve_integration.py --binary artifacts/task-4.1/bin/strmgen-amd64 --fault-binary artifacts/task-4.1/bin/strmgen-testhooks-amd64 --output-dir "$TASK_EVIDENCE/serve"
docker buildx build --platform linux/amd64 --load --tag strmgen:task-4.1-amd64 .
python3 scripts/run_container_integration.py --image strmgen:task-4.1-amd64 --output-dir "$TASK_EVIDENCE/container"
```

若产品可执行输入和容器输入分别与 Slice 3 的已审查 T4 候选完全匹配，可逐项核实源码、依赖、命令、镜像、配置、夹具及产物身份后复用对应证据；任何不匹配项必须重跑，不因文档或报告标签推定可复用。新增 Python runner／测试必须对新输入执行单测。目标服务闭环另执行本 Task runner 的所有目标驱动、生成、插件及 Jellyfin 场景；全量本地测试不替代这些外部门槛。新 runner 的最终执行命令由实施产物确定并在完成报告中逐字列出。

### B 独立验证

B 审查完整 Task Base..Implementation Head 和上表 AC／RO 映射，确认远程 SHA／包／部署身份一致及实际输入证据。独立复现至少一条真实目标 OpenList list/get、发布 CLI 生成的原始 STRM、稳定 `/d/` 字节核验、隔离 Jellyfin 原生扫描与客户端实际播放；独立核对一次变化／探测或重用判定。核查秘密与私有路径扫描、资源标签及 `finally` 清理仅针对 Task 自有资源。不得访问 `/opt/jf-strm-probe`、现存 OpenList／Jellyfin／代理、生产配置或媒体；不得调用插件私有接口或读取其 SQLite。其他矩阵行可复用完全匹配的原始候选证据，但 B 不得以源码阅读替代行为证据。

### 证据与重跑

开发报告提供完整候选 SHA、未提交输入摘要、源码／镜像／产物身份、精确命令／cwd、退出码、耗时、数量、原始脱敏日志路径、首个失败及修复重跑、每项 AC／RO 状态、复用的历史证据身份，以及全部 BLOCKED／NOT RUN 原因。远程 OpenList／浏览器／Docker／镜像或凭据不可用均保留原始失败事实，不将环境 smoke 改写为 PASS。候选后任何可执行 Go、runner、配置、插件源 SHA、构建工具、镜像、媒体或挂载输入变化，按影响重跑；产品 Go 输入变化时完整 T4 重跑。证据仅本机可读时明确标注，不称其为远端证据。

## 非目标

- 不验证、不启动或配置 JellyfinProxy 或任何代理路径。
- 不连接、读取或修改运行中的／生产 OpenList、Jellyfin、代理、挂载、配置、凭据、媒体和用户状态；不触碰 `/opt/jf-strm-probe`。
- 不使用 Local／WebDAV／mock OpenList、手写解析器、未知来源插件包、插件私有 API／SQLite 或仅 HTTP GET 冒充目标驱动／原生播放证据。
- 不实现新调度、Mirror／清理、媒体版本状态、插件或代理功能；不修改 V4.3 方案、稳定协议、比较／删除／归属／锁边界，不为通过测试伪造元数据或编辑插件源码。
- 不做 ARM64 原生实机、10 万至 100 万规模测量或 registry 发布；这些不是本 Task 目标，也不据此降低 Slice 5 后续合同。

## Developer Completion Report

- Changed Files / Implemented：PENDING
- Tests and Results / Evidence：PENDING
- Decisions / Risks / Remaining Required Outcomes：PENDING
## Developer Completion Report

**Status: BLOCKED on the user-supplied target identity — not READY FOR B REVIEW.**

Per TASK 4.1 admission rules (lines 22–26, 30) the target OpenList driver/provider, dedicated
test mount, isolated API + stable unsigned `/d/` base, and Jellyfin-network reachability must be
provided by the user before any external service work. None were supplied this session
(`artifacts/validation-evidence/task-4.1-dev-20261019-043534/admission/target-driver-admission.txt`).
AGENTS/SLICE forbid inferring them from production config/services, so no external target
scenario was started. AC-2..AC-6 are BLOCKED and AC-7's real-service portion is NOT RUN; the
Task is deliberately left `PLANNED` for B to re-drive once the identity is available.

- Changed Files / Implemented: No product Go change. Executed only the admission-independent
  subset: (1) plugin source pinned at `2afc6517fda5a7b4170ebea625619a552c47dabc` into
  `artifacts/task-4.1/plugin-source/` (tracked worktree clean, tree
  `710c190d0643cca3bffa613c4989d32647c4b5b2`); (2) pinned .NET SDK `10.0.401` installed by the
  plugin's own verified tool; (3) plugin package built with the exact TASK command
  `python3 scripts/package_plugin.py --configuration Release --runtime linux-x64 --output-dir artifacts/releases`,
  archive SHA-256 `382d3ef106ed8534b06ab5bff614d826e602a29703dadddbe3ef2e9864bd06d8`, sidecar
  manifest records `sourceRevision` = the pinned SHA and `--verify` exits 0; (4) candidate amd64 +
  testhooks binaries built from HEAD.
- Tests and Results / Evidence: `artifacts/validation-evidence/task-4.1-dev-20261019-043534/`
  (local, Git-ignored). Governance PASS; `go vet` exit 0; gofmt clean (44 files); `quality.py`
  exit 0; `race` exit 0 (9 packages); Python regression 112 tests OK. Total test wall-clock ≈53s,
  under 30% of this round. Full T4 / real-service gates NOT RUN.
- Decisions / Risks / Remaining Required Outcomes: The Go product tree is byte-identical to the
  Slice 3 reviewed candidate `410fc0e0a8964046de4707d58c0c7505385c6002` (`go.mod`/`go.sum` blobs
  match; 0 product files differ), so its reviewed T4 evidence is reusable once an external target
  exists. Remaining: AC-2..AC-6 target OpenList→STRM→plugin→Jellyfin native playback; AC-7 full
  real-service evidence. Blocking input needed from the user: target driver/provider, dedicated
  test mount + synthetic-object method, isolated OpenList API, stable unsigned `/d/` base,
  Jellyfin-network reachability, and a process-env credential injection channel.
- Head SHA: `2531032c88182ab2078cf2d155724f80e7529e49` (no product change; TASK report checkpoint
  follows).

## Developer Blocked Note (2026-10-19)

No target OpenList driver/provider or dedicated test mount identity was provided, so per the
Task admission rule no external service, container, or target credential was started or inferred.
Evidence of the admission check and of the executable subset (plugin provenance/build, static
checks, local regression) is in `artifacts/validation-evidence/task-4.1-dev-20261019-043534/`.
This is not a product or review PASS; B should keep Task 4.1 `PLANNED` until the user supplies the
missing target identity, then re-drive it.

## B Review Result

```text
Reviewed: NOT SET
Reviewed Range: NOT SET
Decision: PENDING
AC-1..AC-7: PENDING
Slice Required Outcomes all satisfied: PENDING
Next: PENDING
```
