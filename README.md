# STRM Generator Go

把 OpenList 目录同步为本地 STRM，配套 [jellyfin-strm-probe](https://github.com/kentinya/jellyfin-strm-probe)。
支持一次性／定时同步、增量更新、Mirror 清理和中断恢复，运行目标为 Linux amd64／arm64。

## 文档

仓库只维护以下 10 份文档；已完成任务的过程记录从 Git 历史查阅。

| 文档 | 用途 |
|---|---|
| 本 README | 构建、运行与日常入口 |
| [部署](docs/deployment.md) | Docker／Compose、发布、权限、备份与恢复 |
| [需求](docs/requirements.md) | V4.4 规则、范围、STRM 协议与比较矩阵 |
| [架构](docs/architecture.md) | 包职责、数据流、持久化与并发设计 |
| [开发流程](docs/development-workflow.md) | 环境安装、测试入口、任务／审查流程和历史查看 |
| [路线图](docs/roadmap.md) | 已交付能力与最终验收索引 |
| [SLICE](SLICE.md) | 最近关闭的能力合同 |
| [TASK](TASK.md) | 当前产品实施任务状态 |
| [AGENTS](AGENTS.md) | 仓库开发约束 |
| [CLAUDE](CLAUDE.md) | 开发工具入口 |

## 构建与运行

仓库根目录执行，需要 Linux、Python 3.11+、Git 和下载 SDK 的 HTTPS 网络：

```bash
python3 scripts/development_environment.py install
mkdir -p artifacts/bin
bash scripts/go-local build -trimpath -o artifacts/bin/strmgen ./cmd/strmgen
cp config.example.yaml config.yaml
```

编辑 `config.yaml` 中的 OpenList API／公开地址、源根、专用输出与数据库路径，再注入
`openlist.token_env` 指定的环境变量。示例配置见 [config.example.yaml](config.example.yaml)。
相对输出与数据库路径按启动工作目录解析。

```bash
export OPENLIST_TOKEN=...  # 替换为自己的 token，不写入配置或提交
./artifacts/bin/strmgen sync --config config.yaml --dry-run
./artifacts/bin/strmgen sync --config config.yaml
./artifacts/bin/strmgen serve --config config.yaml
```

`sync` 运行一轮；`--dry-run` 预览；`serve` 首轮立即运行，随后按
`sync.interval_minutes` 调度，默认 60 分钟。退出码：0 成功，1 运行失败／忙碌／取消，
2 参数或配置错误。`serve` 收到 SIGINT／SIGTERM 后取消当前轮并释放锁，不提交不完整轮。
容器用法与自动发布工作流见[部署说明](docs/deployment.md)。

## 清理与恢复

- 清理默认关闭。开启 `cleanup.enabled` 时要求 `sync.mirror: true`，至少连续三次完整成功轮缺失，并在删除前再次核对源根与目标 Get。
- 源根不可读时保留；源根可读而目标未读到时计缺失，即使原因是权限、驱动或缓存异常。这是当前业务规则，详见[需求](docs/requirements.md#完整轮与缺失确认)。
- 只维护数据库可靠归属的文件。数据库丢失后不会自动接管已有 STRM；优先恢复可信备份，未知文件和冲突保留。
- 生成器单写、消费者只读；共享输出的生成器必须共享同一锁对象。不要在运行时删除 `/tmp/strmgen-locks/sync.lock` 或 SQLite sidecar。
- 普通中断可重试；人工处理残留前先停生成器并备份。暂存、WAL 快照和多实例规则见[部署说明](docs/deployment.md#状态备份与恢复)。

V4.4 与 Slice 1–4 已验收，包含固定 10000 文件、最少同步样本、原始扫描和插件八行决定。
探测启动不等于探测成功；ARM64 原生实机与正式发布验收未包含在原合同内。
