# Slice 路线图

当前[需求](requirements.md)版本为 V4.4。Slice 1–3 已 PASS / CLOSED；
Slice 4 已完成三个 Task 的独立 B 验收、Slice Final 及 [A 最终审查](slice-4-a-final-review.md)，PASS / CLOSED。当前 V4.4 开发需求已完成，根 [Task](../TASK.md) 无活动实施任务，没有必须继续的下一 Slice；新增工作须由用户另行授权。

| Slice | 名称 | Status | 依赖 | 结果与范围 |
|---|---|---|---|---|
| 1 | 安全单轮生成 | PASS / CLOSED | 开发基线 | 配置／API／V1／稳定 URL、原子写入、归属、正常恢复、dry-run／互斥 |
| 2 | 安全目录镜像与清理恢复 | PASS / CLOSED | Slice 1 | 源根／目标 Get、三完整轮、归属／边界清理、移动／再出现与恢复 |
| 3 | 定时与部署 | PASS / CLOSED | Slice 2 | serve／共同互斥、停止恢复、双架构构建及 amd64 隔离容器 |
| 4 | 复用隔离资产的万文件与插件决策验收 | PASS / CLOSED | Slice 3 | 固定复用 10000 个 S3 文件、少量同步样本、原始 STRM 扫描、插件完整决策表 |

Slice 4 Base：`d1c95ad079a0e3f952d9e57ab50299c6ae93ad63`，不随重规划移动。
已按 4.4／4.5／4.6 完成：4.4 本地与远端固定均已实施（适配建集工具与只读 probe、
10000 本地／S3 对象、最终清单与 dataset_id），B4 已接受固定数据集、工具修正及原生任务可用性，4.4 PASS；4.5已完成独立B真实同步／恢复复现，用户明确接受历史凭据UNKNOWN后B决定PASS；4.6经同Task修正及[第三次B审查](task-4.6-b-review-3.md)已PASS：退出禁止DB取证依赖，
B独立完成原始扫描／读取与全部8行决定，确认逐场景及最终恢复、临时认证撤销；
指定全量按准确输入／原始证据复用并补齐当前检查。全部RO的B验收面满足，
[Closure Packet](slice-4-closure-packet.md)已收口；A已审查原Slice Base..
最终Head `24e1ba0ab88173f2c7374f450c9750e5a1e9b504` 及纯文档后继，RO-1..6 PASS、无P0／P1阻塞，并关闭Slice。
旧 4.1–4.3 的任务、审查和修复清单退出当前安排，后续只复用需要的环境／媒体／脚本／固定产物和适用工具。

| 已完成 Task | 结果 | 状态／最终审查 |
|---|---|---|
| 4.4 | 保留复用环境／媒体／三脚本，固定 10000 数据集 | [PASS（B4）](task-4.4-b-review.md#原生可用性补证复审2026-10-20-b4) |
| 4.5 | 万文件生成／测量与最少样本同步／清理／恢复 | [PASS（最终续审）](task-4.5-b-review-5.md#2026-10-21-续审用户接受历史-unknowntask-pass) |
| 4.6 | 最少原始扫描与 D-01..08 实际决策，全部 RO 及 Slice Final | [PASS（B3）](task-4.6-b-review-3.md) |

现有隔离环境、媒体原件及生成／启动／停止脚本原地保留，见 [资产清单](slice-4-assets.md)。
当前无活动任务，见根 [TASK](../TASK.md)；[4.6正式计划](../Task/4.6-plugin-decisions-plan.md)及[4.5规划原件](../Task/4.5-generation-sync-plan.md)保留为已完成任务的历史合同，不作为新任务授权。
无客户端播放、代理、旧暂缓例外或更高规模压测门槛。ARM64 实机及正式发布仍 NOT RUN，属于当前合同之外，不作为必须补做的下一 Slice。

历史关闭记录：Slice 1 见 `930782035615512be811a7b8bcc8416050f0c12f:SLICE.md`；
Slice 2 Head 为 `d0a0f6db5447f38467c67612ed7173d77e6bea27`；
Slice 3 A 已审完整 Base `ca9f91e10a0fc6c28679b5827f453e632eb9854a` 至 Head
`410fc0e0a8964046de4707d58c0c7505385c6002`，Task 3.1／3.2 均有独立 B PASS。
旧路线与审查记录保留在 `f0767f0` 及此前 Git 历史，不作为当前执行清单。
