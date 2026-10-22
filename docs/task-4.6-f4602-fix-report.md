# Task 4.6 F-46-02 修正交接（Developer，2026-10-22）

同一 Task 4.6 的修正轮，只闭合 B2 指出的 F-46-02 与 F-46-03 交接；不新建 Task、
不改 Go 产品、不动锁定插件／服务／数据集／4.5 输出、不触碰 Jellyfin 或插件数据库、
不取密、不触发任何扫描或任务。Base 与实现 Head 不变：Task Base
`b599dbc5c0e6bd394ee361343d74ebbb12d1ecab`，原实现 Head
`69d022e2f7acb41fe1002fad9652386e4460cc75`。本轮修正 Head 见文末。

## Changed Files / Implemented

仅四个已跟踪的 Task 4.6 取证辅助文件及其 `artifacts/task-4.6/` 工作副本
（唯一差异是 `parents[2]`→`parents[3]` 的 REPO_ROOT 深度适配）：

- `docs/task-4.6-artifacts/fingerprint_read.py`：改为 fail-closed 退役桩。
  删除 `sqlite3` 与插件库路径；`STORE=None`；`read()` 抛
  `DirectStoreReadForbidden`；CLI 退出码 3 并提示退役。文件头保留事实历史：
  旧版曾只读打开 `strm-fingerprints.db`（不合规，见 F-46-02），历史证据保留、
  不删不改、在审查记录中标记为不合规，不改写为已获授权。
- `docs/task-4.6-artifacts/matrix.py`：
  - 删除 `fingerprint_of` 对退役条目的调用；`fingerprint_of` 保留为 fail-closed
    桩（`RuntimeError`），误调用即响亮失败。
  - `predicted_reason` 改为从**原生插件决定日志**镜像历史侧：以
    `historical_reason`（AC-2 基准或上一行恢复后确认的复用）加记录在案的基准 A
    事实（`BASELINE_URL`／`baseline_sha1`／`baseline_size`）构造镜像；FirstTrust
    镜像为无可比历史，Unchanged 镜像为仅有 Size 的历史。镜像绑定基准 URL，
    使 D-05 的 URL 变化仍能预测 `UrlChanged`。输出含
    `history_side.fingerprint_store_read=false` 与来源说明。
  - `row_result` 签名增加 `historical_reason`／`baseline_sha1`／`baseline_size`，
    记录 `baseline_input`（来源为原生决定日志、无库读取）；移除
    `fingerprint_before/after/after_restore`。
  - `phase_setup` 增加 `fingerprint_store_reads: "none"` 声明；`phase_rows`
    输出注明历史侧仅来自原生决定日志。
  - **F-46-03**：`phase_restore` 前值改从 `RESULTS["setup"]
    ["plugin_configuration_before"]` 读取；缺失时抛 `RuntimeError` 拒绝回退到
    当前（可能已缩小）配置，指向原生配置页恢复路径。
- `docs/task-4.6-artifacts/decision_model.py`：新增 `hash_url()` 导出（与
  `Manifest.url_hash` 同一算法），供历史镜像使用；决策逻辑零改动。
- `docs/task-4.6-artifacts/test_task46_tools.py`：新增 9 项直接测试
  （16→25）：历史镜像精确性（Sha1Matches/Unchanged/FirstTrust）、
  UrlChanged／NativeUrlMismatch 判别、MissingPrimaryStream 优先、
  `fingerprint_of` fail-closed、退役桩 fail-closed 与退出码。

## Tests and Results / Evidence

证据根（本机、Git 忽略、保留至 A 决定）：
`artifacts/validation-evidence/task-4.6-dev-f4602-20261022T005140Z/`

- `verification.log`：治理 PASS；`python3 -m unittest discover -s
  artifacts/task-4.6/tests -p 'test_task46*.py' -v` 25/25 OK；全部工具副本
  `py_compile` 通过。
- `f4602-retired-entry-behaviour.json`：退役桩实测——`STORE=None`、
  `read()` 抛 `DirectStoreReadForbidden`、`main()` 退出码 3、未 import
  `sqlite3`。
- `f4603-restore-entry-reproduction.json`：restore 入口双案例复现（全部外部
  I/O 打桩，无服务／库副作用）：setup 形状存在时恢复值逐字段等于前值；
  缺失时拒绝回退并抛 `RuntimeError`。
- `historical-index-reverification.json`：只读复核 2026-10-21 证据根
  `task-4.6-dev-20261021T153126Z/INDEX.json`：21 条目中 20 条非自引用条目
  字节摘要全部匹配；自引用的 `INDEX.json` 条目按构造失效（记录原值
  `7f1fda22…1084a`，实际 `e496792f…`），未改写任何旧文件。
- `execution-input-identity.json`：7 对 tracked／工作副本执行输入摘要；
  差异仅为已记录的 `parents[2]→parents[3]` 深度适配，无其他差异。

**未运行项**：Go 产品零改动，未跑 Go 测试（B 指定全量含 Go／race／构建／
集成，仍由 B 持有）；未重跑八行真实矩阵（B2 明确不要求为移除只读依赖而重跑）；
未做任何真实 Jellyfin／S3 操作。16 项既有工具测试全部包含在 25 项结果内。

## 历史证据处置（不伪造、不删除）

- 2026-10-21 的八行窗口与 `fingerprint_*` 字段保留原样；其中
  `fingerprint_before/after/after_restore` 来自现已退役的禁止入口，
  在本文件及矩阵文件头声明为历史不合规证据。行内 `observed_decision`、
  原生日志窗口、`item`／任务窗口与恢复结果不受影响，仍是有效事实。
- `05-diagnostic-side-effect.json` 的原始披露保持不变；F-46-04 已由用户接受，
  本轮未做任何清库、再恢复账户或指纹清理。
- `04-restore.json` 标为 `plugin-config-restored` 的值确为当时缩小的配置
  （B2 已记录）；最终恢复由 `06-final-state.json` 的人工原生恢复完成。
  本轮修正防止该入口缺陷再次发生，不以工具完善重新解释历史结果。

## Decisions / Risks / Remaining Required Outcomes

- 镜像历史侧是插件自身在原生日志中确认的最后理由＋记录在案的基准 A 事实，
  不是从库中读出的指纹；预测仅用于发现无关条件触发的行为，不替代观察。
- 不可比历史（FirstTrust）镜像为 `previous=None`，与锁定
  `ProbeDecisionService` 的分支一致；基准 URL 绑定保证 D-05 预测为
  `UrlChanged`（接受分支含 `NativeUrlMismatch`，与 TASK 一致）。
- 剩余：B 独立原生复现（原始扫描＋完整 8 行）、B 指定全量、Slice Final／
  Closure、A 最终审查。Developer 不自判 PASS，本 Task 状态保持
  **READY FOR B REVIEW**（同 Task 修正后交 B 复审）。

## 计时

本轮墙钟约 50 分钟（含治理、工具修正、25 项测试、只读复核与恢复入口复现）。
测试／验收类（unittest、py_compile、打桩复现、只读索引复核）约 1 分钟，
远低于本轮 30%。未运行 Go／B 全量。

## Head SHA

修正检查点：见 Developer Completion Report 追加段（本次提交的完整 SHA）。
