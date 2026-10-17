# 开发流程

这是 Slice／Task 生命周期、Git、测试与审查的唯一流程定义。复用配套项目的职责和门槛，来源固定在 [references.md](references.md)；长期业务规则见 [AGENTS.md](../AGENTS.md)。原项目的实现、历史授权、SDK 和审查结论均不迁入本项目。

## 1. 对象与职责

Slice 是一个可交付完整能力合同；Task 是合同内可整体审查的实施单元。P0～P4 仅表示原方案优先级，不新增阶段状态机，修正留在原 Task。

| 职责 | 责任 |
|---|---|
| A：Slice 所有者／架构／最终审查 | 确定用户结果、Required Outcomes、边界、Base 与最终验收；协调关闭 |
| B：Task 规划／审查 | 检查真实仓库，规划合理实施单元，独立审查 Task，重评全部 RO |
| Developer：实施 | 按活动 Task 实现、验证、提交可核验证据，不自签独立审查 PASS |

职责不等于必须启动多个模型；多代理仅在用户或适用指令明确要求时使用。没有独立审查就保持 PENDING。

## 2. 状态与实施准入

```text
Task: PLANNED -> IN PROGRESS -> READY FOR B REVIEW -> PASS
                              -> FIX REQUIRED -> IN PROGRESS
Slice: PLANNED -> ACTIVE -> READY FOR A REVIEW -> PASS / CLOSED
                        -> FIX REQUIRED -> 原 Slice 内修正
```

没有活动任务时，根 TASK.md 第一行必须为 `# NO ACTIVE IMPLEMENTATION TASK`。Task 没有 CLOSED 状态；A 才能关闭 Slice。开发准备基线不计为已通过产品 Slice。

合法顺序为：真实基线提交 → A 提交 ACTIVE Slice 合同及同步 Roadmap → B 提交 Task 计划 → Developer 实现检查点。规划阶段 Base、Implementation Head 为 NOT SET；激活后 Base 固定为实施前真实可达完整 SHA，不随文档提交移动。Implementation Head 只指向实际实现检查点，不填自引用 SHA。

实施前执行 `python3 scripts/check_governance.py`，检查已提交合同与当前工作区一致、Roadmap、Task 身份和 SHA 祖先关系。没有 HEAD 时 `--bootstrap` 只接受 PLANNED／NOT SET／无活动 Task，输出不是实施准入。已初始化但未激活也不能开始产品实现。

本次用户仅授权环境与文档。不得因 SDK 可用、Go module 存在或本文件已写完自动激活 Slice。

## 3. 合同与 Task

Slice 包含 ID、Owner、Status、Base、Implementation Head、用户目标、Required Outcomes／Surfaces、约束、不变量、排除范围、最终验收、Closure Packet 和 A Final Review。Task 使用 [模板](../Task/TEMPLATE.md)，声明 Parent Slice、Task Base、Contract Checkpoint、Goal、Scope、Difficulty、Test Level、验收、测试、非目标、完成报告与 B 决定。

一个 Slice 通常规划 3～7 个行为闭环；超过 8 个应检查是否仅按文件或类拆分。首先攻克紧密关联的高风险接入问题。B 依据真实代码规划，不把预想接口当成已存在实现。

不改变结果、边界和验收含义的事实修正可保留合同；改变 RO、安全不变量或验收含义属于实质变更，A 先提交修订合同，B 决定当前 Task 调整或重规划，之后才能实施。PARTIAL / RESCOPE 保留已有效的实现与历史结论，不能粉饰成 PASS。用户明确冻结时记录原因并撤下活动任务，不自动丢弃未接受改动。

Developer 在活动范围内自主完成必要修复、测试和普通本地检查点；实现完成报告记录真实 Head、行为、文件、原始证据、剩余 RO 与限制，将 Task 交为 READY FOR B REVIEW。B 审查完整 Task Base..Head，返回 PASS 或 FIX REQUIRED；修正增加提交，不改写已审查历史。

## 4. 测试等级与执行

| 等级 | 风险 | 门槛 |
|---|---|---|
| T0 | 不改变需求／架构／范围的事实文档 | 相关链接、治理、补丁空白检查 |
| T1 | 仅增加已有行为测试 | 新增与受影响测试 |
| T2 | 单一低风险模块 | 聚焦模块测试和静态检查 |
| T3 | API／CLI／配置等跨模块接入 | 聚焦、相关集成和受影响回归 |
| T4 | 持久化、原子写入、删除、并发、迁移、播放正确性 | 全部已有回归及适用真实运行、故障恢复、打包验证 |

