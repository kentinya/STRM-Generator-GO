# Task 4.5 第五次 B 复审：完整真实闭环（2026-10-21）

> 最新决定：用户于2026-10-21明确接受已披露历史UNKNOWN，Task4.5续审 **PASS**；
> 以下原FIX REQUIRED保留接受前时点，最终处置与全部RO重评见文末。

## 固定候选与决定

```text
Task: 4.5
Task Base: 5182443d0c34215deb7bb1e74a413564aa8e6422
Implementation Head: 9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa
Reviewed Documentation Successor: a28103604a166d1fb590b317325b67ff50bfb82c
Contract Checkpoint: 48ff46a2cfd742190de0e8a673a92ec61c086950
Decision: FIX REQUIRED
B independent real behavior replay: PASS
Slice Required Outcomes all satisfied: NO
Next: SAME TASK 4.5; USER / A DISPOSITION OF F-45-03 HISTORICAL UNKNOWNS
```

入口 main 干净，治理 PASS；沿用原 Task Base..Implementation Head 的完整审查，核对后继
至上述检查点。相对既有候选没有产品、依赖、部署、验收工具或固定数据集变化。
本次先提交用户明确允许的**此次长场景测试占比豁免**，再执行长轮；不是空等凑比例，
也不把该豁免当成接受历史凭据 UNKNOWN 或授权 Task 4.6。

本机原始证据根：`artifacts/validation-evidence/task-4.5-B7-20261021T-full-replay/`。
`INDEX.md`、`result.json`、`completion-summary.json`、逐轮原始 stdout/stderr、wait4
测量、生成器 DB／文件身份快照、S3 列表和原件备份均保留至 A 决定。这是本机 Git 忽略
证据，不是远端可读 CI 产物。Task／Slice 的 Base 与 Implementation Head 不移动。

## 授权与真实入口

- 先独立核对同一 Compose project `strmgen-slice4-s3` 的 RustFS／OpenList 服务、
  锁定镜像、实际 image ID 和准确挂载；之后才选择 RustFS 两个必要键及 OpenList
  `OPENLIST_ADMIN_PASSWORD`。经已有 `/api/auth/login` 换取 token，只在 Python
  内存和子进程环境使用；`rclone --config /dev/null`，不输出／保存完整 Env 或秘密。
- 只用原发布 CLI、原 API／`/s3`／输出／DB。派生配置只将 cleanup.enabled 从 false
  改为 true，保留 confirmations=3 和所有其他配置；原配置不变。没有单文件过滤、
  小源替身、数据库计数写入或强刷；产品所有列表仍 refresh:false。
- 只变更固定清单内 058／059 两个自有样本；058 使用已固定 B 原件更新，再恢复 A；
  059 用已有 rclone moveto 真正移到新路径，再反向移动恢复。原件及路径先备份／记录。
  不重建媒体／数据集、不碰未知对象、bucket、服务配置／生命周期、生产或 Jellyfin。
- 一次性复现脚本只在本次 evidence 目录，使用既有入口串行取证，不作为产品或新框架
  交付。辅助工作副本与四份 tracked 快照逐字节相同；初次检查误把 tests 路径放在
  tools 下，已按现有文档纠正并保留检查说明，没有据此执行产品或更改数据。

## 独立真实闭环：PASS

1. **完整 dry-run 零写**：自然完成，退出0，skipped=10000、partial=false；全部输出和
   state 的路径／Size／mtime_ns／内容身份、DB 摘要及 epoch26 前后完全相同。
   这次不是 B6 的15秒取消，取消不凑轮次。
2. **更新与真实移动**：058 A→B，实际远端字节 Size24404→24430，STRM updated=1；
   059 真正移到 `B7-Moved-Synthetic 059.mkv`，新路径 created=1、created_round=27。
   真实新 STRM 已保存，其稳定 URL 正确，字节 SHA 与第1轮快照相同。
3. **三完整轮与下一轮删除**：旧059同一 created_round=1 行在 epoch27／28／29 中
   missing_count=1／2／3，文件均保留；epoch30 在源根／旧目标原生 Get 事实核对后
   deleted=1、旧文件与归属退出。CLI 自身按固定代码走删除前 Get，未换清理入口。
