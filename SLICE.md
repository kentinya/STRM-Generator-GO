# Slice 3 — 定时与部署

```text
Slice ID: 3
Owner: A
Status: FIX REQUIRED
Base SHA: ca9f91e10a0fc6c28679b5827f453e632eb9854a
Implementation Head: 410fc0e0a8964046de4707d58c0c7505385c6002
```

## 用户目标

管理员可以继续使用一次性的 `sync`／`sync --dry-run`，也可以使用同一份经过
校验的配置运行长期 `serve`。`serve` 首轮立即执行，之后按 `sync.interval_minutes`
有界地触发同步；手动与定时轮次共用同一互斥资源，不重叠、不积压未界定的 tick，
忙碌时给出可解释的跳过／忙碌结果。收到停止或取消信号时，当前轮按既有取消语义
结束，不提交不完整轮，释放锁，后续重启或手动同步可以正常收敛。

管理员还可以获得可追溯的 Linux amd64／arm64 构建和 Docker 运行包，明确 token、
输出、SQLite 数据库、临时目录及共享锁资源的挂载边界；镜像不携带凭证，不把容器
私有的同名 `/tmp` 锁误称为跨容器协调。部署说明能区分一次性／常驻运行、状态和
恢复方式，但不把本 Slice 的构建或夹具结果写成真实播放或外部兼容验收。

## 已接受基线与本轮选择

2026-10-19 A 在 Slice 2 关闭后的独立回合选择本 Slice。Base 固定为选择前真实
HEAD `ca9f91e10a0fc6c28679b5827f453e632eb9854a`，即 Slice 2 A 关闭检查点；合同、
路线图和当前状态文档的提交不移动 Base。

Slice 2「安全目录镜像与清理恢复」已 PASS / CLOSED，产品实现 Head 为
`d0a0f6db5447f38467c67612ed7173d77e6bea27`，关闭协调提交为本合同 Base。Task 2.1／
2.2 均有独立 B PASS，三轮缺失、源根／目标 Get 简化判定、本地安全删除、移动收敛、
dry-run 和正常清理失败恢复不在本 Slice 重做。当前代码仍明确拒绝 `serve`；
`sync.interval_minutes` 只被解析为兼容字段，`internal/scheduler` 尚无产品实现。

本 Slice 承接路线图中尚未交付的定时与部署结果。A 不在合同中预设包拆分、调度器
接口、镜像基础层或 Task 数量；B 必须先按真实代码规划可整体审查的实施闭环。

## Required Outcomes / Surfaces

- **RO-1 配置与操作入口：** `sync`、`sync --dry-run` 的既有行为和退出约定保持不变；
  `serve --config <file>` 接受同一 YAML、环境变量 token、sources、Mirror／清理与
  数据库边界。`sync.interval_minutes` 缺省为 60 分钟，单位在配置／帮助／运行说明中
  一致；
  `serve` 拒绝非正或溢出的间隔，并在任何输出／数据库变更前报告配置错误。首轮
  立即运行，后续按固定间隔触发；不提供单轮 force、热改配置或绕过现有安全校验的
  入口。
- **RO-2 定时与互斥闭环：** 每个定时 tick 最多产生一个待运行轮次；同一 `serve`
  进程不会重叠运行或创建无界 goroutine／队列。手动 `sync` 与 `serve` 使用相同的
  输出／数据库锁资源；锁忙时不进入 Reconciler，报告忙碌／跳过并继续后续调度，
  不把跳过伪称成功轮。不同输出范围仍遵守既有安全边界，不能用各容器私有 `/tmp`
  的同名锁声称跨容器协调。
- **RO-3 停止、取消与重启恢复：** `SIGINT`／`SIGTERM` 或父 context 取消正在执行的
  轮次时，沿用现有 context 传播、清理和数据库提交边界；不提交完整轮、缺失次数或
  删除资格，不启动下一轮，释放共享锁并以非零运行错误／取消结果可识别地退出。
  已完成轮次只计一次；进程重启后不凭内存 tick 补计数、不引入媒体版本／调度状态机，
  下一次正常同步按 Slice 1／2 的恢复和最终一致性收敛。常驻进程遇到单轮 API、文件
  或数据库失败须记录失败并继续后续间隔，除配置／锁／不可恢复启动错误外不静默退出。
