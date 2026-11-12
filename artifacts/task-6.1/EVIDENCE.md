# Task 6.1 本地验证证据索引

- Base: `1bfac55136dc4d3137d9ee9d470ec5c6ba840749`（Task Base == Contract Checkpoint）
- 实现内容: Telegram 通知本地完整链路（`internal/notify`、`cmd/strmgen` 装配、
  `reconciler`/`storage` 结构化事实接口、配置、文档与示例、测试驱动场景）
- 二进制: `artifacts/task-6.1/bin/strmgen`（release）、`artifacts/task-6.1/bin/strmgen-testhooks`
  （`go build -tags strmgen_testhooks -trimpath`，仅用于隔离测试）
- 全部凭证与 token 均为合成值（`SYNTHETIC_CONTAINER_TOKEN_MARKER_20261019`、
  `integration-token`、`container-token`、`strmgen-fixture-only`），无真实秘密。

## 命令与最终结果（最终候选）

| 检查 | 命令 | 结果 |
|---|---|---|
| 治理 | `python3 scripts/check_governance.py` | PASS |
| 常规质量（含 tagged CLI 通知测试） | `python3 scripts/quality.py` | PASS |
| race（全部包） | `CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 ./...` | PASS |
| race（tagged CLI 通知测试） | 同上加 `-tags strmgen_testhooks ./cmd/strmgen` | PASS |
| sync 集成（全部场景，含 notifications／notifications_cleanup） | `python3 scripts/run_isolated_sync_integration.py --binary artifacts/task-6.1/bin/strmgen --fault-binary artifacts/task-6.1/bin/strmgen-testhooks --scenario all --output-dir artifacts/validation-evidence/task6.1-sync-all-bHeDTz` | 14/14 PASS |
| serve 集成（全部场景，含 serve_notifications） | `python3 scripts/run_serve_integration.py --binary artifacts/task-6.1/bin/strmgen --fault-binary artifacts/task-6.1/bin/strmgen-testhooks --output-dir artifacts/validation-evidence/task6.1-serve-final-mJ7f6k` | 8/8 PASS |
| 容器集成（全部场景，含 notifications） | `python3 scripts/run_container_integration.py --image strmgen:task6.1 --hooks-binary artifacts/task-6.1/bin/strmgen-testhooks --scenario all --output-dir artifacts/validation-evidence/task6.1-container-g5kiX2` | 6/6 PASS |
| Compose 脱敏渲染 | `docker compose -f deploy/compose.yaml config --no-interpolate` | 通过；无值透传 `OPENLIST_TOKEN:`／`TELEGRAM_BOT_TOKEN:`／`TELEGRAM_CHAT_ID:` |

镜像 `strmgen:task6.1` 由现有 `Dockerfile` 本地构建（amd64）。testhooks 二进制以
只读 bind-mount 覆盖入口点进入容器，镜像本身不含测试钩子。

## AC → 证据映射

| AC | 证据 |
|---|---|
| AC-1 配置/默认/凭证缺失/dry-run | `internal/config/config_test.go`（notifications 解析与拒绝）、`internal/notify/*_test.go`、`cmd/strmgen/notify_cli_test.go`（dry-run 静默、缺凭证警告退出码 0）、驱动场景 `notifications`（dry-run 静默、缺凭证） |
| AC-2 单条多来源摘要与分类 | `cmd/strmgen/notify.go`、`notify_cli_test.go`、驱动 `notifications`（基线对照、初始化失败不可用计数、失败分类）、`internal/notify/notify_test.go` |
| AC-3 三种策略 | 驱动 `notifications`（error 成功静默／失败发送；change_or_error 未变化静默／变化发送）、`internal/notify/notify_test.go TestShouldSendPolicy` |
| AC-4 逐文件事实与多来源 | `internal/reconciler/facts_test.go`、驱动 `notifications`（30 文件附件逐条核对）、CLI 测试附件场景 |
| AC-5 计数 0→1→2→3、阈值删除、恢复清零 | 驱动 `notifications_cleanup`（含 SQLite 对照）、CLI `TestSyncNotificationReportsMissingProgressAndDeletion` |
| AC-6 失败/取消不虚报、提交事实接口 | `internal/storage/storage_round_test.go`（advances、query_only 注入的 committed+scratch 失败、ClearResult）、`internal/reconciler/facts_test.go`（提交成功但 workspace 失败仍报告已提交事实与推进）、busy-skip／取消不发送（驱动 serve_notifications Phase A、`TestSendRoundCleansTemporaryArtifacts`） |
| AC-7 中文/emoji/控制符/双向符、预览与附件上限 | `internal/notify/notify_test.go`（转义、UTF-16 截断、8 MiB 上限流式、预览 20 条、清单不完整标注）、CLI 附件测试、驱动附件核对 |
| AC-8 HTTP/JSON/429/64KiB/重定向/超时取消 | `internal/notify/sender_test.go` |
| AC-9 开关对照与旁路隔离 | 驱动 `notifications`（STRM 字节、SQLite 行、轮次与退出码对照基线）、`TestSendRoundCleansTemporaryArtifacts`（busy/取消/失败清理临时文件） |
| AC-10 serve 不重入/积压 | `scripts/run_serve_integration.py scenario_serve_notifications`（busy-skip 静默、逐 tick 单条摘要、失败后恢复、SIGTERM 非零退出） |
| AC-11 示例/Compose/安全渲染/私有 tmp | `deploy/compose.yaml` 无值透传、`--no-interpolate` 渲染、容器场景 `notifications`（只读根 + 私有 /tmp、脱敏断言） |
| AC-12 完成报告 | 根 [TASK](../../TASK.md)（实现 Head、剩余 NOT RUN 项） |

