# Task 1.2 — 已拥有 STRM 内容更新与原子恢复

```text
Task ID: 1.2
Parent Slice: 1
Status: READY FOR B REVIEW
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
TASK12_RUN_DIR=artifacts/validation-evidence/task-1.2-fix-20261018-01
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

独立复现 URL／SHA-1／Size 更新、无变化零写、dry-run，以及 F-10／F-11 的第二次本地替换和交换校验前中断。核对目标与残留文件的身份／字节、ownership、rounds、退出码、锁释放和发布二进制重启；检查 F-12 的逐项原始证据。可复用输入相同的完整 T4 证据，不用重复运行全部相同输入；不得用旧用例通过替代新窗口。

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
