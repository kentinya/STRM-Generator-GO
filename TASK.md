# Task 6.1 — Telegram 通知的本地完整链路

```text
Task ID: 6.1
Parent Slice: 6
Status: PASS
Task Base: 1bfac55136dc4d3137d9ee9d470ec5c6ba840749
Contract Checkpoint: 1bfac55136dc4d3137d9ee9d470ec5c6ba840749
Difficulty: High
Test Level: T4
Implementation Head: 757b082855974c43498ff7e3f5896b31d657a4ea
```

以下为原规划／激活记录；当前状态以完成报告及末尾最新补充为准。
2026-11-12 用户授权本会话代替 B 规划首个 Task，正式计划已提交；用户随后授权激活，
Task 进入 IN PROGRESS。本次只激活任务，产品代码尚未改动；
不发送真实 Telegram 消息，不签发独立 B／A 产品 PASS，不 push。
能力边界以 [Slice 6](SLICE.md) 在上述 Contract Checkpoint 的精简合同为准；
Slice Base 保持 `20677f1906837428324d58885e719ed385a07258`。
合同正文中的“等待 B”描述其激活时点；当前任务状态以本文件及合同 Closure Packet 为准。

## Goal / Scope

交付一个可从正常 `sync`／`serve` 入口验证的闭环：读取通知配置 → 同步原流程取证 →
释放同步资源 → 按策略发送中文摘要与可选单个附件 → 清理本轮临时文件。
同步失败仍报告已发生事实；通知失败只降级通知，原产物、计数、退出码及后续调度保持原语义。
本 Task 覆盖本地实现、合成接收验证和操作文档；真实 Telegram 文本／附件送达另行取证。

| 范围 | 实施与验收要求 |
|---|---|
| 配置／启动 | 实现合同的 `notifications.telegram` 五个字段、默认值、严格校验及三种策略；凭证缺失只安全警告并关闭本进程通知；关闭和 dry-run 不读通知凭证、不建清单、不发请求 |
| 实际操作事实 | 新增、更新、实际删除、归属回收；每条含 source 编号和输出根内 STRM 相对路径。保留文件操作已成功而后续步骤失败的事实；不从采样日志反推清单 |
| 缺失进度 | 提交后的首次进入、推进、达阈值，以及独立持久化清零；带可靠前后值、阈值和现有原因。暂存观察不冒充提交；冻结仅复用来源级原因／统计 |
| 摘要／附件 | 所有来源聚合为一条摘要，区分初始化失败、部分失败、冲突及成功；保留既有统计含义。按事件发生顺序预览与流式单附件，不排序或归并 |
| 传输／生命周期 | 官方 HTTPS `sendMessage`，必要时 `sendDocument` multipart 上传；30 秒共同预算、取消、响应上限、拒绝重定向、无重试、错误脱敏；释放锁后串行发送，结束清理临时文件 |
| 部署／说明 | 根与 deploy 配置／环境示例、Compose 透传和现有维护文档按需要更新；默认配置关闭；说明策略、消息含义、容量、降级及真实送达未验证边界 |

本 Task 固定附件**总大小上限为 8 MiB（8,388,608 字节，包含头尾说明）**，不增加配置项。
追加前检查完整事件是否还能容纳，并为“不完整”说明留空间；达到上限停止追加文件事件，
继续同步及事件总数统计。正文和附件均标明不完整，不能把容量截断称为完整清单。
该上限低于合同引用的 Telegram 上传限制，并为现有容器 64 MiB `/tmp` 留出余量；
不据此承诺任意数据集都能生成完整附件或整个临时目录永不耗尽。

正文最多 3500 个 UTF-16 code units、20 条预览；按合同转义控制字符／双向控制符，
Unicode 截断有效。正文容不下全部事件或来源明细时才上传单个 UTF-8 `.txt`。
目录 0700／文件 0600；不要求临时通知文件 fsync、原子发布或崩溃恢复。

## 真实代码依据与最小改动边界

- [CLI](cmd/strmgen/main.go) 的 `runRound` 已用 defer 关闭输出／数据库并释放锁。
  共用包装在它返回后处理通知；同步耗时在发送前结束。serve 通知在 round 包装返回前完成，
  使现有 scheduler 的 next 计算与随后等待衔接；不把耗时发送放进其事件回调，不加队列或 timer。
- [reconciler](internal/reconciler/syncer.go) 的 `SourceResult` 已有业务统计，诊断仅采样 100 条，
  `ConfirmedMissing` 仅表示暂存。新增可选事实接入，直接在原操作窗口捕获事件，
  不增加远端查询、全库扫描或全量路径切片；关闭时不收集通知明细。
- `noteObserved` 在文件处理前就调用 `ClearMissing`，不能到 `handleFile` 才查旧计数。
  确认缺失和删除重核的清零窗口也须覆盖。清零不依赖本来源后续完整轮是否成功。
- [storage](internal/storage/storage.go) 的 `ClearMissing` 在主库更新后还会清理 scratch，
  `CommitRound` 在 `tx.Commit` 后也有 scratch 清理；两者返回 error 都不充分说明持久化未发生。
  最小调整内部结果接口，表达可靠前后值与实际提交结果，保留原事务、epoch 防重复和计数语义。
  按现有有界批次处理，事务内不能经 Store 再查主库（主连接池只有一个连接）。
  通知写入失败不能成为事务失败条件；未提交的候选不能发布成有效进度。
- [文件系统](internal/reconciler/fs.go) 已有 `DeleteOutcome.Removed`／`AlreadyAbsent`。
  直接复用它们，在真实删除窗口取证；后续目录持久化／归属失败不抹去 Removed。
  本 Task 不为通知重写发布、删除、恢复协议；若发现必须改变其业务语义，先回报范围问题。
- 新增 `internal/notify` 集中格式、临时清单及 HTTP；reconciler／storage 只提供结构化事实，
  不依赖 Telegram。优先标准库，不引入通知框架、通用事件总线或可配置生产 API 地址。

如何在一个本轮文件内区分提交成功／失败，由 Developer 选择最小内部实现；验收只认可
实际持久事实和有界资源，不要求额外暂存表、第二套待删状态、跨轮协调或可恢复通知日志。
修正受改接口影响的错误诊断，例如提交成功后不能仍写“no round committed”。

## 验收条件与 RO 映射

以下为未来实施门槛，当前全部 NOT RUN。A 合同逐条义务仍适用，不能用此表缩减范围。

