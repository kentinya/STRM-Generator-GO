# Slice 路线图

状态表由治理工具读取；Slice 1、Slice 2 与 Slice 3 均 PASS / CLOSED。Slice 3 的 Task 3.1／3.2 均有独立 B PASS，A 已审查完整范围至 `410fc0e0a8964046de4707d58c0c7505385c6002` 并关闭 Slice。Task 2.1／2.2 均 PASS，用户最终改为源根／目标 Get 简化判定，所有列表关闭强刷；撤回前两次权威列表／500 例外和刷新预算决定，已关闭 Slice Base 不变。Slice 4 当前 ACTIVE，Base 固定为 `d1c95ad079a0e3f952d9e57ab50299c6ae93ad63`；用户授权的合同修订检查点后，等待 B 规划 Task 4.1。

| Slice | 名称 | Status | 依赖 | 原方案映射与结果 |
|---|---|---|---|---|
| 1 | 安全单轮生成 | PASS / CLOSED | 开发基线 | P0＋必要 P1：配置、API、协议、映射、原子创建／更新、增量比较、最小归属、正常恢复、dry-run／互斥闭环 |
| 2 | 安全目录镜像与清理恢复 | PASS / CLOSED | Slice 1 | P2＋清理所需 P1：移动／删除、完整轮与连续缺失、源根／Get重核、归属／边界、旧库兼容、清理故障恢复及预览 |
| 3 | 定时与部署 | PASS / CLOSED | Slice 2 | P3：serve、共同互斥、停止恢复、Linux 双架构／Docker；修正后保留 sync 零间隔兼容，并以 `config --no-interpolate` 采集脱敏 Compose 渲染 |
| 4 | 隔离真实服务端到端兼容验收 | ACTIVE | Slice 3 | P4：目标 OpenList 驱动／稳定无签名 `/d/`、当前源码构建的插件实际解析与决策、隔离 Jellyfin 原生扫描／播放；不验证代理 |
| 5 | ARM64 实机与规模测量 | PLANNED | Slice 4 | P3＋P4：原生 ARM64 产品运行与恢复、10万至100万有界负载画像、RSS／队列／吞吐／错误与恢复实测 |

## 2026-10-19 A 选择 Slice 4

Slice 3 关闭后，A 在独立回合选择 Slice 4「隔离真实服务端到端兼容验收」，Base 固定为
选择前真实 HEAD `d1c95ad079a0e3f952d9e57ab50299c6ae93ad63`。目标是用 `artifacts/` 下
专用隔离服务和合成媒体验证目标 OpenList 驱动、插件、Jellyfin／代理及实际
播放，不接触生产服务、生产配置或媒体。该原始选择范围后由下方的用户授权合同修订收窄。

原计划将外部端到端、ARM64 实机和规模实测放在同一 Slice。A 将独立硬件与负载画像安排到
后续 Slice 5，以分开服务兼容与平台容量结果；当时没有将交叉构建
改称 ARM64 实机，也没有将合成 API／直连播放改称目标驱动通过。Jellyfin 与目标存储驱动
身份须由 B 在 Task 计划中确立；缺少身份或隔离资源则报 BLOCKED／NOT RUN，不降门槛。

## 2026-10-19 A 用户授权合同修订

用户明确授权 Slice 4 不验证代理，只验证 Jellyfin 原生扫描／播放；插件包使用配套
`jellyfin-strm-probe` 仓库当前源码构建，不再把既有固定 SHA 的预构建包作为部署输入。
Task 4.1 规划须记录当前源码 checkout 的完整源仓库、分支、commit SHA 和必要补丁摘要，
并记录构建工具、命令及插件产物 SHA-256。旧参考 SHA 仍用于 V4.3 协议／决策规则对照，
但不代表实际部署包身份。Slice 4 Base 不变，RO 与 T4 门槛保持真实 OpenList、当前源码
插件解析／决策、Jellyfin 原生扫描／实际播放、分层证据及完整回归；代理从验收和阻断条件中
移除。合同检查点后由 B 规划 Task 4.1，当前尚未开始实施或启动外部服务。

## 2026-10-18 A 选择与重排

原路线 Slice 2 的比较矩阵、稳定输出零写和正常中断恢复已作为 Slice 1 必要安全接受；继续按原标题实施会重复已交付能力。本次把尚未激活的路线重新安排为：Slice 2 交付原计划 Slice 3 的完整 Mirror 旅程；定时／部署和端到端／规模分别留作后续完整结果。计划条目的重排不是新 PASS，未启动后续 Slice 或 Task。

Slice 2 的关闭合同保留于其关闭检查点，Base 为 `930782035615512be811a7b8bcc8416050f0c12f`，Implementation Head 为 `d0a0f6db5447f38467c67612ed7173d77e6bea27`。必要删除安全要求已由 Task 2.2 实现并经 B 审查及 A 最终审查；该 Slice 对真实 OpenList 的范围限于源根／Get 判定所需的固定隔离版本／驱动实证，不能替代 Slice 4 的真实部署、无签名 `/d/`、插件或播放验收。

Slice 3 关闭时的合同以其关闭检查点为准，Base 为 `ca9f91e10a0fc6c28679b5827f453e632eb9854a`，A 已审查完整范围至 Implementation Head `410fc0e0a8964046de4707d58c0c7505385c6002` 并以 PASS / CLOSED 收尾。Task 3.2 的原 Slice 内修正保留环境 token 注入与 `read_only`，脱敏 Compose 证据使用 `config --no-interpolate`。当时未验收的真实 OpenList、无签名 `/d/`、插件／Jellyfin 原生扫描与播放由 Slice 4 覆盖；代理不属于修订后的 Slice 4 范围。ARM64 实机与规模测量经 2026-10-19 A 选择排入 Slice 5。正式发布仍未纳入当前 Slice。

Slice 1 的 Base 保留为 `f90f9560e55b07fd2d6a20e8bac43741c39176c4`，已审查 Implementation Head 为 `248a609eb70dbb8591091463ed51c5146d7c378a`。完整关闭合同及 Closure Packet 位于 `930782035615512be811a7b8bcc8416050f0c12f:SLICE.md`；Task 1.1／1.2 独立 B PASS、A PASS / CLOSED 及原始证据均保留。正常写者与最终一致性边界继续遵循需求 SW-1..SW-6，旧外部写者 FAIL 不改称已修复。

规模验收分十万起步、按实际结果扩展到百万，并报告 RSS、目录宽深、分页、队列、吞吐、错误与恢复；512 MB 作为第一档限制。本地宽目录重复枚举成本为已知 P2；所有远端列表关闭强刷，撤下此前刷新证明与请求预算能力要求，新增清理工作仍必须有界。真实外部依赖缺失时明确 NOT RUN／BLOCKED，不用合成 API 或交叉编译替代。
