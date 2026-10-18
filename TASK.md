# Task 1.2 — 已拥有 STRM 内容更新与原子恢复

```text
Task ID: 1.2
Parent Slice: 1
Status: FIX REQUIRED
Task Base: 15d749b3f151e94a6fb5e9850e6e21d7101aa603
Contract Checkpoint: 951aaeeeca966e8ef79e8ac9500827ce27d0f035
Difficulty: High
Test Level: T4
Planner / Reviewer: B
```

范围 authority：已提交的 `951aaeeeca966e8ef79e8ac9500827ce27d0f035:SLICE.md`。Slice 1 仍 ACTIVE，Base 为 `f90f9560e55b07fd2d6a20e8bac43741c39176c4`；本 Task 不修改合同、Roadmap 或稳定需求。

## Goal

在 Task 1.1 的首次创建、可信归属、比较、互斥和恢复基础上，实现已拥有 STRM 的安全内容更新：当 URL、有效 SHA-1 或可比较 Size 表明需要变化时，同目录原子替换文件；更新失败、进程中断或目标竞态时保留可证明事实，重启后可安全重试或跳过，不覆盖未拥有文件，不新增媒体版本状态表。

## Task 1.1 PASS 基线

- B 已审查 `951aaeeeca966e8ef79e8ac9500827ce27d0f035..15d749b3f151e94a6fb5e9850e6e21d7101aa603`，F-1..F-9 修正均在同一 Task 内完成；Task 1.1 决定为 PASS。
- Task 1.1 独立复核：当前发布二进制 `go test ./...`、`vet`、构建和 hook 字符串检查通过；`fault_recovery` 当前源码聚焦重跑通过。权威完整 T4 证据在 `artifacts/validation-evidence/task-1.1-b-review-fix-20261018-02/`，包含 9/9 隔离场景、race 142 pass、Linux amd64／arm64 交叉构建和 3/3 故障恢复聚焦重跑。
- Task 1.1 的已拥有文件内容替换是明确剩余项；删除／移动清理、serve／定时、发布镜像、真实 OpenList／插件／Jellyfin／代理和性能仍不在本 Task。

## Scope

允许修改 `cmd/strmgen`、`internal/reconciler`、`internal/strm`、`internal/storage` 及其 Go 测试，必要的隔离夹具、CLI 驱动、文档和质量工具；`internal/securepath` 仅允许支持本 Task 的已绑定目录相对安全替换与对应测试。不得改动原方案、需求、Slice 合同、Roadmap 或固定 T4 等级。

### 1. 保留既有比较语义

1. URL 变化必须更新，即使 SHA-1／Size 相同。
2. URL 相同且两侧有效 SHA-1 不同，必须更新。
3. SHA-1 不可比时，两侧有效 Size 不同必须更新；Size 相同或信息不足不得虚构变化。
4. URL 相同且有效 SHA-1 相同，无论 Size 是否变化都跳过并记录异常；无变化不得改字节或 mtime。
5. 未归属既有目标、无效 STRM、符号链接、FIFO、目录或其他非普通文件仍是冲突；不能因为新内容看起来合法而自动认领。

### 2. 实现安全原子替换

1. 复用当前全局互斥、已绑定输出根和相对目录句柄；更新不使用未约束的绝对路径 `rename` 或可覆盖未拥有目标的通用写入。
2. 在目标同目录创建唯一临时文件，限制原始字节数、UTF-8 和协议长度；完整写入、同步文件、关闭后才进入发布步骤。
3. 发布前重新确认目标仍是已拥有的普通 STRM；目标变成陌生文件、符号链接、特殊文件或出现不可证明的竞态时保守失败，保留原目标或外部目标。
4. 使用目录句柄上的原子替换操作；任何临时写入、同步、关闭、替换或目录同步失败都返回非零，不登记错误成功轮，不删除旧文件作为补偿。
5. 成功替换后保留原 ownership 映射；更新不创建 URL／SHA-1／Size 独立版本表，也不改变 source identity 或 epoch 规则。
6. 临时文件残留只作为可观测恢复事实处理，不在重启时盲目删除未知文件；测试须检查目标内容、ownership、rounds、锁释放和可重试性。

### 3. 接入同步、dry-run 和报告

1. `handleFile` 的 `NeedsUpdate` 分支改为实际更新，并区分 `updated`、`skipped`、`conflict`、`errors`；更新失败必须保持非零和 `partial=true`。
2. 一个 source 只有遍历、更新、必要归属检查全部成功后才提交完整 round；部分更新保留已成功文件，但不得提交完整 round。
3. dry-run 复用相同比较和冲突判断，只报告预计更新，不写 STRM、数据库、WAL／SHM、输出目录或缺失计数。
4. 日志仅输出 source 序号、相对路径和脱敏错误类别，不输出 token、签名 URL、原始 API 错误或私有完整路径。

### 4. 覆盖崩溃与竞态闭环

1. 增加 Go／CLI 回归：临时写失败、文件同步失败、发布失败、目标在检查后出现或替换、SIGTERM／SIGKILL 发生在替换前后、归属／round 提交窗口中断。
2. 每个故障用例都检查旧 STRM 或新 STRM 的完整性、陌生目标不被覆盖、ownership 与 rounds 不提前伪造、锁可释放、下一轮能安全跳过／重试／保持冲突。
3. 生产二进制不得包含测试 hook；受控故障只允许使用明确 build tag 的 testhooks 二进制，发布二进制必须完成至少一次真实恢复重启。
4. 仍使用有界目录队列和 HTTP 响应，不引入全库媒体状态列表、每文件 goroutine 或远程版本锁。

## Acceptance Conditions

- [ ] **AC-1 URL 更新**：同一已拥有目标仅改变稳定 URL 时，原子生成新 STRM，字节／mtime 改变，ownership 保持，成功 round 提交。
- [ ] **AC-2 SHA-1／Size 更新**：有效 SHA-1 不同或 SHA-1 不可比且有效 Size 不同时更新；相同 SHA-1 的 Size 异常跳过并记录；信息不足不误判。
- [ ] **AC-3 无变化与陌生文件**：URL／SHA-1／Size 无变化时零写；未归属、无效、符号链接和特殊目标不覆盖、不认领、不删除。
- [ ] **AC-4 原子失败**：临时写入、同步、关闭、替换、目录同步任一失败都不留下半个目标；旧目标、ownership 和完整 round 状态符合恢复规则，重试可继续。
- [ ] **AC-5 发布竞态**：目标在检查后出现或变成不安全类型时，更新非零且保留外部字节；不写入陌生目标，不产生错误 ownership 或成功 round。
- [ ] **AC-6 中断恢复**：替换前、替换后但 round 提交前、ownership 操作窗口的 SIGTERM／SIGKILL 均有实测；重启按 STRM 和可信 ownership 得到确定的跳过、重试或冲突，不自动认领。
- [ ] **AC-7 dry-run**：已有归属、需更新、冲突和故障输入的预览均无文件、数据库、sidecar、目录和 round 副作用，且退出码／摘要准确。
- [ ] **AC-8 交付事实**：Go 回归、Python 回归、race、双架构交叉构建、发布二进制恢复、testhooks 故障场景和 diff check 有匹配原始证据；真实外部联调仍单独标记 NOT RUN。

## Required Tests

### 开发反馈

先运行受影响包，例如 `bash scripts/go-local test ./internal/reconciler ./internal/storage ./internal/strm`，再运行新增 CLI／故障夹具。不能用旧 `needs_update` 测试代替真实更新和恢复断言。

### 最终 T4 门槛

仓库根目录执行并记录新证据目录：

```bash
TASK12_RUN_DIR=artifacts/validation-evidence/task-1.2-fix-20261018-02
mkdir -p "$TASK12_RUN_DIR/bin"
python3 scripts/check_governance.py
bash scripts/go-local mod download
bash scripts/go-local mod verify
python3 scripts/quality.py
CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 -json ./...
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 bash scripts/go-local build -trimpath -o "$TASK12_RUN_DIR/bin/strmgen-amd64" ./cmd/strmgen
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 bash scripts/go-local build -trimpath -o "$TASK12_RUN_DIR/bin/strmgen-arm64" ./cmd/strmgen
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 bash scripts/go-local build -tags strmgen_testhooks -trimpath -o "$TASK12_RUN_DIR/bin/strmgen-testhooks-amd64" ./cmd/strmgen
python3 scripts/run_isolated_sync_integration.py --binary "$TASK12_RUN_DIR/bin/strmgen-amd64" --fault-binary "$TASK12_RUN_DIR/bin/strmgen-testhooks-amd64" --scenario all --output-dir "$TASK12_RUN_DIR/07-isolated"
git diff --check
```

