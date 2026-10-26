# Task 5.1 — 自定义扩展名与移除后的正常清理

```text
Task ID: 5.1
Parent Slice: 5
Owner: B
Status: PLANNED
Task Base: 1db2cb7b6a7ac360af3fae1ea8e2f90db0ef81e0
Contract Checkpoint: 1db2cb7b6a7ac360af3fae1ea8e2f90db0ef81e0
Implementation Head: NOT SET
Difficulty: Medium
Test Level: T4
```

用户于 2026-10-26 授权代行 B 规划首个 Task，并指定 kentinya 身份提交。本计划基于实际
代码制定，完整覆盖 [Slice 5](SLICE.md) 的 RO-1..5；不拆成“先加配置、以后补清理”的半成品任务。
Slice 已提交为 ACTIVE；本 Task 为正式 PLANNED，Developer 尚未开始。

## 提交边界与交接

产品代码基线为 Slice Base `846d0920feba607c33e935da4fac6f0c4cb68a24`。
A 已先提交 ACTIVE 合同及路线图，提交 SHA 为 `1db2cb7b6a7ac360af3fae1ea8e2f90db0ef81e0`；
该提交未改产品代码，本 Task 的 Task Base 与 Contract Checkpoint 均固定为该真实 SHA。
B 随后单独提交本计划，确保 Developer 实施前已有合同和正式 Task。

Git 作者与提交者统一为用户指定的 kentinya，邮箱沿用最近提交的 GitHub noreply 地址。
Developer 从本计划提交后开始，将状态改为 IN PROGRESS 再实施；本轮仅提交规划。
本轮代行 B 规划不构成独立产品审查，后续审查仍须独立，实施者不得自签 B／A PASS。

## Goal / Scope

允许用户以 `sync.media_extensions` 决定生成哪些后缀，并使被排除的已有归属输出按
现有完整轮／阈值／删除机制正常清理，即使源文件仍在。加回后能取消待清理或重新生成。
未配置沿用 11 项默认值；严格类型、规范化与替换语义以 Slice 配置合同为准。

交付程序、必要测试与使用文档。测试用来确认程序正确和相关行为未退化；优先复用现有
工具，只补直接服务本功能的用例与少量辅助代码，不建设复杂测试工具或通用测试平台。

## 实际代码检查与实施路径

| 位置／当前事实 | 本 Task 的必要改动 |
|---|---|
| `internal/config/config.go`：严格 YAML，尚无扩展名字段 | 新增原始字段与 Resolved 值；区分省略、null、空列表及非字符串元素，规范化并校验；加入安全字段诊断 |
| `internal/pathmap/pathmap.go`：全局固定 MediaExtensions、IsMedia | 提供默认集合和按实例使用的最后后缀匹配；规范化列表不修改共享全局 map，不引入 I/O |
| `cmd/strmgen/main.go`：runRound 构造 reconciler.Options | 将解析后的列表传入每轮同步；sync／dry-run／serve 复用；不改变 sourceIdentity 或调度协议 |
| `internal/reconciler/syncer.go`：handleDirectory 仅调用固定 IsMedia | 使用实例列表筛选；保留目录遍历、候选同名冲突、观察登记及唯一候选复用 |
| 同文件 confirmMissing／deleteCandidate／previewCleanup 都把 Get 成功视为保留 | 三处统一接入“仍被当前列表排除，或仍入选但原 Get 判定缺失”的本地清理资格；不能只改计数或实际删除而遗漏预览 |
| 既有 SQLite、LocalFS.DeleteOwned 与恢复、openlist.CheckSourceRoot／Confirmer | 复用归属、源根门槛、计数提交与删除恢复；不改 schema、原子文件原语、HTTP／分页／远端缺失语义 |

实施注意：

- 不能仅用普通字符串切片解码后判断长度：必须证明真实 YAML 入口拒绝 null、数字、
  布尔值、嵌套结构等，不被库的隐式转换当作合法字符串；不削弱 KnownFields 的未知字段拒绝。
- 有效集合在启动时确定并按实例持有。现有内部调用的零值选项仍沿用默认集，不影响旧测试；
  用户显式空列表必须在配置入口拒绝，不能悄悄回退默认。具体类型／私有函数由 Developer 选择。
- 已有记录按可靠的 RemotePath 判断原扩展名，不能从输出 `.strm` 猜后缀；未知精确映射
  沿用原保留规则。当前新候选已占用同一目标时，不按旧扩展名再删这个目标。
