# Task 4.1 — 目标 OpenList 到 Jellyfin 原生播放最简验收

遵循 [开发流程](docs/development-workflow.md)；范围以 [当前 Slice 4](SLICE.md) 为准。
本计划落实用户 2026-10-19 的最简方案、真实内容 SHA-1 授权与三项暂缓，替代旧计划中
冲突的工具／整矩阵要求；继续同一 Task，不新增 Task 4.2，不代表 B／A 验收通过。

```text
Task ID: 4.1
Parent Slice: 4
Status: READY FOR B REVIEW
Task Base: 527570df75ab469eb204d6a42b2d1b4eb07f71e6
Contract Checkpoint: 87be914349db70929461d306e1d183c8e560cd68
Difficulty: High
Test Level: T4
Planner / Reviewer: B
```

## Goal

用既有专用隔离目标确认 OpenList→CLI 原始 STRM／稳定无签名 URL→固定插件→Jellyfin
原生扫描／实际播放；另用真实内容 SHA-1 的标注副本补两行插件决策。复用匹配证据，只补
必要缺口，不把验收辅助 runner 变成产品或通用框架。三项用户暂缓不计 PASS。

## 实际缺口与范围

- 已实施候选：83e5b1901e1c306d1f48fcde92786b353eb3d5e3。产品 Go 未变；已有目标
  runner；本轮测试治理前实际 36 项、精简后 31 项（历史报告为 34）。不是“尚缺目标／等待创建 runner”。既有部署只核对身份，不重搭。
- OpenList 源 3b28270a4066ea52943a69ecddd850d83f7e8bdc，镜像
  openlistteam/openlist@sha256:11b490123d57f88c84ecda46285e1921869cf511da3e5bb4062f183e7e1200b7，
  v4.2.7，专用隔离 S3/RustFS。Jellyfin 为
  jellyfin/jellyfin@sha256:357724bf0ae27a672c7cbaa899db2d9abeb13dbd8657ccce750258a4c059d037，12.2.0。
- 插件来源 https://github.com/kentinya/jellyfin-strm-probe.git，main 已锁定源码
  2afc6517fda5a7b4170ebea625619a552c47dabc。干净源码副本位于
  artifacts/task-4.1/plugin-source/；构建命令为该目录内
  `python3 scripts/package_plugin.py --configuration Release --runtime linux-x64 --output-dir artifacts/releases`，
  .NET 10.0.401；ZIP SHA-256 为
  382d3ef106ed8534b06ab5bff614d826e602a29703dadddbe3ef2e9864bd06d8。已有包沿用，不重选远程 HEAD。
- 已有发布／testhooks CLI SHA-256 分别为
  16b70e13ad9f91dc3c53f6d31240deeb2547d4f06534f91ec9338758ea20494e、
  c6222a14cca2ab66515ec11dceb8e277ccea4e67e6a3949cb887b5f4d0efeb1c。
  新候选必须重新核对实际身份，不能只凭文件名复用。
- 原始链路已有 201 项分页（200+1）、五类名称字节相等 5/5、重复同步零重写、原生播放
  5/5，以及移动生成／扫描／旧路径清理／恢复原树及恢复后播放的证据；B 独立真实复现尚未运行。
- 目标 get 没有可用 SHA-1；用户已允许验收侧计算自有本地合成媒体真实哈希，不再以此
  作为“无法构造输入”的范围阻塞。已有派生输入和 Go 比较通过，但实际插件两行仍 NOT RUN。
- 当前剩余：实际凭据出口／事件记录、失败夹具最小处置和准确报告、两行真实插件观察、
  B 独立一份媒体链路、完整 T4 复用裁定及 Base..Head 审查。本会话所需专用凭据未在进程
  环境注入；不从文件／生产配置恢复，不把未运行写成通过。

## 最简执行与实施边界

1. **核对／复用。** 固定新实现候选，先核对源码、CLI、包／安装文件、镜像、媒体、配置及
   命令身份。201 项分页、五类名称、首轮、重复零写和移动生成／清理／恢复可复用匹配原始证据。