以上 run-id 是下一次修正候选的示例；每次候选／重跑选新的目录，不覆盖已有证据或二进制。B 仅修正已被占用的输出路径，完整 T4 门槛与原 AC 均不降低。

`quality.py` 覆盖非 race 的 Go／Python 检查；testhooks 只用于受控故障，不是发布产物。CGO race、arm64 构建和真实 OpenList／插件／Jellyfin 运行分别报告，不互相替代。

### B 独立验证

独立复现 URL／SHA-1／Size 更新、无变化零写、dry-run，以及 F-10 的检查后清理竞态、F-11 的合法外部 STRM 中断恢复、F-13 的暂存对象替换。核对目标与全部残留文件的身份／字节、ownership、rounds、退出码、锁释放和发布二进制重启；检查 F-12 的逐项原始证据。可复用输入相同的完整 T4 证据，不用重复运行全部相同输入；不得用旧用例通过替代新窗口。

### 证据要求

每次运行使用新的 `task-1.2-<run-id>` 证据目录，记录源码 SHA、未提交输入、精确命令／cwd、工具链、fixture、退出码、计数、首次失败和重跑。任何候选修改源码、测试、依赖或运行配置后，按影响范围重跑完整 T4；NOT RUN／BLOCKED／SKIPPED 不得写成 PASS。

## Non-goals

- 不实现 Mirror 删除／移动旧路径清理、连续缺失确认、serve／定时、正式容器发布或性能承诺。
- 不运行 ffprobe，不下载媒体，不操作 Jellyfin／代理／插件内部接口或其数据库。
- 不新增媒体版本状态机、远程版本锁、全库对象快照或独立 URL／SHA-1／Size 表。
- 不把合成 OpenList、交叉编译或 testhooks 证据写成真实 OpenList、插件解析、Jellyfin 播放或 ARM64 实机通过。

## B Review Result — Task 1.1 PASS (2026-10-18)

```text
Reviewed: 951aaeeeca966e8ef79e8ac9500827ce27d0f035..15d749b3f151e94a6fb5e9850e6e21d7101aa603
Decision: PASS
Slice Required Outcomes all satisfied: NO
Next: DEVELOPER IMPLEMENTS TASK 1.2
```

- F-1..F-9 均完成并有匹配回归；F-9 使用仅 `strmgen_testhooks` 的确定性 `afterOwnershipRecord` 屏障，发布二进制不含 `STRMGEN_TEST*` 环境开关。
- B 独立复核通过：当前源码 `go test ./...`、`vet`、发布／testhooks 构建和发布字符串检查 PASS；`fault_recovery` 独立重跑 PASS。可复用的权威完整 T4 证据为 `artifacts/validation-evidence/task-1.1-b-review-fix-20261018-02/`（9/9、race 142、双架构交叉构建、故障恢复 3/3）。
- Slice RO 复核：RO-1、RO-2、RO-3、RO-5 的实现闭环已满足；RO-4 仍缺已拥有 STRM 的内容替换与其原子恢复证据，因此规划 Task 1.2。真实外部 OpenList／无签名 `/d/`、固定插件解析、Jellyfin／代理、ARM64 实机和性能继续单独标记 NOT RUN。
- B 修正了上一 Task 固定最终命令中遗漏的 `--fault-binary` 与 testhooks 构建步骤；该命令事实已在本 Task 的 T4 模板中固定。

Task 1.1 的详细实施计划、F-1..F-9 修正记录和首次失败证据保留在可达 Git 提交 `15d749b3f151e94a6fb5e9850e6e21d7101aa603` 及本机 evidence 目录；不在当前 Task 重复复制历史流水账。

## Developer Completion Report — 2026-10-18

本节保留送审时的候选报告；当前状态与事实纠正以末尾 B 独立审查为准。

- **Status：** READY FOR B REVIEW（交 B 独立审查；本报告不是 B PASS，也不表示 Slice 完成）。
- **Implementation Head：** `6eb4db67837ca8d3a65e1ce23bac6ea13b2592b9`（父提交 `6d683e85f4b1681dbe17a99af12c11ffa544a63a`）。只改实现、测试与隔离驱动；未改合同、Roadmap 或稳定需求。
- **Changed Files：** `cmd/strmgen/main.go`；`internal/reconciler/{fs.go,fs_test.go,syncer.go,syncer_test.go}`；`internal/securepath/securepath_linux.go`；`internal/strm/strm.go`；新增 `internal/reconciler/{fs_update_hook_default.go,fs_update_hook_integration.go,fs_update_after_hook_default.go,fs_update_after_hook_integration.go}`；`scripts/run_isolated_sync_integration.py`；`tests/test_run_isolated_sync_integration.py`。`go.mod`／`go.sum` 未变。
- **Implemented：** 移除仅报告不写入的 `NeedsUpdate` 分支，`handleFile` 在 `Update` 决策时真正原子替换已拥有 STRM。`LocalFS.Update` 在同目录临时文件写入／`fsync`／关闭后，用 `securepath.Dir.Exchange`（`renameat2` `RENAME_EXCHANGE`）交换目标与临时文件，再逐字节校验被换出的旧目标等于更新前读取的已拥有字节；不等则交换回去并返回 `ErrTargetChanged`，保留外部字节、绝不 `unlink` 旧目标作为补偿。发布前重新确认目标是已拥有普通文件（符号链接／特殊文件／缺失均保守失败）。成功替换后删除旧内容并 `fsync` 目录，保留原 ownership 映射，不新建任何版本表。`handleFile` 区分 `updated`／`skipped`／`conflict`／`errors`，更新失败非零且 `partial=true`；一个 source 仅当遍历、更新、归属全部成功才提交完整 round。dry-run 复用同一比较和冲突判断，只计 `updated` 预览，不写文件、数据库、sidecar、目录或轮次。报告新增 `updated=`，移除 `needs_update=`。受控故障与恢复只经 `strmgen_testhooks` 构建（`STRMGEN_TEST_UPDATE_*`），发布二进制不含任何 `STRMGEN_TEST` 字符串。
- **Tests and Results / Evidence：** 本机权威证据目录 `artifacts/validation-evidence/task-1.2-final/`（非远端可读），源码 SHA `6eb4db67837ca8d3a65e1ce23bac6ea13b2592b9`，构建自干净已提交 HEAD。治理 PASS；`mod download`／`mod verify` PASS；`quality.py`（gofmt／vet／Go test／build＋Python 47 项回归）PASS；`-race -count=1 ./...` PASS（test-level pass=155、fail=0、skip=0）；Linux amd64／arm64 CGO=0 `-trimpath` 与 testhooks amd64 构建 PASS（SHA-256 amd64 `3f4f6d12…`、arm64 `90ae5ac1…`、testhooks `c097434c…`）；发布二进制 `STRMGEN_TEST` 计数 0，testhooks 计数 5；`run_isolated_sync_integration.py --scenario all` 9/9 场景 PASS；`git diff --check` PASS；提交后治理与 Python 回归 PASS。
- **新增／加强测试：** Go 层 `TestURLChangeUpdatesOwnedFileAndCommits`、`TestSHAChangeUpdatesOnlyAffectedFile`、`TestIncomparableSizeChangeUpdates`、`TestUpdateFailurePreservesTargetAndRound`、`TestTargetReplacedBeforeExchangeIsPreserved`、`TestDryRunReportsUpdateWithoutWriting`，以及 `fs_test.go` 的原子替换、外部替换保留、符号链接／FIFO／超长拒绝、写入／同步／关闭／交换各阶段失败保留目标并清理临时文件。隔离驱动新增：真实 URL／SHA-1／不可比 Size 更新、dry-run 更新预览后真实应用、更新临时写入失败与重试、更新后 `SIGKILL` 由发布二进制重启完成原 epoch、交换前外部替换竞态保留外部字节。
- **首次失败与修正：** 首轮 `fault_recovery` 的“更新写入失败”子用例误用了新的 API server，导致 source identity 变化被报为 `conflicts=1`（`errors=0`）。定位为测试夹具错误而非产品缺陷；改为复用同一 server、仅改 `public_url`（identity 不含 public URL），修正后 `errors=1`、旧目标保留、重试成功。另将 `Update` 中交换回滚失败的分支改为保留 `tmpName` 外部字节、不删除，避免回滚失败时误删外部内容；两处均在最终 T4 全量重跑通过。
- **Decisions / Risks / Remaining Required Outcomes：** 使用 `RENAME_EXCHANGE`（已在本环境实证支持）保证替换期间目标永不缺失，并在交换后校验旧目标以检测外部替换；这是本 Task 的关键安全决策。`ErrTargetChanged` 与“交换回滚失败”都保持保守失败、不登记成功轮、不伪造 ownership。RO-4 的已拥有文件内容替换与原子恢复证据本 Task 已提供，最终是否满足由 B 复核。真实 OpenList／无签名 `/d/`、固定插件解析、Jellyfin／代理、ARM64 实机与性能仍 **NOT RUN**；夹具与交叉编译证据不替代外部兼容／播放证据。Mirror 清理、serve／定时、发布镜像仍属后续 Slice。
- **Head SHA：** `6eb4db67837ca8d3a65e1ce23bac6ea13b2592b9`。

