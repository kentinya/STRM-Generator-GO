# Task 4.6 第二次 B 审查（2026-10-22）

## 固定候选与决定

```text
Task ID: 4.6
Task Base: b599dbc5c0e6bd394ee361343d74ebbb12d1ecab
Implementation Head: 69d022e2f7acb41fe1002fad9652386e4460cc75
Reviewed Documentation Successor: e42dc384b2abbb6946cec82cfe0995e76e71d4af
Decision: FIX REQUIRED
Slice Required Outcomes all satisfied: NO
Next: SAME TASK 4.6; FIX NATIVE-ONLY EVIDENCE AND RESTORE; USER / A DISPOSITION OF OUT-OF-SCOPE EFFECTS; B RETAINS FINAL GATES
```

入口 `main` 干净；已读 AGENTS、开发流程、V4.4、范围、需求、用户旅程、架构、路线、
SLICE／TASK及三角色指南，实施前治理 PASS。审查完整 Task Base..Implementation Head
及全部后继，含正式计划／授权／认证交接、七份 Python 工具／测试、Developer 报告及
关联文档；没有 Go 产品、依赖、部署或4.4／4.5辅助工具改动。不新建 Task，不移动 Base
或实现 Head，不修改插件、服务、数据或凭据。按流程第4节，明确阻塞下提前拒绝；本轮
不执行已知不符合 Task 边界的真实矩阵，不把独立复现或全量交回 Developer。

本机证据根：`artifacts/validation-evidence/task-4.6-B2-20261021T234826Z/`（UTC命名，
本地日期为2026-10-22；Git忽略，保留至A决定，不是远端CI产物）。命令、退出码、
复用比较、原始窗口核对、恢复入口复现及本轮计时分别保留。
本轮文档协调尚未提交时，治理如实返回`Working roadmap differs from committed HEAD`；
这是提交合同一致性保护，不改检查器绕过，按正常本地审查检查点提交后再复查。

## 先核对已交付的结果

- 原始副本的源／副本摘要一致；原始扫描后有 item／source 以及该条目的原生任务／
  插件探测启动记录，支持 AC-1 的扫描／读取结果。原始 URL 在容器内不可达、无必要
  主流是已披露限制，不把它提升为新阻塞，不要求改原字节、代理或探测成功。
- 基准 A 有正常原生入口及明确 `Sha1Matches` 记录。八份逐行原始窗口的 item 与
  `03-matrix-rows.json` 一致；D-01／02／03为Sha1Matches，D-04／05／06确有探测启动，
  D-07为Unchanged，D-08为InsufficientFingerprint。D-02保留实际源端时间变化；
  三个探测行失败不因合同不要求成功而被改写为探测成功。它们不是“完全没有交付”。
- 当前原输出独立只读校验 **10000/10000、0问题**；固定清单、CLI、A/B原件、ZIP、
  安装DLL与4.5生成器DB摘要匹配。DB仍为已接受摘要 `985d0370…59b8`，本轮未读
  Jellyfin或插件DB，未取密、登录、恢复账户、触发真实扫描／任务或修改S3。
- `06-final-state.json`记录了最终插件配置恢复；不能把下述restore入口缺陷说成
  最终仍未恢复。第二次临时Key结束结果已保存：显式撤销、同Key返回401、恢复会话
  退出204。当前无可复用活跃租约；旧交接页“当前有效”属于历史，已附最新事实。

## 阻塞与同 Task 修正

### F-46-02 / P1：矩阵通过禁止的插件数据库读取建立证明链

`docs/task-4.6-artifacts/fingerprint_read.py:18-30`直接打开隔离Jellyfin下的插件
`strm-fingerprints.db`并查询`strm_fingerprint`；`matrix.py:290-296`在实际行执行中
调用它，原始矩阵含逐行`fingerprint_before/after/after_restore`。这是已使用的入口，
不是构造假设。只读不等于获准：根TASK AC-2明确不得读写Jellyfin／插件DB，实施指南
第4节也明确不得直接读插件指纹库。报告“未写DB”不能替代该边界。

**修正：**退出这个直接DB读取入口及矩阵依赖，保留旧证据并如实标记历史不合规；
用已有原生扫描／检查／公开API、基准副本与此前明确复用的连续证据说明历史侧，
保留每行主流／地址／当次决定／恢复，不伪造指纹或直接提交插件内部状态。
Developer仅补该受影响链路和直接验证；B随后仍亲自复现原始扫描与全部8行。
不要求改变插件、增加元数据服务或枚举合同之外的组合。

### F-46-03 / P1：既有 restore 入口会把当前临时配置当成原配置

`phase_setup`将前值放在`RESULTS["setup"]["plugin_configuration_before"]`，但
`matrix.py:580`读取顶层同名键，缺失后退回`plugin_config()`，即当前已缩小的库范围。
单独执行`restore`的新进程也没有前值。原始`04-restore.json`实际标成
`plugin-config-restored`的仍是`BatchAllLibraries=false`／决策库ID；随后
`06-final-state.json`才显示人工恢复为原值。最终人工恢复有效，不掩盖入口缺陷。

B已直接调用现有`phase_restore`，仅stub外部I/O（无服务／数据副作用），使用setup
实际产生的状态形状：原值`true/空`，当前`false/决策库`，restore后仍为后者。
复现见`restore-entry-reproduction.json`；不猜路由、不调用不存在的入口。

