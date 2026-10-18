# Task 2.2 — 显式启用 Mirror 清理、移动收敛与正常失败恢复

遵循[开发流程](docs/development-workflow.md)，范围 authority 为已提交的 [Slice 2](SLICE.md)。
Task 2.1 已独立 PASS，完整计划、报告及 B 审查保留于
`c629858ba7559a03c1eba30f8cd91a269f4b2702:TASK.md`。本 Task 复用其两次 Get 判定和普通列表，
完成当前 Slice 尚缺的可操作闭环；不能将本文计划当作现有实现或验收结论。

```text
Task ID: 2.2
Parent Slice: 2
Status: READY FOR B REVIEW
Task Base: c629858ba7559a03c1eba30f8cd91a269f4b2702
Contract Checkpoint: 57ea31b88a0dde95e486f8bac11ebe9e473f74ea
Difficulty: High
Test Level: T4
Planner / Reviewer: B
```

## Goal

管理员显式启用清理后，正常 sync 为已拥有的精确远端路径累计不同完整轮的连续缺失证据，
达到配置阈值后按当前源根／目标 Get 与本地归属边界清理；移动的新路径先生成，旧路径随后退出。
预览不改变原文件／原数据库／计数／轮次，正常删除、目录同步、DB 或进程中断后可重试收敛。
本 Task 覆盖 RO-1、RO-2、RO-3 本地部分、RO-4、RO-5、RO-6 预览与接入实证。

## 实际缺口与范围

规划前工作区干净，`python3 scripts/check_governance.py` PASS；代码基线为上述真实完整 SHA。

- `internal/config/config.go` 已解析 mirror／cleanup 字段，但未传入 Resolved，仍直接拒绝
  cleanup.enabled:true，阈值尚未校验；`cmd/strmgen/main.go` 已有 sync／dry-run、全轮互斥、
  source-v1 身份和摘要。复用入口与身份，不能因启用清理改变 source key 或建立另一把锁。
- `internal/storage/storage.go` 的 ownership 只有 source_key、relative_path、absolute_path、
  created_round、missing_count；Lookup／RecordOwnership／NextEpoch／CommitRound 已存在。
  无精确远端路径、观察集合、分页候选与计数事务；CommitRound 当前仅 upsert epoch。
- `internal/reconciler/syncer.go` 有 source 隔离、源根门槛、有界 walker、目录同目标冲突、
  创建／更新／skip 及失败不提交轮次。当前仅创建后登记归属，已有文件更新／skip 不更新映射，
  也没有观察集合或候选枚举。
- `openlist.Confirmer` 与 `CheckSourceRoot` 已交付；所有 List 每页 false。不得另写错误文本
  算法、祖先刷新链、500 例外或缓存证明。确认器是无持久状态的读取能力，不承担计数事务。
- `internal/reconciler/fs.go` 已有绑定输出根、descriptor-relative 操作、LocalFS／Recover、
  有界暂存枚举与故障接缝；FileSystem 暂无受保护的删除操作。`securepath` 有相对 Remove／Sync。
  沿用这些能力，保留正常更新恢复，不直接用拼接绝对路径的 os.Remove 绕过边界。
- `scripts/run_isolated_sync_integration.py` 已覆盖 9 类 CLI／恢复；真实 OpenList 运行器已有
  固定 v4.2.7 Local／WebDav 8 场景及工具回归，可增补真实 CLI Mirror 验证，无须新建另一套服务。

允许修改上述产品接入及相关 Go／Python 测试、测试标签故障钩子、配置示例、README、架构 CURRENT、
测试矩阵、兼容／环境事实说明和文件登记。原则上不改依赖。SLICE、Roadmap、稳定需求及原方案不变；
发现无法满足合同的阻断交 A，不自行缩减验收。只操作 artifacts／专用隔离夹具，不接管生产服务。

## Developer 分步实施清单

### 1. 先接通配置，保留所有写入前校验

