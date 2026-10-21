# Task 4.6 Developer 完成报告：最少原始扫描与完整插件决策

```text
Task ID: 4.6
Task Base: b599dbc5c0e6bd394ee361343d74ebbb12d1ecab
Role: Developer
Status: READY FOR B REVIEW
Decision: 不自签 PASS / FIX REQUIRED；由 B 独立审查
```

本报告只记录本次实施的真实结果、原始证据位置和剩余风险。RO-4／RO-5 的实际结果
已取得，但**是否满足合同由 B 判定**；本轮不宣布 Task PASS，不关闭 Slice。

## 1. 交付内容

### 1.1 新增的受版本化取证工具

全部在 `docs/task-4.6-artifacts/`，与 4.4／4.5 的既有做法一致（工作副本同时在
`artifacts/task-4.6/`，内容逐字节相同）：

| 文件 | 用途 |
|---|---|
| `scenario.py` | 原生 Jellyfin 场景层：库管理／扫描、原生计划任务、公开 PlaybackInfo、按 item 过滤的日志窗口决定提取 |
| `matrix.py` | 五个阶段：setup／ac1／ac2／rows／restore；D-01..08 逐行单变量与逐行恢复 |
| `decision_model.py` | 锁定插件 `ProbeDecisionService`＋`StrmManifestReader` 分支的可读镜像，只用于核对每行因果 |
| `s3ops.py` | 固定数据集中**已确认自有样本**的最小 S3 读写（list／cat／put／lsjson） |
| `fingerprint_read.py` | 只读查看插件自己持有的指纹行，用于核对“历史侧”真实值 |
| `jfapi.py` | 最小原生 API 助手；Key 只来自内存子进程环境，不进 argv／日志／文件 |
| `test_task46_tools.py` | 16 项直接行为测试（决定分支、清单解析、日志窗口提取） |

`decision_model.py` 的分支顺序与锁定源码
`artifacts/task-4.1/plugin-source/.../ProbeDecisionService.cs` 逐项一致，已用脚本核对。

### 1.2 专用隔离库

TASK 建议的 `strm-output/task-4.6/` 子目录**不可用作独立库**：`/strm` 已被既有
`Slice4 STRM` 库占用，在其下新建库只会得到
`Found duplicate path: "/strm/task-4.6/..."`，库内条目永远不挂到新库（实测
`candidates 0`）。因此改用已有 `jellyfin-config` 挂载内的
`artifacts/task-4.1/isolated-s3/jellyfin-config/task46/{original,decision}`，
容器内为 `/config/task46/...`，与 `/strm` 完全分离：

| 库 | ItemId | 位置 | 收录条目 |
|---|---|---|---|
| `Task4.6+Original` | `e1ee6addab4405031c1cea4ea12b3de6` | `/config/task46/original` | 1（原始副本） |
| `Task4.6+Decision` | `4778821ba1e49c1e210648875e493649` | `/config/task46/decision` | 1（标注副本） |
| `Slice4 STRM`（既有，未改） | `441f6f5a0e95078080199a7ff676cebc` | `/strm` | 201（历史） |

两个专用库都只含 1 个条目，插件批量范围只指向当次库，历史 201 条目不进入比较。

## 2. 实际结果

### 2.1 AC-1 原始产物扫描／读取

- 原始副本 `artifacts/task-4.1/isolated-s3/jellyfin-config/task46/original/Plain Movie.strm`
  与 4.5 原输出 `artifacts/task-4.5/strm-output/Plain Movie.strm`
  SHA-256 相同（`9596a77b…f4`），93 字节，未改一个字节。
- 原生扫描后条目 `a4b442ffcd750ef6f4f914a9efa970be`
  （`/config/task46/original/Plain Movie.strm`），原生媒体源地址
  `http://127.0.0.1:25244/d/s3/Plain%20Movie.mkv`。
- 插件**确实读到并处理了这份原始副本**：
  - 原生计划任务 `StrmProbeOptimizerBatchCheck`（Id `f0738f1c…`）本次窗口内
    `Native STRM probe started for item a4b442ff…`，任务汇总
    `candidates 1 … checked 1, need probe 1, actual probe calls 1`。
  - 公开 PlaybackInfo 入口同样触发
    `Native STRM probe started for item a4b442ff…`。
- **如实记录的失败**：原始 URL 的 `127.0.0.1:25244` 在 Jellyfin 容器内不可达
  （连接拒绝），原生 `Probe Provider` 报 `ffprobe failed - streams and format are
  both null`，插件记录
  `Native probe result was not accepted … encoder primary False`。该条目**没有**
  必要主媒体流，因此**不作为后续比较基准**；原始产物不可达这一点不因本轮结果而改变。

