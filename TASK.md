# Task 1.1 — 首次生成、归属登记与安全重复运行

```text
Task ID: 1.1
Parent Slice: 1
Status: READY FOR B REVIEW
Task Base: 951aaeeeca966e8ef79e8ac9500827ce27d0f035
Contract Checkpoint: 951aaeeeca966e8ef79e8ac9500827ce27d0f035
Difficulty: High
Test Level: T4
Planner / Reviewer: B
```

范围 authority：已提交的 `951aaeeeca966e8ef79e8ac9500827ce27d0f035:SLICE.md`。Slice Base 仍为 `f90f9560e55b07fd2d6a20e8bac43741c39176c4`，本 Task 不改合同、路线图或稳定需求。

## Goal

实现 `strmgen sync --config <file>` 与 `strmgen sync --config <file> --dry-run` 的首次生成闭环：读取配置和 OpenList，镜像目标路径，原子创建版本 1 STRM，登记 SQLite 归属，重复运行跳过已拥有且未变化的文件；所有失败给出真实退出状态，预览不修改产品文件或数据库。

对应 RO-1／RO-2／RO-3 的基础入口，以及 RO-4／RO-5 的首次创建、比较、最小归属、互斥和预览部分。本 Task 不完成已拥有文件的内容替换，也不代表全部 Slice Outcomes 已满足。

## 已核查的代码、测试与缺口

- Task Base 工作区干净；规划前常规治理 PASS，ACTIVE 合同已提交。
- `cmd/strmgen/` 和六个 internal 目录只有占位；没有 Go 产品代码、SQLite schema、YAML 依赖或 Go 产品测试，go.mod 仅固定本地 module 和 Go 1.27.2。
- 可复用 `scripts/go-local`、`scripts/development_environment.py`、`scripts/quality.py`、现有 CI 和 29 项 Python 工具测试；不能用编译 smoke 代替产品测试。
- `scripts/mock_openlist.py`／`make_server(port=0, fixture=...)` 已提供可编程 list/get 服务；`dev/fixtures/openlist.json` 有四个合格媒体文件、非媒体、子目录、特殊名称、缺失／无效哈希及 Int64 Size。get.raw_url 故意带假签名。
- 现有夹具尚不覆盖延迟屏障、分页中途失败、重复页、目标冲突、进程中断和本地故障；在其基础上增加测试能力，不另造产品 API 服务。
- 本机未检出 gcc／cc。需要 race 验证时先在仓库忽略目录或专用隔离构建容器准备 C 工具链；不能因此将 race 写成 PASS。

## 允许修改的范围

`cmd/strmgen`、`internal/openlist`／`pathmap`／`strm`／`storage`／`reconciler` 及必要的配置／同步入口包；go.mod／go.sum；Go 测试；现有夹具和对应 Python 回归；新建隔离同步验收驱动及其回归；必要的质量与开发环境工具。

可以同步配置示例、README、架构 CURRENT、环境和进度的实现事实，维护新增必要文件清单。不修改原方案、SLICE.md、Roadmap、稳定需求、测试等级或验收边界；不借文档同步宣称未实现能力。

## 实施步骤

以下是同一个 Task 内的工作顺序，不拆成独立 Task。先完成前一步的聚焦验证，再接下一步。

### 1. 固定依赖和 CLI 边界

1. 使用现有 Go 1.27.2 与 wrapper。选择能在 Linux amd64／arm64、CGO=0 下构建的 SQLite 驱动和 YAML 库，固定版本并提交 go.sum，记录官方来源及许可证；不以实现方便为由换数据库或升级工具链。
2. 建立可注入配置、HTTP 客户端、输出和退出码的 CLI 入口。约定：成功为 0；配置／参数／尚不支持的命令为 2；API、文件、数据库、冲突或忙碌为 1。取消也必须非零，不打印成功摘要。
3. `serve` 和未知命令明确拒绝，不能返回成功或假装已定时运行。帮助信息只列实际可用能力。

### 2. 在产生输出前校验配置

1. 支持现有 YAML 字段，拒绝未知字段及格式错误；相对文件路径以启动工作目录为准。token 只从 `openlist.token_env` 指定环境变量读取，缺失时报错，不将值写入日志／数据库。
2. 校验 API／public URL 为有效 HTTP(S)，拒绝用户信息、签名／任意查询串和 fragment；保留合法路径前缀。remote 必须是合法绝对远端路径；不能用清理路径的方式隐藏 `..` 等非法输入。
3. 将输出根规范化，检查重复、祖先／子目录重叠和已存在符号链接。数据库不放进输出根。不同来源不能映射到同一输出范围。
4. `atomic_write`、`skip_unchanged` 保持固定安全行为；不接受关闭这些不变量的配置。本 Task 不删除：配置示例改为 `cleanup.enabled: false`；请求启用清理时在任何同步写入前报“不支持清理”。interval 等未来字段可解析，但不能启动后台任务。
5. 初始媒体扩展名固定为 `.mkv .mp4 .m4v .avi .mov .wmv .webm .ts .m2ts .mpg .mpeg`，扩展名比较忽略大小写；其他文件计为过滤，不生成 STRM。文件名主体及目录名保持原大小写。

