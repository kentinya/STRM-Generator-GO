# Task 4.6 Developer 前置核对与阻塞交接（2026-10-21）

> 后续处置：用户已明确将4.5 S3／OpenList授权延伸至4.6 Developer与B；本报告中的
> “没有4.6 S3授权”为原时点事实，不再作为当前阻塞。Jellyfin由B协调提供，尚未取得；
> 当前准入以根[TASK](../TASK.md)为准，原始未运行／阻塞证据不改写为PASS。
> 最新处置：用户另已允许本Task官方账户恢复，B恢复／临时Key／子进程认证实测成功；
> 按[认证交接](task-4.6-jellyfin-access.md)继续实施。下述无认证的原交接事实保留，不代表当前仍无凭据。

本文件记录 [Task 4.6](../TASK.md) 开发者在实施前对固定输入、隔离实例、原生入口与
凭据准入的真实核对结果。按 Task 准入第 1、4 条，当前执行进程没有 4.6 专用 Jellyfin
凭据，也没有 4.6 S3 样本变更／恢复授权，因此 AC-1～AC-5 保持 **BLOCKED**，不得写成
PASS；本轮只继续允许的代码／文档与只读核对，不改服务、媒体、S3、Jellyfin 配置或
4.5 原输出。4.4／4.5 的取密授权、历史 UNKNOWN 接受与一次性计时豁免均未延续到 4.6。

```text
Task ID: 4.6
Task Base: b599dbc5c0e6bd394ee361343d74ebbb12d1ecab
Contract Checkpoint: b599dbc5c0e6bd394ee361343d74ebbb12d1ecab
Repository baseline: e9c5a1a08e348429ce7ad7363f2522f3b7021efd
Status on handoff: READY FOR B REVIEW (AC-1..5 BLOCKED; no 4.6 implementation)
```

## 1. 已核对的固定输入（只读）

| 输入 | 实测身份 | 与 Task／Slice 固定值 |
|---|---|---|
| `artifacts/task-4.4/dataset/manifest.jsonl` | SHA-256 `378068e317ca30787c3fea5bfbefa298a27ad4c430cfa149cd6eeccc8479d3c9`，10000 行，A 9999／B 1 | 一致；dataset ID `slice4-v44-378068e3…` 可复算 |
| `artifacts/task-4.5/bin/strmgen-amd64` | SHA-256 `bcfe9010c365f0eb70c843bf3fbf0b69d908a4ffef1c2641c874241fcbd40d61` | 一致 |
| 插件锁定源码 `artifacts/task-4.1/plugin-source` | HEAD `2afc6517fda5a7b4170ebea625619a552c47dabc`，`git status --porcelain` 0 行 | 一致 |
| 插件 ZIP | SHA-256 `382d3ef106ed8534b06ab5bff614d826e602a29703dadddbe3ef2e9864bd06d8` | 一致（包，不是 DLL） |
| 已安装 DLL | SHA-256 `027b279c068a7ed58996b110fd01e96126f043509918fa546d9f8d4b4d61fdef` | 与规划核对值一致；实际加载窗口仍待 AC-1 取得 |
| A 原件 `artifacts/task-4.4/inputs/A.mkv` | 24404 字节，SHA-1 `1b7aa9d139d059cdcc579d04e9e36a6b005fcd7e`，SHA-256 `7f53d154…f96f12d` | 与 samples／dataset 一致 |
| B 原件 `artifacts/task-4.4/inputs/B.mkv` | 24430 字节，SHA-1 `4799935022aa2bed93478865689aef92ab116ba5`，SHA-256 `e1aee32f…4d7600` | 一致 |
| 4.5 原输出 `artifacts/task-4.5/strm-output` | 10000 个 STRM；独立校验 10000/10000，`problem_count=0`，Size 字段 10000、SHA-1 字段 0（该批 OpenList 未提供 hash） | 保留不变；本轮未运行同步覆盖 |
| 4.5 状态库 `artifacts/task-4.5/state/strmgen.db` | 4128768 字节，SHA-256 `985d03705af31c4970a7a3b33a2b19e531d3641a865185c41b7d479ec8a859b8`（本轮只读身份） | 只读保护对象，本轮零写入 |