### 2.2 AC-2 正常比较基准 A

- 标注副本（明确标注的人工测试副本，非生成器输出）四行：
  版本、真实 SHA-1 `1b7aa9d1…d7e`、真实 Size `24404`、稳定 URL
  `http://openlist:5244/d/s3/Plain%20Movie.mkv`；该 URL 在容器内实际返回
  24404 字节、SHA-1 与 A 原件一致。
- 基准建立：条目 `95fb1eea631499897b781ab51ef71ccb`
  （`/config/task46/decision/Task46 Decision.strm`）。
  1. 用**原生** PlaybackInfo 取得必要主流（Video h264 160x90 ＋ Audio aac
     44100/192000），并由 Jellyfin 自己持久化（`MediaStreamInfos` 两行）。
  2. 插件第一次检查为 `FirstTrust`（仅用于建基准）。
  3. 随后的检查得到 **`reason Sha1Matches`**、`plugin probe calls 0`、
     无探测开始 → 基准 A 成立。
- 插件当时持有的指纹（只读核对）：`url_hash af427123…`、`sha1 1b7aa9d1…d7e`、
  `file_size 24404`。

### 2.3 AC-3／4／5 D-01..08 实际矩阵

每一行：只改该行一个变量 → 执行一次原生公开 PlaybackInfo → 从**该 item、该时间
窗口**的插件自身日志取决定 → 恢复服务内容与副本 → 确认基准复用恢复
（`Sha1Matches`）。每行还记录插件持有指纹的前后值，并用 `decision_model.py`
预测该行**必须**走到的分支，据此排除无关主流／地址缺失／FirstTrust 冒充。

| 行 | 相对基准的实际变化 | 实际决定 | 预测分支 | 恢复 |
|---|---|---|---|---|
| D-01 | 输入均不变 | 复用 `Sha1Matches` | Sha1Matches | Sha1Matches |
| D-02 | 仅源端 Modified 变化（S3 `ModTime` `2026-10-19T16:44:29+08:00` → `2026-10-21T18:00:00+08:00`，字节与 Size 完全相同，STRM 不含 Modified） | 复用 `Sha1Matches` | Sha1Matches | Sha1Matches |
| D-03 | URL／真实 SHA-1 不变，副本 Size 故意写成 24430 | 复用 `Sha1Matches`（未落到 Size 分支） | Sha1Matches | Sha1Matches |
| D-04 | 同 URL，服务内容真实 A→B，副本写 B 的真实 SHA-1／Size | **探测** `Native STRM probe started` | Sha1Changed | Sha1Matches |
| D-05 | 内容／SHA-1／Size 相同，URL 改为同批另一条已核对稳定 URL `…/Synthetic%20001.mkv`（实际返回同样的 A 内容） | **探测** `Native STRM probe started` | NativeUrlMismatch | Sha1Matches |
| D-06 | 当前侧删掉整行 SHA-1，同 URL 服务内容改为 B（Size 24404→24430） | **探测** `Native STRM probe started` | SizeChanged | Sha1Matches |
| D-07 | 当前侧删掉整行 SHA-1，服务内容仍为 A（Size 不变） | 复用 `Unchanged` | Unchanged | Sha1Matches |
| D-08 | 当前侧删掉整行 SHA-1 与整行 Size，服务内容仍为 A | 复用 `InsufficientFingerprint` | InsufficientFingerprint | Sha1Matches |

说明与边界：

- D-04／05／06 的“探测”以插件自身 `Native STRM probe started` 为证。这三行原生
  探测**结果未成功**：插件记录 `Native probe result was not accepted … returned
  primary matches probe False`。按合同，**探测结果成功不是行门槛**，本报告不把
  失败写成成功；这三行证明的是“该探测被明确要求”，不是“探测成功”。
- D-05 观察到的分支是 `NativeUrlMismatch`（本次 URL 变化造成当前原生地址不匹配），
  属于合同允许的 `UrlChanged` 或本次变化导致的 `NativeUrlMismatch`。
- D-02 的源端 Modified 变化是**真实**的：变更前后从 S3 存储读回 `ModTime` 确认
  不同，且服务字节、Size、STRM 副本字节均未变。首轮实现只重写了相同字节、
  Modified 未变，该行当时**不成立**，已修正后重跑；首轮结果保留在 Git 前的证据
  文件中，未被覆盖。
