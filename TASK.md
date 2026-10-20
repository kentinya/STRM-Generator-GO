# Task 4.2 — V4.4 测试工具治理与验收交接

```text
Task ID: 4.2
Parent Slice: 4
Status: READY FOR B REVIEW
Task Base: 66e93bf5b09197be103c04605a7adff06000fdab
Contract Checkpoint: 66e93bf5b09197be103c04605a7adff06000fdab
Difficulty: Medium
Test Level: T2
Planner / Reviewer: B
```

## Goal

落实 Slice 4 RO-6：只保留 V4.4 当前验证及 Slice 1–3 所需工具，保存有效历史证据，
同步当前文档并给出下一 Task 的具体计划。不把工具治理计为真实服务／产品验收。

## 实际缺口与范围

已读全部 scripts、tests 清单与调用关系、环境规范、质量入口、治理测试、16 个 Go 测试
文件职责及旧 Task 4.1 原件结构／摘要。当前旧目标运行器 1256 行、专属测试 898 行／58 项，
绑定播放入口和旧暂缓表，与新合同不一致；其他四个集成驱动分别覆盖 Slice 1–3，继续保留。
现有 check_governance／doctor 的 V4.3 文案过时，版本字段尚未与方案标题一致校验。
治理前 Python 166 项；旧文档 139 项为历史计数，不能沿用为当前实测。

允许删除旧目标运行器及专属测试，调整环境 required_files、治理身份验证及其直接回归；
同步文档／复用账本、规划下一真实验收 Task。产品 Go、现有四个驱动、故障 hook、历史
artifacts 和已通过 Slice 的范围均不改。不新建通用集成框架，不重跑旧播放或修改外部服务。

## 验收条件

- AC-1：两个退出入口不再存在或被质量入口／规范依赖；全部保留工具及 Go 测试清单可核对。
- AC-2：方案 SHA-256、V4.4 版本／标题、引用／合同／路线／任务一致，拒绝未协调漂移。
- AC-3：历史有效原件、失败／未运行及开放事件保持；复用账本列实际内容摘要与当前 RO 限制。
- AC-4：质量聚合、doctor、保留驱动 --help、补丁检查通过；不把 Python／Go 本地结果算真实服务 PASS。
- AC-5：文档说明治理结果；下一 Task 写明固定万文件、少量同步、原始扫描、全部决策行、输入身份、
  分层证据与恢复要求。4.2 未独立 B PASS 前只保留候选计划，不激活真实验收。

## 必要测试

### 开发／修正反馈

先保存现有状态和原件摘要；治理前运行 `python3 -m unittest discover -s tests -v`，
原始输出保留不覆盖。新增版本漂移／协调升级回归由最终 quality 覆盖，不重复运行同一集。

### 最终检查点门槛

- `python3 scripts/check_governance.py`
- `python3 scripts/quality.py`（含全部 Python、AST／Bash／空白、gofmt、Go vet／test／build）
- `python3 scripts/development_environment.py doctor`
- 四个保留驱动逐一 `python3 scripts/<driver>.py --help`
- `git diff --check`，核对产品源码／16 个 Go 测试／四个驱动和原件字节未变。

本 Task T2 仅改工具与文档，产品／外部输入不变；不新增 race、镜像重建或真实服务运行门槛。
下一 T4 真实验收 Task 单列所需证据，不豁免该 Task 的完整候选门槛。

### B 独立验证

审查完整 Task Base..实现 Head，核对删除理由与当前矩阵、保留安全覆盖和原件摘要，
独立运行治理／关键版本漂移回归及保留入口帮助；可复用输入相同的 quality 结果。
核对下一计划无播放／代理／旧暂缓或更高规模门槛，不把自身实施报告当独立 B PASS。

### 证据与重跑

证据目录：`artifacts/validation-evidence/task-4.2-governance-20261020/`。
记录初始 HEAD／工作区摘要、初次治理失败（合同尚未提交）、合同提交后 PASS、基线／最终
命令、版本、退出码／计数／耗时、候选 SHA 与工作区可执行输入摘要、保留文件／原件摘要。
最终修改工具后重跑受影响门槛；纯文档后继只重跑消费文档的治理／链接及空白检查。

## 非目标

不执行下一真实验收，不补签 Task 4.1 PASS，不代独立 B／A 签发结论，不读取或处置历史
凭据，不操作生产／配套仓库；不修改生成器产品、不构建新运行器／媒体服务／自动补偿。

## Developer Completion Report

- Changed Files / Implemented：删除旧目标运行器及 58 项专属工具测试；保留四个 Slice 1–3
  驱动与全部 Go 回归。更新 required_files，治理校验方案版本／标题／摘要，doctor 使用当前
  版本标识，补 3 项版本漂移／缺失／协调升级回归。同步工具清单与当前状态，下一真实验收
  候选为 [Task 4.3](Task/4.3-validation-plan.md)，未激活。
- Tests and Results：治理前 Python 166/166；最终 quality 中 Python 111/111、AST／Bash／
  空白／gofmt／vet／Go test／build 均 PASS（Go 九包为缓存结果）。doctor、四个保留驱动
  --help 均退出 0。质量聚合用时 7.496 秒，治理前测试 2.984 秒。
  58 份保留可执行输入字节相同，V4.4 原文摘要相同，六份历史 JSON 摘要仍匹配。
- Evidence：`artifacts/validation-evidence/task-4.2-governance-20261020/INDEX.md`，含完整
  命令／cwd、原始日志、退出码／计数／耗时、可执行输入逐文件摘要、历史原件复核。
  首次治理失败因未提交新合同，合同提交后通过；初始工作区状态／摘要完整保存。
- Decisions / Risks / Remaining Required Outcomes：AC-1..5 实施完成，独立 B 仍 PENDING；
  RO-6 的工具部分已完成，RO-1..5 的新固定数据／实际决策和 B／A 审查尚缺。未运行真实
  服务、万文件、race／新 T4 或旧播放；无产品源码／四个驱动改变。历史凭据事件仍开放，
  不宣称轮换／删除或风险接受。旧 Task 4.1 仍为历史 FIX REQUIRED。
- Head SHA：8109811b46575576f1b87530d65aea298bf2d576

下一步：独立 B 审查完整 Task Base..上述 Head 及后继文档；通过后才将 4.3 候选填入真实
Task Base 并提交为根活动计划。当前不执行真实验收，不因本轮实施完成签发 B／A PASS。

## B Review Result

```text
Reviewed: NOT SET
Decision: PENDING
Slice Required Outcomes all satisfied: NO
Next: INDEPENDENT B REVIEW OF TASK 4.2; THEN ACTIVATE TASK 4.3
```
