# Slice 路线图

当前需求为 [V4.4](<../STRM Generator Go.md>)。Slice 1–3 已 PASS / CLOSED；
Slice 4 在原 Base 内重规划，未验收、未关闭。当前工作与交接见 [Task](../TASK.md)。

| Slice | 名称 | Status | 依赖 | 结果与范围 |
|---|---|---|---|---|
| 1 | 安全单轮生成 | PASS / CLOSED | 开发基线 | 配置／API／V1／稳定 URL、原子写入、归属、正常恢复、dry-run／互斥 |
| 2 | 安全目录镜像与清理恢复 | PASS / CLOSED | Slice 1 | 源根／目标 Get、三完整轮、归属／边界清理、移动／再出现与恢复 |
| 3 | 定时与部署 | PASS / CLOSED | Slice 2 | serve／共同互斥、停止恢复、双架构构建及 amd64 隔离容器 |
| 4 | 复用隔离资产的万文件与插件决策验收 | ACTIVE | Slice 3 | 固定复用 10000 个 S3 文件、少量同步样本、原始 STRM 扫描、插件完整决策表 |

Slice 4 Base：`d1c95ad079a0e3f952d9e57ab50299c6ae93ad63`，不随重规划移动。
旧 Task 4.1 最后 FIX REQUIRED，已有 201 项及扫描等证据按输入匹配有限复用，不补签 PASS。
旧 Task 4.2 的实现保留、独立 B 仍 PENDING；旧 4.3 候选撤回，本次完全重规划为：

| 新 Task | 结果 | 前置 |
|---|---|---|
| 4.4 | 保留复用环境／媒体／三脚本，承接旧工具待审范围，固定 10000 数据集 | 新 ACTIVE 合同与正式 Task 计划提交 |
| 4.5 | 万文件生成／测量与最少样本同步／清理／恢复 | 4.4 独立 B PASS |
| 4.6 | 最少原始扫描与 D-01..08 实际决策，全部 RO 及 Slice Final | 4.5 独立 B PASS |

现有隔离环境、媒体原件及生成／启动／停止脚本原地保留，见 [资产清单](slice-4-assets.md)。
当前任务见根 TASK，后续候选为 [4.5](../Task/4.5-generation-sync-plan.md)、
[4.6](../Task/4.6-plugin-decisions-plan.md)。
无客户端播放、代理、旧暂缓例外或更高规模压测门槛。ARM64 实机及正式发布另由 A 选择，未自动激活后续 Slice。

历史关闭记录：Slice 1 见 `930782035615512be811a7b8bcc8416050f0c12f:SLICE.md`；
Slice 2 Head 为 `d0a0f6db5447f38467c67612ed7173d77e6bea27`；
Slice 3 A 已审完整 Base `ca9f91e10a0fc6c28679b5827f453e632eb9854a` 至 Head
`410fc0e0a8964046de4707d58c0c7505385c6002`，Task 3.1／3.2 均有独立 B PASS。
旧路线与审查记录保留在 `f0767f0` 及此前 Git 历史，不作为当前执行清单。
