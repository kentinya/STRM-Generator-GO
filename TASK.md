# Task 4.4 — 复用隔离资产并固定万文件数据集

```text
Task ID: 4.4
Parent Slice: 4
Status: READY FOR B REVIEW
Task Base: 77fc768790a1c1ba6f0700c0aab294d3237cf7e4
Contract Checkpoint: 77fc768790a1c1ba6f0700c0aab294d3237cf7e4
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
目录／旧证据、从历史文件／容器 Env 恢复秘密、接管生产或重新选择镜像／插件 HEAD。

## 验收条件

- AC-1／RO-1：记录隔离实例／固定镜像 digest、专用持久目录／端口、插件源码／包／加载
  身份、原件与三脚本摘要。沿用合同已固定版本，核对当前专用访问凭据可用；
  凭据只由进程环境注入，不从历史文件或容器 Env 恢复秘密。
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
该指南细化现有 AC，不改变合同；其中脚本参数、测试文件和清单格式尚待开发。

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
本轮实现 Head: 01104ed2c879ae36b28ac8cacea301296272c92f
证据索引: artifacts/validation-evidence/task-4.4-IKlUvR/INDEX.md（本机、Git 忽略）
可审查文本快照: docs/task-4.4-artifacts/（脚本原版／适配版／测试／工具／配置准确字节）
```

- Changed Files / Implemented：
  - `artifacts/task-4.1/isolated-s3/generate_media.py`（Git 忽略）最小适配：保留原路径，拆分
    可测试函数与 `main`，`__main__` 守卫；新增 `--source/--alternate-source/--media-dir/
    --count/--manifest/--prepare/--verify-only`；补缺失、冲突拒绝、不覆盖、临时文件后
    link 发布、稳定 JSONL 候选清单、重复零写与 `--verify-only`。原版另存 `generate_media.py.orig`。
    准确字节见 `docs/task-4.4-artifacts/generate_media.py`（适配）与 `...orig`（原版）。
  - `artifacts/task-4.4/tests/test_generate_media.py`：11 项直接行为测试（临时夹具在
    `artifacts/task-4.4/tests/tmp` 下，非真实媒体）。
  - `artifacts/task-4.4/tools/openlist_readonly_probe.py`：只读 OpenList 分页／样本／稳定字节核对。
  - `artifacts/task-4.4/dataset/{manifest.candidate.jsonl,dataset.candidate.json,samples.json}`：
    10000 文件候选清单与样本（Git 忽略，摘要见证据索引）。
  - `artifacts/task-4.4/strmgen.yaml`：4.5 专用 CLI 配置（仅写 token 变量名，输出／DB 指向新路径）。
  - 未改 Go 产品、插件源码、生产配置、Jellyfin DB、bootstrap.sh、teardown.sh。
- Tests and Results / Evidence：
  - 治理 `python3 scripts/check_governance.py` PASS；`bash -n` 两脚本、`ast.parse` 适配脚本、
    `git diff --check` 均退出 0。
  - 直接工具测试 11/11 PASS（0.168 s）。
  - 真实本地建集：preserved 201 + created 9799 = 10000（42.7 s）；对候选清单 `--verify-only`
    10000 PASS（1.6 s）；201 原件字节与 mtime 均未变。
  - 真实 OpenList 只读：源根为目录、refresh:false 完整分页 total=201、7 个含特殊名样本精确
    Get 与稳定 `/d/` 字节均与本地内容 SHA-256 匹配。Jellyfin 入口可访问、插件加载身份一致。
  - 本轮测试类进程合计约 4.7 s，占本轮约 1440 s 的约 0.3%，低于 30%。产品全量回归由 B 负责。
- Decisions / Risks / Remaining Required Outcomes：
  - AC-1／AC-2／AC-4 只读部分已完成并有真实证据；AC-3 远端补传 **BLOCKED**。
  - 阻塞原因：当前进程环境缺少专用凭据（`RUSTFS_ACCESS_KEY`、`RUSTFS_SECRET_KEY`、
    `RUSTFS_ENDPOINT`、`RUSTFS_BUCKET`、`OPENLIST_TOKEN`、`OPENLIST_ADMIN_PASSWORD`、
    `OPENLIST_JWT_SECRET`、`RCLONE_REMOTE` 全部 absent；`RCLONE_CONFIG_*`=0）。按指南未从历史
    文件／容器 Env 恢复秘密。远端仍为 201 对象，缺 9799，`manifest.jsonl` 最终固定未进行。
  - 未运行项：远端 S3 补传与最终清单固定、4.5 万文件生成／同步、4.6 扫描／完整决策、
    代理与播放。NOT RUN／BLOCKED 不作为 PASS。
  - RO-1／RO-2 中本地部分已交付，RO-2 远端固定与 RO-3..5 待凭据恢复后由后续任务完成；
    请 B 决定凭据提供或范围处置，不代签 PASS。
- Head SHA：见本轮实现提交；后继文档提交记录 Implementation Head。

## B Review Result

```text
Reviewed: NOT SET
Decision: PENDING
Slice Required Outcomes all satisfied: NO
Next: RESOLVE CREDENTIAL BLOCKER FOR AC-3, THEN INDEPENDENT B REVIEW
```
