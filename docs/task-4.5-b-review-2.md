# Task 4.5 B 复审（2026-10-21）

## 固定候选与决定

```text
Task: 4.5
Task Base: 5182443d0c34215deb7bb1e74a413564aa8e6422
Implementation Head: 9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa
Reviewed Documentation Successor: 23170475617cf987f639dd53f34ca41a239e2176
Contract Checkpoint: 48ff46a2cfd742190de0e8a673a92ec61c086950
Decision: FIX REQUIRED
Slice Required Outcomes all satisfied: NO
Next: SAME TASK 4.5; DEVELOPER FIXES AFFECTED EVIDENCE; B RETAINS FINAL GATES
```

入口 main 干净；先读 AGENTS、流程、方案、范围、需求、旅程、架构、路线、Slice、Task
及三角色指南，治理 PASS。审查完整 Base..Implementation Head 及全部文档后继，
不只审最后一次交接；本轮无产品代码、依赖、固定数据集或既有服务修改，不启动 4.6。
此前审查及其时点结论保留于 [第一次 B 审查](task-4.5-b-review.md)。

本机证据：`artifacts/validation-evidence/task-4.5-B2-20261021T0715CST/`；
源码差异、输入／二进制／原始日志摘要、只读生成器 DB 事实、准确命令／退出码／计时均保留。
证据 Git 忽略，保留至 A 决定；它不是远端可读 CI 产物。

## 必须修正的发现

### F-45-01／P1：真实闭环不能由 delete/restore 充当移动和缺失清零

新交接确有九次测量 JSON 与 stdout/stderr，支持真实更新、缺失、清理、再创建和稳定
重复的运行摘要，不再沿用“Developer 尚无授权、未做任何真实闭环”的旧时点判断。
但 AC-3／4／5 **尚不完整**：

- `INDEX.md` 的 MOVE-ASIDE 实际是 `rclone deletefile`，对象数 10000→9999，
  没有新的远端路径或该新路径的 STRM／归属。它证明删除场景，不证明移动／重命名
  的“新路径生成＋旧路径退出”；不存在的移动入口／步骤不得补称已执行。
- 再出现发生在旧 STRM／归属已删后，摘要 `created=1`。本轮只读 DB 也确认
  `Synthetic 052.strm` 的 `created_round` 从基线 1 变为当前 8。这是重新创建，
  **不是保留同一 ownership 行、已有正数 missing_count 在再出现后清零的实证**。
- 交接证据根只有基线 DB、运行摘要和少量初始／dry-run 前摘要；缺少中间轮次的
  SQLite 观察／文件状态、删除前源根／精确目标 Get 原始结果，以及 S3 变更／恢复
  命令的退出码／非秘密结果。INDEX 称部分记录在 shell history，不能替代可交接
  原始证据，亦不能从 history 恢复秘密。`refresh:false` 和产品路径的既有 Go 证明
  可以复用，但不能仅由源码推断本次具体样本的完整过程。
- 仅有 dry-run **前** DB／输出摘要；随后的稳定同步把当前 epoch 推至 9。
  当前状态不能倒推出当时预览后 epoch 仍为 8，也不能替代最终稳定轮前后字节／
  mtime 的保留快照。S3 10000／恢复字节的叙述同样缺少该根内原始命令结果。

这不是断言产品行为错误，而是当前指定真实行为／原始证据缺项。若确有当时保留的
非秘密原始记录，可直接补交并匹配，**不得事后编造历史快照**。缺记录时只补必要
场景；沿用固定对象、原输出／DB／source identity、发布 CLI 与已有 S3 入口。
不重建媒体、不换小数据集抵扣整树结果、不删库、不改生产或借测试 STRM 副本充当产品。

### F-45-03／P1：凭据临时文件不符合授权中的仅内存使用

[Developer 续修 2](task-4.5-developer-followup-2.md) 第 25–26 行与原始 INDEX 第 29–31 行
均明确写“选择两键至 0600 临时文件、source 后 shred”。当前授权仅允许内存／子进程
环境；0600 和事后 shred **不等于从未写凭据文件**。文件介质、路径及原始执行细节
未提供，B 不推断已经泄露，也不宣称已证明文件无残留或授权合规。

Developer 须保留该真实历史，不把报告改成“始终内存”。说明非秘密的处理方式／存储
介质／清除范围与是否存在副本；后续仅内存选择键并注入子进程，不创建凭据文件、
不输出完整 Env、不从历史文件找秘密。若需要例外接受、轮换或超出现有授权的处置，
交用户／A 协调，B 不追认授权、不自行恢复账户或变更服务生命周期。

