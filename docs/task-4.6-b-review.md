# Task 4.6 B 审查：前置阻塞交接（2026-10-21）

## 固定范围与决定

```text
Task ID: 4.6
Task Base: b599dbc5c0e6bd394ee361343d74ebbb12d1ecab
Reviewed Documentation Head: c05e09f063dad29a9e1431688d6df11baaa141ae
Developer Documentation Checkpoint: cafa00f811b8f0c8a343e67092f158d11c7ca27e
Task 4.6 Product Implementation Head: NOT SET
Decision: FIX REQUIRED
Slice Required Outcomes all satisfied: NO
Next: SAME TASK 4.6; RESOLVE TASK-SPECIFIC PREREQUISITES BEFORE REAL IMPLEMENTATION
```

本轮从干净 `main` 开始，先读必读文件并运行治理 PASS。完整审查上述 Task Base..Head，
包括正式规划、实施指南、Developer 前置报告及交接，而非只审最后一个文档提交。
该范围12个文件均为 Markdown；没有 Go、依赖、产品构建、插件或验收工具改动。
Slice 原 Base 与现存4.5 Implementation Head 保持不变；不把文档检查点冒充4.6实现。

依据[开发流程](development-workflow.md)第4节，有明确阻塞可先拒绝。Developer 如实报告
BLOCKED 是正确交接，不是产品失败或越权取密的理由；但没有扫描／决定结果，不能给
整个 Task PASS。本轮不造新Task、不扩建过程工具、不降低原有AC或独立复现要求。

## 交付核对与阻塞

**F-46-01 / P1：4.6专用准入未解除，核心交付尚未发生。**

- [Developer报告](task-4.6-developer-prerequisite-blocked.md)与11份原始证据相符：只核对
  固定输入、专用三服务身份、已有公开／受保护入口、候选内容及10000本地输出。
  没有原始副本／专用库、扫描、当次插件读取、正常比较基准或八行实际决定。
- 4.4／4.5取密权限、历史UNKNOWN接受及一次性时间豁免不延伸到4.6；本轮用户的B职责
  指令没有新增4.6凭据渠道或S3写授权。B不读容器Env、历史秘密、Jellyfin DB，不恢复
  账户，不改服务生命周期，也没有猜测API路由或调用插件内部接口。
- 需要用户安全注入本Task的隔离Jellyfin认证，或明确4.6取用渠道与必要原生操作范围；
  S3还需明确4.6对同批最少自有样本必要替换／恢复及凭据渠道的授权。须覆盖Developer
  与B各自职责，再核对专用实例身份并实际鉴权。不要把秘密贴入聊天／日志／配置。
- 原始URL容器内连接拒绝、候选同网络URL返回A内容，是已披露的不同事实；不将其
  升级为扫描／插件PASS，也不要求改原始字节、建代理或改网络。仍从已存在的原生库管理、
  扫描和 `StrmProbeOptimizerBatchCheck` 任务入口继续；实际入口不足涉及改RO时才交A。

| 验收 | 本次B核对 |
|---|---|
| AC-1 | BLOCKED；无原始STRM入库、item/source和当次插件读取窗口 |
| AC-2 | BLOCKED；未建立必要主流／历史指纹／原生地址匹配及明确复用基准 |
| AC-3..5 | BLOCKED；D-01..08全部NOT RUN，无逐行因果／决定证据 |
| AC-6 | 仅只读保护身份已核对；无场景变更可恢复，不等于完整恢复及交接验收通过 |
| B独立关键复现 | BLOCKED／NOT RUN；仍须亲自复现原始扫描／读取及全部八行，未交回Developer |

## 本轮独立核对与指定全量责任

新证据根：`artifacts/validation-evidence/task-4.6-B1-20261021T124809Z/`，本机Git忽略，
不是远端CI产物。保留 `input-and-evidence-recheck.json`、`summary.json`、准确命令／退出码／
耗时、树校验原始stdout/stderr及后继文档检查。未修改固定媒体、远端、服务或4.5状态。

- 82项产品／依赖／部署／脚本／测试／夹具输入逐文件SHA-256与4.5 B5索引匹配，0差异；
  四份tracked辅助输入与实际工作副本逐字节匹配。完整差异范围仅文档。
- 清单、发布CLI、4.5状态库和原始主样本均与交接固定摘要匹配；没有将B5旧epoch26的
  状态库摘要当作当前恢复基准。当前状态库匹配Developer／4.5最终交接值。
- B从既有 `docs/task-4.5-artifacts/verify_strm_tree.py` 入口独立只读校验10000份原输出：
  10000/10000，0问题，Size字段10000、SHA-1字段0；不伪造源端未提供的哈希。
  准确命令见本次 `commands.json`，不是重新运行发布CLI或远端同步。
- 11份Developer原始材料逐份读取并记录摘要。原公开200／受保护401、原始URL失败及
  候选内容匹配，仅按该次前置核对证据接受；本轮未重复请求真实服务。

