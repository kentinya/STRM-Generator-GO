# Slice 路线图

状态表由治理工具读取；Slice 1 与 Slice 2 均 PASS / CLOSED，Slice 3 已完成 B Task 审查并等待 A 最终审查。Task 2.1／2.2 均 PASS，Slice 2 已完成 A 最终审查。用户最终改为源根／目标 Get 简化判定，所有列表关闭强刷；撤回前两次权威列表／500 例外和刷新预算决定，已关闭 Slice Base 不变。

| Slice | 名称 | Status | 依赖 | 原方案映射与结果 |
|---|---|---|---|---|
| 1 | 安全单轮生成 | PASS / CLOSED | 开发基线 | P0＋必要 P1：配置、API、协议、映射、原子创建／更新、增量比较、最小归属、正常恢复、dry-run／互斥闭环 |
| 2 | 安全目录镜像与清理恢复 | PASS / CLOSED | Slice 1 | P2＋清理所需 P1：移动／删除、完整轮与连续缺失、源根／Get重核、归属／边界、旧库兼容、清理故障恢复及预览 |
| 3 | 定时与部署 | READY FOR A REVIEW | Slice 2 | P3：serve、共同互斥、停止恢复、Linux 双架构／Docker、状态与挂载运行说明 |
| 4 | 端到端与规模验收 | PLANNED | Slice 3 | P3＋P4：真实 OpenList／无签名地址、固定插件解析、Jellyfin／代理、ARM64 实机与有界负载实测 |

## 2026-10-18 A 选择与重排

原路线 Slice 2 的比较矩阵、稳定输出零写和正常中断恢复已作为 Slice 1 必要安全接受；继续按原标题实施会重复已交付能力。本次把尚未激活的路线重新安排为：Slice 2 交付原计划 Slice 3 的完整 Mirror 旅程；定时／部署和端到端／规模分别留作后续完整结果。计划条目的重排不是新 PASS，未启动后续 Slice 或 Task。

Slice 2 的关闭合同保留于其关闭检查点，Base 为 `930782035615512be811a7b8bcc8416050f0c12f`，Implementation Head 为 `d0a0f6db5447f38467c67612ed7173d77e6bea27`。必要删除安全要求已由 Task 2.2 实现并经 B 审查及 A 最终审查；该 Slice 对真实 OpenList 的范围限于源根／Get 判定所需的固定隔离版本／驱动实证，不能替代 Slice 4 的真实部署、无签名 `/d/`、插件或播放验收。

Slice 3 当前合同以 [SLICE.md](../SLICE.md) 为准，Base 固定为本次选择前真实 HEAD `ca9f91e10a0fc6c28679b5827f453e632eb9854a`，B 已审查到 Implementation Head `2131b54bcec1cd43769d239a30456b4a8607f4aa` 并提交 Closure Packet，等待 A 最终审查。`serve` 首轮立即执行、固定间隔、有界 tick、手动／定时共享锁、取消不提交、重启恢复和 Docker 挂载／双架构构建属于本 Slice；真实 OpenList、无签名 `/d/`、插件／Jellyfin／代理、ARM64 实机、规模与正式发布仍留在 Slice 4。

Slice 1 的 Base 保留为 `f90f9560e55b07fd2d6a20e8bac43741c39176c4`，已审查 Implementation Head 为 `248a609eb70dbb8591091463ed51c5146d7c378a`。完整关闭合同及 Closure Packet 位于 `930782035615512be811a7b8bcc8416050f0c12f:SLICE.md`；Task 1.1／1.2 独立 B PASS、A PASS / CLOSED 及原始证据均保留。正常写者与最终一致性边界继续遵循需求 SW-1..SW-6，旧外部写者 FAIL 不改称已修复。

规模验收分十万起步、按实际结果扩展到百万，并报告 RSS、目录宽深、分页、队列、吞吐、错误与恢复；512 MB 作为第一档限制。本地宽目录重复枚举成本为已知 P2；所有远端列表关闭强刷，撤下此前刷新证明与请求预算能力要求，新增清理工作仍必须有界。真实外部依赖缺失时明确 NOT RUN／BLOCKED，不用合成 API 或交叉编译替代。
