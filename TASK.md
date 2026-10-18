# Task 3.1 — Serve 调度、共享单轮入口与停止恢复

遵循[开发流程](docs/development-workflow.md)，范围 authority 为已提交的
[Slice 3](SLICE.md)。这是 Slice 3 的首个实施 Task；它先完成一次性 CLI 与常驻调度
的行为闭环，部署镜像、Compose、双架构发布包留给后续 Task 3.2，不把两种风险混成
一个不可独立审查的任务。本计划不是现有实现或验收结论。

```text
Task ID: 3.1
Parent Slice: 3
Status: READY FOR B REVIEW
Task Base: ee5001646d4239755606c28b61f340d795ee7014
Contract Checkpoint: ee5001646d4239755606c28b61f340d795ee7014
Difficulty: High
Test Level: T4
Planner / Reviewer: B
```

## Goal

在不改变 Slice 1／2 单轮语义的前提下，提供受同一 YAML、token、输出／数据库边界和
固定互斥保护的 `serve --config <file>`。`serve` 首轮立即执行，随后按校验后的
`sync.interval_minutes` 以有界 tick 运行；手动 `sync` 与定时轮次进入同一单轮入口和
锁。忙碌 tick 只报告跳过、不进入 Reconciler；单轮失败继续等待后续 tick；取消或
`SIGINT`／`SIGTERM` 不提交不完整轮，释放锁并以可识别的非零结果退出。Task 覆盖
RO-1、RO-2、RO-3、RO-5，以及 RO-6 的调度／取消／互斥／重启证据；RO-4 部署与构建
包不在本 Task 验收。

## 真实基线与缺口

规划前工作区干净，`python3 scripts/check_governance.py` PASS；Task Base 与当前 HEAD
均为 `ee50016`，该提交只激活 Slice 3，未移动 Slice Base `ca9f91e...`。

- `cmd/strmgen/main.go` 已装配 `sync`／`sync --dry-run`、token 校验、固定运行时锁、
  `LocalFS`、SQLite ownership store、`reconciler.Syncer.Run` 和信号 context；`serve`
  当前明确返回“不支持”。不能复制第二套同步装配或绕过既有校验。
- `internal/config` 已解析 `sync.interval_minutes`，但默认未解析为 60，当前只拒绝负数，
  `Resolved` 没有可供调度使用的已检查间隔。需定义可追溯的分钟到 `time.Duration` 检查，
  拒绝 0、负数和转换溢出；错误必须发生在锁、输出根、数据库或远端请求前。
- `internal/reconciler` 的 `AcquireLockFor` 是 sync 与未来 serve 应共享的固定跨进程锁；
  `Run(ctx, sources)` 已传播取消并在不完整轮次不提交，但 CLI 尚未把“忙碌”与一般运行
  错误区分为调度结果。不得新增调度状态表、媒体版本状态机或第二把锁。
- `internal/scheduler` 只有占位目录；实现应保持单个活跃 round、无无界 tick 队列和可
  测试的时间依赖。允许用构造器／注入时钟或 ticker 的方式缩短单元测试，不允许发布包
  通过隐藏环境变量绕过配置或制造后台重试。
- 现有 `scripts/run_isolated_sync_integration.py` 的同步场景和 `--fault-binary` 只覆盖
  一次性 CLI；调度证据需新增独立隔离场景／脚本，原有场景和失败断言不能删除或改成
  只看日志标签。

允许修改 `cmd/strmgen`、`internal/config`、新增 `internal/scheduler`、相关 Go／Python
隔离测试、配置示例、README、架构 CURRENT、测试矩阵和文件登记。原则上不改 Slice 1／2
业务逻辑、依赖版本、稳定需求或原方案；发现合同无法满足时退回 A，不用降低门槛放行。
只使用 `artifacts/` 与专用隔离夹具，不接管生产 OpenList、Jellyfin、代理或媒体树。

## Developer 实施范围

### 1. 配置与入口

1. 在 `Resolved` 中提供 `IntervalMinutes` 或等价的已验证 duration；缺省为 60 分钟，
   帮助、示例和运行说明统一写“分钟”。显式值必须是整数且 `> 0`；YAML 整数解析失败、
   非正值及转换为 `time.Duration` 的溢出均在任何输出／DB／锁动作前以 exit 2 失败。
