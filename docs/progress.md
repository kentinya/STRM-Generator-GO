# 进度

## Task 3.2 实现（2026-10-19，READY FOR B REVIEW）

Developer 按已提交 Task 3.2 实现 Linux amd64／arm64 部署运行包，状态置为
**READY FOR B REVIEW**（尚无 B／A 结论）。交付：仓库根 `Dockerfile`（`golang:1.27.2`
与 `alpine:3.20` 均按 digest 固定，构建层在 `$BUILDPLATFORM` 以 `CGO_ENABLED=0`
交叉编译目标架构，运行层只复制 CA 根、非特权账号文件与静态二进制）、`.dockerignore`、
`deploy/compose.yaml`（运行时环境注入 token、只读配置、输出／SQLite／固定锁分别挂载、
容器私有 `/tmp` tmpfs、非 root、`read_only`、`cap_drop: ALL`、`no-new-privileges`、
`stop_grace_period: 60s`）、`deploy/config.example.yaml`、`deploy/.env.example`、
`docs/deployment.md`、`scripts/run_container_integration.py` 与对应 Python 测试。

本机已执行：治理、新增容器驱动单元测试 13 项、amd64 `--load` 构建、arm64 OCI 构建、
隔离容器驱动 6/6（镜像静态检查与非 root 身份、最小 sync 与挂载／token、serve 首轮＋
正常停止＋重启、双容器同输出同锁竞争 busy-skip、Compose 生命周期、arm64 OCI
manifest／ELF 核对）。证据索引见本 Task 完成报告；完整 T4 门禁与 B 独立关键复现仍
待 B 执行。真实 OpenList、无签名 `/d/`、固定插件解析、Jellyfin／代理、ARM64 实机、
规模与正式发布仍 NOT RUN。

Next Action: B REVIEWS TASK 3.2

## Task 3.1 B 独立审查（2026-10-19，PASS）

审查完整范围 `ee5001646d4239755606c28b61f340d795ee7014..b18caba6fd786bfe589f6ac29c2a2e779a5a1628`；
纯完成报告后继为 `8fc3a3ad8a720e9ca4c832967fdf39874581beff`。B 核对 scheduler／config／CLI
实现、race 修正、完整 Task 测试计划及候选二进制身份；复用匹配的 T4 门禁索引
`artifacts/validation-evidence/task-3.1-final-bgG1Ej/INDEX.md`，未机械重跑全量 race／test／build。
B 独立在隔离 OpenList 屏障对运行中的首轮发 SIGINT：serve exit 1 并报告 cancelled，SQLite
round／ownership 均空；之后发布 `sync` 成功取得共享锁并创建预期输出。原有匹配证据覆盖首轮、
固定 interval、busy-skip、失败后继续、SIGTERM 取消、日志脱敏及 sync 回归。Task 3.1 PASS；
证据及审查细节已记入可达 Task 报告。真实外部服务、Docker、ARM64 实机、规模与正式发布仍
NOT RUN。RO-4 及容器生命周期对应 RO-6 未满足，因此 Slice 继续 ACTIVE，不提交 Closure Packet。

Next Action: Developer implements planned Task 3.2 after the planning checkpoint is committed

## Task 3.2 规划（历史，2026-10-19，PLANNED）

B 按当前 Slice 3 合同与真实仓库缺口规划 Linux 双架构 Docker 运行包。范围包括固定 Go 源码／
依赖的 amd64／arm64 CGO=0 镜像、非 root、环境 token、只读配置、输出／DB／sidecar／临时区／
固定锁目录的 Compose 挂载边界，以及隔离容器首轮、正常停止、持久状态重启和同一共享锁的双实例
竞争。没有产品 Dockerfile；开发 Compose 仅为假 OpenList 夹具。准确 AC、Developer 反馈和最终
T4 门禁、B 独立验证见当前 [TASK.md](../TASK.md)。此计划不授权发布 registry 或接触生产服务；
Developer 须等待 Task 计划提交并通过治理检查后实施。

Slice 仍 ACTIVE。Task 3.1 已覆盖 RO-1／RO-2／RO-3／RO-5 与调度相关 RO-6；RO-4 与部署相关
RO-6 仍待 Task 3.2。真实 OpenList、插件解析、Jellyfin／代理、ARM64 实机、规模及正式发布
NOT RUN；不得提前生成 Closure Packet 或把本 Task 计划写成容器验收通过。

Next Action: Developer implements Task 3.2 after the planning checkpoint is committed

## Task 3.1 实现（历史，2026-10-19）