4. **同一归属再出现清零**：058 在 epoch28 缺失计数1，epoch29 恢复 A 后清零，
   created_round 始终1；STRM 再次 updated=1、Size回24404。不是删除后新建行冒充清零。
5. **按正常轮恢复输出**：反向移动059后，S3固定路径／Size集合10000先确认恢复；
   epoch31 创建原059，临时新路径归属经历1／2／3三次完整缺失观察（epoch31–33），
   epoch34 正常清理。未手删本地 STRM／归属、未改计数，短暂10001输出符合移动收敛。
6. **最终稳定及全树**：epoch35，created=0、updated=0、deleted=0、skipped=10000；
   全部输出路径／字节／mtime与该稳定轮前相同。最终全树10000/10000、0问题，
   V1／UTF-8／稳定 URL／Size正确；源未提供有效 SHA-1，10000文件均省略而不伪造。
   DB为10000归属、非零缺失0；原058仍created_round=1，恢复059新行created_round=31。

始终未修改的9998份输出，其路径／Size／mtime／内容聚合在每轮都与基线相同。
最终 S3 恰10000固定路径／Size；两个变更样本字节均恢复原 SHA-256。不是重新下载
全部10000对象计算哈希的声明；其余对象没有变更命令，原数据集内容核对证据继续复用。
本次10次发布 CLI 均自然退出0，没有首次失败／重跑。既有真实失败／取消不推进证据
分别保留 dev3 和 B6 的原始适用结果，不谎称本次制造或重跑了故障。

## 实际测量

以下均为子进程发布 CLI 的 Linux wait4 ru_maxrss，单位 KiB，不是 Python 父进程 RSS。
所有轮使用同一源身份；普通完整轮推进一次 epoch，dry-run不推进。

| 运行 | 结束 epoch | 秒 | 峰值 RSS KiB | 退出码 |
|---|---:|---:|---:|---:|
| 00-full-dry-run | 26 | 726.243 | 30296 | 0 |
| 01-update-and-real-move | 27 | 903.570 | 28648 | 0 |
| 02-two-missing-observations | 28 | 873.700 | 28676 | 0 |
| 03-same-row-reappearance | 29 | 879.256 | 30056 | 0 |
| 04-old-path-cleanup | 30 | 829.437 | 29280 | 0 |
| 05-restore-original-path | 31 | 1001.746 | 29248 | 0 |
| 06-restoration-cleanup-second | 32 | 890.666 | 29060 | 0 |
| 07-restoration-cleanup-third | 33 | 878.949 | 28924 | 0 |
| 08-restoration-cleanup-commit | 34 | 949.899 | 29152 | 0 |
| 09-final-stable-repeat | 35 | 1093.995 | 28548 | 0 |

发布 CLI 合计 **9027.461s**；完整一次性取证（含身份／原生API／S3检查、快照、恢复、
只读全树验收）实际 **9168.067s**，均按测试／真实验证计时，不将它们排成“非测试”。
该长场景明确采用用户此次豁免，不宣称低于50%合规；治理、相关链接／空白复核另存
`documentation-checks.json`，最终回合总耗时见 `timing.json`，不改写历史计时。

## 指定全量与候选审查

82项可执行输入逐文件 SHA 与 B5 相同，二进制、清单、配置及四份辅助工作副本匹配。
Task Base..Head 的产品／依赖／部署没有新改动；辅助 wait4、严格协议／树校验及其
直接测试的完整差异已核对，未发现新的产品／工具阻塞。后继为事实文档，不重编译
相同候选或机械重跑 Final。

