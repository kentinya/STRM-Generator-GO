# Slice 4 — 隔离真实服务端到端兼容验收

```text
Slice ID: 4
Owner: A
Status: ACTIVE
Base SHA: d1c95ad079a0e3f952d9e57ab50299c6ae93ad63
Implementation Head: NOT SET
```

## 用户目标

管理员能在专用隔离环境中确认生成器与固定 OpenList 服务、V4.3 插件及 Jellyfin／代理
实际协同：从已授权的 OpenList 测试挂载读取元数据，生成稳定且不含签名的 `/d/` STRM，
由固定插件解析并按既有 URL／SHA-1／Size 规则决策，最终经隔离 Jellyfin／代理链读取
并播放合成媒体。此 Slice 交付的是可追溯兼容证据，不修改或控制生产服务。

## 已接受基线与选择

Slice 1、2、3 已 PASS / CLOSED。Slice 3 的完整 A 审查范围为
`ca9f91e10a0fc6c28679b5827f453e632eb9854a..410fc0e0a8964046de4707d58c0c7505385c6002`；
Task 3.1／3.2 均有独立 B PASS。现有本地产品、Mirror 清理、serve、双架构交叉构建、
amd64 容器运行及部署挂载证据按原输入身份保留；均不等于真实端到端播放。

2026-10-19 A 在 Slice 3 关闭后的独立选择回合，将真实服务端到端兼容选为当前 Slice。
Base 固定为选择前真实 HEAD `d1c95ad079a0e3f952d9e57ab50299c6ae93ad63`。本合同、
Roadmap 和当前状态文档提交不移动 Base。A 将 ARM64 原生实机和 10 万至 100 万规模
测量分为后续 Slice 5，以便独立验收其硬件与性能结果；没有撤销或降低这两项目标。

固定参考源保持不变：OpenList 源码 `3b28270a4066ea52943a69ecddd850d83f7e8bdc`、
已实证 OpenList 镜像 `openlistteam/openlist@sha256:11b490123d57f88c84ecda46285e1921869cf511da3e5bb4062f183e7e1200b7`
（v4.2.7）、插件源码 `kentinya/jellyfin-strm-probe@1bdb79082c4945e126611f217200555d53c31367`。
这些固定身份不表示已有目标驱动、Jellyfin 或代理端到端证据。B 规划时必须为拟用
OpenList 驱动／挂载、Jellyfin、代理和测试媒体记录可追溯的版本／镜像 digest、来源与
配置身份；不得使用 `latest`。不能从生产配置推断或复制用户凭据。未取得匹配的隔离
目标驱动、代理链或身份时，停止相关外部场景并如实交 A，不以 Local／WebDav 结果或
合成 API 代替目标部署兼容结论。

## Required Outcomes / Surfaces

- **RO-1 固定隔离输入：** 建立只使用本仓库 `artifacts/` 的一次性隔离环境和合成媒体；
  固定 OpenList、插件、Jellyfin、代理、驱动、镜像／源码、配置和工具身份。认证只用
  专用测试身份与进程环境变量，不把 token／Cookie／签名 URL 或私有完整路径写入仓库、
  stdout、日志或证据。不得接入生产端点、真实媒体或宿主服务状态。
- **RO-2 真实 OpenList 到稳定 STRM：** 在已明确的目标 OpenList 驱动与隔离挂载上，
  用发布 CLI 验证认证、list／分页、get、路径编码及可用 hash／size 元数据；至少包含
  普通文件和中文／空格／`#`／`%` 名称。生成 STRM 使用配置的稳定公开 `/d/` 地址，
  不含 `raw_url`、签名查询或凭证；测试可验证 HTTP(S) 路径实际返回与隔离源文件一致的
  字节。若目标挂载本身要求临时签名地址，按当前合同记为不满足前提，不绕过鉴权。
- **RO-3 固定插件实际解析与决策：** 将生成的原始 STRM 输入固定 V4.3 插件，在隔离
  Jellyfin 中实证解析成功及必要主媒体流；通过产品可观察行为覆盖稳定 URL、URL 改变、
  同 URL／同有效 SHA-1／不同 Size、哈希不同、哈希不可比时有效 Size 改变等合同矩阵。
  不以 Go 测试、手写解析器、源码静态阅读或插件内部未公开接口替代实际插件证据。
- **RO-4 Jellyfin／代理／播放闭环：** 隔离 Jellyfin 扫描生成目录并加载固定插件；分别
  验证 Jellyfin 直接路径和目标隔离代理路径均可读取及播放合成媒体，内容身份一致；记录
  两条入口的 HTTP 状态、播放结果、必要流及插件探测／重用结果。代理身份或隔离实例
  不可用时此 outcome 为 BLOCKED／NOT RUN，不得以 Jellyfin 直连结果替代代理通过。
