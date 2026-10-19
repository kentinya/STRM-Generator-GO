# 测试矩阵

下表是目标验证矩阵，不是通过清单；Slice 1、Slice 2 与 Slice 3 均 PASS / CLOSED，Slice 3 的 Task 3.1／3.2 均经独立 B PASS，A 已审查完整范围并关闭。Slice 4「隔离真实服务端到端兼容验收」当前 ACTIVE，Base 为 `d1c95ad079a0e3f952d9e57ab50299c6ae93ad63`，当前 Task 4.1 FIX REQUIRED，执行下方最简方案；Slice 2 的完整证据与 A 决定保留于其关闭检查点，Task 2.2 的 T4 证据索引为 `artifacts/validation-evidence/task-2.2-final-7TWPAV/INDEX.md`（本机证据）。工具层与环境 smoke 不代替产品验收，测试结果不跨层自动继承。

| 范围 | 关键风险／验证 | 等级／阶段 |
|---|---|---|
| 工具与治理 | 原方案身份、链接、合同／SHA／Roadmap、SDK 校验与归档逃逸、假 API 传输／code／分页 | 开发准备工具验证 |
| Go协议 | UTF-8／边界65,536／重复标记／SHA-1／Int64／缺失字段、固定参考插件协议对照 | T2；Slice 4 远程仓库锁定 SHA 插件包实证独立 |
| OpenList | list分页、total变化、不完整响应、外层code失败、超时／权限／存储离线、hash_info／hashinfo | T3；真实部署独立 |
| 路径／URL | 中文、空格、#、%、查询符、前缀、穿越、符号链接、同名冲突、重叠根 | T2纯函数＋T4文件边界 |
| 单轮与增量 | 首次归属、相同内容零写、URL先比较、相同哈希忽略Size、不可比信息、dry-run零副作用 | T4临时目录集成 |
| 持久化／原子写入 | 临时文件／rename／SQLite前后故障、磁盘满、库丢失／损坏、锁竞争、恢复幂等 | T4故障注入与重启 |
| 最终一致性／重试收敛 | SW-1..SW-6：扫描后变化按比较规则下轮纠正；已拥有输出的正常中断／重启恢复；稳定重复运行跳过；独立有界枚举不漏检；真正未归属保护，不要求 UID／权限矩阵或消费者 OS 拒写 | Slice 1 本地 T4 与 A Slice Final PASS；外部服务独立 NOT RUN |
| Mirror／配置／轮次 | 默认清理关闭、mirror false 与清理冲突、默认／最低阈值 3、三轮前不具备最低资格、重复 epoch 不计数、列表／本地失败、分页不全、整轮取消／预览不增加证明、skip 算出现、失败前再出现仍失效、开关与多源隔离、源根不可用与移除不扫旧记录 | Slice 2 RO-1／RO-2，T4；Task 2.2 PASS |
| Mirror／Get判定／移动 | 源根 Get 未读到保留且不增加计数；源根可读、目标 Get 未读到计缺失，不再细分失败原因；父目录删除走相同 Get 规则；再出现清零／类型冲突保留；移动新路径处理失败不删旧路径；所有列表 refresh:false | Slice 2 RO-3／RO-4，T4逐文件断言 |
| Mirror／持久化／恢复 | 旧库／映射缺失／历史轮次不追认、归属／输出边界／符号链接／类型／未知暂存保护、删除／目录sync／DB／SIGKILL窗口、跨文件部分完成后停止与重试收敛、原库及预览零写、观察和候选有界 | Slice 2 RO-5／RO-6，T4故障及发布二进制重启 |
| OpenList Get／缓存实证 | 固定隔离版本／驱动的源根存在／缺失、目标存在／未读到、父目录删除、相同错误响应与缓存滞后；所有列表含分页不强刷；隐藏／虚拟挂载／缓存文件读取反例属用户接受局限，不冒充已辨别成功 | Slice 2 RO-6 必要实证；Task 2.1 已独立 B PASS，仅远端判定层；实际 CLI 清理由 Task 2.2 验证 |
| 调度／并发 | 手动与serve／多进程排他、有界 tick 与工作池、忙碌跳过、停止不提交完整轮、恢复下一轮 | Slice 3 RO-2／RO-3，T4 race＋多进程集成；Task 3.1 B PASS，含 scheduler 单元、隔离 serve 驱动及独立 SIGINT 复现 |
| 性能 | 十万／百万、宽目录／深目录、512MB第一档压力点、RSS／队列／吞吐、失败和恢复 | 计划 Slice 5 真实测量；512MB 不作为未验证的产品保证 |
| 发布 | Linux amd64／arm64构建、Docker权限、token／输出／DB／锁挂载、停止／重启与包恢复 | Task 3.2 的 Dockerfile／Compose／双架构镜像与隔离容器验收经 B 独立 PASS；用户保留环境 token 注入与 `read_only`，Compose 脱敏采集使用 `config --no-interpolate`。本机 amd64 容器实测，arm64 仅 OCI manifest／ELF 核对，原生 ARM64 实机留计划 Slice 5 |
| 联调 | 目标 OpenList 驱动与稳定无签名 `/d/` → 原始 STRM → 插件远程仓库锁定 SHA 构建的包解析／决策 → 隔离 Jellyfin 原生扫描与实际播放 | Slice 4 ACTIVE；要求记录远程源码／构建／产物身份和隔离服务版本，使用合成媒体并分层保留原始证据 |