## B Review Result — Task 1.2 (2026-10-18)

```text
Reviewed: 15d749b3f151e94a6fb5e9850e6e21d7101aa603..6eb4db67837ca8d3a65e1ce23bac6ea13b2592b9
Report Checkpoint: e38ddd32c32419b7e97c238835fcf155c4efc6f8
Decision: FIX REQUIRED
Slice Required Outcomes all satisfied: NO
Next: DEVELOPER FIXES TASK 1.2 IN PLACE
```

### 审查与证据

- 审查完整 Task Base..Implementation Head，以及后续报告提交；报告提交只改变文档，未改变可执行输入。治理 PASS，初始工作区干净。
- 独立执行 `bash scripts/go-local test -count=1 ./internal/reconciler ./internal/storage ./internal/strm ./internal/securepath ./cmd/strmgen` PASS，发布／testhooks amd64 构建 PASS；隔离 CLI 的 `repeat_and_compare`、`dry_run_boundaries`、`fault_recovery` 3/3 PASS。原始日志在 `artifacts/validation-evidence/task-1.2-b-review-20261018-02/`。
- 核对原 `task-1.2-final/` 的源码、go.sum、API 夹具与驱动身份和原始 quality／race／双架构构建／9场景证据，未覆盖旧文件。race JSON 实际为 **147 个测试通过、8 个包通过、0 fail、0 skip**；原报告的“155 test-level pass”混计了包事件，修正报告时须分开计数。B 本轮未重新运行 race 或 arm64 构建；旧证据的通过事实可复核，但不能抵消以下新增失败。
- 新失败证据在 `artifacts/validation-evidence/task-1.2-b-review-20261018-03/index.json`，复现驱动为同目录 `run_review.py`。通过 Go overlay 仅在首次成功 `Exchange` 后、`verifyExchanged` 前加入可暂停／显式放行屏障；校验、回滚、清理及持久化算法不变。外部驱动在暂停时杀进程或替换目标，重启使用无 overlay 的发布二进制。这是受控时序复现，不能声称未插桩发布二进制独立命中了该极短窗口。
- 两次 B 夹具准备失败均保留：`task-1.2-b-review-20261018-01/initial-run-failure.txt` 记录文档后继构建的 VCS 元数据导致二进制摘要不同；`task-1.2-b-review-20261018-02/instrumentation-first-failure.txt` 记录仅发送 SIGSTOP 不能保证发送线程立即停止，后改为显式放行屏障。它们不是产品失败，也未被写成通过。

### F-10 / P1：回滚清理删除第二个外部目标（AC-5／R-06／R-07）

位置：`internal/reconciler/fs.go:292`、`:317`、`:323`。`verifyExchanged` 失败后无条件再次交换两个名字；`removeTemp` 仍为 true，defer 删除此时临时名字下的对象，未证明它仍是生成器可删除的文件。

确定性结果：首次交换把外部 A 留在 tmp；暂停后由外部驱动将目标替换为外部 B；继续执行回滚后，目标变为外部 A，外部 B 被换入 tmp 并删除。CLI 返回 1、round 留在 1，但输出只剩外部 A，外部 B 的字节丢失。错误退出不能代替陌生文件保护。

必须在同一 Task 内修正：

1. 先将上述双替换序列加入受版本控制的 Go／CLI 回归，断言外部 B、外部 A 均不得被清理；核对所有残留路径，而非只检查最终 `A.strm`。
2. 将“临时名字”与“当前该名字对应的对象”分开处理。回滚、显式 remove 和 defer 都要有可靠的对象归属证据；不能仅凭随机临时名或 bool 删除交换后的未知对象。
3. 当前目标已不是本次发布对象时，不得无条件把它交换走再清理。无法证明安全时保留目标与不明残留、返回可辨识的脱敏冲突／恢复错误，不提交完整 round。
4. 给出每次可能覆盖／删除操作的安全依据；只在现有检查后再补一次 Lstat 并不能证明检查与操作之间没有竞态。继续复用目录句柄／根绑定与全局互斥，不增加远端锁或媒体状态机。

### F-11 / P1：交换校验前中断后错误提交成功轮（AC-5／AC-6／R-06／R-07）

位置：`internal/reconciler/fs.go:317`、`:323`，`internal/reconciler/syncer.go` 的已拥有目标跳过／完整 round 提交路径。交换先于校验；在校验发现陌生对象前终止进程，磁盘上新 STRM 已占据目标路径，陌生对象仅保留在隐藏 tmp，ownership 映射没有反映这次未完成的判断。

确定性结果：暂停在交换后、校验前，目标为新 STRM、tmp 为 `external`，SIGKILL 后用发布二进制重启，得到 `exit=0, skipped=1, conflicts=0, partial=false`，round 从 1 升到 2，陌生原文件仍留在 tmp。这不是可直接忽略的“未引用临时文件”：它包含被置换的外部目标。

必须在同一 Task 内修正：

1. 在“交换前”“交换后／校验前”“校验完成／round 前”分别设置确定性 testhooks 屏障，新增发布二进制重启断言；不得只保留目前删除旧 tmp 后的 `afterUpdatePublish` 场景。
2. 修正发布／恢复协议，使校验未完成的中断不能在下一轮只凭目标 STRM 和旧路径 ownership 静默成功；陌生目标与不明残留应保持可识别的保守失败／冲突，不能自动认领或盲删。
3. 区分可证明的旧自有文件残留与可能含外部字节的残留；验证普通更新中断仍可安全重试／跳过，不能为修正这一窗口破坏 Task 1.1 的孤儿文件保护和正常重复运行。
4. 先说明方案如何同时满足原子更新、跨进程终止、可靠归属与“不新增媒体版本表”。若需要改变合同／归属或恢复语义才能实现，先交 A 判定并提交合同修订，再由 B 判断当前 Task 的合法调整；不得自行降低不变量。当前未改合同，也未认定合同不可满足。

### F-12 / P1：既定故障门槛未覆盖完整（AC-4／AC-6／AC-8）

`fs_test.go` 的失败表覆盖临时文件 sync／close 和交换前返回错误，CLI 覆盖写入失败及清理旧 tmp 后的 SIGKILL；未提供目录 fsync 失败、实际替换操作失败／回滚失败、已拥有文件更新路径替换前后的 SIGTERM／替换前 SIGKILL 的匹配证据。旧创建路径的取消／归属中断不能替代新增更新路径。现有 `afterUpdatePublish` 位于目录 `Sync` 前，不能把屏障出现称为目录项已持久化。

按以下顺序补齐，保持原 AC 和 T4：