### 3. 复用夹具验证 OpenList 客户端与目录遍历

1. 实现带 context、超时和响应字节上限的 list/get。同时检查 HTTP 状态、外层 code 和必要 data 形状；HTTP 200/code 500 也必须失败。禁止把错误或 null data 转为空目录。
2. get 可用于读取源根／必要文件元数据；list 按显式有限 page/per_page 遍历。已有足够元数据时不在每次写入前再 get，不下载媒体补算哈希。
3. Size 用可区分“缺失”和零的 Int64 语义。`hash_info` 优先；只有其缺失／null 时才尝试解码历史 `hashinfo` JSON 字符串。只接受有效内容 sha1，缺失／无效可选元数据省略并记录受限原因，不伪造零值或拼接旧哈希。
4. 对 total、分页结束、提前空页、重复条目／页、非法文件名及响应超限给出有界处理与明确错误。分页中途失败或 total 无法一致判读时，不标记该源完整成功；扫描期间变化由下一轮重新读取。
5. 使用配置的有限目录工作数、有限 HTTP 响应和有界队列；交付说明写出实际容量及限制。禁止全库完整对象列表、每文件 goroutine，或者所有消费者同时阻塞向满队列投递的遍历方式。用宽／深目录夹具验证完成或明确失败，不能无限等待。

### 4. 完成纯路径／协议与比较

1. 分别实现本地相对路径映射、稳定 URL 和版本 1 STRM 序列化／解析；纯转换不访问 HTTP／SQLite。URL 逐段编码中文、空格、`#`、`%` 等字符，保留 public_url 前缀和 `/d/` 分隔，不复制 raw_url／sign。
2. 输出 UTF-8、LF、单个 version:1、有效 HTTP(S) URL、有效可选 SHA-1／Size；总长不超过 65,536 字节。缺字段省略，Size=0 保留，SHA-1 小写，不写 Modified。
3. 比较遵循稳定需求中的完整优先级：URL → 两侧有效 SHA-1 → 两侧有效 Size → 信息不足。相同 SHA-1、Size 不同应跳过并报告异常；不能仅比较新旧序列化字节。
4. 对同一目标 `.strm` 的不同输入（如 A.mkv／A.mp4）先确认冲突，再决定是否写该目标；不得按到达顺序选一个。冲突检测也须有界，可使用仅保存路径唯一键的临时索引；不保存独立媒体版本状态。其他目标已完成的创建可以保留，但冲突目标不得成为任意一个输入的成功产物。

### 5. 先落实互斥与最小归属，再接实际创建

1. 同一输出范围的手动进程不得重叠，包括配置文件或 database.path 不同、路径别名和祖先／子目录范围的情况。锁在读取归属／执行本轮前获得，覆盖完整本轮，进程退出后能再次获取；本阶段允许保守串行化独立同步。
2. dry-run 也不能绕过共同互斥入口。锁资源使用输出和数据库以外的运行时位置／内核资源；不能为加锁创建输出目录、数据库或 WAL／SHM。不要通过每轮删除正在使用的锁文件制造不同锁 inode。
3. SQLite 仅持久化可信归属／源身份、路径映射、最近完整成功轮次和必要缺失计数；本 Task 不推进缺失清理计数。不保存 URL／SHA-1／Size 的独立版本表、媒体状态枚举或永久生成计划。
4. 明确区分：目标不存在且没有可靠旧归属；已拥有且比较结果不变；目标存在但未拥有；已拥有但需要内容变化／文件不可解析。不能仅凭“文件可被插件解析”认领它。
5. 对不存在目标，采用同目录临时文件、完整写入和原子可见发布；发布必须拒绝覆盖并发出现的目标，不能用“先 exists 检查，再覆盖式 rename”代替。每次操作保持在已验证输出根内，拒绝符号链接和路径逃逸。
6. 新文件成功发布后登记其可靠归属。禁止预先认领一个尚未成功发布的路径；若登记失败，保留文件、非零退出，不把它算已拥有。重启遇到该未登记文件应保持未归属冲突，不能为了自动恢复强行接管。记录可供管理员备份／恢复后处理的诊断。
7. 已拥有且比较不变：不写文件，不改 mtime；相同哈希的 Size 异常只记诊断。已拥有但需更新：本 Task 明确报“需要更新，当前未执行”，非零退出、保留原文件，不伪报 skip／success；后续连贯 Task 承接内容替换。
8. 每个 source 只有完整遍历、必要处理及归属登记成功后才能提交其完整轮次；失败／取消／预览不提交。允许失败前已成功创建并登记的文件保留，必须报告部分结果；本 Task 没有任何删除分支。

### 6. 将 dry-run 接到相同决策流程

