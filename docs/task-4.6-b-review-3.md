# Task 4.6 第三次 B 审查（2026-10-22）

```text
Task ID: 4.6
Task Base: b599dbc5c0e6bd394ee361343d74ebbb12d1ecab
Implementation Head: 24e1ba0ab88173f2c7374f450c9750e5a1e9b504
Reviewed Documentation Successor: 54f2dd2be0c3bc0c5de06f40c7b9d837b80f6850
Decision: PASS
Slice Required Outcomes all satisfied: YES — B acceptance; A final review remains pending
Next: SLICE READY FOR A REVIEW; NO NEW TASK; NO SLICE CLOSURE BY B
```

## 固定范围与先核对交付

入口 main 干净；先检查真实 status、AGENTS、全部必读文档及三角色指南，治理 PASS。
本轮审完整原 Task Base..修正 Implementation Head 及文档后继，不只看最后四文件。
`reviewed-task-base-to-implementation.diff` 与 `reviewed-document-successor.diff` 保留完整范围。
没有 Go 产品、依赖、部署或锁定插件改动。B 不成为产品实施者、不签 A Final。

本机原始证据根：`artifacts/validation-evidence/task-4.6-B3-20261022T010708Z/`。
`candidate.json` 固定完整源码身份；`task46-inputs.json` 记录七对 tracked／实际工作副本：
仅有已声明的 `parents[2]`→`parents[3]` 路径适配，不称逐字节完全相同。
这些是 Git 忽略的本机证据，不是远端 CI 产物，保留至 A 最终决定。

Developer 已交付的原始读取、A 基准、八行原生日志及最终人工恢复仍是有效事实；
旧指纹 DB 读值不用于本轮证明。未重写历史 INDEX、自引用摘要、首次失败或越界披露。
用户已接受的 F-46-04 及 4.5 历史 UNKNOWN 不重新设为阻塞，也不追认为当时获授权。

## 同 Task 修正判定

- **F-46-02 关闭**：`fingerprint_read.py` 已退役为 fail-closed 桩，无 sqlite 导入、
  库路径或读取实现；实际矩阵不再调用该入口。B 本轮从正常基准的 `Sha1Matches`、
  已记录的 A 原件／副本、逐行当前主流／地址及恢复连续性说明历史侧，不读 Jellyfin／
  插件 DB，不把 Python 镜像或构造的 Fingerprint 当成插件内部实际读值。
- **F-46-03 修正接受**：restore 从 setup 保存的前值取配置，缺失时拒绝回退至当前值。
  它原为不单独阻断的 P2；本轮实际连续调用 setup..restore，原生配置前后逐字段相等。
  不为此追加自动恢复平台、分进程恢复能力或新业务 Task。
- **F-46-04 保持 ACCEPTED BY USER / NON-BLOCKING**：保留两项历史披露；本轮没有
  扩大到历史 201 条目、清理指纹或让 Developer 恢复账户。未来权限不扩大。

## B 独立原生复现

B 重新核对同一 Compose project 的 Jellyfin／OpenList／RustFS 服务、锁定镜像及全部
挂载，先于任何取密。按已有 B 专属授权及现存认证入口执行官方恢复、更改隔离管理员
随机密码和创建本次临时 Key；只读此次新恢复文件，秘密仅在内存／子进程环境。
本次原生 API 描述再次确认 RefreshItem 等实际方法／字段；不猜路由、不调用插件私有接口。

使用既有两个专用库与同一原始／标注副本，不新建库、不重建数据集、不改服务生命周期。
通过已交付 `matrix.py` 阶段入口和公开原生 RefreshItem／PlaybackInfo／计划任务执行。
`independent-native-replay.py` 仅为本轮观察与日志保存包装，未改产品或验收工具实现。
每次 native task 都核对候选范围为专用库的一条，不以聚合计数替代逐 item 决定。