2. **Developer 只修实际阻断。** 最小闭合已复现的转义凭据落盘与原始子进程 error 直出；
   可不记录原始异常／子进程诊断，只报固定安全错误类别与必要字段，补对应针对性回归。
   历史凭据回显／临时文件事件须脱敏记录处置，不能宣称从未泄露；不传播秘密。
   夹具失败时停止、准确标记未恢复；核对恢复或弃用重建均可，不要求自动补偿框架或逐阶段故障矩阵。
   仅 probe-start 的场景不得硬写 HTTP 500／共同根因，人工核对真实结果即可；撤回汇总函数
   “无关 item”反例对实际逐项播放证据的独立 P1，不再要求通用日志引擎。
3. **B 独立一份原始媒体链路。** 在专用进程环境凭据具备后，用真实 OpenList list/get、
   CLI 原始 STRM、稳定 URL 字节，正常扫描／播放一份已有合成媒体，核对必要主流、会话／进度
   及该 item／当次窗口的重用。可用既有命令或人工操作，不要求重跑五份媒体或整个 acceptance。
4. **B 补 SHA-1 两行。** 仅从 artifacts 内自有本地 A／B 实际内容计算 40 位 SHA-1，
   用明确标注的独立 STRM 副本；保留 CLI 原始输出。先通过正常扫描／播放建立同 URL 的 A
   有效历史指纹／必要主流，再保持 A 和哈希，只将 Size 元数据改为另一非负值，观察复用／不探测。
   该 Size 是刻意异常，不称真实大小。另使同 URL 实际提供不同内容 B，写 B 真实哈希／实际 Size，
   观察对应条目启动探测；记录结果接受／播放成败，不把启动当播放成功。若 B 用于播放须确认
   合法可播放，现有追加字节变体尚无播放证据。通过正常扫描／播放／日志，不碰插件私有 API／DB。
5. **B 收口。** 对固定候选审完整 Task Base..Head，逐项完成／复用下列全量门禁及 AC；
   只补缺失／变更输入。满足全部必验项后提交 Closure Packet，返回 SLICE READY FOR A REVIEW，
   不为非阻塞润色新造 Task，不代 A 关闭。有新范围问题交 A，不擅自豁免。

**三项暂缓：** URL-change、hash 不可比 Size-change、移动路径首次播放。保留原始 BLOCKED／失败，
本轮不调查、不修复、不重跑，非当前 B 阻断且不计 PASS；Closure Packet 单列授权与风险，后续交 A。
现有 target runner 的 `--scenario acceptance` 含 move，`--scenario all` 还含前两项，不使用它们
作为本轮一键门禁；已有 metadata／byte_equality／stable／playback 单场景或人工观察即可。
SHA-1 派生输入无现成联合命令，不虚构 runner 参数，也不为此扩建通用工具。

## 验收条件

| ID | 当前必验条件／证据 |
|---|---|
| AC-1（RO-1） | 固定输入、Task 自有隔离边界和纯进程环境凭据；实际泄露出口修正，事件脱敏处置可核对。历史接受交 A，不能假称从未泄露。 |
| AC-2（RO-2） | 目标 S3/RustFS 真实 list/get、201 项分页、五类编码、原始稳定无签名 STRM 与实际字节、重复零写；允许匹配证据复用。 |
| AC-3（RO-1／3） | 锁定源码／干净副本、构建归档和实际安装／加载身份对应，不以旧包或修改插件代替。 |
| AC-4（RO-3） | 原始 STRM 解析／必要主流／稳定重用，加两行真实 SHA-1 派生输入的实际插件复用／探测决策。Go 比较不替代；URL／Size 两项仅用户暂缓。 |
| AC-5（RO-4） | 原生扫描、正确内容／主流、会话／进度与对应 item 决策；B 独立一份原始合成媒体链路。GET、扫描或页面加载不等于播放。 |
| AC-6（RO-5） | 匹配首轮／重复和移动生成／扫描／清理／恢复证据；夹具失败停止并标记／恢复或弃用；移动首播暂缓，不要求整套重跑。 |
| AC-7（RO-6） | B 对固定候选完成全部 T4 门禁或逐项匹配复用、关键独立复现和完整 Task Base..Head 审查；报告真实未运行／暂缓，不交全量给 Developer。 |

