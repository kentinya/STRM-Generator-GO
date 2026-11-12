# Task 6.1 — Telegram 通知的本地完整链路

```text
Task ID: 6.1
Parent Slice: 6
Status: PLANNED
Task Base: 1bfac55136dc4d3137d9ee9d470ec5c6ba840749
Contract Checkpoint: 1bfac55136dc4d3137d9ee9d470ec5c6ba840749
Difficulty: High
Test Level: T4
Implementation Head: NOT SET
```

2026-11-12 用户授权本会话代替 B 规划首个 Task。本次只提交计划，不开始产品实施，
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
Implementation Status: NOT STARTED
Reviewed Range: NOT SET
Local Evidence: NOT RUN
Real Telegram Evidence: NOT RUN — no authorized test target or credentials
B Product Review: PENDING
A Final Review: PENDING
Remaining Work: implementation; local validation; independent B review; real Telegram evidence; A closure
```

实施后在本节填入实际实现 Head、变更摘要、AC／RO 证据、首次失败／修复和剩余限制，
再交独立 B 审查完整 Task Base..Head；实施者不得自签 B PASS。
本次仅代行 B 规划，不构成独立产品审查。
规划文档按 T0 核对合同一致性、检查点可达性、链接／锚点、补丁空白和提交后治理；
本节 NOT RUN 指产品实施证据，规划检查不代替产品测试。

历史 Task 5.1 已独立 B PASS；原计划、报告与审查从
[固定 Git 检查点](docs/development-workflow.md#历史记录)中的
`85a31337bb1a36bba9198fd8ff77959fc8d151cd:TASK.md` 查看，不复制归档。
