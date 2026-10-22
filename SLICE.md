# Slice 4 — 复用隔离资产的 V4.4 万文件与插件决策验收

```text
Slice ID: 4
Owner: A
Status: PASS / CLOSED
Base SHA: d1c95ad079a0e3f952d9e57ab50299c6ae93ad63
Implementation Head: 24e1ba0ab88173f2c7374f450c9750e5a1e9b504
```

## 用户目标

依据 [V4.4 需求](docs/requirements.md)，复用已经完成的隔离环境、媒体原件及媒体生成／
启动／停止脚本，固定一批 10000 个 S3 媒体文件，经真实 OpenList 生成 10000 个 STRM；
同批最少样本证明同步、Jellyfin 原始产物扫描与插件完整探测／复用决策。
记录实际数量、耗时和峰值 RSS，只核对插件决定，不要求探测结果成功或客户端播放。

## 重规划时的执行基线（历史）

按用户最新澄清，旧 4.1／4.2／4.3 从当前任务安排中撤下；不继承其待审事项、修复清单
或验收结论。历史记录留在 Git 与原始证据中，当前只保留后续需要的实际成果。
重规划提交时，Slice 1–3 PASS / CLOSED 与原 Slice Base 不变；新 4.4／4.5／4.6 均未实施，
Implementation Head 为 NOT SET。现三项均已独立 B PASS，实际实现 Head 以上方顶栏为准。

可复用成果仅作为输入资产：隔离环境、媒体原件、生成／启动／停止脚本、固定插件源码／
包及当前适用的产品／测试工具。当前 RO 的结果由新任务取得，不以旧任务记录抵扣。

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

不重选远程 HEAD／latest，不访问 `/opt/jf-strm-probe` 或生产配置。按 2026-10-20 用户
会话授权，Task 4.4 可在核对 Compose project、服务、镜像及挂载身份后，从既有专用
隔离容器 Env 选择性读取必要凭据，仅在内存／子进程环境中使用；不得读取其他项目或
生产容器，不从历史文件恢复秘密。不得输出／保存完整 Env 或将秘密写入配置、日志、
证据及 Git。当前进程未预设变量不再等同于凭据不可取得；实际取用／鉴权仍须验证，
不可用时如实记录。该授权不表示 F-01 已解决，不自动删除隔离数据，其他 RO／AC 不变。

## Required Outcomes / Surfaces

| ID | 必须交付的结果 | 验收面 |
|---|---|---|
| RO-1 | 已有环境／媒体／三脚本可追溯复用，工具范围明确 | 原件与脚本摘要、隔离实例／持久路径身份、正常复用入口与当前所需工具清单 |
| RO-2 | 同一批固定 10000 个 S3 媒体对象 | 本地原件／对象一一对应清单、数量及摘要；只一次补齐，后续不重生成 |
| RO-3 | 真实 OpenList／发布 CLI 万文件生成及最少样本同步 | 10000 STRM 映射／协议／稳定 URL、耗时／RSS、稳定重复零重写；新增／更新／跳过／移动／清理／再出现／dry-run、三完整轮与删除前 Get、样本恢复 |
| RO-4 | 最少原始生成 STRM 被原生扫描并由固定插件读取 | 原始字节摘要、路径／媒体库条目对应、扫描／读取与比较基准；无需全量入库 |
| RO-5 | 完整比较表 D-01..08 的实际插件决定 | 每行输入变化、真实 SHA-1／标注副本、item／source／当次日志窗口、明确探测或理由匹配的复用，无旧暂缓例外 |
| RO-6 | 分层回归、证据复用、独立审查及收尾完整 | Slice 1–3 工具／产品回归保留；完整候选门槛逐项执行或匹配复用；B 重评所有 RO，A 最终审查 |

## 三个行为闭环与实施顺序

| Task | 结果与依赖 | 测试等级 |
|---|---|---|
| 4.4 | 复用隔离资产，固定 10000 文件清单与比较样本；完成后交独立 B | T3 |
| 4.5 | 4.4 PASS 后，发布 CLI 万文件生成／测量与同批最少样本同步、清理、恢复 | T4 |
| 4.6 | 4.5 PASS 后，最少原始 STRM 扫描与 D-01..08 实际决策，完成全部 RO 及 Slice Final | T4 |

当前无活动实施任务，见 [TASK](TASK.md)；重规划时的后续候选分别见
[4.5](Task/4.5-generation-sync-plan.md)、[4.6](Task/4.6-plugin-decisions-plan.md)。
当时的候选不提前填写不存在的 Base／实施 Head，前序 PASS 后由 B 基于真实仓库正式规划提交。
该次提交仅重规划，未执行建集、生成、启动／停止或真实验收；后续完成结果见关闭记录。

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
B 审查各 Task 完整 Base..Head，关键真实行为独立复现；新任务之间可复用身份相同的证据。
旧任务运行记录仅作参考，不作为当前 RO 已完成或替代新任务执行。全部 RO 满足后形成 Closure Packet，A 审查 Slice 原 Base
至最终 Implementation Head，决定关闭；本次重规划不代签 B／A PASS。

## Closure Packet

```text
Closure Packet: docs/slice-4-closure-packet.md (B COMPLETE)
A Final Review: docs/slice-4-a-final-review.md (COMPLETE)
Reviewed Range: d1c95ad079a0e3f952d9e57ab50299c6ae93ad63..24e1ba0ab88173f2c7374f450c9750e5a1e9b504
Reviewed Documentation Successor: 93b0b02aa2bcce854dd82293979aa5f22e8aae27
Decision: PASS
P0/P1 Blockers: NONE
Next Action: NONE — CURRENT REQUIREMENTS COMPLETE
```


2026-10-22 关闭事实：4.4／4.5／4.6 均已独立 B PASS，见
[4.6 B3](docs/task-4.6-b-review-3.md) 与 [Closure Packet](docs/slice-4-closure-packet.md)。
正文中的“本次重规划／未实施”保留规划时点与已提交能力合同，不表示当前交付缺失；
当前状态以上方顶栏及本关闭事实为准。全部 RO 的 B 验收及 Slice Final 已齐备；
A 已审原 Base..最终实现 Head 及纯文档后继，RO-1..6 最终 PASS、无 P0／P1 阻塞，
详见 [A Final](docs/slice-4-a-final-review.md)。不追认历史越界或 UNKNOWN、不宣称探测
成功／播放；非阻断 P2 与未运行边界保留。

## Closure Reconciliation

本关闭检查点同步 SLICE／TASK／README、架构 CURRENT、范围与兼容性等当前文档、
Roadmap／Progress 及 Closure Packet；根 TASK 撤为无活动实施任务，三角色经验已复盘。
原 Base、已审实现 Head、RO／验收及业务规则不变，不修改产品或隔离服务。
本地参考方案随后已退出治理与版本跟踪，见[进度](docs/progress.md)。
Slice 4 **PASS / CLOSED**，当前 V4.4 开发需求已完成，没有必须继续的下一 Slice；新增工作须由用户另行授权。