1. 用矩阵列出：临时写入、文件 sync、close、发布 syscall、目录 sync、回滚失败，以及替换前／交换校验前／替换后 round 前的取消与 SIGKILL；逐项指向真实测试和原始结果。已有通过项可以复用，不伪报未执行项。
2. Go 层注入文件／目录同步与发布／回滚操作的真实失败返回，区分“操作尚未发生”与“已发布但 durability／round 未完成”；测试目标完整性、陌生文件保留、残留、ownership、round 与重试，不只断言 error 非空。
3. CLI 层对新增更新窗口分别测 SIGTERM／SIGKILL；SIGTERM 的暂停屏障须能让 context 取消被处理，不能让测试永久停在 SIGSTOP。每例都由发布二进制验证重启、锁释放、退出码、目标和数据库事实。
4. 聚焦命令：`bash scripts/go-local test -count=1 ./internal/reconciler ./internal/storage ./internal/strm ./internal/securepath ./cmd/strmgen`；扩展既有 `fault_recovery` 后，以修正候选的发布／testhooks 二进制运行 `--scenario fault_recovery` 并验证屏障确定性，再执行上方完整 T4。所有产品回归与 hook 必须提交；本机 B overlay 仅作复现依据。
5. 提交实现检查点后填真实完整 Head，再交 READY FOR B REVIEW；报告按测试／包事件分开计数，记录所有首次失败和重跑，不覆盖现有 `task-1.2-final` 与 B 证据。

### Slice RO 复核与下一步

| Required Outcome | 当前判断 | 剩余内容 |
|---|---|---|
| RO-1 | 已有入口／摘要能力；本 Task 未 PASS | 本地竞态／恢复错误的明确反馈随 F-10／F-11 复核 |
| RO-2 | 既有夹具接入与 URL／元数据能力保留 | 合同要求的真实 OpenList／无签名 `/d/` 实证仍 NOT RUN |
| RO-3 | 既有镜像／过滤／冲突与路径边界能力保留 | 目标被本地替换后的保护须完成 F-10／F-11 |
| RO-4 | 未满足 | 原子更新的陌生文件保护、中断恢复及 F-12；固定插件解析仍 NOT RUN |
| RO-5 | 未满足 | 原更新窗口的故障／取消反馈与可重复恢复证据须补齐；既有 dry-run／有界遍历／互斥保留 |

本轮不规划新 Task、不生成 Closure Packet、不交 A 最终审查。Slice 1 保持 ACTIVE，Task 1.2 Base／Contract Checkpoint／High／T4 不变，原地修正。真实 OpenList、固定插件解析仍是 Slice Final 不能被合成服务替代的证据；Jellyfin／代理、ARM64 实机和性能另报 NOT RUN，不因本次审查扩大实施范围。

## Developer Fix Report — Task 1.2 F-10/F-11/F-12 (2026-10-18)

本节保留送审时的修正候选报告;当前状态与事实纠正以 B 独立审查为准。

- **Status:** READY FOR B REVIEW(交 B 独立审查;本报告不是 B PASS,也不表示 Slice 完成)。
- **Implementation Head:** `f4383f2f59adedadc9252e465c64580570a3bbcf`(父提交 `003776300d24e480ab630f06806d1d3d54c65048`)。只改实现、测试与隔离驱动;未改合同、Roadmap 或稳定需求。`go.mod`/`go.sum`、SQLite schema 与 ownership 语义未变。
- **F-10 修正(回滚不再删除第二个外部目标):** `Update` 的交换回滚改为可证明安全。`undoExchange` 仅在当前目标同时满足设备/inode 身份与逐字节内容等于本次发布对象时才交换回去;删除临时名字前再做同样的字节+身份双重校验。因此“校验失败后目标被再次替换为外部 B”时,回滚不移动该目标,也不删除任何非本次发布对象;外部 A 留在暂存名、外部 B 留在目标名,两者字节都保留,返回非零且不提交完整 round。inode 复用不会再造成误删。
- **F-11 修正(校验前中断不再静默成功):** 更新暂存文件使用确定性隐藏名 `updateStagingName(base)`。`handleFile` 在内容比较前调用新的 `FileSystem.Recover`,按“仅删除与本轮 desired 逐字节相等的自有输出”为证明恢复:目标为 desired 且暂存为旧/陌生内容时,把暂存内容放回目标、只删除 proof 为 desired 的副本,再由正常比较决定重做或冲突;暂存等于 desired 时只删除自有暂存;两者都不等于 desired 时保留全部并报保守冲突。dry-run 只检测不修改。正常更新中断因此可重试(重做更新),被置换的外部目标恢复后按冲突处理,不再出现 `skipped=1` 且提交 round 的静默成功。
- **F-12 修正(故障矩阵补齐):** `LocalFS` 增加仅测试可注入的 `writeTemp`/`exchange`/`remove`/`syncDir` 与既有 `syncTemp`/`closeTemp`/`beforeExchange`/`afterExchange`;Go 回归覆盖临时写入、文件 sync、close、发布交换、目录 sync、回滚交换失败,以及交换后中断、回滚二次替换、恢复成功/外部恢复/歧义/未归属/dry-run 各状态。testhooks 增加 `afterUpdateExchange` 与可轮询 gate(供 SIGTERM 让 context 取消运行)+ SIGSTOP hold(供 SIGKILL);隔离驱动新增 `pre-exchange`、`exchange`、`after-publish` 三个窗口的 SIGKILL/SIGTERM 用例,均由无插桩发布二进制重启验证目标、ownership、rounds、残留、退出码与可重试性。
- **关键决策:** 确定性暂存名 + 字节证明恢复,使“交换先于校验”的中断可被下一轮识别并保守处理,同时不新增任何媒体版本表或恢复专用表;`RENAME_EXCHANGE` 仍保证替换期间目标永不缺失,回滚只在双重证明下进行。
- **Tests / Evidence:** 本机证据目录 `artifacts/validation-evidence/task-1.2-fix-20261018-01/`(非远端可读;`index.json`、逐命令日志、race JSON、隔离输出、双架构与 testhooks SHA-256);证据构建自干净已提交 worktree `7c9561905c0750dab459e360e11a9d772b59dfa7`,其可执行输入与最终文档 HEAD 完全一致(仅后继 TASK.md 文本变化,治理已在最终 HEAD 重跑 PASS)。`check_governance.py`、`mod download`/`mod verify`、`quality.py`、`-race -count=1 ./...`(test-level pass=160、fail=0、skip=0;8 个包通过)、Linux amd64/arm64 与 testhooks 构建、`run_isolated_sync_integration.py --scenario all` 9/9、`git diff --check` 全部 exit=0。发布二进制 `STRMGEN_TEST` 计数 0,testhooks 行计数 9。
- **Risks / Remaining:** 真实 OpenList/无签名 `/d/`、固定插件解析、Jellyfin/代理、ARM64 实机与性能仍 **NOT RUN**;合成夹具、交叉编译与 testhooks 不替代外部证据。Mirror 清理、serve/定时、发布镜像仍属后续 Slice。`./...` 门禁在含 B 未跟踪 `artifacts/validation-evidence/*/overlay-inputs` 的工作区会把 B 的评审 overlay 当作包编译;最终 T4 在 `artifacts/` 被 gitignore 的干净 worktree 中执行,细节见证据索引。

## B Re-review Result — Task 1.2 修正候选（2026-10-18）

```text
Reviewed: 15d749b3f151e94a6fb5e9850e6e21d7101aa603..f4383f2f59adedadc9252e465c64580570a3bbcf
Report Checkpoint: 74119a0e339a8b700768a3f01f6944e45a114603
Decision: FIX REQUIRED
Slice Required Outcomes all satisfied: NO
Next: DEVELOPER FIXES TASK 1.2 IN PLACE
```

### 独立验证与证据边界

- 审查完整 Task Base..Head 及报告后继；合同变化仅为 Implementation Head 指针，RO／边界未变。初始工作区干净，常规治理 PASS。
- 新证据目录：`artifacts/validation-evidence/task-1.2-b-rereview-20261018-01/`。`index.json` 索引命令、源码／输入身份、退出码、产物、首次失败与重跑；`run_review.py` 和 `reproductions.json` 保留四项 CLI 复现。均使用专用临时输出与自建合成 API；没有访问生产服务。
- 独立聚焦 Go 五包 PASS，当前发布／testhooks amd64 构建 PASS；当前 `repeat_and_compare`、`dry_run_boundaries`、`fault_recovery` 均最终 PASS。首轮 B 同时启动两个 CLI 驱动，固定全局锁使前两项返回 busy；原始失败保留在 `05-isolated/`，待另一驱动结束后顺序重跑为 `07-repeat-dry-retry/`。这是 B 调度冲突，不计产品缺陷，不隐藏首次失败。
- F-10 原“双次目标替换且发生在回滚检查前”序列独立 PASS；F-11 普通中断、不可解析外部字节与新增信号窗口的既有用例 PASS。但合法外部 STRM 的恢复，以及检查后的清理竞态仍 FAIL，见下文。
- `06-post-proof-remove.log` 为两个失败的独立 Go overlay 子测试。overlay 只增加审查测试，不改产品算法；使用现有 `remove` 注入点，在最终证明与实际 `unlinkat` 之间替换名字，再调用同一删除 syscall。该证据是确定性竞态注入，不声称发布二进制自然命中该短窗口。新增 overlay 源使用 `.go.txt` 存放，避免污染 `./...`。
- 核对 `task-1.2-fix-20261018-01/` 原始证据：与本次 Head 的可执行输入相同；quality 含 Python **48** 项、Go test／vet／build／gofmt，race 为 **160 个测试、8 个包通过，0 fail／skip**；隔离 9/9、双架构构建和二进制摘要匹配。B 本轮复用这些 T4 通过事实，未重跑 race／arm64／完整 quality；其既有输入不包含以下失败断言，不能抵消阻断。

