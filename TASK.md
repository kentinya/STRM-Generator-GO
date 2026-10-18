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

- Task 1.1 的历史 B PASS 范围是 `951aaeeeca966e8ef79e8ac9500827ce27d0f035..15d749b3f151e94a6fb5e9850e6e21d7101aa603`。其创建／归属／互斥／dry-run 能力继续复用；首次发布后尚未登记归属的中断窗口仍保留未归属冲突，不另造自动认领机制。
- 最新产品候选仍为 `d0e2bacfb824b4876c647059e43c2996a9339417`，不是本轮文档提交。现有 `LocalFS.Update` 使用同目录确定性暂存、RENAME_EXCHANGE、校验与回滚；`Recover` 在比较前执行；SQLite 仍只存最小归属／轮次。上述能力不是待从零重建的模块。
- **F-14 已确认未修：** `securepath.Dir.Duplicate` 使用 dup 共享 open file description，`ReadDirNames` 在该描述上调用 Readdirnames(-1)；`findStagingFiles` 因而既一次装入整目录，又会因共用游标漏检后续文件的残留，错误允许完整 round。相关枚举／恢复接入始于 `f16ea0f176334c9eb5ebf4eb13d0d63970acc5b9`。当前 Task 必须修正并补真实装配回归。
- **正常恢复收敛待实证：** 当前 Recover 的 target==desired／staging==desired／两侧都不同等分支，须覆盖正常中断和重启前远端 desired 变化；从代码风险不能直接宣称已经复现或已经修好。若无外部干预的正常流程产生永久冲突或错误成功，仍在本 Task 修正。
- 历史 F-10／F-13 和外部来源 F-11 按末尾映射重新分类；既有失败日志不改写为 PASS，也不因收窄范围而抹去。没有新产品实现／新独立 B PASS；Task 保持 FIX REQUIRED，A Final Review PENDING。

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

1. **记录当前失败。** 先执行治理，记录真实源码起点和新证据目录；使用现有 PrepareRoots／Recover 接口构造同根 A、B 两个目标，把残留安排在非首个目标，连续查询和重复查询。检查退出码、残留、ownership、round，保存修前结果；不要只断言返回 error 或绕过 PrepareRoots。
2. **让每次枚举独立。** 在 securepath 为目录枚举提供一个小接缝：从已绑定目录 fd 相对 openat 点目录，取得新的 open file description；使用 O_RDONLY／O_DIRECTORY／O_CLOEXEC／O_NOFOLLOW，不绝对路径重开、不在共享 fd 上 Seek，不改其他无需枚举的 Duplicate 用途。打开／读取／关闭失败向上返回，自己打开的 fd 在各退出路径关闭。
3. **分批找残留。** Readdirnames(n) 固定小批次（如 128），不能先 Readdirnames(-1) 再切片。findStagingFiles 最多保留两个匹配名，第二个即可判歧义；只有完整 EOF 能判 0 或唯一。读取错误／自设限额到达不能等同不存在，不推进完整 round。不要缓存整个本地目录或为每个文件开 goroutine。
4. **核对正常恢复分支。** 复用现有故障接缝，测试更新写入后／交换前、交换校验前、发布后 round 前的失败／SIGTERM／SIGKILL；重启先用相同 desired，再测远端 URL 或可比较元数据在重启前变化。不得插入外部文件替换。若既有可靠归属的正常状态被永久冲突卡住，在现有恢复流程中做最小修正；不把全部残留直接删除，也不把全部恢复永久失败。无 DB／无可靠归属或真正歧义仍保守保留；若只有改变 ownership 语义／新增持久状态才能满足，给 A 最小状态对与合同矛盾，不自行扩张。
5. **补收敛与回归断言。** 证明第 1 步的多目标／重复枚举稳定检出，补子目录、多个独立 source、dry-run 后真实运行、发布二进制重启和大量无关条目。扫描后远端变化用既有可控 API 夹具验证后续按比较规则更新；收敛后再跑一次应跳过、不改 bytes／mtime。实际 I/O／权限错误仍非零且不提交 round，不做新的权限安全矩阵。
6. **匹配候选再送审。** 先顺序执行聚焦五包与 CLI 三场景，再对最终候选跑完整 T4。全部新增回归／hook 入库，生产二进制不含测试开关。提交真实 Implementation Head，完整记录首次失败和重跑，填完成报告，交 READY FOR B REVIEW；不得自签 B PASS／A PASS。

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