2. 保留严格 YAML、URL、source、输出重叠、数据库 sidecar、mirror／cleanup、worker、
   token 和安全诊断。配置错误只能反映已知字段名或固定原因，不回显 token、签名 URL、
   配置路径或私有完整路径。`sync` 既有退出码和 `--dry-run` 零写行为必须保持。
3. 将单轮装配抽成 sync 与 serve 共用的受 context 控制的函数／对象。该入口每次运行都
   使用同一 `AcquireLockFor`、输出根准备、store 生命周期、source identity、报告摘要和
   `Syncer.Run`；不提供 force、热改配置、单轮删除旁路或第二个锁命名空间。

### 2. 有界调度

1. `serve` 启动后只做一次立即 round；成功、失败或 busy 均得到有界摘要，随后按固定
   interval 等待下一次 tick。每个 tick 最多排队／触发一个 round；round 未结束时不得
   创建 goroutine、累积 tick 或重入 Reconciler。忙碌由共享锁或明确的 busy 结果识别，
   不把 skip 写成成功 round。
2. 定时失败（API、文件、数据库、同步冲突）记录脱敏失败摘要并继续后续 tick；配置错误、
   锁资源不可用、输出／DB 初始化不安全等不可恢复启动错误在首轮前或明确的启动阶段非零
   退出。不要加入隐式重试、指数退避、后台队列或调度持久化。
3. 复用已有报告上限和诊断脱敏，摘要至少区分 `success`／`failure`／`busy-skip`／
   `cancelled`、下一次调度时间及 created／updated／skipped／cleanup 计数；不打印 token、
   完整签名 URL 或私有完整路径。时间格式和 stdout／stderr 归属在帮助／README 中固定。

### 3. 取消、信号与恢复

1. `SIGINT`／`SIGTERM` 使用已有 `signal.NotifyContext` 传播到当前 `Run`；父 context
   取消时停止下一 tick，等待当前 round 按现有取消边界返回，不提交 round／missing_count／
   deletion qualification，并释放锁、文件根和 DB 连接。
2. 取消／信号退出必须是非零且可识别的 `cancelled` 结果；正常完成的轮只计一次。若信号
   到达两个 tick 之间，不启动额外 round；若首轮正在执行，不允许信号处理器直接 os.Exit
   绕过清理。
3. 重启只从持久 ownership／round 事实继续，不能凭内存 tick 补计数或新增调度状态表；
   下一次 `sync` 或 `serve` 首轮按 Slice 1／2 的既有恢复和最终一致性收敛。单轮取消／
   失败后重新运行必须可观察地证明锁释放且不重复提交已完成 round。

### 4. 证据与文档

1. 新增调度包单元测试：立即首轮、固定 interval、无重叠／无积压、busy skip、失败后继续、
   cancel 不再 tick、ticker／goroutine 结束。时间依赖通过测试注入，不以等待真实 60 分钟
   作为唯一证据。
2. 新增隔离 CLI／多进程脚本或扩展现有脚本，使用动态本地 OpenList、临时输出／DB、发布
   二进制和可控 testhook 二进制分别验证：首轮与后续 tick、手动 sync 抢锁导致 busy、
   单轮服务失败后下一 tick、SIGTERM／SIGINT 窗口、锁释放、重启无重复 round、日志脱敏。
   证据必须断言文件／SQLite／退出码和请求次数，不只断言文本。
3. 更新 `config.example.yaml`、README、架构 CURRENT、测试矩阵和环境说明，明确本 Task
   尚未验证 Docker、镜像、Linux 双架构运行和真实外部播放；不把 scheduler 单元或 Python
   夹具写成真实 OpenList／Jellyfin／插件通过。

## 验收条件

