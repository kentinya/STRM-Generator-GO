# Task 4.5 第三次独立 B 复审（2026-10-21）

## 固定候选与决定

```text
Task: 4.5
Task Base: 5182443d0c34215deb7bb1e74a413564aa8e6422
Implementation Head: 9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa
Reviewed Documentation Successor: 1b7aea6aba1a3cbacd8360acab8a98e23ca2abd2
Contract Checkpoint: 48ff46a2cfd742190de0e8a673a92ec61c086950
Decision: FIX REQUIRED
Slice Required Outcomes all satisfied: NO
Next: SAME TASK 4.5; RESOLVE CREDENTIAL HANDOFF; B RETAINS REAL REPRODUCTION
```

入口 main 工作树干净；先读 AGENTS、流程、V4.4、范围、需求、用户旅程、架构、路线、
Slice、Task 及三角色指南，治理通过。审查原 Base..Implementation Head 和全部后继，
不只审 dev3 文档。产品、依赖、部署及验收工具未改，本轮不改它们，不启动 4.6。

本机证据：`artifacts/validation-evidence/task-4.5-B3-20261021T0742Z/`。
保留固定候选、82 项输入匹配、既有门禁日志／二进制摘要、dev3 原始证据清单与
15 份生成器 DB 快照核对、当前输出校验和本轮检查日志。它不是远端 CI 产物。

## dev3 交付核对

**F-45-01 的两个关键行为缺口已有实证，不再要求 Developer 重跑：**

- 实际移动：原始 Get 记录旧 054 未读到、新 `Moved-Synthetic 054.mkv` 可读；
  `round1/2/3-db.sqlite3` 的旧归属 `created_round=1`、缺失次数 1→2→3，
  新归属 `created_round=10`；第 4 轮 `deleted=1`，旧行消失、新行保留。
  原始轮次依次 10／11／12／13，产品摘要与快照一致。这不再是 delete/restore 冒充移动。
- 同行清零：`057-baseline/round1/round2/round3/round4/reappear-db.sqlite3`
  显示 epoch 21→22→23→24→24→25；同一 `created_round=1` 的 057 行
  missing_count 0→1→2→3→3→0。失败轮退出 1、不推进 epoch／计数，保留首次失败。
  这不是删除后的新行 `created=1` 冒充清零。
- 更新／跳过与恢复：保留发布 CLI 的 `updated=1`／稳定 `skipped=10000` 摘要；
  055 精确 Get Size=24430 与原件 A=24404 的变化相符。9 份备份／恢复媒体均核对
  SHA-256，最终四样本为 A；保留远端列表恰 10000 路径，与固定清单集合相同。
  这是对已交原始记录的核对，不声称本轮重新访问 S3。
- populated dry-run 前后输出身份与 DB 摘要相等；最终稳定轮前后输出身份相等。
  当前本地生成器 DB 为 epoch26、10000 归属、非零缺失0。当前全树另行只读校验，
  不把当前状态倒推成历史中间状态。

交接中“第4轮后9999行”是事实笔误：原始快照实际 **10000行**（移动期间暂有10001行）；
缺失阈值是已提交的三轮证明，不是“3 < 4”的新阈值规则。上述复审以真实快照和需求为准。
这些笔误无需产品修复或重复万文件测试，后续事实文档直接澄清即可。

## 仍阻断的事项

### F-45-03／P1：新轮内存使用叙述不能代替旧凭据文件处置与来源交接

dev3 声称新轮未创建凭据文件，这是对新轮做法的修正，不抹去 dev2 已记载的
“0600临时文件、source后shred”。上次 B 要求的**非秘密**说明仍未交付：旧文件使用的
存储介质／位置范围、清除方式与范围、是否有已知副本或残留，以及需要用户／A
处置的未知项。没有这些信息，不能把 F-45-03 写为已关闭；也不据此推断已经泄露。

此外，dev3 INDEX／交接称 `/api/auth/login` 使用“container env password”，但未说明
是哪一已核对容器／键名及对应授权。当前4.5授权明确为 Developer 从指定 RustFS/S3
容器选择必要键，不能据此自动覆盖别的容器。只需说明来源／身份／适用授权，
**不要提供秘密值，不要从历史文件恢复秘密，也不要为解释而再次取密**。
`measure_run.py` 的 `token_source` 字符串只描述子进程注入方式，不能证明上游登录
helper 的来源和全过程；该 helper 不在交接证据内。可补已有准确的非秘密执行文本，
不要求建设新框架或重跑真实场景来粉饰过去。

