# Slice 4 — 复用隔离资产的 V4.4 万文件与插件决策验收

```text
Slice ID: 4
Owner: A
Status: ACTIVE
Base SHA: d1c95ad079a0e3f952d9e57ab50299c6ae93ad63
Implementation Head: 8109811b46575576f1b87530d65aea298bf2d576
```

## 用户目标

依据 [V4.4](<STRM Generator Go.md>)，复用已经完成的隔离环境、媒体原件及媒体生成／
启动／停止脚本，固定一批 10000 个 S3 媒体文件，经真实 OpenList 生成 10000 个 STRM；
同批最少样本证明同步、Jellyfin 原始产物扫描与插件完整探测／复用决策。
记录实际数量、耗时和峰值 RSS，只核对插件决定，不要求探测结果成功或客户端播放。

## 重规划与历史处理

本次由用户明确要求完全重新规划，替代上轮“4.2 治理后执行单一 4.3”的任务安排。
Slice 保持 ACTIVE 和原 Base；Implementation Head 保留最近实际工具实现检查点，不把
本次规划提交冒充实施 Head。Slice 1–3 PASS / CLOSED 不变。

| 旧工作 | 当前处理 |
|---|---|
| Task 4.1 | 最后 FIX REQUIRED，旧活动范围已撤下；报告／历轮审查保留在 `f0767f0:TASK.md`，不补签 PASS |
| Task 4.2 | `8109811b46575576f1b87530d65aea298bf2d576` 工具修改保留；`4f4cfee:TASK.md` 中 READY FOR B REVIEW／B PENDING 为真实历史。本次撤下其旧活动计划，未完成独立审查由 4.4 明确承接，不以重规划变成 PASS |
| 旧 Task 4.3 候选 | 未激活、未执行；撤回原单一大任务，详细原计划留在 `4f4cfee:Task/4.3-validation-plan.md` |
| 旧真实链路原件 | 201 项分页、五类字节、稳定重复、扫描及移动生成／清理／恢复按 [复用账本](docs/slice-4-evidence.md) 保留；只复用身份匹配的行为，不能代替新万文件或完整决策 |

## 已有资产与复用边界

- 隔离环境、媒体和三个脚本原地保留在 `artifacts/task-4.1/isolated-s3/`；不因重规划
  删除、重建或改名。媒体／工具当前事实及摘要见 [资产清单](docs/slice-4-assets.md)。
- 本轮只读清点：`media/` 有 201 个同内容媒体文件，`media-src/` 有 1 份源媒体，均为
  24404 字节。不是现成的 10000 文件数据集；远端现存对象数和服务运行状态仍须实际核对。
- `generate_media.py`、`bootstrap.sh`、`teardown.sh` 均保留复用；生成脚本当前会覆盖
  201 个路径写占位字节，不能直接重跑覆盖媒体。4.4 只在需要时作有原件记录的最小适配。
  已运行的隔离服务直接复用；需要恢复时复用启动脚本。停止脚本按需使用，保留数据。
- 固定数据集一次补齐，既有媒体不重新生成；新内容／对象仅在建集时补足并纳入同一清单，
  含比较所需的不同真实内容／Size 样本。固定后所有复核／重跑只复用该批对象与本地原件。
- 原始证据、数据库／隔离服务数据和锁定插件包均保留。旧播放工具不恢复；不要求清空
  artifacts，不删除保存历史或复现来源所需的脚本／依赖。保留不等于无条件执行。

## 固定外部输入

- OpenList：源码 `3b28270a4066ea52943a69ecddd850d83f7e8bdc`；镜像
  `openlistteam/openlist@sha256:11b490123d57f88c84ecda46285e1921869cf511da3e5bb4062f183e7e1200b7`，v4.2.7。
- S3/RustFS：复用既有隔离实例；4.4 核对并记录实际镜像 digest、持久目录及 bucket／前缀脱敏标识。
- Jellyfin：`jellyfin/jellyfin@sha256:357724bf0ae27a672c7cbaa899db2d9abeb13dbd8657ccce750258a4c059d037`，12.2.0。
- 插件：`https://github.com/kentinya/jellyfin-strm-probe.git` 锁定源码
  `2afc6517fda5a7b4170ebea625619a552c47dabc`；已有包 SHA-256
  `382d3ef106ed8534b06ab5bff614d826e602a29703dadddbe3ef2e9864bd06d8`，须核对安装／实际加载身份。

不重选远程 HEAD／latest，不访问 `/opt/jf-strm-probe` 或生产配置。凭据只从专用进程
环境注入，不恢复历史文件／容器 Env 中的秘密；历史凭据事件保持真实开放状态，复用前
确认现有授权与凭据允许继续，缺失则如实 BLOCKED，不自动轮换／删除历史文件。

## Required Outcomes / Surfaces