- 建议仅在 reconciler 中提取一个小的本地资格判定辅助函数供计数／删除／预览共用，
  不增加媒体发现状态机。排除分支仍检查 context 和必要源根可读性，可复用公开的
  CheckSourceRoot；入选分支沿用现有 Confirmer，不能伪装成远端真的缺失或改变 Get 业务语义。
- 继续分批枚举归属，新增缺失只由 CommitRound 提交；不能直接修改 missing_count、
  将失败轮算成功，或为每个文件创建 goroutine。实际删除与归属回收仍走原 LocalFS 路径。
- 本次只改程序接入、必要用例和 README／两份示例配置／需求／架构／状态文档；
  实施前示例不提前宣称支持，交付时删除“尚未支持”提示并写清移除后清理和重启生效。

## 验收条件与测试对应

下表是产品结果，不以新增用例数量或辅助工具复杂度计成绩；同一用例可以覆盖多个条件。

| AC | 可核验结果 | 测试与 RO |
|---|---|---|
| AC-01 | 省略时完整沿用 11 项；显式列表完整替换；点／大小写／首尾空白规范化及去重，多个实例不互相污染 | config／pathmap 现有测试中补表驱动用例；RO-1 |
| AC-02 | 空列表、null、非字符串元素、错误结构、空项、路径、通配符、多段后缀和 .strm 被拒；三个命令退出 2，不访问 API 或创建输出／库，错误不回显输入 | 真实 Load 及 CLI 入口测试；RO-1、RO-2 |
| AC-03 | 新增 .flv 能生成正确稳定 URL；未入选跳过；大小写、最后后缀、子目录和多 source 正常；相同输入再次同步不改字节／mtime | 现有 sync 驱动增补小样本；RO-2 |
| AC-04 | 清理启用后，排除但源文件仍存在也每完整轮计一次；前三轮只累计已提交证据，下一成功轮重核后删除；默认 3 和一个更高阈值有效；清理关闭不计数／删除；退回默认列表同样有效 | cleanup 测试及 mirror_cleanup；RO-3 |
| AC-05 | 达阈值后重新入选且源可读时清零、不删；仍被排除时源重现不清零；清理后重新加入可生成；旧计数在排除／源缺失之间沿用原机制，不另建计数状态 | 同一小批夹具顺序操作及聚焦单元测试；RO-3 |
| AC-06 | 两个入选源映射同目标报冲突；唯一入选新源复用自有目标后更新精确映射并清零，不能被旧排除记录误删；未归属／未知映射／本地类型异常继续保留 | 现有 sync／cleanup／路径负面测试补直接交互；RO-3 |
| AC-07 | 源根不可读、列表不完整、冲突、整轮取消不新增缺失或启动清理；排除分支同样遵守。dry-run 在阈值前不产生新证据、阈值后只预览当前有效候选；文件／原库／轮次／计数均不变 | 聚焦 cleanup 测试、现有 dry_run_boundaries／mirror_cleanup；RO-2、RO-3 |
| AC-08 | 排除分支删除失败／中断后重跑正确回收；不丢可靠归属、不误删其他输出、不让失败轮推进计数 | 复用 mirror_recovery 和既有删除钩子，见下文代表场景；RO-3、RO-5 |
| AC-09 | 发布 serve 首轮及 testhooks 后续轮遵守同一列表；运行中修改配置文件后仍使用启动列表，重启后才采用新列表；旧定时取消／失败继续／互斥回归保持 | 现有 serve 驱动增补列表与重启小用例；RO-2 |
| AC-10 | README 和两份示例可按说明使用，写明默认／替换／移除后清理及重启；原始命令与结果可复核，全部必跑通过，独立 B 重评全部 RO 后交 A | 文档检查、最终门槛及完成报告；RO-4、RO-5 |

失败恢复只补能覆盖新增分支的必要代表场景：

- 用既有 DELETE_AFTER_UNLINK_FAIL_TARGET 钩子制造删除后目录同步失败，确认归属保留，
  取消注入后用发布二进制重跑可回收，不再额外造故障系统。
- 用既有 DELETE_BEFORE_UNLINK_GATE／MARKER 取消排除目标的删除，确认未 unlink、
  未提交失败轮；正常重跑可删除。现有 SIGKILL／归属落库失败用例保留在全量驱动中。