1. 同配置、相同远端／本地事实下复用过滤、路径、比较、冲突和错误判断，输出预计创建／跳过／冲突／需更新的脱敏摘要；不能以 dry-run 为理由放过真实同步会拒绝的输入。
2. 无数据库时按没有可证明归属处理，不创建数据库。已有数据库只读打开；不得创建或修改 DB、journal、WAL、SHM、输出目录／文件、完整轮次或缺失次数。不能用忽略已有 WAL 的方式获得错误归属视图；无法安全只读时保守失败。
3. 输出根或数据库父目录原本不存在时仍可预览，不先建目录。测试至少比对目录项、文件字节／mtime 和 SQL 记录；不要只验证“没有 .strm”。
4. stdout／stderr 使用可复核计数和 source 序号／相对项标识；错误也不回显 token、Authorization、原样服务错误正文、签名 URL 或私有完整路径。用户可区分过滤、不变、异常、未执行更新、冲突和部分失败。

### 7. 接入实际二进制隔离验收并交付

1. 新建 `scripts/run_isolated_sync_integration.py`，接受 `--binary`、`--scenario all`、`--output-dir`。复用现有夹具，在动态端口启动本次自有服务；仅使用输出目录内的配置、STRM、数据库、运行时锁和证据。
2. 驱动必须通过构建后的 CLI 子进程检查退出码、实际 STRM、SQL 行、文件快照和日志，而非仅调用 Go 内部函数。运行后关闭自有服务与进程；不接管本机已运行的 15244 夹具，不读取生产 token／挂载。
3. 为驱动的路径边界、失败保留和清理自有子进程增加必要 Python 回归；可增强原夹具，不另写一个与产品相同的生成器来当预期结果。
4. 同步实现事实与准确操作／恢复文档，维护现有质量入口和文件清单。提交实际实现检查点后再填写 Developer 报告的真实 Head；保留本 Task ID／Base／Goal／Scope，交 READY FOR B REVIEW。

## 验收条件

- [ ] **AC-1 首次生成**：对现有合成树使用临时配置，预览得到四个媒体候选且不创建输出／数据库；实际 sync 创建恰好四个合法 STRM，过滤 notes.txt，镜像子目录；SQL 有四个可靠归属，完整成功轮次真实提交。
- [ ] **AC-2 重复与比较**：再次同步四个文件的字节／mtime 不变；相同有效 SHA-1 但 Size 变动报告异常且不重写；URL／有效哈希／不可比情况下有效 Size 变化均报告尚未执行更新并非零退出，不修改旧文件。元数据不足不虚构变化。
- [ ] **AC-3 归属与路径**：预置陌生 STRM、无效既有文件、同名扩展名目标、重叠根、穿越及符号链接时明确拒绝，保护已有文件及根外哨兵文件；跨分页冲突也不得任意选一个输入成功生成。
- [ ] **AC-4 API 与限额**：HTTP 200/code 500、HTTP 503、401／403、超时、无效必要字段、提前空页、重复页和超限响应均非零且不提交失败源完整轮次；合法空目录与失败能区分。宽／深目录、有限队列可结束，处理不丢项。
- [ ] **AC-5 dry-run**：在无输出／无库、已有归属、冲突、需更新及失败情形中检查完整副作用边界；不改 SQL 行／轮次／计数，不创建 WAL／SHM／输出目录，无预览结果被下轮当作提交依据。
- [ ] **AC-6 多进程**：用受控 HTTP 屏障保持第一轮执行，第二个实际二进制在相同、别名、祖先／子目录输出范围及不同 DB 配置下不能同时运行或修改其 DB／输出；第一进程正常退出或被终止后能再次运行。不能只测试同一进程的 mutex。
- [ ] **AC-7 故障恢复**：临时写入失败／发布失败、外部目标恰好出现、文件已发布但归属提交失败、归属成功后退出、分页中断／取消均有实测；不留下半个目标、不覆盖陌生文件、不制造成功轮次，重启后按可信事实跳过、重试或保持冲突，无盲目认领。
- [ ] **AC-8 交付事实**：锁依赖与 go.sum、质量检查、全部回归、race、两种 Linux 架构构建及隔离 CLI 全场景有匹配证据；日志脱敏、准确的部分失败计数与退出码可复核。产品未实现／未验证的部分明确列出，不标作 Slice PASS。

## 必要测试

以下新增包、驱动和产物是 Developer 应创建的目标，不代表规划时已经存在或通过。

### 开发／修正反馈

先完成新增行为和已知失败，按改动包运行 `bash scripts/go-local test ./internal/<已创建包>`；接入 CLI 后跑相关临时根／HTTP／数据库集成。先验证失败和恢复，再运行昂贵全量门槛。没有匹配用例或零测试不能写 PASS。

### 最终检查点门槛

本 Task 跨文件发布、持久化与多进程，T4 要求完整现有回归和实际 CLI 验收。仓库根目录执行：

