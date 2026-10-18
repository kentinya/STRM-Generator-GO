# Slice 2 — 安全目录镜像与清理恢复

```text
Slice ID: 2
Owner: A
Status: READY FOR A REVIEW
Base SHA: 930782035615512be811a7b8bcc8416050f0c12f
Implementation Head: d0a0f6db5447f38467c67612ed7173d77e6bea27
```

## 用户目标

管理员继续使用 sync／dry-run。所有目录列表请求关闭强制刷新；配置源根未成功读取为目录时保留该源 STRM，不增加缺失次数。源根可读取时，对正常对账未观察到的已拥有路径用 Get 判断，未读到即计缺失，不再区分其他原因造成的相同响应。默认连续三次完整轮缺失并按相同规则重核后，清理可靠归属和输出边界内的 STRM；用户接受源根可读时目标读取异常也可能进入删除计数。原协议、本地保护、dry-run 与正常恢复不变。

## 已接受基线与本轮选择

2026-10-18 A 在独立于 Slice 1 关闭的回合选择本 Slice。Base 是选择前真实 HEAD，也是 Slice 1 的关闭检查点；固定后不随合同／Task 文档提交移动。

Slice 1 已 PASS / CLOSED，审查范围为 `f90f9560e55b07fd2d6a20e8bac43741c39176c4..248a609eb70dbb8591091463ed51c5146d7c378a`。完整关闭合同与 Closure Packet 保留于 `930782035615512be811a7b8bcc8416050f0c12f:SLICE.md`；Task 1.1／1.2 均有独立 B PASS。已接受配置、API／协议／路径、增量比较、原子创建／更新、最小归属、正常中断恢复、dry-run 与互斥，不重新为这些能力单设增量 Slice。

真实代码仍拒绝 `cleanup.enabled: true`；ownership 只有本地路径、创建轮和未推进的 missing_count，尚无精确远端媒体路径映射和已观察集合；List 固定 `refresh: false`，`openlist.IsNotFound` 的字符串判断没有权威缺失证据。现有成功轮不能倒推历史缺失。本 Slice 承接原路线图的 Mirror 缺口，保留旧 Slice 1 结论和原始失败记录，不预设新能力 PASS。

## Required Outcomes / Surfaces

- **RO-1 配置与操作入口：** 沿用 sync／dry-run；`cleanup.enabled` 默认 false，只有显式 true 才允许清理。`sync.mirror` 默认 true，false 时只生成／更新；false 与 cleanup.enabled true 为矛盾配置，在输出／数据库变更前拒绝。`cleanup.missing_confirmations` 默认 3，只接受整数 ≥3；提高阈值只延后清理。没有 force／单轮删除捷径。摘要区分实际删除、预览候选及保留原因，不把待确认候选显示为已删除。
- **RO-2 完整对账与连续缺失：** 以 source 为边界，有界处理目录、全部分页与必要本地生成／更新。源根 Get 未成功返回目录时保留该源已有计数及 STRM，不清理；移除 source 也不清扫。观察到文件（含 skip）或 Get 成功读到目标使旧缺失清零。源根可读取、目标 Get 未读到按缺失观察，其他原因不再细分；每个完整轮最多增加一次，与成功轮一致提交。目标 Get 失败本身不再阻止该轮完成，但列表不完整／失败、本地必要处理失败或整轮取消仍不提交新增计数。关闭清理不新增计数／删除，再出现仍可清零；dry-run 不持久化。不同 source 不借用证据。
- **RO-3 简化 Get 判定与本地删除：** 使用已提交的至少三次完整轮缺失证据，删除前按当前相同规则重核源根可读取为目录且目标 Get 未读到；不要求完整刷新列表、祖先链或独立权威不存在证明。所有列表均 refresh:false。目标 Get 的具体失败原因不再区分；成功读到目标或明确类型冲突则保留。可靠归属、精确远端／本地映射、配置输出边界、regular STRM 及未归属保护仍须满足。准确规则以需求“简化删除判定”为准。
- **RO-4 移动与重命名闭环：** 新路径按已接受生成规则交付；旧路径独立经过三轮缺失和再次确认后清理。新路径必要处理失败时，该源不能据此清理旧路径。相同输出目标的合法 URL 改变继续按原比较规则更新，不把扩展名替换误当作同轮缺失；同名目标冲突明确失败。稳定输入下安全条件具备后，正常后续同步完成旧 STRM 和归属记录收敛。
- **RO-5 最小持久化与正常恢复：** 在既有 SQLite 归属／路径映射／完整轮次／缺失次数范围内补齐所需事实，有界查询，不复制媒体指纹、不新增发现或媒体版本状态机。兼容 Slice 1 数据库，保留旧归属且不把旧 round／created_round 当作缺失证明；后续观察可补齐精确远端映射，历史缺失且无法可靠补齐的记录保留并报告限制，不猜扩展名或自动再认领。覆盖删除、目录 sync、DB 提交、取消／进程中断前后的重试；孤儿记录不授权删除未知对象。文件与 DB 不是一笔事务，失败恢复不依赖手改计数或盲删残留。
- **RO-6 预览、可观测性与分层证据：** dry-run 不改 STRM、原数据库、轮次或缺失计数，不迁移旧库。摘要区分缺失观察、保留、预览候选和实际删除。取得固定隔离 OpenList 的源根／目标 Get 与缓存列表实际证据，所有列表 refresh:false；不将错误码当作已辨别真实缺失的证据。旧同码／权限／缓存反例保留为已接受局限，不签虚假安全 PASS。稳定 /d/、插件解析及播放仍独立验收。