| 指定门槛 | 本轮处置与后续责任 |
|---|---|
| 当前治理／相关链接／补丁空白 | B本次执行，提交后再检查当前已提交合同；结果见文档检查证据 |
| Go格式／vet／非缓存全量／race，mod，双架构／testhooks构建 | 保留4.5已接受结果及本次82项输入匹配；本轮无改动，不机械重跑 |
| Python工具、4.5辅助15项、11同步／6serve、固定Get／缓存8场景11项 | 保留4.5原始结果与输入链；没有新增可执行输入，不向Developer追加全量 |
| 打包／Compose／amd64容器6项／arm64 OCI／重启 | 保留4.5已接受结果；本轮未重建容器或触碰真实服务生命周期 |
| 4.6原始扫描、完整实际矩阵、独立复现及恢复 | BLOCKED／NOT RUN，不被前几行的既有结果抵扣 |
| 4.6最终候选全量签收／Slice Final | NOT RUN；明确阻塞下提前拒绝，未据输入匹配冒充整套Final完成 |

既有结果具体身份链见[4.5 B3复用表](task-4.5-b-review-3.md#指定全量与精确复用)、
[4.5最终审查](task-4.5-b-review-5.md)。4.6有可审交付后，B继续负责固定最终Head及
可执行输入、产物、配置／夹具／环境与原始门槛的逐项精确复用或受影响补跑；不是让
Developer重跑整套。真实4.6决定不得复用旧4.1–4.3、4.4环境smoke或4.5同步冒充。

## 同Task继续与全部RO

1. 用户先解决4.6认证／必要变更授权；不要求Developer自行找回密码或通过文档消除阻塞。
2. 准入满足后Developer在同一Task按既有计划完成AC-1..6，先最少原始扫描／读取与正常
   基准，再八行单变量决定和逐场景恢复；仅做受影响验证，保留失败和真实Head。
3. B持有最终全量执行／精确复用、完整Base..最终Head审查和独立原始扫描／全部八行。
   本次严格小于50%的计时规则继续适用；不得空等凑比例或省略必要恢复。

| RO | 本次重评 |
|---|---|
| RO-1／2 | 保留4.4 PASS；固定资产／清单未见变更，本轮未重新建集或访问S3 |
| RO-3 | 保留4.5 PASS及已接受真实闭环；本轮10000本地输出独立校验通过，无新远端通过声明 |
| RO-4／5 | BLOCKED／NOT RUN；原始扫描／插件读取和完整D-01..08均未取得 |
| RO-6 | 部分；本次审查已记录，4.6完整门槛、独立复现、Slice Final／Closure／A Final尚缺 |

**Task 4.6 FIX REQUIRED；Slice 4 ACTIVE。** 当前不满足全部RO，不提交Closure Packet，
不返回SLICE READY FOR A REVIEW，不自行关闭。范围没有改变，不为授权前置另造Task。
计时以证据根 `timing.json` 为准，失败／只读验收／治理均计入；本轮不继承4.5时间豁免。

## 2026-10-21 后续：S3／OpenList授权延伸，Jellyfin由B协调

用户明确要求“S3 Openlist复用4.5 Jellyfin你给他提供”。已将4.5 S3／OpenList渠道和
最少样本必要操作／恢复边界延伸至4.6 Developer及B，写入AGENTS／根TASK；不再要求用户
重复授权这两项。未实际取密、鉴权或修改样本，不把授权登记写成场景完成。

Jellyfin由B协调提供，而非继续要求Developer找密码。B核对当前认证交接：4.4临时Key已
撤销、内存随机新密码未保存，执行进程的已知Jellyfin认证变量未发现可用值。没有可直接
提供给Developer的有效凭据，不从历史证据、Jellyfin DB或其他项目找回秘密。
已有原生账户恢复入口可解决认证，但会实际改变隔离管理员密码；需用户明确允许该副作用
后才能按同专用实例身份核对、临时Key及安全内存交接方案办理。目前尚未执行恢复或签发Key。

F-46-01剩余项缩为Jellyfin有效认证及其必要渠道；原AC、独立八行复现和小于50%计时规则不变。
Task仍FIX REQUIRED、Slice仍ACTIVE。此次只登记授权与认证交接事实，不冒充B Final／PASS。

## 2026-10-21 后续：Jellyfin认证前置解除，继续同Task实施

用户明确允许专用隔离Jellyfin官方恢复、改变管理员密码及本Task临时Key。B先提交授权
`55a08d0a8d08c572aadb233ce048d012d32b2b0f`，再核对专用实例／原生API定义并恢复；
新PIN文件核对／兑换／管理员认证／强随机密码／仅本次Key均成功，原生任务200且Idle。
真实子进程经sealed匿名memfd取得环境Key，受保护API200；不把凭据写文件或交给聊天。
步骤、真实入口、有效期和结束撤销见[认证交接](task-4.6-jellyfin-access.md)。

保留两项首次失败：准备元数据遗漏import、发生于恢复前；启动器使用X-Emby-Token头401，
改为现有已验证MediaBrowser头后200。没有因此反复恢复账户、重签Key或隐藏失败。
6项辅助直接检查通过，实际Key在本次auth／证据普通文件中未检出；不是全局无泄漏声明。
辅助仅用于本Task安全内存交接，不是产品新入口；后续B全量核对纳入其源码摘要与直接证据。

**F-46-01的授权／认证前置已解除。** 没有创建副本／库、改插件配置、扫描或触发检查任务，
AC-1..6和B独立八行仍NOT RUN；不能据此给Task PASS。Developer可开始同Task修正，
仅做受影响验证；最终全量／精确复用及独立原始扫描／全部八行仍由B承担。
临时Key及恢复会话当前保留供实施，尚未撤销／退出；结束／过期清理须按实际结果复核。
