# Task 4.4 独立 B 审查（2026-10-20）

Decision: **FIX REQUIRED**。Task 4.4 保持原 Base，Slice 4 保持 ACTIVE；4.5／4.6 不激活。

审查范围为 `77fc768790a1c1ba6f0700c0aab294d3237cf7e4..01104ed2c879ae36b28ac8cacea301296272c92f`，
并核对纯文档后继 `995fac81905f71bc90cfe351f8ca3d61f96a090a`。
完整范围包含 B 计划／实施指南、工具清单和 Developer 新增工具／测试／配置快照；
未改 Go 产品、依赖、Dockerfile、插件源码或生产配置。后继仅记录真实 Head，没有改变可执行输入。

原始证据：`artifacts/validation-evidence/task-4.4-B-20261020T0533/`。
`reproduce.py`／`reproduction.json` 保存独立反例及输入匹配，`run_gates.py`／`gates.json`
保存命令、cwd、退出码、耗时；各命令另有原始日志。夹具全部在该证据根内，
根外写入反例的目标也仅是夹具内另一个目录；没有改真实媒体／远端对象／服务生命周期。

## 阻塞项与同 Task 修正

| ID | 等级／位置 | 复现、影响与修正要求 |
|---|---|---|
| F-01 | P1；AC-1／3／4／6 | 本地候选确有 10000 行／10000 媒体，最终 manifest.jsonl 不存在。B 真实 OpenList 读取仅见 201 个文件，无子目录，A 样本字节匹配；没有 B／bulk 的完整可消费数据集。S3 写凭据／环境 remote 当前不可用，原报告的远端 201 来自数据目录和 OpenList，不能代替完整 S3 API 对象清单。须在本 Task 补传／逐项对账，保留内容来源验证，固定最终清单／dataset ID 后再交 4.5。 |
| F-02 | P1；generate_media.py:184–193、232 | 预置 `.strmgen-tmp-preserved-original.mkv` 后执行补缺失，既有文件被 unlink，且被计数逻辑忽略。仅名前缀不证明本次创建／归属，违反 AC-2 和实施指南只清理本次创建临时文件的要求。去除跨次前缀清扫；保留未知对象并报冲突，仅清理本次确知创建的临时文件。 |
| F-03 | P1；generate_media.py:207–247 | 已有候选清单，运行前 media/bulk 是指向夹具外侧目录的静态符号链接；prepare 先经该链接发布 000001.mkv，之后枚举才抛 ToolError。复用候选绕过写入前目录校验，不能用最终失败掩盖已发生的越界写。写入／验证前核对媒体根及路径祖先，拒绝静态链接／类型冲突。无需添加外部程序任意并发替换或 UID 加固系统。 |
| F-04 | P1；openlist_readonly_probe.py:102–138 | 独立 I/O 夹具返回样本 Get code=403、HTTP 200 错误媒体且 sha256_match=false，main 仍退出 0。代码只列根目录、只记录子目录名字，无递归或固定清单比对；同样不足以验收真实 bulk/ 数据。须让必要核对失败返回非零，完整分页递归后比较清单集合／Size，精确 Get 和稳定字节覆盖 A、B 及特殊名，逐段编码路径。当前历史成功 JSON 可逐字段核对，但工具退出 0 本身无通过效力。 |

上述三项代码反例均已独立复现；复现程序退出 0 表示成功捕获缺陷，
`reproduction.json` 的行为验收分别为 FAIL，不是产品 PASS。
F-04 的递归缺口由完整源码审查确认，当前真实 201 个平铺文件证据不能证明未来 bulk/。

一并协调事实文档：根 Task 完成报告、实施指南、资产清单、Roadmap／Progress／架构及
Slice 当前执行说明中仍有“均未实施／NOT SET／原生成脚本未改”的旧快照。
规划时事实可标注日期保留，当前状态须与真实 Head、候选／最终、部分完成及 B 决定一致。
这是事实协调，不改变 RO 或验收含义；如提出缩减远端万文件要求，应先交 A 修订合同。