Developer 按已提交 Task 3.1 实现 `serve` 常驻调度、共享单轮入口与停止恢复，状态置为
**READY FOR B REVIEW**（尚无 B／A 结论）。交付：`internal/scheduler` 有界调度外壳
（单 timer、无每 tick goroutine、无积压）；`internal/config` 新增已验证的
`IntervalMinutes`／`Interval`（缺省 60，仅正整数，拒绝非整数与转换溢出）；
`cmd/strmgen` 将单轮装配抽为 `runRound` 供 `sync` 与 `serve` 共用同一
`AcquireLockFor`、输出根准备、store 生命周期与 `Syncer.Run`；`serve` 首轮立即执行、
按固定间隔触发、busy-skip 不进入 Reconciler、单轮失败后继续、SIGINT／SIGTERM 非零
cancelled 退出；`serve_hook_integration.go` 仅为 testhooks 构建提供间隔缩短。

本机已执行：治理、受影响 Go 包与新增 scheduler 单元测试、Python 驱动单元测试、
新增隔离 serve 驱动 6/6（发布二进制首轮与取消、缩短间隔与 skip、手动 sync 抢锁
busy-skip、失败后继续、SIGINT 不提交、脱敏）以及既有隔离 sync 回归。证据索引见
本 Task 完成报告；完整 T4 门禁与 B 独立关键复现仍待 B 执行。真实 OpenList、无签名
`/d/`、固定插件解析、Jellyfin／代理、Docker 镜像、ARM64 实机、规模与正式发布仍
NOT RUN；部署镜像／Compose／双架构发布包属后续 Task 3.2。

Next Action: B REVIEWS TASK 3.1

## 当前状态（2026-10-19，Slice 3 ACTIVE）

Slice 1 与 Slice 2 均已 PASS / CLOSED；Slice 2 的 A 最终审查范围和分层证据保留在其
关闭检查点。A 已激活 Slice 3「定时与部署」，Base 固定为
`ca9f91e10a0fc6c28679b5827f453e632eb9854a`，Implementation Head 为 `NOT SET`。
Task 3.1 已独立 B PASS；Task 3.2 已实现并交 READY FOR B REVIEW。

本 Slice 的 Required Outcomes 是 `serve` 首轮立即执行与固定间隔、有界 tick、手动／
定时共享互斥、忙碌跳过、取消不提交、重启恢复、Docker 隔离挂载及 Linux amd64／arm64
构建。Task 3.1 已实现调度／取消／互斥并独立 B PASS；Task 3.2 已实现双架构镜像、
Compose 与隔离容器证据，尚待独立 B 审查。真实 OpenList、无签名 `/d/`、固定插件
解析、Jellyfin／代理、ARM64 实机、规模与正式发布仍 NOT RUN；尚无 B／A 产品 PASS，
不能把本实现写成能力验收。

Next Action: B REVIEWS TASK 3.2

## Slice 2 关闭状态（历史，2026-10-19）

Task 2.1 与 Task 2.2 均经独立 B **PASS**；Task 2.2 的完整实现报告与证据索引保留于可达历史，产品 Implementation Head 为 `d0a0f6db5447f38467c67612ed7173d77e6bea27`。A 已审查完整 Base..Implementation Head，Slice 2 的 RO-1..RO-6 均满足，状态为 **PASS / CLOSED**。

本轮证据目录 `artifacts/validation-evidence/task-2.2-final-7TWPAV/`（本机 Git 忽略路径）：治理、`mod download`／`verify`、quality（Python 全量、gofmt、vet、test、build）、race（CGO＋固定 cc）、`-count=1` 全量测试、Linux amd64／arm64 与 testhooks 构建、隔离 CLI 11/11（含新增 `mirror_cleanup`／`mirror_recovery`）、集成测试二进制、固定隔离 OpenList v4.2.7 原 8 场景（11 Go 集成测试，0 skip）及新增发布 CLI Mirror 闭环、`git diff --check` 均 exit 0。真实服务层首次运行在 CLI Mirror 路径断言上使用错误本地路径而 FAIL（产品已删除但驱动期望 `Wide/E0000.strm`）；修正驱动断言后同服务版本重跑全部通过，首次失败与重跑同时保留、不粉饰。

真实 OpenList CLI 闭环覆盖生成 250 个 STRM、删除后三个不同完整轮保留、dry-run 候选零写、下一轮当前重核删除、再出现清零、禁用存储使源根不可读时保留；缓存列表轮询全部 `refresh:false`。B 复用候选匹配的 T4 全量门禁证据，并独立重放聚焦 Go 用例及隔离 CLI `mirror_cleanup`／`mirror_recovery`。A 已接受 Closure Packet；`/d/` 播放、固定插件实际解析、Jellyfin／代理、ARM64 实机及规模仍 NOT RUN。未推送。

Next Action: A SELECTS THE NEXT LARGE SLICE

