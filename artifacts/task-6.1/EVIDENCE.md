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