先跑新增 F-14 和正常恢复回归，然后执行：

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

独立复现 F-14 真实装配、URL／SHA-1／Size 更新、稳定零写、dry-run、正常中断重启和 desired 变化后的收敛；核对全部目标／暂存、ownership／round、退出码及锁释放。未归属保护／真实 I/O 错误继续核对，不要求不同 UID 消费者实验。按下面映射区分外部干预诊断和正常故障回归。

每个候选记录完整源码 SHA、未提交输入摘要、命令／cwd／版本／依赖／fixture 身份、退出码／计数／耗时、原始日志、首次失败和重跑。源码、测试、依赖或运行输入改变后，最终 T4 重新匹配候选。旧相同输入的事实可引用，但旧 T4 不能证明修后新候选已通过。NOT RUN／BLOCKED／SKIPPED 不写 PASS，本机 ignored evidence 不宣称已远端交接。

## B 合同后调整决定与测试映射（当前有效）

初始干净工作区常规治理 PASS。A 先提交 `bfdceed5b3c564ca92aef47035dcced314f24d45`，治理随后正确拒绝旧 Task checkpoint；本 Task 更新 checkpoint 后须恢复 PASS 才有实施准入。不更改治理脚本绕过顺序。维持同一 Task／Base／High／T4／FIX REQUIRED，不按权限字段、模块或测试分拆工作。

| 发现／用例 | 本轮明确处理 | 继续验证的合同内行为 |
|---|---|---|
| F-10：最终检查后外部换名，再按名清理 | 不再是任意外部写者绝对保全门槛；保留历史 FAIL，不称已修复 | 正常更新／回滚／暂存清理、系统调用失败、完整轮与真正未归属保护 |
| F-13：最后校验后外部替换 staging 再发布 | 排除任意替换窗口的绝对防护；不要求 inode CAS 或更多轮检查 | 正常暂存完整性、原子发布、失败／取消；已有低成本替换检测回归可保留 |
| F-11：外部注入内容经恢复后的多次运行被覆盖 | 原外部来源序列不再强制三次永久冲突；不换成静态 legacy 继续要求无损保全全部注入字节 | 无外部干预的正常恢复不丢必要依据、不错误成功且后续收敛；真正无归属／类型／路径冲突保留 |
| F-14：prepared root 共用目录游标漏检后续目标 | 与外部干预无关，继续应修／AC-9 | 独立流式枚举、非首个目标残留、重复／交替／多 source、EOF／错误、有界内存 |
| F-12：内部 I/O／发布／回滚／目录 sync／信号故障 | 已补覆盖保留，正常故障门槛不降低 | 目标完整性、DB／round、锁释放、正常重启；不能把观察屏障当持久化证明 |

**现有用例的具体授权：** `TestRecoverRestoresForeignDisplacedContent`、`TestForeignInterruptedUpdateRecoveryConflicts`、`TestValidForeignInterruptedUpdateRecoveryConflicts` 以及 CLI `update_before_verification_foreign`／`update_before_verification_valid_foreign` 的现有有益防御可继续保留，不要求扩展为任意外部内容跨三轮永久保全。若正常恢复修正使其断言需调整，B 允许明确分离为外部干预诊断与正常流程的恢复／未归属回归，报告原名、原触发条件、调整原因和替代断言；不得批量删除、只加 skip 或把已知 FAIL 标 PASS 来凑全绿。`TestRecoverRefusesUnownedStagingWithoutDeleting` 等真正未归属用例不因该分类被删除或削弱。若相同失效能由正常生成器流程复现，按 AC-4／AC-6 修正，不借外部写者排除条款回避。