## Task 2.2 规划与合同状态（历史，2026-10-19）

Task 2.1 已独立 B **PASS**，完整计划／报告／审查保留于 `c629858ba7559a03c1eba30f8cd91a269f4b2702:TASK.md`，证据索引 `artifacts/validation-evidence/task-2.1-b-simple-get-20261019/INDEX.md`。逐项重评 RO-1..6 后尚不满足 Slice，B 已运行治理并依据真实 config／storage／Reconciler／LocalFS／CLI 及运行器，规划 **Task 2.2：PLANNED／High／T4**；Task Base 固定上述审查提交，Contract Checkpoint 仍为 `57ea31b88a0dde95e486f8bac11ebe9e473f74ea`。

Task 2.2 把显式配置、精确映射／旧库兼容、不同完整轮计数、当前 Get／本地保护删除、移动、dry-run 和正常清理失败／重启恢复作为一个可审查闭环。采用第3轮提交资格、第4次成功同步重核清理；详细简单步骤、AC-1..8、旧测试调整、新增 CLI／真实服务入口和完整 T4 门槛已写 TASK.md。新入口目前尚未实现，本轮没有 Task 2.2 产品实现／测试或 PASS。Slice／Roadmap／稳定需求未改，Slice 2 保持 ACTIVE，不交 Closure Packet 或 A 最终结论。

同步 AGENTS／README／范围／用户旅程／CURRENT／测试矩阵的状态事实，不改变能力合同；SLICE 与 Roadmap 中合同修订时点的 Task 2.1 FIX REQUIRED／待适配记录保留，当前交接以本节及 TASK.md 为准。规划验证索引 `artifacts/validation-evidence/task-2.2-b-plan-20261019/INDEX.md`。/d/、固定插件解析、Jellyfin／代理、ARM64 实机和规模仍 NOT RUN。未修改产品代码、未推送。

Next Action: Developer implements Task 2.2 after governance, then returns for independent B review

## Task 2.1 B 独立审查（历史，2026-10-19）

Task 2.1 **PASS**，审查范围 `a8c2d62369a21e8d9a0c2cefec2af09927fb3286..cec07017113c9b8e2d313cc592cc22d98011b097`，纯报告后继 `8263f5d45325d3d4c5da29d04e8a53c5fa435a27`。B 独立治理、Go 两包 169 测试、Python 81、额外 review-only overlay 2 测试，以及固定隔离 OpenList 8 场景／11 测试通过；缓存 TTL 1 分钟、60.2 秒后普通列表自然更新。核对 12 项候选输入与 4 个二进制哈希，复用匹配的完整 T4：race 244 测试／8 包、双架构与旧 CLI 9/9 等。实际门禁 14 项全 exit 0；完成报告的 13 项是计数笔误，已纠正。

证据索引 `artifacts/validation-evidence/task-2.1-b-simple-get-20261019/INDEX.md`；原始 Developer 新候选与首次失败继续保留。AC-1..6 仅对无持久状态的两次 Get 判定及 refresh:false 列表通过。WebDav 自挂载文件 Get 在缓存更新前后仍未读到，隐藏／虚拟视图／同码反例仍属用户接受局限，不声称已辨别真实不存在。

Slice 2 仍 ACTIVE；逐项重评 RO-1..RO-6 后，只有远端判定及真实服务证据部分完成，配置清理、三轮计数、本地删除、移动、旧库映射／正常恢复与预览仍缺失。没有 Closure Packet 或 A 最终 PASS。下一步由 B 基于当前实际实现规划完整 Mirror 清理及恢复 Task；不改变 Slice／Roadmap／稳定需求。/d/、固定插件解析、Jellyfin／代理、ARM64 实机及规模 NOT RUN。未修改产品代码、未推送。

Next Action: B PLANS THE NEXT COHERENT TASK WITHIN SLICE 2

## 前次 Task 2.1 合同适配（历史，2026-10-18）

用户要求 B 同步 Task。B 已核对真实代码和合同 `57ea31b88a0dde95e486f8bac11ebe9e473f74ea`，决定 **Task 2.1 合法原地调整继续，不重规划**。Task 保持 FIX REQUIRED／High／T4，Task Base `a8c2d62369a21e8d9a0c2cefec2af09927fb3286` 不变，Contract Checkpoint 更新为上述 A 合同。新 TASK.md 已替换旧强刷执行清单、AC、错误／缓存测试计划，记录旧新断言与 B-1 映射；旧计划／报告／审查原文保留于 `57ea31b88a0dde95e486f8bac11ebe9e473f74ea:TASK.md`。

