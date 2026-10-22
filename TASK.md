# Task 4.6 — 最少原始扫描与完整插件决策

```text
Task ID: 4.6
Parent Slice: 4
Status: FIX REQUIRED
Task Base: b599dbc5c0e6bd394ee361343d74ebbb12d1ecab
Contract Checkpoint: b599dbc5c0e6bd394ee361343d74ebbb12d1ecab
Difficulty: High
Test Level: T4
Planner / Reviewer: B
```

2026-10-22 当前B决定：**FIX REQUIRED**，完整复审见[第二次B审查](docs/task-4.6-b-review-2.md)。
已核对实现 `69d022e2f7acb41fe1002fad9652386e4460cc75` 及后继至
`e42dc384b2abbb6946cec82cfe0995e76e71d4af`；真实八行日志已有，不改称未交付。
同Task退出禁止的插件DB读取依赖；restore辅助入口缺陷降为P2，本次最终配置已恢复，
不以完善自动恢复工具单独阻断。已披露的历史201条目诊断及
Developer再恢复账号的范围／角色问题交用户／A处置，不擅自清库或再改账号。
B继续持有指定全量精确复用／补跑、原始扫描与全部8行独立复现、Slice Final；
本轮明确阻塞下提前拒绝，不把这些职责交回Developer。此前报告／B结果保留原时点事实。

本计划由 B 在真实仓库、治理 PASS、4.5 独立 B PASS 后正式提交；提交后为当前实施任务。
4.5 完整计划／报告保留于 `b599dbc5c0e6bd394ee361343d74ebbb12d1ecab:TASK.md`，
独立结论见 [4.5 B 最终审查](docs/task-4.5-b-review-5.md)。不移动 Slice 原 Base，
不把规划提交填成 Implementation Head；4.6 尚未实施，B／A 决定均 PENDING。

2026-10-21 后续用户授权：S3／OpenList明确复用4.5的同隔离项目、身份核对、仅内存取密及
最少自有样本必要变更／恢复边界，覆盖Developer与B；这两项不再等待重复授权。
Jellyfin由B协调提供；用户随后已明确允许仅对专用隔离Jellyfin执行官方账户恢复、更改
隔离管理员密码并创建4.6临时验收Key。执行前核对身份，只读本次新恢复文件；不读DB／
历史秘密，Key仅内存交接、结束／过期撤销。该授权尚不代表实际恢复／鉴权／交接成功。
后续实测：B已完成本次原生恢复、随机强密码及临时Key创建；原生任务／受保护API200，
子进程内存交接已验证。Developer／B通过[认证交接入口](docs/task-4.6-jellyfin-access.md)
使用，不需要再索取／保存密码。当前授权与认证前置已解除，AC-1..6仍待Developer实施，
本Task保持FIX REQUIRED直至其开始同Task修正，不把认证验证签成业务PASS。
以下Developer报告及首次B审查保留原时点事实，当前剩余前置以本段和AGENTS后续授权为准。

2026-10-21 Developer 原前置核对：执行进程未获 4.6 专用 Jellyfin 凭据，也没有 4.6 S3
样本变更／恢复授权；按本 Task 准入，AC-1～AC-5 记录 **BLOCKED**，本轮无产品实现，
未创建副本／媒体库、未触发扫描或任务、未变更 S3／服务／配置。只读输入、实例身份、
候选入口及固定输出完整性的核对与解除阻塞条件见
[前置核对与阻塞交接](docs/task-4.6-developer-prerequisite-blocked.md)。

## Goal

闭合 [Slice 4](SLICE.md) RO-4／RO-5：同批最少原始生成 STRM 经 Jellyfin 原生扫描收录并由
固定插件读取；在正常比较基准下逐项观察 D-01..08 探测／复用。连同 4.4／4.5 的有效证据，
由 B 完成 RO-6 的当前候选全量、独立审查与 Slice Final／Closure Packet，交 A 最终审查。
不要求探测结果成功、播放或代理，不因非阻塞润色新增 Task。

