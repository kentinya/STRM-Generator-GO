# NO ACTIVE IMPLEMENTATION TASK

Slice 1–7 和独立修复 FIX-TG-01、FIX-SUB-01 均已通过审查并关闭。

## FIX-SUB-01 — 字幕下载兼容与 Telegram 分类统计

| 项目 | 最终记录 |
|---|---|
| Task 类型与关联 | standalone-fix，Parent Slice NONE，Related Slice 7 |
| 最终决定 | 独立 B **PASS / CLOSED**，B-SUB-01 至 B-SUB-03 全部关闭 |
| 已审完整范围 | `05a8bddcabcc9ad517d509316a0938237309b81d..981f05ce5435763be207727ad8d8d227b54be606` |
| 计划检查点 | `a306e6b8703ae09c69fa100c8c2cc3ae65fa9c39` |
| 交接与历史 | `571ebd8486f8e2806d931466e3e338067a52bb03:TASK.md` 保留完整计划、两轮实施报告、首次 B 审查及开发指导 |
| 剩余阻塞与实施工作 | 无 |

开启外挂字幕后，每轮跨目录与来源共用同一下载间隔，默认 1 秒，最小 0.5 秒。
字幕内容客户端默认使用 HTTP/1.1，显式开启后允许 HTTPS 协商 HTTP/2 并保留 HTTP/1.1
回退。Telegram 在一条消息中分别标明 STRM 与字幕统计，并单列双列表过滤数。CLI
的 `filtered=` 沿用媒体扩展名口径。

### 独立 B 最终复评

| 验收项 | 决定与结果 |
|---|---|
| SUB-1 | PASS。旧配置及默认关闭兼容，1 秒／0.5 秒默认和下限分别核对。合法 YAML 整数按其数值生效，上一轮复现的超范围值在准备输出前退出 2，并返回固定字段诊断。 |
| SUB-2 | PASS。首份立即，慢下载和长空闲用时钟测试核对。含首份 404 的两来源嵌套同步中，B 复跑观察到三个约 0.501 秒的请求间隔，最大在途为 1。 |
| SUB-3 | PASS。本地 TLS 生产构造路径覆盖默认 HTTP/1.1、显式启用 HTTP/2、仅支持 HTTP/1.1 的 HTTPS 服务回退、跨地址重定向和 transport 隔离。 |
| SUB-4 | PASS。sync 等待取消只产生首份新字幕请求，下一份无文件、无归属，轮次及已达阈值的缺失计数冻结，重跑补齐并按门槛清理。serve 和 amd64 容器的停止、锁释放与补齐也有候选证据。 |
| SUB-5 | PASS。dry-run、关闭及同名已有文件路径维持原行为。下载等待先于 30 秒传输预算，原限额、重定向、错误页、发布和清理回归通过。 |
| SUB-6 | PASS。示例、升级说明、实际实现 SHA 和 T4 原始证据已核对，完整范围与三项修正均经独立 B 复审。 |
| SUB-7 | PASS。混合目录跨来源的 CLI 和 Telegram 计数符合各自口径，显式字幕列表替换默认值，关闭开关后过滤口径一致。sync 与 serve 的合成接收端均收到独立分类的一条摘要。 |
| SUB-8 | PASS。字幕失败与轮次级来源问题出现在对应统计或诊断中。B 用 serve 实际补齐两份字幕，再经三轮缺失确认后看到字幕删除 1、归属回收 1，且 STRM 删除／回收均为 0。初始化失败标明统计不可用，明细范围、通知触发和 3500 UTF-16 预算通过。 |

| 关联 Slice 7 结果 | 复评 |
|---|---|
| RO-1 配置兼容 | PASS。旧块可用，三个入口共用配置，新增字段默认值、错误校验和重启生效均有证据。 |
| RO-2 目录资格 | PASS。现有合格目录按原名补齐，跨目录仍逐份下载，字幕单独目录和已有文件路径通过回归。 |
| RO-3 传输与发布 | PASS。稳定地址、协议策略、限额、原子发布与已有文件保护通过源码复核及本地回归。 |
| RO-4 重跑与清理 | PASS。同名跳过、等待取消、计数冻结、阈值重核与实际删除均有 CLI 或容器证据。 |
| RO-5 摘要与诊断 | PASS。CLI 字段、分类通知、来源故障与 STRM 明细范围经重新核对。 |
| RO-6 文档与证据 | PASS。示例和文档对应候选，独立 B 完成了本次修复复评。真实 OpenList 与 Jellyfin 的原 Slice 结论见原关闭记录。 |

### 验证证据与发布状态

- Developer 的最终候选证据位于
  `artifacts/validation-evidence/fix-sub-01-fix2-20261123T083140Z/`。B 核对固定 Go
  1.27.2、干净提交身份、二进制 SHA-256、镜像身份和原始日志。质量门、全包 race、
  sync 19/19、serve 9/9、amd64 容器 7/7 通过，Linux amd64／arm64 静态产物已核对。
- B 的独立复跑位于 `artifacts/validation-evidence/fix-sub-01-b2-GDIs3u/`，包含聚焦
  Go 回归、sync 集成 4/4、serve 字幕的实际下载和清理通知及初始化失败验证。
  首次初始化失败样本是目录，命中配置校验；改用内容非法的普通数据库文件后成功覆盖
  同步初始化失败通知。两次日志均保留。本轮产品测试约 22 秒，少于复核用时的一半。
- 真实 OpenList／115 根因及开关对比、真实 Telegram 投递、Jellyfin／插件、万文件和
  ARM64 原生实机本轮均为 NOT RUN。分发记录见
  [v1.3.1 Release](https://github.com/kentinya/STRM-Generator-GO/releases/tag/v1.3.1)。

## 已关闭任务索引

- FIX-SUB-01 的完整记录见上方[最终复评](#独立-b-最终复评)与候选交接检查点。
- Task 7.1 的计划、实施与修正报告及最终 B PASS 保存在
  `08c22cb5cd743db71182f523185043c038e05590:TASK.md`。
  固定 Task Base／Contract Checkpoint 为 `e59af5280f11c0d69e735d4cbc951cf0f521bedb`，
  实现 Head 为 `173293be70862e530201cccce548534169d9a431`。A 决定与分层证据见
  [Slice 7 关闭记录](SLICE.md#closure-packet)。
- FIX-TG-01 和 Task 6.1 的完整关闭记录保存在
  `e59af5280f11c0d69e735d4cbc951cf0f521bedb:TASK.md`。
- 早期任务与角色指南见[固定历史检查点](docs/development-workflow.md#历史记录)。
