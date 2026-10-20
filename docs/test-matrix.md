# 测试矩阵

当前依据为 [V4.4](<../STRM Generator Go.md>)。Slice 1–3 PASS / CLOSED；[Slice 4](../SLICE.md)
在原 Base 内 ACTIVE。4.4固定10000本地／S3对象已验收，原生任务可用性F-06仍BLOCKED；4.5／4.6尚未实施，
当前结果由新任务取得；所需环境／媒体／三脚本见 [资产清单](slice-4-assets.md)。本表定义当前覆盖与执行入口，
旧任务记录不作为验收完成或审查前置。

| 范围 | 必须覆盖 | 入口／证据层 |
|---|---|---|
| 工具治理 | 版本与摘要、必需文件／链接、已提交合同、真实 SHA、SDK 安全解压 | check_governance.py、development_environment.py、Python 单测 |
| V1／URL／比较 | UTF-8／65536、SHA-1／Int64、元数据缺失、稳定 URL 编码、URL→SHA-1→Size | Go strm／pathmap／openlist；不代替实际插件决策 |
| 单轮生成 | 新增／更新／零重写、归属／路径、dry-run、锁、故障与正常恢复 | Go 回归及 run_isolated_sync_integration.py |
| Mirror／清理 | 源根 Get 保留、目标未读到计缺失、refresh:false、三完整轮／重核、再出现、多源与中断恢复 | Go 回归、同步驱动；真实 Get／缓存另用 run_openlist_absence_integration.py |
| 调度／部署 | 首轮／tick／busy、取消、共享锁、挂载／重启、发布包与 testhooks 隔离 | run_serve_integration.py、run_container_integration.py；amd64 实测与 arm64 构建分开 |
| 资产／固定数据集 | 既有环境、媒体及三脚本保留；一次补齐固定 10000 个对象和本地清单 | RO-1／RO-2；Task 4.4，现有 S3／容器工具，后续只复用 |
| 万文件生成 | 真实 OpenList／发布 CLI → 10000 个 STRM、映射／协议／URL、重复零重写、耗时／峰值 RSS | RO-3；Task 4.5 记录实际值，无十万／百万／512 MB 压测门槛 |
| 最少样本同步 | 同批样本新增／更新／跳过／移动／清理／再出现／dry-run，最后核对恢复 | RO-3；Task 4.5 真实源缺少的元数据分支用 Go 回归另报，不建元数据注入服务 |
| 原始产物扫描 | 原始生成字节、最少媒体库条目、固定包实际读取 | RO-4；Task 4.6 Jellyfin 原生扫描／检查，不能只看任务完成 |
| 插件比较 | 下表每行当次 item／日志窗口的实际探测或明确复用 | RO-5；Task 4.6 正常原生检查，标注副本与生成器输出分开，无旧暂缓例外 |
| 审查 | 当前工具验证、输入匹配复用、完整范围与关键真实链路独立复现 | RO-6；Developer 不自签 B／A PASS |

单元、合成 API、真实 OpenList、插件解析、Jellyfin 扫描／决策、规模测量分层记录。
无配置的 Go 真实服务测试会 SKIP；普通 go test 不能算真实服务 PASS。无签名 URL 的字节
验证只证明该输入，不能推断客户端播放。当前合同无播放／会话／进度、代理、探测结果成功
或 ARM64 实机／正式发布门槛；部署配置的 512m 限额不是新增性能承诺。

## Slice 4 实际插件决策矩阵

哈希／Size 比较和不探测行先建立必要主媒体流、历史指纹及当前匹配的原生地址。
D-05 在旧基准匹配后改变 URL，可由 UrlChanged 或此次变化导致的 NativeUrlMismatch
触发探测；无关主流／地址缺失不能充当该行证据。每行先恢复并核对比较基准，避免上一行
污染下一行；仅改变该行所列条件，异内容的真实 Size 随内容匹配。实际探测启动即可算探测，
复用必须有明确记录且理由匹配；缺日志、FirstTrust 或仅扫描完成均不足。