## 约束与安全不变式

遵守 [需求 R-01..R-12、SW-1..SW-6](docs/requirements.md)、[用户旅程](docs/product-experience.md)、[架构](docs/architecture.md) 和 AGENTS。协议、URL → SHA-1 → Size 比较、未归属保护、路径／符号链接边界、单轮互斥、目录并发与资源上限均不得回退。所有接触同一输出的生成器共享同一互斥资源；消费者只读。不新增 UID／mode 加固或任意外部写者的绝对防护承诺。

本源列表遍历／必要本地处理失败、分页不完整或整轮取消时不推进新增缺失次数，不启动该源清理；目标 Get 未读到按 RO-2 接受为缺失观察。源根判定失败始终保留，其他 source 不受其证据借用。每次删除仅消费已提交的计数，不能借尚未完成的本轮凑次数；稳定且本地条件有效时至迟在达到阈值后的下一次成功同步收敛。所有列表请求 refresh:false，不追加强刷或权威列表证明来收紧用户选择的 Get 规则。

清理多个文件不承诺全源跨文件原子回滚。每次删除均先满足上述证据与检查；后续删除／目录 sync／DB 失败时停止该源后续清理，不追加失败轮的缺失依据，报告已经发生的操作与未完成状态。已合法删除部分不伪称回滚；重启须重新核实远端和本地事实，安全完成剩余删除／记录回收，或在远端再出现时重新生成。不能为追求全部回滚另造文件发现／事务状态机。

仅操作本仓库 artifacts 下的临时输出、状态和新建隔离服务／夹具；不修改 `/opt/jf-strm-probe`、生产 OpenList、Jellyfin、代理或生产媒体。新 Get 判定及缓存行为若尚无实证，RO-6 保持 NOT RUN／BLOCKED，不能据旧强刷候选或合成 API 宣称新产品通过。

## 排除范围与取舍

不重做 Slice 1 增量比较／正常更新恢复；其回归仍是本 Slice 门槛。旧空目录可保留，不做目录树、未知暂存或未归属文件的递归清理。不提供数据库丢失后的自动认领、源身份变化后的归属转移或人工强制删除工具。

serve／定时、正式 Docker／二进制发布、全驱动兼容承诺、固定插件实际解析、Jellyfin／代理播放及 ARM64 实机留在后续路线图。真实 OpenList 的范围仅包含本 Slice 缺失确认所需的固定隔离版本／驱动，不借此宣称端到端通过。十万／百万、512 MB 与宽目录枚举性能等待实际测量；撤下上一版强刷批次／证明复用／请求预算的新增能力，本 Slice 仍必须证明新增路径有界。扫描期间远端变化按下轮纠正，不引入远端版本锁或每次写入前的额外查询。

## 验收与最终验证

本 Slice 为删除／持久化／并发 T4。B 在已提交 ACTIVE 合同后，依据真实实现制定准确 Task、命令和证据计划；A 不在本合同预设接口、文件拆分或 Task 数量。

Slice Final 必须覆盖：

1. 既有创建／更新／比较跳过、无变化输出零写、正常恢复、特殊名称／路径冲突、未归属保护、跨进程互斥与 dry-run 回归。
2. 首次及第二次缺失均保留，三个不同完整成功轮才形成最低资格；重复消费同一 epoch、列表／本地失败、整轮取消、不完整分页／预览不能凑足次数；再出现重置，包括后续处理失败；配置提高阈值、关闭／重启用清理及多 source 证据隔离；配置源根不可用或源已移除不清扫历史归属。
3. 源根 Get 未读到／非目录时不增加缺失、不删除；源根可读取而目标 Get 未读到时按相同规则计数，覆盖实际缺失、父目录删除与其他原因返回同样错误。目标再出现清零，类型冲突保留；所有列表（含分页和 dry-run）refresh:false。整轮取消与列表失败不推进计数，不能以权限过滤／同码反例未被辨别为由重新添加权威列表门槛。
4. 旧库升级／只读预览、缺失精确映射、数据库丢失／损坏、失配或越界记录、未解决更新暂存、符号链接／特殊文件保护；历史轮次不产生删除资格。
5. 由正常生成器流程制造的删除／目录 sync／DB 失败及取消／SIGKILL 窗口，发布二进制重启后再次确认、无重复计数并安全收敛；保留首次失败。跨文件部分成功如实报告，不宣称事务回滚。
6. 新增观察／候选／数据库枚举和再次确认有界；全套已有回归、格式／vet／test／build、依赖校验、显式 CGO race、Linux amd64／arm64 构建、隔离 CLI 与适用故障恢复。ARM64 构建不是实机运行。
7. 固定隔离 OpenList 版本、驱动、源根与目标 Get／缓存列表／分页及未读到响应的原始脱敏实证；它与单元／合成夹具、插件解析、播放和性能分开报告，不接管生产实例。