## 必要测试

### Developer 开发／修正反馈

先执行 `python3 scripts/check_governance.py`。仅修本 Task runner／测试与准确报告，执行：

```bash
python3 -m unittest discover -s tests -p 'test_run_isolated_target_integration.py' -v
```

覆盖所选实际凭据出口与已复现绕过、失败停止／夹具标记及直接受影响逻辑；不要求通用秘密
识别、派生 token 全追踪、逐阶段补偿或全日志关联测试。真实场景只补该修正直接影响且不在
暂缓清单内的证据，不能安全执行则如实报未运行。若选 quality.py 已覆盖同套单测，不重复跑。
产品 Go 本轮不在计划修改内；真实发现产品缺陷须在既有范围内处理并明确受影响验证，范围外交 A。
Developer 提交固定实现 Head 与脱敏报告，READY FOR B REVIEW；不代跑 B 指定全部 Final。

### B 最终检查点门槛（完整目录，先匹配再执行）

下列是 B 必须逐项裁定的全部既有门禁，不是每轮全跑的命令清单。B 先对源码／依赖、工具链、
二进制、镜像、配置／夹具、命令及原始结果匹配；已覆盖项引用原始索引／摘要，缺失或不匹配项
才运行对应命令。quality.py 包含全 Python 单测及 gofmt／vet／Go test／build，不重复其已覆盖的
Python 聚焦／全量；race 与 uncached 不由普通 test 替代。产品 Go 输入变化须重跑相应完整 T4。

以下 cwd 均为仓库根；TASK_EVIDENCE 指向本仓库 artifacts/validation-evidence/task-4.1-final-<run-id>/。

```bash
python3 scripts/check_governance.py
bash scripts/go-local mod download
bash scripts/go-local mod verify
python3 scripts/quality.py
CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 ./...
bash scripts/go-local test -count=1 ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 bash scripts/go-local build -trimpath -o artifacts/task-4.1/bin/strmgen-amd64 ./cmd/strmgen
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 bash scripts/go-local build -tags strmgen_testhooks -trimpath -o artifacts/task-4.1/bin/strmgen-testhooks-amd64 ./cmd/strmgen
python3 scripts/run_isolated_sync_integration.py --binary artifacts/task-4.1/bin/strmgen-amd64 --fault-binary artifacts/task-4.1/bin/strmgen-testhooks-amd64 --scenario all --output-dir "$TASK_EVIDENCE/local-sync"
python3 scripts/run_serve_integration.py --binary artifacts/task-4.1/bin/strmgen-amd64 --fault-binary artifacts/task-4.1/bin/strmgen-testhooks-amd64 --output-dir "$TASK_EVIDENCE/serve"
docker buildx build --platform linux/amd64 --load --tag strmgen:task-4.1-amd64 .
python3 scripts/run_container_integration.py --image strmgen:task-4.1-amd64 --output-dir "$TASK_EVIDENCE/container"
```

本地 sync 的 all 是既有产品回归，不是暂缓的真实 target all，不能误删。Python 全量由 quality.py
覆盖，其独立等价命令为 `python3 -m unittest discover -s tests -p 'test_*.py' -v`。
目标链路按上方最简步骤验证，不默认 all／acceptance。只核对源码子集相同不够复用 T4。

### B 独立验证

B 自己负责一份真实链路、两行实际 SHA-1 插件观察、逐项全量门禁复用裁定和完整 Base..Head 审查。
可核对其他匹配原始行为证据，不因角色不同重复同输入 Final；Developer 实际逐项 item 重用证据
可作为索引核对，不强制新工具。未注入专用凭据则真实场景仍 NOT RUN，不从文件取凭据。

### 证据与重跑

每次固定候选保留源码／产物／镜像／工具／媒体／配置摘要、精确命令／cwd、退出码、数量、
耗时、逐 AC／RO 结果、首次失败／重跑和复用依据；本机证据标本机，不宣称远端可读。
真实目标、派生 SHA-1、本地回归及暂缓分层报告。纯文档后继只重跑治理／链接／diff 检查，
不重跑产品 Final。可执行输入变化按影响补跑；测试执行时间小于本轮总耗时 50%，不空等凑比例，
不因时间预算把未运行项判 PASS。