## 首次失败与修复记录

1. 集成驱动发现 busy-skip／取消路径的临时目录残留（`defer collector.Close()` 注册
   在提前 return 之后）→ 修正 `sendRound`，并新增 `TestSendRoundCleansTemporaryArtifacts`。
2. 集成驱动发现容器场景走 `127.0.0.1` 无法从容器回连 → 容器场景改用
   `host.docker.internal`（产品代码无改动）。
3. 驱动初版断言 mtime/绝对路径跨根对照过严 → 改为字节与业务行对照（产品无改动）。

## NOT RUN / 未覆盖

- 真实 Telegram 文本／附件送达：无授权测试 Bot／凭证，NOT RUN。
- ARM64 原生实机运行：不在本 Task 范围（镜像 arm64 OCI 由既有构建入口另行提供）。
- 真实 OpenList／Jellyfin／插件链路：本 Task 未涉及，历史结论不变。
- 附件 8 MiB 上限在容器内的极端数据集（8 MiB 事件）未在容器层复现；
  上限行为由单元测试与二进制层驱动覆盖。

---

## 修正轮（B-01..B-08，2026-11-12）

独立 B 审查后在本 Task 范围内修正全部八项，并补齐此前缺失的断言。
命令、输入 SHA、二进制摘要与原始日志见
[artifacts/validation-evidence/task6.1-fix-20261112/metadata.json](../validation-evidence/task6.1-fix-20261112/metadata.json)。

| 项 | 修正与证据 |
|---|---|
| B-01 | `Store.CommitRound` 改为可选 `AdvanceSink` 暂存协议：推进记录在原事务窗口内有界批次捕获，提交成功后才 `CommitAdvances` 发布，事务失败 `DiscardAdvances` 丢弃；无 sink 时不构造任何明细。测试：`TestCommitRoundDiscardsAdvancesWhenTransactionFails`、`TestCommitRoundWithoutSinkKeepsCountSemantics`、`TestCommitRoundReportsCommittedAdvances`（sink 版）及 scratch 失败窗口；race 全部包 PASS |
| B-02 | `LocalFS.Publish`／`Update` 返回 `PublishOutcome.Published`／`UpdateOutcome.Replaced`，表达链接／交换完成后的实际字节事实；后续持久化失败仍计入并保留事实。测试：`TestUpdatePublishedBeforeFailureKeepsFactAndBytes`、`TestPublishLinkedBeforeDirectoryFailureKeepsFact`（复用既有 LocalFS 注入点与恢复收敛） |
| B-03 | `ClearMissing` 主库清零成功而 scratch 清理失败时返回持久事实 + 错误；三个清零窗口（noteObserved、confirmMissing、deleteCandidate 重核）均在检查错误前发布 N→0 事实。测试：`TestClearMissingReportsDurableClearAfterScratchFailure`、`TestClearFactSurvivesScratchFailureInEveryWindow` |
| B-04 | `BuildMessage` 整个正文（含来源明细）共享 3500 UTF-16 预算；来源明细溢出进入同一 8 MiB 附件；初始化失败保留配置来源总数。测试：`TestBuildMessageBoundsWholeBodyWithSourceOverflow`、`TestBuildMessageSourceOverflowWithoutEventsRequiresAttachment`、`TestBuildMessageInitFailureKeepsConfiguredSourceCount` |
| B-05 | 来源诊断、实例名等全部动态展示内容统一 `EscapeText`（含 U+202E）；预览与附件一致。测试：`TestBuildMessageEscapesNotesAndInstanceName`、既有事件路径转义 |
| B-06 | 清单创建失败不再静默：`sendRound` 在无 collector 时按策略发送降级摘要，正文标注“明细不完整、进度未知”，change_or_error 回退报告计数、不猜测缺失进度。测试：`TestSendRoundWithoutCollectorStillSendsDegradedSummary`（本地 transport）、CLI `TestSyncNotificationDegradedWhenCollectionFails`（不可用 TMPDIR 入口） |
| B-07 | 正文预览与附件逐事件携带原因与阶段（首次进入待删除、计数推进、达到阈值等待重核、已确认恢复条件清零、已实际删除、归属回收说明）；成功来源的安全保留诊断进入来源区。测试：`TestPreviewLinesCarryReasonAndStage`、CLI `TestSyncNotificationReportsMissingProgressAndDeletion`、驱动 `notifications_cleanup`（0→1→2→3→删除／恢复全闭环文字断言） |
| B-08 | `resolveNotifications` 在 enabled: false 下仍校验已提供字段；`instance_name` 拒绝非字符串标量（自定义 `UnmarshalYAML`，并手工维护未知字段拒绝）；移除合同外 64 字符上限。测试：`TestNotificationsValidateProvidedFieldsWhenDisabled`、`TestNotificationsAcceptsLongInstanceName`、CLI dry-run 拒绝路径 |