| ID | 可观察结果／必要场景 | RO |
|---|---|---|
| AC-1 | 旧配置、缺省关闭、显式启用、合法／非法字段类型与枚举、环境变量名、实例名；通知凭证缺失只警告，同步退出码不变；dry-run 不要求凭证，无通知文件或请求 | RO-1、5 |
| AC-2 | sync／serve 同一轮只发一条多来源摘要；成功、冲突且 Errors=0、部分／全部失败、装配后初始化失败正确分类；无报告时计数不可用；busy-skip、取消不发送，装配前参数／配置错误仅本地诊断 | RO-1、2 |
| AC-3 | all／change_or_error／error 各自触发正确；仅 skipped／filtered／保留不触发 change_or_error；已提交计数增加或清零能触发；源失败即使无文件变化也能触发错误策略 | RO-1、2、4 |
| AC-4 | 新增／更新／删除／回收逐条与实际相对路径核对，多来源及超过 100 条事件不受诊断采样截断；不把 STRM 更新说成媒体更新；删除后目录或归属处理失败仍显示实际删除及后续失败 | RO-2、3 |
| AC-5 | 源目标未读到及扩展名排除各覆盖 0→1→2→3、达到阈值后下一轮重核删除；阈值可配置，通知显示实际阈值；恢复入选且源可读清零，清理关闭不新增缺失证据 | RO-4 |
| AC-6 | 遍历／本地失败、取消不虚报新增进度；事务回滚无有效推进；主库清零成功而 scratch 失败、清零后来源失败、tx.Commit 成功而 scratch 失败分别如实报告；重复 epoch 不伪造推进 | RO-2、4、5 |
| AC-7 | 中文、emoji、超长路径、换行与双向控制符安全展示；正文／预览上限、单附件、来源明细溢出、大小边界及超限部分清单；附件内完整转义路径，事件总数与省略数不冒充完整明细 | RO-3、5 |
| AC-8 | HTTP 非成功、JSON ok=false／无效响应、429 无重试、64 KiB 响应上限、重定向拒绝、超时与取消；摘要失败不发附件、附件失败不补发状态；文本与附件共用 30 秒且锁已释放 | RO-5 |
| AC-9 | 通知关闭／启用／临时文件创建或写入失败／网络失败对照同一同步输入，核对 STRM、SQLite、轮次、计数和退出码；旁路失败后原清理继续，正常失败恢复仍收敛；正常结束／错误／取消清理本轮临时文件 | RO-4、5 |
| AC-10 | serve 慢发送不重入／积压，next 与实际等待一致；通知失败后继续下一轮；发送中停止及时取消并释放资源，未使用独立结束上下文 | RO-1、5 |
| AC-11 | 示例与产品解析一致，Compose 使用无值凭证透传，安全渲染无真实值；只读根与私有 `/tmp` 下摘要／附件可用；诊断、正文和附件无 token、Chat ID、原始响应、媒体 URL 或私有绝对路径 | RO-5、6 |
| AC-12 | 完成报告固定真实 Head、完整 Base..Head、命令和原始证据映射；真实 Telegram 明确 NOT RUN，剩余 RO 及独立审查保持 PENDING，不将 mock 记为真实送达 | RO-6、7 |

## 测试与证据计划

难度 High 来自跨 CLI、同步事实、持久化结果与传输生命周期的接入；T4 来自对 storage
提交／清零结果接口的必要调整。配置／传输本身按 T3 覆盖，T4 复用现有全部回归和适用恢复、
打包验证，不扩展到无关真实 Jellyfin／插件链路。

1. 先补直接受影响的配置、storage、reconciler、notify 和 CLI 测试。
   复用现有故障窗口及 Go `httptest`／可注入 HTTP transport，核对实际请求正文和 multipart 内容；
   不增加生产 Telegram 地址覆盖项。必须从正常 CLI 装配进入验证，不能只有孤立 formatter 测试。
2. 执行 `python3 scripts/quality.py`，覆盖治理、Python、语法／空白、gofmt、vet、全部 Go test／build。
   同一最终候选执行 race：

   ```bash
   CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 ./...
   ```

