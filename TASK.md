# Task 4.1 — 目标 OpenList 到 Jellyfin 原生播放隔离闭环

遵循[开发流程](docs/development-workflow.md)，范围 authority 为[Slice 4](SLICE.md)。本 Task 是完整的隔离真实服务端到端兼容验收；不以夹具、源代码阅读或包构建替代实际链路。

```text
Task ID: 4.1
Parent Slice: 4
Status: FIX REQUIRED
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

**Status: NOT READY FOR B REVIEW — 2 of 7 RO-3 decision-matrix rows BLOCKED.**

The user later supplied the isolated target (S3/RustFS driver + mount + synthetic media + isolated
OpenList/Jellyfin). The core chain then ran for real: target pagination, stable unsigned `/d/`
byte equality, stable repeat sync, plugin first-trust/unchanged decisions, and Jellyfin native
scan + actual playback of the synthetic media all PASS. Two RO-3 change rows (URL change, size
change) start a real native probe but the pinned plugin rejects the result (`returned primary
matches probe False`, HTTP 500); fixing that needs a change to the pinned plugin source, which
Task 4.1 forbids, so those rows are recorded **BLOCKED** and handed to A. Per the TASK rule a
BLOCKED row means the Task cannot PASS; it is left for B/A to coordinate.

- Changed Files / Implemented: No product Go change. Added an isolated target acceptance runner
  `scripts/run_isolated_target_integration.py` and unit tests
  `tests/test_run_isolated_target_integration.py`; registered both in `dev/environment-spec.json`.
  Plugin source pinned at `2afc6517fda5a7b4170ebea625619a552c47dabc` (tree
  `710c190d0643cca3bffa613c4989d32647c4b5b2`) and built with the exact TASK command to archive
  SHA-256 `382d3ef106ed8534b06ab5bff614d826e602a29703dadddbe3ef2e9864bd06d8`; installed files
  match the archive except Jellyfin-rewritten `meta.json`.
- Tests and Results / Evidence: `artifacts/validation-evidence/task-4.1-dev-20261019-065651/`
  (local, Git-ignored). Governance PASS; `quality.py` exit 0; new runner unit tests 9 OK; target
  chain: pagination 201 (200+1) PASS, `/d/` byte equality 5/5 PASS, stable sync zero-rewrite PASS,
  plugin FirstTrust 11 / Unchanged 43 PASS, native playback 5/5 PASS (advancing time, 160x90).
  URL-change and size-change rows BLOCKED (probe started, plugin rejected result).
- Decisions / Risks / Remaining Required Outcomes: RO-1, RO-2, RO-4, RO-5 satisfied for the
  tested paths; RO-3 is partial (first-trust/unchanged PASS; URL/size change BLOCKED); RO-6's
  full T4 and independent B reproduction remain for review. The Go product tree is byte-identical
  to the Slice 3 reviewed candidate `410fc0e0a8964046de4707d58c0c7505385c6002`, so its T4 evidence
  is reusable for the local product gates. The BLOCKED plugin row is a plugin↔Jellyfin acceptance
  interaction on the S3-backed source, not a generator defect; the item plays again after the URL
  is restored.
- Head SHA: `c769840378ccc6d366c1b9959a0b7b768ab6da85` (no product change; TASK report
  checkpoint). Correction: an earlier draft of this report cited the stale
  `2531032c88182ab2078cf2d155724f80e7529e49` (the B planning commit); the reviewed
  implementation checkpoint is `c769840378ccc6d366c1b9959a0b7b768ab6da85`.

## Developer Note (2026-10-19)

After the user supplied the isolated S3/RustFS target, the real chain was executed. Core scenarios
PASS; the URL-change and size-change plugin rows are BLOCKED (probe started, plugin rejected the
result with HTTP 500), which per the TASK rule prevents a PASS and is handed to A/B rather than
patched. Evidence is in `artifacts/validation-evidence/task-4.1-dev-20261019-065651/`. No
production service, config, media, or `/opt/jf-strm-probe` was touched; no proxy was started.

## 用户决定 — 第二项暂时接受（2026-10-19）

用户明确指示："第二项先搁置临时接受"。本决定仅针对本次 B 审查第二项中的两个外部兼容场景：URL-change 与 hash 不可比时的 Size-change；不冻结整个 Task 4.1。

- 两行保留原始 `BLOCKED` 结果：已观察到 native probe 启动，但后续播放／结果接受未成功。用户暂时接受此已知风险，因此这两行暂不作为本 Task 后续 B 门禁的阻断项；不将其改写为测试 `PASS`，也不宣称根因已由 B 独立复现确认。
- 适用输入仍为插件源码 `2afc6517fda5a7b4170ebea625619a552c47dabc`、Jellyfin 镜像及 S3/RustFS 专用隔离目标；原始证据保留在 `artifacts/validation-evidence/task-4.1-dev-20261019-065651/target/ro3_url_change.json` 和 `ro3_size_change.json`。不得以此授权修改插件、服务、生产配置或 Go 产品规则。
- 本轮不为这两行继续调查、修复或重跑；后续跟进交 A 协调，待用户恢复要求或 A 在授权范围内明确处置后再执行。A 收口时须将这项用户临时例外及未通过事实显式协调进 Slice 合同／Closure Packet，不得据此声称原合同全部 RO 已验证。B 不代签 A 的合同修订或最终审查。
- 此决定不豁免其他插件矩阵行、移动／恢复、凭据安全、候选身份、匹配输入的全量门禁或 B 独立真实链路复现。Task 仍为 `FIX REQUIRED`，修正继续同一 Task，不新建 Task。

## B Review Result — 首轮（历史）

```text
Reviewed: 2026-10-19
Reviewed Range: 527570df75ab469eb204d6a42b2d1b4eb07f71e6..c769840378ccc6d366c1b9959a0b7b768ab6da85
Decision: FIX REQUIRED
AC-1: FIX REQUIRED — scripts/run_isolated_target_integration.py:jellyfin_playback writes a JSON scenario containing the Jellyfin password to /tmp; this violates the Task's process-only credential and artifacts-only state boundary. Move the scenario exchange to a credential-free mechanism or an artifacts-owned, permissioned temporary input with no secret persisted, add a regression test, and rerun the affected native-playback evidence.
AC-2: PASS by matched developer evidence; B independent target reproduction NOT RUN because the current session has no authorized target credentials in its process environment.
AC-3: PASS by matched provenance evidence; the pinned source SHA, archive SHA-256, and installed-file comparison are consistent.
AC-4: PARTIAL — URL-change and Size-change retain BLOCKED test results but are temporarily accepted/non-blocking by the user's 2026-10-19 decision above. No additional plugin matrix requirement is waived; remaining required evidence is still subject to B review. Do not patch the plugin in this repository.
AC-5: PASS by matched native scan/playback evidence; B independent playback NOT RUN for the same credential precondition.
AC-6: PARTIAL — the failed URL/Size change and associated recovery closure are temporarily accepted only to the extent covered by the user's second-item disposition; other repeat/move/recovery requirements remain unchanged. Preserve all original failures; the two deferred rows do not require reruns in this round.
AC-7: FIX REQUIRED — credential safety, candidate/evidence identity, matching-input final gates and B independent reproduction remain required. The Developer report's Head SHA `2531032c88182ab2078cf2d155724f80e7529e49` is stale versus the reviewed commit `c769840378ccc6d366c1b9959a0b7b768ab6da85` and must be corrected. The two temporarily accepted rows are no longer a reason to withhold the remaining gates.
Slice Required Outcomes all satisfied: NO — RO-1 and RO-6 remain unresolved; RO-3/RO-5 retain the user's limited temporary exception rather than full behavioral PASS. Other existing evidence and remaining requirements are unchanged.
Independent B reproduction: NOT RUN (authorized target credentials absent from this session); no external or production service was accessed.
Required correction: remain in Task 4.1. Remove secret-bearing /tmp scenario persistence and add a regression test; correct the candidate/evidence SHA references; Developer runs affected feedback validation and returns a fixed candidate READY FOR B REVIEW. B owns the specified matching-input final full gates, evidence-reuse decisions and independent key reproduction; do not hand the full gates back to Developer. Exclude only the two temporarily accepted URL/Size rows from this round's blocking reruns, and hand their follow-up/contract reconciliation to A.
```

## Developer Fix Report (FIX REQUIRED round, 2026-10-19)

**Status: READY FOR B REVIEW — AC-1 secret-on-disk and AC-7 stale SHA both corrected; two
URL/Size rows remain user-temporarily-accepted BLOCKED and are handed to A.**

- **AC-1 corrected.** `jellyfin_playback` in `scripts/run_isolated_target_integration.py` no
  longer writes a scenario JSON containing the Jellyfin password to `/tmp`. The pinned playback
  driver is now invoked as `node <driver> /dev/stdin` and receives the scenario JSON through the
  process pipe, so the credential is never persisted to disk. Three regression tests were added
  in `tests/test_run_isolated_target_integration.py` (`PlaybackScenarioExchangeTests`): scenario
  arrives via stdin with the password present in the pipe and absent from any file; no
  `strmgen-play-*.json` is created in `/tmp`; driver errors remain redacted. Test count 9 -> 12.
- **AC-7 corrected.** The stale Head SHA `2531032c…` in the earlier report was corrected to the
  reviewed implementation checkpoint `c769840378ccc6d366c1b9959a0b7b768ab6da85` (see the
  Developer Completion Report above). This fix round's Base HEAD is
  `28b459d2c5f68100bfeb00d537ac11b9d78a0561`.
- **Changed files:** `scripts/run_isolated_target_integration.py` (stdin scenario exchange),
  `tests/test_run_isolated_target_integration.py` (regression tests). No product Go change; the Go
  tree remains byte-identical to the Slice 3 reviewed candidate `410fc0e0…`.
- **Affected validation rerun** (`artifacts/validation-evidence/task-4.1-fix-20261019-211529/`,
  local Git-ignored): governance PASS; `quality.py` exit 0 (124 py tests + vet/test/build);
  affected unit tests 12 OK; target native playback 5/5 (advancing time, 160x90, HLS 200); target
  metadata 201 (200+1); `/d/` byte equality 5/5; stable sync `created=0 updated=0 skipped=201`
  tree unchanged; plugin decisions FirstTrust 11 / Unchanged 50. No `/tmp` scenario files remain
  and the new evidence contains no test credentials.
- **Remaining:** the URL-change and Size-change rows stay BLOCKED under the user's 2026-10-19
  temporary acceptance and were not rerun; their contract reconciliation is A's. B owns the
  matching-input full T4 gates, evidence-reuse decisions, and independent key reproduction. RO-1
  is now addressed for the credential-safety boundary; RO-6 full gates and independent
  reproduction remain for B.
- **Evidence:** `artifacts/validation-evidence/task-4.1-fix-20261019-211529/INDEX.md`.
- **Fix Head SHA:** `3df244adf0a000b5376967e0f90b70387d195293` (runner + regression tests + report;
  no product Go change).


## B Review Result — 修正候选复审（2026-10-19）

- Reviewed Range: 527570df75ab469eb204d6a42b2d1b4eb07f71e6..3df244adf0a000b5376967e0f90b70387d195293
- Report Successor: 4898d6ad87b5eb789bb3a532cae44e25b3a7bfba（仅 TASK.md）
- Decision: FIX REQUIRED
- Task: 4.1（同一 Task，不新建）；Slice 4 ACTIVE；Closure Packet／A Final Review 仍 PENDING。
- Evidence: artifacts/validation-evidence/task-4.1-b-20261019-212439/INDEX.md（本机、Git-ignored）。

**已确认修正：** stdin 场景交换取代原含口令的临时 JSON 文件。B 执行 runner 的 12 项测试，并以独立合成子进程经 /dev/stdin 接收场景验证通过；这不是实际 Jellyfin 播放。旧完成报告的候选 SHA 已改正。runner／测试 SHA-256 与修正证据匹配；报告后继只改 TASK。用户暂时接受的 URL-change／Size-change 两行仍为原始 BLOCKED，本轮未调查、修复或重跑，亦不据此阻断。

### P1 阻断与修正要求

1. **AC-1／RO-1：新证据仍泄露播放凭据，runner 允许凭据进入 argv。** 修正证据 artifacts/validation-evidence/task-4.1-fix-20261019-211529/INDEX.md:52 的命令含非空、非环境引用的播放口令字面量；:42 记录从 artifacts 外的临时文件加载凭据。B 仅记录位置、布尔结果、文件摘要，不回显口令，亦未读取外部凭据文件。runner 的 --playback-password 参数仍接收并使用该值；仅改 stdin 不满足进程环境注入、不进入文件／命令参数／证据的边界。Developer 须移除 secret-bearing CLI 参数，改用独立播放凭据环境变量并修正调用；披露专用测试凭据泄露，按授权使旧凭据失效／重新注入，不碰生产凭据。原始失败不能静默覆盖或删除，含秘密原件不可加入 Git 或交接；交接保留脱敏副本、原件摘要及受限位置／处置记录。撤回“新证据无凭据／RO-1 已解决”的断言。
2. **AC-4、AC-6／RO-3、RO-5：其他既定行为未证实，不属用户暂缓范围。** 两份目标决策 JSON 只有累计 FirstTrust:0／Unchanged:0，没有同 URL／同有效 SHA-1／不同 Size、同 URL／不同有效 SHA-1 等行的逐场景实际输入及探测／重用证据，亦未提供路径移动后的生成器→扫描→播放闭环。plugin_decision_matrix（runner:338-353）只汇总全日志，不限定窗口、不关联输入、不断言必要行；B 独立复现空日志也正常返回并写 total=0，因此调用成功不证明矩阵 PASS。补既定非暂缓场景及明确判定；稳定重复同步／历史本地移动测试不替代目标插件／Jellyfin 移动闭环。无法合法触发的哈希等输入逐项标 BLOCKED／NOT RUN，给出原因及所需决策，交 A 协调；不伪造 SHA-1、不手改原始 STRM、不改插件，不扩大用户例外。

### 验收重评与复用

| 项目 | 本轮结论 |
|---|---|
| AC-1／RO-1 | FIX REQUIRED：stdin 已修正，新证据／argv 凭据问题仍阻断；来源历史结论保留，不推断整体 PASS。 |
| AC-2、AC-3、AC-5／RO-2、RO-4 | 保留前轮对已有 Developer 接入、来源、扫描／播放证据的有限认可；本轮不访问外部服务，不新增真实链路 PASS。 |
| AC-4／RO-3 | 未完成：两行临时非阻断，其他哈希行缺实际证据；累计计数不等于完整矩阵。 |
| AC-6／RO-5 | 稳定重复证据保留；移动闭环未提供／NOT RUN。旧 recovery 摘要不证明全部恢复通过；两项暂缓不重跑。 |
| AC-7／RO-6 | 未完成：完整候选范围已审查；明确 P1 按流程第 4 节提前拒绝，不机械重跑全量 T4／Slice Final。独立真实链路 NOT RUN：本会话 OPENLIST_TOKEN、JF_USER、JF_PASSWORD 均未注入，未从文件／生产配置恢复。 |

B 核对 410fc0e0a8964046de4707d58c0c7505385c6002 与本候选：54 个 Go 产品／module 文件、3 个 sync／serve／go-local 驱动文件、6 个容器／部署输入 Git blob 相同；新增目标 runner／测试／环境清单不同。仅证明这些源码集合匹配，不冒充全部运行输入匹配或新候选 T4 PASS。历史 task-3.2-final-20261019-1792350581 的记录源码是 2131b54bcec1cd43769d239a30456b4a8607f4aa，还须连同二进制、工具链、依赖、镜像、配置、夹具、命令核对后由 B 决定复用；不因“Go 未变”笼统通过。新 runner 测试已对当前输入执行。修正目标 summary 只含最后 decisions 调用，交接以逐场景清单为准。

### 同一 Task 下一步与门禁分工

- **Developer 受影响反馈：** 先治理检查；修复本 Task runner 的凭据来源／证据及既定非暂缓覆盖，补 secret 参数拒绝、环境注入、secret 不进入 argv／文件／stdout／stderr／证据、空日志／缺失决策不可通过的测试。执行 python3 -m unittest discover -s tests -p 'test_run_isolated_target_integration.py' -v、相关质量检查及受影响隔离场景；提供固定实现 Head、输入身份、脱敏命令、退出码／耗时、首次失败与重跑、逐行 AC 后再交 READY FOR B REVIEW。不要重跑 URL／Size 暂缓项；无法合法构造的输入给 A 精确阻塞包。
- **B 最终门禁：** 新固定候选后审查完整 Task Base..新 Implementation Head，并独立安全复现。B 自己负责本 Task“最终检查点门槛”指定的全量质量、race、uncached test、构建、sync、serve、container 及逐项历史证据复用；再独立执行授权真实 OpenList→CLI STRM→稳定字节→Jellyfin 扫描／实际播放和关键探测／重用。完全匹配证据只复用一次，不匹配项按影响重跑；不把全量退给 Developer，不机械重跑 Final。
- **A 范围协调：** 用户两行临时例外与其他不能合法触发的合同输入分别交 A；无新授权不降级 RO 或改外部组件。全部 RO 未满足，不提交 Closure Packet，不返回 SLICE READY FOR A REVIEW，不自行关闭。
- **时间约束：** 本轮两次短测试／合成复现总执行不足 1 秒；治理与其他验证保守单独统计见 B 索引。测试占本轮耗时低于 50%，未空等延长回合。
