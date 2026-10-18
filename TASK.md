# Task 1.2 — 已拥有 STRM 内容更新与原子恢复

```text
Task ID: 1.2
Parent Slice: 1
Status: FIX REQUIRED
Task Base: 15d749b3f151e94a6fb5e9850e6e21d7101aa603
Contract Checkpoint: bfdceed5b3c564ca92aef47035dcced314f24d45
Difficulty: High
Test Level: T4
Planner / Reviewer: B
```

范围 authority：已提交的 `bfdceed5b3c564ca92aef47035dcced314f24d45:SLICE.md`（2026-10-18 A 按用户澄清修订最终一致性边界）。Slice 1 仍 ACTIVE，Base 为 `f90f9560e55b07fd2d6a20e8bac43741c39176c4`；Task Base 不变。B 判定当前 Task **合法原地调整后继续修正**，不新建 Task，不把合同修订当产品 PASS。Developer 不自行修改 Slice／Roadmap／稳定需求或降低 T4。

本文件替代 `483069c2182e69a19fa8fd1ef605367df0495063:TASK.md` 的当前指令；旧完整计划、Developer 报告、复审、反例和权限扩展清单留在该可达检查点及其祖先，作为历史证据，不再叠加执行。尤其撤回强制 UID／mode 校验、消费者 OS 拒写实验、F-11 外部注入状态三次永久冲突和 F-10／F-13 任意写者绝对防护要求。

## Goal

在 Task 1.1 基础上，完成已拥有 STRM 的内容更新与失败后重试闭环：按既有 URL／有效 SHA-1／有效 Size 规则比较并同目录原子更新；修正 prepared root 的独立有界恢复枚举；生成器正常中断后，输入稳定、存储可用、可靠归属和路径边界有效时，后续成功同步可收敛。保留真正未归属文件、路径／符号链接、dry-run 与完整轮规则，不新增权限安全系统或媒体版本状态。

## Base 与实际未完成项

- Task 1.1 历史 B PASS 范围为 951aaeeeca966e8ef79e8ac9500827ce27d0f035..15d749b3f151e94a6fb5e9850e6e21d7101aa603；创建／归属／互斥／dry-run 继续复用。首次发布后尚未登记归属的中断窗口继续保留未归属冲突，不新增自动认领。
- 被审实现 Head：14119b8389fdc76aa35df5ed7ba3e297810c11c5；报告后继 abdc6e5 仅改 SLICE／TASK，可执行输入未变。完整 Task Base..Head 纳入审查；已提交 A 合同修订作为 authority，不由 B 重签。
- **F-14 修正已核对：** OpenDirReader 相对已绑定目录打开独立 open file description；findStagingFiles 使用 128 项批次、最多缓存两个匹配名、EOF／读取错误分离。prepared root 非首目标／重复查询回归通过，不再列为未修阻断。保留实现，不退回 dup／Readdirnames(-1)。
- **F-15 / P1，空／半写暂存的正常重启仍不收敛：** Update 自己建立确定性 staging 后，在 writeTempFile 尚未完成时退出，留下 0 或 17 字节 regular residual；可靠 ownership 与旧 target 完整保留。Recover 只接受完整 old/new hash 配对，连续两个重试均 conflicts=1、partial=true、目标未更新。相同／改变 desired 四组合均复现。位置：internal/reconciler/fs.go 的 Update 写入段和 Recover 完整哈希匹配分支；无外部目标／暂存替换。
- **F-16 / P1，恢复目录变更未确认持久化即提交 round：** Recover 在交换／删除残留后直接返回 nil，未调用现有 syncDirectory。交换后中断、以旧 desired 重试，会恢复旧 target 并走 Unchanged：注入 syncDir 失败的接缝调用为 0，返回成功、skipped=1、下一 epoch 从 2 变 3。这证明缺失目录 sync／错误传播，不是已实测断电丢数据。
- F-10／F-13／外部来源 F-11 历史失败及 SW 分类保留，不重新要求任意外部注入永久保全。F-15／F-16 阻断 AC-4／AC-6，Task 原地 FIX REQUIRED；Task Base、合同、High／T4 不变，不新增碎片 Task。

## Scope