```bash
python3 scripts/check_governance.py
bash scripts/go-local mod download
bash scripts/go-local mod verify
python3 scripts/quality.py
CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 -json ./...
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 bash scripts/go-local build -trimpath -o artifacts/task-1.1/bin/strmgen-amd64 ./cmd/strmgen
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 bash scripts/go-local build -trimpath -o artifacts/task-1.1/bin/strmgen-arm64 ./cmd/strmgen
python3 scripts/run_isolated_sync_integration.py --binary artifacts/task-1.1/bin/strmgen-amd64 --scenario all --output-dir artifacts/validation-evidence/task-1.1-final
git diff --check
```

`quality.py` 在产品包出现后已执行全部 Go test／vet／build／gofmt 检查及 Python 回归，不另重复一次相同的非 race 全套。允许改进其结构化日志输出，不能减少门槛。race 使用独立运行模式，不被普通测试替代。

上面的 CC 是要求准备的本地工具链入口，当前不存在；Developer 需提供固定来源／版本的可执行入口并记录设置。若选隔离构建容器，先将等效的精确命令、固定镜像／挂载与结果提交 B 核准，保持同一风险门槛；不能擅自跳过。ARM64 构建成功只报告交叉编译，不宣称 ARM64 实机通过。

### B 独立验证

B 审查 Task Base..Developer Head 的全部实际差异，核对锁文件／配置、原始日志和实测产物。独立运行实际 CLI，至少复现首次／重复／dry-run、已有陌生文件／跨页冲突、API 失败不提交完整轮次、跨进程排他及“发布后登记失败再启动”分支，逐项比对文件和 SQL；需更新的文件必须保留并明确失败。

完整 quality／race／构建证据在输入一致且可核验时复用；缺原始日志、候选不匹配、未覆盖关键故障或有矛盾时重跑相关门槛。B 不能凭四个新文件或一条 PASS 摘要接受持久化／恢复。

### 证据与重跑

每次运行使用新的证据子目录，首次失败不覆盖；驱动的 output-dir 不得指向源码或已有证据目录。索引记录完整源码 SHA、未提交输入摘要、精确命令／cwd、SDK／C工具链／依赖身份、配置／夹具摘要、退出码、测试计数／skip／耗时、日志与二进制 SHA-256、对应 AC、复用与缺失门槛。本机证据不可宣称远程可读。

最终候选改变源码、测试、工具、依赖或运行配置后重跑对应完整 T4 指定门槛；纯报告后继可核对可执行输入并复用，仍重跑消费文档的治理检查。必要门槛不可用就记录 BLOCKED／NOT RUN，不能以 mock、编译 smoke 或跳过结果代替。

## 非目标与剩余 Slice Outcomes

- 不替换已拥有 STRM 的内容，不强行认领陌生文件；需更新时保留并明确失败。后续 Task 实现真实变化更新、原子替换及其完整恢复证据；这仍属于 Slice 1 RO-4 的未完成部分。
- 不实现 Mirror 删除／移动旧路径清理、serve／定时、发布包／Dockerfile、百万级或512MB性能承诺。
- 不运行 ffprobe、不下载媒体计算哈希、不修改 Jellyfin／代理或其数据库。
- 不把 Python 合成 API 验收算作真实 OpenList、稳定 `/d/` 可访问性或固定插件解析实证。Slice Final 前仍须补足合同要求的真实证据，不能在本 Task 报告中清零这些缺口；真实 Jellyfin 播放仍不在本 Slice 范围。
- B 在本 Task PASS 后重新核对全部 RO，再决定下一项完整 Task，不预填后续 Task 或 A 最终结论。

## Developer Completion Report — Original Candidate (Historical)

