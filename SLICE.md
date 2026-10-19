# Slice 4 — 隔离真实服务端到端兼容验收

```text
Slice ID: 4
Owner: A
Status: ACTIVE
Base SHA: d1c95ad079a0e3f952d9e57ab50299c6ae93ad63
Implementation Head: NOT SET
```

## 用户目标

管理员能在专用隔离环境中确认生成器与固定 OpenList 服务、从配套插件远程仓库锁定 SHA 构建的
插件包及 Jellyfin 原生链路实际协同：从已授权的 OpenList 测试挂载读取元数据，生成稳定且
不含签名的 `/d/` STRM，由该插件包解析并按既有 URL／SHA-1／Size 规则决策，再由隔离
Jellyfin 扫描并实际播放合成媒体。此 Slice 交付可追溯兼容证据，不修改或控制生产服务；
代理验证不属于本 Slice。

## 已接受基线与选择

Slice 1、2、3 已 PASS / CLOSED。Slice 3 的完整 A 审查范围为
`ca9f91e10a0fc6c28679b5827f453e632eb9854a..410fc0e0a8964046de4707d58c0c7505385c6002`；
Task 3.1／3.2 均有独立 B PASS。现有本地产品、Mirror 清理、serve、双架构交叉构建、
amd64 容器运行及部署挂载证据按原输入身份保留；均不等于真实端到端播放。

2026-10-19 A 在 Slice 3 关闭后的独立选择回合，将真实服务端到端兼容选为当前 Slice。
Base 固定为选择前真实 HEAD `d1c95ad079a0e3f952d9e57ab50299c6ae93ad63`。本合同、
Roadmap 和当前状态文档提交不移动 Base。A 将 ARM64 原生实机和 10 万至 100 万规模
测量分为后续 Slice 5，以便独立验收其硬件与性能结果；没有撤销或降低这两项目标。

OpenList 参考身份保持不变：源码 `3b28270a4066ea52943a69ecddd850d83f7e8bdc`、
已实证镜像 `openlistteam/openlist@sha256:11b490123d57f88c84ecda46285e1921869cf511da3e5bb4062f183e7e1200b7`
（v4.2.7）。Task 4.1 已从远程仓库
https://github.com/kentinya/jellyfin-strm-probe.git 的默认 main 分支锁定源码
2afc6517fda5a7b4170ebea625619a552c47dabc，并在 artifacts/task-4.1/plugin-source/
的干净源码副本用 .NET 10.0.401 和仓库自带 package_plugin.py 构建；插件 ZIP SHA-256 为
382d3ef106ed8534b06ab5bff614d826e602a29703dadddbe3ef2e9864bd06d8。
Jellyfin 固定为 jellyfin/jellyfin@sha256:357724bf0ae27a672c7cbaa899db2d9abeb13dbd8657ccce750258a4c059d037
（12.2.0），目标驱动为专用隔离 S3/RustFS。上述身份沿用，不重取远程 HEAD 或改用 latest。
旧源码 1bdb79082c4945e126611f217200555d53c31367 仅作 V4.3 协议参考，不代表部署包。

B 仍须核对实际包／安装文件、CLI、合成媒体及配置／工具身份与证据一致。不能从生产配置
推断或复制凭据；目标身份或专用进程环境凭据缺失时停止相应场景并如实记录，不以
Local／WebDAV、合成 API 或未知来源插件包替代。不读取／修改 /opt/jf-strm-probe。

## 用户授权的最简验收（2026-10-19，B 据会话落实）

本节落实用户“最简单的验证方案”“第二项先不要管，第一项你可以自己生成SHA1进行验证”
及“把最简方案落实调整相关文档”的直接授权，替代旧合同与 Task 中相冲突的执行要求。
这是用户授权调整的文档记录，不是 B 代签 A 审查；A Final Review 仍 PENDING，Base 不变。