### F-45-04：历史测试时间超限，不能由 B 豁免

九份新测量原始墙钟合计 **7489.797s**。即使采用较长的报告回合 12390s，仅这些运行
就约 **60.45%**；INDEX 同时给出更早的结束时间，精确结束口径也未统一。
无论选哪一口径，均超当时用户 30% 及计划 50% 上限。历史原件不倒改，不以重跑／
空等“修复比例”，B 无权接受“门禁固有耗时”作为用户约束的豁免。
用户本轮要求小于 50% 只约束本轮，不追溯把上一轮超限改称合规；历史偏差提交
用户／A 处置，已取得的有效运行事实可按身份继续使用，不要求因此机械重做九轮。

## B 指定门禁与复用

| 门槛 | 本轮结论及证据边界 |
|---|---|
| 当前治理／quality | 新跑 PASS；quality 含当前文档治理、111 项 Python、AST、Bash、gofmt、vet、Go test/build；消费文档的治理不能仅复用旧结果 |
| mod download／verify、Go 非缓存／race、amd64／arm64／testhooks | 与第一次 B 的执行输入逐项相同，核对原始 checks／logs 和产物后复用 PASS，不重跑相同 Final |
| 合成同步／serve | 同产品、驱动、配置／夹具、依赖；原始 summary、日志与二进制身份匹配，复用 11/11、6/6 PASS，仍不冒充真实固定集闭环 |
| 两辅助工具／直接测试 | 四份 tracked／工作副本逐字节相同；复用第一次 B 的 15 项通过和原始日志 |
| 固定真实 Get／缓存 | 精确复用 Task 2.1 已接受的八场景／11 测试。Go 源／依赖／工具链未变，原 Go 测试二进制摘要匹配，八份原始日志全 PASS 无 skip；现驱动新增可选 CLI Mirror，不改变原场景函数／类，逐项 AST 匹配。该新增 CLI 层不在此复用声明内 |
| 镜像／Compose／容器／arm64 OCI／重启 | 改用 Slice3 接受后的 userdecision 产物，不用早期 e1f66bca。产品／Docker／Compose／运行驱动相同；容器单元测试差异由当前 quality 覆盖。核对 image ID、OCI archive SHA／manifest／arm64 config 后，B 以现有驱动新跑六场景 **6/6 PASS**（含原生停止／重启、共享锁及脱敏 Compose），8.772s；准确 argv／stdout-stderr／退出码新保留。镜像／OCI构建证据按同源码、依赖、Dockerfile和实际产物身份复用，不称本轮重建 |
| 当前 10000 产物 | B 独立只读新跑 PASS：10000 清单／文件／协议映射、0 问题、Size10000／SHA-1零、最大116字节 |
| 当前 DB | 只读核对10000归属、epoch9、非零缺失0；基线epoch2。不是中间轮次／远端恢复证据 |
| 完整范围／本轮补丁 | Base..入口空白检查 PASS；报告后治理／链接／补丁再查并保留 |
| 同批样本真实独立关键复现 | **NOT RUN**，因已发现明确阻断先拒绝，不拿只读审计／合成夹具冒充。B 后续仍负责更新、实际移动／三完整轮、正数缺失再出现清零、dry-run 与恢复；不交回 Developer |
| Slice Final／Closure Packet | NOT RUN／PENDING，RO 尚未齐备，不提前运行或造 Closure |

细节见 `prior-B-input-match.json`、`prior-B-gates.json`、`real-get-reuse.json`、
`packaging-reuse-partial.json`（补跑前的真实阶段记录）、`container/summary.json`、
`container.log`、`dataset-and-runtime-inputs.json`、`checks.jsonl`。
**F-45-02指定候选全量门槛现已由B完成准确复用／针对性补跑；不再作为未齐备阻断。**
AC-6仍缺真实固定样本的独立关键复现，不因全量通过自动满足。
不同 buildinfo 的二进制不称字节相同；原发布包真实测量仍归其
`bcfe9010…40d61`，新包装器修复不倒标旧测量。ARM64 只证明构建／OCI，不是实机。

本轮没有读取容器 Env／历史秘密，没有 S3 写、既有固定服务启停、固定生成器输出／
DB 写入或 Jellyfin 操作。打包驱动只创建／清理本轮专用合成夹具容器与自身临时DB。已有 Env 授权明确写 Developer；B 不自行扩大为 reviewer 容器取密授权。
后续 B 真实复现需在既有安全授权渠道成立后执行；本轮并未验证 B 凭据可用性，
不能再笼统声称“服务无凭据”。只使用查到的现有 CLI／驱动／SDK 入口，不发明业务 API。