- Changed Files：`cmd/strmgen/main.go`（+`main_test.go`）；`internal/config/config.go`（+测试）；`internal/openlist/openlist.go`（+测试）；`internal/pathmap/pathmap.go`（+测试）；`internal/strm/strm.go`（+测试）；`internal/storage/storage.go`（+测试）；`internal/reconciler/{syncer,fs,lock,lock_unix,lock_other}.go`（+`syncer_test.go`、`fs_test.go`）；`go.mod`／`go.sum`；`config.example.yaml`；`internal/README.md`；`scripts/development_environment.py`（`install-cc`）；`scripts/mock_openlist.py`（可编程 hooks）；`scripts/run_isolated_sync_integration.py`（新）；`tests/test_mock_openlist.py`、`tests/test_run_isolated_sync_integration.py`（新）；`dev/environment-spec.json`（cc 身份与新增文件清单）；`.github/workflows/quality.yml`；`README.md`、`docs/architecture.md`、`docs/development-environment.md`、`docs/progress.md`。
- Implemented：`strmgen sync --config <file>` 与 `--dry-run`；严格 YAML（未知字段拒绝、token 仅来自环境变量、URL 拒绝 userinfo／query／fragment、remote 拒绝 `..`、输出根重叠／重复／符号链接拒绝、数据库不入输出根、cleanup 启用拒绝）；有界 OpenList list/get（显式分页、HTTP 状态／外层 code／data 形状、响应字节上限、total 一致性、提前空页／重复条目拒绝、`hash_info` 优先与 `hashinfo` 回退、缺失／无效哈希省略）；纯路径映射与稳定 `/d/` URL 逐段编码；版本 1 协议解析／序列化与 URL→SHA-1→Size 比较；SQLite 最小归属与完整轮次（dry-run 对 WAL 库读私有快照，不创建 DB／WAL／SHM）；有界 BFS 目录遍历（仅排队目录路径、队列上限、无每文件 goroutine）；同目录目标冲突拒绝；同目录临时文件 + `os.Link` 不覆盖发布；跨进程 flock 单轮互斥；需更新时保留原文件并非零退出；退出码 0／1／2。
- Tests and Results / Evidence：证据目录 `artifacts/validation-evidence/task-1.1-final/`（本机，非远程可读）。治理 PASS；`mod download`／`mod verify` PASS；`quality.py`（gofmt／vet／Go test／build＋Python 39 项回归）PASS；`-race -count=1 ./...` PASS（test-level pass=86，fail=0，skip=0）；Linux amd64／arm64 CGO=0 `-trimpath` 构建 PASS（SHA-256 amd64 `4141a8…`、arm64 `42b271…`）；`run_isolated_sync_integration.py --scenario all` 7/7 场景 PASS（首次、重复与比较、归属与路径、API 与限额、dry-run 边界、多进程、故障恢复）；`git diff --check` PASS。Go 依赖固定 `gopkg.in/yaml.v3 v3.0.1` 与 `modernc.org/sqlite v1.60.1`；race 使用仓库固定 Zig `cc`（SHA-256 见 `dev/environment-spec.json`）。
- Decisions / Risks / Deviations：SQLite 选 `modernc.org/sqlite`（纯 Go，CGO=0 可跨架构，无系统 SQLite 依赖）；同目标冲突按单目录 listing 检测，因此天然覆盖同一分页内与分页边界上的 `A.mkv`／`A.mp4`（同目录必在同一 listing）；跨进程互斥本 Task 保守为全局单轮串行（强于按输出范围），通过 `STRMGEN_LOCK_DIR` 在测试／驱动中隔离命名空间；已拥有文件需更新时本 Task 只报告并非零退出，不执行内容替换（合同允许）。无超出范围的偏离。
- Remaining In-Slice Work：已拥有文件的内容替换与原子更新及其恢复证据（RO-4 未完成部分）；真实 OpenList 驱动／鉴权／无签名 `/d/` 实证与固定插件解析实证；`serve`／定时、Mirror 删除／移动清理、发布镜像、性能测量仍属后续 Slice／Task。
- Head SHA：`4752ec2057f998d762829cdb352c8b75198ee900`。

## B Review Result — Historical Candidate

```text
Reviewed: 951aaeeeca966e8ef79e8ac9500827ce27d0f035..4752ec2057f998d762829cdb352c8b75198ee900
Decision: FIX REQUIRED
Slice Required Outcomes all satisfied: NO
Next: DEVELOPER FIXES TASK 1.1 IN PLACE
```


### B 独立审查 — 2026-10-18

- 审查完整 Task Base..Developer Head，含计划与全部实施变更；报告／哈希修正文档后继核到本次起始 HEAD（完整 SHA 见证据 index.json），后继只改 SLICE Implementation Head 和 TASK 报告。B 不实施产品修正，不改 ID／Base／Goal／Scope／T4／Slice 合同。
- B 独立重跑现有隔离 CLI **7/7 PASS**：首次、重复／比较、陌生文件／跨页冲突、API 失败不提交轮次、dry-run、同锁命名空间多进程、发布后归属失败再启动。只说明现有用例通过，不能覆盖以下反例。
- 本机证据目录：artifacts/validation-evidence/task-1.1-b-review-20261018/；含 baseline/summary.json／baseline.log、probe_review.py／probes.json／probes.log、probe_additional.py／additional.json／additional.log、source-collision.json。probe 退出 0 表示采样完成，不是产品通过；全为隔离假服务，不是远程可读或真实 OpenList／插件证据。
- 核对旧 quality（39 Python＋Go test／vet／build）、race JSON（86 pass／0 fail／0 skip）及产物。B 未重跑完整 quality／race／构建，已有明确阻断，不能凭旧绿灯接受候选。产物 SHA-256 与报告相符；VCS stamp 是纯文档后继 ee3e20217675eebee6a135dce58b0d70ae100498、modified=false，可执行输入与实施 Head 一致。

### 必须在同一 Task 完成的修正

均为原有 AC／需求的缺陷，不新增 Task 或产品能力。先补能在当前候选失败的回归，再修正；保留已有断言，不得靠 skip、改合同或声称生产不会这样配置绕过。

#### F-1 / P1：输出根、祖先与物理边界（AC-3／AC-7，R-04／R-06）

锚点：internal/config/config.go:265-318 只 Lstat 最后一段；internal/reconciler/fs.go:29-60,127-161 不约束 root 自身／祖先。证据 ancestor_symlink、root_swapped_after_validation 均退出 0，在隔离根外哨兵各创建四个 STRM，记录四条归属及成功轮次。别名同时绕过输出重叠和 DB 不得入输出根校验。