主样本 `Plain Movie.strm` 来源身份：93 字节，SHA-256
`9596a77b1d841870e9a9e494e8ace574e04de1f94a0d52326eba349adccd25f4`，内容为版本 1、
Size 24404、原始生成 URL `http://127.0.0.1:25244/d/s3/Plain%20Movie.mkv`；无 SHA-1 行。
这是 4.5 的真实生成字节，后续 AC-1 必须原样复制，不能为可达性改写。

## 2. 专用隔离实例身份（只读，未读取 Env）

| 服务 | 容器 | 镜像 digest | 专用挂载（只列本 Task 相关） | 状态 |
|---|---|---|---|---|
| jellyfin | `strmgen-slice4-s3-jellyfin-1` | `jellyfin/jellyfin@sha256:357724bf0ae27a672c7cbaa899db2d9abeb13dbd8657ccce750258a4c059d037` | `strm-output → /strm:ro`、`media → /media:ro`、`jellyfin-config`／`jellyfin-cache`、插件目录 | healthy，已运行 |
| openlist | `strmgen-slice4-s3-openlist-1` | `openlistteam/openlist@sha256:11b490123d57f88c84ecda46285e1921869cf511da3e5bb4062f183e7e1200b7` | `openlist-data` | 运行中 |
| rustfs | `strmgen-slice4-s3-rustfs-1` | `rustfs/rustfs@sha256:9ca9559533b537d10482fe923288b676ad08f92a7069d9a17327fbae3cab688b` | `rustfs-data`、`rustfs-logs` | 运行中 |

Compose project 均为 `strmgen-slice4-s3`，服务标签与 Slice 锁定值一致。未读取任何容器
环境变量，未重启、重建或修改服务。

## 3. 原生入口与决策基线的可达性核对

- Jellyfin 公开端点：`GET http://127.0.0.1:28096/System/Info/Public` → HTTP 200，
  `ServerName=Slice4`、`Version=12.2.0`、`StartupWizardCompleted=true`。
- Jellyfin 受保护端点：`GET /System/Info` 无凭据 → HTTP 401，未尝试猜测密码、读取 DB、
  历史文件或其他项目容器。执行进程环境也没有本 Task 专用 Jellyfin／S3 凭据变量。
- 决策用候选入口（仅从 Jellyfin 容器读取，不冒充原始 URL 可达）：同网络
  `http://openlist:5244/d/s3/Plain%20Movie.mkv` 与 `.../Synthetic%20001.mkv` 均 HTTP 200，
  `Content-Length: 24404`；实际流式读取 SHA-1 均为
  `1b7aa9d139d059cdcc579d04e9e36a6b005fcd7e`，与 A 原件一致。该事实只作为后续 AC-2
  基准的候选入口核对，**不构成**原始产物可达、扫描或插件决策证据。
- 原始生成 URL 在 Jellyfin 容器内仍连接拒绝（`127.0.0.1:25244`）。这与规划一致；
  原始字节保留，后续 AC-1 如实记录原始扫描结果，不禁用或改写。

## 4. 本轮未执行项与原因

| 项目 | 状态 | 原因 |
|---|---|---|
| AC-1 原始副本、专用库、原生扫描、固定插件读取 | **BLOCKED** | 无 4.6 Jellyfin 认证；不能创建库、触发扫描或读取任务日志 |
| AC-2 正常比较基准（主流／历史／地址） | **BLOCKED** | 依赖 AC-1 与原生检查 |
| AC-3／AC-4／AC-5 D-01..08 实际矩阵 | **BLOCKED** | 依赖 AC-1／AC-2；D-02／04／06 另需 4.6 S3 样本写授权 |
| AC-6 场景恢复 | NOT RUN（无可恢复变更） | 本轮未创建副本、库、设置或 S3 变更；只完成只读身份与本地 10000 输出校验 |
| 4.5 万文件重跑／同步 | 未执行（也不需要） | 输入身份匹配，原输出只读保留；不重跑已匹配全流程 |
| Go／辅助工具／B 指定全量 | NOT RUN | 本轮无产品、插件、工具或可执行输入改动；完整门槛归 B |

