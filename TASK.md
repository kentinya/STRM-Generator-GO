# Task 7.1 — 外挂字幕下载、归属与三轮清理

```text
Task ID: 7.1
Parent Slice: 7
Owner: B
Status: FIX REQUIRED
Task Base: e59af5280f11c0d69e735d4cbc951cf0f521bedb
Contract Checkpoint: e59af5280f11c0d69e735d4cbc951cf0f521bedb
Implementation Head: 173293be70862e530201cccce548534169d9a431
Independent B Review: FIX REQUIRED
Difficulty: High
Test Level: T4
```

## Goal / Scope

一次交付字幕配置、按目录补下载、SQLite 归属与三轮清理、命令摘要和使用说明，覆盖
RO-1..6。B 核对了 Slice Base `11b51b9ec4708f553671eefcb9d3b06dd43d0845` 至
合同检查点的完整差异。该范围是 A 的合同和文档提交，产品代码仍是 v1.2.1，
六项 RO 均待实施和验证。Developer 在本计划提交后将状态推进为 IN PROGRESS。
产品规则见 [Slice 7](SLICE.md)。

用户已有 `电影/Movie.strm`，在 OpenList 对应目录给 `Movie.mkv` 加入 `Movie.zh.srt`
和 `中文.srt`。开启字幕后，先 dry-run 查看两个待下载文件，再正式同步。两份字幕
保持原名和字节，第二次同步只报告已有跳过。在隔离库用 Jellyfin 扫描另一部同轮生成
STRM、同轮下载匹配名字幕的样片，在该视频的字幕轨列表查找外部轨。
之后更名 `中文.srt`，新名字当轮补齐，旧本地文件在三次完整缺失轮内保留，
后续一轮重新核对远端后清理。若首个缺失轮同时发生另一份字幕下载失败，该轮
缺失计数保持原值。修复远端下载后重跑，已发布的文件原样保留，缺件继续补齐。

字幕生命周期限于原名原字节补齐及已归属文件清理。关联和展示由 Jellyfin 原生扫描完成。
既有 STRM 协议、视频比较、来源身份、同步锁和 Telegram STRM 事件按现行合同执行。

## 开发指导

1. **配置与入口。** 在 `internal/config/config.go` 复用严格 YAML、扩展名规范化和
   `SafeDiagnostic`。四项默认字幕后缀为 `.srt .ass .ssa .vtt`，显式列表完整替换。
   开启时与已解析的媒体扩展名查重。通过 `cmd/strmgen/main.go` 的 `roundDeps` 和
   `runRound` 接入，三个入口共用，`serve` 每次启动确定配置。
2. **目录资格与映射。** `directoryWalker` 在 `List` 完成分页后串行调用
   `handleDirectory`。令 `handleFile` 明确区分可用 STRM 与该文件处理失败，
   dry-run 对应预计可用。现有新增和更新计数会在后续目录同步或归属登记出错时保留
   已发生的动作，资格应使用成功结果。每目录依据至少一个可用结果处理一次字幕，
   复用同份列表和路径段校验，同一时间只下载一份。视频用 `pathmap.LocalRelative`
   换后缀，字幕用相同边界拼接源内相对目录和原文件名。已归属字幕的出现观察独立于
   目录下载资格，按当前入选后缀与原精确映射确认。
3. **传输与发布。** 在 OpenList 传输层用 `pathmap.StableURL` 构造带前缀的 `/d/` 地址，
   使用无 OpenList 凭证的 GET。随轮次取消，每份 30 秒涵盖重定向与读体，允许最多五次
   HTTP(S) 重定向。完整 HTTP 200 响应限 8 MiB，流式复制时检查实际字节数和读错误，
   识别返回为 200 的 HTML／JSON 错误页。由 `LocalFS` 处理同名普通文件跳过、链接／
   非普通文件冲突，以及基于 `securepath.Dir` 的同目录临时写入、文件同步和无覆盖
   原子发布。按现有 `Publish` 的 `Link` 与父目录同步顺序复用文件原语，
   字幕使用自己的字节上限。失败或取消清理本次未发布的临时文件。发布完成后登记归属，
   中断留下的未登记成品按已有普通文件跳过并保持未归属。dry-run 的下载预览只检查
   路径和目标状态。
4. **归属与完整轮。** 在 `internal/storage/storage.go` 为 `Ownership` 持久化文件类型，
   事务升级将既有记录视为 STRM，保留已有路径、轮次及计数。同步维护 `Lookup`、
   分批枚举及只读打开（含 WAL 快照），使 dry-run 可读取升级前后的库。发布成功的字幕
   登记来源、精确远端及本地路径。下载或本地处理失败、目标安全冲突进入
   `runSource` 遍历后的不完整判断，先于该来源的清理或 `CommitRound`。
5. **清理共用。** 在 `confirmMissing`、`deleteCandidate` 和 `previewCleanup` 按归属类型
   统一判定映射和当前远端资格。字幕按自身精确路径调用现有源根／目标 Get 规则，排除
   扩展名同样进入缺失计数，明确远端类型冲突按合同保留。复用已提交阈值、当前重核和
   `LocalFS` 普通文件删除及已不存在时的归属回收。字幕关闭时跳过该类记录。仅关闭
   清理时，启用的字幕仍可凭列表或精确 Get 的出现观察清零，尤其是本轮没有视频、
   或列表未列出但 Get 仍读到字幕的目录。现有 `confirmAndCleanup` 仅在清理开启时调用，
   此处需要保留字幕的清零路径。继续使用同一来源轮次和有界批次。
