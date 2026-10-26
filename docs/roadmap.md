# Slice 路线图

V4.4 与自定义扩展名增量均已交付，Slice 1–5 PASS / CLOSED。
根 [TASK](../TASK.md) 无活动实施任务，无剩余已授权产品需求；新增功能须由用户另行授权。
业务规则见[需求](requirements.md)，原合同与审查从[固定 Git 检查点](development-workflow.md#历史记录)查看。

| Slice | 名称 | Status | 依赖 | 结果与范围 |
|---|---|---|---|---|
| 1 | 安全单轮生成 | PASS / CLOSED | 开发基线 | 配置／API／V1／稳定 URL、原子写入、归属、正常恢复、dry-run／互斥 |
| 2 | 安全目录镜像与清理恢复 | PASS / CLOSED | Slice 1 | 源根／目标 Get、三完整轮、归属／边界清理、移动／再出现与恢复 |
| 3 | 定时与部署 | PASS / CLOSED | Slice 2 | serve／共同互斥、停止恢复、双架构构建及 amd64 隔离容器 |
| 4 | 复用隔离资产的万文件与插件决策验收 | PASS / CLOSED | Slice 3 | 固定复用 10000 个 S3 文件、少量同步样本、原始 STRM 扫描、插件完整决策表 |
| 5 | 通过配置自定义媒体扩展名 | PASS / CLOSED | Slice 1–4 已交付基线 | Task 5.1 B PASS、A 最终 PASS；默认兼容、显式替换、三入口筛选、排除清理与恢复 |

## Slice 5 已关闭记录

固定 Base 为 `846d0920feba607c33e935da4fac6f0c4cb68a24`，实现 Head 为
`27e2f528cb584fd613e0056eb3ec57825b7e0872`。Task 5.1 独立 B PASS；
A 审查 Base 至 B 交接 `85a31337bb1a36bba9198fd8ff77959fc8d151cd` 的完整范围，
RO-1..5 全部 PASS，无阻塞项；原最终审查位于 `4ee49b872dba0964de1c62ec11d63b1717221b7a:SLICE.md`。

关闭摘要、证据路径与复用边界见 [Closure Packet](../SLICE.md#closure-packet)，
配置与清理规则见[需求 R-13](requirements.md#自定义扩展名)。
验证为 Go／Python 回归和合成 CLI，包含排除清理、重新入选、失败／取消后恢复及定时重启生效。
真实 OpenList、容器、万文件、Jellyfin／插件、播放／代理、ARM64 实机及正式发布均 NOT RUN。

## Slice 4 已关闭记录

固定 Base 为 `d1c95ad079a0e3f952d9e57ab50299c6ae93ad63`，最终实现 Head 为
`24e1ba0ab88173f2c7374f450c9750e5a1e9b504`。三个 Task 均独立 B PASS，
[A 最终审查](https://github.com/kentinya/STRM-Generator-GO/blob/4d16846722f31aa9615494b90df715cf4b5bb6df/docs/slice-4-a-final-review.md)接受 RO-1..6 并关闭 Slice；
完整边界与证据见 [Closure Packet](https://github.com/kentinya/STRM-Generator-GO/blob/4d16846722f31aa9615494b90df715cf4b5bb6df/docs/slice-4-closure-packet.md)。
用户接受历史凭据 UNKNOWN 后通过 4.5，UNKNOWN 不改写为当时已授权或无泄露证明；
4.6 已独立核对原始扫描／读取、插件全部八行决定及恢复，临时认证已撤销。

| 已完成 Task | 结果 | 状态／最终审查 |
|---|---|---|
| 4.4 | 保留复用环境／媒体／三脚本，固定 10000 数据集 | [PASS（B4）](https://github.com/kentinya/STRM-Generator-GO/blob/4d16846722f31aa9615494b90df715cf4b5bb6df/docs/task-4.4-b-review.md#原生可用性补证复审2026-10-20-b4) |
| 4.5 | 万文件生成／测量与最少样本同步／清理／恢复 | [PASS（最终续审）](https://github.com/kentinya/STRM-Generator-GO/blob/4d16846722f31aa9615494b90df715cf4b5bb6df/docs/task-4.5-b-review-5.md#2026-10-21-续审用户接受历史-unknowntask-pass) |
| 4.6 | 最少原始扫描与 D-01..08 实际决策，全部 RO 及 Slice Final | [PASS（B3）](https://github.com/kentinya/STRM-Generator-GO/blob/4d16846722f31aa9615494b90df715cf4b5bb6df/docs/task-4.6-b-review-3.md) |

既有隔离环境、媒体原件和三个辅助脚本按[资产清单](https://github.com/kentinya/STRM-Generator-GO/blob/4d16846722f31aa9615494b90df715cf4b5bb6df/docs/slice-4-assets.md)保留，
不因文档清理重建或删除；历史授权不用于新环境操作。旧 4.1–4.3 计划与修复清单从 Git 查看。
探测启动不等于探测成功；客户端播放、代理和更高规模压力不在合同内。
ARM64 实机及正式发布仍 NOT RUN，不作为必须继续的下一 Slice。

## Slice 1–3 历史入口

- Slice 1 关闭记录：`930782035615512be811a7b8bcc8416050f0c12f:SLICE.md`。
- Slice 2 最终 Head：`d0a0f6db5447f38467c67612ed7173d77e6bea27`。
- Slice 3 A 审查完整 Base `ca9f91e10a0fc6c28679b5827f453e632eb9854a` 至 Head
  `410fc0e0a8964046de4707d58c0c7505385c6002`；Task 3.1／3.2 均独立 B PASS。

详细原件从上述历史提交及开发流程的固定检查点查阅，不作为当前执行清单。