允许修改 `cmd/strmgen`、`internal/reconciler`、`internal/strm`、`internal/storage`、`internal/securepath` 及对应测试，必要的隔离夹具、CLI 驱动、质量工具和事实文档。优先复用现有锁、绑定目录句柄、LocalFS／Recover、比较规则和 SQLite 最小事实，不重建更新协议、不按字段／helper／测试另拆 Task。

### 保留的行为

1. URL 变化必须更新，即使 SHA-1／Size 相同；URL 相同且有效 SHA-1 不同须更新。SHA-1 不可比时，仅两侧有效 Size 不同推导变化；相同或信息不足不虚构变化。
2. URL 相同且有效 SHA-1 相同，忽略 Size 差异、记录异常并跳过；无变化不改字节或 mtime。按现有协议限制生成，不下载媒体、不拼接旧哈希到新版本。
3. 未归属目标不认领／覆盖／删除；无效 STRM、符号链接、FIFO／目录等特殊目标仍按现有冲突规则处理。OS UID、名字或合法内容不能独立建立归属。
4. 更新沿用同目录完整暂存、文件 sync／close、目录句柄相对原子发布和必要目录 sync；失败非零、partial=true，不伪造 ownership 或完整 round。一个 source 必要处理全成功才提交 round；已完成的其他文件可以保留。
5. dry-run 只预览和检测，不修改 STRM、DB／sidecar、输出目录、round 或缺失计数；日志保持脱敏。全局锁、目录队列／HTTP 限额继续有界。
6. 不追加 UID／group／other 写权限检查或不同身份消费者验证；现有低成本防御与已有锁目录校验可保留，不把它们扩张为安全审计承诺。

### Developer 修正步骤（按顺序，留在同一 Task）

1. **保留失败输入。** 执行治理，记录源码 SHA／工作区、工具链和新 artifacts 证据目录。读取 B overlay／原始日志（见 B Review Result），将正常流程断言纳入入库回归，不只断言返回 error；先记录修前失败再改产品。
2. **复现 F-15。** 使用 PrepareRoots、真实 SQLite store 与 Syncer 先创建 A.strm。让 Update 自己建立 staging，分别在任何字节写入前和只写部分后中断，不由外部制造／替换 staging。断言旧 bytes／mtime／ownership 保留、round 不推进。分别使用相同 desired 与改变 URL／可比较 SHA-1／Size 重试，检查退出码、目标、残留、ownership、round。
3. **最小修正未完成写入。** 复用 LocalFS／Recover、双哈希 staging 名、现有 ownership 和目录句柄。区分“可靠拥有、唯一可识别 regular staging、target 仍匹配旧内容”的交换前未完成写入，与无归属、歧义、类型／路径错误；不要仅因 staging 不匹配完整 new hash 就永久冲突。按 SW 正常单写者边界识别自己的未完成暂存，不要求抵御任意伪造同名内容。不能批量删 tmp、按合法 STRM／名字自动认领或增加媒体状态表；若确需改变 ownership／合同，交 A 最小矛盾，不自行扩张。
4. **修正 F-16 完成边界。** 逐一检查 Recover 中 exchange／remove 成功路径，让现有目录同步及错误接缝在返回恢复成功前生效；不能指望后续 Update 必定执行，不能用 DB commit 代替目录 sync。sync／读取／删除失败须非零、partial=true、不提交 round。覆盖恢复后目标已等于 desired、后续只 skip 分支及 sync 失败后再运行；必要时调整现有恢复顺序，不新增持久化恢复状态机。
5. **保留安全与映射。** 未归属、DB 丢失、两个残留、未知／不可解释名称、symlink／FIFO／目录、路径逃逸继续保留冲突；dry-run 不做恢复写入。参见 F-15 专项映射，允许分离外部注入永久保护断言，不删除真正未归属保护或隐藏 skip。枚举 reader 的 open/read/close 错误向上传播，各路径关闭自己打开的 fd；大目录测试应实际确认跨批次，不能凭创建顺序假定。
6. **补确定性进程验证。** 必要屏障仅放 testhooks：staging 创建后未写、部分写入后暂停；SIGKILL 后使用无测试开关发布二进制重启。证明相同／改变 desired 收敛，稳定重跑 bytes／mtime 不变。Go 故障测试补恢复目录 sync 失败、不提交 round、重试；保留既有交换前后／round 前 SIGTERM／SIGKILL、F-14、I/O／DB／互斥测试。不改生产或配套仓库。
7. **匹配新候选再送审。** 先跑下方 F-15／F-16 回归、五包，再顺序运行 CLI repeat_and_compare、dry_run_boundaries、fault_recovery。最终新实现及入库测试执行原完整 T4；不能把 14119b8 T4 当修后通过。提交实际实现检查点、完成报告后交 READY FOR B REVIEW，不自签 B／A PASS。

