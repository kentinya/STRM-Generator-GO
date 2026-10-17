# 架构：CURRENT 与 TARGET

## CURRENT

只有开发方案、Go module／目录占位、仓库规则、Python 环境工具与 OpenList 合成 API 夹具。不存在生成器 Go 包、配置解析、SQLite schema、调度器、镜像或发布产物。环境 smoke 的临时 Go 文件在忽略目录，不是产品实现。

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

SQLite 驱动和 YAML 库尚未选型；优先评估能支持 Linux amd64／arm64 的依赖与工具链，固定版本及 go.sum 后验证。若使用 CGO，必须补齐 C 编译器、race／交叉编译与运行依赖；不能把当前纯标准库 smoke 的 CGO=0 结论套用于未来产品。

生成器通过 STRM 文件交付给 Jellyfin；库扫描、插件决策／探测和媒体播放属于外部链路。协议对照见 [兼容性](compatibility.md)，分层证据见 [测试矩阵](test-matrix.md)。
