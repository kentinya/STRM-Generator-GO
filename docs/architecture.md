# 架构：CURRENT 与 TARGET

## CURRENT

开发准备基线(合同、需求、架构、夹具、Python 工具与 Go 1.27.2 固定 SDK)已交付。Slice 1 的 Task 1.1 有 `cmd/strmgen` 的 `sync` 与 `sync --dry-run` 实现;`internal/config` 严格解析 YAML 并校验输出根/URL;`internal/openlist` 实现有界 list/get 与分页、应用层错误处理;`internal/pathmap` 与 `internal/strm` 提供纯路径/协议转换与比较;`internal/storage` 用 modernc.org/sqlite 记录最小归属与完整轮次;`internal/reconciler` 实现有界目录遍历、同目标冲突检测、同目录临时文件加不覆盖 link 的首次发布和固定运行时 flock;`internal/securepath` 使用 Linux 目录句柄和相对操作绑定输出根及数据库父目录。Task 1.2 候选通过确定性同目录暂存文件和 `RENAME_EXCHANGE` 接通内容更新,报告新增 `updated=`;候选还在比较前执行 `Recover`。暂存名编码上一次内容的 SHA-256 前缀;`Update` 在发布前后检查设备/inode 与内容,但检查不能绑定随后的按名删除／交换。B 对修正候选 `d0e2bacfb824b4876c647059e43c2996a9339417` 的历史复审为 FIX REQUIRED：F-10／F-13 是最终检查后外部替换引起的误清理／误发布，F-11 是外部注入后的冲突证据消费；这些观察不改写为 PASS，但按下述澄清边界不再自动要求无损抵御该类干预。F-14 是绑定根 dup 共享目录游标，导致后续根文件漏检残留并错误提交完整 round，与外部写者无关，仍须修正。正常更新／生成器中断恢复在新范围的收敛验收尚未完成；不能因排除外部干预就宣称候选通过。细节与修正准入见 TASK.md。Mirror 清理、serve/定时、发布镜像仍未实现。

依赖固定为 `gopkg.in/yaml.v3 v3.0.1` 与 `modernc.org/sqlite v1.60.1`（CGO=0，Linux amd64／arm64 可构建），go.sum 已提交。race 门槛使用仓库内固定的 Zig `cc`（`artifacts/dev-env/cc/bin/cc`，仅用于 CGO，不是产品运行时依赖）。目录列表累计限制为每目录 10,000 项和 2 MiB 名称字节，待排队目录最多 10,000 个，诊断最多保留每源 100 条；这些是实现边界，不是吞吐或内存承诺。环境 smoke 与真实 OpenList／Jellyfin 证据仍分开报告。

## TARGET

### 正常单写者与最终一致性（2026-10-18 A 澄清）

需求 SW-1..SW-6 与当前 Slice 明确正常使用方式：生成器写输出／暂存／DB，消费者只读；多生成器共享同一锁对象，不能用各容器私有 /tmp 的同名路径冒充同一锁。专用 UID、权限或只读挂载可作为部署建议，但不追加 UID／mode 强制检查、消费者 OS 拒写实验或全系统写者审计能力。

沿用 securepath 绑定句柄、现有锁、同目录原子写入和最小 SQLite 归属事实；恢复继续使用既有 LocalFS／Recover 入口，不新增媒体版本或事务状态机。修正本地目录枚举时，各次枚举独立打开绑定目录并分批读取，不共享 dup 游标、不加载完整目录。生成器正常中断的残留应依据现有归属与恢复证据处理，并在后续成功同步中收敛；不能把所有正常残留一律永久冲突。

未归属文件、归属丢失与确实无法解释的路径／类型冲突保守保留；合法 STRM、名字、哈希或 OS UID 本身不建立归属。已有可靠归属的可再生成内容出现可识别偏差，不等于必须丢弃旧 ownership 重建。无条件保护任意外部注入的所有字节不是本架构目标，也不能把同一干预转为静态 legacy 状态重新要求。扫描期间变化依现有比较规则后续纠正，不每次写入前查远端、不引入远端锁。

CURRENT 差距：本轮仅协调合同和文档，没有修改产品代码／测试；F-14 与正常更新／中断／重试收敛仍须验证和修正，最新候选没有新 PASS。旧 F-10／F-13／外部来源 F-11 保留历史证据，B 明确测试映射；生成器自身 I/O／DB 失败、取消、重启、未归属保护与 T4 不降低。

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

`cmd/strmgen` 装配入口；`internal/openlist` 处理传输／API语义，不管理删除；`pathmap` 与协议比较保持纯函数；`strm` 封装同目录原子替换与本地读取；`storage` 记录最小持久事实；`reconciler` 决定完整轮和清理资格；`scheduler` 复用相同同步入口。配置与日志装配在活动 Task 确定。

目录增量遍历使用有界队列／工作池，处理完一个分页或目录就释放完整对象；SQLite 提供索引与分批查询。OpenList 服务端可能先加载整个目录，这不等于生成器本地内存有界已获验证；客户端必须验证分页与响应限制。

当前 STRM 是 URL／SHA-1／Size 的比较来源，SQLite 不复制独立媒体指纹状态。完整轮次与缺失计数只是删除证据，不扩展为媒体版本或发现状态机。对账失败不提交成功 epoch、不推进清理；再次确认与拥有关系、路径边界都在删除前重核。

文件 rename 和数据库 commit 不可形成同一事务，Task 须按 SW-1..SW-6 验证正常故障窗口：未完成写入不能误认归属，残留记录不能授权删除真正未归属文件，恢复不能把中断轮当完整轮；具备可靠归属和必要恢复依据的正常中断应支持后续重试收敛。符号链接、类型与同名冲突仍保守处理；不声称检测或无损逆转所有越界外部修改。

SQLite 驱动已选 `modernc.org/sqlite v1.60.1`（纯 Go、CGO=0），YAML 库已选 `gopkg.in/yaml.v3 v3.0.1`；版本与 go.sum 已提交。race 检测需要 C 编译器，仓库固定 Zig `cc` 入口；交叉编译使用 CGO=0。不能把 CGO=0 构建结论当成 race 或 ARM64 实机运行证明。

生成器通过 STRM 文件交付给 Jellyfin；库扫描、插件决策／探测和媒体播放属于外部链路。协议对照见 [兼容性](compatibility.md)，分层证据见 [测试矩阵](test-matrix.md)。