1. 在 Resolved 与 Reconciler Options 中传递实际使用的 mirror、cleanup enabled、阈值。
   默认 mirror=true、cleanup=false、missing_confirmations=3。显式阈值只接受整数 ≥3，
   包括清理关闭时的显式非法值也须拒绝；更大值只延后清理。
2. mirror=false＋cleanup=true 为配置错误，退出 2；在获取会写入的输出／DB 之前拒绝。
   mirror=false＋cleanup=false 继续正常生成／更新，不新增缺失或删除；清理关闭亦如此。
3. 保留严格 YAML、路径／输出重叠、DB sidecar、token、安全诊断、互斥与其他配置保护；
   新错误只反映字段名／source 序号，不能回显秘密或完整私有路径。不加 force／单轮删除选项。

### 2. 补齐可靠映射与旧库兼容，再写缺失逻辑

1. 写模式在事务中兼容升级 Slice 1 表，保留归属和轮次；旧记录远端映射保持未知，旧 epoch／
   created_round 不能推导缺失次数。对旧库已有的未被本版本证明的计数不得追认为资格。
   升级须幂等，失败不留下半升级可写库；未知／损坏 schema 清楚失败，不能重建或认领输出。
2. 精确远端路径来自本轮已校验 dir＋entry.Name，不能逆推 .strm 扩展名、解析 URL 猜路径或
   下载媒体。保持 source-v1 身份不变；token／public URL 变化不丢归属，移除或更换 source
   不跨身份清扫历史记录。
3. 新建成功登记映射；已有归属在创建恢复、更新成功或比较 skip 后也能补齐／替换精确映射。
   仍核对相对路径、绝对路径、配置输出和归属是否一致；不合法的行保留并报告，不能授权删除。
4. 旧库映射未知且远端已缺失的行保留并说明“缺少精确映射”，不靠文件名猜测。正常再观察到后
   可补齐，再从新的缺失轮起计数。真正未归属文件一直不自动认领、覆盖或删除。
5. 为观察、缺失和候选提供有索引、固定批量的枚举，建议每批不超过 200 行。不要全库 Scan 到
   Go slice／map，不为每文件开 goroutine。SQLite 限单连接时关闭当前 rows 后再做写操作，
   避免边持有游标边等待同一连接；用 keyset／稳定键，删除过程中不以 OFFSET 导致跳行。

### 3. 在现有 source 对账内记录出现和缺失

1. 保持源根 Get 为目录后才遍历。根不可读、配置中已移除的 source 不推进其计数／轮次或清理。
   不因 provider:unknown 或虚拟视图额外否决成功目录，全部 List 仍显式 false。
2. 以本轮实际观察到的媒体路径／输出目标记录出现，skip 也是出现。正常写模式一旦确认属于
   该行的源文件再出现，立即保守清零旧缺失，即使之后其他文件失败；不能先等整轮成功才清零。
   出现不授权覆盖未归属文件；精确映射更新仍等必要本地处理成功。
3. 已观察到的目标不再调用 Confirmer 增加缺失；仅分批处理本源未观察到且映射可靠的已有归属。
   复用 Confirmer：Present（含目录类型冲突）保留并清零；Missing 只暂记本轮缺失观察；
   Unknown／调用方取消不成为一次缺失。根不可读时停止该源新增依据与清理，旧计数保留，
   已确认出现的保守清零可以保留。类型冲突有明确保留诊断。
4. 先完整遍历并完成所有必要本地生成／更新，再评估缺失／清理。列表失败／分页不全、同目标
   冲突、必要本地操作／DB 失败、整轮取消，都不提交本轮新增缺失或启动该源清理。
   目标 Get 未读到本身按合同不是失败轮；不能重新引入目标权限／HTTP 错误保留特例。
5. 轮次与新增 missing_count 在同一 SQLite 事务中提交，每源每个不同完整 epoch 对每行最多
   +1。重复提交同一 epoch 不重复计数，也不能用旧 epoch 覆盖新轮次。计数溢出不能变成
   负数或绕过阈值。失败／重启遗留的临时观察不能被下一轮当作已观察或已缺失。