本轮不检查历史秘密内容，不自行清除历史、恢复账户、轮换凭据或修改服务生命周期；
是否需要例外接受／轮换由用户／A 决定。仓库现有脱敏后继不等于已解决上述历史问题。

### AC-6：B 独立真实关键复现仍未完成，由 B 保留

当前进程未注入 OpenList／S3 专用凭据。已登记容器 Env 和样本变更授权明确给
**Developer**，上一轮 B 也明确未扩大为 reviewer 权限。本轮用户要求 B 复核，
未明确扩大这个取密渠道；B 不借 Developer 身份或临时 helper 越权取密。
因此更新／真实移动／三完整轮清理／同行清零／dry-run 的 B 真实复现记
**BLOCKED（未执行）**，不是产品 FAIL，也不能用这次快照审计代替。

需用户／A明确协调 B 在同一隔离项目／同一最少自有样本范围的取密、必要变更与恢复
授权，或安全提供 B 可用的专用凭据及操作范围；不得把秘密贴入文档或聊天。
授权成立后仍由 B 使用已存在发布 CLI、OpenList list/get 和 rclone 入口保留基线，
按原任务复现并确认恢复，先估算当前严格小于50%的测试预算；不足则如实保留未完成，
不靠空等凑比、不缩成另一套小数据集来抵扣当前源身份，不把独立门禁交回 Developer。

## 指定全量与精确复用

- 82项产品／依赖／脚本／测试／夹具输入与 B2 的逐文件 SHA-256 全部相同；
  四份 tracked 辅助文件与工作副本相同。既有原始门禁日志、两种集成使用的发布／
  testhooks二进制身份仍匹配。沿用 B1／B2 完成的 mod、非缓存 Go、race、双架构及
  testhooks构建、11项同步／6项serve、15项辅助测试，不机械重跑同输入 Final。
- 固定真实 Get／缓存八场景11项及打包／Compose／容器／arm64 OCI／重启，保留
  [B2](task-4.5-b-review-2.md) 的准确身份链与6/6容器原始结果；核对保留摘要及日志，
  不把旧4.1–4.3运行抵扣本任务，也不把ARM64交叉构建称实机验证。
- 本轮新跑当前 quality、全树校验、治理／相关链接和补丁空白检查；命令、退出码、
  原始日志和计时见证据。全量门槛 F-45-02 继续齐备，**不退回 Developer**。
- Slice Final NOT RUN：RO-4／5尚缺，不提前跑最终收口，也不以角色不同重跑全部输入。

## 同 Task 修正及全部 RO

Developer 只补 F-45-03 的非秘密历史处置／本次来源和已有执行记录，澄清上述事实笔误；
不要求再次移动媒体或重跑全量。纯事实文档仅治理／相关链接／空白；若实际修改工具或
产品，再按根 TASK 的受影响模块验证，保留首败及准确候选。授权取舍交用户／A。
B 接收修正后继续原 Base..Head 审查，保留已齐备门禁，独立完成上述真实关键复现；
AC-1..6与恢复、授权处置全部齐备才 PASS。

| RO | 本轮重评 |
|---|---|
| RO-1／2 | 保留4.4 PASS；固定清单身份相同，无重生成 |
| RO-3 | 万文件／测量和dev3移动、清零、恢复证据支持；凭据交接与B独立真实门禁仍阻断 |
| RO-4／5 | NOT RUN，4.6未激活，不能以插件任务可用性冒充扫描／实际决策 |
| RO-6 | 指定全量已齐备；独立真实复现、Slice Final、Closure与A审查仍未完成 |

**Task 4.5保持FIX REQUIRED，Slice 4保持ACTIVE。** 不新造Task，不形成已满足全部RO的
Closure Packet，不返回SLICE READY FOR A REVIEW，不自行关闭。
历史计时 F-45-04 仍按用户处置不再阻断；本轮实际检查与只读验收一并计时，见
证据 `timing.json`，不将历史超限倒改为合规。
