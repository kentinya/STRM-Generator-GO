# Task 1.1 — 首次生成、归属登记与安全重复运行

```text
Task ID: 1.1
Parent Slice: 1
Status: PLANNED
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

## Developer Completion Report

- Changed Files：NOT STARTED。
- Implemented：NOT STARTED。
- Tests and Results / Evidence：产品验证 NOT RUN；规划检查不作为产品证据。
- Decisions / Risks / Deviations：待实施后填真实事实；偏离范围／门槛先交 B。
- Remaining In-Slice Work：至少包括已拥有文件更新及恢复、真实 OpenList／固定插件解析实证；其余 RO 按实际实现重新核对。
- Head SHA：NOT SET。

## B Review Result

```text
Reviewed: NOT SET
Decision: PENDING
Slice Required Outcomes all satisfied: PENDING
Next: DEVELOPER IMPLEMENTS TASK 1.1
```