6. 观察和暂记集合也须有界；可复用最小对账字段或隔离临时工作存储，但不能持久化媒体指纹、
   新建发现／媒体版本状态机。若复用未提交 epoch 作标记，须防止失败后重用该 epoch 时残留
   标记误判出现。dry-run 的临时工作不写回原库，不迁移旧库，结束后不留下可供下轮消费的依据。
7. 清理关闭时不增加缺失次数／不删除，已确认出现仍清零，其余旧数保留。重新启用须先完成
   当前遍历，再按当前相同 Get 重核；不能只读历史计数就删除。不同 source 不共用观察或证据。

### 4. 用已提交证据清理，最后才提交本轮新增计数

1. 采用便于验证的顺序：保存本轮开始时可消费的已提交资格 → 完整遍历和缺失判定 → 对符合
   旧资格且本轮未观察到、计数未被清零的行当前重核并清理 → 最后事务提交成功轮和本轮新增计数。默认阈值下第三次完整缺失先形成
   资格，稳定时第四次成功 sync 完成清理，符合合同“至迟下一次”。不得用尚未提交的本轮把 2 凑成 3。
2. 每次删除前重核：本源已完成必要处理、cleanup／mirror 开启、持久计数达到阈值、可靠归属与
   精确映射仍有效，再调用同一 Confirmer 读当前源根和精确目标。Present 保留／清零；
   根不可读或取消保留且不推进本源新增计数。Missing 后才允许进入本地删除检查。
3. 本地检查必须经绑定输出根／相对操作：映射和配置边界一致、目标是 regular .strm、
   无符号链接／特殊文件／目录冲突、没有未解决的更新暂存。复用有界枚举查暂存；无远端 desired
   时不能拿任意空内容调用 Recover 并删残留。不要求哈希永久匹配来冻结可再生成的已拥有内容。
4. 删除流程为安全本地检查 → unlink → 父目录 sync → 删除对应 ownership；各步错误传播。
   增补 FileSystem 的删除／检查能力并复用 LocalFS 故障接缝，不能给调用方一个绕过归属的广域
   递归清理入口。旧空目录可保留，未知暂存和未归属文件不扫。
5. 在 unlink 后目录 sync／DB 失败或进程中断时保留可恢复事实；停止本源后续删除，最终不提交
   失败轮的新增计数。已发生的删除如实报告，不能宣称跨文件回滚。归属回收与已删除行从之后
   计数更新集合移除，不能最终提交时把已删除 ownership 重新 upsert 回来。
6. 重启遇到本地目标已缺失但归属还在，仍从当前完整对账、已提交资格和远端重核出发，完成
   必要目录 sync／记录回收；不能仅凭孤儿行删除后来出现的未知对象。远端若重现按现有生成
   规则重新生成／清零，正常已归属残留在稳定后收敛，不要求手工改数／删库／盲删文件。
7. 移动／重命名的新路径必要处理失败不得清理旧路径。A.mkv→A.mp4 映射同一个 A.strm
   时先按既有 URL 规则更新该目标及精确映射、清零旧证据，不能把它作为旧路径候选删除。
   同轮 A.mkv 与 A.mp4 冲突仍失败，不任意选择。扫描期间远端变化由后续轮纠正。

### 5. 预览和摘要必须与真实操作区分

1. dry-run 沿用只读打开／已有 WAL 私有快照：不创建缺失 DB，不修改原 DB／sidecar、迁移、
   文件／mtime、计数或轮次；不调用真实删除。旧库可读取归属，未知映射保留／提示。
2. 预览在当前完整扫描、当前 Get 和本地保护均满足时，仅以**已有已提交阈值**列清理候选。
   旧数 2 即使本轮会成为第 3 次也不是可删除候选；预览不能为下一轮生成证明。
3. 摘要区分缺失观察、保留、预览候选和实际删除；真实 deleted 永远只反映已执行 unlink，
   unlink 后 sync／DB 失败应同时显示已发生删除和该源未完成。失败／忙碌／冲突仍非零。
   诊断样本沿用每源上限，统计不靠保存所有文件列表；全库规模目标不等于已实测性能。