6. **输出和文档。** 在 `printReport`、`printServeSummary` 展示字幕实际下载、已有跳过、
   失败、实际删除和 dry-run 的计划下载／删除，回收与实际删除分别核对。字幕诊断使用来源
   编号、经转义的相对路径和固定原因。字幕归属类型贯穿缺失提交和删除统计，让
   `FactSink` 继续提供 STRM 事件。同步更新 README、两份示例配置、部署、需求和架构，
   说明稳定下载地址的访问条件、`中文.srt` 复制与 `Movie.zh.srt` 关联、数据库升级和重启生效。

## 验收条件与测试

| 验收项 | 用户结果 | 验证入口与主要证据 |
|---|---|---|
| AC-1 · RO-1 | 旧配置和省略 enabled 时保持关闭，显式列表规范化后替换默认值。错误结构、空列表、非字符串、不在白名单及开启后与媒体后缀重叠，退出 2 并指出配置字段。 | 现有 config 测试覆盖各输入，CLI 三入口各验证代表性错误，核对请求次数和输出／库状态。 |
| AC-2 · RO-2 | 同目录多个视频只下载一次每份入选字幕。已有可用 STRM、嵌套目录和任意字幕原名正常。只有字幕、只有历史 STRM 或视频全部处理失败的目录跳过。 | 用现有 CLI 驱动运行启用、预览、首次同步、稳定重跑。核对完整分页、`refresh:false`、下载数、路径、字节、mtime。用 serve 驱动核对首轮、后续轮与配置重启生效。 |
| AC-3 · RO-3 | 从保留路径前缀的稳定地址下载，链路符合时限、大小和重定向合同。已有普通文件保留原字节和 mtime，链接、非普通目标和发布冲突保留并报告。 | OpenList HTTP 测试验证 URL 编码、无元数据 Token、重定向第五跳与第六跳、完整状态 200、错误响应、截断、大小边界和读体超时。复用 LocalFS、CLI 路径／故障入口核对边界、无部分发布和失败时临时文件清理。 |
| AC-4 · RO-4 | 已有数据库保留 STRM 归属及缺失证据，只有正式发布并登记的字幕可清理。人工放入、丢库遗留、发布后登记前中断遗留的字幕保持原样。 | 复用 storage 的旧库及只读测试、CLI 故障恢复和重跑。对照升级前后原库行、轮次和只读快照。重跑检查字幕字节和归属，覆盖已发布与已登记两个窗口。 |
| AC-5 · RO-4 | 三次完整缺失轮累计 `0 → 1 → 2 → 3`，下一轮按当前源根和精确目标重核后才删除。入选且确认出现时清零，改变扩展名或改名沿用同一计数规则。字幕和视频各自消失时按各自资格处理。 | 同一批样本顺序执行，把再出现安排在达到阈值后，另核对删除前 Get 当轮返回已出现、源根不可读或目标为目录。测试关闭字幕保留计数、关闭清理保留／出现清零、重新开启重核、先后移除字幕与视频及本地已不存在时回收归属。 |
| AC-6 · RO-4、RO-5 | dry-run 预览下载与当前可删除项，原文件、库、轮次及计数维持原值。下载失败、路径冲突、不完整列表和正式轮取消冻结该来源的推进与清理，正常恢复后可重跑。 | 复用 dry-run 快照和故障驱动。阈值前后分别预览，核对内容 GET 次数为零。至少一次失败轮同时有已达阈值的旧字幕和 STRM，核对保留与实际摘要，恢复后复查进度。适用时用现有删除故障入口核对中断后归属回收。 |
| AC-7 · RO-5 | sync、dry-run 和 serve 可区分真实发布、已有跳过、失败、真实删除和预计动作。失败诊断脱敏，原 STRM 摘要及通知事件准确。 | 复用上述 CLI 运行的 stdout／stderr、请求和持久状态核对统计。用现有通知合成回归核对字幕动作与 STRM 事件的归类。 |
| AC-8 · RO-2、RO-3、RO-4、RO-6 | 从真实 OpenList 下载含空格或中文名以及与视频不同名的字幕，重复轮跳过，移除后延迟清理。Jellyfin 隔离库能在同轮生成的 STRM 视频条目上看到实际下载的匹配名字幕轨。 | 用发布 CLI 和明确授权的专用隔离源记录 HTTP 操作、原始字节摘要、重复轮和逐轮计数。通过 Jellyfin 正常媒体库扫描及视频详情核对该条目和外部字幕轨，保存当次证据。 |
| AC-9 · RO-6 | 用户能按示例从旧配置升级并完成预览、下载和扫描。交付包含独立审查和全部适用 T4 证据。 | 核对示例与运行结果、打包产物、完整 Base..Head 和下表证据。 |

| 阶段 | 现有入口和证据安排 |
|---|---|
| 聚焦 | 先跑受影响的 config、openlist、storage、reconciler、CLI 用例及已知失败回归。通过现有功能入口补齐必要样本和断言，复用现有夹具、驱动与故障注入点。 |
| 最终 T4 | 运行 `python3 scripts/quality.py`、按开发流程执行 race、构建发布及 testhooks 二进制，运行已有 `run_isolated_sync_integration.py`、`run_serve_integration.py` 的全套回归。按部署流程验证 amd64／arm64 打包和适用的 `run_container_integration.py` 场景，记录交叉构建与原生运行各自状态。 |
| 真实服务 | OpenList 和 Jellyfin 使用本轮明确授权的专用隔离源与媒体库，优先复用可用夹具入口。旧 OpenList Get 回归只能证明元数据路径，字幕证据来自实际 CLI 下载、删除与 Jellyfin 当次原生扫描。若缺少可用隔离实例、下载权限或原生扫描证据，分别记录具体阻塞原因。 |
| 时间与证据 | 每轮记测试实际占用时长及整轮总时长，前者小于后者的 50%。完整套件覆盖的聚焦项可直接映射，证据身份匹配后可复用。若仍有必要验证，安排后续轮次并记录当前状态。保存候选 SHA、命令／cwd、夹具身份、退出码、原始日志、摘要及首次失败／重跑于 `artifacts/validation-evidence/`。 |

