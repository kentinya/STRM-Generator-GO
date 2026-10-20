# Slice 路线图

当前需求为 [V4.4](<../STRM Generator Go.md>)。Slice 1–3 已 PASS / CLOSED；
Slice 4 在原 Base 内重规划，未验收、未关闭。当前工作与交接见 [Task](../TASK.md)。

| Slice | 名称 | Status | 依赖 | 结果与范围 |
|---|---|---|---|---|
| 1 | 安全单轮生成 | PASS / CLOSED | 开发基线 | 配置／API／V1／稳定 URL、原子写入、归属、正常恢复、dry-run／互斥 |
| 2 | 安全目录镜像与清理恢复 | PASS / CLOSED | Slice 1 | 源根／目标 Get、三完整轮、归属／边界清理、移动／再出现与恢复 |
| 3 | 定时与部署 | PASS / CLOSED | Slice 2 | serve／共同互斥、停止恢复、双架构构建及 amd64 隔离容器 |
| 4 | 固定万文件生成与最小扫描／插件决策验收 | ACTIVE | Slice 3 | 固定复用 10000 个 S3 文件、少量同步样本、原始 STRM 扫描、插件完整决策表 |

Slice 4 Base：`d1c95ad079a0e3f952d9e57ab50299c6ae93ad63`，不随重规划移动。
旧 Task 4.1 最后 FIX REQUIRED，已有 201 项及扫描等证据按输入匹配有限复用，不补签 PASS。
Task 4.2 先治理工具并交独立 B；治理后规划 Task 4.3 验收固定数据集和最小实际链路。
无客户端播放、代理、旧暂缓例外或更高规模压测门槛。ARM64 实机及正式发布另由 A 选择，未自动激活后续 Slice。

历史关闭记录：Slice 1 见 `930782035615512be811a7b8bcc8416050f0c12f:SLICE.md`；
Slice 2 Head 为 `d0a0f6db5447f38467c67612ed7173d77e6bea27`；
Slice 3 A 已审完整 Base `ca9f91e10a0fc6c28679b5827f453e632eb9854a` 至 Head
`410fc0e0a8964046de4707d58c0c7505385c6002`，Task 3.1／3.2 均有独立 B PASS。
旧路线与审查记录保留在 `f0767f0` 及此前 Git 历史，不作为当前执行清单。