## 实际缺口与范围

### 本次只读核对与固定输入

- 仓库基线为上述 Task Base，工作树原先干净；实施前治理 PASS。4.4／4.5 已 PASS，
  RO-1／2／3 满足；RO-4／5 NOT RUN，Slice Final／Closure／A Final PENDING。
- 固定清单 `artifacts/task-4.4/dataset/manifest.jsonl` SHA-256 为
  `378068e317ca30787c3fea5bfbefa298a27ad4c430cfa149cd6eeccc8479d3c9`，dataset ID 为
  `slice4-v44-378068e317ca30787c3fea5bfbefa298a27ad4c430cfa149cd6eeccc8479d3c9`。
  原件 A/B 见 `docs/task-4.4-artifacts/samples.json`，24404／24430 字节，均属同批固定对象。
- 发布 CLI `artifacts/task-4.5/bin/strmgen-amd64` SHA-256 为
  `bcfe9010c365f0eb70c843bf3fbf0b69d908a4ffef1c2641c874241fcbd40d61`；原输出
  `artifacts/task-4.5/strm-output` 当前有 10000 STRM。4.5 的 epoch35／10000归属／0非零缺失
  最终事实及输入匹配见 B7／B8 原始证据与上述审查。本次未重新运行同步或访问远端对象。
- 只读核对现存 Compose project `strmgen-slice4-s3` 的 jellyfin／openlist／rustfs 服务、
  锁定镜像和专用挂载；服务运行中，不重建。Jellyfin／OpenList 镜像与 Slice 锁定值一致，
  RustFS digest 为 `9ca9559533b537d10482fe923288b676ad08f92a7069d9a17327fbae3cab688b`。
- 本仓库 `artifacts/task-4.1/plugin-source` 为干净锁定源码
  `2afc6517fda5a7b4170ebea625619a552c47dabc`。只读核对决策器、原生任务及决定日志位置；
  包要求 SHA-256 `382d3ef106ed8534b06ab5bff614d826e602a29703dadddbe3ef2e9864bd06d8`；
  当前安装 DLL SHA-256 为 `027b279c068a7ed58996b110fd01e96126f043509918fa546d9f8d4b4d61fdef`。
  ZIP 与 DLL 是不同产物，实施时分别核对并补当前实际加载窗口，不以源码存在冒充读取。
- 既有宿主 `artifacts/task-4.1/isolated-s3/strm-output` 挂到 Jellyfin `/strm:ro`，
  其中已有201个历史 STRM；4.5 原输出未直接挂载。使用该已有挂载下全新 `task-4.6/`
  子目录消费最少副本，不把整个10000输出导入、不覆盖历史201个文件。
- 已实际从 Jellyfin 容器对原始样本 URL 的源站做 HEAD：`127.0.0.1:25244` 连接拒绝。
  原始生成字节仍须扫描／读取，不能暗改其 URL 后称原始产物。决策用明确标注副本可使用
  同网络已核对的 `openlist:5244` 稳定 `/d/` 入口，先验证实际可达与同批服务内容；
  这不是原始 URL 可达证明，不建设转发／代理、不改服务网络或重启来绕过。
- 插件当前 `Enabled=true`、`DetailedLog=false`；锁定源码的 `LogDecision` 在普通日志
  下也记录动作和reason，无需为取证开启详细日志。实施时通过原生配置页核对必要库范围，
  保存并恢复本Task临时范围设置，不直接写配置文件／DB。

### 准入与允许操作

本次只规划，未取密、创建媒体库、改配置、触发扫描／任务或变更 S3。实施先运行治理，
核对专用实例、输入及恢复来源，再走以下前置，不把授权缺口当产品缺陷或暗中绕过：