### 6. 补证据，保持已接受行为

1. 先写确定性存储／对账／本地失败用例，再接 CLI 场景；断言实际文件和 SQL 数据，不能只看摘要。
   复用现有测试钩子模式：仅 strmgen_testhooks 构建可控暂停／失败，发布构建没有环境开关。
2. 扩展既有隔离 CLI 驱动的 all：新增 mirror_cleanup、mirror_recovery 两个行为场景；同步
   SCENARIOS／ALL_SCENARIOS／工具回归。保留旧 9 类，不删除已有失败断言；把旧“true 不支持”
   的特定断言显式替换为本 Task 的有效配置与非法组合验收，不继续要求所有 true 都拒绝。
3. 扩展真实 OpenList 运行器的**计划新增** `--sync-binary` 参数：给出时在独立 Harness 内执行
   发布 CLI 的 Mirror 场景，在 summary 单列结果；缺参数不把 CLI 层算 PASS。保留原 8 个 Get
   场景及缺环境 BLOCKED／禁止 skip／只停止自有资源的行为。更新参数、失败传播和脱敏测试。
4. 真实 CLI 至少生成→删除／移动→三次完整缺失保留→下一次当前重核清理；另测源根不可读冻结、
   目标再出现清零、dry-run 零写。全部针对固定隔离 Local 文件、DB／输出在该证据目录；
   8 个原场景继续覆盖 WebDav 自然缓存与已接受限制。缓存滞后不得以强刷／清缓存让断言通过。
5. 增补旧库／新库、正常失败／重启和有界枚举的原始证据；同步 README／CURRENT／测试矩阵，
   新文件登记 environment-spec。只修正已实现事实，不改稳定需求或宣称播放通过。

## 验收条件

| ID | 可核对结果 |
|---|---|
| AC-1 配置与默认 | 默认不清理；mirror 默认 true；阈值默认／最低 3，更大阈值延后；true＋false 矛盾、低阈值／非整数等在输出／DB变更前退出2。关闭清理仍生成／更新／skip。 |
| AC-2 完整轮计数 | 默认阈值下三个不同完整轮前保留，第3次只提交资格，稳定第4次清理；同 epoch 重入不重复加；列表／分页／本地／DB失败、取消和预览不凑数；目标未读到按同一规则计数，源根不可读冻结。 |
| AC-3 出现、开关与隔离 | skip／Get可读清零，类型冲突同时保留并报告；再出现后即使后续失败旧证明也失效；关闭保留旧数且不新增，再启用先当前对账；多source、源移除／身份改变不借用或转移证据。 |
| AC-4 安全清理与移动 | 删除逐项满足已提交资格、当前根／目标Get、精确映射、归属、regular STRM和边界；未归属／符号链接／特殊文件／未知暂存／失配记录保留；移动新路径失败不删旧路径，同目标扩展变化更新不自删，冲突失败。 |
| AC-5 旧库与有界处理 | 升级事务／幂等且保留旧归属，不追认历史轮次／计数；观察可补齐映射，未知历史缺失保留；干净失败处理损坏／丢失库；候选分页不漏／不重复，观察与查询不一次载入全库或逐文件并发。 |
| AC-6 正常失败恢复 | unlink、目录sync、ownership回收、round事务及取消／SIGKILL前后均确定性覆盖；多文件已删部分如实报告并停止后续，不新增失败轮计数；发布二进制重启重新核实后收敛，无需管理员伪造状态。 |
| AC-7 预览及可观测性 | 新库／旧库／WAL库／无库 dry-run 原状态零写、不迁移；已有阈值候选与未来缺失资格分开；输出区分保留／缺失／候选／实际删除，诊断有界脱敏。 |
| AC-8 回归与实证 | 完整新候选 T4、旧 CLI 9类＋新增2类、固定真实OpenList原8场景＋发布CLI闭环、故障恢复和有界处理证据通过；旧协议／增量比较／零写／互斥／正常恢复保持。 |