| ID | 必须交付的结果 | 验收面 |
|---|---|---|
| RO-1 | 已有环境／媒体／三脚本可追溯复用，工具范围明确 | 原件与脚本摘要、隔离实例／持久路径身份、正常复用入口、旧工具处置及 4.2 待审修改的独立审查 |
| RO-2 | 同一批固定 10000 个 S3 媒体对象 | 本地原件／对象一一对应清单、数量及摘要；只一次补齐，后续不重生成 |
| RO-3 | 真实 OpenList／发布 CLI 万文件生成及最少样本同步 | 10000 STRM 映射／协议／稳定 URL、耗时／RSS、稳定重复零重写；新增／更新／跳过／移动／清理／再出现／dry-run、三完整轮与删除前 Get、样本恢复 |
| RO-4 | 最少原始生成 STRM 被原生扫描并由固定插件读取 | 原始字节摘要、路径／媒体库条目对应、扫描／读取与比较基准；无需全量入库 |
| RO-5 | 完整比较表 D-01..08 的实际插件决定 | 每行输入变化、真实 SHA-1／标注副本、item／source／当次日志窗口、明确探测或理由匹配的复用，无旧暂缓例外 |
| RO-6 | 分层回归、证据复用、独立审查及收尾完整 | Slice 1–3 工具／产品回归保留；完整候选门槛逐项执行或匹配复用；B 重评所有 RO，A 最终审查 |

## 三个行为闭环与实施顺序

| Task | 结果与依赖 | 测试等级 |
|---|---|---|
| 4.4 | 复用隔离资产、承接工具边界／旧 4.2 待审修改，固定 10000 文件清单与比较样本；完成后交独立 B | T3 |
| 4.5 | 4.4 PASS 后，发布 CLI 万文件生成／测量与同批最少样本同步、清理、恢复 | T4 |
| 4.6 | 4.5 PASS 后，最少原始 STRM 扫描与 D-01..08 实际决策，完成全部 RO 及 Slice Final | T4 |

当前可实施任务仅以 [TASK](TASK.md) 为准；后续候选分别见
[4.5](Task/4.5-generation-sync-plan.md)、[4.6](Task/4.6-plugin-decisions-plan.md)。
候选不提前填写不存在的 Base／实施 Head，前序 PASS 后由 B 基于真实仓库正式规划提交。
本次仅重规划，不执行建集、生成、启动／停止或真实验收，不宣称新的环境／产品 PASS。

## 比较与证据判定

按 [需求](docs/requirements.md) 和 [D-01..08](docs/test-matrix.md#slice-4-实际插件决策矩阵)。
哈希／Size 比较和不探测行须有必要主媒体流、历史指纹和当前原生地址匹配；URL 变化行
先建立地址匹配的旧基准，变更后可由 UrlChanged 或本次变化导致的 NativeUrlMismatch
触发探测。必须排除无关主流／地址缺失，不能借其他条件冒充该行通过。

目标缺元数据的生成器分支以既有 Go 回归单列；实际源可提供的变化仍真实同步。插件
测试允许用同批本地原件的真实 SHA-1 和标注 STRM 副本；异常 Size 明示，异内容的哈希、
实际 Size 和服务字节匹配。原始生成文件单独保留，副本不作为同步结果。
每行先核对／恢复基准，再改变该行所列条件；FirstTrust、缺日志或只见任务完成不足。
探测明确要求或启动即可，不探测须有理由匹配的明确复用记录。探测结果成功不作行门槛；
后续缺基准时先正常恢复建立。按场景结束核对恢复，未知远端状态不算已恢复、不带污染续验。

## 约束与最终验收

遵守 AGENTS、需求与开发流程；生成器不运行 ffprobe、不下载媒体补算哈希、不调用插件
内部接口／Jellyfin DB，不验证播放、会话／进度、代理、探测结果成功或更高规模压力。
保持归属／路径／互斥／SQLite／dry-run／完整轮边界，不建元数据服务、通用补偿框架或三组件编排。
ARM64 实机与正式发布不在当前合同；512m 部署配置不是新增内存保证。

全部 RO 都须有固定源码／可执行输入／产物／镜像／插件／配置／媒体清单身份、命令、
退出码、数量、耗时及适用 RSS、原始日志／摘要、首次失败／重跑与未运行项。
B 审查各 Task 完整 Base..Head，关键真实行为独立复现，逐项复用相同输入证据；不能只凭
工具全绿或旧 201 项记录通过。全部 RO 满足后形成 Closure Packet，A 审查 Slice 原 Base
至最终 Implementation Head，决定关闭；本次重规划不代签 B／A PASS。

## Closure Packet

```text
Closure Packet: PENDING
A Final Review: PENDING
Decision: PENDING
Next Action: COMMIT TASK 4.4 PLAN, THEN IMPLEMENT AND INDEPENDENT B REVIEW
```