## 非目标

不扩建验证平台、通用秘密／日志／补偿框架，不为非阻塞润色造 Task；不改 V4.3 原文、产品
协议／比较／删除／锁／归属／SQLite 边界。不改插件源码或 /opt/jf-strm-probe，不调用其私有
API／DB，不接生产状态。生成器不下载媒体计算哈希；测试侧本地真实内容计算只按用户授权。
不运行代理；ARM64 实机、规模测量与 registry 发布不属本 Task，不据此撤销后续目标。

## Developer Completion Report（本轮第三修正，2026-10-19）

```text
State: READY FOR B REVIEW（Task 头部 Status 字段为唯一权威）
Implementation Head: a213e227b2e84675205110e7a51aee94a300acd9
本轮 Base（B 上轮已审 HEAD）: f828c7f008be4abcf6bef73009c4d8526b768256
证据索引: artifacts/validation-evidence/task-4.1-dev3-20261019-232352/INDEX.md（本机、Git 忽略）
```

只修 B 上一轮 P1 的三项与失败夹具最小处置；不改产品 Go、插件、V4.3 协议、比较规则、
删除／锁／归属／SQLite 边界，不扩建通用秘密／日志／补偿框架，不动暂缓三项。

### 1. AC-1／RO-1：凭据出口（B 复现的三条路径全部关闭）

| B 复现的出口 | 修正 | 针对性回归 |
|---|---|---|
| `assert_secret_free` 只比对原始值，JSON 转义后的同一凭据可写入证据 | 每个秘密按原始、JSON 转义、`repr` 转义、unicode 转义四种形态比对与剔除 | `test_recorder_refuses_json_escaped_secret` |
| 子进程 stdout 的 JSON `error` 被原样提升到顶层 `FAIL:` 行 | `jellyfin_playback` 在边界校验 payload 并 `scrub`；`Failure.__str__` 输出前统一脱敏 | `test_structured_driver_error_is_scrubbed_not_raised_raw`、`test_failure_str_scrubs_registered_secret` |
| 运行时派生 token（`AuthenticateByName` 返回的 `AccessToken`）不受保护 | 新增 `register_secret()`，派生凭据进入同一脱敏与拒写集合 | `test_derived_token_is_registered_and_scrubbed` |

附带：非 JSON／超时／非零退出改成分类别 `Failure`（不再静默或裸抛）；rclone 诊断改用
`scrub`；`main` 在失败时先写 `run_failure.json` 再非零退出；`Recorder.failure` 即使被
拒写也只写固定安全类别，不回显值。

### 2. AC-6／RO-5：移动失败必须恢复夹具并如实标记

`scenario_move` 现跟踪已完成步骤。移动成功后任何失败都会反向 `rclone_move` 复原远端对象，
`ro5_move_failure.json` 记录 `failed_step`、`recovery_attempted`、`remote_object_restored`，
恢复本身失败写入 `recovery_error` 且不被首次异常掩盖；配置一律在 `finally` 复原。
`scenario_size_change`／`scenario_url_change` 的复原失败也分别写入
`ro3_size_change_restore_failure`／`ro3_url_change_restore_failure`，不再静默。
回归：`MoveFailureRecoveryTests`（恢复成功／恢复失败两条）。

### 3. AC-4／RO-3：矩阵行必须绑定本次真实 item，且不硬写根因

- `scenario_native_playback` 返回本次扫描并播放的 `STRM 名 → Jellyfin item id` 映射；
  `scenario_decision_rows` 改为按该映射逐项要求窗口内属于**该 item** 的复用决策，
  不再以「窗口内 item 数量 ≥ 文件名数量」充数。空期望集、缺项、无关 item 全部失败。
  回归：`test_decision_rows_reject_unrelated_item`、`test_decision_rows_require_every_expected_item`、
  `test_decision_rows_reject_empty_expected_set`。