AC-2 指定的第四轮顺序是本 Task 在 Slice 允许窗口内的实现选择，无需第三轮立即删除。
必要安全条件不满足不能作为 P2 延期。Task 实现完成后仍由 B 独立 Review，不自签 A／B PASS。

## 必要测试

### 开发／修正反馈

工作目录 `/opt/STRM-Generator`；治理通过后从配置／迁移／轮次、出现重置、删除／恢复聚焦验证。
命令覆盖受影响包，具体新增用例须按上表映射，而非只比较字段或内部函数名。

```bash
python3 scripts/check_governance.py
bash scripts/go-local test -count=1 ./internal/config ./internal/storage ./internal/reconciler ./cmd/strmgen
python3 -m unittest discover -s tests -p 'test_*integration*.py' -v
```

先对新二进制运行新增 mirror_cleanup、mirror_recovery，再对最终候选运行完整门槛。
上述场景名与 `--sync-binary` 为本 Task 须实现的测试入口，当前基线尚不存在，不得伪报已执行。

### 最终检查点门槛

```bash
mkdir -p artifacts/task-2.2/bin artifacts/validation-evidence
TASK_EVIDENCE="$(mktemp -d artifacts/validation-evidence/task-2.2-final-XXXXXX)"
python3 scripts/check_governance.py
bash scripts/go-local mod download
bash scripts/go-local mod verify
python3 scripts/quality.py
CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 -json ./...
bash scripts/go-local test -count=1 -json ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 bash scripts/go-local build -trimpath -o artifacts/task-2.2/bin/strmgen-amd64 ./cmd/strmgen
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 bash scripts/go-local build -trimpath -o artifacts/task-2.2/bin/strmgen-arm64 ./cmd/strmgen
bash scripts/go-local build -tags strmgen_testhooks -trimpath -o artifacts/task-2.2/bin/strmgen-testhooks-amd64 ./cmd/strmgen
python3 scripts/run_isolated_sync_integration.py --binary artifacts/task-2.2/bin/strmgen-amd64 --fault-binary artifacts/task-2.2/bin/strmgen-testhooks-amd64 --scenario all --output-dir "$TASK_EVIDENCE/cli"
bash scripts/go-local test -c -tags openlist_integration -o artifacts/task-2.2/bin/openlist-integration.test ./internal/openlist
python3 scripts/run_openlist_absence_integration.py --test-binary artifacts/task-2.2/bin/openlist-integration.test --sync-binary artifacts/task-2.2/bin/strmgen-amd64 --output-dir "$TASK_EVIDENCE/real-openlist"
git diff --check
```

quality.py 已覆盖全部 Python、gofmt、vet、test、build；完整门槛覆盖同输入聚焦测试，无须机械重复。
新行为／故障恢复适用真实服务与 CLI 要求单列；race 显式 CGO，双架构构建不代表 ARM64 实机。
除单元中的接缝外，保留从真实程序产生正常残留、SIGKILL 后发布二进制重启的证明。不得把
手工直接写入合格 missing_count 的夹具作为唯一三轮／恢复证据。

### B 独立验证

核对完整 Task Base..Head、新候选源码／二进制／夹具身份和原始日志。至少独立重放：
三个完整轮与重复 epoch／失败／预览不能凑数；再出现后续失败与开关；旧库未知映射；
移动／同目标扩展名变化；删除后目录 sync／DB 失败再启动及跨文件部分完成；符号链接／
未知暂存／未归属保护；多source隔离；真实 CLI 的根不可读和清理闭环。新增路径有界通过
固定批量、跨多批候选和观察集合的测试／测量说明，不冒充十万／百万性能已验收。

可在输入完全一致、无疑点时复用 Developer 全套 quality／race／构建／旧 CLI 原始证据；
不能只凭成功标签放行。原本真实 Get 8 场景已通过，但接入候选需验证新 CLI，不以 Task 2.1
只读确认器 PASS 替代可写删除。所有 RO 满足后停止创建 Task，执行 Slice Final 并交 Closure
Packet 给 A；有阻断留在 Task 2.2 FIX REQUIRED，不按字段或新测试另外建 Task。