## Task Report

实施 Head：`eefbef028b60eabaad29e75c9f94e2eee8ee518d`（单提交，含产品代码、测试、
集成驱动与文档；Base..Head 全差异为 Task Base 后唯一产品提交，无其他隐藏改动）。

**改动摘要**

- `internal/config`：`sync.subtitles` 严格解析（enabled＋四项白名单 extensions，默认
  关闭；关闭时列表仍校验；开启时与 `media_extensions` 查重；错误字段进入
  `SafeDiagnostic`，退出 2）。
- `internal/pathmap`：`SubtitleExtensions()` 白名单与 `SubtitleRelative`（与视频相同
  的段校验，保原名拼接源内相对目录）。
- `internal/openlist/download.go`：`SubtitleDownloader`，稳定 `/d/` 无凭证 GET、
  30 秒预算、5 次重定向、8 MiB 上限，HTML/JSON 错误页／截断／超大分类为固定错误。
- `internal/reconciler`：`handleFile` 返回本轮可用结果；目录资格驱动
  `handleSubtitle` 下载；出现观察独立于资格；`LocalFS.PublishSubtitle` 流式同目录
  临时写入＋无覆盖 Link 原子发布＋父目录同步，自带字幕字节上限并显式拒绝超限；
  `currentTarget`/`confirmMissing`/`deleteCandidate`/`previewCleanup` 按
  `file_type` 统一判定；清理关闭时的 `confirmSubtitlePresence` 出现清零路径；
  字幕行不进入 `FactSink`；新增 `afterSubtitlePublish` testhooks 窗口
  （已发布未登记）。
- `internal/storage`：schema v3 新增 `file_type`（缺省 0＝STRM，升级保留既有
  行／轮次／计数），只读打开对升级前后的库均可用（缺列按 STRM 读取）。
- `cmd/strmgen`：三入口装配字幕集与下载器；`printReport`/`printServeSummary`
  输出 `subtitle_downloaded/skipped/failed/deleted/reclaimed/candidates`。
- 驱动：`scripts/mock_openlist.py` 支持固定 `/d/` 字节与下载计数；
  `run_isolated_sync_integration.py` 新增 `subtitles`、`subtitles_cleanup` 场景；
  `run_serve_integration.py` 新增 `serve_subtitles` 场景。
- 文档：README、两份示例配置、deployment、requirements（R-15）、architecture。

**逐项验收结果（本地可验证部分）**

| 验收项 | 结果 | 主要证据 |
|---|---|---|
| AC-1 配置 | PASS | `internal/config/config_test.go` 字幕用例（默认关闭、显式替换、白名单外拒绝、关闭仍校验、开启重叠拒绝）；CLI 三入口代表性错误 `cmd/strmgen/main_test.go`（sync/serve 退出 2、无输出／数据库） |
| AC-2 目录资格 | PASS | `internal/reconciler/subtitle_test.go`（多视频目录一次、无视频目录跳过、冲突目录跳过、嵌套目录、重跑跳过且字节/mtime 不变）；serve 驱动 `serve_subtitles`（首轮下载、后续轮跳过、编辑配置运行中不生效、重启关闭后不变）；驱动场景 `subtitles`（分页列表 `refresh:false` 由既有夹具断言） |
| AC-3 传输与发布 | PASS | `internal/openlist/download_test.go`（URL 编码、无 Authorization、5/6 次重定向、非 HTTP 重定向、非 200、HTML/JSON 错误页、截断、8 MiB 边界、内容恰好等限时接受、取消）；`internal/reconciler/fs_test.go`（流式字节、已有/符号链接拒绝、写失败临时清理、超限拒绝无残留）；CLI 驱动核对真实下载字节与同名跳过 |
| AC-4 归属升级 | PASS | `internal/storage/storage_round_test.go`（v2→v3 升级保行／轮次／计数、只读旧库按 STRM 读、字幕行往返、typed pending）；CLI `subtitles` 场景 testhooks 窗口（已发布未登记 → 重跑按未归属跳过不认领）；已登记窗口沿用既有 `afterOwnershipRecord` 机制 |
| AC-5 三轮清理 | PASS | `subtitles_cleanup` 场景与 `subtitle_test.go`：0→1→2→3、第 4 轮删除且 STRM 保留、再出现清零、扩展名排除照计（Get 仍成功）、改名走下载＋旧路径三轮、功能关闭冻结计数、清理关闭出现清零（含列表未列出但 Get 读到）、本地已不存在回收归属 |
| AC-6 dry-run 与冻结 | PASS | dry-run：预计下载／`subtitle_candidates=1` 预览、无文件无库行无轮次、下载请求为 0；失败冻结：下载失败轮不删除已达阈值字幕（`TestSubtitleDownloadFailureFreezesThresholdCleanup`＋驱动 `broken-download` 分支），恢复后同轮补齐并删除 |
| AC-7 摘要与通知 | PASS | `printReport` 六项字幕计数（`main_test.go`）；`TestSubtitleLifecycleNeverEntersTheFactStream`：字幕发布／推进／删除／清零均不进事实流，STRM 事件不变；既有通知合成回归全部通过 |
| AC-8 真实服务 | BLOCKED | 本轮无新授权的真实 OpenList 隔离实例（含稳定 `/d/` 下载授权）与隔离 Jellyfin 媒体库；按任务预定的记录方式，见下方"未运行与阻塞" |
| AC-9 交付 | PASS（除 AC-8） | 示例配置可用（驱动即按示例结构驱动真实 CLI）；完整回归、race、双架构交叉构建、arm64 OCI、容器六场景见下；独立 B 审查待提交后进行 |