真实插件行为按当前 Slice／Task 核对当次播放地址、必要媒体流和探测／重用；既有要求的重启回归按输入匹配复用，不额外要求把所有外部场景再做一轮重启；原生 Jellyfin 客户端实际播放必须有会话／进度证据，curl 或仅请求 `/d/` 不算播放。Slice 4 不启动或验证代理。同步成功不等于播放成功。清理安全须检查最终文件／库状态，不能仅依据日志计数。Slice 4 的外部服务必须是专用隔离实例；生产端点、配置、媒体和进程不在准入范围。

## 测试代码职责与治理（2026-10-19，B）

按用户“先说明功能，再删除调整不需要的测试”盘点真实代码。此次只治理测试，不改产品、
集成 runner 或 Slice 合同；不为减少数量删除唯一行为断言。跨层同名测试不等于重复：
纯函数验证规则，真实文件／发布 CLI 验证接线与持久化，外部服务验证兼容。

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

### Python 工具测试：8 个文件

默认由 python3 scripts/quality.py 内的 unittest discovery 运行；若只改 Python 测试，
可单独 python3 -m unittest discover -s tests -p 'test_*.py' -v，不重复跑两遍。
这些用例不启动真实 OpenList／Jellyfin，也不构成原生播放 PASS。

| tests 下文件 | 验证功能 | 本轮前→后方法数 |
|---|---|---|
| test_development_environment.py | SDK 压缩包越界／链接拒绝、校验和、可执行权限与本地缓存／平台 | 7→7 |
| test_governance.py | 原文／必读文档、链接、状态、真实 SHA、已提交合同与 Task 检查点 | 14→14 |
| test_mock_openlist.py | 合成 HTTP 服务分页／鉴权／错误／元数据形状、故障钩子与测试身份轮换 | 11→11 |
| test_run_openlist_absence_integration.py | 真实服务 runner 的目录／参数／镜像前置、失败不冒充 skip、归属清理及简化 Get 约束 | 35→35 |
| test_run_isolated_sync_integration.py | 同步 runner 的证据边界、STRM 断言、DB／sidecar、确定性故障 hook 与 Mirror 场景登记 | 20→19 |
| test_run_serve_integration.py | 定时 runner 的证据边界、结果计数、场景登记和 testhooks 隔离 | 10→9 |
| test_run_container_integration.py | 容器 runner 的证据边界、ELF／DB、镜像构建约束、Compose 环境／脱敏 | 15→13 |
| test_run_isolated_target_integration.py | 目标 runner 的凭据／stdin／证据、STRM 读取、摘要、日志窗口／item 和库路径映射 | 36→31 |

实际发现总数 148→139；历史报告的“目标 34 项”是当时报告，不覆盖本轮盘点的 36 项。

### 五个集成驱动：全部保留，不等于工具单测

| scripts 下文件 | 实际运行验证 |
|---|---|
| run_isolated_sync_integration.py | 发布 CLI 配合合成 API：首次／比较、未归属／路径、dry-run、DB、跨进程、故障与 Mirror 清理／恢复 |
| run_openlist_absence_integration.py | 固定真实隔离 OpenList + Go 集成测试：源根／Get、分页、缓存、权限和挂载限制，可另接 CLI Mirror |
| run_serve_integration.py | 发布／testhooks CLI 多进程：首次、tick／busy、失败继续、取消与诊断脱敏 |
| run_container_integration.py | 真实隔离 Docker／Compose：镜像静态、最小运行、serve 生命周期、共享锁、挂载及可选 arm64 OCI（非实机） |
| run_isolated_target_integration.py | 已固定 S3/RustFS→CLI→稳定字节→插件→Jellyfin；当前按单场景或人工观察，三项暂缓不重跑 |

quality.py 是质量聚合入口，不是第六条外部链路。故障注入 hook 是测试支撑，默认产品构建不启用；
不能因工具单测已过就推定发布二进制、容器或真实服务已过。暂缓仅停止执行指定真实场景，
不删除相应代码或产品比较／移动安全回归，也不删除此前失败证据。

### 本轮删除／调整映射

