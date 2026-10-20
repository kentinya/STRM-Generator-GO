# 架构：CURRENT 与 TARGET

## CURRENT

Slice 1–3 PASS / CLOSED；Slice 3 A 已审至 `410fc0e0a8964046de4707d58c0c7505385c6002`。当前 CLI 为 sync／dry-run／serve。Slice 4 在原 Base 内按 V4.4 重规划；只新增隔离验收证据和必要验证支持，不引入产品运行时组件。4.4 已适配本地建集工具与只读 OpenList probe；经用户授权从已核对的专用隔离容器内存取用凭据后，真实 S3 对象补足至 10000、最终清单／dataset_id 已固定，真实 OpenList 递归分页与样本匹配，现为 READY FOR B REVIEW；插件完整决策尚未验收，当前交接见 [TASK](../TASK.md)。用户本轮要求完全重排为 4.4 资产复用／建集、4.5 生成／同步、4.6 原始扫描／完整决策；4.5／4.6 尚未实施；旧任务不作为审查依赖或当前验收完成依据。

`internal/scheduler` 提供有界调度外壳：`serve` 首轮立即执行，之后用单个可复用 timer 按校验后的 `sync.interval_minutes`（缺省 60 分钟，必须为正整数且分钟到 `time.Duration` 转换不溢出）触发；每个 tick 最多一个 round，不创建每 tick goroutine、不累积 tick、不重入 Reconciler。手动 `sync` 与 `serve` 共用 `reconciler.AcquireLockFor` 固定锁与同一单轮装配；锁忙只报告 `busy-skip` 不进入 Reconciler；单轮失败记录脱敏摘要后继续下一 tick；SIGINT／SIGTERM 经 `signal.NotifyContext` 传播，当前轮按既有取消边界结束、不提交不完整轮、释放锁并以非零 cancelled 结果退出。调度状态不持久化，重启只从既有 ownership／round 事实继续。

部署层由仓库根 [Dockerfile](../Dockerfile) 与 [deploy/compose.yaml](../deploy/compose.yaml) 定义：构建阶段在 `$BUILDPLATFORM` 以 `CGO_ENABLED=0` 交叉编译 `$TARGETARCH`，运行层不执行架构相关命令，只复制 CA 根、非特权账号文件与静态二进制；镜像按 digest 固定基础层，不含 token／源码／testhooks／宿主状态。Compose 以运行时环境注入 token、只读挂载配置、分别挂载输出／SQLite 状态／固定锁目录、容器私有 `/tmp` tmpfs，非 root 且 `read_only`／`cap_drop: ALL`／`no-new-privileges`，正常 SIGTERM 让当前轮取消并释放 flock。共享输出的多容器必须挂同一锁目录对象并使用一致 UID／GID；容器私有 `/tmp` 同名锁不构成互斥。验收驱动 `scripts/run_container_integration.py` 只用隔离合成服务与本仓库 artifacts 路径，不接触生产。

`internal/config` 严格解析 YAML 并校验输出根／URL；`internal/openlist` 实现有界 list/get、分页和应用层错误处理；`internal/pathmap` 与 `internal/strm` 提供路径／稳定 URL、版本 1 协议及 URL → SHA-1 → Size 比较；`internal/storage` 用 SQLite 记录最小归属、精确远端／本地路径映射、完整轮次与连续缺失次数，不保存媒体指纹。`internal/reconciler` 以有界目录工作池完成生成／更新／跳过、同目标冲突、观察清零、源根／目标 Get 缺失确认、已提交阈值消费和绑定目录句柄内的安全删除；首次发布使用同目录临时文件加不覆盖 link，已拥有更新使用确定性双哈希暂存名及 `RENAME_EXCHANGE`，删除使用相对 unlink 加父目录 sync。Linux `securepath` 通过绑定目录句柄和相对操作限制输出及 DB 父目录访问。

恢复沿用 LocalFS／Recover：每次相对绑定目录独立打开枚举流，按 128 项分批、最多保留两个匹配名；可靠归属、唯一可识别 regular 暂存及仍匹配旧哈希的目标允许恢复未写／半写状态，正常交换中断也支持 desired 改变后的重试。恢复目录变更须经过目录 sync，失败非零且不提交完整 round。无归属、歧义、类型或路径冲突保留；首次发布后归属未登记仍不自动认领。Slice 3 的 serve 必须复用这一取消／恢复边界。

正常写者边界为需求 SW-1..SW-6：消费者只读，多生成器共享固定 flock 资源；不承诺无损抵御绕过锁的任意外部写入。F-14／F-15／F-16 已由新候选回归与进程重启验证，F-10／F-13／外部来源 F-11 的旧 FAIL 按边界保留为历史，不改称绝对防护已实现。A Slice Final、B 完整 T4 及复用身份保留于 Slice 2 关闭检查点；真实 OpenList 的固定隔离 Get／缓存证据已在 Task 2.1 独立验收；当前 S3/RustFS、媒体原件及固定插件包作为输入保留复用，当前生成／同步／原始扫描／
完整插件决策仍须由新任务验证；ARM64 实机、万文件测量及正式发布仍 NOT RUN，代理不属于当前 Slice 4 验收范围。Mirror 清理已由 Task 2.2 实现并经 B 审查和 A 最终审查接受；serve／定时和发布镜像已由 Task 3.1／3.2 实现并经 B PASS，Slice 3 A 已完成最终审查并关闭。

