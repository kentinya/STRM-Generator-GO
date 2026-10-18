# 仓库开发约束

项目：STRM Generator Go，OpenList 到本地 STRM 的独立生成器，配套 jellyfin-strm-probe V4.3。

开始工作先读本文件、[开发流程](docs/development-workflow.md)、[V4.3 原方案](<STRM Generator Go.md>)、[范围](docs/scope.md)、[需求](docs/requirements.md)、[用户旅程](docs/product-experience.md)、[架构](docs/architecture.md)、[路线图](docs/roadmap.md)、[当前 Slice](SLICE.md) 与 [当前 Task](TASK.md)。按职责读 [A](docs/agent-experience/slice-owner-architect-final-reviewer.md)、[B](docs/agent-experience/task-planner-reviewer.md)、[Developer](docs/agent-experience/developer-implementation-role.md) 指南。

先检查 `git status --short --branch`，保留已有改动；实施前运行 `python3 scripts/check_governance.py`。无 HEAD 的准备阶段可使用 `--bootstrap`，不据此开始产品实施。开发准备已完成，用户已明确要求激活第一个 Slice；当前范围以 Slice 1 合同及后续 Task 为准。先提交 ACTIVE 合同，再由 B 规划 Task，实施不越过 Task 准入。用户会话授权优先于本文件。

## 固定业务边界

- 原方案原样保留；修改需求须显式协调需求、架构、合同及测试，不能由下层文档悄悄改变。
- 只生成／更新／安全清理自己拥有的 STRM；不运行 ffprobe、不下载媒体计算哈希、不操作 Jellyfin 数据库、不调用插件内部接口、不改写 JellyfinProxy。
- 使用 UTF-8 版本 1 协议、稳定 HTTP(S) URL；SHA-1 必须是真实内容的 40 位十六进制哈希；Size 为非负 Int64 字节数。缺失或无效元数据省略，不写 Modified、不使用签名临时播放 URL，总长最多 65,536 字节。
- URL 变化必须更新，即使 SHA-1／Size 相同；URL 不变且有效 SHA-1 相同则忽略 Size 差异并记录异常；SHA-1 不可比时比较有效 Size。详细缺失信息规则见需求。
- Directory Reconciler + Mirror + 最终一致性；不另造文件发现或媒体版本状态机。SQLite 仅记录归属、路径映射、完整对账轮次与连续缺失次数，媒体指纹从 STRM 读取。
- 删除必须有连续两次完整成功对账缺失、再次明确确认不存在、可靠归属及输出边界证据。失败、超时、权限问题、分页不完整不得当作不存在；任何不完整轮次不推进清理依据。
- dry-run 不改 STRM、数据库、缺失计数或轮次；允许输出脱敏日志。不能用 dry-run 结果作为下一轮删除依据。
- 只在已配置输出根内操作；拒绝目录穿越、符号链接逃逸、输出重叠及同名目标冲突。不覆盖或删除未拥有的文件。
- 同一输出范围禁止同步轮次重叠，包含手动／定时与多进程；轮内目录并发有界，不一次加载全库对象或创建每文件 goroutine。
- 本地输出按生成器单写者、消费者只读的正常使用方式设计；多生成器共享同一互斥资源。不保证绕过锁的外部程序在操作间隙任意改写／替换本地对象时仍绝不误操作，也不开发 UID／权限加固系统或强制只读消费者实证。已拥有、可重新生成的 STRM 出现暂时偏差，按现有比较规则和正常恢复流程在后续成功同步中收敛；不自动认领、覆盖或删除真正未归属文件。详见需求 SW-1..SW-6。
- 扫描期间变化由下一轮纠正，不要求每次写入前再请求远端，不引入远程版本锁。

## 工作与证据

Slice／Task 状态和 Git 检查点仅由开发流程定义。A／B／Developer 是职责，不自动授权多代理；只有用户或适用指令明确要求才委派。实施者不得签发自己的独立 B 审查或 A 最终审查 PASS。

保留用户文件与真实状态。已授权范围内持续完成必要实现、修复和隔离验证；不自动推送、不重写历史、不破坏性清理。只操作本仓库 `artifacts/` 及专用隔离夹具，不修改 `/opt/jf-strm-probe`、运行中的 Jellyfin、OpenList、代理和生产媒体。Git 身份缺失时如实记录，不捏造用户身份或提交。

单元、夹具、真实 OpenList、插件解析、真实 Jellyfin／代理和性能证据分开报告。NOT RUN／BLOCKED／SKIPPED 不能写成 PASS，环境 smoke 不代表产品通过。日志不包含真实 token、Cookie、签名 URL 或私有完整路径。