1. 补祖先链接、配置通过后 HTTP 屏障期间根换成链接、词法独立但实际重叠根、DB 经别名入输出根测试；覆盖根存在／不存在。配置期已知不安全须在产品写入前失败。
2. 统一校验所有已有祖先、根自身和 DB 边界，不能只靠 Clean／词法 Rel／末段 Lstat。读／建目录／发布使用绑定已验证目录的受限根或相对目录句柄，抵抗检查后替换；不能只多加一次 Lstat 后继续可被替换的绝对路径操作。
3. 实际 CLI 断言根外目录项／字节／mtime 不变，失败源无假归属／成功轮次；保留目标恰好出现时不覆盖保障。

#### F-2 / P1：共同互斥与预览锁副作用（AC-5／AC-6，R-10／R-11）

锚点：internal/reconciler/lock.go:20-43 按环境、TMPDIR、UID 选择锁。lock_namespace_bypass 第一实际进程仍在屏障，第二实际进程同输出、不同 DB／锁目录退出 0 并创建四文件。dry_run_lock_inside_output 预览退出 0 却创建 out/locks/sync-0.lock。报告的全局串行保证不成立。

1. 生产入口选择不会被配置／每进程环境／DB／UID 分裂的协调机制；允许保守全局串行，同物理输出所有受支持运行方式须争用同一锁。测试隔离接口不能成为发布二进制旁路。
2. 任何 mkdir／锁文件创建前证明运行时位置在输出／DB／sidecar 外且祖先可靠；默认 TMPDIR 或 override 落入输出不得先写后查。不得每轮 unlink 活跃锁。
3. 补实际双进程同输出、真正祖先／子目录、别名、不同 DB／锁环境／TMPDIR、dry-run 对实际轮次。当前 driver 的 ancestor 是另一个不相交根，应修正；TestLockIsCrossProcessSafe 实为同进程两次调用，不能以名称代替进程证据。
4. 第二轮忙碌／安全拒绝且无 DB／输出变化；第一轮正常退出、SIGTERM、SIGKILL 后再获锁。预览锁位置误配保持目录项／SQL／文件快照不变。

#### F-3 / P1：source 归属与轮次被合并（AC-1／AC-2／AC-7，R-06／R-07）

锚点：cmd/strmgen/main.go:125-132 仅 remote 作 Key；internal/storage/storage.go:20-33,213-223 据此 upsert。同 /Movies 配两个合法独立根，首轮成功八文件却仅四条归属，第二轮第一个根四个冲突（source-collision.json）。改 API 服务但保留 remote／output／DB 也沿用旧源归属与轮次。

1. source 身份稳定绑定已校验 API 源、远端根与输出映射；不含 token，不按数组序号或 public_url 建身份。public_url 变化仍走既有 URL 更新判断。
2. 不同 API／输出不能覆盖彼此归属／round；sources 重排、同源重复、凭证轮换保持正确归属。不新增媒体版本表。
3. 明确旧 schema／remote 键恢复：不默认对新源可信。无法证明映射则非零并保留文件；安全迁移必须独立验证，不静默认领。
4. 两独立根首轮八文件／八归属／两轮次，二轮全跳过且字节／mtime 不变；换 API／源映射不借旧归属更新或推进别人的轮次。

#### F-4 / P1：数据库文件名被当作 URI（AC-1／AC-5／AC-7，R-06／R-07）

锚点：internal/storage/storage.go:55,92,160 直接拼 file:＋路径＋query。database_path_uri_interpretation 配 db?name.sqlite，退出 0，配置文件不存在而创建截断 db；预览随后看不到可靠归属。

1. 统一正确构造／编码文件 URI 与读写 DSN；问号、井号、百分号、空格不能注入 query／fragment／pragma。若拒绝不支持名称，须在产品写入前明确拒绝，不能静默换位置。
2. 测精确文件名、无替代文件、重复 sync／dry-run 同归属、正确 sidecar；联合 F-1 校验 DB 边界。错误不输出私有路径，不用临时 DB／自动认领补洞。

#### F-5 / P1：必要响应形状与可选 Size 混淆（AC-1／AC-4，R-01／R-02）

锚点：internal/openlist/openlist.go:133-151,206-245。missing_content_accepted_as_empty 仅 data={total:0}，产品退出 0 并提交 epoch。负值／溢出／小数／错误类型 Size 阻断合法媒体而非省略无效可选字段；零对照正常保留。

1. 区分 content 缺失／null／数组／错误类型；缺失失败，合法空数组成功。null 须有可核验 OpenList 证据支持，未证实不默认为空。root get 核对目录语义。
2. 将必要字段失败与可选元数据无效分开；保留 Size 原始形状，仅 0..MaxInt64 整数有效；缺失／null／无效值省略标记、报告受限原因，不伪造零、不阻断合法媒体；list／get 统一。
3. 逐项检查退出码、STRM、归属和 rounds：必要形状失败无成功轮；无效可选字段仍生成合法协议，重复遵守信息不足不推断变化。