- 撤回「probe 启动 ⇒ HTTP 500／插件拒绝自身 probe 结果／同一外部原因」的推断。
  `ro3_decision_rows` 的 DEFERRED 行改为「本轮未重跑、原始观察仅为 probe 启动、根因未复核」；
  `ro5_move_closure.moved_playback`／`replay` 只记 `observed`（playback_error、probe_started、
  窗口条目数）与 `cause: NOT DETERMINED`。回归：`test_deferred_rows_do_not_assert_a_root_cause`。
  这两行仍**不是** PASS，仍交 A/B 判定。

### 4. 凭据事件（脱敏，不再声称「从未泄露」）

`credential-incident-record.md` 逐项列出历史暴露点与当前状态：证据索引内的播放口令字面量、
`/tmp/t41env.sh`（mode 644，仍存在，本轮未读取／未 source）、第二轮口令／JWT 回显的轮换声明
（**我未独立复核**）、两个已删临时凭据文件。说明为何本轮不能靠删除／轮换收口：轮换需要从
文件或容器恢复秘密，删除属未授权的破坏性清理。列为待 B／A 处置的开放项。

### 5. 验证（Developer 反馈层，产品全量仍归 B）

| 项 | 命令 | 结果 |
|---|---|---|
| 治理 | `python3 scripts/check_governance.py` | exit 0，PASS（仅合同一致性） |
| 静态 | `python3 -m py_compile` 两文件 | exit 0 |
| 受影响测试 | `python3 -m unittest discover -s tests -p 'test_run_isolated_target_integration.py' -v` | 45 项 OK，exit 0（原 31，新增 14） |
| B 反例回放 | `b-repro-recheck.py`（由 B 的 reproduce.py 派生，调用签名适配） | 5 项反例全部 `false`，见 JSON |
| 治理单测 | `python3 -m unittest discover -s tests -p 'test_governance.py'` | 14 项 OK |
| 空白 | `git diff --check` | exit 0 |

**未运行并如实记录：** 真实 OpenList／Jellyfin／播放链路、SHA-1 两行实际插件观察、URL-change／
Size-change／移动首播（三项用户暂缓）、Go 全量／race／构建、本地 sync／serve／容器回归。
本会话进程环境没有注入 `OPENLIST_TOKEN`／`JF_*`／`RUSTFS_*`；凭据只存在于容器环境与
`/tmp/t41env.sh`，从这两处取用违反「不从文件恢复秘密」，故真实场景保持 NOT RUN，
不以环境 smoke 或单元测试代替。

### 6. 固定输入身份（本轮实测，未重选版）

- CLI：`strmgen-amd64 16b70e13…`、`strmgen-testhooks-amd64 c6222a14…`（与 B 上轮核对一致）。
- 插件源码 `2afc6517fda5a7b4170ebea625619a552c47dabc`，tracked worktree clean；ZIP SHA-256
  `382d3ef1…`；OpenList `11b49012…`（v4.2.7）、Jellyfin `357724bf…`（12.2.0）、
  RustFS `9ca95595…` 三个隔离容器镜像与合同一致；合成媒体 201 项。

### 7. 风险与未决

- RO-3 两行仍 BLOCKED（目标无 SHA-1），两行 DEFERRED（用户暂缓）；移动首播 BLOCKED 且根因
  改为 NOT DETERMINED。均不因本轮视为通过。
- 凭据历史暴露点的轮换／删除未完成（见 incident record），仍需 B／A 处置。
- 本机证据，非远端可读；产品 T4 与独立真实复现仍由 B 执行。
- 测试执行合计 1.072 秒（py_compile 0.073 + 目标测试 0.193 + 治理单测 0.691 + 治理 0.115），
  本轮保守回合分母约 560 秒，占比约 0.19%，远低于 30%；无空等凑比例。

## Developer Completion Report（上一轮候选，历史事实）

- Implementation Head：83e5b1901e1c306d1f48fcde92786b353eb3d5e3；不是本轮新候选。
- 变更：目标 runner／测试与环境登记，产品 Go 未变。第二轮报告曾交 READY FOR B REVIEW，
  随后独立 B 仍判 FIX REQUIRED，当前状态以本计划与下方决定为准。
- 原始证据：artifacts/validation-evidence/task-4.1-fix2-20261019-215918/INDEX.md（本机）。
  元数据 201、字节 5/5、重复零写、原生播放 5/5；移动生成／扫描／清理／恢复证据保留，首播 BLOCKED。