新 Task 仅交付源根／目标 Get 两步判定与固定真实服务实证：根不可读保留，根可读且目标未读到按缺失观察，整次调用 context 中止优先返回 Unknown。所有列表包括 Python 准备／取证均 refresh:false，确认器不列目录或查询祖先。现有 ListFresh、祖先链、目标 403 保留和缓存立即可见断言均须按新计划替换；不能将接受的错误歧义写成已安全辨别／已修复。B-1 的整轮取消缺陷仍需专门修正，父 context 有效的单请求超时则已被新合同接受为目标未读到。

Slice 2 仍 ACTIVE，Base `930782035615512be811a7b8bcc8416050f0c12f`、Implementation Head NOT SET。当前产品仍为旧候选，CLI 清理继续拒绝；本 Task 不累计缺失次数、不改库或删除文件。三次资格、本地归属／边界、完整轮与清理恢复等须在后续连贯接入中落实，RO-1..RO-6 尚未全部满足，无 Closure Packet。本次只同步 B 计划与当前交接状态，无产品实现或独立 B／A 产品 PASS。

本轮计划证据：`artifacts/validation-evidence/task-2.1-simple-get-b-plan-20261018/`。原方案身份、文档链接、固定合同／Task 祖先关系、治理与空白检查按本轮实际结果记录。产品 Go／race／CLI／真实 OpenList 本轮 NOT RUN，Developer 修改后须按新 Task 重跑完整 T4 与固定真实服务。旧 Developer `artifacts/validation-evidence/task-2.1-final-ftETbR/`、B `artifacts/validation-evidence/task-2.1-b-review-20261018/` 及三次 A 文档证据继续保留，不作为新候选验收。

Next Action: Developer implements the revised Task 2.1 and returns for independent B review

## Slice 2 激活（历史，2026-10-18）

Slice 2「安全目录镜像与清理恢复」由 A 在 Slice 1 关闭后的独立回合选择并激活为 **ACTIVE**。Base 固定为选择前真实 HEAD `930782035615512be811a7b8bcc8416050f0c12f`，Implementation Head 为 NOT SET。以下保留激活时状态：当时无活动实施 Task，下一步由 B 规划首个 Task；当前状态见本文首节。

原路线 Slice 2 的增量比较与正常恢复已在 Slice 1 接受，避免重复交付；未激活路线重排为安全 Mirror、定时／部署、端到端／规模三个后续结果。当前合同 RO-1..RO-6 覆盖显式清理配置、完整成功轮及连续缺失、精确路径／归属、当前权威再确认、移动旧路径、旧库兼容、预览和正常清理失败恢复。固定隔离 OpenList 的所选缺失确认语义实证是 Slice 2 必要安全证据；真实 `/d/`、插件解析、Jellyfin／代理、ARM64 实机与规模仍为后续独立层。

A 同步需求、用户旅程、架构 TARGET、验收矩阵与当前状态，明确清理默认关闭、计数阈值≥2、失败轮不新增证明、再出现保守失效、旧轮次不追认、旧映射缺失保留及跨文件部分清理恢复。原方案、协议／比较、安全与 SW-1..SW-6 边界不变；不新增外部写者绝对防护。详细实现顺序与 Task 由 B 决定。

激活文档验证和原始日志保存在本机 `artifacts/validation-evidence/slice-2-activation-20261018-gnh9np0u/`。原方案 SHA-256 未变，23 份 Markdown 的 66 个本地链接、合同／真实 Base／无活动 Task 预检和空白检查通过。治理回归首轮因仓库内临时夹具向上发现主 Git 而 1/14 失败；限制该夹具的 Git 查找边界后，同一测试 14/14 通过，首次日志保留。未提交合同的常规治理拒绝符合预期；提交后的常规治理结果记录于同一索引。本轮产品 Go／race／CLI／真实服务 NOT RUN，不把合同激活当 T4 或产品 PASS；未推送。

Next Action: B PLANS FIRST TASK

以下均为历史时点；当前状态以上节、SLICE 与 TASK 为准。完整 Slice 1 关闭合同保留于 `930782035615512be811a7b8bcc8416050f0c12f:SLICE.md`；Task 1.2 报告保留于 `315cc6fd22798c4c085f990fb2d223ce313811cf:TASK.md`。

## Slice 1 关闭（历史，2026-10-18）

Slice 1 **PASS / CLOSED**。A 审查完整 `f90f9560e55b07fd2d6a20e8bac43741c39176c4..248a609eb70dbb8591091463ed51c5146d7c378a`，接受 RO-1..RO-5 的本地单轮生成／更新／恢复闭环，未发现 P0/P1 阻断；Task 1.1／1.2 均有独立 B PASS。关闭时无活动 Task，尚未选择或激活后续 Slice。

