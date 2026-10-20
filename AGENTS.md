# 仓库开发约束

项目：STRM Generator Go，OpenList 到本地 STRM 的独立生成器；需求版本为 V4.4，协议／决策规则与配套 jellyfin-strm-probe 锁定源码对照。

开始工作先读本文件、[开发流程](docs/development-workflow.md)、[V4.4 方案](<STRM Generator Go.md>)、[范围](docs/scope.md)、[需求](docs/requirements.md)、[用户旅程](docs/product-experience.md)、[架构](docs/architecture.md)、[路线图](docs/roadmap.md)、[当前 Slice](SLICE.md) 与 [当前 Task](TASK.md)。按职责读 [A](docs/agent-experience/slice-owner-architect-final-reviewer.md)、[B](docs/agent-experience/task-planner-reviewer.md)、[Developer](docs/agent-experience/developer-implementation-role.md) 指南。

先检查 `git status --short --branch`，保留已有改动；实施前运行 `python3 scripts/check_governance.py`。Slice 1–3 已 PASS / CLOSED，Slice 3 A 已审至 `410fc0e0a8964046de4707d58c0c7505385c6002`。Slice 4 在原 Base `d1c95ad079a0e3f952d9e57ab50299c6ae93ad63` 内按用户本轮授权完全重规划：保留复用已有隔离环境、媒体原件及 generate_media.py／bootstrap.sh／teardown.sh，固定 10000 个 S3 文件，少量同步及原始扫描／完整插件决策。新顺序为 Task 4.4 资产复用与固定数据集 → 4.5 万文件生成／最少同步 → 4.6 原始扫描／完整决策。旧 4.1／4.2／4.3 不再是当前任务，不继承其待审或修复要求；只保留后续需要的环境、媒体、脚本、固定产物和适用工具。新 4.4／4.5／4.6 均未实施，不能用旧任务记录抵扣验收。当前实施仅以 SLICE／TASK 为准，无播放、代理、旧暂缓例外或更高规模压力门槛。用户会话授权优先于本文件。

## 固定业务边界

- 当前方案为 V4.4，版本与内容摘要由 dev/environment-spec.json 锁定；用户授权更新时显式协调需求、架构、合同及测试，不保留退出要求作为当前兼容门槛。
- 只生成／更新／安全清理自己拥有的 STRM；不运行 ffprobe、不下载媒体计算哈希、不操作 Jellyfin 数据库、不调用插件内部接口、不改写 JellyfinProxy。
- 使用 UTF-8 版本 1 协议、稳定 HTTP(S) URL；SHA-1 必须是真实内容的 40 位十六进制哈希；Size 为非负 Int64 字节数。缺失或无效元数据省略，不写 Modified、不使用签名临时播放 URL，总长最多 65,536 字节。
- URL 变化必须更新，即使 SHA-1／Size 相同；URL 不变且有效 SHA-1 相同则忽略 Size 差异并记录异常；SHA-1 不可比时比较有效 Size。详细缺失信息规则见需求。
- Directory Reconciler + Mirror + 最终一致性；不另造文件发现或媒体版本状态机。SQLite 仅记录归属、路径映射、完整对账轮次与连续缺失次数，媒体指纹从 STRM 读取。
- 删除采用用户明确简化的规则：配置源根未成功读取为目录时保留，不增加缺失次数；源根可读取时，目标 Get 未读到即按缺失处理，不再辨别其他原因造成的相同响应。所有列表请求均使用 refresh:false。连续三次完整轮缺失、当前相同 Get 判定、可靠归属与输出边界仍是删除条件；整轮取消、列表分页不完整或本地必要处理失败不推进计数。详见需求的简化删除判定。
- dry-run 不改 STRM、数据库、缺失计数或轮次；允许输出脱敏日志。不能用 dry-run 结果作为下一轮删除依据。
- 只在已配置输出根内操作；拒绝目录穿越、符号链接逃逸、输出重叠及同名目标冲突。不覆盖或删除未拥有的文件。
- 同一输出范围禁止同步轮次重叠，包含手动／定时与多进程；轮内目录并发有界，不一次加载全库对象或创建每文件 goroutine。
- 本地输出按生成器单写者、消费者只读的正常使用方式设计；多生成器共享同一互斥资源。不保证绕过锁的外部程序在操作间隙任意改写／替换本地对象时仍绝不误操作，也不开发 UID／权限加固系统或强制只读消费者实证。已拥有、可重新生成的 STRM 出现暂时偏差，按现有比较规则和正常恢复流程在后续成功同步中收敛；不自动认领、覆盖或删除真正未归属文件。详见需求 SW-1..SW-6。
- 扫描期间变化由下一轮纠正，不要求每次写入前再请求远端，不引入远程版本锁。

## 工作与证据

2026-10-20 用户会话授权：Task 4.4 开发者可从本任务既有专用隔离容器的 Env 读取完成验收所需凭据，并仅在内存／子进程环境中使用。读取前核对 Compose project、服务、镜像及挂载身份，禁止读取生产或其他项目容器。不得打印、记录、提交完整 Env 或秘密，不写入配置／证据文件，不从历史文件恢复秘密。本授权替代此前禁止从容器 Env 取凭据的限制；不表示凭据已取得、访问已成功或 F-01 已关闭，不改变其他验收条件。

Slice／Task 状态和 Git 检查点仅由开发流程定义。A／B／Developer 是职责，不自动授权多代理；只有用户或适用指令明确要求才委派。实施者不得签发自己的独立 B 审查或 A 最终审查 PASS。

保留用户文件与真实状态。已授权范围内持续完成必要实现、修复和隔离验证；不自动推送、不重写历史、不破坏性清理。只操作本仓库 `artifacts/` 及专用隔离夹具，不修改 `/opt/jf-strm-probe`、运行中的 Jellyfin、OpenList、代理和生产媒体。Git 身份缺失时如实记录，不捏造用户身份或提交。

单元、夹具、真实 OpenList、当前源码插件解析、真实 Jellyfin 原生扫描／插件逐项决策及万文件测量证据分开报告。Slice 4 不运行或验证代理。NOT RUN／BLOCKED／SKIPPED 不能写成 PASS，环境 smoke 不代表产品通过。日志不包含真实 token、Cookie、签名 URL 或私有完整路径。