#### F-6 / P1：脱敏和真实失败摘要（AC-8，R-12）

锚点：cmd/strmgen/main.go:90-118,178-187 原样输出 YAML／DB／锁错误及完整 src.Remote。diagnostic_redaction、yaml_value_redaction 用假秘密复现未知键／值片段泄漏；成功摘要也输出完整远端根。

1. CLI 统一稳定错误类别、可信字段名／行号、source 序号和相对项；不输出原始 YAML、os.PathError、SQLite 正文、远端完整根或 API 正文。
2. 在配置类型／未知键／不存在配置／重复根／DB／锁／API 错误植入合成 token、Cookie、签名 URL、私有路径标记，检查 stdout／stderr 不泄漏完整值或前缀；不能只匹配完整 token。
3. 保持可操作性；API 根／分页失败当前可 errors=0，须有真实错误计数或明确失败／partial 标记，不能把失败显示为零错误成功摘要。

#### F-7 / P1：特殊文件阻塞与容量边界（AC-4／AC-7／AC-8，R-02／R-11）

锚点：internal/reconciler/fs.go:62-70 无类型／长度上限 ReadFile；handleFile:267-274 不要求常规文件。owned_fifo_hangs 隔离内替换已拥有目标为 FIFO 后 sync 阻塞，SIGTERM 不能退出，只能 SIGKILL。openlist.List:215-272 累积单目录对象至最多100000页×200；SourceResult.Diagnostics 逐文件积累，平铺源即全库对象／诊断。

1. 安全打开后验证常规文件，FIFO／socket／设备／目录／链接快速保守失败，避免 Lstat/open 之间替换后阻塞。读取限协议上限＋1，解析在规范化前查原始长度和 UTF-8。
2. 补实际 CLI 特殊／超大文件和取消回归，明确超时，保留原目标、不提交轮次、退出后再获锁。
3. 设实际可执行每目录累计条目／名称字节上限，或分页＋有界临时路径索引检测冲突；超限明确失败、不提交轮次，不能只限单响应。保留跨页冲突目标一个也不生成。
4. 诊断保留有限样例＋总数／截断计数，不全轮累积；宽／深／平铺目录、批量异常和 dry-run 可结束或明确失败、不死锁。报告实际容量，不新增性能承诺。

#### F-8 / P2（本次一并修正）：cwd 路径语义与交付事实（AC-8／步骤2.1）

internal/config/config.go:107-111,270-273,290-293 以配置目录而非批准的启动 cwd 解析相对路径；relative_path_base 已复现。

1. 按既有 Task／config.example.yaml 的 cwd 语义修正；配置在 cwd 外，测试 cwd／输出／DB 全为专用夹具，不试写源码目录。
2. 修正文档矛盾：环境文档同时说尚无依赖／go.sum和已提交；Progress 当前指针仍 PLANNED／NOT SET。补准确权限、DB丢失、未登记文件、锁／临时文件恢复说明，不建议盲目认领或删除未知文件。旧报告作历史，新报告说明修正事实。

### 修正验证与重新送审

1. 把反例迁入受版本控制 Go／CLI 回归，本机 probe 不是唯一交接。建议 F-1／F-2 → F-3／F-4 → F-5／F-6／F-7 → F-8，各组先跑影响包和 CLI 失败恢复；仍是一个整体 Task。
2. 现有 fault_recovery 主要测建目录失败、归属插入失败、子目录 HTTP 失败、SIGTERM；补实际 CLI 临时写入／发布失败、目标恰好出现、归属成功后进程中断、page>1 中断、SIGKILL 恢复。不能用内部注入、完成轮后重启或子目录失败冒充。必要时测试专用构建／受控 OS 故障须标与发布二进制差异，不开放生产安全旁路。
3. 完整 T4 不变：原门槛中的治理、mod download／verify、quality、race、双架构 CGO=0 构建、隔离 CLI all；新 artifacts/validation-evidence/task-1.1-fix-<run-id>/，驱动用新空子目录。不得覆盖 final／B证据；保留首次失败／重跑、命令／cwd、计数／skip／耗时、源码／配置／fixture／二进制身份及逐 F 映射。
4. 增量提交，不改写旧实现；填新真实 Head，送 READY FOR B REVIEW。B 审原 Base..新 Head，复现所有 P1 与关键恢复；未满足保持 FIX REQUIRED，不新建 Task 1.2。

### Slice 全部 RO 复核（未 PASS，不进入 Closure）

| Outcome | 本次判断 | 缺口 |
|---|---|---|
| RO-1 | 未满足 | F-6／F-8 脱敏、失败摘要、路径语义；基础入口已复现 |
| RO-2 | 未满足 | F-5 形状／Size；真实 OpenList／无签名 /d/ 仍 NOT RUN |
| RO-3 | 未满足 | F-1 祖先／根替换／物理重叠；基础映射／特殊名称／既有跨页冲突用例 PASS |
| RO-4 | 未满足 | F-3／F-4／F-7 归属／DB／读取；内容替换原已留后续连贯 Task；固定插件解析 NOT RUN |
| RO-5 | 未满足 | F-2 互斥／预览副作用、F-7 容量；关键恢复证据待补 |