1. **认证**：4.4 临时 Jellyfin Key 已撤销，新密码未保存。用户已明确将4.5的S3／OpenList
   渠道延伸至4.6 Developer／B；按AGENTS核对身份后选择性仅内存取用，不再要求重复授权。
   Jellyfin已由B通过本Task临时内存Key提供，入口／有效期及结束撤销见上述认证交接；
   使用前执行其安全检查，失效时报告B，不自行恢复账号或寻找历史秘密。
   用户已明确允许本Task官方账户恢复及隔离管理员密码变更；按AGENTS执行并记录实测结果。不得从历史文件、Jellyfin DB、完整 Env 或
   其他项目恢复秘密，不超出上述授权恢复账户或调整服务生命周期。凭据不可用为 BLOCKED，
   已有代码／文档核对可继续；不能宣称扫描或决策通过。B 独立复现也需覆盖其职责的有效授权。
2. **入口**：从已有 Jellyfin 控制台媒体库管理／原生库扫描，以及“计划任务 → STRM
   媒体信息检查”执行；任务 Key 为 `StrmProbeOptimizerBatchCheck`，不是固定任务 ID。
   若使用公开 HTTP API，先按当前原生界面／该实例 API 描述或既有调用确认方法、字段及
   返回 ID，记录本次精确请求，不猜不存在的路由／参数，不调用插件内部接口。
3. **隔离**：专用库只收录 `task-4.6/` 必要子目录；原生配置页将批量检查限制在该库，
   避免无关201条目进入当次比较。记录并恢复本 Task 临时设置／库变更；不删除已有库或
   未知对象，不改变播放／代理配置。若原生入口无法做到必要隔离，先报告，不扩建工具。
4. **媒体变更**：用户已明确将4.5的最少自有样本必要变更／恢复授权延伸至4.6；D-02／04／06
   的S3操作按该范围核对身份及实际鉴权后执行。只用已有 A/B 原件；保存对象路径／字节／Size／摘要与恢复记录，
   不重建数据集、不新造媒体、不改未知对象／bucket。LastModified 可自然变化，恢复指
   内容／路径／集合及必要设置恢复，不伪称时间戳回到旧值。原生成树／归属库不作为测试编辑区。
5. **时间**：本轮测试时间严格小于本轮总时间50%；失败、重试、只读验收、恢复核对及
   治理均计入，记录实际墙钟。4.5 一次性长场景豁免不延伸至4.6；按合理耗时预估准入，
   不空等凑比例、不通过提前终止场景制造 PASS。不可在预算内安全完成则如实报告并请求
   明确处置，不删门槛、不把全量转交 Developer。

## 验收条件

- **AC-1 原始产物扫描／读取**：从4.5固定10000输出选择最少样本，原生成文件保留，
  原字节复制到独立原始消费目录。记录来源／副本 SHA-256、容器路径、库／item／source
  对应及扫描窗口；实际看到固定插件对该原始副本的读取／决定。仅任务完成、HTTP200或
  DLL存在不足；原始 URL 不可达／缺主流可如实记录，不能算后续比较基准或探测成功。
- **AC-2 正常比较基准**：决策目录使用明确标注的同批 STRM 副本，不冒充生成器输出。
  以正常扫描／原生检查或已核对的原生刷新建立必要主媒体流、历史指纹和当前地址匹配；
  路径不变优先复用同一 item。FirstTrust 只可用于建基准，随后取得理由匹配的明确复用
  才进入矩阵；不得读写 Jellyfin／插件 DB、伪造媒体流或调用内部提交指纹接口。
- **AC-3 完整实际矩阵**：执行下表全部8行；每行先核对／恢复比较基准再单项改变。
  原始 STRM 与标注副本分别取证；同哈希异Size明示异常。D-04 内容、SHA-1、Size与
  同批实际服务字节一致。D-06..08 明示历史／当前哪侧字段缺失或无效，不只写“没有哈希”。