- **RO-4 部署与构建包：** 提供可复现的 Linux amd64／arm64 `CGO_ENABLED=0` 产品构建，
  与适用的 Dockerfile／Compose 或等价运行包。运行包以非特权、无凭证镜像方式装配，
  token 只从环境注入；输出根、SQLite 文件及其 sidecar、临时写目录和锁目录的挂载／
  权限／备份边界有明确说明。共享同一输出的多个容器必须显式共享同一锁资源，单容器
  私有 `/tmp` 不得被文档当作互斥保证。构建和包检查不得修改生产 OpenList、Jellyfin、
  代理或媒体树。
- **RO-5 可观测性与有界资源：** 一次性与常驻模式的 stdout／stderr 摘要区分成功轮、
  失败轮、忙碌跳过、取消、下一次调度时间及已完成的新增／更新／跳过／清理计数；不
  输出 token、签名 URL 或私有完整路径。调度 timer、round context、目录 worker、
  HTTP 响应和日志诊断保持既有上限；停止后没有泄漏的 timer、goroutine、文件句柄或
  数据库连接。不存在未定义的后台重试、指数队列或全库驻留状态。
- **RO-6 分层验证与交接：** 提供可重复的隔离 CLI／多进程／信号／重启证据，覆盖
  `sync` 回归、`serve` 首轮和间隔、忙碌跳过、单轮失败后继续、取消不提交、锁释放、
  Docker 配置／挂载／凭证注入及 amd64／arm64 构建。T4 证据需区分本地实现、隔离
  OpenList、容器环境和未运行的真实外部依赖；不把交叉构建、环境 smoke 或 Python
  夹具写成 ARM64 实机、稳定 `/d/` 播放、插件解析、Jellyfin／代理或规模通过。

## 约束与安全不变量

遵守 [需求 R-01..R-12、SW-1..SW-6](docs/requirements.md)、[用户旅程](docs/product-experience.md)、
[架构](docs/architecture.md) 和 AGENTS。Slice 1／2 已接受的协议、URL → SHA-1 → Size
比较、可靠归属、输出／符号链接边界、dry-run 零写、源隔离、完整轮与简化 Get 清理
规则、共享锁、目录并发和有界资源均不得回退。

`serve` 只是复用同一个同步入口的调度外壳，不另造文件发现、媒体版本、删除或远端
版本状态机。定时 tick 不改变对账 epoch；只有完整成功轮按既有规则提交 round／缺失
事实。常驻失败不把部分轮标成成功；关闭清理、源根不可读、列表不完整、必要本地
处理失败及 dry-run 的既有语义保持。消费者只读，生成器共享同一锁对象；不增加 UID／
mode 强制检查、消费者 OS 拒写实验或绕过锁外部写者的绝对防护承诺。

仅操作本仓库 `artifacts/` 下的临时输出、状态、镜像构建和隔离服务；不修改
`/opt/jf-strm-probe`、生产 OpenList、Jellyfin、代理或生产媒体。所有真实服务、
播放和发布证据必须在专用隔离实例中取得并分层记录。

## 排除范围与取舍

不重做 Slice 1／2 的协议、目录镜像、清理、持久化或故障恢复；其完整回归是本 Slice
门槛。运行包不承诺 Kubernetes／systemd 的专属控制器、远程管理 API、HTTP health／
metrics、配置热重载、动态 token 轮换、自动备份或发布到 registry；这些需要独立
需求与证据时另行规划。不会为了调度再增加媒体状态表、跨资源事务或远端锁。

真实 OpenList 驱动／鉴权、无签名 `/d/` 播放、固定插件实际解析、Jellyfin／代理联调、
ARM64 实机运行、十万／百万规模和正式发布留在 Slice 4；Docker 在本 Slice 只验证
构建、最小隔离运行和挂载／停止行为，不宣称生产兼容。

## 验收与最终验证

本 Slice 为调度、并发、停止恢复、打包部署 T4。B 在本 ACTIVE 合同提交后依据真实
实现制定准确 Task、命令和证据计划；A 不预设接口、文件拆分或 Task 数量。

Slice Final 必须覆盖：

1. 既有 `sync`／`sync --dry-run`、协议／比较、Mirror、dry-run 零写、未归属保护、
   输出边界、跨进程锁和正常恢复全量回归；Serve 配置缺失、非法／零／过大间隔及
   缺失 token 在触碰输出／数据库前失败。