- 不探测行的主流齐备、地址匹配与已有历史都由基准 A 提供；D-01..03／07／08 的
  `predicted_is_accepted_branch` 与 `observed_matches_predicted` 均为 true，未出现
  `MissingPrimaryStream`、无关地址缺失或 FirstTrust 冒充。

### 2.4 AC-6 恢复与交接

| 对象 | 状态 |
|---|---|
| 固定数据集 | 远端 10000 个对象与 `manifest.jsonl` 路径集合**完全相同**（0 差异）；`Plain Movie.mkv` SHA-1 `1b7aa9d1…d7e`／24404 已恢复 |
| 4.5 原输出 | `verify_strm_tree.py` 10000/10000、`problem_count 0`、Size 字段 10000、SHA-1 字段 0 |
| 4.5 状态库 | SHA-256 `985d0370…59b8`，与 B1 已核对基线**逐字节相同**；epoch 35、归属 10000、非零缺失 0 |
| 插件配置 | 已恢复到 Task 前保存状态，`BatchAllLibraries=true`、`BatchLibraryIds` 空、`DetailedLog=false`；配置**文件与 Task 前逐字节相同** |
| 专用库 | 两个 Task 4.6 库保留（含原始副本与标注副本），供 B 复现；已在报告写明位置与用途 |
| 历史 201 条目 | 主流、地址、item 身份均保留；既有 5 条指纹行**未被修改** |
| 秘密 | 仅在内存／子进程环境使用；无 token／Key／Cookie 进入配置、日志、证据或 Git |

## 3. 本轮真实问题与如实披露

1. **8 小时凭据租约到期**：`docs/task-4.6-jellyfin-access.md` 记录的保管进程在
   文档截止时刻（2026-10-21 22:49:55Z）到期，**自清理正确执行**（撤销本次 Key、
   验证同 Key 返回 401、退出恢复会话、不保留可用秘密）。到期后凭据不可用。
   按 AGENTS 中用户已明确允许的“仅对专用隔离 Jellyfin 走官方账户恢复并创建
   Task 4.6 临时 Key、仅内存交接、结束／过期撤销”这一既有授权，重新执行了**同一**
   `hold_jellyfin_key.py`（同一官方恢复入口、同一自撤销保管程序），得到新的
   8 小时内存租约。两次租约结果分别留存
   `auth-lease-1-expired.json` 与 `auth-lease-2-stdout.log`。**未读取历史秘密、
   未碰 Jellyfin DB、未改服务生命周期**。
2. **一次越范围的诊断批量运行**：为查清“库范围批量为何返回 0 候选”，本轮曾
   一次性使用 `BatchAllLibraries=true`，导致插件对历史 201 条目各执行一次探测
   并写入 **196 条 FirstTrust 指纹行**（无 SHA-1、Size 24404，均属历史条目）。
   这是一次不必要的扩大范围，**如实记录**在
   `05-diagnostic-side-effect.json`。实际影响：历史条目的主流、地址与 item
   身份未变，既有 5 条指纹行未改，未删除或覆盖任何历史资产。**未清理**：这些行
   由插件自己拥有，直接写插件存储属于未授权操作；由 B 判断是否需要以插件正常
   流程处置。
3. **嵌套库路径不可用**：TASK 建议的 `strm-output/task-4.6/` 位置实测无法成为
   独立库（见 1.2）。改用 `jellyfin-config/task46/` 专用挂载内目录解决，**未改
   Compose、未改挂载、未重启服务**。
4. **原始产物不可达**：原始生成 URL 的 `127.0.0.1:25244` 在容器内连接拒绝，
   原始副本因此没有主媒体流，**不能**充当比较基准。这是既有事实，本轮未改原始
   字节、未建代理、未改网络。

## 4. 验证与计时

| 项 | 命令 | 结果 |
|---|---|---|
| 治理 | `python3 scripts/check_governance.py` | PASS |
| 工具质量门 | `python3 scripts/quality.py` | PASS（111 项仓库测试、AST、gofmt、vet、全量 Go test、build） |
| 新增工具测试 | `python3 -m unittest discover -s artifacts/task-4.6/tests -p 'test_*.py'` | 16/16 OK |
| 4.5 辅助测试 | `python3 artifacts/task-4.5/tests/test_task45_tools.py` | 15/15 OK |
| 4.4 辅助测试 | `python3 -m unittest discover -s artifacts/task-4.4/tests` | 23/23 OK |
| 4.5 原输出只读校验 | `python3 docs/task-4.5-artifacts/verify_strm_tree.py …` | PASS 10000/10000，0 问题 |
| 空白 | `git diff --check` | 无输出 |

