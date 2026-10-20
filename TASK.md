# Task 4.4 — 复用隔离资产并固定万文件数据集

```text
Task ID: 4.4
Parent Slice: 4
Status: PASS
Task Base: 77fc768790a1c1ba6f0700c0aab294d3237cf7e4
Contract Checkpoint: 48ff46a2cfd742190de0e8a673a92ec61c086950
Difficulty: High
Test Level: T3
Planner / Reviewer: B
```

## Goal

闭合 [Slice 4](SLICE.md) RO-1／RO-2 并推进 RO-6：保留复用已完成的隔离环境、媒体
原件及生成／启动／停止脚本，固定一批可重复使用的 10000 个 S3 媒体文件，交下一任务
直接消费。只验收当前目标，不继承旧 4.1／4.2／4.3 的任务或审查要求。

## 实际缺口与范围

规划时只读核对 [资产清单](docs/slice-4-assets.md)：media/ 201 个同内容 .mkv，media-src/
1 份源媒体，均为 24404 字节；三脚本原地存在，摘要已留存。本轮没有运行服务命令或读取
历史秘密，远端对象数／当前服务健康／凭据可用性仍须核对，不能假设万文件已经建好。
旧 generate_media.py 会覆盖 201 路径写占位内容；不能直接重跑。bootstrap.sh 会准备
自有目录／属主、启动专用 Compose 并等待健康；teardown.sh 是不带 -v 的专用 down。
保留其来源和作用，需要适配时先留可核对原版，最小修改，不另建通用环境管理器。

允许：只在 artifacts 自有隔离对象内核对身份、按需恢复环境，复用本地原件／三脚本，
首次补足固定数据集／比较变体并记录清单；必要最小工具适配及直接回归。现有环境正在
运行时直接复用，不要求为了展示脚本而重建或下线。禁止重生成／覆盖现有媒体、删数据
目录／旧证据、从历史文件恢复秘密、接管生产或重新选择镜像／插件 HEAD。
2026-10-20 用户已授权：核对 Compose project、服务、镜像与挂载身份后，可从本任务
既有专用隔离容器 Env 选择性取用必要凭据，仅在内存／子进程环境中使用；不得读取
生产／其他项目容器，不得输出或保存完整 Env、秘密或带秘密的配置／证据。

## 验收条件

- AC-1／RO-1：记录隔离实例／固定镜像 digest、专用持久目录／端口、插件源码／包／加载
  身份、原件与三脚本摘要。沿用合同已固定版本，核对当前专用访问凭据可用；
  凭据由进程环境注入；按本轮用户授权，允许从已核对的专用隔离容器 Env 选择性读取
  必要值并仅在内存／子进程环境中使用，不从历史文件恢复秘密，不输出或落盘秘密。
- AC-2／RO-1：原件不动，三脚本保留。生成入口适配（如需要）只用真实保留媒体作源，
  支持补缺失及冲突拒绝，不覆盖现有媒体／清单；测试执行只用 artifacts 下另设的最少
  临时样本，不在真实媒体上验证覆盖拒绝。bootstrap／teardown 仅按需作用于既有专用
  project／路径，不包含删持久数据／媒体／证据；已运行服务不因验收强制重建。
- AC-3／RO-2：先核对远端现有自有对象，复用并仅一次补足至固定清单内恰好 10000 个
  S3 媒体对象。逐项对应本地保留内容，保存 dataset ID、相对路径／真实 Size／SHA-256
  清单及对象清单摘要；至少包含后续 SHA-1／Size 变化所需不同真实内容样本。当前原件
  只有一种内容，先查可复用既有变体，确实缺少时仅首次建集补最少变体并纳入同批清单。
  不为每行重新生成媒体，不用 ETag／路径哈希冒充 SHA-1，不清理未知远端对象凑数量。
- AC-4／RO-1／2：验证真实 OpenList 可读取配置源根与该数据集的分页／样本元数据、稳定
  URL 的对应自有字节，列表 refresh:false。测试侧可对照本地媒体计算真实 SHA-1；
  生成器仍不下载媒体补哈希。确认 Jellyfin 专用入口／固定插件可用于后续原生检查，
  不在此提前宣称扫描／全部决策通过。
- AC-5／RO-6：列明当前需要的生成／启动／停止入口、固定工具和测试命令，提供本任务
  执行结果及脚本原版／必要修改的准确摘要与可审查文本。只保留后续适用的成果，
  旧任务记录不作为本任务完成依据，也不形成独立审查前置。