| 原检查 | 处置与理由／替代覆盖 |
|---|---|
| 三个 driver 测试中的 test_summary_shape_is_json_serializable | 删除 3 项：只对测试自建字典做 json.dumps/loads，不调用 runner，不验证其真实 summary；各集成实际结果／summary 门禁保留 |
| container 的 test_release_binary_has_no_testhooks_markers | 删除 1 项：实际上只读 serve hook 源码，与 serve 的默认／integration hook 两项重复；容器实际产物 marker 检查仍保留 |
| target 的 test_unsigned_url_has_no_query | 删除 1 项：只读取自己写入的无查询 URL，不验证签名拒绝；sync 的 unsafe URL 拒绝断言及 target metadata 对实际 STRM 的无签名断言保留 |
| target 的 test_require_raises_failure | 删除 1 项：通用 if／raise 包装的独立测试；凭据、日志等真实拒绝分支继续验证 Failure |
| target 的 test_sha1_absent_is_none | 合入 version／size／URL 读取测试，保留缺 SHA-1 为 None 断言；不减少输入语义覆盖 |
| target 的 test_normalize_item_id | 合入实际 require_reuse 测试，以大写／无连字符 ID 对照固定小写期望，不再以被测函数算期望 |
| target 的 test_no_tmp_scenario_file_is_created | 合入 stdin 场景交换：mock 拒绝 write_text／NamedTemporaryFile，保留 argv 无口令／stdin 有值；不再扫描宿主共享 /tmp（并非所有写 API 的绝对无落盘证明） |
| target 摘要自比较、口令参数拒绝与日志窗口 | 改强：两个独立同内容树／反向创建；先提供完整合法参数再检验未知口令选项；先放旧有效决策再确认只见新决策 |

清掉无用 import／空操作，纠正“普通 compose config 不泄露”的旧注释和“汇总已关联输入”的
过强测试命名。未修改断言以隐藏既有凭据出口缺陷；已复现出口修正仍在 Task 4.1 待办。
本轮 B 作为测试修改作者不签自己的独立行为审查 PASS，Task／Slice 状态不变。

## Slice 4 最简执行矩阵（用户授权，2026-10-19）

| 必验／暂缓 | 最小动作与证据 | 责任 |
|---|---|---|
| 固定身份／安全 | 核对已部署输入；修实际凭据出口并保留事件处置；不建通用秘密框架 | Developer 受影响修正；B 审查 |
| 原始 CLI 链路 | 201 项分页、五类名称、字节一致、首轮／重复零写复用匹配证据；B 独立扫描／播放一份已有合成媒体及核对对应重用 | B |
| 同 SHA-1／Size 异常 | 本地真实 A 哈希，标注 STRM 副本只改 Size 元数据；正常插件观察复用／不探测 | B；真实插件仍 NOT RUN |
| 不同 SHA-1 | 同 URL 服务真实 B，使用 B 实际哈希／Size；观察对应条目启动探测，接受／播放另报 | B；真实插件仍 NOT RUN |
| 移动生成／清理／恢复 | 核对既有匹配证据；失败停止，核对恢复或弃用夹具，不强制通用自动补偿 | B |
| 用户暂缓三项 | URL-change、hash 不可比 Size-change、移动首播：保留 BLOCKED，不调查／修复／重跑，不计 PASS | A 后续处置 |
| 完整 T4／Final | 当前 Task 列出的全量门禁逐项匹配证据，只运行缺失／变更项；完整 Base..Head 审查 | B，不交回 Developer |

派生 SHA-1 输入不冒充原始 CLI／目标 OpenList 元数据；Go 比较通过不能代替实际插件。现有 target runner 的 acceptance 包含移动，all 还包含两项暂缓，不作为本轮一键门禁；用已有单场景或人工观察。产品旧回归中同名用例不因此删除。测试执行时间须小于本轮总时间 50%，不得靠空等凑比例；不能完成的门禁如实留未运行。

2026-10-18 用户最终指令撤回此前权威刷新列表、最终 500 有限例外及刷新预算方案。B 须显式重映射旧用例：源根不可读保留、不计数；源根可读时目标未读到（含权限、应用码／HTTP 错误、驱动／缓存及单请求失败）按相同缺失观察；目标成功／类型冲突保留；所有 list 请求 refresh:false。整轮取消／deadline、列表失败／不完整分页、本地 I/O／DB 故障仍不提交新计数；三轮阈值、再出现清零、dry-run 零写及本地安全／恢复门槛保持。

真实缓存滞后与同码反例保留原始事实，不要求证明不会因这些原因计数，也不能改称旧安全问题已修复。新候选需按新 AC 重跑相应完整 T4 与固定真实服务。详见[需求](requirements.md#简化删除判定2026-10-18-用户最终决定)。

2026-10-18 A 澄清撤回额外 UID／mode 校验、消费者 OS 拒写门槛和外部注入残留永久保全要求。F-10／F-13 及外部来源 F-11 保留历史观察，B 显式映射为越界诊断或现有有益防御回归；不能仅改成静态 legacy 初始状态继续强求三次重启永久冲突。若同类问题能在无外部干预的正常生成器流程产生，仍属应修缺陷，须补确定性回归并验证后续收敛；确实无可靠归属的文件仍保留。F-14、正常系统调用失败／信号／重启、dry-run、完整轮规则与完整 T4 不降低。调整现有测试须保留历史证据、说明替代覆盖，不隐藏 skip／首次失败。真实外部联调仍按原合同独立执行。