### 证据与重跑

新证据目录保留到 A 决定：源码完整 SHA／工作区输入摘要、版本、命令／cwd、配置／夹具／
二进制哈希、逐门禁退出码／计数／耗时、原始日志、首次失败与重跑、每轮文件／SQL 快照及
请求顺序、旧库身份、故障接缝与发布构建区别、实际删除／部分完成及 AC／RO 映射。配置和
状态仅使用专用夹具；token、Cookie、签名URL、私有完整路径或原始错误正文不入日志。

旧证据不覆盖；新代码／测试／运行器／配置变动后按影响重跑，T4最终候选保留完整门槛。
纯报告后继可在核对可执行输入完全一致后复用，但重跑治理／空白。NOT RUN／BLOCKED／
SKIPPED 如实报告，真实环境缺失非零 BLOCKED，不改用合成API冒充。证据仅本机 artifacts，
不声称远端可读。/d/、固定插件实际解析、Jellyfin／代理、ARM64 实机和规模仍独立 NOT RUN。

## 非目标

不新增 serve／调度、发布镜像、全驱动兼容、真实播放／插件内部接入、媒体下载／哈希补算、
数据库丢失后的自动认领、人工强制删除工具、源身份转移、递归清理空目录／未知暂存。
不引入远端版本锁、每文件生成前Get、祖先强刷、错误码权威证明、刷新预算、媒体指纹表、
独立发现／恢复状态机、UID／权限加固或抵御任意外部写者的绝对承诺。

## Developer Completion Report

- Changed Files / Implemented：
  - `internal/config/config.go`：Resolved 增加 Mirror／CleanupEnabled／MissingConfirmations；
    默认 mirror=true、cleanup=false、阈值 3；显式阈值只接受整数 ≥3，清理关闭时非法值同样
    拒绝；mirror=false＋cleanup=true 在输出／数据库变更前拒绝；安全诊断映射新字段，不回显
    秘密或私有完整路径。
  - `internal/storage/storage.go`：写模式在事务中幂等升级 Slice 1 表，新增 `remote_path` 与
    `missing_round`；旧行保留归属与轮次、映射未知且计数不具资格；新增精确映射观测／清零、
    删除归属、keyset 批量枚举、资格查询与未映射计数。观察集合与待提交缺失路径放入进程私有
    内存 scratch（有页上限），`CommitRound` 与轮次同一事务提交每 epoch 至多一次的缺失计数，
    溢出不反转，重复／旧 epoch 不能覆盖新轮次。
  - `internal/reconciler/syncer.go`：列表观察（含 skip／目标冲突）立即保守清零；仅对未观察且
    映射可靠的已拥有目标复用 `openlist.Confirmer` 重核；只消费已提交阈值证据，删除前再重核
    源根与精确目标；本地失败停止该源后续清理、不提交失败轮计数；dry-run 只用已提交阈值列
    候选，不写文件／库／计数／轮次；移动时新路径先交付，旧路径独立三轮后退出；同目标扩展名
    变化按比较规则更新。
  - `internal/reconciler/fs.go` 与删除故障钩子：`InspectDeletable`／`DeleteOwned` 使用绑定
    输出根与相对 unlink＋父目录 sync，拒绝符号链接／目录／特殊文件与未知暂存；testhooks
    构建提供 unlink 前 gate、unlink 后 hold／sync 失败窗口，发布构建无环境开关。
  - `cmd/strmgen/main.go`：把清理开关／阈值传入 Reconciler；摘要区分 deleted／missing／
    reclaimed／retained／candidates。
  - 测试与运行器：新增 `cleanup_test.go`、`fs_delete_test.go`、`storage_round_test.go`，更新
    config／CLI 测试；隔离 CLI 新增 `mirror_cleanup`／`mirror_recovery`；真实 OpenList 运行器
    新增可选 `--sync-binary` 的发布 CLI Mirror 闭环；旧“cleanup true 一律拒绝”断言改为有效
    配置与非法组合验收；`dev/environment-spec.json` 登记新文件。
  - 文档：README、`config.example.yaml`、`docs/architecture.md` CURRENT、`docs/test-matrix.md`、
    `docs/development-environment.md`、`docs/progress.md`、`internal/README.md` 同步已实现事实
    与待审状态，不改变需求、Slice 边界或 Roadmap。