2. `serve` 首轮立即执行、按最小隔离 interval 触发后续轮；稳定输入的重复轮保持
   skip／零写，失败轮输出脱敏摘要且下一 tick 仍可运行；不产生重叠轮、无界队列或
   timer／goroutine 泄漏。
3. 手动 `sync` 与 `serve` 争用同一固定锁：忙碌时只报告跳过／忙碌，不触碰输出／DB；
   锁释放后下一轮可运行，独立进程重启不重复提交已完成 round。
4. 在列表／本地／数据库／取消故障以及 SIGTERM／SIGINT 窗口中，当前轮不提交不完整
   round／缺失计数／删除；进程退出后重启或手动同步按已有事实安全恢复。保留首次
   失败与重跑，不宣称跨文件事务回滚。
5. Linux amd64／arm64 产品构建、镜像构建／配置检查、非特权最小运行、环境 token
   注入、输出／DB／锁挂载、容器停止与重新启动证据完整且可追溯；共享输出的多容器
   互斥说明和实际隔离验证一致。ARM64 运行标为 NOT RUN，除非有真实目标机证据。
6. 全套质量门槛、格式／vet／test／build、依赖校验、显式 CGO race、适用多进程／
   Docker／故障恢复验证及 `git diff --check`；准确源码／二进制／配置／镜像身份、
   命令／cwd、退出码／计数／耗时和原始日志进入证据索引。真实 OpenList、插件、
   Jellyfin／代理、播放、性能和正式发布单列 PASS／NOT RUN／BLOCKED。

Slice Final 仍审查固定 Base..Implementation Head 全范围；P2 的部署润色、非必要
平台集成或可选证明不阻止 PASS，但任何调度重叠、取消提交、锁失效、凭证泄露、
越界挂载或既有清理安全回退均为 P0/P1 阻断。

## Closure Packet

```text
Packet Status: REVIEWED - FIX REQUIRED
Slice Base: ca9f91e10a0fc6c28679b5827f453e632eb9854a
Reviewed Implementation Head: 2131b54bcec1cd43769d239a30456b4a8607f4aa
Task Index: Task 3.1 PASS (ee5001646d4239755606c28b61f340d795ee7014..b18caba6fd786bfe589f6ac29c2a2e779a5a1628); Task 3.2 PASS (8fc3a3ad8a720e9ca4c832967fdf39874581beff..2131b54bcec1cd43769d239a30456b4a8607f4aa)
Required Outcomes: B submitted RO-1..RO-6 PASS; A review found RO-1, RO-4 and RO-6 FIX REQUIRED
Final Evidence: artifacts/validation-evidence/task-3.1-final-bgG1Ej/INDEX.md; artifacts/validation-evidence/task-3.2-final-20261019-1792350581/INDEX.md; artifacts/validation-evidence/task-3.2-b-review-20261019/
Invariants: PASS; existing sync/mirror/dry-run/ownership/path/lock/round boundaries retained; deployment uses runtime token, read-only config, explicit persistent mounts, and shared lock object for multi-container output
Known limitations / deferred: Slice 4 external integration, ARM64 runtime, scale and release
Documentation coordination: current FIX REQUIRED status and A blockers synchronized; no closure performed
A Final Review: FIX REQUIRED (2026-10-19)
Next Action: B PLANS AN IN-SLICE CORRECTION
```

## A Final Review

```text
Reviewed Range: ca9f91e10a0fc6c28679b5827f453e632eb9854a..2131b54bcec1cd43769d239a30456b4a8607f4aa
Decision: FIX REQUIRED
P0/P1 Blockers:
- P1 / RO-1: `internal/config.validate` rejects `sync.interval_minutes: 0` for every command, although the Base accepted zero and Slice 3 requires positive validation for `serve`; `sync` and `sync --dry-run` therefore no longer preserve existing configuration behavior.
- P1 / RO-4, RO-6 / Task 3.2 AC-2: Compose interpolates `OPENLIST_TOKEN` into `docker compose config` output, and the container acceptance driver asserts that the token appears there. This contradicts the no-token rendered-config/output acceptance and exposes real runtime values to terminal or command-output capture.
Next Action: B PLANS AN IN-SLICE CORRECTION; Developer implements only after the plan checkpoint and governance pass
```