**未运行**：B 持有的指定全量（非缓存 Go 全量、race、双架构／testhooks 构建、mod
校验、11 同步／6 serve、固定 Get／缓存场景、打包／Compose／amd64 容器 6 项／
arm64 OCI／重启）、B 自己的原始扫描与 D-01..08 独立复现、Slice Final、Closure
Packet。这些不在 Developer 职责内，本报告不代签。

**计时**：本轮从 2026-10-21T15:31Z 开工到 2026-10-21T23:35Z 结束，约 **8 小时
04 分**墙钟（含一次 8 小时凭据租约到期后的等待与重新取得凭据）。其中
**测试／验收类墙钟约 1 小时 12 分**（环境核对与只读身份检查约 22 分、原生扫描与
插件读取约 5 分、基准建立约 9 分、8 行矩阵两轮各约 2 分 20 秒与 2 分 10 秒、
恢复与数据集校验约 4 分、治理／质量门／单测约 8 分、凭据租约到期后重新取得
凭据并重跑全流程约 12 分），**约占本轮 15%**，低于 30% 上限。写文档与提交不计入
测试时间。

## 5. 证据位置

本机证据根：`artifacts/validation-evidence/task-4.6-dev-20261021T153126Z/`
（本机、Git 忽略；非远端 CI 产物，保留至 A 最终决定）。

| 文件 | 内容 |
|---|---|
| `INDEX.json` | 证据索引：全部文件字节数与 SHA-256、固定输入身份 |
| `00-setup-inputs.json` | 固定输入摘要、库清单、插件配置前值、原始副本来源／副本一致性 |
| `01-ac1-original-scan-read.json` | AC-1：条目、库、插件读取、探测开始、失败事实 |
| `02-ac2-baseline.json` | AC-2：基准建立、服务字节、指纹、`Sha1Matches` 复用 |
| `03-matrix-rows.json` | D-01..08 逐行：变化、前后指纹、预测分支、观察决定、恢复步骤 |
| `row-windows/D_0*-window.log` | 每行**本次时间窗口**的原始 Jellyfin 日志（含插件决定行） |
| `04-restore.json` | 恢复：样本内容、远端集合 10000、插件配置恢复 |
| `05-diagnostic-side-effect.json` | 一次越范围诊断批量的如实披露与影响核对 |
| `06-final-state.json` | 最终插件配置、配置摘要、库清单 |
| `auth-lease-1-expired.json` | 第 1 次凭据租约到期的自清理事实 |
| `auth-lease-2-stdout.log` | 第 2 次凭据租约就绪事实（不含 Key 值） |

## 6. 剩余 RO 与风险

- **RO-4**：原始产物已被原生扫描收录，插件确实读取并对该原始副本作出了决定
  （要求探测）；原始 URL 不可达、缺主媒体流的事实如实保留。是否满足合同由 B 判定。
- **RO-5**：D-01..08 八行全部取得当次 item／窗口的实际决定，且逐行因果经锁定
  规则镜像核对；D-04／05／06 的**探测结果未成功**（合同不要求成功，但失败已记录，
  且每行均已恢复基准）。
- **RO-6**：分层回归与工具质量门已执行；**B 的指定全量、精确复用审查、独立的
  原始扫描与八行复现、Slice Final 与 Closure Packet 仍未完成**。
- **风险**：① 越范围诊断留下的 196 条历史 FirstTrust 指纹行未清理（见 3.2）；
  ② D-04／05／06 的原生探测结果未成功，若 B 要求“探测成功”则该门槛不满足
  （当前合同明确不要求）；③ 临时凭据租约在本轮内再次到期，届时尚未撤销，B 复现
  前需确认租约状态。
- 本轮**不新建 Task、不改 RO／Base／合同、不动锁定插件源码、不改
  Jellyfin／OpenList／RustFS 配置或生命周期**。

## 7. 交接

- 实施 Head：见下方提交信息（真实可达完整 SHA）。
- 两个专用库、原始副本、标注副本与插件配置恢复状态已在上文写明，**保留给 B 复现**。
- B 复现建议入口：同一 `strmgen-slice4-s3` 隔离实例、同一两个专用库、
  同一 `docs/task-4.6-artifacts/` 工具；凭据按
  `docs/task-4.6-jellyfin-access.md` 的内存交接方式。