A Slice Final：治理、quality（48 项 Python 与 Go 格式／vet／test／build）、F-14～F-16／未归属／歧义残留聚焦回归、隔离 CLI 全场景 9/9、diff check 全部 PASS。原始证据见 `artifacts/validation-evidence/slice-1-a-final-20261018-ksy6lbym/`。核对可执行输入与二进制身份后复用 B 完整 T4 `artifacts/validation-evidence/task-1.2-b-review-20261018-3rVdg8/` 的依赖校验、race（180 测试＋8 包通过事件，0 fail／skip）及双架构构建；A 未重复这些门槛。

真实 OpenList／无签名 `/d/`、固定插件实际解析、Jellyfin／代理、ARM64 实机、性能、正式发布仍 NOT RUN；Mirror 清理和 serve／定时未实现。宽目录重复枚举成本为非阻塞 P2，旧外部写者 FAIL 保留历史分类。未找到独立 V2 需求扩展，本次不推断额外需求。

关闭检查点同步合同、无活动 Task、README、CURRENT、Roadmap、Progress 和相关事实说明，每份角色指南各补一条有证据的方法；产品代码／测试／依赖／夹具及原方案未改。文档检查按关闭状态重跑，证据仅本机；未 push。

Next Action: A SELECTS THE NEXT LARGE SLICE

以下保留更早时点及首次失败，不覆盖后续独立审查结论。

## 前次 B 复审（历史，F-15／F-16 修正前）

B 已按已提交的 bfdceed5b3c564ca92aef47035dcced314f24d45 合同，独立审查完整 Task Base 15d749b3f151e94a6fb5e9850e6e21d7101aa603..14119b8389fdc76aa35df5ed7ba3e297810c11c5 及纯文档交接 abdc6e5。F-14 独立有界目录枚举修正有效；Task 1.2 仍 **FIX REQUIRED**：新复现 F-15（正常空／部分 staging 写入中断，发布二进制连续重试不收敛）及 F-16（Recover 交换／删除后未目录 sync，比较 skip 时错误提交 round）。细化修正步骤与测试映射已写 TASK.md，保持同一 Task／Base／High／T4，不新增任务，不改变 Slice／Roadmap／稳定需求；不要求抵御任意外部写者。Slice ACTIVE、A Final PENDING，无 Closure Packet。

独立结果：治理与 Go 五包 PASS；amd64 发布／review-only overlay 构建 PASS；既有三个 CLI 场景顺序复跑 3/3 PASS。F-15 Go 四组合与 SIGKILL 后发布二进制八次重试 FAIL，F-16 故障接缝反例 FAIL；未修改产品代码，确定性插桩仅位于 artifacts，不冒充无插桩命中。核对 Developer 原 T4（Python 48、race 174 测试＋8 包、0 fail／skip、all 9/9、双架构）；旧门槛未覆盖新反例，不代表 Task PASS。B 因阻断未重跑完整 quality／race／arm64／all。证据仅本机 artifacts/validation-evidence/task-1.2-b-convergence-review-20261018-bc87Qy/，包含 overlay／输入／首次 harness 失败与重跑；真实 OpenList、固定插件解析、Jellyfin／代理、ARM64 实机及性能仍 NOT RUN。下一步 Developer 修正 F-15／F-16、保留 F-14 并用新候选重跑完整 T4。

## 先前合同调整（历史事实）

用户进一步澄清：不要求完全保护任意外部写者，只要求正常单写者使用及后续同步收敛。A 本轮同步修订需求、架构、Slice、说明与测试矩阵，撤回 `66facd2` 中追加的 UID／mode 强制检查、消费者 OS 拒写实验及外部注入状态永久冲突门槛；原始方案与 Base 不变。F-14 与正常中断后重试收敛仍须修正／验证，真正未归属保护、路径边界、dry-run 和完整 T4 保留。只有文档修订，没有产品改动／新 PASS；Task 1.2 仍 FIX REQUIRED，Slice ACTIVE，A Final PENDING，无 Closure Packet。A 合同已先提交为 `bfdceed5b3c564ca92aef47035dcced314f24d45`；B 随后判定 Task 1.2 合法原地调整，更新 Contract Checkpoint，重写当前执行清单与明确旧／新测试映射；旧详细修复流水账保留在可达 `483069c`。

本轮文档验证：原方案 SHA-256 未变，Python 工具回归 48 项通过；未提交合同、A 已提交而旧 Task 尚未更新两次治理拒绝均符合预期。Task 更新后常规治理、diff check 及再次执行的 Python 工具回归 48 项均 PASS；此处 PASS 仅指文档／工具检查，不是产品或独立审查通过。证据仅本机 `artifacts/validation-evidence/slice-1-convergence-contract-20261018-01/`；Go／race／CLI／外部服务在本轮 NOT RUN，未把旧产品证据写成新合同 PASS。