补齐的证据缺口：`TestSendRoundCleansUpAfterDeliveryFailure` 改用本地失败 transport
（不再依赖真实官方地址不可达）；AC-8 附件失败分支 `TestSendAttachmentFailureAfterSuccessfulSummary`；
AC-10 serve 驱动 Phase F（接收端 hold：阻塞发送期间并行 sync 完成证明锁已释放、
不重入不积压、SIGTERM 发送中及时取消）；AC-11 容器 notifications 场景在发送阻塞窗口
实测临时目录 0700／清单 0600。

最终候选结果：quality PASS；race 全部包与 tagged CLI PASS；sync 集成 14/14、
serve 8/8（含 Phase F）、容器 6/6（含权限断言）PASS；Compose `--no-interpolate`
无值透传通过。首次失败日志保留（metadata.json first_failures_preserved）。

仍然 NOT RUN：真实 Telegram 文本／附件送达（无授权测试目标）；真实 OpenList／
Jellyfin／插件链路；ARM64 原生实机；容器内 8 MiB 级数据集附件。

---

## B 复审修正轮（B-R1..B-R3，2026-11-12）

独立 B 复审判定三项剩余阻塞后，在本 Task 范围内修正。命令、输入 SHA、二进制摘要、
原始日志与首次失败记录见
[artifacts/validation-evidence/task6.1-br-fix-evidence/metadata.json](../validation-evidence/task6.1-br-fix-evidence/metadata.json)。

| 项 | 修正与证据 |
|---|---|
| B-R1 | `factSinkAdapter.Record` 将 `FactMissingAdvanced` 路由到 `Collector.RecordProvisional`，使真实事务失败后的 `DiscardStaged` 真正生效；`DiscardProvisional` 在清单文件无法回退（写入失败叠加回滚）时仍撤销未提交的逻辑事实（total／preview／anyChange／limitHit），只保留写失败标记与此前已发生事实；`Finalize` 增加一次性守卫避免双尾部。测试：`TestMissingAdvancesFollowRealCommitFacts`（真实 Store + 真实 syncer + 生产 adapter + 真实 collector：取消 ctx 使真实事务在 StageAdvance 后失败，核对 DB 计数、预览、附件与正文）、`TestCollectorDiscardAfterWriteFailureWithdrawsFacts` |
| B-R2 | `buildHead` 对转义后的实例名实施 Unicode 安全预算（`headMaxUTF16Units`，截断后附省略号），标题＋尾部始终受控，不再重新引入配置长度拒绝。测试：`TestBuildMessageLongInstanceNameStaysBounded`（合法长名与 U+202E 转义膨胀）、CLI `TestSyncNotificationLongInstanceNameStaysBounded`（真实 sync 入口，家×5000 与 U+202E×800，正文 ≤3500、统计保留、无原始控制符） |
| B-R3 | `BuildMessage` 在追加溢出来源明细并写入尾部之后才固定尾部措辞与附件决策：追加写失败／尾部写失败／8 MiB 截断时正文如实标注“明细不完整”，不再声称“完整内容见附件清单”，损坏清单不上传；`sendRound` 不再在 BuildMessage 后忽略 Finalize 结果。测试：`TestBuildMessageSourceDetailWriteFailureMarksBodyIncomplete`、`TestBuildMessageTrailerWriteFailureMarksBodyIncomplete` |

最终候选结果（`python3 scripts/quality.py` PASS；race 受影响四包 PASS；
tagged CLI PASS）：sync 集成 14/14、serve 集成 8/8、容器集成 6/6（含
notifications 权限断言）PASS；Compose `--no-interpolate` 无值透传通过。
首次失败保留：容器驱动首次调用缺 `--hooks-binary` 参数（驱动侧调用错误，非产品缺陷），
原始目录 `task6.1-br-fix-container-cMxclq`；CLI 扩展场景首版把原始控制字符直接写入
YAML 双引号标量导致退出 2（配置语法错误入口，非产品缺陷），已改用合法 U+202E 入口。

仍然 NOT RUN：真实 Telegram 文本／附件送达（无授权测试目标／凭证）；真实
OpenList／Jellyfin／插件链路；ARM64 原生实机；容器内 8 MiB 级数据集附件。
