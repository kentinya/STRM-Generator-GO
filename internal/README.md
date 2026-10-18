# 目标包边界

`cmd/strmgen` 装配 CLI 与同步入口。`internal/openlist` 负责 API；`pathmap` 负责路径与稳定 URL（纯函数）；`strm` 负责版本 1 协议解析、序列化与比较（纯函数）；`storage` 负责 SQLite 最小归属／轮次；`reconciler` 负责目录对账、有界遍历、跨进程互斥与原子发布；Linux 专用 `securepath` 通过目录句柄执行边界受限的路径操作。

Slice 1 已 PASS / CLOSED，已接受首次生成、最小归属登记、重复跳过、URL／SHA-1／Size 比较与已拥有文件原子更新、同哈希 Size 异常记录、正常中断恢复、dry-run 和跨进程互斥。Task 1.1／1.2 均有独立 B PASS，完整关闭合同见 `930782035615512be811a7b8bcc8416050f0c12f:SLICE.md`。当前 [Slice 2](../SLICE.md) 已 ACTIVE，目标为安全目录镜像与清理恢复，尚无活动实施 Task。Mirror 删除／移动旧路径清理、serve／定时、发布镜像仍未实现，真实外部联调与性能仍 NOT RUN。依赖从接口注入；协议／路径纯函数不执行网络或数据库操作。

`internal/reconciler` 的发布／归属／staging 写入／交换前后屏障在发布二进制中均为空实现；只有显式 `-tags strmgen_testhooks` 的测试二进制会读取环境变量注入受控故障。发布二进制写入完整暂存，不含 hook 环境开关；隔离驱动使用发布二进制实际重启验证恢复。

## 文件归属与故障恢复

- 只有数据库中具有当前 source identity 和精确相对路径映射的文件才算已归属。SQLite 丢失、恢复了不匹配的旧源键、文件已发布但归属写入失败，均不会触发自动接管；已存在但无可信归属的目标保持原样并报告冲突。
- 一轮失败前已经成功发布／登记的文件可以留下；失败源不会提交完整轮次。重新运行时，匹配且未变化的已拥有 STRM 跳过，缺失文件可重新创建；未登记目标仍冲突。Slice 1 无 Mirror 删除分支，不以部分状态推导删除。
- 固定锁文件 `/tmp/strmgen-locks/sync.lock` 由内核 `flock` 协调，文件保留以避免替换活动 inode。进程正常结束、SIGTERM 或 SIGKILL 后锁由内核释放；锁目录由当前有效 UID 所有且不可被组／其他用户写入。不要在进程可能运行时删除锁文件。
- 已拥有更新使用双哈希确定性暂存名，Recover 以独立目录游标分批发现残留；可靠归属与唯一可识别残留可在正常重试中收敛，目录变更 sync 失败不提交 round，未知／歧义／未归属仍保留。
- `LocalFS` 正常失败时会尽力清理 `.strmgen-*.tmp`；SIGKILL／断电可留下该文件。dry-run 对 WAL DB 使用 DB 父目录中的私有 `.strmgen-ro-*` 快照，正常关闭时删除，异常中断可能残留。SQLite `-wal`／`-shm`／`-journal` sidecar 不能在数据库进程仍可能使用时手动清除；未知文件保留并先核实来源。
- 相对 `output`／`database.path` 以启动工作目录为基准。同步需要输出父目录的可遍历和创建权限、已有 STRM 的读取权限、数据库父目录的写入权限，以及可用的固定锁目录；符号链接路径组件被拒绝。
