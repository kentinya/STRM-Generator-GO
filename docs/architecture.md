# 架构：CURRENT 与 TARGET

## CURRENT

开发准备基线（合同、需求、架构、夹具、Python 工具与 Go 1.27.2 固定 SDK）已交付。Slice 1 的 Task 1.1 有 `cmd/strmgen` 的 `sync` 与 `sync --dry-run` 实现；`internal/config` 严格解析 YAML 并校验输出根／URL；`internal/openlist` 实现有界 list/get 与分页、应用层错误处理；`internal/pathmap` 与 `internal/strm` 提供纯路径／协议转换与比较；`internal/storage` 用 modernc.org/sqlite 记录最小归属与完整轮次；`internal/reconciler` 实现有界目录遍历、同目标冲突检测、同目录临时文件加不覆盖 link 的首次发布和固定运行时 flock；`internal/securepath` 使用 Linux 目录句柄和相对操作绑定输出根及数据库父目录。Task 1.2 候选通过确定性同目录暂存文件和 `RENAME_EXCHANGE` 接通内容更新，报告新增 `updated=`；候选还在比较前执行 `Recover`，并在回滚前检查字节与 inode。B 对修正候选 `f4383f2f59adedadc9252e465c64580570a3bbcf` 独立复审仍判 FIX REQUIRED：检查与删除之间仍可换入外部对象，合法外部 STRM 恢复后仍被当作自有内容覆盖，暂存对象替换后可误发布／误删。现有 F-10 双替换序列与新增故障窗口通过，不能证明整个归属协议安全；细节见 TASK.md。Mirror 清理、serve／定时、发布镜像仍未实现。

依赖固定为 `gopkg.in/yaml.v3 v3.0.1` 与 `modernc.org/sqlite v1.60.1`（CGO=0，Linux amd64／arm64 可构建），go.sum 已提交。race 门槛使用仓库内固定的 Zig `cc`（`artifacts/dev-env/cc/bin/cc`，仅用于 CGO，不是产品运行时依赖）。目录列表累计限制为每目录 10,000 项和 2 MiB 名称字节，待排队目录最多 10,000 个，诊断最多保留每源 100 条；这些是实现边界，不是吞吐或内存承诺。环境 smoke 与真实 OpenList／Jellyfin 证据仍分开报告。

## TARGET

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

文件 rename 和数据库 commit 不可形成同一事务，Task 必须证明各崩溃窗口安全：未完成写入不能导致误认归属，残留记录不能授权删除陌生文件，恢复不能把中断轮当完整轮。内容外部修改、符号链接及同名目标冲突均需保守处理。

SQLite 驱动已选 `modernc.org/sqlite v1.60.1`（纯 Go、CGO=0），YAML 库已选 `gopkg.in/yaml.v3 v3.0.1`；版本与 go.sum 已提交。race 检测需要 C 编译器，仓库固定 Zig `cc` 入口；交叉编译使用 CGO=0。不能把 CGO=0 构建结论当成 race 或 ARM64 实机运行证明。

生成器通过 STRM 文件交付给 Jellyfin；库扫描、插件决策／探测和媒体播放属于外部链路。协议对照见 [兼容性](compatibility.md)，分层证据见 [测试矩阵](test-matrix.md)。