本轮文档调整不产生产品通过结论。旧报告／复审详见 `483069c2182e69a19fa8fd1ef605367df0495063:TASK.md`（包含历次候选和验证索引）；最新 B 技术复审记录为 `e42f68644b7d7a0b16b5df62c5c6b59467df8927`，报告校正见 `e474e64d8a895283b4641a980ce8523e2b17b190`。既有本机证据 `artifacts/validation-evidence/task-1.2-b-rereview-20261018-02/` 保留，不冒充新合同验证。

## Non-goals

- 不实现 Mirror 删除／移动旧路径清理、连续缺失确认、serve／定时、正式容器发布或规模性能承诺。
- 不运行 ffprobe、不下载媒体、不操作 Jellyfin／代理／插件内部接口或其数据库，不触及生产媒体／权限／服务或配套仓库。
- 不新增媒体版本／发现／独立恢复状态机、远端锁、每次写入前的远端复查、全库对象快照或独立 URL／SHA-1／Size 表。
- 不开发 UID／mode 强制加固、权限管理、全系统写者审计、消费者 OS 拒写实验、任意写者 inode CAS、防攻击设施、自动 chmod／chown／ACL／挂载修复或通用再认领。
- 不把合成 API／testhooks／交叉构建当真实 OpenList、固定插件解析、Jellyfin 播放或 ARM64 实机 PASS；不把正常失败恢复降为一律人工处理。

## Slice 全部 Required Outcomes / Surfaces 复核

| RO | 当前判断与剩余 |
|---|---|
| RO-1 配置／入口／摘要 | 既有实现保留；F-14 与正常故障退出码／partial／round 待本 Task 复验 |
| RO-2 API／元数据／稳定 URL | 夹具能力保留；真实 OpenList／无签名 /d/ 实证仍 NOT RUN |
| RO-3 映射／筛选／冲突／路径 | 既有能力保留；按新合同核对基线边界，不强求任意外部替换绝对保全 |
| RO-4 协议／比较／原子写入／最小归属 | 本 Task 尚未接受；F-14、正常失败恢复和重试收敛待完成；固定插件解析仍 NOT RUN |
| RO-5 互斥／有界工作／dry-run／证据 | 本 Task 尚未接受；独立有界恢复枚举、故障及无写验证待完成 |

Task PASS 后 B 再逐项核对全部 RO，满足则停止创建 Task，完成最终验证和 Closure Packet 交 A；本轮没有 Task PASS／Closure Packet，不代 A 宣布 Slice 关闭。

## Developer Completion Report

上一候选和验证结果保留在上述历史检查点。以下仅填写按新合同完成的实际新修正；未做不得复用为新完成报告。

- Changed Files / Implemented：待修正。
- Tests and Results / Evidence：澄清合同下新候选产品验证 NOT RUN。
- Decisions / Risks / Remaining Required Outcomes：F-14、正常更新／恢复收敛与全部 AC 待完成／复验；真实联调缺口如上。
- Head SHA：NOT SET（待 Developer 提交真实新实现；此前产品候选仍为 d0e2bacfb824b4876c647059e43c2996a9339417）。

## B Review Result

```text
Reviewed: 15d749b3f151e94a6fb5e9850e6e21d7101aa603..d0e2bacfb824b4876c647059e43c2996a9339417
Decision: FIX REQUIRED
Slice Required Outcomes all satisfied: NO
Next: DEVELOPER FIXES F14 AND VALIDATES NORMAL RECOVERY IN TASK 1.2
```

保留上一候选的 FIX REQUIRED，不自签新产品审查。A 澄清后的新实现尚未送审；本次仅为 B 合法调整当前 Task 和测试计划，不是独立产品 PASS。