### F-10 / P1 仍未关闭：最终证明与按名删除之间存在竞态

位置：`internal/reconciler/fs.go:326`、`:369`、`:375`、`:408`、`:425`、`:429`。

现有字节／inode 检查只证明检查时的对象，不能保证随后 `Remove(staging)` 删除同一对象。独立 overlay 在两条路径复现：

| 路径 | 受控操作 | 实际结果 | 必须结果 |
|---|---|---|---|
| 旧目标校验成功后的清理 | 校验后将 staging 换成外部文件，再执行原删除 | 外部字节消失，Update 返回 nil | 不删除外部对象；无法证明安全则非零且不提交 round |
| 回滚后的清理 | inode／字节检查后将 staging 换成外部文件，再执行原删除 | 外部字节消失，Update 返回 ErrTargetChanged | 保留外部对象与可恢复事实 |

修正步骤：

1. 把两条失败序列变成受版本控制的回归，断言目录中所有外部字节仍存在，不只检查 `A.strm`。
2. 列出 Update／undoExchange／Recover／defer 的所有删除、交换、覆盖操作，并写清每个操作实际作用对象的归属依据；不得把“检查过这个名字”当作对象在操作时仍相同的保证。
3. 修正清理与回滚协议，使外部替换不能进入可删除集合；不能证明时保留残留并返回冲突。仅增加一次 Lstat、比较字节、比较 inode 或切换随机文件名都不能关闭该竞态。
4. 保留原 F-10 双替换回归、回滚失败回归、目录句柄边界和锁行为；禁止通过忽略删除失败、先删目标再创建或无条件回滚获得通过。

### F-11 / P1 仍未关闭：合法外部 STRM 恢复后被再次更新并删除

位置：`internal/reconciler/fs.go:503`、`:523`，`internal/reconciler/syncer.go:318`、`:330`、`:365`。

`foreign_valid_recovery` 使用已提交 testhooks 在交换前／交换后暂停，外部驱动放入合法版本 1 STRM，在校验前 SIGKILL；重启完全使用发布二进制。实际 `exit=0, updated=1, conflicts=0, partial=false`，round 从 1 变为 2，外部 STRM 在目标与全部残留中均消失。现有用例只用不可解析的 `external`，其冲突来自 Parse 失败，不能证明归属安全。

修正步骤：

1. 新增合法外部 STRM 的上述 CLI／Go 回归；同时保留不可解析外部文件、普通中断与未归属目标用例。
2. 恢复结果必须把未证明的归属／残留状态传给调用者，不能在恢复外部内容后返回普通成功，再借旧路径 ownership 进入 Update。
3. 内容合法、内容相同、目标路径有 ownership 都不能单独证明恢复出的对象属于生成器。无法区分旧自有文件与外部文件时，保持保守冲突，保留目标与全部不明残留，不提交完整 round；不得自动认领。
4. 恢复涉及交换／删除时同样满足 F-10；dry-run 只报告、不恢复、不清理。每个反例检查 ownership 不扩张、round 不推进、发布重启与锁释放。
5. 若方案必须改变合同、ownership 语义或新增不被允许的持久化状态，先交 A；当前未认定合同不可满足，不授权弱化安全不变量。AC-6 本就允许无法证明的中断状态保守冲突；若需调整既有“必须恢复成功”用例，先向 B 列出具体不可区分状态及调整理由，不得批量放宽所有恢复断言。

### F-13 / P1：发布／失败清理未绑定本次创建的暂存对象

位置：`internal/reconciler/fs.go:314`、`:326`、`:353`、`:369`。

`tmpID` 仅用于回滚。交换前没有证明 staging 仍是已完整写入的新文件，成功路径仅校验被换出的旧目标。已提交 testhooks 的 pre-exchange gate 足以复现，未改产品算法：

| CLI 反例 | 实际结果 | 必须结果 |
|---|---|---|
| 外部替换 staging，再放行交换 | exit=0、updated=1、round=2；目标是 `foreign-staging-object`，并非合法 STRM | 拒绝发布不明对象；保留旧自有目标和外部对象，不提交 round |
| 外部替换 staging，并移走目标，再放行交换 | 交换失败，exit=1、round=1；defer 删除外部 staging | 保留外部对象；失败不能授权清理同名陌生文件 |

修正步骤：

1. 两个序列均加入版本控制的 CLI／Go 回归；外部驱动使用 unlink＋新建以区分名字和对象，并检查字节、身份、全部残留及数据库。
2. 将“已经写入并同步的对象”绑定到实际发布操作，既验证旧目标，也保证发布的是本次准备的新对象。随机名、O_EXCL 创建成功或发布前再读一次均不是整个后续窗口的证明。
3. 去掉错误分支／defer 对当前 staging 名字的无条件删除。tmp.Stat、write、sync、close、exchange 失败及取消路径都须遵守同一归属规则。
4. 暂存被换成符号链接／特殊文件时同样保守失败，不跟随、不阻塞、不发布；当前反例已证明普通文件存在缺口，不能只新增类型检查。

### F-12 与修正执行顺序

原 F-12 缺少的写入／文件 sync／close／发布 syscall／目录 sync／回滚失败注入，以及三个更新窗口的 SIGTERM／SIGKILL 用例已补入。其通过事实保留，下一候选继续完整 T4；最终矩阵需逐项关联测试、目标／残留、ownership、round 和重试结果。交换屏障仅证明进程观察到交换，不等于目录 fsync 已完成，不应在证据中写成已证明持久化。

Developer 按以下顺序在同一 Task 修正：

1. 先复现 F-10、F-11、F-13，保留当前失败输出；旧 B artifact 仅为复现参考，正式回归必须进入仓库测试。
2. 说明发布、恢复、清理统一如何证明对象归属，再完成相关实现；复用现有路径句柄、互斥和比较能力，不另造媒体状态机。
3. 聚焦运行 `bash scripts/go-local test -count=1 ./internal/reconciler ./internal/storage ./internal/strm ./internal/securepath ./cmd/strmgen`；扩展 `fault_recovery`，使用新发布／testhooks 二进制顺序运行，避免两个验收驱动争抢全局锁。
4. 聚焦通过后，执行上方完整 T4，新目录使用 `task-1.2-fix-20261018-02` 或其他未占用目录。源码／测试／运行配置变化后不能沿用本次 race／构建作为新候选门槛。
5. 实现提交后记录真实 Head、首次失败、重跑、精确输入与矩阵，更新事实文档并送 READY FOR B REVIEW；不改 Task Base、Contract Checkpoint、High 或 T4。

### Slice Required Outcomes 重评

| RO | 判断 | 未完成项 |
|---|---|---|
| RO-1 | 既有 CLI／配置／脱敏保留，Task 未通过 | 本地发布／恢复错误不得误报成功 |
| RO-2 | 夹具接入与稳定 URL 能力保留 | 合同要求的真实 OpenList／无签名 `/d/` 实证仍 NOT RUN |
| RO-3 | 镜像／筛选／冲突与根边界保留 | F-10／F-13 本地替换对象保护 |
| RO-4 | 未满足 | F-10／F-11／F-13；固定插件解析仍 NOT RUN |
| RO-5 | 未满足 | 新反例的非零／不提交完整轮及恢复证据；既有互斥／dry-run／有界遍历保留 |

不规划新 Task，不生成 Closure Packet，不宣布 Slice PASS／关闭。Slice 1 保持 ACTIVE；真实 OpenList、固定插件解析不能被合成夹具替代。Jellyfin／代理、ARM64 实机和性能仍单独 NOT RUN。