## 历史检查点与证据（旧要求已由上述澄清取代）

- A 合同检查点为 `66facd2ecf02fcd8f296e4026025ab1468a731af`；B 随后判 Task 1.2 合法调整、继续原地修正，已同步活动 Task checkpoint／Scope／AC／旧新测试映射与逐步实施清单。Task Base、High／T4 和 FIX REQUIRED 不变，无产品改动或新 PASS。单写者合同提交后旧 Task checkpoint 曾被治理正确拒绝，B 更新后常规治理恢复 PASS；原方案 SHA-256 未变，Python 工具回归 48 项通过，产品新合同权限／恢复验证 NOT RUN。实现者下一步按 TASK 进入修正；不修改生产权限或服务。

- 用户授权本会话行使 A 权限并限制外部写者：已批准需求 SW-1..SW-6 单写者部署合同，生成器独占输出／暂存／状态写入，消费者只读，生成器共享同一锁资源；范围贯穿轮次间与恢复。不承诺抵御同 UID／root 或任意不合作写者，不追认历史失败为 PASS。原方案、Slice Base／ACTIVE、SQLite 边界不变；本轮仅协调合同与文档，不修改生产权限或产品实现。A 合同提交后由 B 合法调整 Task 1.2 及具体测试映射，状态仍 FIX REQUIRED；已知权限条件、F-14、未知恢复事实跨重启保护与完整 T4 尚待实施验证。以下 B 记录是旧合同下的历史事实，失败证据保留。

- 最新 B 独立复审（报告检查点 `e474e64`，候选 `d0e2bacfb824b4876c647059e43c2996a9339417`）：Task 1.2 继续 **FIX REQUIRED**，不新建 Task。原双替换／提前暂存替换／首次合法外部恢复通过；F-10 检查后按名清理仍删除不同存活 inode 的外部对象，F-11 第二次发布重启覆盖外部 STRM，F-13 最后证明后的替换仍误发布，新增 F-14 为根目录 dup 共享游标漏检后续文件残留并错误提交 round。聚焦五包、amd64 发布／testhooks 构建、三个既有 CLI 场景顺序执行通过；新反例失败证据在 `artifacts/validation-evidence/task-1.2-b-rereview-20261018-02/`。核对并复用 Developer T4：Python 48、race 163 测试／8 包、隔离 9/9、双架构；B 本轮未重跑完整 quality／race／arm64。详细技术修复清单见 TASK.md；Slice ACTIVE、无 Closure Packet，合同／Roadmap／需求不变。以下较早状态保留为历史。
- Slice 1 仍为 ACTIVE，Base 为 `f90f9560e55b07fd2d6a20e8bac43741c39176c4`；没有 Slice PASS 或 Closure Packet。
- Task 1.1 经 B 独立复审 PASS；Task 1.2「已拥有 STRM 内容更新与原子恢复」修正候选 `f4383f2f59adedadc9252e465c64580570a3bbcf` 经本次独立 B 复审仍为 **FIX REQUIRED**。F-10 检查后按名清理仍会删除外部对象；F-11 恢复出的合法外部 STRM 被再次覆盖并提交 round；新增 F-13 暂存对象替换可导致误发布或失败清理误删。Task Base、合同与 T4 不变，继续原地修正。
- B 用仅加入暂停/显式放行屏障的 Go overlay 确定性复现:F-10 回滚会将第二个外部目标换入 tmp 并删除;F-11 交换校验前 SIGKILL 后,发布二进制重启错误地跳过新目标、提交 round 2,陌生原目标留在隐藏 tmp。F-12 要求补齐目录 fsync、发布/回滚失败与更新路径信号窗口证据。详情和步骤见 TASK.md;原始复现见 `artifacts/validation-evidence/task-1.2-b-review-20261018-03/`,独立已有场景 3/3 与聚焦包/构建 PASS 证据见 `task-1.2-b-review-20261018-02/`。未将受控插桩写成未插桩二进制命中窗口。
- 2026-10-18：Developer 提交 `f4383f2f59adedadc9252e465c64580570a3bbcf` 后，B 核对完整 Task Base..Head 及纯文档后继，独立聚焦 Go 五包、amd64 发布／testhooks 构建、URL／SHA-1／Size 更新、零写、dry-run 与既有故障恢复最终 PASS；F-10 原双替换序列 PASS，新反例为三个 CLI FAIL 与两个确定性注入子测试 FAIL。证据在 `artifacts/validation-evidence/task-1.2-b-rereview-20261018-01/`；首轮并行启动验收驱动造成的 busy 保留，顺序重跑通过，不计产品缺陷。复核 Developer T4：Python 48、race 160 个测试／8 个包、隔离 CLI 9/9 和双架构构建通过，输入相同可复用，但未覆盖新失败，B 本轮未重跑完整 T4。F-12 新增故障注入及三个信号窗口的覆盖事实保留；修正候选未获 PASS，不创建新 Task 或 Closure Packet。
- Task 1.2 原候选 T4 证据保留在本机 `artifacts/validation-evidence/task-1.2-final/`（race 原始 JSON 为 147 个测试、8 个包通过，0 fail／skip；原报告的 155 混计包事件；隔离 CLI 9/9、双架构交叉构建与 testhooks 构建通过）。这些旧用例未覆盖本次失败窗口。首轮夹具错误及 B 的构建身份／屏障准备失败均保留原始记录；没有 Task 1.2 PASS 或 Closure Packet。
- 前一候选 T4 证据位于本机 `artifacts/validation-evidence/task-1.1-fix-20261018-05/`（race 134 pass）；Task 1.1 F-9 修正的权威 T4 证据位于 `artifacts/validation-evidence/task-1.1-b-review-fix-20261018-02/`（race 142 pass、隔离 CLI 9/9、故障恢复聚焦 3/3）。`task-1.1-fix-20261018-04/` 保留一项首轮断言失败及修正前后证据，不覆盖、不隐藏。
- SIGKILL 恢复场景观察到一个未引用的 `.strmgen-*.tmp`；已验证已有 STRM／归属／完整轮次恢复断言，恢复文档要求先核实而非盲目清理。真实外部 OpenList／无签名 `/d/`、固定插件解析、Jellyfin／代理、ARM64 实机及性能仍为 NOT RUN；无生产服务、媒体或配套仓库测试。
- 历史 B 独立复审证据位于 `artifacts/validation-evidence/task-1.1-b-review-20261018-02/`：首次完整隔离复跑在归属中断恢复处失败，随后单独 `fault_recovery` 重跑通过；失败由 SIGSTOP 落在“发布后、归属登记前”窗口导致，程序按合同正确保持未归属冲突，但驱动断言不稳定。该问题已在同一 Task 内改为确定性注入，并由后续 T4 证据闭环。
- 2026-10-18：Developer 在同一 Task 内修正 F-9：新增仅 `strmgen_testhooks` 构建可见的 `afterOwnershipRecord` 确定性屏障，进程在 `RecordOwnership` 成功后写标记并 `SIGSTOP`，驱动不再轮询；发布二进制为无环境开关的空实现（`strings` 核对无 `STRMGEN_TEST*`）。`fault_recovery` 聚焦重跑 5/5、隔离 CLI 9/9、race 142 pass 全 PASS，实现检查点 `15d749b3f151e94a6fb5e9850e6e21d7101aa603`；B 独立复审后 Task 1.1 PASS，Task 1.2 已规划，Slice 仍 ACTIVE，无 Slice PASS 或 Closure Packet。

