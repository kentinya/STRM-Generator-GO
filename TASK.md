# Task 2.1 — 源根与目标 Get 缺失判定及真实 OpenList 实证

遵循[开发流程](docs/development-workflow.md)，合同为已提交的 [Slice 2](SLICE.md)。本 Task 交付一次无持久状态的远端判定，不开放 CLI 清理，不累计缺失次数，不代替完整 Mirror 交付。

```text
Task ID: 2.1
Parent Slice: 2
Status: PASS
Task Base: a8c2d62369a21e8d9a0c2cefec2af09927fb3286
Contract Checkpoint: 57ea31b88a0dde95e486f8bac11ebe9e473f74ea
Difficulty: High
Test Level: T4
Planner / Reviewer: B
```

## Goal

对配置源根内的精确远端路径，提供“源根不可读／本次调用中止”“目标可读”“目标未读到”的当前判定。源根未成功 Get 为目录时保留；源根可读取、目标 Get 未读到即形成缺失观察，不再区分导致未读到的错误原因。所有列表请求均显式 refresh:false，判定器本身不调用列表或逐级祖先 Get。规则依据[需求](docs/requirements.md#简化删除判定2026-10-18-用户最终决定)。

对应 RO-3 的远端 Get 判定和 RO-6 的固定真实服务实证，并保护 RO-2 的源根／取消边界。是否计入一次完整轮、观察集合、三轮计数、当前删除重核及本地删除由后续 Reconciler／storage 接入负责，本 Task 只返回一次调用结果。

## B 合同重评与准入决定

2026-10-18，用户要求 B 同步 Task。B 已核对真实代码及 A 最新合同 `57ea31b88a0dde95e486f8bac11ebe9e473f74ea`，决定**原 Task 2.1 合法原地调整并继续，不撤回或另建任务**。目标仍为远端判定加真实服务实证，所需改动位于既有 API／确认器、测试及隔离运行器，不需要扩大至持久计数／本地删除。Task Base 不变，保留 FIX REQUIRED、High／T4；本次不是产品独立审查 PASS。

前两次 A 决定 a2a3b18（刷新列表证明与最终 500 例外）、82cceae（按需强刷与请求预算）已被用户撤回；本计划全面替换旧强刷执行清单。B 依开发流程显式调整旧测试语义，逐项映射见下节，不通过隐藏 skip 或删除历史失败获得通过。合同指针已同步，Developer 可在常规治理通过后按本计划实施。

## 实际缺口与允许范围

- `internal/openlist/absence.go` 仍依赖 FreshLister／ListFresh、真实驱动与祖先链，且目标 403 为 Unknown，不符合新合同。`probeFilteredTarget` 未在最后请求返回后检查 context；旧 B-1 的整轮取消问题仍需修正。
- `internal/openlist/openlist.go` 普通 List 使用 refresh:false，但 ListFresh／共享分页实现可每页传 true。Get 将传输、HTTP、应用错误和响应解析失败转为 error；新规则在源根／目标两个位置分别解释，无需辨别目标错误消息。
- `internal/openlist/absence_test.go` 仍断言每页强刷及目标权限拒绝保留；标签测试 `openlist_integration_test.go` 仍依赖强刷／祖先链及原结果字段，须随接口同步。
- `scripts/run_openlist_absence_integration.py` 已存在，固定 v4.2.7、源码 `3b28270a4066ea52943a69ecddd850d83f7e8bdc`、镜像 digest `sha256:11b490123d57f88c84ecda46285e1921869cf511da3e5bb4062f183e7e1200b7`。wait_listed、分页／权限／隐藏／虚拟挂载等辅助请求仍有 refresh:true，不能只改 Go 客户端。缓存场景目前 TTL 60 分钟且强制要求刷新后的立即收敛，须重新设计。
- `internal/reconciler/syncer.go` 已有源根 Get、普通 List、有界 walker、失败不提交完整 round 及多 source 隔离；`internal/config` 仍拒绝 cleanup.enabled:true，继续保持。无当前缺失计数调用点，不添加占位 Store 接口或虚假清理入口。

允许修改 internal/openlist 的判定／列表接口和相关测试，必要时做 internal/reconciler 的小型适配／回归；允许调整既有专用真实运行器、Python 回归、环境说明与文件登记。原则上不改依赖。所有夹具和运行产物仅位于 artifacts，不接管生产 OpenList、Jellyfin、代理、配套仓库或生产媒体。

## Developer 实施清单

### 1. 以两次 Get 替换强刷链

1. 复用现有绝对路径、严格后代、逐段名称／长度／深度校验；非法输入、根本身、相邻前缀、穿越等在任何请求前拒绝。不从 STRM 名字或 URL 猜远端路径。
2. 保留零值不构成缺失、nil 依赖无法判定、有限脱敏原因码及无跨调用状态。可调整现有 AbsenceResult／状态名；明确“未读到”是业务观察，不能沿用声称权威证明真实不存在的注释、字段或日志。同步所有实际调用者及标签测试，移除无用途祖先链／强刷预算字段，不另造状态机。
3. 有效调用先 Get 配置源根。error、nil／无效结果、非目录均为源根不可读，不请求目标；不再按 Virtual／provider:unknown 否决一个已成功读取的目录，不额外 Get 中间目录。
4. 源根可读后直接 Get 精确目标；成功读到有效对象为存在／保留，目录或类型冲突也保留。返回 error 或无可用对象则为目标未读到，包含 HTTP 404／403／500、JSON 应用错误、传输／读取失败、单请求超时、畸形／缺失数据等，不检查 Detail／message，也不要求错误必须是 APIError。
5. 每次请求前及返回后检查调用方 ctx.Err()；整次调用取消／deadline 优先返回中止／Unknown，不返回缺失。区别于父 context 仍有效的 HTTP 客户端单请求超时，后者发生在目标阶段按未读到，发生在源根阶段则源根不可读。使用确定性屏障分别覆盖根与目标阶段，不依赖易抖动睡眠。
6. 单次合法调用至多两次 Get；确认器不发 List，不为证明不存在追加父目录／真实驱动探测。不跨调用缓存结果，不为全部文件创建 goroutine。后续计数／删除接入只针对正常对账未观察到的已有归属路径调用；已观察或 Get 可读的目标不累积缺失。

### 2. 全部列表关闭强制刷新

1. 普通 List／全部分页固定显式 refresh:false，保留现有 30s 请求超时、8 MiB 响应、200 项分页、10,000 项／2 MiB 名称、total／重复名／结构校验及取消退出。保留实测空目录仅第一页面 content:null、total:0 的精确接受规则；缺字段或 null 且 total>0 等形状仍拒绝。
2. 删除 ListFresh／FreshLister 等无用强刷接口，或仅在确有内部兼容需要时转为普通 List 且注明旧名废弃；任何可调用路径都不能发送 refresh:true，不保留强刷开关或回退。IsNotFound 错误文本判断不能成为新的 Get 规则分支。
3. 真实运行器的准备、等待、取证与测试请求也全部 refresh:false；不得为让断言通过在夹具幕后强刷、清缓存或修改生产服务。保留固定镜像／版本核验和自有资源生命周期。

### 3. 调整真实服务场景与工具回归

1. 复用现有 --test-binary／--output-dir／--scenario 入口及 openlist_integration 标签。仍由 Go 产品判定实现给出结果；Python 只建环境、改变专用文件、采集原始响应与运行测试，不重写业务算法。
2. Local 驱动覆盖精确名称／特殊字符、存在／未读到、源根不可读与源内父目录删除；分页与空目录改用普通 List。只读身份无需刷新权限即可执行新判定；隐藏项的目标拒绝按未读到，不能继续要求 Unknown。
3. 实际缓存驱动仍必跑，记录缓存预热、删除／新建、API 原始形状、时间顺序及 Go 判定。缓存还返回对象时预期可读；缓存漏项且 Get 也未读到时预期缺失观察。再出现断言以当次真实 Get 可读为前提，不假设修改后立即可见。采用固定短 TTL、有限等待／确定性阶段或新的隔离服务阶段证明缓存自然更新后的结果；记录实际参数，不能使用强刷／清缓存捷径或把尚未更新写成即时收敛 PASS。
4. 保留隐藏／嵌套挂载吞错的真实原始反例，按两次 Get 的实际结果验证新规则；源根若成功返回目录，不再因虚拟视图添加保留条件，目标若未读到则为缺失观察。它们属于用户接受的局限，不能称已安全识别或修复。
5. 覆盖客户端和 Python 辅助的请求轨迹，断言所有 list 的 refresh 字段显式为 false，判定器只有根／目标 Get，无祖先或强刷请求。工具回归继续覆盖证据目录边界、参数、子进程失败、缺环境 BLOCKED、仅停止自有容器、默认场景全覆盖及无意外 skip；调整场景名须同步 SCENARIOS、GO_TESTS、测试与说明，保留旧新映射。
6. 保持 CLI 生成／更新／dry-run／互斥与旧恢复基线，不开放清理。新候选的源码、测试、运行器和配置变化后重新跑完整 T4 与固定真实服务，不套用旧候选 PASS。

## 验收条件

| ID | 可核对结果 |
|---|---|
| AC-1 路径与请求 | 精确名称、中文／空格／#／%／大小写／相似前缀正确；非法路径与深度超限请求前拒绝。合法判定至多根、目标两次 Get，不查询祖先、不调用 List。 |
| AC-2 源根门槛 | 根 Get 的失败／nil／无效对象／非目录均保留且不查询目标；成功目录（含 Virtual 标记）可继续。源根被删与仅源内父目录被删得到不同结果；根 / 的后代合法。结果无持久写入。 |
| AC-3 目标规则与重读 | 根可读后，目标 Get 成功或类型冲突保留；未读到按相同缺失观察，覆盖错误码、传输／单请求超时及无效响应，不依赖错误类型或文本。每次调用重读，目标后来可读即存在；真实缓存滞后如实分类，不要求即时可见。 |
| AC-4 取消与默认值 | 根／目标返回后调用方已取消或 deadline 到期均中止，不返回缺失；目标阶段单请求超时但父 context 有效则未读到。零值、nil confirmer／依赖和非法输入不构成缺失；取消可退出，并发调用不串结果。 |
| AC-5 列表与隔离 | 全部列表及 Python 准备／取证均显式 refresh:false；多页／空目录成功形状仍正确，total 改变／重复名／不完整响应仍失败，有界限制保留。只改变专用夹具，不写 STRM／ownership／round／missing_count。 |
| AC-6 实证与基线 | 固定真实 Local＋缓存驱动、源根／父目录删除、只读／隐藏／虚拟视图及自然缓存变化证据齐全；新 T4 与旧 CLI 全套通过，清理仍拒绝。没有把接受的同码歧义或缓存滞后写成权威证明。 |

上述只验收远端判定与列表请求参数，不验收三轮计数、文件归属、本地删除或 Mirror 闭环。取消不提交轮次及旧源根／列表失败规则沿用 Reconciler 回归；未来真正接入计数／删除仍须独立验证。

## B 对旧测试与发现的显式调整

| 旧要求／发现 | 新映射及理由 |
|---|---|
| ListFresh 每页 refresh:true、祖先链及虚拟目录保守否决 | A 已撤回；改测确认器只作两次 Get、列表全程 false、路径校验和源根可读门槛。不保留强刷用例为必跑产品行为。 |
| 目标 HTTP／JSON 404／403／500、传输／超时一律 Unknown | 用户选择未读到计缺失；在根可读且父 context 有效时改为缺失观察。对应源根失败仍保留。真实隐藏与同码反例保留为已接受局限。 |
| B-1 吞最终 Get 的取消／超时 | 拆分：整调用取消／deadline 仍是待修问题，必须补目标阶段回归；父 context 有效的单请求超时／其他失败已按新合同纳入未读到，不伪称原问题全部修复。 |
| 最后 Get 成功但与刷新列表矛盾为 Unknown | 不再调用刷新列表；成功读到目标即保留／存在，不产生缺失观察。 |
| 缓存删除／再出现必须刷新并立即纠正 | 撤回即时刷新前提；验证实际缓存响应及自然更新后的重读，不删除缓存场景，也不跳过失败。 |
| 只有具备刷新权限才能确认、刷新拒绝必须 Unknown | 新合同不用刷新；改测只读身份可正常 Get／List(false)，目标拒绝与源根拒绝分别按新规则处理。 |
| 每目录保留内存／分页界限、并发隔离、零值保守、脱敏 | 保留；确认器不再持有目录证明，分页与限额在普通 List 中验证。 |

旧计划／Developer 完成报告／B 独立审查原文保留于 `57ea31b88a0dde95e486f8bac11ebe9e473f74ea:TASK.md`。旧实现为 `2fc3f6f06b6bc02ec9503e373cb031fc438f0346`，历史 B 审查范围 `a8c2d62369a21e8d9a0c2cefec2af09927fb3286..2fc3f6f06b6bc02ec9503e373cb031fc438f0346`、决定 FIX REQUIRED；这些历史结论不改成新合同 PASS。

## 必要测试

### 开发／修正反馈

工作目录 /opt/STRM-Generator；先治理，再聚焦新增／受影响行为。用请求轨迹与状态断言，不只比较原因字符串。

```bash
python3 scripts/check_governance.py
bash scripts/go-local test -count=1 ./internal/openlist ./internal/reconciler
python3 -m unittest discover -s tests -p 'test_*openlist*.py' -v
```

### 最终检查点门槛

完整 T4 保留。使用全新证据目录，不覆盖历史；对本 Task 的远端只读判定风险运行这些门槛，不表示实现删除。

```bash
mkdir -p artifacts/task-2.1/bin artifacts/validation-evidence
TASK_EVIDENCE="$(mktemp -d artifacts/validation-evidence/task-2.1-simple-get-final-XXXXXX)"
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

真实运行入口已存在，但须完成上述新规则改造后才运行，不执行旧强刷场景作为新证据。quality.py 覆盖全部 Python 回归、gofmt、Go vet／test／build；完整门槛覆盖同输入聚焦测试，不机械重复。发布二进制不得带测试环境开关。使用包装运行器保存每条命令 stdout／stderr、退出码、计数和耗时；真实环境缺失非零 BLOCKED，不以默认 Go 全绿替代真实服务。

### B 独立验证

审查完整 Task Base..新实现 Head，核对候选源码／二进制／夹具身份及原始证据。至少独立重放：源根 Get 失败不请求目标；Local 父目录删除与源根删除的不同结果；根可读而目标权限／同码失败的新判定；目标阶段取消与单请求超时的区别；多页列表 false 及缓存自然更新重读。核对 Python 准备请求也无 refresh:true。完整 quality／race／双架构／旧 CLI 可在输入一致且证据充分时复用 Developer 新候选原始日志，出现疑点按影响重跑。

PASS 后逐项重评 Slice RO-1..RO-6，不以本 Task 判定能力替代配置入口、完整轮／三次计数、本地归属／删除、移动、迁移／恢复与预览。它们尚未全部交付，不提交 Closure Packet，不宣布 Slice PASS／关闭。

### 证据与重跑

索引包含完整源码 SHA、未提交输入摘要、命令／cwd、Go／CC、固定服务源码 SHA／镜像 digest／版本、驱动与缓存 TTL／身份配置、专用文件变化与请求顺序、脱敏 HTTP／应用码及结果、测试数／skip／退出码／耗时。路径用夹具标签；token、Cookie、签名 URL、原始错误正文或宿主私有完整路径不入日志。证据保存在 artifacts/validation-evidence/<new-run-id>/，不覆盖旧 run，不将本机忽略目录说成远端可读证据。

旧 Developer `artifacts/validation-evidence/task-2.1-final-ftETbR/`、旧 B `artifacts/validation-evidence/task-2.1-b-review-20261018/` 保留历史；新规则改了源码／测试／运行器，须重跑完整 T4 与固定真实服务。纯文档后继可核对可执行输入一致后复用，但重跑治理／空白检查。合成 Go／Python、真实 Get／缓存、旧 CLI／恢复、编译／race 分开报告；真实 /d/、插件解析、Jellyfin／代理、ARM64 实机与规模仍 NOT RUN。保留首次失败与重跑。

## 非目标

不开放 cleanup.enabled:true；不累计缺失计数、不迁移 SQLite、不删除 STRM／ownership，不增加 Mirror 清理／移动／预览入口。三次阈值、文件归属与本地边界由后续接入落实，本 Task 不修改已有保护。无强刷、祖先权威证明、500 特例、目录证明复用／请求预算配置、远端版本锁、每文件生成前查询、媒体指纹／发现状态机、强制管理员／无嵌套挂载等部署新门槛。serve、发布、播放、规模及权限加固不属本 Task。

## Developer Completion Report

按 B 调整后的新合同原地修正并交付；旧强刷实现与旧证据仅为历史，不构成本 Task 通过。

- Changed Files / Implemented：
  - `internal/openlist/absence.go`：`Confirmer` 改为至多两次精确 Get（源根、目标），
    状态为 present／missing／unknown，原因码有界且不含私密信息；每次请求前后检查调用方
    context，整次取消／deadline 优先中止，父 context 有效的单请求超时按阶段归入源根不可读
    或目标未读到。新增导出的 `openlist.CheckSourceRoot` 作为唯一源根门槛。
  - `internal/openlist/openlist.go`：删除 `ListFresh` 与 refresh 开关、`IsPermissionDenied`／
    `StatusCode`、`Entry.Virtual` 与 Get 的 provider 否决；`List` 每页显式 `refresh:false`；
    `IsNotFound` 保持废弃且无调用。
  - `internal/reconciler/syncer.go`：源轮次改用同一 `CheckSourceRoot` 门槛（失败保留、取消
    区分），未接入计数或删除。
  - 测试与运行器：重写 `absence_test.go`（路径与深度、源根全部门槛变体含虚拟视图、目标
    各类错误／超时／无效响应／名称不符、取消屏障、重读、并发、读取上限与列表轨迹断言）、
    `openlist_integration_test.go`、`scripts/run_openlist_absence_integration.py` 与
    `tests/test_run_openlist_absence_integration.py`（场景 `refresh_permission` 改为
    `read_only_identity`，缓存改短 TTL 自然到期，全部准备／轮询／取证列表显式 `refresh:false`）。
  - 文档：`docs/development-environment.md`（新场景与实测语义）、`docs/architecture.md`
    CURRENT、`docs/test-matrix.md` 状态、`docs/compatibility.md` 实证层。未新增文件，
    `dev/environment-spec.json` 无需变更。
- Tests and Results / Evidence：最终检查点 14 项门禁全部 exit 0：治理、`mod download`／
  `verify`、`quality.py`（Python 81 项、gofmt、vet、test、build）、独立 gofmt、race
  `-race ./...`（244 测试／8 包，0 fail／skip）、`-count=1 ./...`、Linux amd64／arm64 构建、
  testhooks 构建、隔离 CLI 9/9、集成测试二进制、固定隔离 OpenList v4.2.7 全 8 场景
  （11 个 Go 集成测试，0 skip）、`git diff --check`。证据索引
  `artifacts/validation-evidence/task-2.1-simple-get-final-m8SE4t/INDEX.md`（含门禁退出码／
  耗时、二进制 SHA-256、源码清单摘要与真实服务原始观测）；实现检查点提交后逐字核对文件
  哈希一致。开发期间首轮真实服务失败（removed_parent、nested_mount_masking 的夹具预期与
  实测驱动行为不符）保留在早期 probe 目录，按实测探测后修正夹具与断言；详细驱动事实见
  INDEX.md 观测清单，不是产品缺陷粉饰。
- Decisions / Risks / Remaining Required Outcomes：真实固定版本实测并作为用户接受局限
  记录：Local 挂载后备目录被删后其挂载根仍读取为目录（该源继续、目标未读到计缺失）；禁用
  存储的挂载根不可读（保留）；WebDav 自挂载缓存自然到期后目录可读但文件 Get 始终不可读
  （受限身份隐藏项同码 403）；嵌套挂载虚拟祖先可读并掩盖父驱动故障。判定器不发列表、不查
  祖先、不跨调用缓存；`cleanup.enabled: true` 仍在任何写入前被拒绝。本 Task 只交付远端一次
  判定：缺失计数、完整轮、本地归属／删除、移动、旧库迁移、dry-run 预览与清理恢复未交付，
  RO-2／RO-3 删除部分／RO-4／RO-5／RO-6 预览仍未满足。真实 `/d/` 播放、固定插件实际解析、
  Jellyfin／代理、ARM64 实机与规模仍 NOT RUN。未自签 B／A 产品 PASS。
- Head SHA：实现检查点 `cec07017113c9b8e2d313cc592cc22d98011b097`；门禁运行后再无产品、
  测试、运行器或文档输入变化，本节仅更新 Task 状态与报告。
- Evidence Run：`artifacts/validation-evidence/task-2.1-simple-get-final-m8SE4t/`（本机
  Git 忽略路径，不是远端可读证据）。

## B Review Result

```text
Reviewed: a8c2d62369a21e8d9a0c2cefec2af09927fb3286..cec07017113c9b8e2d313cc592cc22d98011b097
Docs-only Handoff: 8263f5d45325d3d4c5da29d04e8a53c5fa435a27 (TASK.md only)
Decision: PASS
P0/P1 Blockers: NONE FOUND within Task 2.1
Contract Adaptation: Task 2.1 accepted under 57ea31b88a0dde95e486f8bac11ebe9e473f74ea
Slice Required Outcomes all satisfied: NO
Next: B plans the remaining complete Mirror/cleanup/recovery behavior within Slice 2
```

2026-10-19 B 独立审查完整 Task Base..实现 Head，包含旧实现及其后新合同修正；核对入口
工作区干净、治理通过及报告后继仅改 TASK.md。没有以旧强刷候选或旧失败记录代替新合同验收。

- **独立验证：** openlist／reconciler 未缓存回归 169 测试／2 包通过；Python 81 项通过；
  重建集成测试二进制并运行固定 v4.2.7 Local／WebDav 全 8 场景、11 测试，0 fail／skip。
  缓存 TTL 1 分钟，61 次普通列表轮询、60.2 秒后自然更新。全部准备、取证和分页列表 false。
  另用仅位于 artifacts 的 Go overlay 独立通过 2 项检查：第一页 200 项后第二页 null／total:0
  必须拒绝；根请求返回时 deadline 已结束必须中止且不查目标；普通非 APIError 的目标失败
  在父 context 有效时按未读到处理。未修改产品实现或已提交测试。
- **证据复用：** Developer 新候选 12 项源码／文档及 4 个二进制 SHA-256 逐项一致，
  cec0701..8263f5d 仅 TASK.md 改变。复用新候选依赖校验、quality、race（244 测试／8 包，
  0 fail／skip）、未缓存全包、双架构／testhooks 构建及旧 CLI 9/9 原始证据；实际 gates.tsv
  为 14 项 exit 0，B 据此纠正上文报告的 13 项笔误。没有因角色不同机械重复完整门槛。
- **AC-1..AC-6：PASS。** 路径与两次 Get、源根保护、各种目标未读到、无跨调用状态、
  取消与单请求超时、列表结构／界限和真实服务语义均满足。WebDav 目录缓存自然更新成立，
  文件 Get 在更新前后仍未读到；没有将它写成文件再出现可读或权威识别成功。隐藏项拒绝、
  后备目录已删但挂载根可读、虚拟祖先掩盖父驱动故障，按已接受规则分类并保留局限。
- **RO 全量重评：** RO-1 未满足（清理仍拒绝、开关及阈值未接入）；RO-2 部分满足
  （根／取消／遍历门槛已有，连续缺失未接入）；RO-3 部分满足（远端规则完成，本地删除
  未交付）；RO-4 未满足（旧路径退出未交付）；RO-5 未满足（精确映射、旧库迁移和清理
  恢复未交付）；RO-6 部分满足（固定服务实证完成，清理预览／摘要未交付）。不交 Closure
  Packet，不改 Slice ACTIVE 或替 A 签最终 PASS。

本机证据索引：`artifacts/validation-evidence/task-2.1-b-simple-get-20261019/INDEX.md`，
包含精确命令／cwd、退出码／耗时、原始日志、复用哈希、独立 overlay 与 RO 映射；真实服务
版本及原始响应见子目录 `real-openlist/summary.json`。旧首次失败及 Developer 新候选证据继续
保留。本轮没有生产服务／媒体操作。真实 /d/、固定插件实际解析、Jellyfin／代理、ARM64 实机
和规模 NOT RUN。证据仅在本机 Git 忽略目录，不能声称远端可读。
