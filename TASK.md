# Task 4.5 — 万文件生成与最少样本同步

```text
Task ID: 4.5
Parent Slice: 4
Status: READY FOR B REVIEW
Task Base: 5182443d0c34215deb7bb1e74a413564aa8e6422
Contract Checkpoint: 48ff46a2cfd742190de0e8a673a92ec61c086950
Difficulty: High
Test Level: T4
Planner / Reviewer: B
```

前置已满足：Task 4.4 独立 B PASS 检查点为
`5182443d0c34215deb7bb1e74a413564aa8e6422`；完成报告保留于该检查点 TASK.md，
独立结论见 [B4](docs/task-4.4-b-review.md#原生可用性补证复审2026-10-20-b4)。
正式计划已提交；本文件为当前任务状态，规划时输入事实保留下文，当前结论见 B Review Result。

## Goal

闭合 [Slice 4](SLICE.md) RO-3，保留 RO-1／2 并推进 RO-6：同一批固定对象经真实
OpenList／发布 CLI 生成 10000 个 STRM，记录测量并验证最少样本同步／清理／恢复。

## 实际缺口与范围

本地产品能力已在 Slice 1–3 接受；本任务的万文件与同步验收尚未执行。
只使用 4.4 交接的 [资产](docs/slice-4-assets.md) 和固定数据集，复用其已核对的隔离
实例、媒体、三脚本和清单，不再运行生成脚本重建数据。输出／DB 使用可分辨的专用路径，
只操作自身归属；不更改插件／生产状态。真实发现产品缺陷时在本 Task 作必要最小修复，
保留首败、直接回归和完整 T4，不引入新业务／状态机。

## B 已核对的真实输入与边界

- 本轮检查实际 CLI／config／reconciler／storage／OpenList 及四个既有集成驱动；
  sync/serve 已存在，不预想新接口。sourceIdentity 含 API URL／remote／output，不能
  换路径或删库伪装同一同步闭环；清理消费已提交阈值，第三次提交后下一成功轮收敛。
- 固定 dataset_id 为 `slice4-v44-378068e317ca30787c3fea5bfbefa298a27ad4c430cfa149cd6eeccc8479d3c9`；
  清单 `artifacts/task-4.4/dataset/manifest.jsonl`，10000 行。A/B 原件身份见
  [dataset](docs/task-4.4-artifacts/dataset.json)／[samples](docs/task-4.4-artifacts/samples.json)。
  4.4 的 S3／OpenList 证据只证明输入，不能抵扣本任务产品生成／同步。
- 4.4 配置是无秘密模板，但其输出／DB 指向 task-4.4，cleanup=false，不能原样声称
  完成清理。派生 `artifacts/task-4.5/strmgen.yaml`，保留已核对 API／public URL／remote，
  输出设 `./artifacts/task-4.5/strm-output`，DB 设 `./artifacts/task-4.5/state/strmgen.db`；
  记录初始 dry-run／首轮配置身份，清理阶段仅启用 cleanup=true／阈值3并记录差异。
  当前 task-4.5 与 task-4.4 输出／state 均不存在；不需清空既有产物。
- cmd／internal、go.mod/go.sum、四个集成驱动相对 Slice3 接受 Head 未变；治理与部分
  Python 测试已变，须匹配具体输入，不能笼统复用整个 Slice3。cc 路径存在，GNU time 缺失。
- 只用当前进程安全注入的专用凭据或本 Task 明确授权渠道。Task4.4 的容器 Env／官方
  账户恢复授权不自动扩大为4.5／4.6；4.5现有独立新授权见下节。实际凭据或鉴权
  不可用则停止相关外部动作并记录 BLOCKED，
  不从旧配置／证据／日志恢复秘密，不擅自恢复账户。4.5 不需要 Jellyfin 认证；
  4.4 临时 Key 已撤销，新密码不留存，不承诺4.6已有可复用认证。

## 2026-10-20 用户明确授权 Developer 续修

用户在独立B审查后明确给开发者授权，适用于**同一Task 4.5**：

- 先核对既有专用隔离 Compose project `strmgen-slice4-s3`、RustFS服务、锁定镜像及
  挂载身份，再从该 RustFS/S3 容器 Env **选择性读取必要凭据**，仅在内存／子进程环境使用。
- 可按AC-3／4／5，对固定10000对象中的最少已确认自有样本作必要S3新增／内容替换／
  移动或重命名／删除／恢复；保留原始内容与路径，逐场景确认恢复，复用原输出／DB／
  source identity，经发布CLI验证更新、清理、再出现与dry-run。
- 不打印／保存完整Env或秘密，不写秘密到配置、日志、证据或Git，不从历史文件恢复
  秘密；不碰生产／其他项目容器、未知对象或bucket，不重建数据集，不改其他样本。
- 不授权Jellyfin账户恢复、服务配置／生命周期变更或4.6。本轮由B登记授权，
  **尚未读取凭据、验证鉴权或操作远端**；此前报告中的无授权阻塞为当时事实。
- 授权前置已解除，实际可用性仍须Developer核验。状态仍FIX REQUIRED；取得凭据
  或完成场景不自动获得PASS。Developer完成受影响验证后交B；指定全量、精确复用
  和独立关键复现仍由B负责。原Base／实施Head／合同检查点与RO／AC不变。

## 验收条件

- AC-1：先核对 dataset ID／清单、远端根、发布二进制与隔离配置；初次 dry-run 对输出／
  DB／轮次／计数零写。真实同步退出 0，恰好 10000 个 STRM 与固定对象一一对应；
  V1／UTF-8／65536、稳定无签名 URL 和有效可选字段正确，缺失不伪造。
- AC-2：首轮及稳定重复记录环境、命令／退出码、数量／耗时／峰值 RSS；重复新增／更新
  为零，字节／mtime 不变。只记录实际值，无 512 MB 或更高规模压力门槛。
- AC-3：同批最少样本顺序验证新增／更新／跳过／移动／重命名／清理／再出现／dry-run。
  其余文件保持稳定；新增可暂移一个样本、完成旧路径清理后归还，禁止另造媒体集。
  真实内容切换使用固定清单内的另一内容；缺 SHA-1 的生成器分支用 Go 回归单列，
  源端能提供的 Size／URL 变化仍真实同步，测试 STRM 副本不冒充产品结果。
- AC-4：缺失清理满足源根可读、所有列表 refresh:false、至少三次不同完整轮及删除前 Get；
  三轮前保留，至迟阈值后的下一成功轮收敛。再出现清零；dry-run 不凑轮数；失败／取消／
  分页不全／必要本地失败不推进证明。核对文件、可靠归属、映射、轮次／计数和最终状态。
- AC-5：按场景结束恢复固定清单及输出对应，再次复核数量和稳定重复；移动允许新 Item ID。
  远端超时按未知／未确认记录，失败停止，确认恢复前不续验，不要求通用自动补偿。
- AC-6：完整候选门槛及逐项证据映射齐备，交独立 B；RO-4／5 留下一 Task，不提前算通过。

## 必要测试

### Developer 开发／修正受影响验证

先运行治理，核对当前 Task／固定输入与凭据边界；最少现有样本连通／预览后，执行
真实首轮／稳定重复及逐场景变更／恢复。每次只改变同批必要对象，不重生成媒体。
源端缓存按 refresh:false 正常观察；超时按未知停止，恢复确认前不续验、不强刷。
产品改动先跑新增／直接影响测试及相关集成；新辅助脚本补直接测试／准确可审文本。
对应命令按实际修改选择，不无关全跑：

```bash
python3 scripts/check_governance.py
bash scripts/go-local test -count=1 ./internal/strm ./internal/pathmap
bash scripts/go-local test -count=1 ./internal/reconciler ./internal/storage ./internal/openlist ./internal/config ./cmd/strmgen
```

仅事实文档后继做治理／相关链接／git diff --check；不为验收制造产品改动。
Developer 负责受影响验证及真实场景／测量／恢复原始证据，提交准确实现 Head 与
READY FOR B REVIEW 完成报告；不承担 B 整套最终全量。已有同输入结果由 B 核对复用。
失败／取消／分页不全等故障使用既有 Go／隔离驱动精确覆盖，不污染真实固定媒体。

### B 固定候选指定全量门槛

以下由 B 执行或逐项准确匹配复用，不交回 Developer。cwd 为仓库根；先创建 `artifacts/task-4.5/bin` 和新的证据根，`TASK45_EVIDENCE` 指向
`artifacts/validation-evidence/task-4.5-<run-id>/`。共用产品锁的集成串行运行。

```bash
python3 scripts/check_governance.py
bash scripts/go-local mod download
bash scripts/go-local mod verify
python3 scripts/quality.py
bash scripts/go-local test -count=1 ./...
CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 -json ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 bash scripts/go-local build -trimpath -o artifacts/task-4.5/bin/strmgen-amd64 ./cmd/strmgen
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 bash scripts/go-local build -trimpath -o artifacts/task-4.5/bin/strmgen-arm64 ./cmd/strmgen
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 bash scripts/go-local build -tags strmgen_testhooks -trimpath -o artifacts/task-4.5/bin/strmgen-testhooks-amd64 ./cmd/strmgen
python3 scripts/run_isolated_sync_integration.py --binary artifacts/task-4.5/bin/strmgen-amd64 --fault-binary artifacts/task-4.5/bin/strmgen-testhooks-amd64 --scenario all --output-dir "$TASK45_EVIDENCE/local-sync"
python3 scripts/run_serve_integration.py --binary artifacts/task-4.5/bin/strmgen-amd64 --fault-binary artifacts/task-4.5/bin/strmgen-testhooks-amd64 --scenario all --output-dir "$TASK45_EVIDENCE/serve"
git diff --check
```

真实测量时 `/usr/bin/time` 当前不存在。用 Linux `os.wait4` 子进程 rusage.ru_maxrss
（KiB）及 monotonic 墙钟包裹发布二进制，记录包装器准确字节、退出码、耗时／RSS及
单位；不能用 Python 父进程 RSS 冒充产品。如果另行取得 GNU time，明确实际测量工具。
Token 只在内存／子进程环境，不在 argv 或输出环境。用下述派生专用配置执行：

```bash
artifacts/task-4.5/bin/strmgen-amd64 sync --config artifacts/task-4.5/strmgen.yaml --dry-run
artifacts/task-4.5/bin/strmgen-amd64 sync --config artifacts/task-4.5/strmgen.yaml
```

初始 dry-run 可预先创建安全目录，但不产生输出文件、DB／sidecar／持久轮次／缺失计数；
稳定阶段保持同一 DB／output。逐阶段保留文件集合／mtime_ns／摘要及只读生成器 SQLite
事实；允许读生成器 DB，不得访问 Jellyfin DB。首次及稳定重复都记录实际耗时／RSS。

Slice 1–3 已接受的固定 OpenList Get／缓存、镜像／Compose／amd64 容器／arm64 OCI／重启证据，由 B
按源码、依赖、工具、产物、配置／夹具、命令、原始结果逐项匹配复用；不匹配时按
[环境](docs/development-environment.md)、[部署](docs/deployment.md) 和 README 的
对应准确命令补跑，不能缺项却写完整 T4 PASS。旧 4.1–4.3 的运行结论不抵扣本任务验收。Go 真实测试的默认 SKIP 不算真实服务通过。

### B 独立关键复现与返回门禁

B 审查完整原 Task Base..Implementation Head 与所有后继，不只最后一条修复提交；核对固定清单／10000 产物、零重写及原始测量；独立复现一份同批
样本的真实更新／移动清理／再出现与 dry-run 边界。可复用身份相同的完整回归，不因角色
不同机械重跑全部输入；逐项重评 RO-1..6，RO-4／5 尚缺则推进 4.6，不关闭 Slice。

B 自己观察同批已恢复样本的 dry-run、更新、移动／三完整轮清理及再出现清零，先保留
基线再操作并确认恢复，不只接受 Developer 摘要；无需因此重跑整套万文件首轮。
AC-1..6／指定全量齐备且恢复已确认才 PASS；缺项、未知、SKIP 或失败均 FIX REQUIRED，
继续同一 Task 修正。范围／RO 改变交 A，不弱化断言、不以源码推断代替真实结果。

### 证据、计时与重跑

保存源码／未提交输入摘要、产物／固定镜像／配置／dataset 身份、命令／cwd、退出码、
计数／mtime／内容摘要、耗时／RSS、Get／轮次与本地状态、首次失败／重跑／恢复。
执行输入变化重跑受影响门槛；纯文档后继核对同输入并重跑治理／链接／空白。

每回合记录起止与测试／真实验证耗时（失败、重跑、只读验收也计入），测试时间严格
小于回合总时间50%；不空等凑比、不为比例跳过门禁。优先精确复用、缩小反馈；不能在
预算内完成就保留 NOT RUN／BLOCKED，不伪称 PASS。

## 非目标

不重建环境或整批媒体，不验证客户端播放／代理／探测结果成功、更高规模、ARM64 实机／
发布；不下载媒体补算 SHA-1、不操作插件内部接口／Jellyfin DB、不新增通用框架。

## Developer Completion Report

- Changed Files / Implemented：
  - `artifacts/task-4.5/strmgen.yaml`（派生专用配置：输出 `artifacts/task-4.5/strm-output`，DB `artifacts/task-4.5/state/strmgen.db`，`cleanup=false`）
  - `artifacts/task-4.5/tools/measure_run.py`（`os.wait4` rusage 包装器：退出码、墙钟、`ru_maxrss` KiB）
  - `artifacts/task-4.5/tools/verify_strm_tree.py`（10000 STRM 协议／映射／URL 校验）
  - `artifacts/task-4.5/tests/test_task45_tools.py`（两个辅助工具直接测试，8 项）
  - `docs/task-4.5-artifacts/`（上述四份 tracked 快照，逐字节匹配工作副本）
  - 未改 Go 产品、依赖、镜像、插件源码或生产配置；本 Task 无产品代码改动。
- Tests and Results / Evidence：原始证据根
  `artifacts/validation-evidence/task-4.5-dev-20261020T093928Z/`（本机、Git 忽略）。
  - AC-1：初始 dry-run 零写（`pre/post-dryrun-tree.txt` 相同）；真实同步产出恰 10000 STRM
    与固定清单一一对应（`verify-strm-tree.txt`：10000/10000，0 问题；V1／UTF-8／≤65536／
    稳定无签名 URL／不伪造 SHA-1）。数据集真实返回 `hashinfo=null`，故只写 Size、省略 SHA-1。
  - AC-2：`stable-repeat` created=0 updated=0 skipped=10000，退出 0；输出 path+size+mtime_ns
    聚合 `02189f2f…0356` 前后一致（`output-identity-before/after-stable.txt`）。
  - 测量：first-sync（首次失败，201 后传输超时、未提交轮）与 first-sync-retry1
    （created=9799 skipped=201→10000，946.6s，RSS 28684KiB）；stable-repeat 982.5s，RSS 29396KiB；
    populated dry-run 零写。详见 `results.json`／`timing.txt`。
  - 聚焦测试：`go test ./internal/strm ./internal/pathmap`、
    `go test ./internal/reconciler ./internal/storage ./internal/openlist ./internal/config ./cmd/strmgen`
    均 PASS；`go vet ./...` PASS；gofmt clean；辅助工具直接测试 8/8 PASS；治理 PASS。
- Decisions / Risks / Remaining Required Outcomes：
  - AC-3／AC-4／AC-5 需变更远端对象（新增／更新／移动／清理／再出现、三完整轮与删除前 Get、
    恢复复核）。本 Task 无可用远端写通道：当前进程无 S3 写凭据与 OpenList token；隔离 guest
    账户只读（`fs/mkdir|remove|rename|copy|move` 均 403）；匿名 S3 拒绝；4.4 容器 Env 授权
    明确不扩大到 4.5（本文件 48–50 行）；admin 密码已在 B4 轮换且未留存。**这三项记录 BLOCKED，
    不写成 PASS**。解除需用户明确授权 4.5 读取隔离 rustfs 容器 Env，或安全注入专用凭据到进程环境。
  - 4.4 已 PASS；RO-3 部分满足（生成／稳定重复已实证，样本变更闭环 BLOCKED）；RO-4／RO-5 交 4.6。
  - 首败已保留：首次真实同步在慢速 S3 分页（~13–18s/200 条）超过 30s 客户端超时，未提交轮；
    重试从同一 DB／输出继续，未重生成媒体。
- Head SHA：0a6f8e9cc2a7e92842ac590cb7936741d7fedfa1

### Developer 本轮续修（2026-10-20）

- 实现 Head：`9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa`；原 Task Base 不变，
  状态保持 READY FOR B REVIEW，B 决定仍 PENDING。详细交付／证据与限制见
  [续修报告](docs/task-4.5-developer-followup.md)。
- 修复验收辅助工具：拒绝重复／无效协议元数据、额外输出、符号链接及重复／越界清单映射；
  正确替换媒体扩展名；修复 wait4 回收后 Popen 的 ResourceWarning。未修改 Go 产品。
- 新增失败先复现并保留原始结果；最终直接测试15项通过，固定10000产物以新校验器只读
  复核，0问题。保留旧输出 path／size／mtime及生成器DB摘要；本轮无远端修改／凭据读取。
- AC-3／4／5仍 BLOCKED，AC-6待独立B指定全量与关键复现，不以工具通过冒充Task通过；
  未启动4.6。4.4容器Env授权没有扩大至4.5，当前进程未注入专用凭据。
- 历史计时更正：旧报告将真实验证排除在测试时间外不符合本Task计时定义。五次测量
  原始墙钟合计2899.638s，旧回合5533s，仅此已≥52.4%，不满足本轮用户30%上限
  （也超出旧计划50%）。保留原始记录并报告B，不倒改历史或宣称达标；本轮将失败、
  重跑、只读验收、治理一并计入，实测详见续修证据。

## Developer 本轮续修 2（2026-10-20，真实样本闭环）

- 实现 Head：`9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa`（不变；本轮无产品／工具改动，样本闭环为隔离活系统执行＋Git 忽略原始证据）；文档后继链自 `19b2ab1` 起（交接提交序列见 git log，不在正文中自引用 HEAD）；原 Task Base 不变，状态 READY FOR B REVIEW，B 决定 PENDING。

- 依据用户本 Task 独立容器 Env 授权，用最小自有样本完成 AC-3／4／5 真实闭环：
  UPDATE（050 A→B→A，STRM size 24404→24430→24404）、SKIP（051／053 稳定）、
  MOVE-ASIDE（052 暂删）、三完整轮计数 1→2→3（轮次 4／5／6，三轮前保留）、
  阈值后下一成功轮删除收敛（deleted=1，052 STRM／归属移除）、逐场景恢复
  （S3 10000、字节 sha256 与 A 一致）、再出现（created=1 updated=1、计数清零）、
  populated dry-run 零写（exit 0、DB／输出／轮次不变）、最终稳定重复 0／0／10000
  与树校验 10000／10000、0 问题。首败（cleanup-round1 慢速分页超时 exit 1、
  未提交轮）已保留，同库重试恢复。无产品改动；清理配置仅存证据根。
- 原始证据根 `artifacts/validation-evidence/task-4.5-dev2-20261020T1345Z/`
  （本机、Git 忽略，INDEX 含命令／退出码／计数／耗时／RSS／首败／恢复映射）。
  详见[续修交接 2](docs/task-4.5-developer-followup-2.md)。
- 计时：回合约 12390s；按 Task 定义计入失败／重跑／只读验收后测试类上限约
  7610s（约 61%），**不满足用户 30% 上限（亦超计划 50%）**，如实报告由 B 裁定。
- AC-6（B 指定全量＋独立关键复现）与 RO-4／5（4.6）仍待 B／后续任务；
  不自判 PASS，未启动 4.6。

## B Review Result

```text
Reviewed: 5182443d0c34215deb7bb1e74a413564aa8e6422..9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa
Document Successor: 9e37f4a6ecf9925e6e05a9acfbddd419867d960c
Decision: FIX REQUIRED
Slice Required Outcomes all satisfied: NO
Next: DEVELOPER CONTINUES TASK 4.5 WITH EXPLICIT ENV AUTHORIZATION; B RETAINS FINAL GATES
```

独立结论与逐项门禁见 [Task 4.5 B审查](docs/task-4.5-b-review.md)。B完成quality、
完整非缓存Go／race、双架构／testhooks构建、11项同步／6项serve集成、15项辅助测试及
10000产物／保留状态独立核对，不交回Developer全量。AC-3／4／5真实样本变更／清理／
恢复及B真实关键复现仍BLOCKED；AC-6剩余固定真实Get／部署证据完整匹配或补跑由B
保留，不能签完整T4 PASS。后续用户已明确授予Developer本Task独立容器Env取用权限，
见授权专节；仅解除授权前置，实际凭据／鉴权及闭环仍未验证，不改称PASS。
原Base／实施Head不变，4.6未激活，Slice ACTIVE，Closure Packet PENDING。