## 开发准备

2026-10-17：按用户要求保留原始V4.3方案，复用配套项目Slice／Task流程，建立需求、用户旅程、架构、兼容性、测试矩阵、四个Slice路线、规划合同及角色指南。创建本地Go环境工具、合成OpenList夹具、配置示例和CI定义。

已验证：Go 1.27.2 本地安装及版本检查；标准库样例运行；Linux amd64／arm64 编译 smoke；29项Python工具回归；文档身份／链接／规划治理、Python／Bash语法与全部交付文件空白检查。原始V4.3内容SHA-256保持不变。

Compose夹具已在专用 `strm-generator-dev` 项目启动并通过health检查，地址 `http://127.0.0.1:15244`；容器端list分页、应用层500、HTTP503及合成不存在语义已验证。夹具仍运行，停止命令见环境说明；未访问或修改同机生产Jellyfin／OpenList。

用户补充Git身份后，已仅在本仓库配置 `kentinya <szd123956@outlook.com>`，开发准备文件保存为main的首个本地基线提交。真实完整提交号通过 `git log --reverse --format='%H %s'` 查看；未配置远程，未推送。基线提交后治理使用常规模式。CI定义尚未在GitHub运行，环境准备不计为产品Slice通过。

## 初始指针（历史，2026-10-17）

2026-10-17：用户明确要求激活第一个Slice；A提交Slice 1「安全的单轮生成闭环」ACTIVE合同及同步Roadmap，固定开发准备基线 `f90f9560e55b07fd2d6a20e8bac43741c39176c4`。本次只激活合同，没有产品实现或审查结论。

