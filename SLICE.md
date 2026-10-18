# Slice 1 — 安全的单轮生成闭环

```text
Slice ID: 1
Owner: A
Status: PASS / CLOSED
Base SHA: f90f9560e55b07fd2d6a20e8bac43741c39176c4
Implementation Head: 248a609eb70dbb8591091463ed51c5146d7c378a
```

## 用户目标

管理员对一个 OpenList 源运行一次同步或预览，得到可被 V4.3 插件读取的稳定地址 STRM，并能理解失败原因。

## Required Outcomes / Surfaces

- RO-1：目标 YAML、环境变量 token、sources 校验；`strmgen sync` 和 `sync --dry-run` 的明确成功／失败退出状态及脱敏摘要。
- RO-2：OpenList list／get 的分页、应用层错误、可用元数据与缺失信息处理；稳定 `/d/` URL 的逐段编码，不复制 raw_url／sign。
- RO-3：目录镜像映射、扩展名筛选与冲突报告；中文、空格、`#`、`%` 等名称处理；路径及符号链接边界检查。
- RO-4：版本 1 解析／序列化、现有文件比较、同目录原子写入；首次遇到未归属既有 STRM 不覆盖。SQLite 建立最小归属与可恢复写入顺序。
- RO-5：单轮互斥、有界目录工作与故障反馈；遵循 SW-1..SW-6 正常单写者与最终一致性边界，说明消费者只读的使用方式；dry-run 无持久化副作用。提供可重复的临时根集成证据。

## 约束与不变量

遵守 [需求](docs/requirements.md) 的 R-01..R-12 与 AGENTS。无删除行为；不把共享目录旧文件自动认领。原子写入与数据库提交跨资源失败要能安全恢复。初版从同步／持久化闭环开始，不按类文件拆任务。

本地写者边界见需求 SW-1..SW-6：正常使用时生成器单写、消费者只读，多生成器共享互斥资源；不对绕过锁的外部程序在任意 syscall 间隙改写／替换本地对象作绝对安全承诺。真正未归属文件保护、程序自身失败恢复、路径边界、dry-run 和完整轮规则不变。已拥有、可再生成输出的可识别偏差按既有比较规则后续收敛；正常中断不能普遍退化为永久冲突。

### A 合同澄清修订决定（2026-10-18）

用户已授权 A，并进一步确认原架构只要求正常运行与后续同步的最终一致性，不要求完全保护任意外部写者。本修订纠正 `66facd2ecf02fcd8f296e4026025ab1468a731af` 中超出该意图的强制 UID／mode 检查、消费者 OS 拒写证据和外部残留永久保护要求；不是把这些新增安全工程继续转交 Developer。外部写者反例不得仅换成静态 legacy 初始状态继续充当绝对门槛；正常生成器失败／恢复缺陷与 F-14 仍须验证和修正。

Slice ID／Base／ACTIVE 不变，原始方案不变；本合同先提交，再由 B 对 Task 1.2 做合法原地调整和测试映射。撤回过度要求不等于产品修好或旧 FAIL 变 PASS，Implementation Head 不变，完整 T4 不降低。

## 排除范围

Mirror 删除／移动旧路径清理、定时常驻、正式容器发布、百万级保证、真实 Jellyfin 播放验收；这些由后续 Slice 承接。实施单元由 B 按本合同规划，不因激活扩展到后续 Slice。

不开发抵御任意不合作写者的文件系统仲裁、按 inode 条件操作、UID／mode 加固、全系统写者审计或自动权限／容器修复；不新增确认开关、强制权限矩阵／只读消费者 OS 实验或独立恢复／媒体版本状态机。

## 验收与最终验证

将一次真实本地临时根同步与 dry-run 作为入口，覆盖分页、应用错误、路径冲突、符号链接、无效哈希、无可比信息、相同 SHA-1 的 Size 异常、URL 改变、写入失败／恢复和跨进程互斥。由 B 在激活后以真实代码与风险填写准确命令；并发／持久化门槛为 T4。真实 OpenList 和插件解析实证不能由 Python 夹具替代。

另按 SW-6 验证：相同锁资源下多生成器互斥，具有可靠归属的更新在无外部干预的失败／取消／进程中断后可经成功同步恢复；扫描后远端变化按现有比较规则下轮收敛；稳定重复运行跳过；prepared root 多目标、重复查询和残留发现不漏检且枚举有界。真正未归属文件不覆盖，实际 I/O／权限错误不得误报完整成功轮。任意外部写者反例按旧／新边界如实映射，不称通过或隐藏；不要求新增权限强制校验或消费者身份隔离证据。首次创建在发布后、归属登记前中断仍遵循 R-07 保留未归属冲突，不为消除此跨资源窗口另造认领机制。以上夹具不替代真实 OpenList／插件解析实证。

## Closure Packet

2026-10-18 A 已审查并接受本 Closure Packet；Task 1.1／1.2 均有独立 B PASS。