合同可执行，无需 A 改 Contract；Task 1.1 原地修正，Slice 保持 ACTIVE。没有 Task PASS、Closure Packet 或 A 最终结论。删除／serve／生产联调／ARM64实机／性能不伪报通过。可复用经验留待 Slice 收尾，不将审查流水账写入角色经验文档。

## Developer Completion Report — FIX Candidate (2026-10-18)

- **Changed Files / Implementation Checkpoint：** 实现检查点 `6423698f3b4ccb2a3c8250486033ebb5352b8ed0`，父提交 `596dbae3c4d36b3a577145ba211f63bab6e3e3bb`。包含 CLI、config、OpenList、pathmap、STRM、SQLite、reconciler、Linux `securepath`、Go/Python 回归、测试专用发布竞态 hook 和实现／恢复文档。`SLICE.md` 的 Implementation Head 随后的报告提交更新到该检查点；未修改合同边界、Roadmap 或稳定需求。
- **Implemented：** F-1 使用逐组件无符号链接检查、绑定输出根的目录句柄和相对文件操作，拒绝物理别名／输出与数据库及 SQLite sidecar 重叠；F-2 生产 CLI 共用固定 `/tmp/strmgen-locks/sync.lock`，不受 DB、TMPDIR 或环境覆写分裂，锁路径冲突在创建锁之前拒绝；F-3 source identity 绑定校验后的 API URL、remote 与输出映射，不含 token／public URL，旧 remote-key 行不被静默信任；F-4 以 URI 编码的文件名访问 SQLite；F-5 必需 list `content` 形状与可选 Size 分开校验，非法 Size 省略并报告，不伪造零值；F-6 统一脱敏配置、API、数据库及路径错误并统计部分失败；F-7 对单目录条目数（10,000）、名称总字节（2 MiB）、待处理目录（10,000）、每 source 诊断样例（100）设界，并有界读取 STRM、拒绝 FIFO／非普通目标；F-8 相对 output／database 路径按启动 cwd 解析并校正文档事实。
- **Tests / Results / Evidence：** 最终候选全套 T4 原始证据位于本机 `artifacts/validation-evidence/task-1.1-fix-20261018-05/`（不在远端可读）。`python3 scripts/check_governance.py`、`bash scripts/go-local mod download`、`bash scripts/go-local mod verify`、`python3 scripts/quality.py`（41 项 Python 回归及 Go gofmt／vet／test／build）、`CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 -json ./...`（134 test PASS、0 FAIL、0 SKIP）、`git diff --check` 均 PASS。Linux amd64 与 arm64 `CGO_ENABLED=0 -trimpath` 构建 PASS，SHA-256 分别为 `6a48a78623450155b396d8ff009f5e41a061e5506b1635f2a9c2dfd075475dff` 与 `3df8a7e714d56a98edaaa9559d6f608a0bb5ad03a84cc942b58869eeace8dac5`；另一个仅启用 `strmgen_testhooks` 的 amd64 故障注入测试二进制 SHA-256 为 `18d6f4b101beb256a2278038f16d2f419b308ebb71a2e872975833a2ac973b84`，不作为发布产物。实际 CLI 隔离驱动 `--scenario all` 9/9 场景 PASS，包含路径换链、不同锁输入多进程、特殊 DB 名、API／容量边界、WAL dry-run、精确发布竞态和 SIGKILL 恢复。交叉构建不代表 ARM64 实机验证。
- **首次失败与修正：** 保留 `task-1.1-fix-20261018-04/` 的完整首轮 T4 失败证据；`ownership_and_paths` 当时观测到根路径换链后四个媒体候选分别失败（`errors=4`），而测试断言错误地要求 `errors=1`。修正断言为四项错误并保持 `partial=true`；聚焦场景及后续完整 T4 均 PASS，未覆盖或删除首次失败记录。SIGKILL 恢复用例观察到一个未引用 `.strmgen-*.tmp`；已登记 STRM、归属与轮次恢复断言通过，残留恢复说明已加入文档，不能盲目清理。
- **Remaining / Risk：** 本 Task 不执行已拥有文件的内容更新；Mirror 删除／移动清理、serve／定时、发布镜像与性能保证未实现。真实 OpenList／无签名 `/d/`、固定插件解析、Jellyfin／代理、ARM64 实机及性能均 **NOT RUN**；夹具证据不替代外部兼容／播放证据。无生产服务、媒体或配套项目改动。
- **Head SHA：** `6423698f3b4ccb2a3c8250486033ebb5352b8ed0`。
- **交接：** 当前 Task 状态为 **READY FOR B REVIEW**。这是 Developer 交付报告，不是独立 B 审查 PASS，也不表示 Slice 完成。