本轮没有向 `/strm`、媒体、Jellyfin 配置、插件配置或 S3 写入任何字节；没有创建
`task-4.6/` 子目录、媒体库或临时设置，因此不存在需要恢复的 Task 变更。`git status`
除本轮新增文档／证据目录外无第三方改动，4.5 输出与状态库身份保持上表值。

## 5. 解除阻塞所需（交用户／A／B）

1. **Jellyfin 认证（4.6 专用）**：用户安全注入本 Task 执行进程可用的隔离实例凭据，或
   明确定义 4.6 的取用渠道与允许操作。仅限 `strmgen-slice4-s3` 的 Slice4 Jellyfin；
   不读历史文件／Jellyfin DB／完整 Env，不重置账户或改服务生命周期。凭据只进内存与
   子进程环境，不写入 argv、配置、日志、证据或 Git。
2. **S3 样本变更（4.6 专用）**：明确授权对同批已确认自有对象 `Plain Movie.mkv` 与
   `Synthetic 001.mkv`（需要时仅这两个）用 A／B 原件做最小内容替换与恢复；不重建数据集、
   不改未知对象／bucket。无此授权时 D-02／D-04／D-06 保持 BLOCKED，其余行可先做。
3. **入口确认**：获权后按 Task 使用原生媒体库管理、扫描与“计划任务 → STRM 媒体信息
   检查”（Key `StrmProbeOptimizerBatchCheck`）；若公开 HTTP API 方法／字段不明确，先
   核对当前实例描述，不猜路由、不调用插件内部接口。
4. **时间**：Task 4.6 不继承 4.5 的计时豁免；下一次实施按“测试时间严格小于本轮 50%”
   预估，并在场景及恢复完成后如实记录。本轮开发按用户新指令执行更严格的小于 30%。

在 1、2 满足前，不创建半成品库／副本或改配置来“先占位”，也不把原始 URL 不可达写成
插件决策通过。B 的独立复现同样需要其职责范围内的有效授权。

## 6. 证据与计时

原始命令输出在 `artifacts/validation-evidence/task-4.6-dev-20261021T1242Z/`（本机，
Git 忽略）：

| 文件 | 内容 |
|---|---|
| `00-fixed-inputs.txt` | 输入／清单／原件／插件源码／4.5 输出与 DB 摘要 |
| `01-container-identity.txt` | project／服务／镜像 digest／挂载（未读 Env） |
| `02-jellyfin-access-and-entry.txt` | 公开 200、受保护 401、候选 URL 响应头 |
| `03-verify-tree.json`／`.time`／`.exit` | 10000 输出独立只读校验（3.669s，0 问题） |
| `04-content-entry-check.txt` | 原始 URL 不可达；候选入口流式内容 SHA-1 与 A 一致 |
| `timing.txt` | 本轮墙钟与测试类用时 |

本轮从 `2026-10-21T12:42:27Z` 开始，只读验收／治理／校验合计约十余秒；剩余为必读、
核对与文档撰写，测试类时间远低于用户本轮 30% 上限。未运行 Go 或 B 的全量门槛，不把
本轮只读事实当作扫描、决策或产品 PASS。

## 7. 结论

AC-1～AC-5 因缺少 4.6 专用认证与 S3 变更授权而 **BLOCKED**；AC-6 无可恢复变更，仅完成
只读身份与输出未变核对。固定输入、隔离实例、插件产物与候选入口可用性已核对，未发现
输入或环境漂移。交 B 审查本轮交接事实；Authorization 解除后在同一 Task 内继续实施，
不重规划、不扩大范围，也不自行签署 PASS。
