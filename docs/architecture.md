# 架构

## CURRENT

当前实现支持一次性 `sync`、`sync --dry-run` 和定时 `serve`。V4.4 开发需求已完成，
Slice 1–4 PASS / CLOSED；当前合同见 [SLICE](../SLICE.md)／[TASK](../TASK.md)，
已接受范围及未运行项见 [A 最终审查](https://github.com/kentinya/STRM-Generator-GO/blob/4d16846722f31aa9615494b90df715cf4b5bb6df/docs/slice-4-a-final-review.md)和
[Closure Packet](https://github.com/kentinya/STRM-Generator-GO/blob/4d16846722f31aa9615494b90df715cf4b5bb6df/docs/slice-4-closure-packet.md)。本页说明现有实现与设计约束，
任务过程从[开发流程](development-workflow.md#历史记录)列出的 Git 检查点查看。
Slice 5 自定义扩展名已实现并提交独立 B 审查，最终结论以独立审查记录为准；
正式合同、验收面与边界见 [SLICE](../SLICE.md)／[TASK](../TASK.md)。

## 自定义扩展名接入

`internal/config` 以原始 YAML 节点严格解析并规范化 `sync.media_extensions`：
省略时取内置 11 项，显式列表完整替换且不合并，`null`／非字符串／空项等配置错误在触碰
远端或输出前退出 `2`。`cmd/strmgen` 在 `config.Load` 后一次构建
`pathmap.MediaSet` 并放入 `roundDeps`，`sync`、`sync --dry-run` 与 `serve` 的每轮
装配都用它；运行中的 `serve` 不热加载，重启后读取新配置。

`pathmap.MediaSet` 只按最后后缀做纯匹配，构造时不修改共享的默认集合，因此同一进程内
不同实例的列表互不污染；零值仍回落到默认项，未设置该字段的既有调用行为不变。
`reconciler` 用该实例集合筛选目录候选，同名映射冲突仍只在当前入选候选之间判定。

清理资格由 `Syncer.currentTarget` 单点判定，供缺失计数、实际删除与 dry-run 预览共用：
精确映射的源扩展名不在当前有效列表时，即使源文件仍可 Get 成功也形成一次缺失观察；
扩展名仍入选时沿用 R-08 原判定；目标 Get 读到目录等类型冲突始终保留。排除分支仍复
现有源根门槛、完整轮、可靠归属、精确远端映射、源根之下合法子路径与深度边界、输出
边界与 regular STRM 类型检查；任一不满足即保留且不计数。恢复入选后回到精确 Get 判定，
源文件可读即清零并取消删除。`sourceIdentity`、锁资源、SQLite schema 与 STRM 协议不变。

## 模块与数据流

```text
CLI / YAML / env
      |
      v
scheduler -> 单轮互斥 -> reconciler -> openlist (list/get, bounded HTTP)
                            |   |
                            |   +-> pathmap (本地相对路径 / 稳定 URL)
                            |   +-> strm (解析 / 序列化 / 比较)
                            |   +-> LocalFS / securepath (原子发布 / 恢复 / 删除)
                            +-----> storage (归属 / 路径 / 成功轮次 / 缺失次数)
```

| 模块 | 职责 |
|---|---|
| `cmd/strmgen` | CLI、配置与依赖装配、信号取消、摘要与脱敏诊断 |
| `internal/config` | 严格 YAML、扩展名列表、环境变量名、URL、路径、输出根重叠及清理／调度配置校验 |
| `internal/openlist` | 有界 HTTP、list/get、分页完整性、可选元数据；`Confirmer` 执行源根／目标 Get 判定与扩展名资格查询 |
| `internal/pathmap` | 本地路径映射、实例级扩展名匹配与稳定 `/d/` URL 逐段编码，保持纯函数 |
| `internal/strm` | V1 解析、序列化与 URL → SHA-1 → Size 比较，保持纯函数 |
| `internal/storage` | SQLite 最小归属、精确路径、成功轮次与缺失计数；不保存媒体指纹 |
| `internal/reconciler` | 有界目录对账、归属／冲突、完整轮提交、清理资格，以及 LocalFS 发布／恢复／删除 |
| `internal/securepath` | Linux 目录句柄与相对操作，限制输出及数据库路径边界 |
| `internal/scheduler` | 首轮立即执行、单 timer、busy-skip、失败后继续与取消；复用同一同步入口 |

协议和路径函数不执行网络、文件写入或数据库操作。文件 I/O 位于
`reconciler.LocalFS`／`securepath`，由 Reconciler 协调；`strm` 不负责原子替换。

## 对账与持久化

OpenList 列表全部使用 `refresh:false`。目录工作池、队列、响应与数据库枚举有界：
每目录累计最多 10,000 项／2 MiB 名称字节，待排队目录最多 10,000 个，
每源诊断最多 100 条。完整对象不会一次加载全库，也不创建每文件 goroutine。
OpenList 服务端可能先加载整个目录，这不构成客户端内存或吞吐承诺。

STRM 是 URL／SHA-1／Size 的比较来源；SQLite 只保存归属和对账事实。
旧库保留归属，历史成功轮不追认为缺失证据；未知精确远端映射通过后续观察补齐，
无法补齐时保留并报告，不猜扩展名或自动认领。

`cleanup.enabled` 默认 false，启用时要求 `sync.mirror: true`，
`cleanup.missing_confirmations` 默认与最低均为 3。清理资格为“精确映射的源扩展名不在
当前 `sync.media_extensions` 列表”或既有“目标 Get 未读到”之一。源根未成功 Get 为目录时冻结缺失计数；
源根可读而目标未读到即按缺失处理，不区分同码错误的原因。列表不完整、必要本地处理失败
或整轮取消不提交成功轮与新增缺失证据；出现观察可保守清零旧计数。
删除只消费已提交阈值，并重核当前 Get、可靠归属、精确映射、输出边界与 regular STRM 类型。
准确业务边界见[需求](requirements.md#完整轮与缺失确认)。

## 文件写入与恢复

正常部署由生成器单写，消费者只读；多个生成器共享固定
`/tmp/strmgen-locks/sync.lock` 的同一 flock 资源。固定锁不因数据库、配置、TMPDIR 或
环境变量改变。共享输出的容器必须挂同一宿主锁目录并使用一致 UID／GID。

首次发布使用同目录临时文件和不覆盖 link；已拥有文件更新使用双哈希确定性暂存名与
`RENAME_EXCHANGE`；删除通过绑定目录句柄执行相对 unlink，并同步父目录。
文件操作与 SQLite commit 不是同一事务；必要 I/O 或 DB 失败不提交完整轮。

恢复沿用 `LocalFS.Recover`：每次目录枚举独立打开游标，按 128 项分批读取，
最多保留两个匹配名。可靠归属、唯一可识别 regular 暂存和有效内容依据允许正常中断后收敛，
目录同步失败必须传播；未知、歧义或未归属对象保留。首次发布后尚未登记归属的文件仍是冲突。
不承诺无损抵御绕过单写者／锁边界的外部写入，详见[需求 SW-1..SW-6](requirements.md#本地写者与最终一致性边界sw2026-10-18-a-澄清修订)。

`dry-run` 不修改 STRM、原数据库、轮次或计数，也不迁移旧库；读取 WAL 库时可在
数据库父目录建立私有只读快照，正常退出清理。异常残留与备份规则见[部署说明](deployment.md#状态备份与恢复)。

## 调度、部署与依赖

`serve` 首轮立即同步，随后每 `sync.interval_minutes` 分钟触发一次，默认 60。
共享配置拒绝负数；`serve` 另行拒绝零和 duration 溢出，`sync`／dry-run 接受零。
单 timer 不积压 tick 或重入；锁忙报告 busy-skip，普通单轮失败后继续。
SIGINT／SIGTERM 取消当前轮、不提交不完整轮并释放锁，以非零结果退出。
调度状态不持久化，重启沿用既有归属与轮次。

[Dockerfile](../Dockerfile) 在构建平台交叉编译 Linux amd64／arm64 静态二进制；
[Compose](../deploy/compose.yaml) 注入环境 token、只读配置及独立输出／状态／锁挂载，
采用非特权身份、只读根文件系统和私有 `/tmp`。部署层不编排 OpenList 或 Jellyfin，
详细使用与发布工作流见[部署说明](deployment.md)。

依赖固定为 `gopkg.in/yaml.v3 v3.0.1` 和 `modernc.org/sqlite v1.60.1`，
版本见 `go.mod`／`go.sum`。产品使用 CGO=0，race 检查使用固定 Zig C 工具链，
后者不是运行依赖。准确工具与外部输入身份见[来源](architecture.md#固定外部输入)。

## 外部链路与验收边界

生成器仅维护自有 STRM，不下载媒体补算哈希、不运行 ffprobe、不调用插件内部接口或
修改 Jellyfin 数据库。Jellyfin 扫描与插件原生检查由管理员分别安排。

固定 10000 文件、最少同步样本、原始扫描与完整插件八行决定已在 Slice 4 接受；
探测启动不等于结果成功。ARM64 原生实机及正式发布验收不在原合同内，
播放、代理、更高规模压力和权限加固系统也不在当前范围。
分层验证入口见[测试矩阵](development-workflow.md#测试入口)，协议与外部版本边界见[兼容性](requirements.md#协议与兼容)。

## 固定外部输入

版本与摘要以 [dev/environment-spec.json](../dev/environment-spec.json) 为准：

| 输入 | 身份与来源 |
|---|---|
| 配套插件 | `kentinya/jellyfin-strm-probe@2afc6517fda5a7b4170ebea625619a552c47dabc`；[reader](https://github.com/kentinya/jellyfin-strm-probe/blob/2afc6517fda5a7b4170ebea625619a552c47dabc/src/Jellyfin.Plugin.StrmProbeOptimizer/Services/StrmManifestReader.cs)、[决策器](https://github.com/kentinya/jellyfin-strm-probe/blob/2afc6517fda5a7b4170ebea625619a552c47dabc/src/Jellyfin.Plugin.StrmProbeOptimizer/Services/ProbeDecisionService.cs) |
| OpenList | `OpenListTeam/OpenList@3b28270a4066ea52943a69ecddd850d83f7e8bdc`；[fsread.go](https://github.com/OpenListTeam/OpenList/blob/3b28270a4066ea52943a69ecddd850d83f7e8bdc/server/handles/fsread.go)、[object.go](https://github.com/OpenListTeam/OpenList/blob/3b28270a4066ea52943a69ecddd850d83f7e8bdc/internal/model/object.go)；固定隔离镜像 v4.2.7，digest 见集成驱动 |
| Go／C 工具链 | Go 1.27.2，官方工具链模块 ZIP；Zig 0.14.1 仅用于 race／CGO。下载源与 SHA-256 见环境规范 |

固定源码对照不保证任意新版本兼容；包身份须与实际安装／加载文件核对。
升级输入时显式更新规范与相关验证，不复用不匹配的旧结论。