## Acceptance Conditions

- [ ] **AC-1 URL 更新：** 同一已拥有目标仅改变稳定 URL 仍原子更新，ownership 不变，成功 round 正确提交。
- [ ] **AC-2 SHA-1／Size：** 有效 SHA-1／可比较 Size 的变化、相同 SHA-1 的 Size 异常及信息不足严格符合既有比较表。
- [ ] **AC-3 无变化／未归属：** 稳定重复同步零写；真正未归属、无效、符号链接及特殊目标保留、不认领／覆盖／删除；DB 丢失不自动认领。
- [ ] **AC-4 原子失败：** 写入、文件 sync、close、发布、目录 sync、回滚或 DB 失败不留半个目标，不伪造归属／成功轮；恢复状态可解释，正常已拥有更新可重试。
- [ ] **AC-5 正常写者边界：** 生成器共享现有锁互斥；正常单写者下更新、回滚、残留处理和再运行满足 SW-1..SW-6。不要求新增 UID／mode、只读消费者 OS 拒写证据或任意外部写者按 inode CAS 保证。
- [ ] **AC-6 正常中断收敛：** 已拥有更新的替换前后、校验前、round 提交前的 SIGTERM／SIGKILL 有确定性证据；重启 desired 不变／改变都能在后续成功同步收敛，再重复运行跳过。正常更新残留不能普遍永久冲突；首次创建归属未登记和真正无归属／不可解释冲突仍保留，不以自动认领补洞。
- [ ] **AC-7 dry-run：** 更新、冲突及故障预览无文件／数据库／sidecar／round 写入，退出码／摘要准确，之后真实同步重新获取事实。
- [ ] **AC-8 交付事实：** Go／Python 回归、race、双架构交叉构建、发布二进制重启、testhooks 故障场景和 diff check 有匹配原始证据；未运行外部实证独立标记 NOT RUN。
- [ ] **AC-9 独立有界枚举：** 同一 prepared root 多目标／重复查询／dry-run 后真实运行／重启稳定检出非首个目标残留；枚举独立游标、固定批次、匹配缓存至多两个，EOF 与错误区分，错误不推进 round。

## Required Tests

### 开发／修正反馈

先将 F-15／F-16 入库回归命名为 TestInterruptedPartialStagingRecoveryConverges、TestRecoveryDirectorySyncFailureDoesNotCommit（可加子测试），先执行：

```bash
bash scripts/go-local test -count=1 -v -run 'TestInterruptedPartialStagingRecoveryConverges|TestRecoveryDirectorySyncFailureDoesNotCommit' ./internal/reconciler
```

然后执行原反馈门槛：

```bash
python3 scripts/check_governance.py
bash scripts/go-local test -count=1 ./internal/reconciler ./internal/storage ./internal/strm ./internal/securepath ./cmd/strmgen
```

复用并扩展 `TestInterruptedUpdateRecoveryRedoesUpdate`、`TestUpdateInterruptedBeforeVerificationLeavesRecoverableState`、`TestRecoverDryRunDetectsWithoutModifying`、`TestRecoverAmbiguousResidualIsConflictAndLeavesEverything`、`TestRecoverRefusesUnownedStagingWithoutDeleting` 和 `TestRecoverRemovesOwnStagingWhenTargetAbsent`，不能用旧 needs_update 计数替代实际字节／归属／round／重试断言。

用新发布／testhooks 二进制顺序运行 CLI `repeat_and_compare`、`dry_run_boundaries`、`fault_recovery`；每次驱动只传一个 --scenario，或最终跑 all。不同时启动多个驱动争用固定全局锁；每次用新的输出目录，不覆盖旧二进制／证据。afterUpdateExchange 只说明交换已发生，不等于目录已 fsync。

### 最终 T4 门槛

仓库根目录执行，使用未占用的新目录；以下命令保留原完整门槛：