- 复用已固定的隔离 S3/RustFS、OpenList、Jellyfin、CLI 与插件包，不重新选版或搭环境。
- 原始 CLI 输出链路验证首轮／重复同步、稳定无签名 URL、分页／特殊名称与真实播放；
  既有匹配证据复用，B 只独立重现一份已有合成媒体的关键链路。
- SHA-1 两行允许测试侧从 artifacts 内自有本地合成媒体实际字节计算真实 SHA-1，
  写入明确标注的独立 STRM 副本。保留原始 CLI 输出，不冒充目标 OpenList 原生 hash。
  同内容／同哈希只改 Size 元数据以制造异常；不同内容改用其真实哈希与实际 Size，
  同一稳定 URL 提供对应真实内容。正常扫描／播放和插件日志为判据，不用私有接口／DB。
- 仅暂缓三项：URL-change、哈希不可比的 Size-change、移动路径首次播放。保留原始
  BLOCKED／失败及用户接受的风险；本轮不调查、不修复、不重跑，不作为当前 B 门禁阻断，
  也不标 PASS。移动生成／扫描／清理／恢复证据仍需核对。后续处置交 A，不扩大例外。
- 现有命令、人工操作和逐项日志核对即可；不建设通用验证框架、自动补偿系统或日志关联引擎。
  已复现的凭据出口仍须最小修正；夹具失败后停止并标记，可核对恢复或弃用重建，不能带污染续验。

## Required Outcomes / Surfaces

- **RO-1 固定隔离输入：** 核对 Task 4.1 已固定的源码／镜像、目标驱动、CLI、插件归档及
  安装文件、合成媒体与脱敏配置身份。只用 artifacts 下 Task 自有隔离状态，专用凭据仅从
  进程环境注入，不接入生产。已披露凭据事件如实保留脱敏处置记录，不能宣称从未泄露；
  当前泄露出口及未完成处置仍是阻断，历史事件的接受由 A 在最终审查中明确决定。
- **RO-2 真实 OpenList 到稳定 STRM：** 目标 S3/RustFS 经真实 list／get 与发布 CLI 生成
  原始 STRM；覆盖超过一页对象及普通、中文、空格、#、% 名称；所有 list 为 refresh:false。
  稳定无签名 /d/ 返回字节与合成源一致，重复同步零重写。不以 Local／WebDAV／mock 替代。
- **RO-3 固定插件实际解析与决策：** 固定远程源码构建的实际插件解析原始 CLI 输出，
  有必要主媒体流并观察稳定输入的重用；另以本节授权的真实内容 SHA-1 派生副本验证
  同 URL／同有效 SHA-1／Size 异常时复用，以及不同有效 SHA-1 时启动探测。
  逐行关联输入、实际 item 与观测窗口；探测启动、结果接受及播放成败分开记录。
  URL-change 与哈希不可比 Size-change 按用户决定暂缓，不冒充通过。
- **RO-4 Jellyfin 原生扫描与播放：** 原始 CLI STRM 经隔离 Jellyfin 正常扫描及受支持原生
  入口实际播放，核对内容、必要主流、会话／进度与对应插件重用结果。B 独立重现一份
  已有合成媒体即可，其他匹配证据复用；curl、扫描完成或页面加载不能代替播放。
- **RO-5 重复与恢复：** 核对首轮、稳定重复和移动生成／扫描／旧路径清理／恢复原路径
  播放的匹配证据。移动路径首次播放按用户决定暂缓；不要求重跑整套移动场景。
  夹具操作失败须停止、如实标记改变，核对恢复或弃用重建；不要求通用自动回滚。
- **RO-6 回归与独立审查：** B 对固定最终候选负责指定全部 T4 门禁、输入匹配的逐项证据
  复用裁定及完整 Task Base..Head 审查；只补缺失或输入变化项，不将全量交给 Developer，
  不机械重跑 Final。真实服务、派生 SHA-1 输入、本地回归及未运行项分别记录。

## 约束与安全不变量

