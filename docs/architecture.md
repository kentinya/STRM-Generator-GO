# 架构：CURRENT 与 TARGET

## CURRENT

开发准备基线(合同、需求、架构、夹具、Python 工具与 Go 1.27.2 固定 SDK)已交付。Slice 1 的 Task 1.1 有 `cmd/strmgen` 的 `sync` 与 `sync --dry-run` 实现;`internal/config` 严格解析 YAML 并校验输出根/URL;`internal/openlist` 实现有界 list/get 与分页、应用层错误处理;`internal/pathmap` 与 `internal/strm` 提供纯路径/协议转换与比较;`internal/storage` 用 modernc.org/sqlite 记录最小归属与完整轮次;`internal/reconciler` 实现有界目录遍历、同目标冲突检测、同目录临时文件加不覆盖 link 的首次发布和固定运行时 flock;`internal/securepath` 使用 Linux 目录句柄和相对操作绑定输出根及数据库父目录。Task 1.2 候选通过确定性同目录暂存文件和 `RENAME_EXCHANGE` 接通内容更新,报告新增 `updated=`;候选还在比较前执行 `Recover`。暂存名编码上一次内容的 SHA-256 前缀;`Update` 在发布前后检查设备/inode 与内容,但检查不能绑定随后的按名删除／交换。B 对修正候选 `d0e2bacfb824b4876c647059e43c2996a9339417` 独立复审仍判 FIX REQUIRED:原双替换、提前替换暂存对象和第一次合法外部 STRM 恢复通过,但 F-10 检查后清理仍误删外部对象,F-11 第一次恢复消费唯一冲突证据而在下一轮覆盖外部 STRM,F-13 最后检查后的暂存替换仍误发布;新增 F-14 是绑定根 dup 共享目录游标,导致后续根文件漏检残留并错误提交完整 round。普通已覆盖中断、首次发布等通过事实保留,不代表上述窗口安全;细节与修正准入见 TASK.md。Mirror 清理、serve/定时、发布镜像仍未实现。

依赖固定为 `gopkg.in/yaml.v3 v3.0.1` 与 `modernc.org/sqlite v1.60.1`（CGO=0，Linux amd64／arm64 可构建），go.sum 已提交。race 门槛使用仓库内固定的 Zig `cc`（`artifacts/dev-env/cc/bin/cc`，仅用于 CGO，不是产品运行时依赖）。目录列表累计限制为每目录 10,000 项和 2 MiB 名称字节，待排队目录最多 10,000 个，诊断最多保留每源 100 条；这些是实现边界，不是吞吐或内存承诺。环境 smoke 与真实 OpenList／Jellyfin 证据仍分开报告。

## TARGET

### 本地单写者边界（2026-10-18 A 决定）

需求 SW-1..SW-6 与当前 Slice 是本地并发安全前提：受信任生成器独占目标父目录、目标文件、暂存、DB／sidecar 和锁资源的写入，消费者只读；同一输出的生成器进程必须看到同一锁对象。边界贯穿轮次之间与中断恢复；现有固定路径相同不证明跨容器私有 /tmp 中的锁对象相同。输出别名、硬链接、可写挂载、父目录替换和文件原地写入均不得绕过边界。身份、ACL、挂载隔离由部署者落实，不宣称 chmod 0700 或 advisory flock 能抵御同 UID／root 写者。

实现沿用 securepath 绑定句柄和既有互斥／storage 分工；在访问相应输出／状态对象进行修改前校验可直接取得的运行 UID 属主及 group／other 不可写条件，失败保持非零与不完整轮。dry-run 只检查既有对象，不创建探测输出；不存在的路径按最近既有父目录检查，真实创建后复核。未知对象检查先于任何自动修复，不 chmod／chown、不扩张归属、不新增媒体或事务状态表。普通暂存只能因可信生成器来源与既有 ownership／完整恢复证据被清理；名字或内容哈希本身不是认证。legacy／可识别的不明残留不得因声明单写者就变成可信。

CURRENT 差距：本次只提交合同与文档；现有输出／状态对象的上述已知条件校验、消费者写入拒绝实证、F-14 独立有界枚举及跨重启不明冲突保护尚未完成。此前 F-10／F-13 对任意外部 syscall 间隙替换的失败是旧保证下的有效事实，不改写为 PASS；新合同排除这类不合作写者的绝对安全保证，保留可观测异常拒绝与所有生成器自身故障恢复要求。具体测试转换由 B 在合同提交后明确。

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

文件 rename 和数据库 commit 不可形成同一事务，Task 必须在 SW-1..SW-6 前提下证明各崩溃窗口安全：未完成写入不能导致误认归属，残留记录不能授权删除陌生文件，恢复不能把中断轮当完整轮。预先存在或可观测的内容／类型异常、符号链接及同名冲突仍保守处理；不声称检测或逆转所有越过单写者边界的并发外部修改。

SQLite 驱动已选 `modernc.org/sqlite v1.60.1`（纯 Go、CGO=0），YAML 库已选 `gopkg.in/yaml.v3 v3.0.1`；版本与 go.sum 已提交。race 检测需要 C 编译器，仓库固定 Zig `cc` 入口；交叉编译使用 CGO=0。不能把 CGO=0 构建结论当成 race 或 ARM64 实机运行证明。

生成器通过 STRM 文件交付给 Jellyfin；库扫描、插件决策／探测和媒体播放属于外部链路。协议对照见 [兼容性](compatibility.md)，分层证据见 [测试矩阵](test-matrix.md)。
