# Slice 7 — 按目录下载与清理外挂字幕

```text
Slice ID: 7
Owner: A
Status: PASS / CLOSED
Base SHA: 11b51b9ec4708f553671eefcb9d3b06dd43d0845
Implementation Head: 173293be70862e530201cccce548534169d9a431
```

开启外挂字幕后，合格目录中的入选字幕会按原名和原始字节补齐到 STRM 旁边。
已登记的字幕按自身路径累计缺失轮次并在删除前重核。Jellyfin 原生扫描可识别
`Movie.zh.srt` 等匹配名字幕，任意名字幕按原名复制。配置和业务规则见
[README](README.md#外挂字幕)与[需求 R-15](docs/requirements.md#外挂字幕)。

## Closure Packet

```text
A Final Review: 2026-11-20
Decision: PASS
Reviewed Range: 11b51b9ec4708f553671eefcb9d3b06dd43d0845..173293be70862e530201cccce548534169d9a431
B Handoff: 08c22cb5cd743db71182f523185043c038e05590
B Task: 7.1 / Independent B PASS
Blockers: none
Remaining Authorized Work: none
```

A 审查了完整实现范围及后续交接记录。Base 是字幕合同提交前的 v1.2.1 基线。
实现 Head 至 B 交接只更新文档和审查记录。完整合同、Task 计划、B-01 至 B-05 的修正和
复核保存在 `08c22cb5cd743db71182f523185043c038e05590:SLICE.md`／`TASK.md`。

| RO | A 决定 | 核验结果 |
|---|---|---|
| RO-1 | PASS | 默认关闭且兼容旧配置。三个命令入口共用严格配置，错误在同步前返回退出码 2 并指出字段。 |
| RO-2 | PASS | 同目录多视频复用已完整分页的列表，目录资格取成功处理或确认的 STRM。真实 OpenList 首轮在嵌套目录生成 STRM，并按原名下载匹配名与任意名两份字幕。 |
| RO-3 | PASS | 稳定 `/d/` 请求不带 API token。核对传输限额、错误响应拒绝、同目录原子无覆盖发布和已有普通文件／链接保护。真实 HTTP 响应与本地字幕摘要相符。 |
| RO-4 | PASS | 重跑跳过两份字幕并保留字节及 mtime。移除任意名字幕后，三个完整轮计数依次为 1、2、3，文件始终保留。dry-run 只预览且原库、文件和轮次不变，第四轮重核后只删除目标字幕及其归属。本地证据覆盖迁移、未归属保护、失败冻结和重跑恢复。 |
| RO-5 | PASS | CLI 和定时摘要区分实际发布、跳过、失败、实际删除及预览。诊断使用脱敏相对路径，字幕动作不进入 STRM 通知事件。 |
| RO-6 | PASS | 两份示例配置、升级说明和分层 T4 证据齐备。隔离 Jellyfin 12.3.0 完成原生扫描，视频详情在同轮生成的 `Movie.strm` 上列出实际下载的中文 SubRip 外部字幕轨。独立 B 与 A 审查完成。 |

### 证据与范围

- A 用干净实现 Head 的发布／testhooks 二进制独立复跑已有 `subtitles`、
  `subtitles_cleanup`、`serve_subtitles`，3/3 通过。构建信息、SHA-256、命令与结果
  保存于 `artifacts/validation-evidence/slice-7-a-hPMOah/`。
- 核对并复用同一候选的质量门、10 包 race、sync 16/16、serve 9/9、容器 7/7 和
  真实 OpenList 元数据 8 场景 + CLI Mirror。双架构静态产物和 arm64 OCI 摘要匹配原件。
  索引为 `artifacts/validation-evidence/task-7.1-fix2-INDEX.md`。
- 真实 OpenList v4.2.7 字幕生命周期与 Jellyfin 12.3.0 扫描分别核对 B 的原始证据。
  A 核对了候选身份、稳定地址 HTTP 200 与字节摘要、逐轮状态、原生扫描完成日志及视频详情。
  首次签名要求导致的 401、夹具调整和通过的第 9 次运行均保留在
  `artifacts/validation-evidence/task-7.1-b-services-NEJauV/INDEX.md`。A 复用已记录的授权隔离实测。
- R-02／R-05 的 STRM 协议和比较规则、来源身份、共享锁、有限批处理与清理门槛保持适用。
  字幕只补缺件，稳定地址可访问是部署前提。ARM64 原生实机、客户端播放、插件决定矩阵、
  万文件测量及正式发布在本 Slice 均为 NOT RUN。无剩余 P2 或延期项。