- 凭据回显／临时文件事件及轮换为 Developer 报告，B 未独立确认全部处置，不保留“从未泄露”的断言。

## B Review Result（现存决定与本轮文档调整）

- Reviewed Range：527570df75ab469eb204d6a42b2d1b4eb07f71e6..83e5b1901e1c306d1f48fcde92786b353eb3d5e3。
- Decision：**FIX REQUIRED**，本轮没有新产品审查 PASS；继续 Task 4.1。
- B 证据：artifacts/validation-evidence/task-4.1-b3-20261019-223640/INDEX.md（本机）。
  quality.py 原始环境 PASS（148 Python 测试及 Go 检查）；最初 TMPDIR 覆盖导致两项夹具
  失败，恢复原环境通过，首次日志保留。独立合成反例不是实际插件／播放证据。
- 当前修正要求仅为上方最小范围：凭据实际出口／事件记录，夹具失败停止／准确标记和结果
  描述；撤回通用补偿／秘密／日志工具要求及汇总反例的独立 P1。三项用户暂缓不再阻断。
- SHA-1 夹具及 Go 比较证据：artifacts/validation-evidence/task-4.1-b-sha1-20261019-225444/。
  同哈希／Size 异常 skip+anomaly、不同哈希 update 已通过；实际插件两行仍 NOT RUN。
- B 独立真实链路 NOT RUN，完整 T4 逐项复用裁定尚未完成。全部必验尚未满足，Closure Packet／
  A Final Review PENDING，不返回 SLICE READY FOR A REVIEW，不自行关闭。

## 历史与检查点

详细原始计划、Developer 各轮报告、B 审查、首次失败和用户逐次决定完整保留于
`0f1e411bab74b9d15c2c6711910459cdb11488c1:TASK.md`；可用
`git show 0f1e411bab74b9d15c2c6711910459cdb11488c1:TASK.md` 核对。
本文件只保留当前可执行要求与事实索引，不继续叠加互相冲突的旧指令；历史 BLOCKED／FAIL 未改成 PASS。
当前 Contract Checkpoint 是用户授权最简合同的真实提交，Task Base／Slice Base 均未移动，A 未被代签。

## 本轮文档检查（2026-10-19）

治理检查（含必读文档链接、V4.3 原文摘要与已提交合同／Task SHA 一致性）PASS；
治理单测 14/14 PASS；git diff --check PASS。治理、单测与空白检查一组总计 0.549 秒，
仅从合同提交到检查完成的保守回合分母已为 35.784 秒，低于 50%；无空等。
本轮未改产品／测试工具代码，未执行产品全量或真实服务测试，未新增行为 PASS。

## 用户授权的测试代码治理（2026-10-19，B 实施）

先向用户说明三层测试功能，逐文件职责和删除／替代映射写入
[测试代码职责与治理](docs/test-matrix.md#测试代码职责与治理2026-10-19b)。
只改四份 Python 工具测试：删除 9 个无效／重复或已合并的方法（总数 148→139），
改强合法参数前提、独立目录摘要和日志窗口断言，不改产品、集成 runner、质量门禁或暂缓清单。
16 个 Go 产品测试文件、五个集成驱动与安全／失败恢复回归全部保留，历史失败证据不删。
本轮是用户明确授权的测试治理，不是为了消除失败取得 PASS；独立审查角色要求不变，
B 不对自己编写的测试修正自签独立 PASS，Task 4.1 仍 FIX REQUIRED。

治理验证：修改前 116 项集成工具单测通过；修改后全 Python 139/139 通过（含目标 runner
31 项），治理／文档链接／原文身份／补丁空白及全部 Python 语法检查通过。
证据：artifacts/validation-evidence/task-4.1-test-governance-20261019-230741/INDEX.md（本机）。
含基线的验证进程总耗时 3.189 秒，保守回合分母 125.242 秒，占 2.55%，低于 50%；无空等。
本轮未运行 Go 全量、race、Docker／真实 OpenList／Jellyfin 或播放，不签完整 T4／Task PASS。