## Developer Fix Report — Task 1.2 F-10/F-11/F-13 (2026-10-18)

本节提交送审候选;当前状态与事实纠正以末次 B 独立审查为准。

- **Status:** READY FOR B REVIEW(交 B 独立审查;本报告不是 B PASS,也不表示 Slice 完成)。
- **Implementation Head:** `d0e2bacfb824b4876c647059e43c2996a9339417`(父提交 `e778cdaebb3f1c0f5b61ee32bd3f6e5f11ed50df`)。其中 `f16ea0f176334c9eb5ebf4eb13d0d63970acc5b9` 只改实现、测试与文档;`d0e2bac` 只在已提交驱动 `scripts/run_isolated_sync_integration.py` 增加合法外部 STRM 的 CLI 回归,Go 产品源码与 `f16ea0f` 逐字节相同。未改合同、Roadmap、稳定需求或 SQLite schema。`go.mod`/`go.sum` 未变。
- **统一对象归属证明:** `Update` 先读取并确认目标字节等于 `expected` 并记录其设备/inode(`oldID`);写入的后台暂存对象记录 `tmpID`。发布前后对 `base` 与暂存名各自执行“设备/inode 相同且内容逐字节相等”的双重校验(`entryMatches`),防止 inode 号复用造成的假阳性。任一侧不成立即调用保守回滚:`undoExchange` 仅在 `base` 仍逐字节等于本次写入对象时才换回,并且只删除同样满足双重证明的对象;否则目标与全部不明残留原样保留并返回 `ErrTargetChanged`。
- **F-13(暂存对象绑定):** 暂存写入/同步/关闭后、每次发布尝试前都要求暂存名仍指向 `tmpID` 且内容等于新文档;失败路径只在同一双重证明下清理,删除了原来无条件的 `defer remove`。外部替换暂存对象既不会被发布到目标,也不会被失败清理删除(两条新回归)。
- **F-11(合法外部 STRM 恢复):** 暂存名改为 `.strmgen-<baseHash16>-<oldHash16>.update.tmp`,编码本次 `expected` 的 SHA-256 前缀。`Recover` 通过目录句柄扫描该目标的暂存名:当目标已是 desired(交换已完成)时,把暂存内容换回目标并只删除本次 desired 输出;若暂存内容哈希不等于名字里编码的旧内容哈希,则恢复后返回 `ErrStagingPresent`,让调用者把目标当作保守冲突而不是继续 `Update`,因此合法外部 STRM 不会在重启后被覆盖。普通中断(暂存内容可证明为旧自有内容)仍换回并进入正常重做路径。
- **F-10(清理竞态):** 成功路径和回滚清理都要求发布对象与旧对象双重证明;双替换与回滚失败序列保留外部字节。第二次外部替换发生在证明之后、按名删除之前的极短窗口仍无法用 POSIX 按名 `unlink` 原子消除;候选把最终删除对象收敛到已证明对象,并在报告中如实记录该残余,未"假报关闭"。
- **Tests / Evidence:** 新证据目录 `artifacts/validation-evidence/task-1.2-fix-20261018-03/`(`index.json`、逐命令日志、race JSON、9 场景隔离输出、B 复现 `reproductions.json`、双架构与 testhooks SHA-256)。源码 SHA `d0e2bacfb824b4876c647059e43c2996a9339417`,在 `artifacts/` 被 gitignore 的干净 detached worktree 中执行。`check_governance.py`、`mod download`/`mod verify`、`quality.py`(gofmt/vet/Go test/build + Python **48** 项)、`-race -count=1 -json ./...`(test-level pass=**163**、8 个包通过、fail=0、skip=0)、Linux amd64/arm64 CGO=0 `-trimpath` 与 testhooks amd64 构建、`run_isolated_sync_integration.py --scenario all` **9/9**(含新增 `update_before_verification_valid_foreign=1`)、`git diff --check` 全部 exit=0。发布二进制 `STRMGEN_TEST` 计数 **0**,testhooks 计数 **9**。
- **B 复现:** 用 B 的 `task-1.2-b-rereview-20261018-01/run_review.py` 对新二进制重跑:`legacy_double_replacement`、`foreign_valid_recovery`、`stage-replaced-publication`、`stage-replaced-exchange-failure` 全部 **PASS**(此前 3 项 FAIL)。
- **首次失败:** 开发中两条既有 F-10 Go 回归一度因 inode 号复用导致 identity-only 证明误判(返回 nil),修正为 identity+content 双重证明后通过;已在 `index.json` 明示,未隐藏。
- **已知残余:** B 的临时 Go overlay `remove_review_test.go.txt` 包装的是 `LocalFS.remove` 接缝——它先把名字换成外部对象、再由该包装函数自己执行 `unlink`;只要算法使用任何按名 `unlink`,外部对象都会被包装函数自身删除。因此该 overlay 无法在保留按名删除的前提下通过。其针对的实质行为(F-11 合法外部恢复、F-13 暂存替换、F-10 双替换)已由仓库回归与 B 集成复现覆盖为 PASS;残余按名删除窗口如实记为未完全关闭,交由 B 判断。
- **Risks / Remaining:** 真实 OpenList/无签名 `/d/`、固定插件解析、Jellyfin/代理、ARM64 实机与性能仍 **NOT RUN**;合成夹具、交叉编译与 testhooks 不替代外部证据。Mirror 清理、serve/定时、发布镜像仍属后续 Slice。
- **Head SHA:** `d0e2bacfb824b4876c647059e43c2996a9339417`。

## B Re-review Result — Task 1.2 第二次修正复审（2026-10-18）

```text
Reviewed: 15d749b3f151e94a6fb5e9850e6e21d7101aa603..d0e2bacfb824b4876c647059e43c2996a9339417
Report Checkpoint: e474e64d8a895283b4641a980ce8523e2b17b190
Decision: FIX REQUIRED
Slice Required Outcomes all satisfied: NO
Next: DEVELOPER FIXES TASK 1.2 IN PLACE; A DECIDES ANY NECESSARY CONTRACT CHANGE
```

### 审查范围与可复用证据

- 本轮 B 已完成 AGENTS／必读清单与三份角色指南，审查前工作区干净、常规治理 PASS。核对完整 Task Base..Head；合同能力、Roadmap、稳定需求、原方案、依赖及 SQLite schema 未变。`d0e2bac..e474e64` 仅 TASK 与架构事实文档变化，可执行输入相同。本轮不改产品实现、不改变合同指针／状态。Task 头部继续 FIX REQUIRED；Developer 报告中的 READY FOR B REVIEW 仅是送审事实，不是当前决定。
- 新独立证据：`artifacts/validation-evidence/task-1.2-b-rereview-20261018-02/`。`index.json`、输入 SHA-256、逐命令日志、`reproductions.json`、两份 `.go.txt` overlay、构建产物和原始场景摘要均保留；仅本机可读，不声称已远端交接。所有 CLI 运行均为自建 localhost 合成 API、仓库 artifacts 内专用输出／SQLite，不访问生产或配套仓库。
- B 独立聚焦五包、amd64 发布／testhooks 构建 PASS。`fault_recovery`、`repeat_and_compare`、`dry_run_boundaries` 分别顺序执行均 PASS；旧 F-10 双替换与两个 pre-exchange F-13 CLI 反例独立 PASS。合法外部 STRM 第一次恢复也 PASS，但第二次重启及多文件根目录反例 FAIL，不能据第一轮通过关闭 F-11。
- `04-post-proof-remove.log` 中原 F-10 两条注入仍 FAIL；`06-recovery-and-distinct-inodes.log` 使用保留旧 fd 的新反例，证明旧对象与外部对象为不同且同时存活的 inode，两条清理仍删除外部对象。因此问题不是 inode 复用；`entryMatches` 仅证明检查时对象，不能绑定稍后的名字操作。`08-late-stage-exchange.log` 另确认 F-13 最后检查后的交换窗口仍 FAIL。overlay 只增加测试，经现有 remove／exchange 接缝执行原 syscall，不修改校验算法；是确定性时序证据，不声称无插桩发布二进制自然命中该极短窗口。
- 核对 `task-1.2-fix-20261018-03/` 原始 evidence 与可执行输入：Python 48 项，race **163 个测试／8 个包 PASS、0 fail／skip**，隔离 9/9、双架构和 testhooks 构建、二进制摘要均匹配；当前发布二进制无 `STRMGEN_TEST`。B 复用这些已通过事实，未重跑完整 quality／race／arm64；新失败不在原断言内，不能被旧通过抵消。详见 `11-reused-evidence-audit.json`。
- 保留 B 夹具准备问题：artifact-only logger 首次 SyntaxError，修正后才执行检查；一次重复传 `--scenario` 的 argparse 命令实际只执行最后的 `fault_recovery`，随后另行顺序跑两场景。没有把该首次命令计成 3/3，也没有把准备错误计为产品缺陷。