**修正：**从本次已保存的准确前值恢复，支持交付中实际采用的分阶段调用；前值缺失
时明确停下，不以当前值冒充恢复。只补这个入口的直接测试（连续阶段、分进程阶段、
缺前值）和获准原生配置的实际恢复核对，不建设通用恢复平台、不重跑万文件同步。

### F-46-04 / P1：已有超范围副作用须由用户／A处置，B不追认

Developer报告与`05-diagnostic-side-effect.json`明确披露：为了诊断零候选，曾用
`BatchAllLibraries=true`处理历史201条目，留下196条FirstTrust指纹。根TASK要求
仅检查专用库，隔离做不到先报告；这不是最少样本范围内的必要操作。现有“历史流／
地址／旧5行不变”的说明保留为Developer披露，不冒称B本轮直接DB复核或全部回滚。

报告还明确由Developer在首租约到期后再次运行官方恢复、改管理员密码并重发Key；
AGENTS记录的该副作用授权给B，Developer应通过B的内存交接使用。现有第二租约日志
证明恢复／撤销发生，不能单凭相同脚本名或B-verification客户端字段证明B实际办理。

**处置：**保留现有披露，不再要求追查历史秘密或证明绝对无残留；如实际办理角色有
事实误述，只补已有非秘密记录。已发生的扩大范围／角色授权偏差交用户／A决定是否
接受及是否要求经原生入口处置，B不擅自接受、不直接清库／删除历史行、不再次恢复
账户来粉饰历史。未来凭据过期由B按授权协调，不再让Developer自行重置；此次4.5
UNKNOWN接受不自动覆盖4.6。该事项不意味着产品失效或已发生秘密泄露。

## B 指定门槛与证据复用

| 门槛 | 本轮事实 |
|---|---|
| 当前治理 | PASS；文档后继再查 |
| 仓库Python回归 | 本次111项PASS，日志`repository-python-tests.log` |
| 新工具直接回归 | 本次16项PASS；不覆盖restore／禁止DB访问问题，不抵真实矩阵 |
| 原输出全树 | 本次10000/10000、0问题，约2.892秒 |
| 当前产品／依赖／部署／原辅助输入 | 与4.5 B5的82项逐文件SHA完全相同；Task范围无这些输入改动 |
| mod、quality中的Go格式／vet、非缓存Go／race、双架构／testhooks、15项4.5辅助测试 | 核对4.5 B1原始命令／退出码／日志摘要及B2二进制摘要匹配，保留适用复用；不改称本轮新跑 |
| 合成同步／serve | 原始11／6场景summary无失败，日志／执行二进制与已接受身份匹配，复用 |
| 固定真实Get／缓存、容器／Compose／arm64 OCI／重启 | 保留4.5 B2／B3／B5已接受身份链；本轮不重复启动服务或机械重跑Final，最终签收时仍由B确认必要运行环境／产物，不交回Developer |
| B原始扫描／8行真实独立复现 | NOT RUN：本轮明确阻塞下提前拒绝，未用Developer结果冒充B观察 |
| 当前4.6完整T4签收／Slice Final／Closure | NOT RUN／PENDING；已有可复用子门槛不是整个Task通过 |

两条B1核对记录原本为结构化审计而非同名`.log`，不能将缺少猜测的日志文件名当成
产品FAIL。4.6四份工作副本与tracked文件仅`parents[2]→parents[3]`的路径适配差异，
已经逐份记录；不称逐字节相同。原始INDEX的20份非自身条目摘要匹配，自身INDEX条目
已失效；修正交接补准确执行输入摘要／路径差异、命令／退出码及无自引用的索引，
保留首败／重试，不重写旧文件来伪造当时输入身份。

## 受影响验证、返回门禁与全部 RO

Developer继续**同一Task4.6**，只修上述原生证据链／精确恢复和准确交接，跑新增与
受影响工具测试／实际必要场景；不接管B全量、不改Go产品／锁定插件／业务RO，不重建
数据集、重跑4.5多轮或通过直接清库处理历史副作用。纯事实补充仅治理／链接／空白。

B在修正后固定新实现Head，审完整原Task Base..Head及后继，匹配已接受门槛、补变化
部分，独立执行原始扫描／完整8行和恢复；AC-1..6、范围处置、指定全量及B实证全部
齐备才PASS。不要求探测结果成功。该责任分工与原计划相同，未删减门槛。

| RO | 本轮重评 |
|---|---|
| RO-1／2 | 保留4.4 PASS，固定清单／原件身份一致，未重生成 |
| RO-3 | 保留4.5 PASS；当前10000输出／DB身份匹配，不要求重复已接受闭环 |
| RO-4 | Developer原始扫描／读取证据支持结果，B独立原生复现尚未执行 |
| RO-5 | 八行当次日志已有；禁止DB依赖／范围处置及B独立实证未闭合，不签PASS |
| RO-6 | 原门槛可复用部分保留；当前完整签收／Slice Final／Closure／A Final未完成 |

本轮所有治理、单测、恢复入口复现、证据摘要匹配、只读验收及文档检查均计入测试；
准确墙钟与保守汇总见`timing.json`，严格小于本轮50%，没有采用4.5豁免、空等或将
测试等待排除。**Task4.6 FIX REQUIRED，Slice4 ACTIVE**；不造新Task，不交已满足
全部RO的Closure，不返回SLICE READY FOR A REVIEW，不自行关闭。