证据遵循 [开发流程](docs/development-workflow.md)：准确源码／二进制／配置／夹具身份、命令／cwd、退出码／计数／耗时、原始日志、首次失败与重跑、复用输入一致性、RO 映射及 NOT RUN／BLOCKED。最终审查覆盖固定 Base..Implementation Head 全范围，P2／P3 和可选证明不单独阻止 PASS；任何必要删除安全条件不满足仍为阻断。

## A 决定：源根与 Get 简化规则（2026-10-18 用户最终指令）

用户明确要求源根缺失保留、源根存在时目标 Get 未读到计缺失，不再辨别其他原因产生的相同响应，且所有列表禁止强制刷新。A 据此实质修订 RO-2／RO-3／RO-6、需求、架构、用户旅程、AGENTS 和测试矩阵。完整语义见[需求](docs/requirements.md#简化删除判定2026-10-18-用户最终决定)。三轮阈值、完整轮的列表／本地处理要求、取消不提交、可靠归属与本地边界保留。

先前 a2a3b18 的权威列表／最终 500 有限例外及 82cceae 的按需强刷／目录证明复用／确认预算均撤回；原文、历史实现、失败与实证在可达 Git 中保留，不改称修复／PASS。用户接受目标因权限、驱动、缓存等原因未读到也可能最终删除本地 STRM，不再将排除这些反例作为验收门槛。本轮无产品实现，不自签独立 B 或 A 产品 PASS。

B 须在本合同提交后重评 Task 2.1，调整或重规划目标、接口、AC、旧新测试映射与 Contract Checkpoint；现有 Task 保持 FIX REQUIRED，旧强刷候选不能视为符合新合同。B 的适配先于 Developer 实施；Slice Base、Task Base 和已有历史身份不移动。

## Closure Packet

```text
Packet Status: READY FOR A REVIEW
Slice Base: 930782035615512be811a7b8bcc8416050f0c12f
Reviewed Implementation Head: d0a0f6db5447f38467c67612ed7173d77e6bea27
Task Index: Task 2.1 PASS at c629858ba7559a03c1eba30f8cd91a269f4b2702; Task 2.2 B PASS, reviewed c629858ba7559a03c1eba30f8cd91a269f4b2702..d0a0f6db5447f38467c67612ed7173d77e6bea27
Required Outcomes: RO-1 PASS; RO-2 PASS; RO-3 PASS; RO-4 PASS; RO-5 PASS; RO-6 PASS
Final Evidence: artifacts/validation-evidence/task-2.2-final-7TWPAV/INDEX.md (local-only); independent B replay: artifacts/validation-evidence/b-review-task-2.2-20261019/summary.json
Invariants: protocol/comparison, unowned-file protection, output boundary, dry-run zero-write, refresh:false, source isolation, bounded traversal/ownership batches, and shared output lock retained
Known limitations / deferred: /d/ playback, plugin parsing, Jellyfin/proxy, ARM64 runtime, and 100k/1M scale remain NOT RUN; wide-directory repeated enumeration cost is P2; unlink plus SQLite ownership is not a cross-resource transaction
Documentation coordination: README, architecture CURRENT, scope, product journey, test matrix, progress, environment spec, and examples updated in the Task 2.2 range; no requirement, Slice, Roadmap, or V4.3 behavior change
A Final Review: PENDING
Next Action: A REVIEWS SLICE 2 BASE..IMPLEMENTATION HEAD
```

## A 合同修订记录

以下为早前阈值修订的历史记录；远端缺失语义现按上节用户最终决定执行。

2026-10-18：用户明确授权本轮行使 A 权限，将 `cleanup.missing_confirmations` 默认值及最低允许值从 2 调整为 3。清理须依赖三个不同的完整成功缺失轮次后才具备最低计数资格；阈值提高只会延后清理一个或多个同步周期，不新增持久状态、竞态或删除捷径。其余缺失、权威再确认、归属、输出边界与恢复语义保持不变；Slice Base、Roadmap 与原样保留的 V4.3 方案不变。需求、用户旅程、架构 TARGET、测试矩阵、配置示例、AGENTS 与 README 已同步。

## A Final Review

```text
Reviewed Range: PENDING
Decision: PENDING
P0/P1 Blockers: NOT ASSESSED
Next Action: A REVIEWS SLICE 2 BASE..IMPLEMENTATION HEAD
```