- AC-6：将固定清单、原件位置、隔离环境／专用输出配置身份、最少样本与恢复方法交给
  4.5／4.6。已有媒体／源原件与历史证据保持；新建集结果不能冒充生成器产品验收。
  Task 完成报告逐项映射 RO／AC，交 READY FOR B REVIEW，不自签 PASS。

## 实施方式

开发者按 [Task 4.4 开发实施指南](docs/task-4.4-implementation.md) 顺序实施。
该指南细化现有 AC，不改变合同；其规划建议已有实现，当前结果以完成报告与独立 B 复审为准。

| 顺序 | 开发动作 | 完成后应留下什么 |
|---|---|---|
| 1 | 核对现有隔离环境／版本／当前访问，保存原件和脚本身份 | 环境描述、初始媒体摘要／mtime；运行正常直接复用 |
| 2 | 核对 A 和已有 B 候选，保留到稳定原件位置 | 两种真实内容与 Size／摘要，后续无需逐行造媒体 |
| 3 | 最小改造生成脚本为补缺失／冲突拒绝，先在小目录测试 | 可审查脚本／测试、零覆盖／重复零写／失败重试结果 |
| 4 | 形成确定候选，仅补足本地与 S3 缺失对象并对账 | 恰好 10000 的本地／远端清单及真实内容对应 |
| 5 | 经 OpenList 完整分页、精确样本和稳定 URL 核对，确认原生入口可用 | 状态／计数／摘要和当前可用性；不提前执行 4.5／4.6 |
| 6 | 固定清单、样本与恢复来源，完成本任务报告 | dataset_id、工具／配置身份和 AC-1..6 证据，交独立 B |

## 必要测试

### 开发／修正反馈

先运行治理，核对保留资产摘要／隔离边界；有脚本适配时只在新专用小目录验证补缺失、
已有文件不覆盖、冲突拒绝及重复零改写，再操作真实自有数据集。脚本不改则不编造新增
测试。失败停止并记录已尝试／已确认／未知状态，确认恢复前不带污染继续。

### 最终检查点门槛

cwd 为仓库根；证据放新的 `artifacts/validation-evidence/task-4.4-<run-id>/`。

```bash
python3 scripts/check_governance.py
python3 scripts/quality.py
bash -n artifacts/task-4.1/isolated-s3/bootstrap.sh artifacts/task-4.1/isolated-s3/teardown.sh
python3 -c 'import ast; from pathlib import Path; p=Path("artifacts/task-4.1/isolated-s3/generate_media.py"); ast.parse(p.read_text(), filename=str(p))'
git diff --check
```

quality 包含 Python／Go 基础回归；改了 artifacts 辅助脚本时还须有上述直接行为验证。
环境已运行就使用已有实例，不强制运行 bootstrap／teardown；按需执行前先核对专用
project、挂载／固定镜像／凭据和当前状态，用现有命令／脚本，不打印环境或带秘密渲染。
记录真实 S3 对象计数／本地摘要对应、OpenList 分页／样本和稳定字节读取；仅本地文件数
或脚本语法检查不足。原始媒体／脚本无改动部分逐项比对，修改部分记录前后身份与理由。

本 Task 不改产品行为，不要求为环境复用重跑全部镜像／race／故障重启；若发现必须改变
产品，先由 B 按真实风险调整同一 Task 到 T4 并补齐门槛，不在 T3 下暗改产品安全规则。
4.5 的真实生成／同步及 4.6 的扫描／全部决策不由本 Task smoke 代替。

### B 独立验证

审查本任务完整 Task Base..Head；核对当前合同、版本摘要、工具
保留理由、三个脚本原件／实际行为和固定清单。独立复核一个保留媒体摘要、S3 清单恰好
10000 及对应 OpenList 样本，确认未重建原环境、未覆盖原件且后续无需重新生成媒体。
脚本发生修改时独立复现补缺失／冲突拒绝关键路径；未改则可复用相同输入的工具结果。
B 必须独立于实现作者，逐项重评所有 RO，PASS 后才正式规划 4.5，不代 A 关闭 Slice。

### 证据与重跑

保存完整源码 SHA、未提交／artifacts 可执行输入的准确字节／摘要、脚本原版及必要补丁、
固定镜像／插件加载／配置／数据集身份、命令／cwd、退出码／计数／耗时、原件摘要、
首次失败／重跑、未知状态与恢复。日志脱敏，不包含 token、Cookie、
签名 URL 或私有完整路径。纯文档后继可复用输入相同的结果并重跑治理／链接／空白；
脚本、配置、数据或产物改变补跑受影响行为，不隐藏 skip／失败。

