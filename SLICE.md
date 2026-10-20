# Slice 4 — 固定万文件生成与最小扫描／插件决策验收

```text
Slice ID: 4
Owner: A
Status: ACTIVE
Base SHA: d1c95ad079a0e3f952d9e57ab50299c6ae93ad63
Implementation Head: 8109811b46575576f1b87530d65aea298bf2d576
```

## 用户目标

按 [V4.4 方案](<STRM Generator Go.md>) 完成两个结果：固定并持续复用隔离 S3 中的
10000 个媒体文件，证明本程序能够生成 10000 个 STRM，并以少量样本验证同步；
从同批数据选最少量 STRM，证明 Jellyfin 能扫描原始产物，配套插件能按比较规则作出
实际的探测／复用决策。只看决策，不要求探测成功或客户端播放。

## 本次重规划与已有进度

用户授权以 V4.4 取代旧执行范围，同时治理验证工具。Slice Base 不移动，Slice 不关闭。
Task 4.1 最后结论为 FIX REQUIRED；其实施和审查完整保留于
`f0767f0:TASK.md`，本次撤下旧活动计划，不补签 PASS。旧播放门槛、旧暂缓例外和
十万／百万／512 MB 压测均退出当前合同，历史失败／未运行记录保留为历史事实。

已有 Developer 证据包括：201 项分页、五类名称、原始 STRM／稳定 URL 字节、重复同步、
扫描及移动生成／清理／恢复；历史播放结果不参与新门槛。索引位于本机
`artifacts/validation-evidence/task-4.1-fix2-20261019-215918/INDEX.md`。
新合同的固定 10000 数据集、完整插件决策逐行观察和独立 B 复现均未完成。
复用必须核对输入与原件，不能把旧 201 项证据扩称万文件通过。
原件摘要与逐项处置见 [复用账本](docs/slice-4-evidence.md)；插件 D-01..08 验收见
[测试矩阵](docs/test-matrix.md)。

## 固定隔离输入

- OpenList：源码 `3b28270a4066ea52943a69ecddd850d83f7e8bdc`，镜像
  `openlistteam/openlist@sha256:11b490123d57f88c84ecda46285e1921869cf511da3e5bb4062f183e7e1200b7`，v4.2.7。
- 存储：已有 Task 自有隔离 S3/RustFS；记录实际镜像 digest、bucket／前缀的脱敏标识。
- Jellyfin：`jellyfin/jellyfin@sha256:357724bf0ae27a672c7cbaa899db2d9abeb13dbd8657ccce750258a4c059d037`，12.2.0。
- 插件：`https://github.com/kentinya/jellyfin-strm-probe.git` 的源码
  `2afc6517fda5a7b4170ebea625619a552c47dabc`，已有归档 SHA-256
  `382d3ef106ed8534b06ab5bff614d826e602a29703dadddbe3ef2e9864bd06d8`；核对安装／加载身份。
- 不重选远程 HEAD 或 latest，不读取／修改 `/opt/jf-strm-probe` 或生产配置。
  凭据仅从专用进程环境注入，不从历史文件／容器配置恢复。历史凭据事件保持开放并交独立审查处置；
  本轮不删除历史文件、不宣称已轮换。复用隔离环境前须确认其凭据与事件处置允许继续。

## Required Outcomes / Surfaces

| ID | 必须交付的结果 | 验收面 |
|---|---|---|
| RO-1 | 固定、可重复使用的 10000 个 S3 媒体对象 | 一次性数据集标识、对象清单及摘要；复核恰好 10000；后续不重新生成媒体 |
| RO-2 | 真实 OpenList → 发布 CLI 生成 10000 个 STRM | 数量／镜像路径／V1 协议／稳定 URL、生成摘要、环境／耗时／峰值 RSS；稳定重复零重写 |
| RO-3 | 同批少量样本的同步正确性 | 新增／更新／跳过／移动／清理／再出现／dry-run；三完整轮与删除前 Get；样本恢复可核对 |
| RO-4 | 原始生成 STRM 被 Jellyfin 扫描并被插件读取 | 最少样本的原始字节摘要、媒体库条目及原生检查记录；无需全量入库 |
| RO-5 | V4.4 比较表每项的实际插件决策 | 每次单项变化、真实 SHA-1／标注副本、正确 item／当次窗口、预期与实际探测／复用；无旧暂缓例外 |
| RO-6 | 工具范围清晰、回归和独立审查完整 | Slice 1–3 的现有适用工具与产品回归保留；逐项复用依据、真实结果、独立 B／A 审查 |

目标没有 SHA-1 等元数据时，生成器比较分支用现有 Go 回归单列证明，不建立元数据注入服务；
真实源能提供的变化须用真实同步样本验证。插件规则可用测试侧真实本地 SHA-1 和标注副本，
原始生成文件单独保留；同哈希行的不同 Size 是刻意异常元数据，异内容行必须与实际服务字节一致。
插件比较先满足已有主媒体流、历史指纹及原生地址匹配等条件；缺前置条件或只见扫描完成不能算决策通过。

## 实施顺序

1. Task 4.2：治理测试文件／工具，删除退出范围的旧目标运行器，完成文档与质量验证，交独立 B。
2. Task 4.3：4.2 PASS 后，固定万文件数据集并完成少量同步、原始扫描与插件逐行决策验收。
   使用发布 CLI、既有 rclone／容器命令及 Jellyfin 原生界面／公开入口；人工日志核对即可，
   不为此建设通用框架。详细计划在治理完成后形成。

## 约束与不变量

遵守 [AGENTS](AGENTS.md)、[需求](docs/requirements.md) 与 [开发流程](docs/development-workflow.md)。
只操作 artifacts 下专用隔离对象，保留归属／路径／锁／SQLite／dry-run／完整轮安全边界。
生成器不运行 ffprobe、不下载媒体补算哈希、不调用插件内部接口、不操作 Jellyfin DB、不验证代理。
样本变更失败时停止，准确标记已改变／未确认状态，核对恢复后才续验；不要求自动补偿平台。
正常扫描／原生检查由验证人员操作，不加入生成器产品编排。ARM64 实机、正式发布不在本合同。

## 最终验收

全部 RO 都要有匹配证据。插件日志明确要求探测或记录启动即可证明探测；
不探测必须有明确复用／跳过记录，且原因符合该行，不能以无日志或 FirstTrust 代替已建立基准后的比较。
记录固定源码／二进制／插件／镜像／配置／数据集身份、命令、退出码、数量、耗时、首次失败与重跑。
同输入证据允许复用；当前工具改变的出口必须补对应验证，不机械重跑无变化的全部外部门槛。
B 独立验证关键真实链路及逐行决策并审查完整范围，全部 RO 满足后提交 Closure Packet 给 A。
NOT RUN／BLOCKED／历史播放／工具单测均不能转成新的行为 PASS。

## Closure Packet

```text
Closure Packet: PENDING
A Final Review: PENDING
Decision: PENDING
Next Action: INDEPENDENT B REVIEW OF TASK 4.2; THEN ACTIVATE TASK 4.3
```