- 归属落库失败的排除分支用现有 store 测试替身或既有受控故障方式覆盖；不得靠手工
  写计数伪造三完整轮。无需穷举所有文件操作、全部扩展名与所有故障的笛卡尔组合。

## 验证顺序与最终命令

先补缺失行为测试并记录首次失败，再实现／修复；先跑直接影响的聚焦检查：

```bash
bash scripts/go-local test ./internal/config ./internal/pathmap ./internal/reconciler ./cmd/strmgen
```

最终候选执行以下门槛；全量检查已覆盖的聚焦检查不再机械重复。本代码块仅供未来正式
实施后执行，本轮没有构建或运行产品测试：

```bash
python3 scripts/quality.py
mkdir -p artifacts/task-5.1/bin artifacts/validation-evidence
bash scripts/go-local build -trimpath -o artifacts/task-5.1/bin/strmgen ./cmd/strmgen
bash scripts/go-local build -tags strmgen_testhooks -trimpath -o artifacts/task-5.1/bin/strmgen-testhooks ./cmd/strmgen
TASK51_RUN_DIR="$(mktemp -d artifacts/validation-evidence/task51-XXXXXX)"
python3 scripts/run_isolated_sync_integration.py --binary artifacts/task-5.1/bin/strmgen --fault-binary artifacts/task-5.1/bin/strmgen-testhooks --scenario all --output-dir "$TASK51_RUN_DIR/sync"
python3 scripts/run_serve_integration.py --binary artifacts/task-5.1/bin/strmgen --fault-binary artifacts/task-5.1/bin/strmgen-testhooks --scenario all --output-dir "$TASK51_RUN_DIR/serve"
```

复用 scripts/mock_openlist.py 的动态端口服务，只改现有驱动的配置生成参数并加入必要
场景；默认保留全部旧场景。测试样本无需实际视频或播放能力。日志和现有结果文件足够，
不要求新运行器、通用断言库、服务编排、全库导出或新报告系统。

## 额外验证的触发判断与非目标

计划不改变 OpenList HTTP 请求／解析／分页／远端缺失语义，因此不默认重跑真实
OpenList 验收；只更新 YAML 配置示例不改变容器装配，不默认重跑 Docker／Compose 验收。
若实际改动触及这些接口、部署、构建依赖、平台代码，或出现具体兼容失败／证据缺口，
B 先列清触发原因、所需原有检查与隔离范围，交 A 修订边界后再实施；不能直接跳过。

按 Slice 排除万文件、Jellyfin／插件、播放、代理、ARM64 实机、正式发布、新依赖、
数据库迁移、新媒体状态机和每 source 独立配置。不动生产、配套仓库或旧隔离服务，
不读旧认证、不删远端源文件。既有文件／锁／数据库处理机制优先复用。

## 完成报告与审查

Developer 提供真实实现 SHA、完整 Task Base..Head 变更、构建／配置／夹具身份、
AC／RO 到测试及原始结果的对应表、首次失败与修复重跑、已知限制和未运行项。
报告使用上述既有证据目录，不按测试工具代码量评价完成程度。

独立 B 审查完整范围，并复现关键生成、排除清理、重新入选和失败重试；可复用身份相同
且适用的完整回归证据，不机械重复整套。全部 AC 满足且无 P0／P1 后才签 Task PASS，
汇总 RO-1..5 交 A 最终验收；非阻断润色不自动新增任务。实施者不得自签独立 B／A PASS。

本轮定稿检查：80 个本地文档链接、锚点、10 条 AC 对应、现有代码／测试引用、未来命令
Bash 语法及补丁空白通过；真实 Task Base／Contract Checkpoint 与已提交 ACTIVE 合同匹配。
此前治理因合同未提交返回 2；A 合同提交后以及本 Task 填写真实 SHA 后均已重跑通过，
不改写先前失败。产品测试均 NOT RUN，未签发独立产品审查 PASS。

```text
Planning Source: 1db2cb7b6a7ac360af3fae1ea8e2f90db0ef81e0
Planning Result: 正式 Task 5.1 PLANNED；真实合同和任务边界已固定
Implementation: NOT STARTED
Product Tests: NOT RUN
Independent B Review: PENDING
A Final Review: NOT STARTED
Next Action: Developer 按已提交 Task 5.1 开始实施
```