需求／架构改变不伪装成 T0。Go 产品包创建后，Task 指定准确的 `bash scripts/go-local test ...`、`vet`、`gofmt`、构建命令；race 检测需要 C 编译器并显式开启 CGO。尚无产品包时环境 smoke 不能替代这些门槛。

测试计划分开发／修正反馈、最终检查点、B 独立验证、证据与重跑四部分。先验证新增、已知失败和直接影响；最终候选执行指定等级门槛。完整套件已覆盖聚焦测试时说明覆盖关系，不机械重复；真实 OpenList、插件解析、Jellyfin／代理、重启、删除恢复或性能的单独要求仍执行。最终后修改可执行输入，按影响重跑；T4 候选保留完整门槛。

B 独立复现指定关键行为，检查可复用的原始证据，不因角色不同重跑所有相同输入，也不凭报告标签通过。有明确阻塞可先拒绝；未执行门槛如实记录。既有测试计划只有 B 可显式调整并说明原因，不能删测试、弱化断言、隐藏 skip／首次失败来获得通过。

## 5. 证据

每次候选保留证据索引：完整源码 SHA；未提交输入准确内容摘要；命令、工作目录、运行版本、依赖／配置／夹具身份；退出码、计数、耗时；原始日志、产物摘要、首次失败与重跑；验收映射、复用与未运行项。

证据放 `artifacts/validation-evidence/<run-id>/` 或 CI artifact，保留至 A 最终决定。本机证据不是远端可读证据，临时目录不作为唯一交接来源。纯文档后继提交可在核实全部可执行输入相同时复用编译／运行证据，但消费文档的治理检查应重跑。Slice Final 仍验证当前合同。

结果使用 PASS、FAIL、NOT RUN、BLOCKED、SKIPPED，并说明缺失原因。Python 夹具不代表真实 OpenList 兼容，交叉编译不代表 ARM64 实机运行，环境验证不代表 STRM 生成／播放通过。真实联调只操作明确隔离实例和临时媒体，不自动使用生产配置。

## 6. Git

提交前检查 status、完整 diff、未跟踪文件、空白、秘密及范围；使用显式文件清单暂存，不一概 `git add .`。普通检查点保留可达历史，不自动 push、不把配套仓库设为本项目 origin、不重写历史或破坏性 clean／reset。

本次可初始化本地 main。没有真实 Git 用户身份时保留未提交状态并报告，不填虚假用户身份；无真实提交不得伪造 Base。Git 不可写时报告实际限制，不另设 Git 目录绕过。

## 7. Slice 收尾

每个 Task PASS 后 B 逐项重评全部 RO；满足后不为非阻塞润色新造 Task。运行 Slice Final，形成 Closure Packet（Base／Head、RO、Task 索引、最终证据、不变量、P2、延期、文档协调项），交 READY FOR A REVIEW。

A 审查完整 Base..Implementation Head 与用户结果，记录 Reviewed Range、Decision、P0/P1 Blockers，可判 PASS、FIX REQUIRED、PARTIAL / RESCOPE。FIX REQUIRED 保持原 Base 回到当前合同修正。

PASS 后在一个关闭检查点协调 SLICE、TASK、README、架构 CURRENT、Roadmap、Progress，记录已审查 Implementation Head、Closure Reconciliation 和 PASS / CLOSED；TASK 撤为无活动，写 `Next Action: A SELECTS THE NEXT LARGE SLICE` 后停止。下一 Slice 的选择／激活是独立动作。关闭后发现原范围 P0／P1 保留历史重新激活同一 Slice。P2 不无限延长交付。

关闭时阅读三份角色经验，每份最多新增 1～3 条真实可复用方法，按“问题 → 做法 → 适用条件”；无新经验不凑数，不复制原项目实施经历或 Task 流水账。

## 8. 文档分工

AGENTS 保存不变量；本文件保存流程；原方案保留输入身份；需求／用户旅程定义稳定行为；架构分 CURRENT 与 TARGET；SLICE／TASK 限定当前授权；Roadmap 保存次序与依赖；Progress 保存简短事实账本；角色文件只保存方法。详细任务与审查历史留在可达 Git 检查点。