现有配置支持显式 `cleanup.enabled: true` 且 `sync.mirror` 为 true；mirror=false 与 cleanup=true 在写入前拒绝，`cleanup.missing_confirmations` 最低 3。`sync.interval_minutes` 缺省 60；`sync`／`sync --dry-run` 不调度，故接受零间隔，负数由共享 `config.Load` 拒绝；`serve` 在任何输出／数据库／锁／token 动作前调用 `Resolved.ValidateServeInterval()` 拒绝零／负数／非整数／分钟到 `time.Duration` 溢出间隔。SQLite 写模式事务升级 Slice 1 表，新增精确 `remote_path` 与 `missing_round` 标记；旧行保留归属但映射未知、历史计数不具资格，后续观察可补齐。每个完整成功轮对未观察到且映射可靠的已拥有目标重核，缺失仅在轮次提交事务中增加一次；再出现／Get 成功清零。清理只消费已提交阈值证据并在删除前重核，本地失败停止该源后续删除且不提交失败轮计数；进程重启后依据当前事实收敛。观察、缺失待提交集合与候选枚举均有界（固定批量、keyset、内存 scratch 工作区）；dry-run 不写原库／文件／计数／轮次，也不迁移旧库。列表仍只有普通 `List`，每页显式 `refresh:false`，`ListFresh`／强刷接口与祖先链确认已移除；`openlist.Confirmer` 只做“源根 Get + 目标 Get”两次读取，源根门槛同时被 Reconciler 复用（`openlist.CheckSourceRoot`）。serve／定时与部署镜像／Compose 已由 Task 3.1／3.2 实现并经独立 B PASS，A 已完成 Slice 3 最终审查并关闭；以下 TARGET 不表示新的活动 Slice。

依赖固定为 `gopkg.in/yaml.v3 v3.0.1` 与 `modernc.org/sqlite v1.60.1`（CGO=0，Linux amd64／arm64 可构建），go.sum 已提交。race 门槛使用仓库内固定的 Zig `cc`（`artifacts/dev-env/cc/bin/cc`，仅用于 CGO，不是产品运行时依赖）。目录列表累计限制为每目录 10,000 项和 2 MiB 名称字节，待排队目录最多 10,000 个，诊断最多保留每源 100 条；这些是实现边界，不是吞吐或内存承诺。环境 smoke 与真实 OpenList／Jellyfin 证据仍分开报告。

测试侧保留复用已完成的隔离环境、媒体原件、媒体生成／启动／停止脚本；详见
[资产清单](slice-4-assets.md)。旧生成脚本仍为 201 占位写入，不能重跑覆盖现有媒体；
按需先保留原版再最小适配，实例运行中不强制重建。测试侧使用既有 CLI／S3 命令和 Jellyfin 原生检查。允许同批本地真实 SHA-1 与标注副本，不让生成器补算哈希；插件完整决策须实际取证。过时目标运行器由工具治理退出，当前工具清单见 [测试矩阵](test-matrix.md)。

## TARGET

### 正常单写者与最终一致性（2026-10-18 A 澄清）

需求 SW-1..SW-6 与当前 Slice 明确正常使用方式：生成器写输出／暂存／DB，消费者只读；多生成器共享同一锁对象，不能用各容器私有 /tmp 的同名路径冒充同一锁。专用 UID、权限或只读挂载可作为部署建议，但不追加 UID／mode 强制检查、消费者 OS 拒写实验或全系统写者审计能力。

沿用 securepath 绑定句柄、现有锁、同目录原子写入和最小 SQLite 归属事实；恢复继续使用既有 LocalFS／Recover 入口，不新增媒体版本或事务状态机。修正本地目录枚举时，各次枚举独立打开绑定目录并分批读取，不共享 dup 游标、不加载完整目录。生成器正常中断的残留应依据现有归属与恢复证据处理，并在后续成功同步中收敛；不能把所有正常残留一律永久冲突。

未归属文件、归属丢失与确实无法解释的路径／类型冲突保守保留；合法 STRM、名字、哈希或 OS UID 本身不建立归属。已有可靠归属的可再生成内容出现可识别偏差，不等于必须丢弃旧 ownership 重建。无条件保护任意外部注入的所有字节不是本架构目标，也不能把同一干预转为静态 legacy 状态重新要求。扫描期间变化依现有比较规则后续纠正，不每次写入前查远端、不引入远端锁。

CURRENT 与 TARGET 的差距由 Slice 4 证明：固定复用 10000 个 S3 文件，经真实 OpenList 生成 STRM；少量样本同步、原始产物扫描与插件完整决策。使用已锁定插件包和隔离实例，无客户端播放／代理／更高规模压力或旧暂缓例外。ARM64 实机及发布未授权实施，不作为当前验收门槛。每目标本地枚举成本须体现在万文件测量中。