- Tests and Results / Evidence：证据索引
  `artifacts/validation-evidence/task-2.2-final-7TWPAV/INDEX.md`（本机 Git 忽略路径）。最终
  实现检查点后门禁：治理、`mod download`／`verify`、`quality-final2`（Python 87 项、
  gofmt、vet、test、build）、race `-race -count=1 ./...`（277 测试／8 包，0 fail／skip）、
  `-count=1 -json ./...`（277 测试／8 包，0 fail／skip）、testhooks vet／构建、Linux amd64／
  arm64、集成测试二进制、隔离 CLI `--scenario all`（`cli-final2` 11/11 PASS，含新 2 场景）、
  `git diff --check` 全部 exit 0。固定隔离 OpenList v4.2.7：原 8 场景（11 个 Go 集成测试，
  0 skip）与新增发布 CLI Mirror 闭环（`real-openlist-final`：生成 250、三轮保留、dry-run 零写
  候选、下一次重核删除、再出现清零、禁用存储源根不可读保留）全部 PASS，缓存列表自然到期
  61.66 秒，所有列表轮询 `refresh:false`。开发期间保留首次失败与修正：真实服务首轮因驱动
  错误本地路径（期望 `Wide/E0000.strm` 而实际为 `E0000.strm`）FAIL 后重跑；`cli-rerun` 的
  legacy dry-run 断言取样顺序错误；`cli-rerun-focus` 的取消 gate 存在 SIGTERM 投递竞态；
  自查又发现并修正“同一轮先缺失后重核 present 仍会提交暂存 +1”的证据缺陷（新增链式
  确认器回归），其后重建二进制并重跑全部产品门禁与两层 CLI／真实服务。
- Decisions / Risks / Remaining Required Outcomes：清理采用“第 3 个不同完整轮提交资格、
  第 4 次成功同步重核删除”，不把未提交的本轮观察凑成阈值；删除只消费已提交计数，源根不可读
  或整轮失败冻结计数。观察／待提交集合有界但为进程内 scratch，跨进程仅依赖已提交计数；
  清理本地失败不承诺跨文件回滚，重启按当前源根／Get 与本地事实收敛。旧库映射未知的历史
  缺失记录保留并报告，不猜扩展名、不自动再认领。RO-1～RO-6 的本轮实现与本地／隔离 CLI／
  固定隔离服务证据已提交；B 仍需独立重放关键窗口、逐项重评全部 RO 并决定 PASS 或
  FIX REQUIRED。真实 `/d/` 播放、固定插件实际解析、Jellyfin／代理、ARM64 实机与十万／百万
  规模仍 NOT RUN；宽目录重复枚举成本仍为已知 P2。未自签 B／A 产品 PASS。
- Head SHA：`d0a0f6db5447f38467c67612ed7173d77e6bea27`（含实现 `de80280`、测试／运行器
  补强 `3fa242b` 与自查证据修正；发布 amd64 `26f31c5a…`、arm64 `2b1374c3…`，与最终源码
  同次构建）。门禁运行后仅更新本报告与 Task 状态，无产品／测试／运行器输入变化。
- Evidence Run：`artifacts/validation-evidence/task-2.2-final-7TWPAV/`（本机 Git 忽略路径，
  不是远端可读证据；二进制 SHA-256 见 `logs/binaries.sha256`）。

## B Review Result

```text
Reviewed: NOT SET
Decision: PENDING
Slice Required Outcomes all satisfied: NO
Next: Developer implements Task 2.2 after governance, then returns for independent B review
```