## 门禁与证据复用

| 检查 | 本轮结果／证据 |
|---|---|
| 初始 status／治理 | 工作区干净，main；check_governance.py PASS。治理不代表产品验收。 |
| Task 指定 quality 全量 | B 执行 PASS，10.420 s；111 Python 测试、AST／Bash／空白、gofmt、Go vet／test／build。Go test 使用有效缓存，不称 uncached／race／真实服务通过。原 Developer 未提供 quality，本轮由 B 补齐。 |
| artifacts 工具直接测试 | B 独立执行 11/11 PASS，0.358 s；这些断言未覆盖本轮三个反例，不能据此接受 AC-2／4。 |
| 两环境脚本 bash -n／生成工具 AST | B 执行 PASS；语法通过不代表启动／停止实证。本轮无需执行生命周期操作。 |
| 当前真实 OpenList | 只读 probe 1.699 s；逐字段核对源根目录、refresh:false、201 文件／无子目录、A 精确 Get 及稳定字节 SHA-256 匹配。只支持这一样本和本次列表。 |
| 独立本地身份 | 10000 候选行／本地媒体，清单 SHA-256 为 `378068e317ca30787c3fea5bfbefa298a27ad4c430cfa149cd6eeccc8479d3c9`；保留样本 SHA-256 为 `7f53d1542382ea9035ca3059315b2ec0d2efd47bc5d70b0cc36cfd24bf96f12d`。最终清单缺失。 |
| 输入匹配 | 五份 tracked 快照与实际 artifacts 脚本原版／适配版／测试／probe／配置字节全部一致，具体摘要在 reproduction.json。完整 diff 已审；没有未提交可执行输入。 |
| 可复用 Developer 部分证据 | `task-4.4-IKlUvR/INDEX.md` 及对应原始文件。相同工具／候选／本地媒体身份支持既有原件摘要／mtime和 A/B 内容记录、已执行本地补缺失记录；不重复建集。七个旧平铺样本 JSON 逐字段均匹配，仅作为相同样本证据。 |
| 外部身份参考 | 核对 environment-identity.json 的固定镜像／包／源码身份与合同一致，启动日志是历史加载窗口；本轮不宣称独立重建／重新加载或当前插件原生任务全部可用。原报告环境部分不能替代缺失的专用访问验证。 |
| NOT RUN／BLOCKED | 完整 S3 API 对账、补传、最终固定及 B 10000 远端复核 BLOCKED；最终数据集的 OpenList 递归／B 样本 NOT RUN。4.5 生成／同步／RSS、4.6 原始扫描／D-01..08、Slice Final 均 NOT RUN。 |

B 未重跑 T4 race／容器／故障重启：本 Task 为 T3、没有产品变化，也未触及这些输入。
不重跑已知未满足 RO 的 Slice Final，不把 smoke、历史插件日志或缺日志解释为验收通过。
原始证据仅本机可读，继续保留至 A 最终决定。

## Developer 受影响验证与 B 返回门槛

Developer 在同一 Task 先修正 F-02..04，并在专用小夹具执行直接测试：补缺失、重复零写、
冲突／中断续传、未知前缀对象保留、候选复用／verify-only 静态根与祖先链接拒绝、
清单禁止放媒体根内、同名 STRM 映射冲突写前拒绝；probe 的失败退出、递归分页／重复／
不完整列表拒绝、集合／Size／样本字节比较和特殊路径编码。保留所有首败，更新实际工具及
tracked 准确快照；不另造媒体集或通用管理平台。

恢复专用进程环境凭据后复用同一候选和本地 10000 文件，先列 S3 再只补缺失，逐项核对
来源／内容与集合，完成真实 OpenList 递归／样本、原生入口可用性和最终清单／恢复交接。
不从历史文件／容器 Env 恢复秘密，不清理未知对象凑数。凭据缺失允许继续本地修正，
远端依赖步骤保持 BLOCKED；B 不自行缩减合同。