| ID | 可核对结果 |
|---|---|
| AC-1 配置前置 | 缺省 60；0、负数、非整数和 duration 溢出在输出／DB／锁／远端请求前 exit 2；sync 既有校验、token 脱敏和 dry-run 零写不回退。 |
| AC-2 首轮与固定 tick | serve 首轮立即执行；后续按固定 interval 运行；每个 tick 最多一个 round，不重叠、不积压、不创建无界 goroutine；稳定输入重复轮仍 skip／零写。 |
| AC-3 共同互斥 | 手动 sync 与 serve 使用同一固定锁和输出／DB scope；锁忙只报告 busy-skip，不进入 Reconciler、不提交 round；释放后下一 tick 可运行。 |
| AC-4 失败与取消 | 单轮 API／文件／DB 失败可观测且后续 tick 继续；父 context／SIGINT／SIGTERM 停止后续调度，当前轮不提交不完整事实，非零 cancelled 退出并释放资源。 |
| AC-5 恢复与安全 | 重启不凭内存 tick 补计数、不重复成功 round；下一次首轮按既有 ownership／round／清理恢复；无 token／签名 URL／私有完整路径泄露。 |
| AC-6 证据与回归 | 新增 scheduler 单元及隔离 CLI／多进程／信号证据可复现；既有 sync 全量回归、治理、格式、vet、race、构建和 diff-check 通过；真实外部联调、Docker、ARM64 实机明确 NOT RUN。 |

## Developer 验证计划

工作目录 `/opt/STRM-Generator`。开发／修正反馈先跑受影响包和新增场景，不能以计划中的
场景名伪报现有基线已经通过：

```bash
python3 scripts/check_governance.py
bash scripts/go-local test -count=1 ./internal/config ./internal/scheduler ./internal/reconciler ./cmd/strmgen
python3 -m unittest discover -s tests -p 'test_*.py' -v
```

候选最终检查点至少保留以下门禁（命令、cwd、版本、退出码、耗时和原始输出进入新的
`artifacts/validation-evidence/task-3.1-*`，不覆盖旧证据）：

```bash
mkdir -p artifacts/task-3.1/bin artifacts/validation-evidence
TASK_EVIDENCE="$(mktemp -d artifacts/validation-evidence/task-3.1-final-XXXXXX)"
python3 scripts/check_governance.py
bash scripts/go-local mod download
bash scripts/go-local mod verify
python3 scripts/quality.py
CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 -json ./...
bash scripts/go-local test -count=1 -json ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 bash scripts/go-local build -trimpath -o artifacts/task-3.1/bin/strmgen-amd64 ./cmd/strmgen
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 bash scripts/go-local build -trimpath -o artifacts/task-3.1/bin/strmgen-arm64 ./cmd/strmgen
bash scripts/go-local build -tags strmgen_testhooks -trimpath -o artifacts/task-3.1/bin/strmgen-testhooks-amd64 ./cmd/strmgen
python3 scripts/run_serve_integration.py --binary artifacts/task-3.1/bin/strmgen-amd64 --fault-binary artifacts/task-3.1/bin/strmgen-testhooks-amd64 --output-dir "$TASK_EVIDENCE/serve"
python3 scripts/run_isolated_sync_integration.py --binary artifacts/task-3.1/bin/strmgen-amd64 --fault-binary artifacts/task-3.1/bin/strmgen-testhooks-amd64 --scenario all --output-dir "$TASK_EVIDENCE/sync-regression"
git diff --check
```

交叉构建仅证明可构建，不证明 ARM64 实机；Docker／Compose、真实 OpenList、无签名 `/d/`、
插件解析、Jellyfin／代理、规模与正式发布均由后续 Slice／Task 单列。测试证据需区分
NOT RUN、BLOCKED、SKIPPED 与 PASS；任何新增脚本／场景不存在时不得把命令标成已执行。

## B 独立最终门禁

B 审查完整 `Task Base..Implementation Head`，核对实现、测试脚本、二进制／夹具哈希和
原始证据输入匹配。B 不机械重跑已匹配的全部 Slice 1／2输入，但必须独立关键复现：

1. 用隔离发布二进制验证首轮立即执行、两个 interval、稳定输入无写／skip、忙碌手动 sync
   不进 Reconciler，锁释放后后续 tick 运行。
2. 在真实 round 进行中发送 SIGTERM／SIGINT，检查进程非零 cancelled、输出／SQLite 无新
   不完整 round／missing_count／删除资格，锁可被新的 sync 获取；重启只产生预期下一轮。
3. 让一轮 API／文件／DB 失败，确认服务保持运行并在下一 tick 重试；检查 stderr／stdout
   不含 token、签名 URL、私有完整路径且摘要有界。
4. 独立检查 0／负数／溢出／缺 token／缺配置在任何产品文件或数据库变更前失败；复核
   `sync`／`sync --dry-run`、Mirror／清理、未归属保护和跨进程锁的直接影响回归。