## 非目标

不执行万文件产品生成／RSS 和全部同步验收、Jellyfin 原始扫描／完整决策；不恢复旧播放
runner、不验证播放／代理、更高规模、ARM64 实机／发布；不重搭环境或重新生成旧媒体，
不建设通用凭据／日志／补偿框架，不改生成器协议／状态机／归属／删除／锁／SQLite。

## Developer Completion Report

```text
State: READY FOR B REVIEW（Task 头部 Status 字段为唯一权威）
上一实现 Head: e76a21c8e54a373fe53219b7a1aba5e86036f960
本轮实现 Head: 12a9c4f273efbdb9856a85b9a335528c32585d57（后继文档提交记录）
证据索引: artifacts/validation-evidence/task-4.4-8eF41E/（本机、Git 忽略）
可审查文本快照: docs/task-4.4-artifacts/（脚本原版／适配版／两份测试／工具／配置准确字节）
```

- Changed Files / Implemented（本轮，修正 B2 的 F-01／F-02／F-05）：
  - `artifacts/task-4.1/isolated-s3/generate_media.py`（Git 忽略）最小适配：
    - F-02：`_publish` 与 `write_manifest` 的临时文件改为 `xb` 排他创建；已存在同名临时对象
      一律以 `conflict` 保留并拒绝；只有本次成功创建后才在 `finally` 清理，创建失败不再删除既有对象。
      媒体临时名与清单临时名两处均验证。
    - F-05：新增 `_strm_target`／`_assert_unique_targets`，镜像 `internal/pathmap` 的扩展名替换，
      在写入前拒绝映射到同一 `.strm` 目标的媒体路径（如 `Movie.mkv`／`Movie.mp4`）。
  - `artifacts/task-4.4/tests/test_generate_media.py`：新增 F-02 同 PID 媒体临时名保留拒绝、
    清单临时名保留拒绝、F-05 同目标映射写前拒绝 3 项。
  - 准确字节同步至 `docs/task-4.4-artifacts/`（`generate_media.py`、`test_generate_media.py`）。
  - 未改 Go 产品、插件源码、生产配置、Jellyfin DB、bootstrap.sh、teardown.sh、probe 工具、现有媒体或原件。
  - 数据集固定：`artifacts/task-4.4/dataset/manifest.jsonl`（10000 行）与 `dataset.json`（dataset_id）已生成；
    `manifest.candidate.jsonl` 保留。
- Tests and Results / Evidence：
  - **F-01 远端固定**：核对专用容器身份后，从 rustfs 容器 Env 内存取用凭据。S3 API 从 201 补足至 10000；
    上传后集合 missing／extra／size_mismatch 全 0；`rclone check --download --one-way` 10000/10000 内容匹配；
    最终清单 SHA-256 `378068e317ca30787c3fea5bfbefa298a27ad4c430cfa149cd6eeccc8479d3c9`，
    dataset_id `slice4-v44-378068e3…`。
  - **真实 OpenList**（`openlist-probe-final.json`，约 17 min）：源根目录、递归 10000、清单集合／Size 匹配、
    8 个样本（A、B、空格／`#`／`%`／引号／中文）精确 Get 与稳定 `/d/` 字节 SHA-256 全部匹配，退出 0；
    全程 `refresh:false`，无签名 URL 或秘密进入日志。
  - **F-02／F-05 独立重放**（`reproduce_fix2.py`／`reproduction-fix2.json`）：3/3 PASS，既有临时对象保留、
    无目标写入；同目标映射退出 2。
  - 直接测试 23/23 PASS（0.166 s）；治理／`bash -n`／两脚本 `ast.parse`／`git diff --check` 全部退出 0；
    `scripts/quality.py` PASS（15.5 s）。
  - 201 原件字节／mtime 未变，0 临时文件；A／B 原件 SHA-256 未变。
  - Jellyfin 入口 HTTP 200；安装 DLL 与包内 DLL SHA-256 一致 `027b279c…`；未宣称扫描／决策通过。
  - 本轮约 2280 s；本地测试类约 16 s（<1%），含必要真实只读 probe 后测试类约 48%，未空等凑比。
    产品全量回归由 B 负责。