**证据位置（均在 `artifacts/validation-evidence/`，另有 `artifacts/build/oci/`）**

- 全量 CLI 回归（16 场景含 `subtitles`、`subtitles_cleanup`）：
  `task-7.1-sync-full-8rEG2M/`（PASS，约 130s）
- 全量 serve 回归（9 场景含 `serve_subtitles`）：`task-7.1-serve-full-s7OieV/`（PASS）
- 容器 `image_static`（含 arm64 OCI 核对）、`minimal_run`、`serve_lifecycle`、
  `cross_container_busy`、`compose_lifecycle`、`notifications`：`task-7.1-container-*`
  七个目录（全 PASS；镜像 `strmgen:task-7.1` 本地构建）
- `python3 scripts/quality.py`：治理＋Python 125 项＋gofmt/vet/Go 全套（PASS）
- race：`CC=artifacts/dev-env/cc/bin/cc CGO_ENABLED=1 go test -race -count=1 ./...` 全 PASS
- 双架构交叉构建：CGO=0 amd64/arm64 静态二进制成功；
  arm64 OCI 归档 `artifacts/build/oci/strmgen-arm64.oci.tar`（8.7M）
- 候选 SHA、命令与退出码：本报告与 evidence `summary.json` 一致；
  二进制为提交 `eefbef0` 的 `-trimpath` 构建

**首次失败／修复记录**

- race 模式下原始 TCP 截断用例偶发失败（服务端未消费请求即回包）：先在测试内消费
  请求头后修复，重跑通过；非产品代码缺陷。
- 修复过程中的驱动断言修正：`public_url` 未指向夹具导致字幕 404（改用夹具地址）、
  损坏下载场景遗漏一个正文映射（补齐后失败计数恰为 1）、窗口重跑跳过计数 2→3
  （第三个已存在文件）、排除扩展场景先用默认列表种子再收窄。
- `fs.subtitleLimitWriter` 初版对单次超限写入静默截断：改为显式拒绝并清理临时
  文件（产品修复，含对应测试）。

**未运行与阻塞（如实记录）**

- 真实 OpenList 字幕下载（AC-8 前半）：NOT RUN。本轮未获得新的专用隔离 OpenList
  实例及稳定地址下载授权；既有 `run_openlist_absence_integration.py` 按任务约定
  只能证明元数据路径，不能作为字幕证据，故未纳入本轮运行。
- Jellyfin 隔离库原生扫描（AC-8 后半）：NOT RUN。本轮未提供／授权专用隔离 Jellyfin
  媒体库；需要用户按任务场景提供隔离源、媒体样片与扫描入口后补证。
- ARM64 原生实机运行：不在合同范围（仅交叉构建与 OCI 产物核对）。

**时间与测试占比**

本轮次实施约 50 分钟，其中测试／验证（含全部驱动、race、质量门、容器与双架构
构建）约 11 分钟，占比约 22%，低于 30% 上限。

## 修正轮报告(2026-11-20 · Developer,回应 B-01..B-04)

实施 Head:`f51db7b448e761569ea39eba83109efd51ec7a7a`(单个产品提交,提交身份 kentinya)。
相对 B 审查的实施 Head `eefbef0…`,其后只有 `adbb8d0`(Developer 报告)与 `ca05810`
(B 审查)两个文档提交;本次修正是唯一新增产品代码提交,工作区干净,未 push。

**逐项修正**

| 编号 | 修正 | 位置 |
|---|---|---|
| B-01 | 200 响应在内容类型之外按流式字节判定错误正文:`sniffWriter` 保留前 512 字节窗口,`<`/`{` 前缀判为 HTML/JSON 错误页;`[` 前缀只有完整响应为合法 JSON 才判错(否则会误伤 ASS 段头)。判定在完整性检查之后、调用方发布临时文件之前,有效字幕字节仍原样流式保存 | `internal/openlist/download.go` |
| B-02 | `handleFile` 只有创建、更新或有效归属确认全部完成才返回可用:未变化分支刷新归属失败、更新分支写入失败或刷新失败、发布分支发布失败或归属登记失败均返回不可用;已发生的 Created/Updated 计数与事实保留 | `internal/reconciler/syncer.go` |
| B-03 | `storage.ClearResult` 增加 `FileType`(同一条 SELECT),`noteObserved` 仅对 `file_type=STRM` 发出 `FactMissingCleared`,字幕列表重现清零不再进入事实流;STRM 路径不变 | `internal/storage/storage.go`、`internal/reconciler/syncer.go` |
| B-04a | `serve` 摘要聚合并输出 `subtitle_failed`;同名链接/非普通文件冲突与字幕发布后归属登记失败同时计入 `SubtitlesFailed`,实际发布计数保留 | `cmd/strmgen/main.go`、`internal/reconciler/syncer.go` |
| B-04b | 终端诊断行统一经 `notify.EscapeText` 输出(控制字符与双向控制字符转 `\uXXXX`),存储的诊断与磁盘文件名不变;覆盖 `printReport` 与 `printServeSummary` | `cmd/strmgen/main.go` |
| B-04c | `sync.subtitles` 严格解码:`enabled` 非布尔/空值与未知键返回字段命名错误,`Load` 只放行已命名固定字段的解码错误,`SafeDiagnostic` 输出 `invalid sync.subtitles.enabled`,退出码 2 与脱敏不变 | `internal/config/config.go` |

**新增与扩展的验证(均走真实入口)**

- `internal/openlist/download_test.go`:普通/缺省 Content-Type 下的 HTML、JSON 对象、
  JSON 数组(含超窗口)错误正文拒绝;ASS 段头正文(超窗口)接受且字节不变;在途读体
  取消(首块字节已到达写入方后取消);30 秒预算真实截断在途读体(30.03s,常量用例保留)。