3. 按[已有测试入口](docs/development-workflow.md#测试入口)构建普通／`strmgen_testhooks` 二进制，
   放入 `artifacts/task-6.1/bin/`。用 `run_isolated_sync_integration.py` 与
   `run_serve_integration.py` 的 `--scenario all` 执行全部现有本地场景，
   补齐上表的通知对照、计数与失败窗口；已由这些入口覆盖的聚焦检查不机械重复。
4. 用现有 Dockerfile 构建本地 amd64 镜像并执行 `run_container_integration.py` 的适用既有场景
   （静态镜像、最小运行、serve 生命周期、跨容器锁、Compose 生命周期），
   在专用隔离目录验证本功能启用后的摘要／附件及临时目录权限。
   用 `docker compose -f deploy/compose.yaml config --no-interpolate` 捕获安全配置。
   容器通知验证复用测试接收方式／现有 testhooks 模式，不建设通用模拟平台。
   固定双架构构建可复用既有构建入口；ARM64 原生实机不在本 Task 范围。
5. 所有集成证据使用 `artifacts/validation-evidence/` 下新的空目录，记录准确源码 SHA、
   配置／夹具、命令／cwd、退出码、产物摘要、请求数及对应 AC／RO；保留首次失败及重跑。
   文件权限、锁释放、旁路隔离和计数须有断言，不能只凭日志文字判 PASS。

真实 OpenList、固定万文件、Jellyfin／插件实际决定及真实 Telegram 当前 NOT RUN，
不能拿本地回归改写历史验收。若实际代码超出本计划边界而影响原远端缺失判定／兼容行为，
由 B 重评适用检查，不能自行豁免或扩大业务范围。

## 非目标与交付边界

不增加业务库表／字段、通知历史、持久队列、发送状态、跨轮去重、最终状态归并、
外部排序、附件分卷、逐文件冻结清单、新锁／后台任务、重试／补发或 Bot 收消息功能。
不改变删除资格、三完整轮最低阈值、先清理后提交的时序、STRM 协议及正常恢复边界。
不操作生产服务／媒体、配套仓库，不从历史文件恢复任何凭证。

Task 6.1 的独立 B 审查可判定本地范围是否通过，但不能据此关闭 Slice 6。
完成本地范围后重评剩余 RO；真实送达须在用户另行明确授权的测试 Bot／聊天中，
从正常产品 CLI 验证摘要和附件并保存脱敏证据，当前不预签下一 Task 或真实验收 PASS。

## 完成报告与审查

```text
Implementation Status: PASS (prior independent local review plus user-requested P2 self-review)
Reviewed Range: 1bfac55136dc4d3137d9ee9d470ec5c6ba840749..7c54baf660150ced9ac87e41d34dc4631f923fa1
Fix Range: 20e5325fd034b5295d383cd82d2d966d63252a53..9dbb58c20be182edbdc64637e1c9ee9a79c901d7 (B-01..B-08 corrections within Task 6.1)
B-R Fix Range: 9dbb58c20be182edbdc64637e1c9ee9a79c901d7..7c54baf660150ced9ac87e41d34dc4631f923fa1
Local Evidence: independent quality/full race/prior failure probes/notification CLI/serve 8 of 8 PASS; Developer full sync 14 of 14 and container 6 of 6 evidence reviewed
Real Telegram Evidence: NOT RUN — no authorized test target or credentials
B Product Review: independent PASS through 7c54baf; P2 self-review PASS by explicit user request (not independent)
A Final Review: PENDING
Remaining Work: separately authorized real Telegram evidence; A final Slice review
```

### Developer 修正轮报告（2026-11-12，Head `9dbb58c`）

在原 Task 范围内修正独立 B 审查全部八项，未扩大业务范围、未改删除资格、协议或
远端兼容语义；旧检查点 `606e310` 与首次失败记录均保留。逐项修正与测试证据见
[artifacts/task-6.1/EVIDENCE.md](artifacts/task-6.1/EVIDENCE.md) 修正轮小节及
[artifacts/validation-evidence/task6.1-fix-20261112/metadata.json](artifacts/validation-evidence/task6.1-fix-20261112/metadata.json)
（输入 SHA、二进制摘要、命令、退出码、原始日志，含首轮失败）。

- **B-01（P1）**：`Store.CommitRound` 改为可选 `AdvanceSink` 暂存协议：推进记录
  在原事务窗口内按 200 项有界批次捕获，事务提交成功后 `CommitAdvances` 才发布为
  有效进度，事务失败 `DiscardAdvances` 丢弃；无 sink（通知关闭）时不构造任何明细。
  内存不再随全源条数增长；未新增业务状态、第二套持久清单或表字段。
- **B-02（P1）**：`LocalFS.Publish`／`Update` 返回 `PublishOutcome.Published`／
  `UpdateOutcome.Replaced`，表达链接／交换完成后目标已持有新字节的事实；后续
  目录持久化失败仍计入并保留事实，同步仍失败，归属／恢复协议不变。
- **B-03（P1）**：`ClearMissing` 在主库清零成功而私有工作区清理失败时返回持久
  事实 + 错误；三个清零窗口（noteObserved、confirmMissing、删除重核）均在检查
  错误前发布已持久化的 N→0 事实，不再漏报。
- **B-04（P1）**：整个正文（标题、来源明细、事件预览与尾注）共享 3500 UTF-16
  预算；来源明细溢出进入同一单个 8 MiB 附件（零文件事件时同样成立）；
  初始化失败摘要保留已知配置来源总数，完成／未完成计数标为不可用。
- **B-05（P1）**：来源诊断、实例名等全部动态展示内容统一 `EscapeText`
  （含 U+202E 双向控制符），正文与附件一致。
- **B-06（P1）**：临时清单创建失败不再静默：无 collector 时按策略发送降级摘要，
  正文标注“明细不完整、缺失计数进度未知”，不把未知报告为 0；change_or_error
  回退报告计数、不猜测缺失进度；busy-skip／取消仍静默。
- **B-07（P1）**：正文预览与附件逐事件携带原因与阶段说明（首次进入待删除、
  计数推进、达到阈值等待后续重核且不等于已删除、已确认恢复条件清零、已实际
  删除、归属回收说明）；完成来源的安全保留诊断进入来源区，不再丢失。
- **B-08（P2）**：`resolveNotifications` 在 enabled: false 下仍校验已提供字段；
  自定义 `UnmarshalYAML` 拒绝非字符串 `instance_name` 并手工维护未知字段拒绝；
  移除合同外 64 字符上限。校验不读凭证、不创建文件。
- **补齐证据**：`TestSendRoundCleansUpAfterDeliveryFailure` 改用本地失败
  transport；AC-8 附件失败分支与共享预算取消用例；AC-10 serve 驱动新增 Phase F
  （接收端 hold：阻塞发送期间并行 sync 完成证明锁已释放、不重入不积压、SIGTERM
  发送中及时取消且非零退出）；AC-11 容器 notifications 场景在发送阻塞窗口实测
  临时目录 0700／清单 0600。B 审查遗留的 overlay 测试副本移入同目录 `_overlay/`
  子目录（内容不变）以恢复 `scripts/quality.py` 的 `go vet ./...` 入口。
- **验证（最终候选 `9dbb58c`）**：`python3 scripts/quality.py` PASS；race
  `CGO_ENABLED=1 -race -count=1 ./...` 与 tagged CLI PASS；sync 集成 14/14、
  serve 集成 8/8（含 Phase F）、容器集成 6/6（含权限断言）PASS；Compose
  `--no-interpolate` 无值透传通过。首次失败日志保留于证据目录。
- **剩余限制与风险**：真实 Telegram 送达 NOT RUN（无授权测试目标／凭证）；真实
  OpenList／Jellyfin／插件链路与 ARM64 原生实机不在本 Task 范围；附件 8 MiB 上限
  行为由单元与二进制层覆盖，未在容器内复现 8 MiB 级数据集；无 collector 时
  change_or_error 的变化判定回退报告计数（缺失进度不可知，如实不触发）。

原交付流程：实施后在本节填入实际实现 Head、变更摘要、AC／RO 证据、首次失败／修复和剩余限制，
再交独立 B 审查完整 Task Base..Head；实施者不得自签 B PASS。
原计划由本会话代行 B 制定；规划与本次激活均不构成独立产品审查。
规划与激活文档按 T0 核对合同一致性、检查点可达性、链接／锚点、补丁空白和提交后治理；
本节 NOT RUN 指产品实施证据，规划检查不代替产品测试。

Developer 报告（Task Base `1bfac551` → Implementation Head `606e310`）：

- **实现范围**：`notifications.telegram` 五字段配置与三策略（缺省关闭、旧配置兼容、
  严格校验）；`internal/storage` 最小结果接口扩展（`CommitResult` 携带已提交推进与
  committed 事实、`ClearResult` 携带清零前计数、scratch reason 列），未改业务表；
  `internal/reconciler` 可选 `FactSink` 在原操作窗口取证（新增／更新／实际删除／
  归属回收、计数推进与清零）；新增 `internal/notify`（摘要、单个 8 MiB 流式附件、
  官方 Bot API 传输）；`cmd/strmgen` 装配与每轮生命周期（锁释放后串行发送、
  busy-skip／取消不发送且清理临时文件）；文档、示例与 Compose 无值凭证透传。
- **验证**：`python3 scripts/quality.py`（含 tagged CLI 通知测试）PASS；race 全部包与
  tagged CLI PASS；sync 集成 14/14、serve 集成 8/8、容器集成 6/6（含新增
  notifications／notifications_cleanup／serve_notifications／容器 notifications 场景）
  PASS；Compose `--no-interpolate` 脱敏渲染通过。证据与 AC 映射见
  [artifacts/task-6.1/EVIDENCE.md](artifacts/task-6.1/EVIDENCE.md)。
- **首次失败／修复**：集成驱动发现 busy-skip／取消路径临时目录残留
  （defer 注册时机）→ 修正并补回归测试；容器场景回连地址改用 host-gateway；
  驱动初版跨根对照断言过严 → 改为字节／业务行对照。后两项为驱动侧修正，
  产品代码无改动。
- **剩余限制与风险**：真实 Telegram 送达 NOT RUN；真实 OpenList／Jellyfin／插件与
  ARM64 原生实机不在本 Task 范围；附件 8 MiB 上限行为由单元与二进制层覆盖，
  未在容器内复现 8 MiB 级数据集；collect 器损坏时 change_or_error 的变化判定
  回退到报告计数（缺失进度不可知，如实不触发），已在代码注释说明。

原交付流程：实施后在本节填入实际实现 Head、变更摘要、AC／RO 证据、首次失败／修复和剩余限制，
再交独立 B 审查完整 Task Base..Head；实施者不得自签 B PASS。
原计划由本会话代行 B 制定；规划与本次激活均不构成独立产品审查。
规划与激活文档按 T0 核对合同一致性、检查点可达性、链接／锚点、补丁空白和提交后治理；
本节 NOT RUN 指产品实施证据，规划检查不代替产品测试。

### 独立 B 审查（2026-11-12）

**Decision: FIX REQUIRED。** 本轮仅审查与维护交接状态，未修改产品实现，未代替 Developer
修复后自签 PASS，未启动多代理、push 或发送真实 Telegram 消息。

- 完整 Task 范围：`1bfac55136dc4d3137d9ee9d470ec5c6ba840749..606e31089a2f41fe09f57610f34ce006f622828b`。
  另核对 Slice Base `20677f1906837428324d58885e719ed385a07258` 以来的合同／规划增量，
  以及交付报告 Head `e7df63213dea2497a0fead6f1b3ad544d7321e8d`；后者只改 SLICE／TASK，
  产品代码与 Implementation Head 相同。固定 Base、合同和实现检查点均不移动。
- 角色依据为[开发流程的任务与审查](docs/development-workflow.md#任务与审查)；当前文档已合并
  旧角色指南，不从历史角色文件引入新范围。先检查状态（干净，main ahead 6）与治理（PASS）。
- 新证据位于本机 `artifacts/validation-evidence/task6.1-b-review-20261112/`：
  `metadata.json` 记录输入 SHA、二进制摘要、命令、工具链及退出码；`cli-results.json`、
  `failure-windows.log` 为下列失败原始结果。此目录不随克隆提供。

| ID／级别 | 真实入口、发现与影响 | 原 Task 内修正及验收 |
|---|---|---|
| B-01 / P1 | `Store.CommitRound`（storage.go:980、1017）跨所有 200 项批次持续 append `Advances`，直到整源提交后返回；即使通知关闭也收集全源路径。大量既有文件缺失／被扩展名排除时，内存随全源条数增长，违反有界处理与禁止全量路径切片；附件 8 MiB 截断并不限制该切片。代码审查确认。 | 用有界批次捕获原事务前后事实，仍仅在真实提交后发布有效进度；关闭时不构造通知全量明细。不得新增业务状态／第二套持久清单。以既有 storage／reconciler 测试覆盖多批、回滚、重复 epoch、commit 后 scratch 失败及关闭路径；审查资源界限，不新增规模平台。 |
| B-02 / P1 | 正常 `sync` 更新在 `LocalFS.Update` 原有 afterUpdate／目录持久化失败窗口中，目标已是新字节，但 syncer.go:605 见 error 即返回，`Updated` 与事实均为 0。复用既有 LocalFS 注入点的 `TestBReviewUpdatePublishedBeforeFailure` 实测失败。`Publish` 链接成功后 `dir.Sync` 失败也存在同类接口盲区。 | 最小表达实际发布／替换结果及后续持久化不确定性，失败摘要保留实际操作；同步仍失败，归属／恢复协议不变。用既有文件系统故障入口断言原字节、新字节、事实、失败及重跑收敛，覆盖新增与更新，不只测 DB 登记失败。 |
| B-03 / P1 | `ClearMissing` 主库已成功清零后，scratch DELETE 失败返回空 `ClearResult`（storage.go:768）；实测 DB 为 0、返回 PriorMissing=0。三个调用窗口又都在检查 error 或后续 `MarkObserved` 后才 emit，已持久化 N→0 会永久漏报。复用原 query_only 故障入口的 `TestBReviewClearSurvivesScratchFailure` 失败。 | 明确返回主库是否已清零及可靠前值，在后续失败前记录事实。覆盖 noteObserved、确认缺失与删除重核三个窗口、scratch／MarkObserved 失败和来源后续失败；不得用再次读库猜旧值，不把未提交观察发布为进度。 |
| B-04 / P1 | `BuildMessage` 只裁文件预览，不裁 `buildHead` 的来源诊断；无预览时直接返回超长头（message.go:145）。正常 CLI 配置 60 个不可读来源实测发送正文 **4903 UTF-16 units**，超过 3500，且无附件；零文件事件时附件逻辑也完全忽略来源溢出。 | 对整个正文统一预算，来源明细溢出进入同一个有界附件；保留完整统计与省略／附件说明。覆盖多来源失败、长合法相对路径、零事件且来源溢出和单附件完整性。 |
| B-05 / P1 | 文件事件经过 EscapeText，但失败来源诊断在 message.go:197 原样插入。正常 CLI 对已有未归属 `evil\u202E.strm` 报冲突时，正文仍含原始 U+202E，可改变呈现顺序。该文件名通过现有路径校验，已从真实 CLI 复现；不依赖被拒绝的换行文件名。 | 对所有进入正文／附件的动态展示内容统一转义，包含诊断中的合法相对路径、实例名及 Unicode 控制／方向字符；保留固定安全原因，不转发原始错误。通过实际冲突入口核对字节及行结构。 |
| B-06 / P1 | `newCollector` 创建失败返回 nil，`sendRound` 在 notify.go:78 立即退出，后面的 fallback 永不可达。正常 CLI 设置不可用 TMPDIR，实测同步退出 0、生成 1 文件并提交 1 轮，却发送 0 条 all 摘要。与“明细失败只降级，摘要标明不完整”不符。 | collector 缺失时仍按策略发送已有可靠报告摘要，明细／进度未知必须明示；busy-skip 与取消仍静默。用现有 CLI 接收端覆盖 all／error／change_or_error、成功／失败、临时创建／写入失败，并与关闭通知的产物及 DB 对照。 |
| B-07 / P1 | 短轮次不发附件，而 `previewLine` 完全省略 Reason；首次、推进、达阈值都只显示“缺失计数”，没有“文件仍保留／等待后续重核”的说明。因而常见单文件通知无法区分源目标未读到与扩展名排除。`buildRunInfo` 还只保留失败来源的诊断，成功来源的安全保留原因消失。代码及现有 cleanup 场景断言确认该缺口。 | 同一事件在正文和附件都带合同要求的原因与阶段说明，阈值不等于实际删除；清零注明恢复条件，安全保留复用已有来源级原因。不得增加状态、逐文件冻结扫描或额外远端查询。用单文件两种原因的完整 0→1→2→3→删除／恢复闭环核对实际消息，不只匹配数字。 |
| B-08 / P2 | 配置未严格遵循字段合同：disabled 提前返回跳过非法枚举／环境名校验；YAML 数字 instance_name 隐式转字符串。正常 CLI dry-run 对 disabled+非法 policy、enabled+数字实例名均退出 0。另新增了合同没有规定的 64 字符拒绝限制。 | 在不读取凭证、不创建临时文件的前提下校验已提供配置；字符串类型按合同检查，非法配置在动作前退出 2。按现合同处理实例名；若要新增输入限制，先由 A 明确修订。用现有配置表驱动与 CLI dry-run 覆盖。 |

以上均为原合同缺口；P1 阻塞本地 PASS，B-08 是有界的小项，随本 Task 修正，
不另开功能 Slice、不重新激活 Slice 1–5。未发现需要修改原删除资格、协议或远端兼容的授权需求。

**全部验收重评：** AC-1 FIX（B-08）；AC-2 FIX（B-02、04）；AC-3 FIX（B-06、07）；
AC-4 FIX（B-02）；AC-5 FIX（B-07）；AC-6 FIX（B-01、03）；AC-7 FIX（B-04、05）；
AC-8 已有本地 HTTP 错误／取消回归通过，但共享预算及附件失败分支的证据仍须补齐；
AC-9 FIX（B-06，尚缺旁路故障与清理继续对照）；AC-10 PENDING（现有场景仅检查 next 字段存在，
未阻塞发送来验证锁释放、慢发送等待与发送中取消）；AC-11 PENDING（容器摘要／附件场景有结果，
权限及完整失败消息安全尚未覆盖）；AC-12 检查点可追溯，验收映射须按本结论更正。
初始化失败摘要还应保留已知的配置来源总数，不能因 report 为 nil 一并丢失。

| RO | 本轮决定与剩余结果 |
|---|---|
| RO-1 | FIX REQUIRED：默认／旧配置、基本策略与 dry-run 已有本地通过证据，严格配置及降级策略需修正 |
| RO-2 | FIX REQUIRED：基本聚合已实现；发布后失败事实、来源摘要有缺口 |
| RO-3 | FIX REQUIRED：基本预览／附件通过；全正文上限、来源溢出与安全展示未满足 |
| RO-4 | FIX REQUIRED：正常缺失数字闭环通过；清零失败窗口、原因与阶段展示不完整 |
| RO-5 | FIX REQUIRED：无重试、取消、锁外发送结构存在；全源内存、创建失败降级与安全展示有缺口，部分运行断言缺失 |
| RO-6 | PENDING：操作文档需与修正后行为一致；真实 Telegram 文本／附件送达 NOT RUN |
| RO-7 | PENDING：独立 B 本轮已审完整范围但不通过；修复后再审，A 最终验收尚未进行 |

**验证与证据可信度：** 本轮 config／storage／reconciler／notify 聚焦包回归与 tagged CLI
通知用例通过；两个补充合同断言失败，保留原始日志，未修改产品或弱化断言。补充 Go 用例通过
`go test -overlay` 接入现有包测试，替换副本仅在 artifacts；CLI 复用既有 Harness、mock OpenList、
TelegramReceiver 和正常 `sync --config`／dry-run 入口，仅用合成凭证与本仓库隔离夹具。
首次 CLI 试探的 40 来源正文尚未超限，换行名被现有路径校验拒绝，不作为缺陷；保留首次记录，
改用合同允许的 60 来源及合法 bidi 文件名后复现 B-04／05，未猜造产品入口。

Developer 的 sync 14/14、serve 8/8、container 6/6 summary.json 在本机存在且场景状态一致；
只认可对应断言已覆盖的结果，不提升为全部 AC PASS。原始摘要固定二进制 hash，但未固定源码
SHA／完整命令原始日志；质量／race 仅有报告，未找到对应原始日志，不能独立复核为本轮 PASS。
现有 TestSendRoundCleansUpAfterDeliveryFailure 使用真实官方地址并假定其不可达，应改为本地失败
transport；本轮未运行该外网用例。所有验证只需在既有测试／驱动补必要断言，不建设新平台。

已确认阻塞项，本轮不重跑完整质量、race、sync／serve／容器长套件。修复后仍按原 T4 门槛
执行最终候选质量／race、受影响完整集成与适用打包恢复验证，并固定准确输入及原始产物；
无关真实 OpenList／Jellyfin／插件、固定万媒体、ARM64 原生及真实 Telegram 均未运行。
本轮测试为秒级聚焦执行，测试耗时低于本轮总耗时 50%；未以长测试替代代码／合同复核。

交接：Developer 在 **Task 6.1** 原范围修复 B-01..08、补上述缺失断言并提交新 Implementation Head，
保留首次失败及旧检查点；B 再审完整 Task Base..新 Head 并重评所有 RO。
真实 Telegram 仍需用户另行明确授权的测试 Bot／聊天；本轮不请求秘密、不预签下一 Task。

### Developer B-R 修正轮报告（2026-11-12，Head `7c54baf`）

在 Task 6.1 原范围内修正独立 B 复审全部三项（B-R1..B-R3），未扩大业务范围、未改
删除资格、协议或远端兼容语义；旧检查点 `9dbb58c` 保留。逐项修正与测试证据见
[artifacts/task-6.1/EVIDENCE.md](artifacts/task-6.1/EVIDENCE.md) B-R 修正轮小节及
[artifacts/validation-evidence/task6.1-br-fix-evidence/metadata.json](artifacts/validation-evidence/task6.1-br-fix-evidence/metadata.json)
（输入 SHA、二进制摘要、命令、退出码、原始日志，含两次驱动侧首失败）。

- **B-R1（P1）**：`factSinkAdapter.Record` 将缺失推进路由到
  `Collector.RecordProvisional`，真实事务失败后的 `DiscardStaged` 现在真正撤销
  未提交候选；`DiscardProvisional` 在清单文件无法回退（写入失败叠加回滚）时仍撤销
  total／preview／anyChange／limitHit 的未提交事实，只保留写失败标记与已发生事实。
  验收用例 `TestMissingAdvancesFollowRealCommitFacts` 走生产链路：真实 Store、
  真实 syncer、生产 adapter、真实 collector；取消 context 使真实事务在
  `StageAdvance` 之后失败，核对 DB 计数不变、预览与正文无 1→2 候选、已提交的
  0→1 及其附件行保留。
- **B-R2（P1）**：`buildHead` 对转义后的实例名实施 Unicode 安全预算，超长时截断
  并附省略号；标题＋尾部受控，所有成功／失败／初始化失败消息均不超过 3500，
  未重新引入合同外的配置长度拒绝。CLI 真实入口覆盖合法家×5000 与合法
  U+202E×800（转义膨胀）两种入口，正文 ≤3500、统计保留、无原始控制符。
- **B-R3（P1）**：`BuildMessage` 在追加溢出来源明细并写入尾部之后才固定尾部措辞
  与附件决策；追加写失败、尾部写失败与 8 MiB 截断均使正文如实标注“明细不完整”，
  不再声称“完整内容见附件清单”，损坏清单不上传；`sendRound` 不再在 BuildMessage
  后忽略 Finalize 结果。复用单个清单文件，不重复追加来源明细。
- **验证（最终候选 `7c54baf`）**：`python3 scripts/quality.py` PASS；race
  （notify／cmd／storage／reconciler 四包）PASS；tagged CLI PASS；sync 集成
  14/14、serve 集成 8/8、容器集成 6/6（含 notifications 权限断言）PASS；Compose
  `--no-interpolate` 无值透传通过。首次失败保留：容器驱动首次调用缺
  `--hooks-binary`（驱动调用错误，非产品缺陷）；CLI 扩展场景首版把原始控制字符
  直接写入 YAML 双引号标量导致退出 2（配置语法错误入口，非产品缺陷），改用合法
  U+202E 入口后验收；控制字符实例名被产品校验拒绝，不是合法入口，不作为场景。
- **剩余限制与风险**：真实 Telegram 送达 NOT RUN（无授权测试目标／凭证）；真实
  OpenList／Jellyfin／插件链路、ARM64 原生实机与容器内 8 MiB 级数据集不在本轮。

### 独立 B 复审（2026-11-12，修正轮）

**Decision: FIX REQUIRED。** 原有八项中，多项修正成立；仍有三个阻塞结果。
本次只修改审查／交接文档，未改产品代码、未代行 Developer 自签通过、未 push。

- 完整审查范围为固定 Task Base `1bfac55136dc4d3137d9ee9d470ec5c6ba840749`
  至新实现 `9dbb58c20be182edbdc64637e1c9ee9a79c901d7`；结合首次审查重新核对完整增量，
  重点复核新修正。另核对 Slice Base 以来合同／规划及截至
  `1e7a3813af6ab93e7339aeb24480064223d509fb` 的交付报告增量，后者只改 TASK 和证据索引。
  原实现 `606e310`、首次 B 审查 `20e5325` 和固定 Base 均保留。
- 新证据在本机 `artifacts/validation-evidence/task6.1-b-rereview-20261112/`，
  入口为 `metadata.json`、`failure-windows-confirmed.log`、`cli-results.json`。
  Go 探测通过既有测试入口及 overlay 加载；复用现有 backing-file 写失败注入方式，
  事务探测使用实际 reconciler、SQLite Store 与 CLI fact adapter，模拟提交写失败。
  CLI 探测复用现有 Harness／本地 Telegram 接收端，不访问真实外部服务。
  首次探测中的测试夹具字段笔误及日志保留，修正夹具后才判定产品结果。

| ID／级别 | 已验证的剩余问题 | 原 Task 内修正与验收 |
|---|---|---|
| B-R1 / P1（B-01 未闭合） | `cmd/strmgen/notify.go:25` 仍将所有事实送入 `Collector.Record`；缺失推进没有走 `RecordProvisional`，所以真实事务失败后的 `DiscardStaged` 无效。实测提交 UPDATE 失败后主库 missing_count=0，但通知仍有 A.strm 的 0→1 及 change 标记。另 `collector.go:162` 在写失败／Truncate／Seek 失败时提前返回，不恢复内存预览、总数和 change；直接补上接线仍会在明细写失败叠加回滚时虚报。 | 将缺失推进接入实际暂存协议；回滚无论文件能否回退，都撤销未提交的逻辑事实，保留此前已发生的文件操作／清零事实和写失败标记。通过生产 adapter + 实际 Store 的失败／成功／commit 后 scratch 失败用例核对 DB、正文和附件，覆盖写失败叠加回滚；保持有界处理，不增加业务状态或全量切片。 |
| B-R2 / P1（B-04 未闭合） | `internal/notify/message.go:196,231` 只移除来源／事件，固定 head 超长时直接退出裁剪。取消实例名 64 字符限制后，合法单行实例名 `家`×5000 从正常 CLI 生成 **5219 UTF-16 units** 的正文；违反 3500 上限，并超过正常 Telegram 文本容量。 | 在展示层对实例名／标题实施 Unicode 安全预算，保留必要统计与省略说明，所有成功／失败／初始化失败消息均不超过 3500；不重新添加合同外的配置拒绝限制。用合法长实例名和转义后膨胀的实例名从正常 CLI 验收。 |
| B-R3 / P1（B-06 的写失败路径未闭合） | `message.go:160,205` 在追加溢出来源明细前就生成 tail；追加写失败后只取消附件，正文未更新，仍声称“完整内容见附件清单”，且无“明细不完整”。复用既有写失败注入实测 `WriteFailed=true`、附件为空、正文不完整标记=false。`sendRound` 又在 BuildMessage 后 Finalize 且忽略最终写失败，因此尾部写失败也不会修正已定正文／附件决定（代码审查）。 | 来源明细追加和最终尾部写入完成后，再确定降级说明与可用附件；复用单个文件，不重复追加来源明细。覆盖来源追加、8 MiB 截断和 Finalize 失败，正文与本地诊断如实标记不完整，不能承诺不可用的完整附件；同步产物、计数、退出码不受影响。 |

**已核实的修正：** B-01 的全源路径切片已去除，但事务事实完整性见 B-R1；
B-02 的实际发布／替换结果接口与 B-03 三个清零窗口修正成立，相关回归通过。
B-04 的 60 个失败来源场景已降至 **3295 UTF-16 units** 并上传一个附件，但长标题见 B-R2。
B-05 原正常 CLI bidi 冲突场景不再泄露原始方向控制符。
B-06 不可用 TMPDIR 场景现在同步退出 0、生成 1 文件、提交 1 轮且发送 1 条降级摘要，
后续写失败仍见 B-R3。B-07 的原因／阶段及安全保留展示、B-08 的 disabled 校验／字符串类型
修正成立；两个非法配置的 CLI dry-run 均退出 2。上述确认不构成整个 Task PASS。

**验证与证据边界：** 本轮独立执行受影响四包测试及 tagged CLI 测试，均 PASS；
三个新增失败断言均稳定 FAIL，另有长实例名 CLI 超限实测。构建来自干净报告 Head，
二进制 SHA-256 为 `01933abebbe91a8aa6c6debe59bfdb417fccde383c83bfc6aade6af892b30bcb`。
已查阅 Developer 的 race、sync 14/14、serve 8/8、container 6/6 和安全 Compose 原始结果，
并审阅新增慢发送／权限断言；本轮不重复跑这些全套。其二进制记录为 `20e5325+dirty`，
不将该历史全套标签表述为本轮独立重现的精确 Head 全套结果。
测试执行时间低于本轮复核时间的 50%；未新建过程工具平台。

**全部 AC 重评：** AC-1 本地通过；AC-2 FIX（B-R2）；AC-3 正常策略回归通过；
AC-4 实际文件操作事实回归通过；AC-5 正常清理／恢复闭环通过；AC-6 FIX（B-R1）；
AC-7 FIX（B-R2、B-R3）；AC-8 本地传输测试通过；AC-9 FIX（B-R3）；
AC-10／11 新增代码与原始证据支持慢发送隔离和私有临时权限，未发现本轮新增阻塞；
AC-12 本次已更新真实范围、失败证据及交接，不把 Developer 汇总通过当作产品通过。

| RO | 本次重评与剩余结果 |
|---|---|
| RO-1 | 本地配置／默认／策略／sync 与 serve 接入通过；不据此验收其他 RO |
| RO-2 | FIX REQUIRED：回滚仍虚报进度，超长标题破坏可交付摘要（B-R1、B-R2） |
| RO-3 | FIX REQUIRED：完整正文边界及来源附件完整性说明仍有缺口（B-R2、B-R3） |
| RO-4 | FIX REQUIRED：正常计数、清零、删除展示已修正；未提交推进仍可被报告（B-R1） |
| RO-5 | FIX REQUIRED：全源切片已去除，锁外发送／取消及权限证据已补；写失败降级仍不可靠（B-R3） |
| RO-6 | PENDING：真实 Telegram 文本与附件送达 NOT RUN；说明与最终修正结果仍需同步 |
| RO-7 | PENDING：本次独立完整范围复审完成但未通过；后续修正复审与 A 最终验收未完成 |

交接：只在 **Task 6.1** 修复 B-R1..B-R3，保留已验证的改动；先跑上述失败用例和直接影响检查，
最终候选按原 T4 计划完成对应回归。不得以孤立 Collector 测试替代真实 adapter 接线验证。
不扩展过程工具、不新开功能 Slice。真实 Telegram、真实 OpenList／Jellyfin／插件链路、
ARM64 原生实机及容器内 8 MiB 极端数据集本轮均 NOT RUN；Slice 1–5 保持 PASS / CLOSED。

### 独立 B 最终复审（2026-11-12）

**Decision: PASS（Task 6.1 本地范围）。** B-R1..B-R3 阻塞项已修正；未发现剩余 P0／P1。
保留一项非阻塞 P2，真实 Telegram 送达仍为 NOT RUN，不能据此关闭 Slice 6。
本轮仅维护审查／状态文档，没有修改产品代码、启动多代理、push 或发送真实消息。

审查完整固定范围 `1bfac55136dc4d3137d9ee9d470ec5c6ba840749..7c54baf660150ced9ac87e41d34dc4631f923fa1`，
结合前两轮审查重核配置、CLI 生命周期、事实捕获、storage／文件系统结果接口、通知格式／传输、
测试与部署增量；本次六个代码／测试文件的修正未改业务删除资格、远端规则或持久表结构。
另核对 Slice Base `20677f1906837428324d58885e719ed385a07258` 以来合同／规划和截至
`fe4512e` 的报告增量；报告 Head 与实现 Head 产品代码相同。所有原检查点及失败记录保留。

| 修正项 | 本次独立核对结果 |
|---|---|
| B-R1 | 重跑原失败探测：真实 SQLite UPDATE 失败后 missing_count=0、通知事件=0；明细文件写失败叠加回滚时也无未提交预览／change 标记。生产 adapter 现接入 RecordProvisional。新增真实 Store／syncer／adapter 回归覆盖成功提交、暂存后事务失败、正文和附件；原 commit 后 scratch 失败与三个清零窗口回归仍通过。 |
| B-R2 | 正常 CLI 的 `家`×5000 实例名正文为 **3310 UTF-16 units**（原为 5219）；超长和转义膨胀实例名回归均通过，配置仍允许合法长名字，统计保留。 |
| B-R3 | 原来源追加失败探测现为 WriteFailed=true、附件为空且正文有不完整标记；新增尾部写入失败用例通过。明细追加／Finalize 在尾注及附件决策前完成，不重复写尾部，大小截断继续明确标为不完整。 |

独立证据位于本机 `artifacts/validation-evidence/task6.1-b-final-20261112/`：
`metadata.json` 记录输入、命令、二进制摘要、复用与未运行项；`prior-failures.log` 为原三个失败用例
3/3 PASS，`cli-results.json` 为正常 CLI 探测结果，`quality.log`／`race-all.log`、
`sync.log`／`sync-notifications.log`／`serve.log` 为原始运行结果。
从干净报告 Head 新构建 testhooks 二进制，SHA-256 为
`dbc88967609141d24833b3e59f267bc58c5b7604487d8d5da43b1237fa46468a`。

- 本轮独立执行：`python3 scripts/quality.py`（含 Python 112 项、vet、Go test／build 和 tagged CLI）、
  全包 `-race -count=1 ./...`、原三个失败探测、正常 CLI 配置／多来源／bidi／TMPDIR／长实例名，均 PASS。
- 用新二进制执行既有 notifications、notifications_cleanup 两个场景及 serve 全部 8 场景，均 PASS；
  涵盖策略、附件、缺失 0→1→2→3→删除／恢复、锁外慢发送、不重入、发送中取消与临时清理。
  第一次 sync 调用重复传 `--scenario`，实际只执行最后的 cleanup；随后单独执行 notifications，
  两份结果分开保留，没有将首次调用算成两个场景。
- 查阅并复用 Developer B-R 修正轮完整 sync 14/14、容器 6/6、Compose 安全渲染原始证据，
  其预提交输入与二进制摘要限制如实记录；不称为本轮独立重跑的全套集成。
  容器权限、只读根、附件与清理断言沿用已有驱动，无产品部署变化。
- 测试执行为几十秒，复核及文档整理为数分钟，低于本轮时间 50%；没有新增过程平台。

**非阻塞 P2：** `sendRound` 对 collector 写入／Finalize 失败尚无专用本地警告（代码审查）；
消息已正确标注不完整并禁用损坏附件，已有同步结果不受影响，网络发送失败仍有脱敏本地警告。
后续可补固定措辞警告与本地断言；此诊断便利性缺口不延长本地 Task，不记为已修复或用户已接受，
交 A 连同本记录评估。

**全部 AC／RO 重评：** AC-1..8 本地通过；AC-9 核心隔离／降级／清理通过，保留上述 P2；
AC-10 由独立 serve 8/8 支持；AC-11 配置／脱敏与容器证据通过；AC-12 范围和证据交接齐全。

| RO | 最终 B 本地结论及剩余项 |
|---|---|
| RO-1 | 本地 PASS：缺省关闭、严格校验、凭证／dry-run、三策略和双入口 |
| RO-2 | 本地 PASS：多来源结果、原统计、失败时实际文件／持久计数事实 |
| RO-3 | 本地 PASS：有界正文／预览、单个 8 MiB 清单、路径转义及完整性说明 |
| RO-4 | 本地 PASS：提交后推进、独立清零、阈值重核／删除；回滚不再虚报 |
| RO-5 | 核心本地 PASS；非阻塞 P2 如上。通知隔离、锁外发送、取消、边界／脱敏均有证据 |
| RO-6 | 本地文档／部署证据 PASS；真实 Telegram 文本／附件送达 NOT RUN，整体 PENDING |
| RO-7 | Task B 审查 PASS；Slice A 完整范围最终验收 PENDING |

Task 6.1 本地交付完成。Slice 6 恢复 ACTIVE，等待另行授权的真实 Telegram 取证及 A 最终审查，
不预签下一 Task、不索取或恢复历史秘密。真实 OpenList／Jellyfin／插件、固定万媒体、
ARM64 原生实机及容器内 8 MiB 极端数据集本轮未运行；既有 Slice 1–5 结论不变。

### P2 日志补充修复（2026-11-12）

用户要求先补齐上述 P2。本次按独立小修复实施，范围仅为 `sendRound` 的本地诊断，
不改通知策略、同步／清理事实、数据库、退出码或发送协议；沿用 Task Base 和原合同。
修复前基线 `521c76c84978ac94440331fbf2156ec58b4690bd`，实现 Head
`757b082855974c43498ff7e3f5896b31d657a4ea`。此前完整范围的 B PASS 保留，
本次由当前会话实施，**新增修复独立复核 PENDING**，不自签新的 B／A PASS。

验收范围：明细收集写入或生成摘要时 Finalize 写入失败均只记录一条固定脱敏警告；
正常情况不误报；发送策略跳过摘要时仍报告本地故障；继续使用原有不完整说明与临时清理。
实现把诊断放在现有释放清单的 defer 中，读取现有 `WriteFailed()` 状态，覆盖摘要生成后的失败，
不输出底层错误、临时路径、文件名、Token 或 Chat ID。

测试按该诊断小修复采用 T2 聚焦验证，原 Task T4 历史证据不重写。本轮执行：

- 新增正常、收集写失败、Finalize 写失败、策略跳过四个子用例，均通过。
  用真实文件大小限制触发写失败，走现有 collector／sendRound 与本地 HTTP 接收入口；
  核对固定警告、降级摘要、原发送策略，不新增生产注入接口。
- `python3 scripts/quality.py` 通过，含完整既有 Go／Python 回归、vet、构建与 tagged CLI。
- 受影响 `cmd/strmgen`、`internal/notify` 两包 race 最终通过（`-p 1 -count=1`）。
  首次并行运行时，原清理测试的全局临时目录扫描报告新增临时目录；
  两包退出后无残留，与跨包共享临时目录的扫描竞争一致，随后串行复跑通过。
  保留首次日志，不改旧断言或产品清理逻辑。

证据：`artifacts/validation-evidence/task6.1-p2-warning-20261112/` 的 `metadata.json`、
`focused.log`、`quality.log`、`race.log`、`race-serial.log`。测试时间低于本轮 50%。
真实 Telegram、容器与真实 OpenList／Jellyfin 未运行；本修复不改变此前真实送达 NOT RUN 和
A 终审 PENDING。下一步独立复核完整 Task Base..新 Head，重点核对该 P2 增量及历史结论。

### P2 自复核（2026-11-12，用户指定）

**Decision: PASS（自复核），P2 关闭。** 用户明确要求“你自己复核下吧，改动不多”，
本次小修复按该指令由实施者自复核，不标作新的独立 B／A PASS，不改变其他工作的独立审查要求。
审查完整增量 `521c76c84978ac94440331fbf2156ec58b4690bd..17733f373812656b58471f377a448e366a73dc80`，
涵盖两个代码／测试文件及四份状态文档；此前 Task Base..`7c54baf` 的独立审查结论保留。

核对结果：defer 在 BuildMessage／Finalize 之后检查现有失败状态，能够报告收集及收尾写失败；
固定日志不含路径、凭证或原始错误；每轮只输出一次，策略跳过仍有本地警告；
原 collector.Close 仍在所有既有返回路径执行。无新增发送、业务写入、状态或并发流程。
测试的文件大小限制会恢复，未并行运行；本轮未发现阻塞项或新 P2。

已核验上一轮测试记录中的源码 SHA-256 与当前代码完全一致，复用四个针对性用例、
完整质量检查、受影响包串行 race 的通过证据及首次并行失败记录；未机械重跑产品测试。
本轮仅更新结论和治理检查，不新增产品改动。Task 6.1 本地范围恢复 PASS；
真实 Telegram 送达仍 NOT RUN，Slice 6 保持 ACTIVE，A 终审 PENDING。

历史 Task 5.1 已独立 B PASS；原计划、报告与审查从
[固定 Git 检查点](docs/development-workflow.md#历史记录)中的
`85a31337bb1a36bba9198fd8ff77959fc8d151cd:TASK.md` 查看，不复制归档。