可复用 Developer 的 quality、race、构建或旧同步场景证据，前提是完整源码 SHA、命令、
配置、夹具、环境和二进制身份逐项一致；不能只凭 PASS 标签或日志摘要放行。若发现调度
重叠、取消提交、共享锁失效、泄密、越界或既有清理安全回退，结论为 `FIX REQUIRED`，
在本 Task 内修正并增加提交；不能另造润色 Task。RO-1..RO-6 全部满足后，B 停止创建 Task，
按流程更新 Slice Closure Packet 并交 `SLICE READY FOR A REVIEW`，不自行关闭 Slice。

## 证据、完成报告与 B 决定

证据索引至少包括：完整源码 SHA、未提交输入摘要、Go／Python／Docker 工具版本、隔离
OpenList 身份、配置与 token 仅以脱敏标识记录、命令／cwd、退出码／计数／耗时、首次失败
与重跑、文件／SQLite 快照、信号时间线、锁竞争过程、goroutine／资源收尾观察和 AC／RO
映射。日志不包含真实 token、Cookie、签名 URL 或私有完整路径。

```text
Implementation Head: b18caba6fd786bfe589f6ac29c2a2e779a5a1628
Developer Completion Report: READY FOR B REVIEW
Developer Evidence Index: artifacts/validation-evidence/task-3.1-final-bgG1Ej/INDEX.md
B Reviewed Range: NOT SET
B Decision: PENDING
P0/P1 Blockers: PENDING
Remaining RO: RO-1／RO-2／RO-3／RO-5 待 B 重评；RO-4 部署／构建包延期至 Task 3.2；RO-6 调度证据已交付、真实外部联调／Docker／ARM64 实机 NOT RUN
Next Action: B REVIEWS TASK 3.1
```

## Developer 完成报告

按活动 Task 3.1 实现并交 **READY FOR B REVIEW**，不自判 PASS。

- 实现：`internal/scheduler` 有界调度（单 timer、无每 tick goroutine、无积压、
  busy／failure／cancelled／fatal 分类）；`internal/config` 已验证 `IntervalMinutes`／
  `Interval`（缺省 60、仅正整数、拒绝非整数与分钟到 duration 溢出，且在任何锁／输出／DB
  动作前失败）；`cmd/strmgen` 将单轮装配抽为 `runRound` 供 `sync` 与 `serve` 共用同一
  `AcquireLockFor`、输出根准备、store 生命周期与 `Syncer.Run`；`serve` 首轮立即执行、
  固定间隔触发、busy-skip 不进入 Reconciler、单轮失败继续、SIGINT／SIGTERM 非零
  cancelled 退出并释放锁。`cmd/strmgen/serve_hook_integration.go` 仅为 testhooks 构建
  提供间隔缩短；发布二进制无隐藏环境开关。未改 Slice 1／2 业务规则、依赖、需求或原方案。
- 文件：`internal/scheduler/scheduler.go`、`scheduler_test.go`；`internal/config/config.go`、
  `config_test.go`；`cmd/strmgen/main.go`、`main_test.go`、`serve_hook_default.go`、
  `serve_hook_integration.go`；`scripts/run_serve_integration.py`、
  `tests/test_run_serve_integration.py`；`dev/environment-spec.json`、`config.example.yaml`、
  README、AGENTS、架构、环境、测试矩阵、进度。
- 验证（详见证据索引）：治理、`mod download`／`verify`、`quality.py`、race（290 pass／0 fail／0 skip／9 包）、`-count=1` 全量（同计数）、Linux amd64／arm64 与 testhooks 构建、隔离 serve 驱动 6/6、既有隔离 sync 回归 11/11、`gofmt`、`vet`、`git diff --check` 全部 exit 0。首次 race 运行在 scheduler 测试辅助发现 data race，修复后重跑通过（事实保留）。
- 风险与未运行：真实 OpenList、无签名 `/d/`、固定插件解析、Jellyfin／代理、Docker 镜像／Compose、ARM64 实机、规模与正式发布均 NOT RUN；RO-4 部署／发布包延期至 Task 3.2。B 需独立关键复现首轮／间隔、忙碌跳过、SIGTERM／SIGINT 取消、失败后继续、配置前置失败与既有 sync 回归。
