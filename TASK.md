# Task 2.1 — 远端缺失的权威再确认与真实 OpenList 实证

遵循 [开发流程](docs/development-workflow.md)，范围 authority 为已提交的 [Slice 2](SLICE.md)。本 Task 交付一次确认的完整判断及其真实服务证据；尚不开放 CLI 清理，不以本 Task PASS 代替 Mirror 交付。

2026-10-18 当前交接：A 已按用户认可裁定最终 Get／列表证明边界，见文末新增 A 决定及 [Slice 合同](SLICE.md#a-决定列表证明与最终-get2026-10-18)。本 Task 的 AC-4、Contract Checkpoint 与下列 B 历史记录尚待 B 在新合同提交后协调；保留旧计划与审查原文，不将本次 A 裁定当作 B 复审 PASS。Developer 在 B 完成合同适配前不得采用新的 500 例外实施。

```text
Task ID: 2.1
Parent Slice: 2
Status: FIX REQUIRED
Task Base: a8c2d62369a21e8d9a0c2cefec2af09927fb3286
Contract Checkpoint: 56167f7884682a0d8d0269440f7357ed19892ca1
Difficulty: High
Test Level: T4
Planner / Reviewer: B
```

## Goal

对配置源根内的**精确远端媒体路径**，提供可供后续 Reconciler 直接调用的当前确认能力，明确区分“存在”“已权威确认不存在”“无法确认”。只有成功刷新、完整分页及可靠的路径链证明才能返回不存在；权限、离线、缓存、响应错误、取消与类型冲突均不能误判。取得固定隔离 OpenList 版本／驱动的实际证据，使后续安全删除有可审查的远端依据。

对应 RO-3 的远端确认部分与 RO-6 的确认语义实证，并保护 RO-2 的源根边界。完整轮／计数、数据库迁移、本地删除与清理恢复留待本 Task 审查后的连贯接入 Task；本次不预排其 ID／Base 或签发验收。

## 实际缺口与范围

规划前工作区为干净 main；常规 `python3 scripts/check_governance.py` PASS。已核对 OpenList 实现及测试、Reconciler／LocalFS、storage schema／接口、CLI／配置、现有隔离驱动、环境与测试矩阵。当前没有待审 Developer 提交。

- `Client.List` 已实现有界分页、total 一致性、重复名称及结构校验，但固定 refresh:false。`IsNotFound` 只是错误详情包含 404 的字符串判断，没有权威性。
- Reconciler 已有 source 根 Get、有界 walker、失败不提交 round、逐 source 报告及独立 source 继续运行；继续复用。当前没有清理调用点，不为了测试新建删除入口。
- storage 只有 ownership 本地路径／created_round／missing_count 与 rounds；本 Task 不迁移它，也不把历史 round 当缺失证明。配置继续拒绝 cleanup.enabled:true，不出现已启用却只预演的半成品清理模式。
- 环境已有固定 Go／Zig 与可用 Docker／Compose；doctor 仅是环境检查。现有 dev/compose.yaml 是 Python 合成服务，真实确认目前 NOT RUN。

允许修改 `internal/openlist` 的确认／分页能力及测试，必要时在 `internal/reconciler` 增加小型远端确认适配及测试；不为接口占位改写 Syncer／Store。允许新增专用隔离 OpenList 配置、运行器、工具回归与操作说明；所有状态／媒体／产物仅落在 artifacts。调整既有响应解析须有固定版本源码及实测解释，并覆盖原有失败语义。原则上不改产品依赖；确需修改，在完成报告说明。

## 实施步骤

按以下依赖顺序完成；每步的测试属于同一 Task，不拆成参数、模块或单测试 Task。

### 1. 核实真实语义，建立隔离环境

1. 复用 dev/environment-spec.json 的 OpenList 源码身份 `3b28270a4066ea52943a69ecddd850d83f7e8bdc`。固定可追溯到该提交的二进制／镜像 digest 与配置；无匹配发布产物可在 artifacts 下构建该提交，不漂移到 latest，不改全局 SDK 或生产服务。
2. 新建专用 Compose project／容器，仅绑定 loopback 动态端口；媒体、状态、配置使用专用 artifacts 子目录。不能复用／重配现有 OpenList、Jellyfin、代理、配套仓库或旧证据。端口／项目冲突应失败或另选本 Task 名称，不停止别人的服务。结束时只停止自己创建的资源，保留证据。
3. 先用固定版本 Local 驱动证明精确名称、分页、实际删除／父目录移动与失败。关闭缩略图／媒体探测，不下载媒体补算哈希；仅创建小型专用测试文件。
4. 额外验证实际缓存路径。该提交 Local 配置声明 NoCache:true，Local 成功不能代替缓存实证。可另建同版本 WebDav 挂载，指向专用隔离 Local 的 DAV 端点并设置非零缓存，或采用等价可控的真实缓存驱动；记录后端、排序、TTL、权限。不能用 Python 假 API 的响应覆盖冒充真实驱动缓存。
5. 先核对这些源码线索，再实测：handler 的 refresh 权限与空 slice 序列化、op.List 的缓存／刷新／singleflight、fs.list 的可见性过滤／虚拟挂载错误处理、Local 的 List／Get。源码是调查依据，不是运行 PASS。

固定源参考：[handler](https://github.com/OpenListTeam/OpenList/blob/3b28270a4066ea52943a69ecddd850d83f7e8bdc/server/handles/fsread.go)、[缓存与刷新](https://github.com/OpenListTeam/OpenList/blob/3b28270a4066ea52943a69ecddd850d83f7e8bdc/internal/op/fs.go)、[过滤及虚拟挂载](https://github.com/OpenListTeam/OpenList/blob/3b28270a4066ea52943a69ecddd850d83f7e8bdc/internal/fs/list.go)、[Local 配置](https://github.com/OpenListTeam/OpenList/blob/3b28270a4066ea52943a69ecddd850d83f7e8bdc/drivers/local/meta.go)。

### 2. 复用有界分页，增加独立刷新读取

1. 保留普通 List(ctx,path) 的既有用途；抽取共享分页实现，提供显式刷新读取，不复制整套 HTTP／JSON 解码。禁止刷新失败后退回普通缓存 List 来证明不存在。
2. 每个用于否定的目录均须成功刷新并完整读取。最小方案为确认分页每页显式 refresh:true；采用其他顺序须证明每一页的新鲜性并记录理由，不能混入旧页或扫描结果。
3. 保留 30s 单请求超时、8 MiB 响应、每页 200 项、目录累计 10,000 项／2 MiB 名称、total／重复名／字段校验及 context 取消。不加载全源路径、不为每文件建 goroutine、不新增跨轮缓存。每目录结果用完释放；路径深度／总请求数有明确有限上限，超限无法确认。
4. 真实空目录可能返回 content:null,total:0。先保存实测，再决定是否精确接受此成功形状；B 允许据该证据调整旧合成测试“所有 null content 均无效”的假设。缺少 content／total、null 且 total>0、错误 code、非数组或其他不可判读形状仍拒绝，不能通配 null 为 [] 来掩盖失败。
5. 移除或明确废弃 IsNotFound 的误导性权威注释／用法。不存在不依赖错误文本或数字包含关系；Get 失败仍是失败。错误只提供安全原因，不回显原始 message、token、Cookie、raw_url 或完整私有路径。

### 3. 实现精确路径确认，不管理删除资格

1. 输入为源根与准确远端路径；验证绝对路径、逐段名称、长度、严格后代关系。拒绝根自身、相邻前缀、穿越、空段、NUL／反斜杠等；复用路径校验，不先 clean 隐藏非法输入。不从 .strm、basename、URL 或扩展名猜远端路径。
2. 默认结果为无法确认。可用小型结果类型表达存在／不存在／无法确认，加有限原因码与证明位置；只是本次调用结果，不是持久状态机。不存在须显式携带完整证据，任何 error／零值不等同于不存在。
3. 可从源根向下逐段确认：先确认**配置源根当前可访问且为目录**，再刷新并完整读其目录，按精确名称找到下一段；该段为目录才继续，直到目标父目录。最终名字存在时保留，即使已变成目录或其他类型也不能按缺失删除。
4. 中间目录名在成功刷新且可访问的父目录中明确缺失，可形成其后路径不存在的链证明；记录验证链，不把“直接列父目录失败”当父目录不存在。不跨出配置源根，为已缺失／离线的源根寻找删除理由。
5. 源根不可用／移除、权限／离线／分页错误、非目录中间段、名称或映射歧义，都无法确认；新调用须重新获取事实。目标再出现返回存在，不能复用上一次不存在结果。
6. **成功列表不自动等于权威列表。** 用真实隔离权限／隐藏和虚拟子挂载反例检查：可见性过滤、底层失败被虚拟项目遮蔽时，机制须能排除误判，或返回无法确认。普通用户可读但不能刷新时必须保留。不能仅证明管理员平坦 Local 的 happy path 就宣称所有 token／驱动可安全清理。
7. 若现有 list/get 无法辨认必要事实，先提交具体反例和最小替代机制给 B；涉及新增权限要求、配置约束、支持范围或验收变化，交 A 提交修订后再判断当前 Task 调整／继续。不能以下层 Task 的“部署者保证无隐藏／无嵌套挂载”代替合同。安全条件不满足时保持无法确认，不先开放删除，不标 PASS。

### 4. 补齐可重复运行器与回归

1. 新增 `scripts/run_openlist_absence_integration.py`，调用本 Task Go 确认实现，不在 Python 另写判断算法。采用 internal/openlist 的 openlist_integration tag 测试二进制，通过 --test-binary 传入；运行器负责隔离服务、文件变化、请求／结果采集及确定性屏障。
2. 入口为 --test-binary <path> --output-dir <new-empty-dir>；输出限制在 artifacts/validation-evidence，拒绝复用非空证据或接管其他容器。默认跑全部必要场景；缺驱动／权限／产物时非零并标 BLOCKED，不静默 skip。提交不含凭证的固定环境说明和复跑命令。
3. Python 工具回归覆盖目录限制、参数、子进程失败传播和只停止自己资源。Go 默认全套不依赖外部服务；真实门槛用独立入口执行并检查测试数／无意外 skip。
4. 保持生产 CLI 清理拒绝、sync／dry-run 行为及旧回归。无需增加 confirm／force 公开命令、额外产品配置或测试开关；测试辅助仅在隔离测试构建中使用。

## 验收条件

| ID | 必须可核对的结果 |
|---|---|
| AC-1 精确存在／缺失 | 文件在任意页，包括第 201 项以后时返回存在；完整刷新后精确名称缺失才返回不存在。中文、空格、#、%、大小写、相似前缀不混淆，非法路径在请求前拒绝。 |
| AC-2 父链／源根 | 消失父目录可由源内可访问祖先的完整逐段证明确认；源根缺失／不可访问／存储离线或父链不能完成时无法确认。根 / 的合法后代正确处理。 |
| AC-3 缓存／再出现 | 真实缓存驱动覆盖旧缓存仍有已删对象、旧缓存漏掉新出现对象，刷新后的事实决定结果。新调用重新读取；刷新失败、缓存空结果不构成不存在。 |
| AC-4 失败／可见性 | HTTP／JSON 404／403／500、超时／取消、畸形／不完整分页、total 改变／重复名／限额、刷新权限不足均无法确认。隐藏／读权限过滤及虚拟挂载掩盖驱动失败有实际反例和保守结论，不误判。 |
| AC-5 有界／隔离 | 内存只随受限目录／路径增长，深度与请求数有限，取消可退出；并发独立确认不共享可变分页游标或陈旧证明。只改变专用远端夹具，不写 STRM／ownership／round／missing_count。 |
| AC-6 实证／基线 | 固定真实 OpenList＋Local 及实际缓存路径的成功／失败原始证据齐全，空目录形状与源码对照。既有本地 T4 通过，生产 CLI 未开放清理。 |

AC-3／AC-4 的实际 OpenList 条目不能用 httptest／Python 夹具替代；合成 API 仍用于穷举传输、分页与结构错误。所有 AC 仅为本 Task 验收；本地归属、三轮资格及删除尚无本 Task 结论。

## 必要测试

### 开发／修正反馈

工作目录统一 /opt/STRM-Generator；先常规治理，再实施。

```bash
python3 scripts/check_governance.py
bash scripts/go-local test -count=1 ./internal/openlist ./internal/reconciler
python3 -m unittest discover -s tests -p 'test_*openlist*.py' -v
```

聚焦测试覆盖刷新每页、精确路径／祖先、再出现、失败／零值保守、深度／请求上限、取消和并发独立确认；断言返回结论及实际请求，不只测原因字符串。新增运行器测试文件若不匹配上述 glob，完成报告列出并执行准确命令。先修已知失败，再跑最终门槛。

### 最终检查点门槛

完整 T4 必跑；等级来自确认结果将控制删除的风险，不表示本 Task 实现删除。建立全新 TASK_EVIDENCE，不覆盖旧 run；外部测试夹具也限制在该目录。

```bash
mkdir -p artifacts/task-2.1/bin artifacts/validation-evidence
TASK_EVIDENCE="$(mktemp -d artifacts/validation-evidence/task-2.1-final-XXXXXX)"
python3 scripts/check_governance.py
bash scripts/go-local mod download
bash scripts/go-local mod verify
python3 scripts/quality.py
CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 -json ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 bash scripts/go-local build -trimpath -o artifacts/task-2.1/bin/strmgen-amd64 ./cmd/strmgen
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 bash scripts/go-local build -trimpath -o artifacts/task-2.1/bin/strmgen-arm64 ./cmd/strmgen
bash scripts/go-local build -tags strmgen_testhooks -trimpath -o artifacts/task-2.1/bin/strmgen-testhooks-amd64 ./cmd/strmgen
python3 scripts/run_isolated_sync_integration.py --binary artifacts/task-2.1/bin/strmgen-amd64 --fault-binary artifacts/task-2.1/bin/strmgen-testhooks-amd64 --scenario all --output-dir "$TASK_EVIDENCE/cli"
bash scripts/go-local test -c -tags openlist_integration -o artifacts/task-2.1/bin/openlist-integration.test ./internal/openlist
python3 scripts/run_openlist_absence_integration.py --test-binary artifacts/task-2.1/bin/openlist-integration.test --output-dir "$TASK_EVIDENCE/real-openlist"
git diff --check
```

最后的真实集成编译／运行入口是**本 Task 必须新增的内容**，目前尚不存在，并非声称已能运行。quality.py 已覆盖全部 Python 回归、gofmt、Go vet／test／build，不机械重复同输入聚焦测试。真实集成测试与 strmgen_testhooks 是不同用途；发布二进制不得带测试环境开关。所有命令用包装运行器保存 stdout／stderr、退出码、耗时，不只复制末行。真实环境不可用可先提交候选与 BLOCKED 证据，B 不签本 Task PASS。

### B 独立验证

审查完整 Task Base..实际 Implementation Head，包括测试、夹具、环境固定及范围；核对候选 SHA、二进制与原始证据，不凭摘要通过。

至少独立重放：多页最后一页存在；源内父目录删除与源根离线的不同结果；缓存漏掉再出现文件及刷新失败；权限过滤或虚拟挂载吞错反例。使用同固定真实服务／驱动，核对夹具真实文件、API 页与 Go 结论；另独立核对 content:null 实测分类。确认生产 CLI 清理仍拒绝，dry-run 与输出／DB 保持原边界。完整 quality／race／双架构及旧 CLI 全套，在输入身份一致时可复用 Developer 原始证据；变更输入／出现疑点按影响重跑。

PASS 后逐项重评 Slice RO-1..RO-6，不以确认能力替代完整轮、归属、删除、迁移与恢复。这些当前仍未完成，不提交 Closure Packet，不宣布 Slice PASS／关闭。

### 证据与重跑

证据保存在 artifacts/validation-evidence/<new-run-id>/。索引包含完整源码 SHA、未提交可执行输入摘要、cwd／命令、Go／CC、服务源码 SHA／镜像 digest／二进制 SHA-256、驱动／权限／缓存／挂载配置身份、夹具变化时间和请求顺序、响应与 Go 结论、测试数／skip／退出码／耗时。token、Cookie、签名 URL、宿主私有完整路径脱敏；不保存未经脱敏的服务启动凭证输出。

合成 Go／Python、真实 OpenList 确认、旧 CLI／故障恢复、编译／race 分开报告；真实 /d/、固定插件实际解析、Jellyfin／代理、ARM64 实机及规模保持 NOT RUN。保留首次失败及重跑，不覆盖。最终后修改产品、测试、运行器、驱动配置或可执行输入，T4 候选保留完整门槛；纯文档后继确认输入相同后可复用，重跑文档消费的治理与空白检查。

## 非目标

不开放 cleanup.enabled:true；不累计缺失计数、不迁移 SQLite、不删除 STRM／归属、不实现完整 Mirror／移动恢复或 dry-run 清理候选。不新增远端版本锁、每次生成前请求、媒体指纹／发现／恢复状态机。不新增强制管理员身份、无嵌套挂载等稳定要求来绕过反例；必要合同变更交 A。serve／发布 Docker／端到端播放／全驱动承诺／规模与权限加固系统不属本 Task。

## Developer Completion Report

- Changed Files / Implemented：新增 `internal/openlist/absence.go` 实现一次确认的有界判断
  （`AbsenceStatus` 零值为“无法确认”、有限原因码、完整验证链）；`openlist.go` 增加
  `ListFresh`／共享分页实现、结构化 `APIError.Code`／`HTTPStatus`、`IsPermissionDenied`，
  `Entry.Virtual` 与 `Get` 的 `provider` 解析，并把 `IsNotFound` 明确废弃为不适合作缺失依据；
  新增 `internal/openlist/openlist_integration_test.go`（`openlist_integration` 标签）与
  `scripts/run_openlist_absence_integration.py` 固定隔离运行器、`tests/test_run_openlist_absence_integration.py`；
  `docs/development-environment.md` 补运行说明与固定版本语义；`dev/environment-spec.json` 登记新文件。
- Tests and Results / Evidence：最终 T4 候选全部 12 项门禁 PASS（治理、mod download／verify、
  quality、`-race ./...` 221 测试 0 skip、linux amd64／arm64 构建、testhooks 构建、
  integration 测试二进制、隔离 CLI 9/9、真实 OpenList 8/8 场景 9 个集成测试 0 skip、
  `git diff --check`）。证据索引见 `artifacts/validation-evidence/task-2.1-final-ftETbR/INDEX.md`，
  首次失败与修复见同目录 `first-failures/README.md`。
- Decisions / Risks / Remaining Required Outcomes：确认为“真实存储根 + 逐级刷新完整列表 +
  精确名称缺失”，直接读取仅用于发现可见性过滤或列表矛盾；实测固定版本对缺失对象、驱动
  故障与虚拟挂载视图返回同一应用码，且嵌套挂载会掩盖父驱动故障，因此单一 `get`／`list`
  成功都不作为缺失依据。旧合成假设“所有 null content 无效”按实测收窄为
  `null + total==0 + 第一页`。本 Task 不开放清理：`cleanup.enabled: true` 仍在任何输出／
  数据库变更前被拒绝，RO-1（配置／入口）、RO-2（完整轮与缺失计数）、RO-3 的本地删除部分、
  RO-4（移动闭环）、RO-5（持久化与恢复）、RO-6 的预览部分均未实现；真实 `/d/` 播放、
  固定插件解析、Jellyfin／代理、ARM64 实机与规模仍 NOT RUN。
- Head SHA：实现检查点 `2fc3f6f06b6bc02ec9503e373cb031fc438f0346`。该提交的可执行输入
  （`internal/`、`scripts/`、`tests/`、`dev/`、`docs/`）与门禁运行时的候选工作树逐字一致；
  门禁运行后仅 `TASK.md` 状态与本节文字变化，属纯文档后继，按流程重跑消费文档的治理检查。
- Evidence Run：`artifacts/validation-evidence/task-2.1-final-ftETbR/`（含 `INDEX.md`、
  `gates.tsv`、`logs/`、`cli/`、`real-openlist/`、`first-failures/`）。该目录位于本机
  Git 忽略路径，不是远端可读证据。

## B Review Findings / Required Fixes

完整审查范围为 Task Base `a8c2d62369a21e8d9a0c2cefec2af09927fb3286` 至实现 Head
`2fc3f6f06b6bc02ec9503e373cb031fc438f0346`；其后 `da2c718` 只记录证据与检查点身份。
实现、CLI 基线及隔离证据大体符合范围，但下列 AC-4 问题阻止 PASS。

### B-1 最终目标 Get 的请求失败可能被确认为不存在

`internal/openlist/absence.go:233` 的 `probeFilteredTarget` 只特殊处理权限拒绝，随后把任意
`*APIError` 当作可忽略结果并返回 `ReasonNone`。`internal/openlist/openlist.go:123` 的
`Client.do` 会把传输失败、取消／超时、非 2xx HTTP、JSON／响应形状错误以及应用层错误都包装成
`APIError`；其中传输失败的 `Code` 与 `HTTPStatus` 均为 0。若根 Get 和刷新完整列表成功，最终
目标 Get 随后超时或被取消，当前确认器仍会返回 `AbsenceConfirmed`。现有
`TestConfirmCancellationAndTimeout` 没有让取消／超时发生在这个最终目标探测请求中；
`TestConfirmGenericGetFailureIsAbsenceOnlyAfterListing` 反而将宽泛接受行为固定为预期。

### Developer 修正步骤

1. 保留现有路径校验、请求／目录上限、每页刷新完整读取和祖先链行为；只修正最终目标探测的
   失败分类，不引入跨调用缓存、清理资格或持久状态。
2. 在最终目标 Get 返回后重新检查 `ctx.Err()`。取消／deadline、传输失败、非成功 HTTP、畸形或
   不完整响应及未明确获准的错误必须返回 `AbsenceUnknown`，不能因其被包装为 `*APIError` 而
   当作不存在。只通过结构化字段判断，不检查 `Detail` 文本或数字子串；保留权限拒绝为保守
   Unknown，并保留成功 Get 与此前列表矛盾时的 Unknown。
3. 增加最终目标探测专用取消与超时回归：让源根 Get 与 `ListFresh` 成功，仅阻塞最后的目标 Get
   直到 context cancel／deadline；断言结果不是 `AbsenceConfirmed`，且原因和请求轨迹能证明失败
   发生在目标探测阶段。现有早期请求取消／超时用例继续保留。
4. 为最终目标 Get 增加结构化失败表格用例，至少覆盖 transport（`Code=0`、`HTTPStatus=0`）、
   HTTP 404／403／500、无效／缺失响应及应用错误码；同时覆盖实际缺失对象和隐藏权限对象的
   已知响应形状，断言结论而非只断言原因字符串。
5. 按 A 对下方合同问题的裁定实现缺失对象语义。更新实际运行说明与测试，使 fixed OpenList 的
   实际缺失、隐藏项及失败语义彼此区分；不能仅通过放宽断言或降低真实场景覆盖完成修正。
6. 运行治理、聚焦 OpenList/Reconciler 与 Python runner 测试，并对改动后的完整 T4 候选重跑
   必要门禁和固定隔离 OpenList 实证。代码／测试／运行器变更后的旧候选证据不得直接当成新
   输入的 PASS；保留旧证据与首次失败。

### A Contract Decision Required

当前 Task AC-4 要求 HTTP／JSON 404、403、500 均无法确认；Slice RO-3 也规定任意 Get 错误本身
不构成不存在。另一方面，Task 已固定版本的真实证据显示：真正缺失对象的 Get 返回 HTTP 200、
OpenList 应用码 500；真实目录刷新列表完整且不含该名称。若严格将该应用码 500 也判为 Unknown，
当前确认机制无法对真实缺失对象给出 `AbsenceConfirmed`；若像当前实现一样忽略所有非权限
`APIError`，最终探测的超时／取消及其他传输／响应错误会被吞掉。B 不修改 Slice／AC-4。请 A
裁定：是否调整 AC-4，明确“完整刷新列表是唯一缺失证据，而最终 Get 仅可按经实测的结构化结果
否决该证据”的边界；或指定另一个满足现有 AC-4 的可用确认机制／支持范围。A 提交合同决定后，
B 再判断 Task 2.1 是按合法调整继续，还是撤回并重规划；Developer 在此之前不扩大接口或产品范围。

## B Review Result

```text
Reviewed Range: a8c2d62369a21e8d9a0c2cefec2af09927fb3286..2fc3f6f06b6bc02ec9503e373cb031fc438f0346
Decision: FIX REQUIRED
Blocker: B-1; await A decision on the AC-4 / fixed OpenList application-code-500 conflict
Independent Verification: governance PASS; focused ./internal/openlist ./internal/reconciler PASS; fixed real OpenList replay 5/5 scenarios PASS (artifacts/validation-evidence/task-2.1-b-review-20261018/summary.json)
Slice Required Outcomes all satisfied: NO
Next: A decides the contract boundary; then Developer addresses B-1 in Task 2.1 and returns for B review
```

## B Re-evaluation after A Contract Revision

2026-10-18，A 按用户授权提交合同检查点 `56167f7884682a0d8d0269440f7357ed19892ca1`，仅将
`cleanup.missing_confirmations` 的默认值与最低值从 2 调整为 3；三轮资格只延后清理，未改变本
Task 的精确远端确认目标、AC-4 或 T4 门槛。Task Base 保持
`a8c2d62369a21e8d9a0c2cefec2af09927fb3286`，本 Task 继续，不撤回或重规划。

我重新核对了 `internal/openlist/absence.go` 与 `absence_test.go`：B-1 仍未修复。成功读取源根并
完整刷新目标父目录后，若最终目标 `Get` 因取消、超时、传输失败或其他未分类原因返回 `*APIError`，
`probeFilteredTarget` 仍返回 `ReasonNone`，可能将请求失败吞成权威不存在。现有取消／超时测试只覆盖
前置请求；`TestConfirmGenericGetFailureIsAbsenceOnlyAfterListing` 仍明确锁定宽泛接受行为。

另一个 AC-4 冲突仍待 A 裁定：固定真实 OpenList 对真正缺失目标的 `Get` 实测为 HTTP 200、应用码
500，而当前 AC-4 要求 HTTP／JSON 500 无法确认。提高缺失轮数没有授权改变此语义。Developer 可在同一
Task 内修复超时／取消／传输等明确失败的吞没，但不得自行把应用码 500 改判为缺失；最终缺失分类及
真实服务验收须遵循 A 对 AC-4 的裁定，并重跑受影响的完整 T4／真实 OpenList 证据。

```text
Reviewed Contract Checkpoint: 56167f7884682a0d8d0269440f7357ed19892ca1
Decision: FIX REQUIRED
Task: continue Task 2.1; no replanning
Blockers: B-1; A decision still required for the AC-4 / actual OpenList application-code-500 conflict
Next: Developer fixes B-1 in this Task without changing the unresolved 500 semantics; A adjudicates AC-4 before final real-service acceptance
```

## A Contract Decision — 2026-10-18

用户认可完整刷新列表作为缺失证据的方向。A 已在 SLICE RO-3／验收、需求、架构 TARGET 与测试矩阵同步实质修订：最终目标 Get 的 HTTP 200＋完整有效应用码 500 错误信封，在独立完整列表证明已成立且 context 有效时，允许不否决该证明。它本身不证明缺失，源根／祖先锚点 Get 与 ListFresh 失败不适用此例外。权限／矛盾、最终请求的取消／超时／传输／读取失败、非成功 HTTP、畸形／不完整响应与其他未允许应用错误仍须 Unknown。完整边界以[需求](docs/requirements.md#最终-get-与列表证明的边界2026-10-18-a-裁定)为准。

这已解决“是否允许调整 AC-4”的 A 合同决策，尚未解决 B-1 实现缺陷或签发测试通过。B 下一步须核对已提交合同，决定继续／调整或重规划 Task，更新 Contract Checkpoint、AC-4、步骤与测试映射；维持 High／T4、真实服务权限／缓存／虚拟挂载覆盖和历史证据。A 本回合不代写 B 决定、不改现有产品实现。Task 仍 FIX REQUIRED，当前旧 Checkpoint 将因合同能力不一致被治理拒绝，直到 B 完成适配；这不是产品回归结论。