```bash
TASK12_RUN_DIR="$(mktemp -d artifacts/validation-evidence/task-1.2-convergence-20261018-XXXXXX)"
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

quality.py 包含非 race 的 Go／Python 检查；race、arm64 交叉构建、真实 OpenList／插件／Jellyfin 运行分开报告。testhooks 仅供受控故障；发布二进制不含 STRMGEN_TEST 开关，并由发布二进制实际重启验证恢复。

### B 独立验证与证据

独立复现 F-15 的 0／半写后重启及 F-16 的恢复目录 sync 失败／round 不推进；保留 F-14 真实装配、URL／SHA-1／Size 更新、零写、dry-run、正常重启及 desired 变化收敛；核对全部目标／暂存、ownership／round、退出码及锁释放。未归属保护／真实 I/O 错误继续核对，不要求不同 UID 消费者实验。按下面映射区分外部干预诊断和正常故障回归。

每个候选记录完整源码 SHA、未提交输入摘要、命令／cwd／版本／依赖／fixture 身份、退出码／计数／耗时、原始日志、首次失败和重跑。源码、测试、依赖或运行输入改变后，最终 T4 重新匹配候选。旧相同输入的事实可引用，但旧 T4 不能证明修后新候选已通过。NOT RUN／BLOCKED／SKIPPED 不写 PASS，本机 ignored evidence 不宣称已远端交接。

## B 合同后调整决定与测试映射（当前有效）

初始干净工作区常规治理 PASS。A 先提交 `bfdceed5b3c564ca92aef47035dcced314f24d45`，治理随后正确拒绝旧 Task checkpoint；本 Task 更新 checkpoint 后须恢复 PASS 才有实施准入。不更改治理脚本绕过顺序。维持同一 Task／Base／High／T4／FIX REQUIRED，不按权限字段、模块或测试分拆工作。

| 发现／用例 | 本轮明确处理 | 继续验证的合同内行为 |
|---|---|---|
| F-10：最终检查后外部换名，再按名清理 | 不再是任意外部写者绝对保全门槛；保留历史 FAIL，不称已修复 | 正常更新／回滚／暂存清理、系统调用失败、完整轮与真正未归属保护 |
| F-13：最后校验后外部替换 staging 再发布 | 排除任意替换窗口的绝对防护；不要求 inode CAS 或更多轮检查 | 正常暂存完整性、原子发布、失败／取消；已有低成本替换检测回归可保留 |
| F-11：外部注入内容经恢复后的多次运行被覆盖 | 原外部来源序列不再强制三次永久冲突；不换成静态 legacy 继续要求无损保全全部注入字节 | 无外部干预的正常恢复不丢必要依据、不错误成功且后续收敛；真正无归属／类型／路径冲突保留 |
| F-14：prepared root 共用目录游标漏检后续目标 | 本候选修正已核对；保留 AC-9 回归 | 独立流式枚举、非首个目标残留、重复／交替／多 source、EOF／错误、有界内存 |
| F-12：内部 I/O／发布／回滚／目录 sync／信号故障 | 已补覆盖保留，正常故障门槛不降低 | 目标完整性、DB／round、锁释放、正常重启；不能把观察屏障当持久化证明 |

**现有用例的具体授权：** `TestRecoverRestoresForeignDisplacedContent`、`TestForeignInterruptedUpdateRecoveryConflicts`、`TestValidForeignInterruptedUpdateRecoveryConflicts` 以及 CLI `update_before_verification_foreign`／`update_before_verification_valid_foreign` 的现有有益防御可继续保留，不要求扩展为任意外部内容跨三轮永久保全。若正常恢复修正使其断言需调整，B 允许明确分离为外部干预诊断与正常流程的恢复／未归属回归，报告原名、原触发条件、调整原因和替代断言；不得批量删除、只加 skip 或把已知 FAIL 标 PASS 来凑全绿。`TestRecoverRefusesUnownedStagingWithoutDeleting` 等真正未归属用例不因该分类被删除或削弱。若相同失效能由正常生成器流程复现，按 AC-4／AC-6 修正，不借外部写者排除条款回避。

**F-15 专项测试授权（B 本次显式调整）：** TestRecoverAmbiguousResidualIsConflictAndLeavesEverything 当前把任意外部字节写入一个正确双哈希名字；该单残留外部注入 fixture 不应反过来阻止正常半写恢复。如需调整此断言，保留旧名／旧触发条件／历史结果，明确转为外部干预诊断或按新正常识别条件细分，不称原外部保全目标已修复；以 Update 自己生成 0／半写残留的入库回归替代正常覆盖。TestRecoverTwoResidualsIsConflictAndPreservesAll、TestRecoverRefusesUnownedStagingWithoutDeleting 和无效名称／类型／路径保护仍必跑且不得削弱。本授权不允许删除无归属文件或任意 tmp。

先前合同调整不产生产品通过结论。旧报告／复审详见 `483069c2182e69a19fa8fd1ef605367df0495063:TASK.md`（包含历次候选和验证索引）；前次 B 技术复审记录为 `e42f68644b7d7a0b16b5df62c5c6b59467df8927`，报告校正见 `e474e64d8a895283b4641a980ce8523e2b17b190`。既有本机证据 `artifacts/validation-evidence/task-1.2-b-rereview-20261018-02/` 保留，不冒充新合同验证。

## Non-goals

- 不实现 Mirror 删除／移动旧路径清理、连续缺失确认、serve／定时、正式容器发布或规模性能承诺。
- 不运行 ffprobe、不下载媒体、不操作 Jellyfin／代理／插件内部接口或其数据库，不触及生产媒体／权限／服务或配套仓库。
- 不新增媒体版本／发现／独立恢复状态机、远端锁、每次写入前的远端复查、全库对象快照或独立 URL／SHA-1／Size 表。
- 不开发 UID／mode 强制加固、权限管理、全系统写者审计、消费者 OS 拒写实验、任意写者 inode CAS、防攻击设施、自动 chmod／chown／ACL／挂载修复或通用再认领。
- 不把合成 API／testhooks／交叉构建当真实 OpenList、固定插件解析、Jellyfin 播放或 ARM64 实机 PASS；不把正常失败恢复降为一律人工处理。

## Slice 全部 Required Outcomes / Surfaces 复核

| RO | 当前判断与剩余 |
|---|---|
| RO-1 配置／入口／摘要 | 配置／CLI／摘要基线保留；F-15 重试失败、F-16 错误成功阻断闭环 |
| RO-2 API／元数据／稳定 URL | 夹具能力保留；真实 OpenList／无签名 /d/ 实证仍 NOT RUN |
| RO-3 映射／筛选／冲突／路径 | 既有映射／保护能力保留，不强求任意外部替换绝对保全 |
| RO-4 协议／比较／原子写入／最小归属 | F-14 修正有效；F-15 半写恢复、F-16 目录 sync 阻断；固定插件解析 NOT RUN |
| RO-5 互斥／有界工作／dry-run／证据 | 独立有界枚举与既有隔离证据有效；正常中断／恢复持久化仍不完整 |

无 Task PASS，不创建新 Task／Closure Packet；Slice ACTIVE、A Final Review PENDING。修后 PASS 时 B 再核对全部 RO 与真实 OpenList／固定插件解析缺口，不自动进入后续 Slice，不代 A 关闭。

## Developer Completion Report

- Changed Files / Implemented:
  - `internal/securepath/securepath_linux.go`: replaced `Dir.ReadDirNames` (shared-cursor and unbounded `Readdirnames(-1)`) with `Dir.OpenDirReader`, which `openat`s `"."` relative to the bound descriptor with `O_RDONLY|O_DIRECTORY|O_CLOEXEC|O_NOFOLLOW` to obtain an independent open file description; open errors are returned upward and the caller closes it. Other `Duplicate` uses are unchanged.
  - `internal/reconciler/fs.go`: `findStagingFiles` now streams an independent reader in fixed 128-entry batches, retains at most two matching names (a second proves ambiguity), and treats any read error as an error rather than "absent". `newStagingName` now encodes SHA-256 prefixes of both the previous owned content and the new content (`stagingHashes` replaces `stagingOldHash`). `Recover` gains an explicit provably-ours path: with reliable ownership and exactly one regular residual, a name/target/staging hash match in either exchange order reclaims only the stale residual and lets normal comparison converge; unprovable residuals, unowned targets and symlink/special entries stay conservative.
  - `internal/reconciler/recover_convergence_test.go` (new) and `internal/reconciler/fs_test.go`: F-14 real-assembly regression, bounded enumeration, two-residual ambiguity, closed-directory error, multi-target/subdirectory/source enumeration, dry-run then real run, and pre-/post-exchange convergence with unchanged and changed desired.
  - `scripts/run_isolated_sync_integration.py`: `fault_recovery` adds h18 (release-binary restart with a different desired URL after an exchange SIGKILL) plus a stable re-run.
- Tests and Results / Evidence:
  - First failure recorded before the fix: `TestRecoverDetectsResidualOnNonFirstTargetInPreparedRoot` → `recover B = <nil>, want ErrStagingPresent`; log `artifacts/validation-evidence/task-1.2-convergence-20261018-dev/pre-fix/f14-first-failure.log`.
  - Full T4 on head `14119b8`: governance PASS, mod download/verify PASS, `quality.py` PASS, race `./...` PASS (8 product packages, 0 failures), amd64/arm64/testhooks builds PASS, isolated `--scenario all` PASS (9/9), `git diff --check` clean. Evidence index `artifacts/validation-evidence/task-1.2-convergence-20261018-TkmsKb/index.json`.
  - CLI three scenarios run individually before T4: `repeat_and_compare`, `dry_run_boundaries`, `fault_recovery` all PASS.
  - Real OpenList, fixed plugin parsing and Jellyfin/ARM64 execution: NOT RUN (cross build is not ARM64 execution).
- Decisions / Risks / Remaining Required Outcomes:
  - Decision: extended the deterministic staging name to encode both content hashes. This is neither ownership semantics nor new persistent state; it makes interrupted-update proof unambiguous in both exchange orders. Flagged for B review.
  - Gate hygiene: added untracked `artifacts/validation-evidence/go.mod` so the mandated `./...` gate excludes B's untracked `overlay-inputs` review scratch; no B evidence file was modified.
  - Remaining: real OpenList / fixed plugin parsing / Jellyfin evidence and final B independent verification.
- Head SHA: 14119b8389fdc76aa35df5ed7ba3e297810c11c5

## B Review Result

```text
Reviewed: 15d749b3f151e94a6fb5e9850e6e21d7101aa603..14119b8389fdc76aa35df5ed7ba3e297810c11c5
Decision: FIX REQUIRED
Slice Required Outcomes all satisfied: NO
Next: DEVELOPER FIXES F15/F16 IN TASK 1.2; RETAINS F14 REGRESSION; RERUNS T4
```

- B 独立审查以 bfdceed5b3c564ca92aef47035dcced314f24d45 合同为 authority；同 Task／Base／High／T4。F-14 与完整 staging 的相同／改变 desired 恢复通过，不抵销 F-15／F-16。
- 独立结果：常规治理 PASS；Go 五包 -count=1 PASS；amd64 发布／review-only overlay 构建 PASS。F-15 Go 四组合 FAIL，实际 SIGKILL 后发布二进制八次重试均 FAIL（旧目标／归属／round 安全保留，但不收敛）；F-16 一例 FAIL（sync 接缝 0 次、错误提交 round）。overlay 仅位于 ignored artifacts，未改产品代码；不称无插桩程序恰好命中窗口，不涉及外部目标／暂存替换。
- 原始证据：artifacts/validation-evidence/task-1.2-b-convergence-review-20261018-bc87Qy/index.json；overlay、反例脚本、日志、构建输入均保留。首轮 Python 准备转义错误、SIGSTOP 屏障未可靠停住的 harness 失败保留，修正屏障后才统计四组合结果。
- 核对 Developer 候选 T4：quality 含 Python 48 项及 Go／vet／build；race 为 174 测试事件＋8 包通过，0 fail／skip；隔离 all 9/9、双架构及 testhooks 构建，日志／三二进制 SHA-256 匹配。14119b8..abdc6e5 仅报告文档变更；ignored evidence/go.mod 隔离旧 B overlay scratch 的事实显式保留，不是产品依赖。原 T4 未覆盖新反例，不代表 Task PASS。
- B 确认阻断后不重跑完整 quality／race／arm64／all；顺序 CLI repeat_and_compare、dry_run_boundaries、fault_recovery 独立复跑 3/3 PASS（新构建发布二进制＋已核对哈希的候选 testhooks）；记录见同索引，不以未执行写 PASS。真实 OpenList、固定插件解析、Jellyfin／代理、ARM64 实机及性能仍 NOT RUN；无生产／配套仓库改动，无 push。