- Decisions / Risks / Remaining Required Outcomes：
  - F-01／F-02／F-05 已修正并有证据；AC-1..6 的本地、S3 与只读 OpenList 部分可复审。
  - 真实凭据仅内存／子进程使用，未打印／落盘；本轮未启停或重建隔离服务。
  - 未运行项：4.5 生成／同步／RSS、4.6 扫描／完整决策、代理与播放、产品 race／故障重启。
    NOT RUN 不作为 PASS。
  - 未改产品行为，不触发 T4 门槛。
- Head SHA：实现提交 `12a9c4f273efbdb9856a85b9a335528c32585d57`（修正 F-01／F-02／F-05、固定最终清单与 dataset、同步快照与事实文档）；本报告后继文档提交记录该 Implementation Head。

### 上一轮（F-02..F-04）完成报告（历史保留）

```text
上一实现 Head: 01104ed2c879ae36b28ac8cacea301296272c92f
证据索引: artifacts/validation-evidence/task-4.4-FIX-20261020T055937Z/
```
  - `artifacts/task-4.1/isolated-s3/generate_media.py`（Git 忽略）最小适配：
    - F-02：删除跨次 `clean_stale_temps` 前缀清扫；`iter_media` 遇到未知 `.strmgen-tmp-` 对象
      一律以 `conflict` 保留并拒绝，不再 unlink 无法证明本次创建的文件。`_publish` 仍只清理
      本次写入的 `TEMP_PREFIX+pid+name` 临时文件。
    - F-03：新增 `_assert_owned_path`，写入／验证前核对媒体根及每个路径祖先，拒绝静态符号
      链接／类型冲突；`prepare`／`verify_only` 在任何写入前先校验全部计划路径，候选复用不会
      先经链接发布再失败。`_verify_target`／`_publish` 增加 `media_dir`／`rel` 参数并在内部复核。
    - 清单位置：`main` 新增 `_is_within` 校验，`--manifest` 位于媒体根内时写前以 `usage` 拒绝。
  - `artifacts/task-4.4/tools/openlist_readonly_probe.py`（Git 忽略）重写为验证门禁：
    - F-04：样本 Get 的 HTTP／应用 code、`is_dir`、Size、稳定 URL 字节任一失败均 `return 2`；
      新增 `walk_tree` 完整递归分页并逐目录核对 total／重复名，`--manifest` 时比较递归集合与逐项
      Size（缺／多／不符均非零）；稳定 URL 逐段 `quote` 编码，覆盖 `bulk/`、中文及特殊名。
  - `artifacts/task-4.4/tests/test_generate_media.py`：新增 F-02 未知前缀保留拒绝、F-03 候选
    复用静态链接写前拒绝、清单置于媒体根内写前拒绝 3 项；修正旧 `_publish` 调用签名。
  - `artifacts/task-4.4/tests/test_probe_readonly.py`（新增）：9 项 probe 直接测试，覆盖坏 Get／
    目录 Get／Size 不符／不完整分页／清单缺多不符非零，及含 `bulk/`、中文、特殊名的递归集合成功。
  - 准确字节见 `docs/task-4.4-artifacts/`（含新增 `test_probe_readonly.py` 快照）。
  - 未改 Go 产品、插件源码、生产配置、Jellyfin DB、bootstrap.sh、teardown.sh、现有媒体或候选清单。
- Tests and Results / Evidence：
  - B 三反例独立重放（`reproduce_fix.py`／`reproduction-fix.json`）：未知前缀保留不变＋ToolError
    PASS；候选复用静态链接无根外写入＋ToolError PASS；probe 坏 Get／字节 `exit_code=2` PASS。
  - 直接测试 20/20 PASS（0.175 s，含 F-02..F-04 新覆盖）。
  - 真实只读 OpenList（未传 manifest）：源根目录、递归 201 文件、7 个样本（含中文／`#`／`%`／引号／
    空格）Get 与稳定 `/d/` 字节 SHA-256 全部匹配，退出 0。
  - 真实候选清单 `--verify-only` 10000 PASS；媒体根仍 10000 文件、0 临时文件；201 原件与 A／B
    原件 SHA-256 未变；候选清单 SHA-256 未变。
  - 治理、`bash -n`、两脚本 `ast.parse`、`git diff --check` 全部退出 0（`final-gates.log`）。
  - 本轮测试类进程合计约 0.33 s，占本轮约 1200 s 的约 0.03%，低于 30%。产品全量回归由 B 负责。