**AC-1**：对现有原始 item 经当前 API 明确支持的原生 RefreshItem 排队扫描，随后
本次原生检查及同 item PlaybackInfo 均实际观察到 `Native STRM probe started`。
源／副本 SHA-256 同为 `9596a77b1d841870e9a9e494e8ace574e04de1f94a0d52326eba349adccd25f4`，
item 为 `a4b442ffcd750ef6f4f914a9efa970be`，路径为 `/config/task46/original/Plain Movie.strm`。
原字节未改；容器内原 URL 不可达、没有必要主流的限制保留，不要求探测成功。

**AC-2..5**：同一决策 item `95fb1eea631499897b781ab51ef71ccb`，A 正常主媒体流、
原生地址与基准副本一致，先取得 B 自己的明确 `Sha1Matches`，而不是 FirstTrust。
每行前保留主流／地址／副本事实，每行后恢复并再观察 `Sha1Matches`。

| 行 | B 本次逐 item 原生观察 | 单变量及因果核对 |
|---|---|---|
| D-01 | 明确复用 Sha1Matches | 不变 |
| D-02 | 明确复用 Sha1Matches | S3 Modified 实际改变，字节／STRM 不变 |
| D-03 | 明确复用 Sha1Matches | 同 URL／真实 SHA-1，只有副本 Size 故意异常 |
| D-04 | Native STRM probe started | 同 URL，服务内容 A→B，真实 SHA-1／SizeB；正常前提对应 Sha1Changed |
| D-05 | Native STRM probe started | 内容不变，仅换同批稳定 URL；本次变化造成 NativeUrlMismatch |
| D-06 | Native STRM probe started | 当前缺 SHA-1、历史 A 有效，同 URL／实际 SizeB；对应 SizeChanged |
| D-07 | 明确复用 Unchanged | 当前缺 SHA-1，两侧有效 SizeA 同，不声称哈希验证 |
| D-08 | 明确复用 InsufficientFingerprint | 当前缺 SHA-1 和 Size，历史 A 有效，不凭空探测 |

探测行日志证明的是**实际启动**，不是日志带有 Sha1Changed／SizeChanged reason 字段；
后两者由已核对正常前提、历史连续性及单变量输入排除其他分支。镜像预测只辅助核对，
没有冒充日志或成功探测。`windows/`、`D-01.json`..`D-08.json`、
`03-matrix-rows.json` 保留本次 item／source、窗口、原始决定、输入与恢复。
三个探测行不要求结果成功，没有把探测启动改写为成功更新媒体信息。

## 恢复与 AC-6

- S3 只改已确认自有 `Plain Movie.mkv`，使用既有 A/B 原件；另一稳定 URL 样本只读。
  八行结束均确认 A 基准；最终 A 字节为 24404、SHA-1 `1b7aa9d139d059cdcc579d04e9e36a6b005fcd7e`。
  最终 S3 API 对账 **10000 路径及 Size 与固定清单完全相同**；不是用 OpenList 代替 S3 清单。
  原件与其他对象未重生成；Modified 的变化不声称回到旧值。
- 原始 4.5 输出再次 **10000/10000、0 问题**；B7 保存的 10001 条输出／状态记录逐项
  字节、Size、mtime 完全相同，生成器 DB SHA-256 仍为 `985d0370…59b8`，未跑新同步。
- 插件全部原生配置前后逐字段相等；两个已交付专用库及原始／A 标注副本继续保留，
  没有删除已有库、未知对象或历史指纹。见 `configuration-restore-confirmed.json`。
- 本次 Key 已显式撤销，使用同 Key 返回 401，恢复会话退出 204，holder 退出 0；
  `auth-B3-final-revoked.json` 保留实际事实。未误撤其他 Key，无活跃租约可继续沿用。

## 指定全量与精确复用

当前 82 项产品／依赖／部署／测试执行输入与 4.5 B5 全部相同；四份 4.5 工作副本、
八份 4.4 工作输入也准确匹配。`gate-reuse-confirmed.json` 逐项保留原命令、退出码、
摘要、产物／镜像身份及适用范围，不仅凭 Go 源码相同复用。

