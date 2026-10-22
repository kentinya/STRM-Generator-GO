# 文档导航

当前需求版本为 V4.4，Slice 1–4 均已 **PASS / CLOSED**，根 [TASK](../TASK.md)
无活动实施任务。当前开发需求已完成，没有必须继续的下一 Slice；新增工作须由用户另行授权。
ARM64 实机、正式发布及合同排除项的 NOT RUN 不改为 PASS，也不自动变为开发待办。

## 使用与维护

| 目的 | 入口 |
|---|---|
| 构建、CLI 与安全操作 | [项目 README](../README.md)、[配置示例](../config.example.yaml) |
| Docker／Compose、权限、备份与共享锁 | [部署说明](deployment.md)、[容器配置](../deploy/config.example.yaml) |
| 行为要求与限制 | [需求](requirements.md)、[范围](scope.md)、[用户旅程](product-experience.md) |
| 模块、持久化与外部协议 | [架构](architecture.md)、[包边界](../internal/README.md)、[兼容性](compatibility.md) |

## 开发治理

| 目的 | 入口 |
|---|---|
| 约束与唯一流程 | [AGENTS](../AGENTS.md)、[开发流程](development-workflow.md) |
| 本地 SDK、测试和固定输入 | [开发环境](development-environment.md)、[测试矩阵](test-matrix.md)、[来源](references.md) |
| 当前状态与已关闭合同 | [SLICE](../SLICE.md)、[TASK](../TASK.md)、[路线图](roadmap.md)、[进度](progress.md) |
| 角色方法与空白模板 | [A](agent-experience/slice-owner-architect-final-reviewer.md)、[B](agent-experience/task-planner-reviewer.md)、[Developer](agent-experience/developer-implementation-role.md)、[Task 模板](../Task/TEMPLATE.md) |

`SLICE` 保留已接受的结果与验收边界，`TASK` 表示是否存在当前实施任务；规划记录、
旧报告中的“当前”或模板示例不能覆盖这两个入口。本地参考方案已退出必读、必需文件、
内容锁定和版本跟踪，不是新克隆或治理检查的前置。

## 审查与历史

| 记录 | 入口与含义 |
|---|---|
| Slice 4 最终接受 | [A 最终审查](slice-4-a-final-review.md)、[Closure Packet](slice-4-closure-packet.md) |
| 三项独立 B 结果 | [4.4 B4](task-4.4-b-review.md#原生可用性补证复审2026-10-20-b4)、[4.5 最终续审](task-4.5-b-review-5.md#2026-10-21-续审用户接受历史-unknowntask-pass)、[4.6 B3](task-4.6-b-review-3.md) |
| 隔离资产与操作输入 | [资产清单](slice-4-assets.md)、`docs/task-4.*-artifacts/`；保留用于追溯，不能未经当前授权重跑真实操作 |
| 历史授权、未知项与撤销 | [授权记录](authorization-history.md)、[凭据处置](task-4.5-credential-disposition.md)、[认证交接](task-4.6-jellyfin-access.md) |

`Task/4.*` 计划、`docs/task-4.*` 报告／实施指南及进度旧账本均为对应时点记录。
PENDING、FIX REQUIRED、BLOCKED、首败与 UNKNOWN 保持历史事实；最终是否通过由上表
最终审查及根状态文件确定。Slice 1–3 的关闭检查点见[路线图](roadmap.md)。

## GitHub 推送与证据边界

2026-10-22 本轮为本地文档治理准备，未执行推送；准备开始时未配置 Git remote。
GitHub 目标仓库与访问范围须在实际推送前明确，不从配套插件仓库推导本项目远程。
仓库模块名仍为 `strm-generator-go`，本次不重命名模块或建立正式发布。

Git 推送包含所选分支的可达历史。已从当前树移出的本地参考方案仍在已有历史中，
本次不重写历史。`artifacts/`、本地配置、凭据、数据库、媒体和原始验证日志不加入提交；
示例假 token 与合成元数据不是生产凭据或真实媒体指纹证明。

历史验收原始证据保存在本机 Git 忽略目录；GitHub 上的报告是可追溯索引，
不能据此声称远端读者已取得原始日志或 CI 已复现真实 S3／OpenList／Jellyfin。
需要交接原始材料时按[证据规则](development-workflow.md#5-证据)准备脱敏副本，单独安排交付。
本轮本地检查记录位于 `artifacts/validation-evidence/github-doc-preparation-20261022T033059Z/`。

新克隆须保留治理所需的完整 Git 历史；CI 已配置 `fetch-depth: 0`。
安装固定工具链后执行 `python3 scripts/quality.py`；GitHub Actions 的 race、双架构
smoke 等额外步骤见 [CI 配置](../.github/workflows/quality.yml)，其远端执行结果以实际运行为准。
