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

允许修改 `cmd/strmgen`、`internal/reconciler`、`internal/strm`、`internal/storage` 及其 Go 测试，必要的隔离夹具、CLI 驱动、文档和质量工具。不得改动原方案、需求、Slice 合同、Roadmap 或固定 T4 等级。

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
python3 scripts/check_governance.py
bash scripts/go-local mod download
bash scripts/go-local mod verify
python3 scripts/quality.py
CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 -json ./...
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 bash scripts/go-local build -trimpath -o artifacts/task-1.2/bin/strmgen-amd64 ./cmd/strmgen
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 bash scripts/go-local build -trimpath -o artifacts/task-1.2/bin/strmgen-arm64 ./cmd/strmgen
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 bash scripts/go-local build -tags strmgen_testhooks -trimpath -o artifacts/task-1.2/bin/strmgen-testhooks-amd64 ./cmd/strmgen
python3 scripts/run_isolated_sync_integration.py --binary artifacts/task-1.2/bin/strmgen-amd64 --fault-binary artifacts/task-1.2/bin/strmgen-testhooks-amd64 --scenario all --output-dir artifacts/validation-evidence/task-1.2-final
git diff --check
```

`quality.py` 覆盖非 race 的 Go／Python 检查；testhooks 只用于受控故障，不是发布产物。CGO race、arm64 构建和真实 OpenList／插件／Jellyfin 运行分别报告，不互相替代。

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

- **Status：** READY FOR B REVIEW（交 B 独立审查；本报告不是 B PASS，也不表示 Slice 完成）。
- **Implementation Head：** `6eb4db67837ca8d3a65e1ce23bac6ea13b2592b9`（父提交 `6d683e85f4b1681dbe17a99af12c11ffa544a63a`）。只改实现、测试与隔离驱动；未改合同、Roadmap 或稳定需求。
- **Changed Files：** `cmd/strmgen/main.go`；`internal/reconciler/{fs.go,fs_test.go,syncer.go,syncer_test.go}`；`internal/securepath/securepath_linux.go`；`internal/strm/strm.go`；新增 `internal/reconciler/{fs_update_hook_default.go,fs_update_hook_integration.go,fs_update_after_hook_default.go,fs_update_after_hook_integration.go}`；`scripts/run_isolated_sync_integration.py`；`tests/test_run_isolated_sync_integration.py`。`go.mod`／`go.sum` 未变。
- **Implemented：** 移除仅报告不写入的 `NeedsUpdate` 分支，`handleFile` 在 `Update` 决策时真正原子替换已拥有 STRM。`LocalFS.Update` 在同目录临时文件写入／`fsync`／关闭后，用 `securepath.Dir.Exchange`（`renameat2` `RENAME_EXCHANGE`）交换目标与临时文件，再逐字节校验被换出的旧目标等于更新前读取的已拥有字节；不等则交换回去并返回 `ErrTargetChanged`，保留外部字节、绝不 `unlink` 旧目标作为补偿。发布前重新确认目标是已拥有普通文件（符号链接／特殊文件／缺失均保守失败）。成功替换后删除旧内容并 `fsync` 目录，保留原 ownership 映射，不新建任何版本表。`handleFile` 区分 `updated`／`skipped`／`conflict`／`errors`，更新失败非零且 `partial=true`；一个 source 仅当遍历、更新、归属全部成功才提交完整 round。dry-run 复用同一比较和冲突判断，只计 `updated` 预览，不写文件、数据库、sidecar、目录或轮次。报告新增 `updated=`，移除 `needs_update=`。受控故障与恢复只经 `strmgen_testhooks` 构建（`STRMGEN_TEST_UPDATE_*`），发布二进制不含任何 `STRMGEN_TEST` 字符串。
- **Tests and Results / Evidence：** 本机权威证据目录 `artifacts/validation-evidence/task-1.2-final/`（非远端可读），源码 SHA `6eb4db67837ca8d3a65e1ce23bac6ea13b2592b9`，构建自干净已提交 HEAD。治理 PASS；`mod download`／`mod verify` PASS；`quality.py`（gofmt／vet／Go test／build＋Python 47 项回归）PASS；`-race -count=1 ./...` PASS（test-level pass=155、fail=0、skip=0）；Linux amd64／arm64 CGO=0 `-trimpath` 与 testhooks amd64 构建 PASS（SHA-256 amd64 `3f4f6d12…`、arm64 `90ae5ac1…`、testhooks `c097434c…`）；发布二进制 `STRMGEN_TEST` 计数 0，testhooks 计数 5；`run_isolated_sync_integration.py --scenario all` 9/9 场景 PASS；`git diff --check` PASS；提交后治理与 Python 回归 PASS。
- **新增／加强测试：** Go 层 `TestURLChangeUpdatesOwnedFileAndCommits`、`TestSHAChangeUpdatesOnlyAffectedFile`、`TestIncomparableSizeChangeUpdates`、`TestUpdateFailurePreservesTargetAndRound`、`TestTargetReplacedBeforeExchangeIsPreserved`、`TestDryRunReportsUpdateWithoutWriting`，以及 `fs_test.go` 的原子替换、外部替换保留、符号链接／FIFO／超长拒绝、写入／同步／关闭／交换各阶段失败保留目标并清理临时文件。隔离驱动新增：真实 URL／SHA-1／不可比 Size 更新、dry-run 更新预览后真实应用、更新临时写入失败与重试、更新后 `SIGKILL` 由发布二进制重启完成原 epoch、交换前外部替换竞态保留外部字节。
- **首次失败与修正：** 首轮 `fault_recovery` 的“更新写入失败”子用例误用了新的 API server，导致 source identity 变化被报为 `conflicts=1`（`errors=0`）。定位为测试夹具错误而非产品缺陷；改为复用同一 server、仅改 `public_url`（identity 不含 public URL），修正后 `errors=1`、旧目标保留、重试成功。另将 `Update` 中交换回滚失败的分支改为保留 `tmpName` 外部字节、不删除，避免回滚失败时误删外部内容；两处均在最终 T4 全量重跑通过。
- **Decisions / Risks / Remaining Required Outcomes：** 使用 `RENAME_EXCHANGE`（已在本环境实证支持）保证替换期间目标永不缺失，并在交换后校验旧目标以检测外部替换；这是本 Task 的关键安全决策。`ErrTargetChanged` 与“交换回滚失败”都保持保守失败、不登记成功轮、不伪造 ownership。RO-4 的已拥有文件内容替换与原子恢复证据本 Task 已提供，最终是否满足由 B 复核。真实 OpenList／无签名 `/d/`、固定插件解析、Jellyfin／代理、ARM64 实机与性能仍 **NOT RUN**；夹具与交叉编译证据不替代外部兼容／播放证据。Mirror 清理、serve／定时、发布镜像仍属后续 Slice。
- **Head SHA：** `6eb4db67837ca8d3a65e1ce23bac6ea13b2592b9`。