B1／B2 已齐备的 mod、Go全量／race、双架构与testhooks构建、11项同步／6项serve、
15项辅助测试、固定真实Get／缓存和打包／容器／重启门槛，继续按
[第三次审查](task-4.5-b-review-3.md#指定全量与精确复用) 的身份链及原始结果复用。
F-45-02仍齐备，不交回Developer。B此次完整独立关键复现已不再是 NOT RUN／BLOCKED。
纯事实文档只做治理／相关链接／补丁空白，见证据；Slice Final仍未运行，不能提前收口。

| AC | 当前事实 |
|---|---|
| AC-1／2 | 初次生成／测量保留原始接受结果；本次独立完整dry-run、全树及稳定重复PASS |
| AC-3／4 | 本次更新、真实移动、三轮保留／随后清理、同行清零PASS；故障边界按适用原始证据复用 |
| AC-5 | 本次两样本远端路径／字节、临时输出正常退出及最终稳定恢复PASS |
| AC-6 | 指定全量与B独立真实验证已齐备；安全交接的历史处置尚未闭合，Task不予PASS |

## 唯一剩余 Task 阻塞与全部 RO

**F-45-03／P1：历史凭据 UNKNOWN 的接受或轮换，仍待用户／A明确决定。**
[处置说明](task-4.5-credential-disposition.md) 已由 B 按用户要求纠正文案，不再退回
Developer重复修文档，不要求扫描历史／全盘或证明绝对无残留，也不推断已经泄露。
新轮合规内存取密及真实行为PASS不能追认 dev2历史处置、dev3密码来源／当时授权。
本次时间豁免不等于接受这些未知项，B不擅自轮换账户、变更服务或自签自己文案的独立PASS。

| RO | 重评 |
|---|---|
| RO-1／2 | 保留4.4 PASS；固定资产／清单相同，无重生成 |
| RO-3 | 万文件／同步／清理／恢复的产品行为证据与B独立关键复现齐备 |
| RO-4／5 | NOT RUN；4.6未激活，未扫描／执行插件决定 |
| RO-6 | 指定全量及B真实验证齐备；历史处置、Slice Final、Closure和A最终审查未闭合 |

**Task 4.5 FIX REQUIRED，Slice 4 ACTIVE。** 不再要求Developer重跑已接受行为或全量，
不为润色另造Task；用户／A处置F-45-03后回到同Task核对，不借机扩大范围。
本次不激活4.6，不提交“全部RO已满足”的Closure，不返回SLICE READY FOR A REVIEW，
不自行关闭Slice。

## 2026-10-21 续审：用户接受历史 UNKNOWN，Task PASS

```text
Reviewed: 5182443d0c34215deb7bb1e74a413564aa8e6422..9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa
Reviewed Documentation Successor: 481be61b158542c47d6baa6bf3e32a9ed4d7ba7b
Decision: PASS
F-45-03: ACCEPTED BY USER; HISTORICAL FACTS REMAIN UNKNOWN
B independent real behavior replay: PASS
Slice Required Outcomes all satisfied: NO
Next: B PLAN TASK 4.6 WITHIN ACTIVE SLICE; NO CLOSURE YET
```

B解释剩余项是dev2临时凭据文件处置、dev3登录密码来源／当时授权的历史记录未知，
而非程序故障；没有据此认定泄露。区分“接受历史未知”与“要求轮换”后，用户答复
**“接受”**。这是待定风险的用户处置，不是B证明历史操作合规或无残留。
[处置说明](task-4.5-credential-disposition.md)已同步，原UNKNOWN和审查事实保留。
F-45-03不再阻断，也不再向Developer派同一说明／重跑要求，不做账号／服务变更。

入口main干净、治理PASS。续审核对94份B7证据文件摘要全部匹配、82项执行输入及
二进制／原配置／清单相同；当前10000输出和state逐文件路径／Size／mtime／内容
身份与B7最终一致，DB仍epoch35／10000归属／0非零缺失。已接受全量和B完整真实闭环
继续精确复用，不机械重跑；本次没有产品改动、凭据读取、远端／媒体／服务操作。
本轮只做接受登记、上述只读身份核对及治理／相关链接／空白检查，证据见
`artifacts/validation-evidence/task-4.5-B8-user-acceptance-20261021/`。

AC-1..6、指定全量、独立真实复现、恢复和历史处置均齐备，**Task4.5 PASS**。
RO-1／2／3满足；RO-4／5未执行，4.6候选未激活；RO-6的4.5门禁完成，Slice Final、
Closure和A Final仍未运行。Slice保持ACTIVE，不提前交已满足全部RO的Closure、
不返回SLICE READY FOR A REVIEW，不自行关闭。不新增4.5润色任务；后续按原Slice
范围正式规划4.6，该用户接受不扩大4.6凭据或运行授权。