- **Reviewed range:** `f90f9560e55b07fd2d6a20e8bac43741c39176c4..248a609eb70dbb8591091463ed51c5146d7c378a`。
- **Task index:** Task 1.1 historical B PASS (`951aaeeeca966e8ef79e8ac9500827ce27d0f035..15d749b3f151e94a6fb5e9850e6e21d7101aa603`); Task 1.2 B PASS (`15d749b3f151e94a6fb5e9850e6e21d7101aa603..248a609eb70dbb8591091463ed51c5146d7c378a`).
- **RO-1:** PASS in local CLI/config/error and dry-run evidence.
- **RO-2:** PASS in bounded list/get, pagination/application-error, metadata and stable URL implementation evidence; real OpenList and fixed plugin parsing remain `NOT RUN` and are not replaced by fixtures.
- **RO-3:** PASS in mapping, extension filtering, special-name, conflict, path and symlink-boundary evidence.
- **RO-4:** PASS in protocol/compare, atomic update, ownership, SQLite ordering, F-15 recovery convergence and F-16 directory-sync failure evidence; unowned and ambiguous residuals remain protected.
- **RO-5:** PASS in shared-lock, bounded enumeration, dry-run, failure feedback and isolated temporary-root evidence; F-14 enumeration regression and normal single-writer SW-1..SW-6 recovery are covered.
- **Invariants:** No Mirror cleanup, serve, permission hardening, media-version state, remote lock, ffprobe, Jellyfin database, plugin-internal API or production-tree changes. Owned update staging cleanup is part of normal recovery. External integration, ARM64 execution, performance and release packaging are deferred/`NOT RUN` at their defined roadmap layers.
- **Evidence:** B independent T4 index at `artifacts/validation-evidence/task-1.2-b-review-20261018-3rVdg8/`; Developer T4 and first-failure evidence remain at `artifacts/validation-evidence/task-1.2-convergence-20261018-dev2/`.

### Slice Final 与证据复用

- A 审查完整 Base..Implementation Head 的配置／CLI、API、协议／比较、路径、SQLite、互斥、原子创建／更新／恢复、测试及工具变更；未只审最后一次修复。最终合同 authority 为 `bfdceed5b3c564ca92aef47035dcced314f24d45`，没有改写 RO、安全不变量或验收含义。
- A 在干净的 `315cc6fd22798c4c085f990fb2d223ce313811cf` 执行治理、quality（48 项 Python、Go 格式／vet／test／build）、F-14～F-16 及未归属／歧义残留聚焦回归、隔离 CLI `--scenario all`（9/9）和 diff check，全部 PASS。原始命令、退出码、耗时和二进制摘要见 `artifacts/validation-evidence/slice-1-a-final-20261018-ksy6lbym/index.json`。
- B 的完整 T4 二进制构建于干净的纯文档后继 `2b51e74486414c1e109b9b5943be7fc00395c5a7`。A 核对它与实现 Head、审查入口之间的源码／测试／依赖／夹具／工具输入一致，并核对二进制 SHA-256；复用 B 的 mod download／verify、race 和双架构构建证据，不宣称 A 重跑了这些门槛。race 原始记录为 **180 个测试＋8 个包**通过事件，0 fail／skip；不是 188 个测试。
- 真实 OpenList／无签名 `/d/`、固定插件实际解析、Jellyfin／代理、ARM64 实机、性能及正式发布仍为 **NOT RUN**。本次接受本地生成闭环及协议实现证据，不把合成 API、进程重启或交叉构建当作外部兼容／播放证明；外部验证保持既有路线图层级。
- 仓库文件与可达提交主题未找到独立 V2 需求扩展；未推断其内容，也未将其列为已审查输入。本次结论限于已提交需求及当前 Slice 合同。

### 非阻塞事项与延期

- P2：恢复枚举虽按 128 项分批且独立游标，但每个目标仍遍历所在目录；宽目录成本留给后续实际性能测量，不作十万／百万或 512 MB 承诺。
- P3：部分历史代码注释仍用绝对措辞描述外部替换保护；交付边界以 SW-1..SW-6 为准，F-10／F-13／外部来源 F-11 的历史 FAIL 未改称修复或 PASS。
- Mirror 删除／旧路径清理、serve／定时与发布部署保持延期。当前没有 Mirror 旧 STRM 删除分支；未来不存在确认仍须满足 R-08，未使用的 `openlist.IsNotFound` 字符串判断不能直接作为权威缺失证据。

### Closure Reconciliation

本关闭检查点同步 SLICE、无活动 TASK、README、架构 CURRENT、Roadmap、Progress、范围／用户旅程、兼容性／测试矩阵、运行说明和角色经验；Base 与已审查 Implementation Head 保留。原方案、产品实现、测试、依赖和夹具未变；本地 ignored 原始证据保留，不宣称远端可读。后续 Slice 均未激活。

## A Final Review

```text
Reviewed Range: f90f9560e55b07fd2d6a20e8bac43741c39176c4..248a609eb70dbb8591091463ed51c5146d7c378a
Decision: PASS
P0/P1 Blockers: NONE
Next Action: A SELECTS THE NEXT LARGE SLICE
```

A 接受 RO-1..RO-5 的本地合同证据；必要安全不变式、正常单写者恢复和完整轮规则满足本 Slice。P2／P3 与分层 NOT RUN 不单独阻止本次 PASS。Slice 1：**PASS / CLOSED**。