- `internal/reconciler/subtitle_test.go`:唯一视频归属登记失败时字幕内容请求为 0、同目录
  另一部视频完成时正常下载;更新分支归属刷新失败、发布分支目录同步失败均不取得资格;
  字幕重现清零不进事实流而 STRM 重现照常;同一失败轮同时冻结已达阈值的 STRM 与字幕,
  恢复轮一起补齐并删除。
- `internal/config/config_test.go`、`cmd/strmgen/main_test.go`:`enabled: "true"`/数字 →
  `invalid sync.subtitles.enabled`、退出 2、不创建输出与库;`printServeSummary` 含
  `subtitle_failed`;诊断转义且存储诊断不变。
- 驱动: `subtitles` 新增纯文本错误正文两轮恢复、归属阻断(唯一视频 0 下载 / 兄弟视频
  正常下载 / 字幕登记失败双计数)、链接冲突计数、方向控制字符预览转义;
  `notifications` 新增字幕重现无消息、STRM 重现有消息;`serve_subtitles` 新增 404 首轮
  摘要含 `subtitle_failed=1` 且恢复轮补下载。

**证据**(`artifacts/validation-evidence/`,索引 `task-7.1-fix-INDEX.md`)

- `python3 scripts/quality.py` PASS(48s);race 全包 PASS(39s);Python 125 项 PASS。
- sync 16/16 PASS `task-7.1-fix-full-sync-Ydc97z`(130s);serve 9/9 PASS
  `task-7.1-fix-full-serve-nr3Iwa`(25s)。
- 容器 7/7 PASS(含 `arm64_oci`)`task-7.1-fix-container2-9e4e5b`;真实 OpenList 8 场景 +
  `cli_mirror` PASS `task-7.1-fix-real-openlist2-zoD02R`(108s)。
- 双架构静态交叉构建、arm64 OCI 归档、镜像 `strmgen:task-7.1-fix`;二进制 build info
  `vcs.revision=f51db7b…`、`vcs.modified=false`。

**首次失败与重跑**:见索引 —— vet 报新用例未使用变量后补齐;容器首跑漏传
`--hooks-binary`(用法错误,补参整轮复跑通过);真实 OpenList 首跑缺固定镜像,按 digest
拉取环境固定镜像后通过;`exact-limit` 用例首字节固定为测试调整(引入正文分类后随机首
字节可能落入判定),断言未变。反向对照:把 `syncer.go`/`config.go` 还原到 `adbb8d0` 后
三条新增用例全部 FAIL,恢复后 PASS,证明用例走真实入口而非构造场景。

**未运行与阻塞**:B-05 两段(真实 OpenList 字幕下载、Jellyfin 隔离库外部字幕轨)仍为
NOT RUN,原因同上一轮审查记录,待用户明确授权的专用隔离源与媒体库;ARM64 原生实机
不在合同范围。

**时间**:本轮约 75 分钟,测试/验证约 10 分钟,占比约 13%,低于 30% 上限。

**剩余风险**:错误正文判定依赖四种字幕格式的首字节约定(`.srt`/`.vtt` 不以 `<`/`{`
开头,`.ass`/`.ssa` 段头需整段 JSON 校验);`[` 前缀正文判定期间在内存镜像整段(上限
8 MiB、同一时间只有一份在途);诊断转义只作用于终端输出行,存储与数据库保持原字节。

## 第二修正轮报告(2026-11-20 · Developer,回应 B-01 BOM 变体与 B-04 剩余项)

实施 Head:`173293be70862e530201cccce548534169d9a431`(单个产品提交,提交身份 kentinya,
父提交 `957d1df` 为上一轮报告)。相对上一轮审查的实施 Head `f51db7b`,其后只有
`d47c06b`、`957d1df` 两个文档提交;本次修正是唯一新增产品代码提交,工作区干净,未 push。

**逐项修正**

| 编号 | 修正 | 位置 |
|---|---|---|
| B-01 | 正文分类先跳过 UTF-8 BOM 与空白:新增 `stripBodyPrefix`,同时用于签名判定与最终判定;`sniffSignature` 在窗口落在 BOM 中间时不落定,由后续字节决定标记是否完整,跨读分割的 BOM 不再把 `0xEF` 当成首字节;`[` 分支在去标记后的正文上执行 `json.Valid`;写入字节始终原样,不改写存储内容 | `internal/openlist/download.go` |
| B-04 剩余 | `sync.subtitles` 非映射块与未知键改为固定字段消息(`sync.subtitles: must be a mapping`、`sync.subtitles: unknown field`,均不回显配置值),`safeFieldFromError` 增加 `sync.subtitles` 条目,`SafeDiagnostic` 输出 `invalid sync.subtitles`,退出码 2 与脱敏路径不变 | `internal/config/config.go` |

**新增与扩展的验证(均走真实入口)**

- `internal/openlist/download_test.go`:`TestSubtitleFetchRejectsBOMErrorPages`
  (BOM HTML、空白+BOM HTML、BOM JSON 对象/数组/超窗口数组 × text/plain、
  application/octet-stream、缺省类型);`TestSubtitleFetchRejectsBOMErrorPageSplitAcrossReads`
  (首块只发3字节标记并 Flush,200ms 后再发正文,覆盖跨读分割);
  `TestSubtitleFetchAcceptsBOMSubtitles`(BOM srt 与 BOM ASS 段头、均超窗口,原字节不变)。
- `internal/config/config_test.go`:`TestSubtitlesBlockDiagnosticNamesField`(标量块、字符串块、
  序列块、未知键 → `invalid sync.subtitles`)。