- Slice 1（当时合同见 `951aaeeeca966e8ef79e8ac9500827ce27d0f035:SLICE.md`）：ACTIVE；Base为 `f90f9560e55b07fd2d6a20e8bac43741c39176c4`，Implementation Head为NOT SET。
- Task 1.1（初始计划留在可达历史）：PLANNED；首次生成、归属登记与安全重复运行，Task Base与Contract Checkpoint均为 `951aaeeeca966e8ef79e8ac9500827ce27d0f035`。尚无Developer实现，产品审查PENDING。
- 产品Go测试／构建、真实OpenList、插件解析、Jellyfin／代理、压力、产品发布：NOT RUN。
- B独立审查与A产品最终审查：PENDING。
- 下一步：Developer按Task 1.1实施并提交真实验证证据；B审查后重新核对全部RO。已拥有文件内容更新及恢复、真实OpenList／固定插件解析等缺口仍未完成，不预设Slice通过。

初始工具证据仅本机保存在 `artifacts/validation-evidence/preparation-20261017/`；Git身份配置、基线与常规治理验证保存在 `artifacts/validation-evidence/git-baseline-20261017/`；合同激活及提交后的治理／质量检查保存在 `artifacts/validation-evidence/slice-1-activation-20261017/`。旧证据中的无HEAD／身份缺失状态保留为当时事实，SDK与运行证据按可执行输入身份复用。后续Task／审查历史保存在可达Git检查点。

2026-10-17：B完成必读与实际仓库核查，规划前常规治理PASS，提交Task 1.1详细实施／验收／T4测试计划；规划验证证据见 `artifacts/validation-evidence/task-1.1-planning-20261017/`。本次没有产品实现或独立产品审查PASS，Slice／Roadmap／稳定需求保持不变。

2026-10-17：Developer按Task 1.1实现首个产品闭环并提交检查点，状态置为READY FOR B REVIEW（待提交）。交付：`cmd/strmgen` 的 `sync`／`sync --dry-run`；`internal/config` 严格YAML校验；`internal/openlist` 有界list/get与分页／应用层错误；`internal/pathmap`／`internal/strm` 纯路径、稳定URL、版本1协议与比较；`internal/storage` 用 `modernc.org/sqlite v1.60.1` 记录最小归属与完整轮次，dry-run对WAL库读私有快照不产生sidecar；`internal/reconciler` 有界遍历、同目标冲突、原子发布（同目录临时文件 + 不覆盖link）与跨进程flock互斥。依赖 `gopkg.in/yaml.v3 v3.0.1`；race门槛使用仓库固定Zig `cc`。

本机已执行（证据见 `artifacts/validation-evidence/task-1.1-final/`）：治理、mod download／verify、quality（gofmt／vet／Go test／build＋Python回归）、race、Linux amd64／arm64 CGO=0构建、以及隔离CLI全场景驱动（首次、重复与比较、归属与路径、API与限额、dry-run边界、多进程、故障恢复）全部PASS。产品未完成：已拥有文件内容替换、Mirror删除／移动、serve／定时、发布镜像；真实OpenList、固定插件解析、Jellyfin／代理、ARM64实机与性能仍NOT RUN。B独立审查PENDING，不预设Slice通过。


## B 审查记录（历史，2026-10-18）

B 已读取仓库约束／必读文档并在审查前取得常规治理 PASS；独立审查 Task Base 951aaeeeca966e8ef79e8ac9500827ce27d0f035 至 Developer Head 4752ec2057f998d762829cdb352c8b75198ee900，核对纯报告后继和本机原始 quality／race／构建证据。现有实际 CLI 七类场景独立重跑 7/7 PASS，但新增隔离反例复现了根／祖先符号链接逃逸、锁命名空间分裂及预览锁写输出、多 source 归属覆盖、DB URI 路径误解、错误列表形状提交成功轮、可选 Size 阻断、诊断泄露、FIFO 阻塞及相对路径语义不符。

Task 1.1 为 **FIX REQUIRED**，详细 F-1..F-8 修正与复验步骤在 TASK.md；修正保留同一 Task／Base／T4，不新建 Task。Slice 1 仍 ACTIVE，Implementation Head 仍指向被审候选；RO-1..RO-5 均未全部满足，无 Closure Packet 或 A 最终 PASS。本次 B 未实施产品代码，未改 Slice／Roadmap／稳定需求，未触及配套仓库及生产服务，未推送。早先 PLANNED／NOT SET／PENDING 为历史记录，以本节及活动 Task 为当前状态。

审查证据仅本机：artifacts/validation-evidence/task-1.1-b-review-20261018/（index.json、原始日志、隔离输出／SQL、反例脚本）。B 未重跑完整 quality／race／双架构构建，发现阻断后先退回；Developer 修正后须用新证据目录重跑完整 T4。真实 OpenList／固定插件解析、Jellyfin／代理、ARM64实机、性能仍 NOT RUN。下一步：Developer 在 Task 1.1 增量修正并重新送 B 审查。