## 同 Task 修正分工与验收

**Developer 受影响验证：**

1. 先治理、核对固定清单与已恢复基线。补可追溯的 S3 更新／实际移动新路径／旧路径
   三完整轮清理／同一行正缺失再出现清零／dry-run／恢复所缺原始证据；每步记录准确
   命令、退出码、最少对象路径映射、内容身份、非秘密 Get 结果、生成器行／epoch／计数、
   输出摘要和稳定轮前后状态。源端未知则停止并确认恢复，不强刷、不伪造。
2. 先审查保留记录，能精确补证不重跑。无产品／工具变更时只跑治理、链接与空白；
   真有改动才跑 TASK 中相应聚焦命令。凭据方法调整只验证选择键／内存子进程边界，
   不因此要求全部 Go/race、构建、镜像、serve 重跑。完整全量仍由 B 接收／执行。
3. 纠正“完整移动／清零／逐场景恢复已验收”的过度叙述，保留已发生运行和首次失败。
   不把历史计时／凭据偏差写成修复通过；需要授权或验收取舍时交用户／A。
4. 交准确 implementation checkpoint、文档后继、原始证据索引与 READY FOR B REVIEW，
   不自签独立 PASS。估算当轮测试预算，失败／重跑／只读验收也计入；不足则明确
   NOT RUN／BLOCKED，不空等凑比，不删门禁。

**B 返回门禁：**固定原 Base 审全范围；接收受影响修正，精确保留本轮已完成复用，
保留已齐备的指定全量，完成独立真实关键复现；AC-1..6 和恢复证据齐全、授权问题处置明确后才 PASS。
F-45-02已由B补齐，AC-6真实独立复现仍由B所有，不把全量退给Developer。
不改变 RO／安全不变量；如需改变验收含义，A 先协调并提交合同。

## AC 与全部 RO 重评

- AC-1／2：原生成／初始预览／测量及稳定重复已支持，保留；本轮当前10000输出另行通过。
- AC-3：PARTIAL；更新／缺失／再创建摘要有效，实际移动／重命名缺失。
- AC-4：PARTIAL；有三次成功缺失摘要及阈值后删除摘要，但原始逐轮计数／Get、
  正缺失保留行再出现清零、当次 dry-run 后状态证据未齐。
- AC-5：PARTIAL；当前本地10000和稳定轮摘要有效，远端逐场景恢复原始结果及最终
  稳定轮前后快照不足。不把缺证等同远端尚未恢复或已损坏。
- AC-6：未满足；B指定全量已齐备，真实独立关键复现仍缺。

| RO | 本轮判断 |
|---|---|
| RO-1／2 | 4.4 PASS 保留；清单身份相同，本轮未重生成。远端当前全量未重核，不伪称新做验收 |
| RO-3 | PARTIAL，10000生成／测量成立，但上述真实闭环和证据缺口阻断 |
| RO-4／5 | NOT RUN；4.6未激活，无原始扫描或D-01..08实际决定 |
| RO-6 | PARTIAL；完整范围审查／指定全量已齐备，真实关键复现／最终收尾仍缺 |

**保持 Task 4.5 FIX REQUIRED、Slice 4 ACTIVE；不新造 Task、不提交 Closure Packet、
不返回 SLICE READY FOR A REVIEW，更不自行关闭。**

## 本轮计时与首败诚实记录

按现有 Task 统计实际治理、quality、只读验收、证据身份核对、失败／重跑及最终检查，
准确自动化时间／起止见 `checks.jsonl`、`timing.json`。没有真实长轮复跑，没有空等。
人工阅读合同／源码、审查和撰写修正计划与报告属于审查工作，不伪报成测试执行。
初查曾读取两个不存在的推测路径（审查文档 glob／probe 脚本），均未执行任何产品或
远端探测，随后按实际目录及已存在工具继续；不将这些失败读操作标成测试 PASS。
报告后的治理首次因本轮尝试协调SLICE事实文字而报
`Working Slice contract differs from committed HEAD`；已撤回本轮自己的SLICE改动，
完整保留A已提交合同；重试又因未提交roadmap事实修订被拦截，亦撤回本轮自己的
roadmap改动，只更新Task、Progress和B报告后重跑。两次首败日志保留，不标成PASS。