### F-10 / P1：检查后清理仍删除未知对象（AC-3／AC-5／R-06／R-07）

位置：`internal/reconciler/fs.go:393-400`、`:420-429`、`:695-713`；Recover 的 `:496`、`:532`、`:541` 也仍按名删除。

`entryMatches` 成功后 `removeFile` 仍调用 `unlinkat(name)`。在最终证明后、syscall 前替换名字，两条旧反例和保留旧 fd 的两条新反例均丢失外部字节；成功分支甚至返回 nil。报告已承认残余，但该残余是 P1，不是可接受 P2。不能把“反例在 remove 接缝注入，所以任何 unlink 都失败”当作排除反例的理由：这正是按名操作未绑定被证明对象的窗口。

Developer 在同一 Task 按次序处理：

1. 把 `TestBReviewCleanupNameCanChangeAfterProof` 与 `TestBReviewCleanupWithDistinctLiveInodes` 的关键行为纳入版本控制；目标和所有残留逐一检查字节与身份，不能只断言 error。先保存当前失败。
2. 为 Update／undoExchange／Recover／早期错误清理逐项列“实际破坏性 syscall、受作用对象、对象归属证明、证明后仍可改名的范围”。写入失败等分支的 `removeIfIdentity` 也必须纳入，不能只修成功路径。
3. 设计能把实际破坏性操作限制于可证明对象的统一协议；无法证明时保留目标／残留并保守失败，不执行按名盲清理。可复用已有目录句柄、锁和文件接口；不能只再加 Lstat／字节／inode 检查、随机名或注释证明。
4. 保留正常更新可交付性、原子目标完整性、原双替换／回滚失败／发布失败回归。若不能在当前合同内实现，先提供最小不可区分状态／系统调用限制和所需合同变更交 A；A 提交修订前不继续自定降低不变量，B 再判断当前 Task 是否合法调整。当前 B 未判合同不可满足，也未授权修改合同。

### F-11 / P1：冲突证据被消费，下一次运行覆盖合法外部 STRM（AC-3／AC-6）

位置：`internal/reconciler/fs.go:512-538`；`internal/reconciler/syncer.go:318-321`、`:330-365`。

`valid-foreign-three-restarts` 使用已提交 pre／post-exchange testhooks 和真正 SIGKILL；随后连续三次均为无插桩发布二进制。第一次：exit 1、conflicts=1、round=1、外部 STRM 留在目标；第二次：exit 0、updated=1、round=2、目标及全部残留均无外部字节；第三次正常 skip、round=3。Recover 第一次返回冲突前已删掉唯一 staging，旧 ownership 路径映射不变，下一进程找不到该冲突事实。只延迟一次覆盖不能通过归属安全验收。

修复清单：

1. 扩展现有合法外部恢复 Go／CLI 用例，在相同 cfg、同一 API 身份、同一 DB／输出上连续运行至少三次；每次检查 exit、摘要、所有外部字节、ownership 不扩张、round 不推进。首轮恢复通过不是结束条件。
2. 保留可供后续运行判定的不明归属事实；不得在返回 `ErrStagingPresent` 前清除唯一证据，再让旧路径 ownership 自动恢复为可写权限。不能通过仅改日志／计数，或把外部 STRM 改成不可解析来通过。
3. 覆盖 dry-run 后真实运行、再次 SIGKILL／恢复中断、普通已拥有中断、无 DB／未归属与多残留；无法区分时保守冲突，但不批量放宽正常恢复用例。任何交换／清理同时满足 F-10。
4. 不新增媒体版本状态表。若确需改变 ownership 语义或合同允许的持久事实，先交 A 决策，随后 B 重新判断当前 Task；不在下层实现中悄悄改需求。

5. **B 显式测试计划调整（仅不明外部恢复）：** `TestRecoverRestoresForeignDisplacedContent`、`TestForeignInterruptedUpdateRecoveryConflicts`、`TestValidForeignInterruptedUpdateRecoveryConflicts` 与 CLI `update_before_verification_foreign`／`update_before_verification_valid_foreign` 中强制“无 staging 残留／仅一个目标”的断言可改为“保留必要的不明归属证据，连续至少三次运行保守冲突且全部外部字节保留、round 不推进”。理由是原断言强制消费唯一证据，与 AC-3／AC-6 冲突；这不是允许删安全断言。普通已证明更新中断的成功重试／无未知残留、未归属保护、dry-run 零写和完整 T4 均不降低；若方案无需保留 staging，也仍须证明等价的跨进程保护。

### F-13 / P1：提前的暂存替换已修正，最终证明后的发布仍未绑定对象

位置：`internal/reconciler/fs.go:369-395`、`:420-429`。

现有 hook 前替换 staging 的两个 CLI 反例本轮 PASS；该通过事实保留。但 `TestBReviewStagingReplacementAfterFinalProof` 在最后一次 `entryMatches` 后、原 `RENAME_EXCHANGE` 前换入不同的存活外部对象，Update 返回 `ErrTargetChanged` 时目标已经是外部 staging，旧自有目标留在残留。事后校验虽发现错误，`undoExchange` 因目标不是 tmpID 不再恢复旧目标，仍未满足“发布的是本次完整写入对象”的原要求。

1. 将晚于最后证明的反例纳入仓库，区分“hook 前被拒绝”和“实际 syscall 对象安全”；补齐普通文件、符号链接、FIFO 和缺失目标，不跟随、不阻塞、不误发布。
2. 与 F-10 使用同一对象归属协议修复，不能再把临时名创建成功／最后读一次当作发布时绑定保证；失败后不能把陌生 staging 当目标完成，也不能清理未知对象。
3. 扩展 CLI 中断覆盖到该发布窗口；若必须改动插桩，仅限明确 testhooks，恢复重启仍使用发布二进制。已通过的 pre-exchange 反例保留为回归，不因新失败删减。

### F-14 / P1：绑定根的目录游标被消耗，后续文件漏检恢复残留（AC-6／AC-7／R-11）

位置：`internal/securepath/securepath_linux.go:381-396`；`internal/reconciler/fs.go:451-456`、`:635-650`。

`BoundDir.Open` 通过 `dup` 返回根目录描述符；`Readdirnames(-1)` 与绑定 fd 共享底层目录 offset。第一个文件的 Recover 消耗根目录流，第二个根文件扫描得到 EOF，将已有残留误判为无残留。同一 prepared root 连续三次 dry-run Recover 的 Go 反例为 `[ErrStagingPresent, nil, nil]`。CLI `root-second-file-recovery` 中 A 无变化先处理、B 留有 SIGKILL 后外部残留，发布二进制第一次恢复即 exit 0、skipped=2、round 1→2；连续重启 round 继续到 4，外部残留一直被隐藏而不是报告冲突。

1. 先提交已绑定根／至少两个根文件的 Go 和 CLI 回归，残留放在非首个文件；测试多次查询、dry-run、重启和锁释放。不能用不调用 PrepareRoots 的 LocalFS 测试替代真实 CLI 装配。
2. 枚举必须使用独立的目录 open description，且仍从已绑定目录安全打开；不能重新用绝对路径、不能依赖 dup 产生独立游标。避免在共享 fd 上随意 Seek 干扰其他查询。
3. 恢复扫描须有界读取／及时停止；当前 `Readdirnames(-1)` 每次加载全部本地目录名字，不能以远端 10,000 项上限替代本地残留／未归属条目上限。无需扩展到性能 Slice，只落实 R-11 的内存边界和保守错误。
4. 复核有无多文件／多 source 的同类共享状态；新发现修正保留在本 Task，不按内部 helper 再拆 Task。

### 修正顺序、验证与交接

