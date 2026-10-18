# Task 1.2 — 已拥有 STRM 内容更新与原子恢复

```text
Task ID: 1.2
Parent Slice: 1
Status: PLANNED
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