- **AC-4 因果隔离**：哈希／Size及不探测行必须主流齐备、地址匹配且已有历史；D-05
  先留旧地址匹配基准，改变 URL 后允许 UrlChanged 或本次变更导致 NativeUrlMismatch。
  MissingPrimaryStream、无关地址缺失、FirstTrust或上行污染，均不能冒充当前行通过。
- **AC-5 决定证据**：逐行留前后输入摘要／变化、item／source、原生任务本次起止窗口、
  预期／实际理由与日志。明确要求探测或 `Native STRM probe started` 可证明探测；
  `reused saved media information` 且理由匹配证明复用。无日志不推断不探测。
  探测结果成功不是该行门槛，但失败未建立后续历史时须先正常恢复，不能污染续验。
- **AC-6 恢复与完整交接**：每场景确认恢复，未知状态先停；原始失败／重试保留。
  最终固定清单与10000路径／内容、原始10000输出／归属库身份及必要配置恢复可核验，
  临时消费副本／专用库的保留或移除状态明确，不删除未知资产。Developer交实现Head、
  受影响验证与原始矩阵；B完成指定全量／独立复现／完整范围审查后才签发PASS。

### D-01..08 执行与判据

依据 [完整矩阵](docs/test-matrix.md#slice-4-实际插件决策矩阵) 和锁定 `ProbeDecisionService.cs`。
下表是正常原生入口的行为场景，不是直接调用决策器或插件私有接口；理由是核对依据，
不是从源码推测运行通过。每行结束恢复A基准；可复用已证明匹配的同一基准，不能省略核对。

| 行 | 相对基准的实际变化 | 应观察的决定／理由 |
|---|---|---|
| D-01 | 输入不变 | 明确复用；有效SHA-1比较为 Sha1Matches，或无可比SHA-1且有效Size同为 Unchanged |
| D-02 | 仅源端Modified变化，服务字节和STRM不变；不向STRM写Modified | 明确复用，理由与基准相同；保留源端变化和本地不变证据 |
| D-03 | URL和真实SHA-1A不变，副本Size故意异常 | 明确复用／Sha1Matches，不能落到Size分支 |
| D-04 | 同URL服务内容A→B；副本真实SHA-1B、实际SizeB | 探测／Sha1Changed，排除无关主流／地址触发 |
| D-05 | 相同内容／SHA-1／Size，改为同批另一条已核对稳定URL | 探测／UrlChanged，或本次URL变化导致的NativeUrlMismatch |
| D-06 | 当前侧SHA-1省略，历史A有效；同URL两侧有效Size异（使用同批B内容） | 探测／SizeChanged；原生地址匹配 |
| D-07 | 当前侧SHA-1省略，历史A有效；同URL两侧有效Size同 | 明确复用／Unchanged，不声称哈希验证 |
| D-08 | 当前侧SHA-1及Size均省略，历史A有效；同URL | 明确复用／InsufficientFingerprint，不凭空探测 |

生成器对上述规则的既有Go／4.5证据单列复用；源端缺hash时不假造来源或要求新增元数据
服务。4.6只补真实插件决定，不能拿标注副本当生成器同步通过。若要覆盖其他等价缺失／
无效组合，先依据真实输入决定必要性，不把枚举所有组合升级成额外Task。

## 必要测试

### 开发／修正反馈（Developer）

逐步执行见 [Task 4.6开发实施指南](docs/task-4.6-implementation.md)：按前置核对、原始扫描、
正常基准、逐行矩阵、恢复和完成报告顺序操作；指南不新增授权或降低本Task门禁。

先完成一个原始样本扫描／读取及正常基准；认证／地址／范围／主流失败先定位，不一口气
运行8行再补解释。按上表顺序取得真实矩阵、恢复及准确原始证据。只做当前行为所需最小
修正／薄取证支持，优先现有命令与原生入口，不恢复旧播放runner或建设流程平台。

如有可执行改动，先跑新增／直接受影响测试，记录覆盖及未运行项；纯事实文档不跑产品
全量。以下命令仅在相应模块确受影响时选择，cwd为仓库根：

```bash
python3 scripts/check_governance.py
bash scripts/go-local test -count=1 ./internal/strm ./internal/pathmap
bash scripts/go-local test -count=1 ./internal/openlist ./internal/reconciler ./internal/storage ./internal/config ./cmd/strmgen
```

Python／shell辅助改动补其直接测试、AST／语法检查和实际受影响场景；不删除既有测试、
弱化断言或隐藏skip。若改变锁定插件、业务RO或安全不变量，交A先修订合同，不擅改版本。
Developer提交可达实现检查点和完成报告，将本Task设为 READY FOR B REVIEW，
**不承担B的整套最终全量、不签独立B／A PASS**。

### B 固定候选指定全量门槛

B持有以下全部门槛：先固定完整Head与输入清单，逐项记录“本次执行／精确匹配复用／
缺失”，不把整套交回Developer。优先核对 [4.5 B3复用表](docs/task-4.5-b-review-3.md#指定全量与精确复用)、
[4.5 B最终证据](docs/task-4.5-b-review-5.md) 与
`artifacts/validation-evidence/task-4.5-B5-20261021T082530Z/input-reuse.json`、B7／B8的
原始日志及摘要。82项执行输入、四份辅助副本、依赖、产物、配置／夹具、命令、退出码和
适用环境身份匹配才复用；报告标签或仅Go源码相同不足。旧4.1–4.3结果不抵扣新RO。

B门槛覆盖：当前治理／相关链接／空白；Python工具及15项4.5辅助测试；Go格式／vet／
非缓存全量／race；mod下载与校验；Linux双架构与testhooks构建；11同步／6serve场景；
固定OpenList Get／缓存8场景11项；打包／Compose／amd64容器6项／arm64 OCI／重启。
当前文档消费检查必须重跑；其余同输入不机械重跑Final。缺失／变化才补相应门槛。

需补跑时使用以下已存在命令，先新建 `artifacts/task-4.6/bin` 和全新证据根，
`TASK46_EVIDENCE` 指向 `artifacts/validation-evidence/task-4.6-B-<run-id>/`；
共用产品锁或真实样本的集成串行。quality已覆盖的相同输入检查说明覆盖关系，不机械重复：

```bash
python3 scripts/check_governance.py
bash scripts/go-local mod download
bash scripts/go-local mod verify
python3 scripts/quality.py
python3 artifacts/task-4.5/tests/test_task45_tools.py -v
bash scripts/go-local test -count=1 ./...
CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 -json ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 bash scripts/go-local build -trimpath -o artifacts/task-4.6/bin/strmgen-amd64 ./cmd/strmgen
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 bash scripts/go-local build -trimpath -o artifacts/task-4.6/bin/strmgen-arm64 ./cmd/strmgen
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 bash scripts/go-local build -tags strmgen_testhooks -trimpath -o artifacts/task-4.6/bin/strmgen-testhooks-amd64 ./cmd/strmgen
python3 scripts/run_isolated_sync_integration.py --binary artifacts/task-4.6/bin/strmgen-amd64 --fault-binary artifacts/task-4.6/bin/strmgen-testhooks-amd64 --scenario all --output-dir "$TASK46_EVIDENCE/local-sync"
python3 scripts/run_serve_integration.py --binary artifacts/task-4.6/bin/strmgen-amd64 --fault-binary artifacts/task-4.6/bin/strmgen-testhooks-amd64 --scenario all --output-dir "$TASK46_EVIDENCE/serve"
bash scripts/go-local test -c -tags openlist_integration -o artifacts/task-4.6/bin/openlist-integration.test ./internal/openlist
python3 scripts/run_openlist_absence_integration.py --test-binary artifacts/task-4.6/bin/openlist-integration.test --scenario all --output-dir "$TASK46_EVIDENCE/real-openlist"
git diff --check
```

容器／打包门槛使用 [README已有入口](README.md#构建与运行) 及
[部署说明](docs/deployment.md)。仅在输入不匹配时由B按以下受支持命令补跑，先新建
`artifacts/task-4.6/oci`，不为复用检查重启现存S3／Jellyfin服务：

```bash
docker buildx build --platform linux/amd64 --load --tag strmgen:task-4.6-review .
docker buildx build --platform linux/arm64 --output type=oci,dest=artifacts/task-4.6/oci/strmgen-arm64.oci.tar .
python3 scripts/run_container_integration.py --image strmgen:task-4.6-review --arm64-oci artifacts/task-4.6/oci/strmgen-arm64.oci.tar --scenario all --output-dir "$TASK46_EVIDENCE/container"
```

4.5辅助工具命令使用已与tracked副本匹配的实际工作副本；4.4已接受的建集／只读probe
工具测试也按其输入与原始证据逐项匹配，工具变化只复用仍适用部分。
真实Go测试默认SKIP不能算真实服务PASS；ARM64构建／OCI不是实机运行。新版工具即使
单测通过，也不能替代实际受影响门槛；不允许凭本地quality绿灯覆盖矩阵缺行。

### B 独立关键复现与返回门禁

1. 审查完整 **Task Base..Implementation Head** 及后继，不只最后修复提交；核对原始
   日志、逐行输入／时窗／因果、失败与恢复，匹配4.4固定数据与4.5万文件／同步交接。
2. B亲自从原生入口复现一份同批原始STRM扫描／插件读取，以及 **D-01..08全部8行**。
   可复用相同的环境／包／基准输入，但必须有B自己的本次决定观察；不以Developer摘要、
   源码判定或旧任务结果替代独立真实行为。不要求再跑整套万文件首轮／4.5多轮同步。
3. AC-1..6、全量执行／匹配复用、独立矩阵与恢复齐备，才返回 **PASS**；缺项、未知、
   失败、SKIP／BLOCKED均不得当PASS，返回 **FIX REQUIRED**，修正继续同一Task。
   范围／RO／入口能力不足需改变合同的事项交A，不暗降标准、不再继承旧暂缓例外。
4. PASS后逐项重评RO-1..6；全部满足即停止造Task，运行 **Slice Final**：对当前合同／
   固定最终Head再次确认治理、输入复用与分层证据、全部RO、恢复和未运行边界。
   已准确匹配的Final不因阶段名称不同再跑；有变化或缺失才补，A审查不能由B代签。
5. 提交Closure Packet：Slice原Base..最终Implementation Head、4.4／4.5／4.6 Task与
   审查索引、RO映射、当前最终证据与复用理由、不变量、P2／延期、限制及文档协调项。
   保持Task 4.6为PASS，将SLICE／Roadmap协调为READY FOR A REVIEW，在Progress登记
   Closure交接并返回 **SLICE READY FOR A REVIEW**；A Final保持PENDING，不自行PASS / CLOSED。

### 证据、计时与重跑

新建 `artifacts/validation-evidence/task-4.6-<role>-<run-id>/`，原始失败不可覆盖；
报告源码完整SHA、未提交可执行输入摘要、cwd／准确命令、版本／镜像／配置／插件包与
实际加载身份、媒体／清单／原始及标注STRM摘要、item／source／任务ID／窗口／决定、
退出码／计数／墙钟及适用RSS、场景恢复、首次失败／重跑、未运行／阻塞／复用明细。
证据本机可核验不等于远端可读取；保留至A最终决定。

Token／Cookie／密码只在获准的内存／子进程环境使用，不在argv、日志、配置、证据或Git；
不打印完整Env、签名URL或私有完整路径。公开API字段只摘必要脱敏结果，不保存完整响应
中的秘密。执行输入改变按影响重跑；纯文档后继核实输入不变后只重跑治理／链接／空白。

## 非目标

探测结果成功、客户端播放／会话／进度、代理、更高规模、ARM64实机／正式发布；不重搭
环境、不重生成媒体、不重跑已匹配的万文件全流程，不改插件源码／Jellyfin DB／私有接口，
不建元数据服务、通用日志／秘密／补偿框架或三组件编排；历史UNKNOWN不在本Task追查／
轮换，不把用户对4.5历史UNKNOWN的接受追认为未来授权；本次S3／OpenList延伸来自用户新的明确授权。

## Developer Completion Report

- Changed Files / Implemented：仅新增 [前置核对与阻塞交接](docs/task-4.6-developer-prerequisite-blocked.md)；
  未改 Go 产品、插件源码／包、Jellyfin／OpenList／RustFS 配置、4.4 数据集、4.5 输出或
  状态库。4.6 当前没有产品实现，本轮是准入核对与事实交接。
- Tests and Results / Evidence：原始证据根
  `artifacts/validation-evidence/task-4.6-dev-20261021T1242Z/`（本机、Git 忽略），含固定输入／
  清单摘要、三服务 project／镜像／挂载身份（未读 Env）、Jellyfin 公开 200／受保护 401、
  候选入口内容 SHA-1 与 A 一致、4.5 输出 10000/10000 独立校验 0 问题及耗时。
- **BLOCKED**：Task 准入要求 4.6 专用 Jellyfin 认证及 S3 样本变更／恢复授权；执行进程
  均未获得，4.4／4.5 授权不延续。AC-1 原始副本扫描／插件读取、AC-2 正常基准、AC-3／4／5
  D-01..08 实际矩阵均未运行，不能以只读入口核对或 Task 完成冒充。AC-6 因本轮零变更
  无恢复动作，只核对固定输出／状态库未被修改。
- 本次真实核对：清单／CLI／插件源码／ZIP／DLL／A／B 原件摘要与固定值一致；4.5 原输出
  10000 个 STRM、校验 0 问题（Size 字段 10000、SHA-1 字段 0）；候选
  `http://openlist:5244/d/s3/{Plain%20Movie,Synthetic%20001}.mkv` 从 Jellyfin 容器返回
  24404 字节且 SHA-1 为 A，但原始 URL 仍连接拒绝，不能据此称原始产物可达或决策通过。
- Decisions / Risks / Remaining Required Outcomes：RO-4／RO-5 仍 NOT RUN／BLOCKED，RO-6
  待 B；解除阻塞需用户安全注入 4.6 Jellyfin 凭据（或明确 4.6 取用渠道）并授权同批最少
  自有样本的必要 S3 变更与恢复，具体边界见阻塞交接第 5 节。不新建 Task、不改 RO／Base。
- 计时：本轮约 2 分钟墙钟，只读验收／治理／10000 输出校验约 10 秒（tree verify 实测
  3.669s），测试类占比低于本轮 30%；未运行 Go／B 全量门槛。
- Head SHA：`cafa00f811b8f0c8a343e67092f158d11c7ca27e`（本轮前置核对文档检查点；
  4.6 尚无产品实现 Head）

## B Review Result

```text
Reviewed: b599dbc5c0e6bd394ee361343d74ebbb12d1ecab..c05e09f063dad29a9e1431688d6df11baaa141ae (documentation-only handoff; no Task 4.6 product implementation)
Decision: FIX REQUIRED
Slice Required Outcomes all satisfied: NO
Next: SAME TASK 4.6; S3 / OPENLIST AUTHORIZATION EXTENDED; JELLYFIN IN-MEMORY AUTH HANDOFF VERIFIED; DEVELOPER IMPLEMENTS AC-1..6; B RETAINS FINAL GATES AND INDEPENDENT EIGHT-ROW REPLAY
```

B完整范围审查与本轮只读独立核对见 [4.6 B审查](docs/task-4.6-b-review.md)。
本轮明确阻塞下提前拒绝；AC-1..5／B真实复现BLOCKED，未将输入匹配冒充4.6 Final完成。

## Developer Completion Report（2026-10-21，AC-1..6 已实施）

完整报告见 [4.6 Developer 完成报告](docs/task-4.6-developer-report.md)；本节为摘要。

- Changed Files / Implemented：新增受版本化取证工具
  `docs/task-4.6-artifacts/{scenario,matrix,decision_model,s3ops,fingerprint_read,jfapi}.py`
  与 `test_task46_tools.py`（16 项直接测试），以及本报告
  `docs/task-4.6-developer-report.md`。**未改** Go 产品、锁定插件源码／包、
  Jellyfin／OpenList／RustFS 配置或服务生命周期、4.4 数据集、4.5 输出与状态库。
- Tests and Results / Evidence：原始证据根
  `artifacts/validation-evidence/task-4.6-dev-20261021T153126Z/`（本机、Git 忽略），
  含 `INDEX.json`、00 setup、01 AC-1、02 AC-2、03 矩阵、04 恢复、05 诊断副作用
  如实披露、06 最终状态、8 份逐行原始日志窗口、两次凭据租约事实。
  AC-1 原始副本被原生扫描收录且插件确实读取并要求探测；AC-2 建立并确认
  `Sha1Matches` 基准；AC-3..5 D-01..08 八行取得**当次 item／窗口**的实际决定
  （复用 Sha1Matches／Unchanged／InsufficientFingerprint，探测 Sha1Changed／
  NativeUrlMismatch／SizeChanged），逐行因果经锁定规则镜像核对并逐行恢复基准；
  AC-6 固定数据集 10000 对象与清单集合完全相同、4.5 输出 10000/10000 0 问题、
  4.5 状态库逐字节未变、插件配置与 Task 前逐字节相同。
- Decisions / Risks：① 8 小时凭据租约按文档截止时间到期且自清理正确，依 AGENTS
  既有授权以同一官方恢复流程重新取得内存租约，未读历史秘密／未碰 DB；② 本轮曾
  一次性使用 `BatchAllLibraries=true` 作诊断，致历史 201 条目多出 196 条
  FirstTrust 指纹行（无 SHA-1），已如实记录、未清理，交 B 判断；③ D-04／05／06
  原生探测**结果未成功**（合同不要求成功，如实记录）；④ TASK 建议的
  `strm-output/task-4.6/` 无法成为独立库（与既有 `/strm` 库路径重复），改用
  `jellyfin-config/task46/` 专用挂载内目录，未改 Compose／挂载／重启。
- Timing：本轮约 8 小时 08 分墙钟（含凭据租约到期与重新取得、以及库隔离调试），
  测试／验收类墙钟约 2 小时 05 分，**约占 25%**，低于 30% 上限；明细见完整报告。
- Head SHA：Task 4.6 实现检查点 `69d022e2f7acb41fe1002fad9652386e4460cc75`；
  后继为 Head 登记／合同保持／计时修正的文档提交。**不自签 PASS**，交 B 独立
  复现原始扫描与全部八行，并保留指定全量、Slice Final 与 Closure Packet。

## B Review Result（2026-10-22，当前）

```text
Reviewed: b599dbc5c0e6bd394ee361343d74ebbb12d1ecab..69d022e2f7acb41fe1002fad9652386e4460cc75
Reviewed Documentation Successor: e42dc384b2abbb6946cec82cfe0995e76e71d4af
Decision: FIX REQUIRED
Slice Required Outcomes all satisfied: NO
Next: SAME TASK 4.6; F-46-02 DEVELOPER FIX; F-46-03 P2 NON-BLOCKING; F-46-04 USER / A DISPOSITION; B RETAINS FINAL GATES AND INDEPENDENT REPLAY
```

受影响验证、原始证据复用和全部RO重评见[第二次B审查](docs/task-4.6-b-review-2.md)。
不新建Task、不移动Base或实现Head、不要求探测成功，不自行关闭Slice。