B 在新的固定实施 Head 审查完整原 Base..Head，负责重新执行本 Task 指定的 quality 全量、
治理／语法／空白及受影响工具门禁，独立重放三个反例并核对补缺失／冲突闭环；
独立复核保留媒体、S3 10000 清单、最终 dataset 身份及 OpenList A/B／完整集合。
未改变的原始媒体／环境证据按准确输入匹配复用；不将全量交回 Developer。
若改变 Go 产品行为，B 先调整同一 Task 为 T4，再明确新增完整门槛。

## 全部 RO 重评

| RO | 当前结论 |
|---|---|
| RO-1 | 部分：资产／原版及字节身份可追溯；工具保留安全和专用访问尚有 F-02..04／F-01 缺口。 |
| RO-2 | 未满足：仅本地候选 10000，远端／最终固定 BLOCKED。 |
| RO-3 | NOT RUN：4.5 尚未激活。 |
| RO-4 | NOT RUN：4.6 尚未激活，旧加载窗口不抵扣。 |
| RO-5 | NOT RUN：D-01..08 当前任务实际决定待验收。 |
| RO-6 | 部分：本轮已独立审查及补齐本候选 T3 本地门禁；后续完整验收／Final／A 尚缺。 |

全部 RO 未满足，不产生 Closure Packet，不返回 SLICE READY FOR A REVIEW，不自行关闭。
保留现有 4.5／4.6 候选；没有为了修正增造 Task。

本轮计时从 05:33:01 UTC 开始；截至 05:34:44 UTC 已 103 s，已完成测试类进程合计约
12.7 s（含 quality、直接测试、反例、只读 probe／语法及初始治理），约 12.4%。
后续仅文档／治理／空白和提交检查，最终计时见同证据根 INDEX.md；低于用户 50% 限额，
没有空等凑比。

## 修复候选复审（2026-10-20 B2）

Decision: **FIX REQUIRED**。本节为当前结论，上文保留首轮失败与审查历史。
Task 4.4 回到 FIX REQUIRED，沿用原 Task Base；Slice 4 ACTIVE，4.5／4.6 不激活。

完整 Reviewed Range：`77fc768790a1c1ba6f0700c0aab294d3237cf7e4..e76a21c8e54a373fe53219b7a1aba5e86036f960`。
核对文档后继至 `a93d22fabb354cc73d53d05df458b61df8defee9`，后继仅记录实现 Head、
恢复固定合同原文及交接状态；可执行输入与实施 Head 相同。完整差异留于本轮
`reviewed-range.diff`／`document-successors.diff`。没有 Go 产品、依赖、容器或插件实现变化，
继续 T3；没有用最后一个修复提交代替完整 Task 审查。

原始证据：`artifacts/validation-evidence/task-4.4-B2-20261020T060853Z/`，
本机、Git 忽略，保留至 A 最终决定。`review.py`／`reproduction.json` 保存 B 独立反例、
准确输入与本地核对；`supplement.py`／`supplement.json` 保存递归两页与首败判定；
`gates.json` 及逐命令日志保存 cwd、命令、退出码和耗时。仅在此证据根内构造冲突对象，
没有改真实媒体、S3、服务生命周期、生产配置或 Jellyfin DB。

### 当前发现与修复判定