- `cmd/strmgen/main_test.go`:`TestSubtitleBlockTypeErrorFailsBeforeAnyRound`(sync、
  sync --dry-run、serve 三入口均退出 2、诊断 `configuration error: invalid sync.subtitles`、
  不创建输出根与数据库)。
- 驱动 `scripts/run_isolated_sync_integration.py` 的 `subtitles` 场景新增 `bom-error-body`
  分支:BOM HTML 错误页首轮 exit 1、`subtitle_failed=1`、无成品/无归属行/无轮次与缺失证据,
  同轮 BOM 字幕按原字节发布;地址恢复后只补1份、原字幕不变、轮次提交1次。

**首次失败与反向对照(真实入口)**

- 修复前:openlist 两条 BOM 用例 `error = <nil>`;config 用例 `invalid configuration`;
  CLI 用例 `configuration error: invalid configuration`。
- 驱动反向对照:把 `download.go` 还原到 `f51db7b` 并重建两个二进制后,`subtitles` 场景
  FAIL于新分支(BOM 错误页被发布,`subtitle_downloaded=3 subtitle_failed=0`);恢复后重建,
  二进制 SHA 与候选一致。

**证据**(`artifacts/validation-evidence/`,索引 `task-7.1-fix2-INDEX.md`,日志 `task-7.1-fix2-5z6okg/`)

- `python3 scripts/quality.py` PASS(48.6s);race 10 包 PASS(39.8s)。
- sync 16/16 PASS `task-7.1-fix2-sync-Fi096A`(130.7s);serve 9/9 PASS
  `task-7.1-fix2-serve-5Z81jQ`(25.1s)。
- 容器 7/7 PASS(含 `arm64_oci`)`task-7.1-fix2-container-GnkCdm`(20.0s,镜像
  `strmgen:task-7.1-fix2` = `sha256:9af3eab3…`);真实 OpenList 8 场景 + `cli_mirror` PASS
  `task-7.1-fix2-real-openlist-WgUdf8`(110.5s,固定 digest 镜像)。
- 双架构静态交叉构建、arm64 OCI 归档;二进制 build info
  `vcs.revision=173293be70862e530201cccce548534169d9a431`、`vcs.modified=false`。

**未运行与阻塞**

- B-05 两段(真实 OpenList 字幕下载、Jellyfin 隔离库外部字幕轨)仍为 NOT RUN:本轮未获得
  新的专用隔离源与媒体库授权;真实 OpenList 驱动只覆盖元数据路径,不能充当字幕证据。
- ARM64 原生实机运行不在合同范围(仅交叉构建与 OCI 产物核对)。

**时间与测试占比**

本轮约 55 分钟;测试/验证命令(质量门、race、四套驱动、聚焦用例与反向对照)累计约
7.5 分钟,连同双架构、镜像与 OCI 构建约 9 分钟,占比约 16%,低于 30% 上限。

**剩余风险**

- 正文分类只识别 UTF-8 标记与空白前缀;UTF-16 等其他编码的错误页按二进制字节接受。
- 首字节约定(上一轮风险)不变:`<`/`{` 与整段 JSON 判定仍依赖四种字幕格式的开头形式。
- `notifications.telegram` 的块级诊断仍为旧消息形态(本轮 B-04 只点名 `sync.subtitles`,
  未改动,待 B 判断是否需要同一处理)。
- 诊断转义仍只作用于终端输出行,存储诊断与磁盘文件名保持原字节。

## Independent B Review

### 2026-11-20 · FIX REQUIRED

B 已审查 `e59af5280f11c0d69e735d4cbc951cf0f521bedb..eefbef028b60eabaad29e75c9f94e2eee8ee518d`
的完整差异，并核对后续报告提交 `adbb8d050604913255bcd78ac4f3846edb741198`。
从干净的报告提交构建两个复核二进制。该提交与实施 Head 的差异只有 TASK 报告。

| RO | B 复核结果 |
|---|---|
| RO-1 | FIX REQUIRED。关闭默认值、白名单和入口复用已有本地证据。配置里把 `enabled` 写成字符串时，退出 2，但诊断缺少应修正的字段。见 B-04。 |
| RO-2 | FIX REQUIRED。合格目录补齐、嵌套目录与稳定重跑已验证。视频 STRM 归属登记失败仍触发字幕下载。见 B-02。 |
| RO-3 | FIX REQUIRED。稳定地址、无 API 凭证、限额和正常原子发布有本地证据。实际错误正文可被当作字幕发布。见 B-01。 |
| RO-4 | FIX REQUIRED。三轮、下一轮重核、重现清零、dry-run 和部分故障恢复有本地证据。错误正文使同步报告成功，远端恢复后也无法补齐。见 B-01。 |
| RO-5 | FIX REQUIRED。正常字幕计数和 CLI 成功路径通过。定时摘要漏报字幕失败，路径未转义，字幕重新出现会触发 STRM 通知。见 B-03、B-04。 |
| RO-6 | BLOCKED。示例配置和使用说明已交付。真实 OpenList 字幕生命周期和 Jellyfin 原生外部字幕轨仍为 NOT RUN，本轮没有明确授权的专用隔离源和媒体库。见 B-05。 |

#### 阻塞项与 Developer 修正指导

**B-01 · 错误正文被登记成字幕。** 用户启用字幕并同步 `Movie.mkv` 和 `Movie.zh.srt`。
OpenList 下载链路返回 HTTP 200，正文是 JSON 错误，响应头写成 `text/plain`。
CLI 退出 0，报告下载 1 份并登记文件归属。下载服务恢复后，第二轮直接跳过已有文件，错误正文仍在。
`readSubtitleBody` 只看 Content-Type。用现有下载夹具补上普通 Content-Type 下的 HTML／JSON
错误正文用例，在临时文件发布前识别错误正文，继续流式保存有效字幕原字节。复跑错误及恢复两轮，
确认首轮标为不完整且无字幕成品、归属或新缺失证据，恢复轮成功补下载。