- **RO-5 可重复性与安全边界：** 首轮、稳定重复同步、URL 变化／媒体变化和目录移动后的
  预期 STRM／插件行为在专用隔离实例可重放；不发生 token、Cookie、签名 URL 或私有
  完整路径泄露。同步成功、STRM 可解析、Jellyfin 入库、代理响应和真实播放分别记录，
  不相互推断。
- **RO-6 全范围回归与证据：** 对最终产品候选执行匹配输入的 T4 质量／回归门槛，覆盖
  既有 sync、Mirror 清理、dry-run、锁、恢复、serve、Docker／Compose 和故障场景；保留
  隔离 OpenList 服务、固定插件、Jellyfin／代理、播放和本地产品结果各自的原始证据与
  NOT RUN 原因。每项记录源码／镜像／配置／夹具身份、命令与 cwd、退出码、计数、耗时、
  首次失败和重跑；绝不把环境 smoke、交叉构建或合成夹具标为真实服务 PASS。

## 约束与安全不变量

遵守 [AGENTS.md](AGENTS.md)、[需求 R-01..R-12 与 SW-1..SW-6](docs/requirements.md)、
[用户旅程](docs/product-experience.md)、[架构](docs/architecture.md) 及固定 V4.3 方案。
不修改原方案、稳定 STRM 协议、比较规则、归属／删除安全、锁、SQLite 持久事实边界或
既有 Slice 结果。任何改变稳定需求、架构或验收含义的需要，必须先交 A 协调，不得在
Task 内弱化门槛。

真实服务只可在专用隔离实例运行；只创建、停止及清理本 Task 在 `artifacts/` 下明确标记
归属的容器、网络、临时配置和合成媒体。不得读取／修改 `/opt/jf-strm-probe`、运行中的
Jellyfin、OpenList、代理、生产配置、生产媒体或用户状态。部署目标凭据不进文件或证据，
运行时通过环境注入；日志只保留脱敏标识和必要摘要。正常服务必须具有版本和来源身份，
不能跟随 `latest` 漂移。

生成器不运行 ffprobe、不下载媒体计算哈希、不操作 Jellyfin 数据库、不调用插件内部
接口、不改写 JellyfinProxy。插件解析／播放是隔离验收端的独立观测，不成为生成器运行时
依赖或新产品职责。目标驱动的选择不能通过访问生产配置或服务来猜测。

## 排除范围与后续顺序

本 Slice 不实施新调度、清理、媒体版本状态、插件或代理功能；只允许在现有需求和架构
范围内修复被真实隔离测试证实的产品缺陷。若问题需要改变稳定需求或外部组件，停止并交
A 协调合同，不自行扩张。

ARM64 原生实机运行、10 万至 100 万目录／文件规模测量与资源画像保留给规划中的
Slice 5；本 Slice 的 amd64 隔离结果、Go 交叉构建或 OCI manifest 不替代这些验收。正式
registry 发布亦不属于 Slice 4／5 的隐含门槛。

## 验收与最终验证

Slice Final 必须覆盖：

1. RO-1 所有输入身份、配置摘要和隔离归属可核验；运行仅使用专用合成媒体、测试凭据和
   `artifacts/` 数据。任何生产状态被触碰或秘密进入证据均为阻断项。
2. RO-2 目标驱动真实 OpenList 接入与 stable `/d/` 输出通过；页面／对象内容身份、编码
   边界、API 认证、元数据与实际内容逐项核验。目标存储不支持稳定无签名 `/d/` 时记录
   不满足前提，不伪造可用播放地址。
3. RO-3 使用固定插件部署验证原始生成 STRM 的解析和必要决策矩阵；必须能从 Jellyfin／
   插件实际可观察状态判定，而非只调用生成器测试桩或插件内部接口。
4. RO-4 从隔离 Jellyfin 直连及经目标隔离代理分别到合成媒体完成实际播放，核对内容、
   必要流与探测／重用结果；没有任一路径的代理或播放证据时此 RO 不通过。
5. RO-5 首次、重复、更新、移动的端到端结果稳定，敏感值和私有路径脱敏，层级结果互不
   冒充。
6. RO-6 指定的全量 T4 门槛及独立 B 复审完成；候选后任何可执行输入变化按影响重跑。
   真实目标输入不可用的场景为 BLOCKED／NOT RUN，不能作为 PASS 或被环境 smoke 替代。

Slice 为隔离真实外部依赖的 T4。B 按实际源代码、固定身份与隔离能力规划 Task、准确命令、
故障／成功案例和证据路径；Task 计划不得降低上述验收。若固定目标驱动、隔离 Jellyfin 或
代理身份未能合法取得，B 应在计划或准备阶段明确列出阻塞和所需用户输入，不启动生产
联调；只有用户明确授权后才可补充目标身份或变更合同。

```text
Closure Packet: PENDING
A Final Review: PENDING
Next Action: B PLANS TASK 4.1 AFTER THIS CONTRACT CHECKPOINT
```