| ID | 等级／结论 | 证据、影响及同 Task 动作 |
|---|---|---|
| F-01 | P1；未解决／BLOCKED | 最终 manifest.jsonl 仍不存在；本地候选／媒体各 10000，不能代表 S3。当前六个专用环境变量均 absent，仅记录存在性。真实 OpenList 可见 201，传候选核对明确缺 9799、退出 2。S3 API 清单／补传／内容对账、最终 dataset_id 及原生访问验证仍未完成；恢复当前专用进程环境凭据后完成，不恢复历史秘密、不用 OpenList 代替 S3 API。 |
| F-02 | P1；部分修复，仍阻塞 | 原未知前缀反例现保留并抛 ToolError。但 generate_media.py:232 的 _publish 仍以 wb 打开固定 PID 临时名，finally 无本次成功创建依据即 unlink。独立夹具先保存候选，再预置 `.strmgen-tmp-<当前pid>-000001.mkv`；prepare 返回 1（成功新增），既有临时对象被覆盖并删除。209 行的 write_manifest 对既有 `.strmgen-tmp-manifest.candidate.jsonl` 也覆盖并改名。都是运行前已存在的静态对象，无并发写者。媒体和清单临时文件均须排他创建，冲突保留；清理须以本次成功创建为前提。不要只将 wb 改成 xb 后仍在 finally 删除创建失败的既有对象。 |
| F-03 | 已关闭：指定静态链接缺陷 | 独立重放候选复用／静态 bulk 链接：写前 ToolError，夹具外侧目录保持空；源码确认全部计划路径先验及 verify-only 校验。只接受正常单写者下该修复，不声称任意并发替换防护。 |
| F-04 | 已关闭：工具逻辑缺陷 | B 独立验证样本 HTTP／应用 code／目录／Size／同大小错误字节均退出 2；清单缺／多／Size 不符均 ProbeError。递归 bulk 两页、total 改变／重复／空中间页、refresh:false、特殊路径编码分别复现。真实最终万文件与 B 字节仍随 F-01 未验收。 |
| F-05 | P2；不单独阻塞当前固定候选 | 实施指南第 3／5 节与此前 B 要求的同目标映射拒绝未实现：夹具已有 Movie.mkv、Movie.mp4，--count 3 --prepare 仍退出 0 并新增 bulk。当前真实候选已独立确认 10000 个不同 STRM 映射，因此不把任意新数据布局升级为当前固定数据集阻塞，也不因此新造 Task；同 Task 修正时补最小校验／直接测试，或在交接中保留限制供 A 决定。该项不写成测试 PASS。 |

首轮反例修复不等于 F-02 完全关闭。`reproduction.json` 中精确 PID 和清单临时名两个
case 的 exception=null 表示成功捕获缺陷，行为验收为 **FAIL**；不是产品通过。
Developer 的 `reproduce_fix.py` 出自实施回合，仅作为实施反馈；本节独立结论来自 B 本轮。
其旧 source_head 字段指向实现前提交，故不单靠该 SHA 复用；以实际六份文件摘要核对，
与提交 e76a21c 及 artifacts 逐字节均相同后才接受对应结果。

### 固定候选门禁与匹配复用

| 检查 | 本轮结果与实际边界 |
|---|---|
| 初始 status／治理 | main 干净；治理 PASS，不代表业务验收。 |
| 指定 quality 全量 | B 执行 PASS，7.702 s；111 Python、AST／Bash／空白、gofmt、Go vet／test／build。Go test 各包 cached；不是 uncached／race／真实服务。 |
| 直接工具测试 | B 执行 20/20 PASS，0.246 s。实际为 14 项媒体工具＋6 项 probe；Developer 报告“新增 9 项 probe”不准确。新增独立反例未被这 20 项覆盖。 |
| 脚本语法 | B 执行 bootstrap／teardown 的 bash -n 及媒体／probe AST，全部退出 0；没有强制启停或重建运行中环境。 |
| 六份 artifacts 输入 | 均匹配 tracked 快照及实施 Head 的准确字节；原版生成脚本、bootstrap／teardown 摘要与上轮相同。没有未提交产品输入。 |
| 本地数据集 | 只读 verify 10000；候选 SHA-256 仍为 378068e317ca30787c3fea5bfbefa298a27ad4c430cfa149cd6eeccc8479d3c9。原有 201 内容均匹配，template／A／B 摘要吻合；当前候选到 STRM 的映射恰有 10000 个。没有重新补齐或生成媒体。 |
| 当前真实 OpenList | 样本 probe 退出 0，0.999 s：源根目录、201 文件，A 精确 Get／稳定字节一致、hash_info 缺失。候选比对退出 2，0.563 s：缺 bulk/ 等 9799。此预期拒绝证明工具检测到当前不完整，数据集验收仍 FAIL。 |
| 复用边界 | 匹配 FIX 回合的六份实际输入及候选摘要后，复用其七个旧平铺特殊名的成功 JSON；本轮只独立重读 A。匹配 IKlUvR 的原件／候选身份及 mtime 摘要，保留原件来源记录。固定环境／包身份只作此前只读描述参考，旧加载窗口不冒充当前原生任务可用性。 |
| BLOCKED／NOT RUN | 完整 S3 API 集合／内容与补传、最终清单固定 BLOCKED；最终 OpenList 万文件／B 样本、当前专用原生访问仍未完成。4.5 生成／同步／RSS、4.6 原始扫描／D-01..08、Slice Final 均 NOT RUN。代理／播放不在本合同。 |