**B-02 · 视频未成功登记也会取得目录下载资格。** 首次同步时让状态库拒绝唯一视频
`Movie.strm` 的归属登记，CLI 报同步失败，但仍请求并发布旁边的 `Movie.zh.srt`。
重启后，原 STRM 按未归属文件报告冲突。原因是 `handleFile` 在新建归属失败时仍返回可用，
更新和发布分支也用已发生的文件动作充当成功结果。让目录资格取已成功完成的创建、更新或有效
归属确认，继续按实际结果记录已发生的 STRM 动作。复用现有 CLI 归属失败夹具，核对唯一视频
未完成时字幕内容请求为零，并保留同目录有另一部成功视频时的正常下载行为。

**B-03 · 字幕恢复误触发 STRM 通知。** 用户启用 `change_or_error`，同步一部影片，
暂时移走字幕再放回。缺失轮字幕计数为 1，恢复轮正确清零且 STRM 没有变化，但仍收到
一条含 `Movie.zh.srt` 恢复事件的 STRM 通知。字幕列表观察调用通用 `noteObserved`，
绕过了其他清理路径的类型筛选。按归属类型决定清零事件是否进入事实流。复用现有 Telegram
合成接收器，验证字幕列表重现清零时无消息，同样操作 STRM 仍发正常恢复消息。

**B-04 · 失败摘要和诊断未满足命令契约。** 用户用 `serve` 启动首轮，字幕地址返回
404，轮次报告失败，摘要却没有 `subtitle_failed`。同名链接冲突和字幕发布后归属登记
失败也未加这项计数。让摘要汇总失败数，保持实际发布与后续登记失败两项事实各自可见。
用户预览含方向控制字符的字幕文件名时，路径里的控制字符原样进入终端输出。统一转义字幕诊断的
相对路径，文件名继续原样保存。把 YAML 中 `enabled` 误写成 `"true"` 时，启动只报
`invalid configuration`。在严格配置解析入口返回固定的字段名，复用 CLI 的退出码和脱敏断言。

**B-05 · 真实服务验收缺少运行条件。** 在经明确授权的隔离 OpenList 源中，用发布 CLI
下载含中文或空格的匹配名字幕和一份任意名字幕，核对原始字节、重跑跳过和删除前的逐轮结果。
在获授权的隔离 Jellyfin 库中扫描同轮产物，从视频详情确认外部字幕轨。这两段证据分别提交。

#### 验证记录

- B 独立运行 10 包 Go 测试、CLI 的 `subtitles`、`subtitles_cleanup`、
  `notifications_cleanup` 和 `serve_subtitles`，均通过。另复用相同 CLI、合成
  OpenList／Telegram 接收器和现有 SQLite 故障夹具，复现 B-01 至 B-04。命令、候选身份、
  输出和计时保存在 `artifacts/validation-evidence/task-7.1-b-GexFje/`。
- Developer 的 sync 16/16、serve 9/9 原始摘要与现存二进制 SHA-256 相符。两份二进制的
  Go 构建信息标为 `66b768b8b35adf46d680704f9f1df7686af836b7` 且 `vcs.modified=true`，
  与报告所称提交后构建不符。七份容器场景摘要有镜像 ID 和场景结果。下次提供全量回归及适用
  打包的候选源码身份和命令日志。补齐现有计划内的在途读体取消、超时，以及同轮冻结已达阈值
  的字幕和 STRM 的证据。现有取消用例在发请求前取消，时限用例只核对常量。
- B 本轮测试及构建按各命令时长相加约 31 秒，少于本轮复核耗时的一半。完整容器复跑、
  真实 OpenList 和 Jellyfin 扫描为 NOT RUN。

### 2026-11-20 · 第二次独立审查 · FIX REQUIRED

复核完整范围
`e59af5280f11c0d69e735d4cbc951cf0f521bedb..f51db7b448e761569ea39eba83109efd51ec7a7a`，
结合首轮逐文件审查，核对了全部修复差异及报告提交
`d47c06bc2eda6f99c5415dfe87fab2c86da2905a`。发布及 testhooks 二进制均标明候选
提交 `f51db7b` 和干净工作区，摘要与 Developer 全量 CLI 记录一致。修正证据索引写错了
候选的直接父提交。Git 中的直接父提交是 `ca05810`。

| RO | 续审结论 |
|---|---|
| RO-1 | 部分通过。原配置兼容，三入口共用配置，`enabled: "true"` 已正确定位字段。整个配置块类型错误或拼错键时仍缺少字段提示，B-04 部分未关闭。 |
| RO-2 | 本地通过。唯一视频归属失败时没有字幕下载请求。同目录另一部视频完成时正常补字幕，B-02 关闭。真实 OpenList 结果见 B-05。 |
| RO-3 | FIX REQUIRED。普通 HTML／JSON 错误页已拒绝，读体取消和 30 秒时限通过。带 UTF-8 BOM 的 HTML 错误页仍能发布，B-01 未关闭。 |
| RO-4 | FIX REQUIRED。三轮清理、重现清零、失败冻结、dry-run 和正常恢复均有本地证据。B-01 的错误文件会被后续同步跳过。 |
| RO-5 | 本地通过。字幕重现清零不再触发 STRM 通知，B-03 关闭。serve 失败计数、字幕链接冲突、字幕归属登记失败和路径转义已通过，B-04 对应部分关闭。 |
| RO-6 | BLOCKED。说明与配置示例就绪。真实 OpenList 字幕下载及清理和 Jellyfin 外部字幕轨仍为 NOT RUN，见 B-05。 |