1. 先固化 F-10／F-11／F-13／F-14 当前失败，并提交简短的统一对象归属／恢复证据设计；若需要 A 合同决策先停在设计交接，不自行变更 RO、ownership 或测试含义。
2. 实现目录枚举修正和可跨进程保留的冲突判定，连同统一发布／回滚／清理协议整体验证；不得只修反例中的某一个 hook 时刻。
3. 开发反馈：`bash scripts/go-local test -count=1 ./internal/reconciler ./internal/storage ./internal/strm ./internal/securepath ./cmd/strmgen`，随后用新二进制顺序执行关键 CLI 反例、`repeat_and_compare`、`dry_run_boundaries`、`fault_recovery`。CLI 每次只传一个 `--scenario`，或者最终用 `--scenario all`；不要把多个驱动同时启动争抢全局锁。
4. F-12 已补齐的故障矩阵继续保留；下一候选执行本 Task 原完整 T4，选择新目录（例如未占用的 `task-1.2-fix-20261018-04`）。源码／测试／驱动改变后旧 race／构建不计新候选门槛。正式回归必须入库，B artifact overlay 只作复现参考。
5. 验证后提交真实 Implementation Head 和匹配证据，逐项列命令／源码 SHA／退出码／目标与残留／ownership／round／重试，记录首次失败；交 READY FOR B REVIEW。Task Base／Contract Checkpoint／High／T4 不变，只有 B 可对测试计划作有理由的调整。`afterUpdateExchange` 是观察屏障，不是目录 fsync 持久化证明；新增驱动中“durable exchange”措辞一并纠正。

### Slice 全部 Required Outcomes / Surfaces 复核

| RO | 判断 | 剩余／限制 |
|---|---|---|
| RO-1 | 入口／配置／脱敏既有能力保留；未全部满足 | F-11／F-14 仍错误报告完整成功轮 |
| RO-2 | 合成 API、分页、元数据与稳定 URL 能力保留 | 真实 OpenList／无签名 `/d/` 证据仍 NOT RUN |
| RO-3 | 镜像／筛选／冲突／路径句柄能力保留 | F-10／F-13 的对象替换保护未闭环 |
| RO-4 | 未满足 | F-10／F-11／F-13／F-14；固定插件解析仍 NOT RUN |
| RO-5 | 未满足 | 漏检恢复与跨轮冲突、按名操作错误；既有互斥／dry-run 无写／远端有界遍历保留 |

不新建 Task，不生成 Closure Packet，不交 A 最终审查或宣布 Slice 关闭。Slice 1 保持 ACTIVE；真实 OpenList 与固定插件解析仍是最终验收缺口。Jellyfin／代理、ARM64 实机及性能另报 NOT RUN，不以其缺失扩大本 Task。

## B 修复执行补充与 A 判定请求（2026-10-18）

本节细化同一 Task 的修正，不改变 FIX REQUIRED、Base、Contract Checkpoint、High／T4 或验收含义；不是产品实现或新的独立复审。

### 1. 先固化失败，不再增加检查次数追逐窗口

1. 把 B 的 F-10／F-13 最终检查后注入反例纳入版本控制，保留旧 fd 以排除 inode 复用；F-11 固定同一 DB／API 身份的至少三次发布二进制重启；F-14 固定 PrepareRoots 后非首个根文件残留。先记录未修实现的实际失败。
2. 为 Update／undoExchange／Recover／早期错误清理列出每次 exchange／unlink 的实际名字、归属依据、证明后仍能变化的位置，以及中断后留下什么事实。复用已有 LocalFS／securepath 接缝，不另造文件发现或媒体状态机。
3. F-10／F-13 的统一协议进入实施前先回答第 4 节；多做一次检查、随机名、返回错误或同 UID 的 0700 目录都不是完整对象安全证明。

### 2. F-14：独立、流式目录枚举，按当前合同直接修正

1. 枚举时从已绑定目录 fd 相对打开点目录，使用 O_RDONLY／O_DIRECTORY／O_CLOEXEC／O_NOFOLLOW 得到新的 open file description；失败返回错误。不重新打开绝对路径，不在共享 fd 上 Seek，不一律改写其他 Duplicate 用途。
2. 每次枚举使用自己的 fd，结束、错误和提前停止均关闭。固定批次调用 Readdirnames(n)，例如每批 128 项；不能先 Readdirnames(-1) 再切片。
3. findStagingFiles 只需区分 0、1、至少 2 个匹配；最多保留两个匹配名字，第二个即按歧义结束。判定 0 或唯一项须读到 EOF；若设置扫描上限，达到上限返回不完整／冲突，不能当作不存在。
4. 回归覆盖同一 prepared root 重复查询、交替查询 A／B、dry-run 后真实运行、发布二进制连续重启及大量无关本地条目。每次稳定检出非首个文件的残留；枚举或关闭失败不推进 round。不能用未调用 PrepareRoots 的测试代替 CLI 装配。

### 3. F-11：先保留跨进程冲突事实

1. 最小修正候选：不明归属分支在 exchange／unlink 前返回 ErrStagingPresent，保留目标与残留，不先删除唯一见证再返回冲突。此前仅针对不明外部恢复的测试计划调整仍适用。
2. 检查所有 target==desired／staging==desired 分支；内容相同、可解析、名字中的哈希匹配，不能单独授予删除或认领权限。保留事实须在远端 desired 改变、dry-run、再次中断及后续进程中仍能识别。
3. 连续至少三次重启，逐次检查全部外部字节、退出码、冲突摘要、ownership 和 round，不只看目标；不得仅改日志或首轮退出码。
4. 不允许把所有正常中断恢复永久改为冲突以通过。正常可证明恢复／重试仍须满足；若安全区别需要新的 ownership 语义或持久事实，先交 A，不新增媒体版本表或偷偷引入事务状态机。该最小候选不代表 F-10／F-13 已解决。

### 4. F-10／F-13：交 A 判定写者模型，不再要求盲目补检查

**待 A 判定：是否要求在具有目录写权限、且不遵守生成器锁的外部进程可在任意 syscall 间隙替换目标／暂存名字时，仍绝不误删或误发布陌生对象？**

- 当前 unlinkat／renameat2 接口按目录 fd 和名字操作，没有 expected-inode 参数；绑定目录不等于把已检查的目录项绑定到下一次操作。因此当前协议的最终检查后窗口不能靠多检查一次关闭。此结论针对当前接口与协议，不宣称一切平台机制都不可能实现。
- B 推荐提交 A 评估的路线是明确可执行的单写者部署边界：目标父目录、暂存命名空间及文件原地写入由实际权限隔离，或所有写者共同遵守的互斥约束；消费者只读。只保护暂存目录仍不解决目标窗口，同 UID 的 0700 或不参与协议的进程也不受现有 flock 约束。不能把启动权限检查写成“已证明未来无外部写者”。
- 该推荐改变并发威胁／部署前提，必须由 A 协调需求、架构、合同及测试并提交；B 未授权实施、弱化 AC-5 或删除既有反例。仍须保护预先存在的未归属文件，不能以单写者为由自动认领。
- 若 A 保留任意外部写者保证，先提交能约束实际命名空间变更的机制设计及最小实证，同时覆盖目标、暂存、回滚与清理。私有暂存目录、O_TMPFILE、持有文件 fd 或更多事后校验不能单独视为全部解决。需要超出 Scope 的设施则交 A 评估，不在现有按名协议上无限补丁。
- A 交接写明所选写者模型、未归属保护、故障／恢复语义、持久事实边界及部署验证方法。实质修订提交后，B 再决定当前 Task 继续、合法调整或重新规划；此前可固定反例、修正 F-14 和保留不明恢复证据，但不得声称候选满足原 AC-5。

接口依据：Linux man-pages 的 unlink(2)、rename(2)、dup(2)、open(2)，分别为 https://man7.org/linux/man-pages/man2/unlink.2.html 、https://man7.org/linux/man-pages/man2/rename.2.html 、https://man7.org/linux/man-pages/man2/dup.2.html 、https://man7.org/linux/man-pages/man2/open.2.html 。参考文档不替代项目实证。

### 5. 交接顺序

失败回归 → F-14／F-11 合同内修正 → A 决策及必要合同检查点 → 统一发布／回滚／清理协议 → 聚焦 CLI → 完整 T4 → B 独立复审。全部留在 Task 1.2，可分普通本地提交，不按内部 helper 或测试另拆 Task。设计／治理 PASS 不等于产品 PASS；可执行候选使用新的匹配证据目录。