| ID | 单项变化（其余条件相同） | 生成器要求 | 实际插件预期 |
|---|---|---|---|
| D-01 | 输入均不变 | 跳过、字节／mtime 不变 | 明确复用／不探测 |
| D-02 | 远端仅 Modified 变化 | 跳过，不写 Modified | 明确复用／不探测 |
| D-03 | 有效 SHA-1 相同、Size 元数据不同 | 跳过并记录异常（Go 覆盖） | 忽略 Size，明确复用 |
| D-04 | URL 不变，实际内容及有效 SHA-1 改变 | 更新（目标缺 hash 时 Go 另报） | 探测 |
| D-05 | URL 改变，SHA-1／Size 不变 | 必须更新 | 探测 |
| D-06 | URL 不变，SHA-1 不可比，两侧有效 Size 不同 | 更新 | 探测 |
| D-07 | URL 不变，SHA-1 不可比，两侧有效 Size 相同 | 跳过并说明未比较哈希 | 明确复用 |
| D-08 | URL 不变，SHA-1 不可比，Size 信息不足 | 不推断变化，记录限制 | 明确不因信息不足而探测 |

D-03 可用同批本地原件的真实 SHA-1 与标注异常 Size 副本；D-04 的新内容、SHA-1、Size
和实际服务字节须一致。D-06..08 明示历史／当前哪侧 SHA-1 或 Size 缺失／无效。
原始 CLI 文件单独留存用于 RO-4，副本不作为生成器同步通过证据。D-02 只有源端 Modified
变化，STRM 中不增加该字段。全部行实际证据待下一验收 Task，不能由旧例外豁免。

## 当前工具清单

### Go 产品测试：16 个文件，保留

下列路径相对仓库根；默认命令是 bash scripts/go-local test ./...。真实 OpenList 文件
另需显式隔离配置，不能把未配置时的 skip 说成真实服务 PASS。

| 测试文件 | 验证功能 |
|---|---|
| cmd/strmgen/main_test.go | CLI 参数／退出码、首次同步、dry-run、serve 间隔前置校验与脱敏错误 |
| internal/config/config_test.go | YAML／未知字段、token 环境、路径／根重叠、URL、清理／调度配置 |
| internal/openlist/openlist_test.go | HTTP／外层 code、分页完整性、响应限制、hash／size 与取消 |
| internal/openlist/absence_test.go | 源根保留、目标未读到即缺失、类型／取消、refresh:false 和并发独立性 |
| internal/openlist/openlist_integration_test.go | 固定真实隔离 OpenList 的 Get／缓存／权限／挂载行为；无配置不运行 |
| internal/pathmap/pathmap_test.go | 扩展名、相对路径、非法名称与稳定 URL 逐段编码 |
| internal/strm/strm_test.go | V1 读写、UTF-8／大小边界、可选字段和 URL→SHA-1→Size 比较矩阵 |
| internal/reconciler/syncer_test.go | 首次／重复／更新／dry-run、归属、冲突、源根门槛、有界遍历与失败轮 |
| internal/reconciler/cleanup_test.go | 连续完整轮、阈值／重核、再出现、旧映射、多源隔离、移动与清理恢复 |
| internal/reconciler/fs_test.go | 原子创建／更新、目标类型、暂存／失败窗口、锁与未归属保护 |
| internal/reconciler/fs_delete_test.go | 仅删已拥有 regular STRM、拒绝类型／暂存、unlink／sync／取消边界 |
| internal/reconciler/recover_convergence_test.go | 正常中断／重启收敛、有界目录枚举、暂存歧义、变化后恢复及 dry-run |
| internal/storage/storage_test.go | SQLite 归属／轮次、只读零写、WAL 快照、特殊路径及符号链接拒绝 |
| internal/storage/storage_round_test.go | 旧库升级、schema 拒绝、每 epoch 一次计数、溢出、有界候选批次 |
| internal/securepath/securepath_linux_test.go | 绑定目录句柄、祖先符号链接、根替换与输出／DB 物理边界 |
| internal/scheduler/scheduler_test.go | 首轮即执行、固定间隔、busy／失败继续、无积压、取消与 fatal |