#### 本轮阻塞项与 Developer 修正指导

- **B-01**。用户启用字幕并同步 `Movie.mkv`。下载地址暂时返回用 UTF-8 BOM 保存的
  HTML 错误页，状态为 HTTP 200，类型为 `text/plain`。程序退出 0，把错误页写进
  `Movie.zh.srt`、登记归属并提交轮次。地址恢复后再跑，程序报告已有跳过，下载请求总数
  仍为 1。原因是正文分类在 BOM 处停止，没检查后面的 HTML 标记。在现有正文判定中
  识别 UTF-8 BOM，再检查 HTML／JSON 标记。将这个变体加到现有失败与恢复用例，覆盖
  首轮无成品和归属、恢复轮实际补齐，以及有效 UTF-8 BOM 字幕仍按原字节写入。
- **B-04 剩余项**。用户把示例简写为 `sync.subtitles: true`，或误写成
  `enable: true`。dry-run 退出 2，输出及状态库保持未创建，但两种错误都只显示
  `invalid configuration`。解码错误应使用固定的 `sync.subtitles` 字段，
  让 CLI 指出应检查的配置块。用现有配置错误用例核对退出码、字段和本地状态。
- **B-05**。Developer 的真实 OpenList 回归覆盖 List、Get 和 CLI Mirror。
  字幕验收仍需明确授权的隔离 OpenList 源和 Jellyfin 库，完成字幕下载、重跑、延迟清理，
  然后在原生扫描后查看同轮生成的 STRM 视频详情中的外部字幕轨。

#### 续审核对的证据

- B 独立运行 config、reconciler、storage、CLI 和 openlist 五包测试，均通过。
  openlist 包含在途读取取消及实际 30 秒时限。使用已核对身份的候选二进制运行
  `subtitles`、`subtitles_cleanup`、`notifications` 和 `serve_subtitles`，均通过。
  首次并行运行时 `subtitles` 争用固定同步锁失败，串行重跑通过。单独用发布 CLI 和
  同一 OpenList 夹具复现 B-01 和 B-04 剩余项。
- Developer 的全量 sync 16/16、serve 9/9 场景摘要与候选二进制匹配。核对容器 7/7
  摘要、ARM64 交叉构建身份及真实 OpenList 元数据／Mirror 摘要。B 本轮未重跑容器或访问
  真实服务。
- B 证据位于 `artifacts/validation-evidence/task-7.1-b-recheck-EsGagt/`。
  按各测试命令时长相加约 60 秒，少于本轮复核耗时的一半。

### 2026-11-20 · 第三次独立审查 · 本地通过，真实服务验收待补

审查范围为
`e59af5280f11c0d69e735d4cbc951cf0f521bedb..173293be70862e530201cccce548534169d9a431`。
在前两次审查基础上核对全部新增差异。后续提交
`03fb38e27afd71d91b8b8b1552609d9bf464a307` 只更新任务报告。
**Task 决定为 FIX REQUIRED，B-01 至 B-04 关闭。B-05 是唯一剩余验收项。**

| RO | 本轮结果 |
|---|---|
| RO-1 | PASS。默认关闭和三入口共用配置。把字幕块写成标量或拼错键，退出 2 并指出 `sync.subtitles`。 |
| RO-2 | 本地 PASS。同目录多视频只补齐一次，字幕原名与镜像路径保留。真实 OpenList 下载见 B-05。 |
| RO-3 | 本地 PASS。稳定地址、传输限额、已有文件保护和原子发布有候选证据。带 UTF-8 BOM 的 HTML／JSON 错误页被拒绝，合法 BOM 字幕字节保留。真实服务见 B-05。 |
| RO-4 | 本地 PASS。错误页轮次未发布失败字幕、登记归属或提交轮次。地址恢复后只补缺件。三轮计数、下一轮重核、dry-run 和清零通过。真实清理见 B-05。 |
| RO-5 | PASS。sync 和 serve 字幕统计、路径转义及 STRM 通知边界通过。 |
| RO-6 | BLOCKED。配置示例、文档及本地回归已核对。真实字幕下载和 Jellyfin 原生字幕轨仍为 NOT RUN。 |

独立 B 复跑了三个 Go 包的聚焦用例和串行 CLI 场景 `subtitles`、`subtitles_cleanup`，
全部通过，合计约 13 秒，少于本轮耗时的一半。证据在
`artifacts/validation-evidence/task-7.1-b-final-local-fJ4OIX/`。核对 Developer 的
质量、race、sync 16/16、serve 9/9、容器 7/7 和真实 OpenList 元数据／Mirror 记录。
sync、serve 的两个二进制摘要与带干净 `173293b` 构建标记的产物一致，真实 OpenList
Mirror 使用同一发布二进制。容器及真实元数据回归沿用 Developer 记录。

**B-05 等待专用隔离环境授权。** 获授权后，用同一候选 CLI 同步隔离 OpenList 的视频、
匹配名字幕 `Movie.zh.srt` 和一份含中文或空格的任意名字幕。核对下载字节、重跑跳过、
移除字幕后三次完整轮保留和下一轮重核删除。在字幕尚在时，让隔离 Jellyfin 库通过正常
扫描读取同轮生成的 STRM 和匹配字幕，并在对应视频详情查看外部字幕轨。分别保存两段
实际结果。Slice 7 继续 ACTIVE。

## 已关闭任务索引

- FIX-TG-01 和 Task 6.1 的完整关闭记录保存在
  `e59af5280f11c0d69e735d4cbc951cf0f521bedb:TASK.md`。
- 早期任务与角色指南见[固定历史检查点](docs/development-workflow.md#历史记录)。