遵守 [AGENTS.md](AGENTS.md)、[需求 R-01..R-12 与 SW-1..SW-6](docs/requirements.md)、
[用户旅程](docs/product-experience.md)、[架构](docs/architecture.md) 及固定 V4.3 方案。
不修改原方案、稳定 STRM 协议、比较规则、归属／删除安全、锁、SQLite 持久事实边界或
既有 Slice 结果。除上节用户已明确授权的验收调整外，任何改变稳定需求、架构或验收含义的需要，
必须先交 A 协调，不得在 Task 内另行弱化门槛。

真实服务只可在专用隔离实例运行；只创建、停止及清理本 Task 在 `artifacts/` 下明确标记
归属的容器、网络、临时配置和合成媒体。不得读取／修改 `/opt/jf-strm-probe`、运行中的
Jellyfin、OpenList、代理、生产配置、生产媒体或用户状态；Slice 4 不启动或验证代理。部署目标凭据不进文件或证据，
运行时通过环境注入；日志只保留脱敏标识和必要摘要。正常服务必须具有版本和来源身份，
不能跟随 `latest` 漂移。

生成器不运行 ffprobe、不下载媒体计算哈希、不操作 Jellyfin 数据库、不调用插件内部
接口、不改写 JellyfinProxy。插件解析／播放是隔离验收端的独立观测，不成为生成器运行时
依赖或新产品职责。目标驱动的选择不能通过访问生产配置或服务来猜测。

## 排除范围与后续顺序

本 Slice 不实施新调度、清理、媒体版本状态、插件或代理功能，也不验证代理；只允许在现有需求和架构
范围内修复被真实隔离测试证实的产品缺陷。若问题需要改变稳定需求或外部组件，停止并交
A 协调合同，不自行扩张。

ARM64 原生实机运行、10 万至 100 万目录／文件规模测量与资源画像保留给规划中的
Slice 5；本 Slice 的 amd64 隔离结果、Go 交叉构建或 OCI manifest 不替代这些验收。正式
registry 发布亦不属于 Slice 4／5 的隐含门槛。

## 验收与最终验证

1. 逐项核对 RO-1..RO-6；最简执行与 Developer／B 分工见 [Task 4.1](TASK.md)。
   原始 CLI 链路、真实 SHA-1 派生输入、本地回归分别给结论，不跨层推断。
2. B 完成至少一份原始 CLI 合成媒体的 OpenList→稳定字节→Jellyfin 原生扫描／播放
   独立复现和对应插件重用核对；两行 SHA-1 必须有固定实际插件证据，Go 比较通过不替代。
   不同 SHA-1 行以预期探测决策为判据，后续接受／播放另列真实结果；未解决新失败交 A。
3. 首轮、201 项分页、五类名称、重复零写、移动生成／扫描／清理／恢复与 T4 门禁可复用
   完全匹配的证据。候选后输入变化按影响重跑；所需证据缺失仍为 BLOCKED／NOT RUN。
4. 三项用户暂缓结果不是 PASS，不要求本轮重跑；Closure Packet 必须逐项列出原始事实、
   授权及风险，不能声称原全矩阵通过。除这三项外，不推导新的豁免。
5. 检查实际凭据出口修正和事件处置；缺授权运行凭据时停止相应真实场景，不从文件或生产
   配置恢复秘密。没有真实证据不得签发 Task PASS。
6. 满足当前全部必验项且完成 B 审查后，停止造 Task，提交 Closure Packet 并返回
   SLICE READY FOR A REVIEW；A 仍需最终审查，B 不自行关闭。

本轮仅调整验收文档，不新增运行或行为 PASS。Slice 保持 ACTIVE，Task 4.1 保持 FIX REQUIRED；
实际插件 SHA-1 两行、B 独立真实复现与完整 T4 复用裁定仍未完成。

```text
Closure Packet: PENDING
A Final Review: PENDING
Next Action: CONTINUE TASK 4.1 UNDER USER-AUTHORIZED MINIMAL PLAN
```