- Decisions / Risks / Remaining Required Outcomes：
  - F-02／F-03／F-04 已修正并有独立复现证据；AC-2／AC-4 的本地与只读部分可复审。
  - **F-01 仍 BLOCKED**：当前进程环境专用凭据（`RUSTFS_ACCESS_KEY`、`RUSTFS_SECRET_KEY`、
    `RUSTFS_ENDPOINT`、`RUSTFS_BUCKET`、`OPENLIST_TOKEN`、`RCLONE_REMOTE`）仍全部 absent。
    按 B 与指南未从历史文件／容器 Env 恢复秘密；远端仍 201，缺 9799，`manifest.jsonl` 最终
    固定未进行。AC-3／RO-2 远端部分未完成，请 B 决定凭据提供或交 A 处置范围。
  - 未运行项：远端 S3 补传／最终清单固定、4.5 生成／同步、4.6 扫描／完整决策、代理与播放。
    NOT RUN／BLOCKED 不作为 PASS。
  - 本次未运行产品全量 quality（B 负责）；未改产品行为，不触发 T4 门槛。
- Head SHA：实现提交 `e76a21c8e54a373fe53219b7a1aba5e86036f960`（修正 F-02..F-04、同步快照与事实文档）；本报告后继提交记录该 Implementation Head。


## 2026-10-20 用户授权补充：专用原生账户恢复

用户已明确同意：仅对本任务已核对的专用隔离 Jellyfin，尝试官方管理员账户恢复流程，
恢复访问后创建临时验收 API Key；不直接改库、不重建容器、不动媒体或生产实例，
官方流程不能完成即停止。该授权优先于本 Task 先前“不重置用户”的限制，但不授权
读取历史秘密、改插件、触发扫描／探测／播放或提前放行任何 RO。
只读取本次官方请求新生成且身份／路径／时间核对通过的恢复文件，PIN／密码／token
只在进程内存与原生请求中使用，证据仅记脱敏状态。临时 Key 在本次验证结束后撤销；
不将访问秘密写入仓库、日志或证据。官方恢复／临时认证验证已完成，详见下方 B4。
隔离 admin 密码已由官方流程更改为内存生成强随机值，未持久保存该值；
临时 Key 已撤销、恢复会话已退出，没有后续可复用认证留存。

## B Review Result

```text
Reviewed: 77fc768790a1c1ba6f0700c0aab294d3237cf7e4..12a9c4f273efbdb9856a85b9a335528c32585d57
Document Successor: 23748ea95fbf46748fcfd8935fcd0c7008394115
Decision: PASS
Slice Required Outcomes all satisfied: NO
Next: B FORMALLY PLANS TASK 4.5 FROM COMMITTED PASS CHECKPOINT
```

当前独立结论见 [B4 原生可用性补证复审](docs/task-4.4-b-review.md#原生可用性补证复审2026-10-20-b4)。
完整 Task Base..Implementation Head 审查、quality 全量、23 项直接测试及关键独立复现
由 B3 完成。本轮补审纯文档后继至 `45ff78b02fddf7030181f0bc4b0f9363061f2e07`，
再次匹配八份实际输入／tracked 快照／实现 Head 字节及固定万文件清单，复用已匹配门禁；
不机械重跑 quality／17 分钟完整分页，不把全量交回 Developer。

**F-06 已关闭，仅限原生任务可用性。** 经用户明确授权，B 对已核对专用隔离实例执行
官方恢复／临时 Key 验证：原生 `GET /ScheduledTasks` 200，共 21 项，存在
`STRM 媒体信息检查`／`StrmProbeOptimizerBatchCheck`，状态 Idle。实际固定 DLL 摘要未变。
只撤销本次 Key（204），再读 401，恢复会话退出 204。B 此处是获授权的环境验证，
未修改产品实现，不作为产品实施者自签自己的实现，也不代签 A 最终审查。

证据：`artifacts/validation-evidence/task-4.4-F06-recovery-20261020-iKiKlR/`（本机、Git 忽略）。
**隔离 admin 密码确已改变；新强随机值不留存，Key 已撤销，无后续可复用认证。**
后续原生操作需用户安全注入专用凭据或另获明确授权；不从此次证据恢复秘密、不重复恢复。
没有直接数据库访问、媒体／服务生命周期改变、扫描、任务执行、探测或播放。

AC-1..6 均通过本任务限定验收；RO-1／2 满足，RO-3..5 NOT RUN、RO-6 部分。
4.5／4.6 与 Slice Final 尚未执行，不出 Closure Packet，不返回 SLICE READY FOR A REVIEW，
不关闭 Slice。B 下一步从已提交 PASS 检查点正式规划 4.5，明确 Developer 受影响验证
与 B 自有完整范围、独立关键复现及指定全量执行／准确匹配复用门禁。