```text
CLI / YAML / env
      |
      v
scheduler -> 单轮互斥 -> reconciler -> openlist (list/get, bounded HTTP)
                            |   |
                            |   +-> pathmap (本地相对路径 / 稳定 URL)
                            +-----> strm (解析 / 比较 / 原子写入)
                            +-----> storage (归属 / 路径 / 成功轮次 / 缺失次数)
```

`cmd/strmgen` 装配入口；`internal/openlist` 处理传输／API语义，不管理删除；`pathmap` 与协议比较保持纯函数；`strm` 封装同目录原子替换与本地读取；`storage` 记录最小持久事实；`reconciler` 决定完整轮和清理资格；`internal/scheduler` 决定何时复用相同同步入口、共享锁和取消 context，不持有文件／数据库／锁状态。配置与日志装配由 Task 3.1 落地。

目录增量遍历使用有界队列／工作池，处理完一个分页或目录就释放完整对象；SQLite 提供索引与分批查询。OpenList 服务端可能先加载整个目录，这不等于生成器本地内存有界已获验证；客户端必须验证分页与响应限制。

当前 STRM 是 URL／SHA-1／Size 的比较来源，SQLite 不复制独立媒体指纹状态。完整轮次与缺失计数只是删除证据，不扩展为媒体版本或发现状态机。再次确认与拥有关系、路径边界都在删除前重核。

### Slice 2 Mirror 与清理恢复目标

按需求的完整轮／缺失约定，以 source 隔离观察、成功轮及连续缺失事实；观察／候选和数据库枚举均有界，不加载全库媒体对象。完整遍历与必要生成／更新成功后才允许增加缺失证据；skip 仍算出现，正常同步确认再出现使旧证明失效，后续失败不得把该观察算成缺失。可单独保守清零而不追加成功轮；dry-run 不持久化观察或迁移旧库。源根缺失、不可访问或源已移除时不清扫历史归属。

清理默认关闭，`cleanup.missing_confirmations` 默认值与最低允许值均为 3；更高阈值只延后清理。Mirror 开关与阈值按需求校验。关闭期间不积累新缺失证明，已确认再出现仍使旧证明失效；重新启用须先完成当前源必要处理和再确认。存储仅补齐归属、精确远端／本地路径映射、完整轮及缺失次数所需事实。旧库归属保留，旧成功轮不变成缺失证据；无法通过后续观察补齐映射的历史缺失记录保留并可解释，不猜扩展名。当前接口和表结构由 B 根据真实代码规划，本合同不预设 SQL 或实施顺序。

远端删除判定按用户最终简化规则：配置源根 Get 未成功读取为目录时保留且不增加计数；源根可读取时，对正常对账未观察到的已有归属路径调用 Get，未读到即作缺失观察，不再按 HTTP／应用错误、权限、驱动、缓存或单请求失败原因细分。目标 Get 成功则保留／清零，类型冲突保留。整轮取消仍中止提交；列表失败／分页不全和必要本地处理失败仍不能成为完整轮。精确规则见[需求](requirements.md#简化删除判定2026-10-18-用户最终决定)。

所有列表请求统一 refresh:false，包含扫描、分页、辅助确认和 dry-run；强刷 `ListFresh`、祖先刷新链、目录证明复用与确认预算／冷却能力均已撤下并从代码移除。缺失计数使用不同完整轮；删除前消费已提交的至少三次证据，按相同源根／目标 Get 规则重核，并检查可靠归属、精确映射、输出边界及 regular STRM。稳定且本地条件有效时至迟在阈值满足后的下一次成功同步收敛。用户接受源根可读而目标异常未读到也会计数，不宣称权威辨别了真实不存在。远端判定已按本规则实现并有隔离真实服务证据；计数／删除／恢复接入已由 Task 2.2 实现并经独立 B 审查和 A Slice Final 接受。

文件 rename 和数据库 commit 不构成同一事务。清理本地失败停止该源后续删除，不保留失败轮新增证明，报告已完成项；后续重启按当前源根／Get 规则与可靠本地事实安全收敛。保持 SW-1..SW-6、未归属保护、符号链接与类型冲突检查，不新增媒体版本／发现状态机。

SQLite 驱动已选 `modernc.org/sqlite v1.60.1`（纯 Go、CGO=0），YAML 库已选 `gopkg.in/yaml.v3 v3.0.1`；版本与 go.sum 已提交。race 检测需要 C 编译器，仓库固定 Zig `cc` 入口；交叉编译使用 CGO=0。不能把 CGO=0 构建结论当成 race 或 ARM64 实机运行证明。

生成器通过 STRM 文件交付给 Jellyfin；库扫描、插件决策／探测和媒体播放属于外部链路。协议对照见 [兼容性](compatibility.md)，分层证据见 [测试矩阵](test-matrix.md)。