| 门槛 | 本轮处理 |
|---|---|
| 当前治理／相关链接／补丁空白 | 本轮执行；文档交接提交后再次检查当前合同 |
| 仓库 Python | 本轮 111/111 PASS，约 2.9 秒；包括现有文档消费测试 |
| 4.6 工具 | 本轮 25/25 PASS；先因未设置 TASK46_EVIDENCE 导入失败，补正确环境后通过，首败保留 |
| 4.6 语法／退役入口及直接影响 | 源码／AST 与 25 项覆盖、B 原生完整阶段和配置恢复；无禁止 DB 依赖 |
| mod download／verify、gofmt／vet／Go 全量／race | 精确复用 4.5 B1 原始成功日志及命令；非缓存／race 与普通缓存结果分开 |
| amd64／arm64／testhooks 构建与 15 项 4.5 辅助测试 | 精确复用 B1；保留原二进制摘要与命令／退出码 |
| 11 同步／6 serve | 精确复用 B1 summary、执行二进制摘要及原始成功日志，无失败 |
| 固定真实 OpenList Get／缓存 | 精确复用原 8 场景／11 项；核对 retained binary、8 份原始 Go 日志无 FAIL／SKIP、函数 AST 与源／镜像身份；不扩大到新增 mirror 入口 |
| 打包／Compose／amd64 容器 6 项／arm64 OCI／重启 | 精确复用实际 4.5 B2 container-completion，不用其较早 incomplete 报告；镜像 ID、OCI 摘要、命令／退出 0、6 个 PASS 再核对 |
| 4.4 建集／只读 probe 工具 | 八项输入不变，复用 B3 的 23 测试、三反例修正、语法与 B4 接受的真实固定证据；不重建媒体／下载万文件 |
| 4.5 万文件／最少同步与恢复 | 保留独立 B7／B8 已接受闭环，94 份原始证据字节摘要再次全部匹配；本轮不机械重跑十轮 |
| B 独立原始扫描与 D-01..08 | 本次执行 PASS，完整阶段约 187.85 秒；最终 S3 只读对账另约 42.09 秒 |

所有门槛由 B 签收，未交回 Developer。当前阶段不重复同输入 Final，不将 Python 夹具、
交叉构建、HTTP200 或任务完成冒充真实决定。ARM64 实机、正式发布、客户端播放、
代理、更高规模均 NOT RUN／合同排除，不标 PASS。

## Slice Final 与交接

本轮对当前已提交能力合同、固定最终实现 Head、输入匹配、分层证据、全部 RO、恢复、
历史处置和未运行边界完成 Slice Final 核对。文档后继只协调事实，不改能力合同、Base、
版本或安全不变量；提交后的治理是同一 Final 的文档收口，不再重复编译／全套真实运行。

| RO | B 当前重评 |
|---|---|
| RO-1 | 满足：4.4 可追溯资产／三脚本／专用服务及固定插件身份保持 |
| RO-2 | 满足：固定清单、同批原件，最终 10000 S3 路径／Size 无差异，样本内容恢复 |
| RO-3 | 满足：4.5 独立完整闭环保留，当前原输出及状态身份不变 |
| RO-4 | 满足：B 独立原始字节刷新扫描、条目对应及固定插件实际读取 |
| RO-5 | 满足：B 独立八行当次决定、正常基准／因果与逐行恢复，不靠 DB 或源码预测代替 |
| RO-6 | B 交接面满足：完整 Task 审查、指定门槛、复用、独立验证及 Closure 齐备；A Final 仍 PENDING |

**Task 4.6 PASS。** 无待 Developer 实施的阻塞，不为非阻塞工具润色造 Task。
完整 Slice Base..Head、Task／RO／最终证据、不变量、历史处置与文档协调项见
[Closure Packet](slice-4-closure-packet.md)。Slice 4 **READY FOR A REVIEW**，不自行关闭。

计时见本证据根 `timing.json`：真实任务／API等待、失败与重试、恢复和只读验收均计入；
没有采用 4.5 豁免、空等凑比例或扣除真实测试等待。未单独计时的早期只读检查另计保守
上界，连同实测测试时间仍严格小于本轮总墙钟 50%。