### Python 工具测试：7 个文件

`python3 scripts/quality.py` 聚合 unittest、AST、Bash、空白、gofmt、vet、Go test／build。
Python 工具单测不启动真实外部服务，不替代四个驱动的实际运行。

| tests 下文件 | 覆盖 | 当前方法数 |
|---|---|---|
| test_development_environment.py | SDK 校验／解压／缓存边界 | 7 |
| test_governance.py | 当前方案身份、文档链接、Slice／Task 状态和提交边界；版本漂移／缺失／协调升级 | 17 |
| test_mock_openlist.py | 合成 API 鉴权／分页／错误／元数据及故障 hook | 11 |
| test_run_openlist_absence_integration.py | 固定真实服务驱动前置、失败／SKIP、归属清理与 Get 约束 | 35 |
| test_run_isolated_sync_integration.py | 发布 CLI 驱动证据边界、STRM／DB、故障与 Mirror 场景 | 19 |
| test_run_serve_integration.py | 调度驱动证据边界、计数、场景与 hook 隔离 | 9 |
| test_run_container_integration.py | 容器驱动证据边界、ELF／DB、镜像与 Compose 脱敏 | 13 |

### 四个集成驱动：保留

| scripts 下文件 | 适用 Slice／用途 |
|---|---|
| run_isolated_sync_integration.py | Slice 1–2：CLI、比较、锁、归属、故障、清理／恢复 |
| run_openlist_absence_integration.py | Slice 2：固定隔离 OpenList + Go 真实 Get／缓存／权限／挂载限制 |
| run_serve_integration.py | Slice 3：发布／testhooks CLI 调度、多进程与取消 |
| run_container_integration.py | Slice 3：隔离 Docker／Compose、挂载／生命周期、共享锁、可选 arm64 OCI |

mock_openlist.py、dev/fixtures/openlist.json、dev/compose.yaml 是 Slice 1–3 的合成服务支撑；
所有既有 Go 测试与默认关闭的 testhooks 均保留。跨层测试分别验证规则、装配和真实兼容，
不能仅因名称相同删除。真实服务驱动的隐藏／同码／虚拟挂载案例保留为用户接受的局限，
不改称已权威辨别。正常中断恢复、未归属保护和 dry-run 等安全要求保持。

当前 Python 共 7 个文件、111 个测试方法；执行结果以新任务报告为准。
旧目标运行器 `run_isolated_target_integration.py` 及其专属测试已不在当前工具清单，
后续使用发布 CLI、已有 S3／容器工具和 Jellyfin 原生检查，不恢复播放运行器。

### Slice 4 明确保留的三个辅助脚本

`artifacts/task-4.1/isolated-s3/` 下的 generate_media.py、bootstrap.sh、teardown.sh
全部保留复用；准确行为／摘要见 [资产清单](slice-4-assets.md)。生成脚本原版写 201 个
占位文件；4.4 已留原版并最小适配为用真实原件补缺失、拒绝覆盖／冲突、拒绝静态链接与
未知前缀对象，不能直接重跑覆盖已有真实媒体。启动／停止按需使用既有隔离实例，不能为
工具验收重建或删数据。保留脚本不恢复播放门槛。

新顺序为 4.4 资产复用与固定数据集 → 4.5 万文件／少量同步 → 4.6 原始扫描／完整决策。
4.4固定10000本地／S3对象、最终清单与工具修正已由B3接受；原生任务入口认证／可用性
F-06仍BLOCKED，Task保持FIX REQUIRED。4.5／4.6尚未实施；不继承旧任务审查或修复要求。
