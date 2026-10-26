# 仓库开发约束

STRM Generator Go 是 OpenList 到本地 STRM 的独立生成器。先读本文件、
[需求](docs/requirements.md)、[架构](docs/architecture.md)与[开发流程](docs/development-workflow.md)；
实施范围和状态见 [SLICE](SLICE.md)／[TASK](TASK.md)。用户当前会话授权优先。

## 工作方式

- 开始先检查 `git status --short --branch`，保留用户改动，运行 `python3 scripts/check_governance.py`。
- 在用户授权范围内完成实现、修复和必要验证；不自动 push、不重写历史、不捏造 Git 身份。
- 本仓库 Git 提交统一使用用户指定的 `kentinya` 身份，邮箱沿用该身份最近提交的 GitHub noreply 地址。
- A／B／Developer 是职责，不自动启动多代理；实施者不得自签独立 B／A PASS。
- Slice 1–4 已 PASS / CLOSED；用户于 2026-10-26 授权规划 Slice 5 自定义扩展名，
  当前 ACTIVE，任务状态见 TASK。先由 A 提交 ACTIVE 合同，再由 B 提交正式 Task，
  Developer 才开始实施；独立产品审查要求不变，不自动扩展范围。
- 本次文档维护采用 10 份文件结构，入口见 [README](README.md)。重复说明合并，已完成任务的过程记录从 Git 历史查阅，不再复制归档目录。

## 业务不变量

- 只维护可靠归属的 STRM；不覆盖、认领或删除未归属文件。SQLite 只保存归属、路径、完整轮次和缺失计数，媒体指纹从 STRM 读取。
- V1 UTF-8、稳定 HTTP(S) URL、最多 65,536 字节；真实 SHA-1 为 40 位十六进制，Size 为非负 Int64；无效字段省略，不写 Modified 或签名临时 URL。
- URL 变化必更新；同 URL 且有效 SHA-1 相同则跳过并记录 Size 异常；哈希不可比才比较有效 Size，信息不足不伪造变化。
- 所有列表 `refresh:false`。源根未成功读取为目录时冻结缺失；源根可读而目标 Get 未读到即计缺失，不区分相同响应的其他原因。至少三次完整成功轮及删除前重核后才清理。
- Slice 5 规划按用户 2026-10-26 最新指令新增：被当前扩展名列表排除的已归属输出也走
  正常计数／清理，即使源文件仍存在；沿用清理开关、源根门槛、完整轮阈值与删除前重核。
  此增量尚未实施，不回写 V4.4 历史验收，具体合同见 SLICE。
- 整轮取消、列表不完整或必要本地处理失败不推进缺失计数。dry-run 不改 STRM、原数据库、轮次或计数，不为后续删除提供新证据。
- 拒绝穿越、符号链接逃逸、输出根重叠和同名冲突；同输出范围跨进程互斥，目录与数据库处理有界，不加载全库或创建每文件 goroutine。
- 正常部署由生成器单写、消费者只读，多生成器共享同一锁。正常失败与已拥有文件的可识别偏差应在后续成功同步中收敛；不承诺抵御绕过锁的任意外部写入，不建设 UID／权限加固系统。
- 采用目录对账、Mirror 与最终一致性，不另造媒体版本／发现状态机或远端版本锁；扫描期间变化由后续轮纠正。
- 不运行 ffprobe、不下载媒体补算哈希、不操作 Jellyfin／插件数据库、不调用插件内部接口、不改写 JellyfinProxy。

## 环境与证据

只操作本仓库 `artifacts/` 和明确授权的专用隔离夹具；不修改配套仓库、运行中的生产
Jellyfin／OpenList／代理或生产媒体。历史授权不用于新操作，不从旧文件恢复秘密。
Slice 4 的临时认证已撤销，历史 UNKNOWN 与用户接受不改写为当时已授权或无泄露证明。

单元、合成夹具、真实 OpenList、Jellyfin 扫描／插件实际决定和规模测量分层报告。
NOT RUN／BLOCKED／SKIPPED 不写成 PASS，探测启动不等于探测成功。
不输出真实 token、Cookie、签名 URL 或私有完整路径。测试等级、证据和历史查看命令见
[开发流程](docs/development-workflow.md)。