没有因角色不同机械重跑相同的所有历史输入，也没有把全量交回 Developer。
未改变产品输入，不重跑 T4 race／容器／故障重启；所有 RO 尚未满足，不机械运行 Slice Final。
合同执行基线段仍保留重规划时“均未实施／NOT SET”原文，当前实际实现指针以 Slice 头部
e76a21c 和本 Task 报告为准；B 不擅改 A 已提交能力合同，事实协调项交最终收尾。

### Developer 修正验证与 B 返回门槛

Developer 在原 Task 修正 F-02：保存候选后预置精确 PID 临时名，既有 regular 对象／
静态链接均不覆盖、不删除；清单临时名作相同验证；创建失败及写入失败只清理本次确知
创建的临时对象，并保持正常补缺失、重复零写、冲突和中断重试。夹具仅放 artifacts
专用小目录，不拿真实媒体试错。保留 F-03／F-04 直接覆盖，快照与实际脚本同步，
记录新 Head、首败和重跑。F-05 如未处理明确留 P2，不伪报已覆盖同目标拒绝。

环境条件具备后沿用同一候选补足并核对自有 S3，保存完整对象清单／内容来源、最终
manifest／dataset_id／样本恢复入口；真实 OpenList 逐目录完整分页及 A／B／特殊名
Get／稳定字节匹配，全程 refresh:false，并确认专用 Jellyfin／固定插件原生入口可用。
凭据仅由当前专用进程环境注入；继续不可用则如实保留 BLOCKED。需要缩减 RO 或改变
凭据边界时由 A 决定，B 不自行改合同。

B 对新固定 Head 审完整原 Base..Head，负责本 Task 已指定 quality 全量、治理／语法／
空白、直接工具测试和上述关键独立复现；最终 S3 10000／最终清单／OpenList 集合及
必要样本独立复核。输入未变的证据准确匹配后复用；纯文档后继仅补治理／相关链接／
空白，不机械再跑已匹配的全量。Developer 不承担 B 的全量最终门禁。

### 全部 RO 重评与交接

| RO | 当前结果 |
|---|---|
| RO-1 | 部分：原件／脚本与固定环境来源可追溯；F-02 仍破坏既有对象，专用访问未完成。 |
| RO-2 | 未满足：本地候选 10000，S3 API／最终固定 BLOCKED。 |
| RO-3 | NOT RUN：4.5 未激活。 |
| RO-4 | NOT RUN：4.6 未激活；旧加载日志不替代新扫描。 |
| RO-5 | NOT RUN：D-01..08 当次实际决定未取得。 |
| RO-6 | 部分：本轮完整审查、指定 T3 全量及独立关键复现完成；后续真实验收／Final／A 未完成。 |

仍在同一 Task 修正；不生成 Closure Packet，不返回 SLICE READY FOR A REVIEW，不关闭 Slice。
测试计时将独立复现和本地全量摘要核对一并计入，约 15.2 s；本轮从 06:08:53 UTC 起，
截至完成门禁后已超过 130 s，测试占比小于 12%，低于用户 50% 限额。最终提交计时见
证据 INDEX／timing.json；没有空等凑比。
