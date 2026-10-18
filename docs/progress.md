# 进度

## 当前状态（2026-10-18）

B 已按已提交的 bfdceed5b3c564ca92aef47035dcced314f24d45 合同，独立审查完整 Task Base 15d749b3f151e94a6fb5e9850e6e21d7101aa603..14119b8389fdc76aa35df5ed7ba3e297810c11c5 及纯文档交接 abdc6e5。F-14 独立有界目录枚举修正有效；Task 1.2 仍 **FIX REQUIRED**：新复现 F-15（正常空／部分 staging 写入中断，发布二进制连续重试不收敛）及 F-16（Recover 交换／删除后未目录 sync，比较 skip 时错误提交 round）。细化修正步骤与测试映射已写 TASK.md，保持同一 Task／Base／High／T4，不新增任务，不改变 Slice／Roadmap／稳定需求；不要求抵御任意外部写者。Slice ACTIVE、A Final PENDING，无 Closure Packet。

独立结果：治理与 Go 五包 PASS；amd64 发布／review-only overlay 构建 PASS；既有三个 CLI 场景顺序复跑 3/3 PASS。F-15 Go 四组合与 SIGKILL 后发布二进制八次重试 FAIL，F-16 故障接缝反例 FAIL；未修改产品代码，确定性插桩仅位于 artifacts，不冒充无插桩命中。核对 Developer 原 T4（Python 48、race 174 测试＋8 包、0 fail／skip、all 9/9、双架构）；旧门槛未覆盖新反例，不代表 Task PASS。B 因阻断未重跑完整 quality／race／arm64／all。证据仅本机 artifacts/validation-evidence/task-1.2-b-convergence-review-20261018-bc87Qy/，包含 overlay／输入／首次 harness 失败与重跑；真实 OpenList、固定插件解析、Jellyfin／代理、ARM64 实机及性能仍 NOT RUN。下一步 Developer 修正 F-15／F-16、保留 F-14 并用新候选重跑完整 T4。

## 先前合同调整（历史事实）

用户进一步澄清：不要求完全保护任意外部写者，只要求正常单写者使用及后续同步收敛。A 本轮同步修订需求、架构、Slice、说明与测试矩阵，撤回 `66facd2` 中追加的 UID／mode 强制检查、消费者 OS 拒写实验及外部注入状态永久冲突门槛；原始方案与 Base 不变。F-14 与正常中断后重试收敛仍须修正／验证，真正未归属保护、路径边界、dry-run 和完整 T4 保留。只有文档修订，没有产品改动／新 PASS；Task 1.2 仍 FIX REQUIRED，Slice ACTIVE，A Final PENDING，无 Closure Packet。A 合同已先提交为 `bfdceed5b3c564ca92aef47035dcced314f24d45`；B 随后判定 Task 1.2 合法原地调整，更新 Contract Checkpoint，重写当前执行清单与明确旧／新测试映射；旧详细修复流水账保留在可达 `483069c`。

本轮文档验证：原方案 SHA-256 未变，Python 工具回归 48 项通过；未提交合同、A 已提交而旧 Task 尚未更新两次治理拒绝均符合预期。Task 更新后常规治理、diff check 及再次执行的 Python 工具回归 48 项均 PASS；此处 PASS 仅指文档／工具检查，不是产品或独立审查通过。证据仅本机 `artifacts/validation-evidence/slice-1-convergence-contract-20261018-01/`；Go／race／CLI／外部服务在本轮 NOT RUN，未把旧产品证据写成新合同 PASS。

## 历史检查点与证据（旧要求已由上述澄清取代）

- A 合同检查点为 `66facd2ecf02fcd8f296e4026025ab1468a731af`；B 随后判 Task 1.2 合法调整、继续原地修正，已同步活动 Task checkpoint／Scope／AC／旧新测试映射与逐步实施清单。Task Base、High／T4 和 FIX REQUIRED 不变，无产品改动或新 PASS。单写者合同提交后旧 Task checkpoint 曾被治理正确拒绝，B 更新后常规治理恢复 PASS；原方案 SHA-256 未变，Python 工具回归 48 项通过，产品新合同权限／恢复验证 NOT RUN。实现者下一步按 TASK 进入修正；不修改生产权限或服务。

- 用户授权本会话行使 A 权限并限制外部写者：已批准需求 SW-1..SW-6 单写者部署合同，生成器独占输出／暂存／状态写入，消费者只读，生成器共享同一锁资源；范围贯穿轮次间与恢复。不承诺抵御同 UID／root 或任意不合作写者，不追认历史失败为 PASS。原方案、Slice Base／ACTIVE、SQLite 边界不变；本轮仅协调合同与文档，不修改生产权限或产品实现。A 合同提交后由 B 合法调整 Task 1.2 及具体测试映射，状态仍 FIX REQUIRED；已知权限条件、F-14、未知恢复事实跨重启保护与完整 T4 尚待实施验证。以下 B 记录是旧合同下的历史事实，失败证据保留。

- 最新 B 独立复审（报告检查点 `e474e64`，候选 `d0e2bacfb824b4876c647059e43c2996a9339417`）：Task 1.2 继续 **FIX REQUIRED**，不新建 Task。原双替换／提前暂存替换／首次合法外部恢复通过；F-10 检查后按名清理仍删除不同存活 inode 的外部对象，F-11 第二次发布重启覆盖外部 STRM，F-13 最后证明后的替换仍误发布，新增 F-14 为根目录 dup 共享游标漏检后续文件残留并错误提交 round。聚焦五包、amd64 发布／testhooks 构建、三个既有 CLI 场景顺序执行通过；新反例失败证据在 `artifacts/validation-evidence/task-1.2-b-rereview-20261018-02/`。核对并复用 Developer T4：Python 48、race 163 测试／8 包、隔离 9/9、双架构；B 本轮未重跑完整 quality／race／arm64。详细技术修复清单见 TASK.md；Slice ACTIVE、无 Closure Packet，合同／Roadmap／需求不变。以下较早状态保留为历史。
- Slice 1 仍为 ACTIVE，Base 为 `f90f9560e55b07fd2d6a20e8bac43741c39176c4`；没有 Slice PASS 或 Closure Packet。
- Task 1.1 经 B 独立复审 PASS；Task 1.2「已拥有 STRM 内容更新与原子恢复」修正候选 `f4383f2f59adedadc9252e465c64580570a3bbcf` 经本次独立 B 复审仍为 **FIX REQUIRED**。F-10 检查后按名清理仍会删除外部对象；F-11 恢复出的合法外部 STRM 被再次覆盖并提交 round；新增 F-13 暂存对象替换可导致误发布或失败清理误删。Task Base、合同与 T4 不变，继续原地修正。
- B 用仅加入暂停/显式放行屏障的 Go overlay 确定性复现:F-10 回滚会将第二个外部目标换入 tmp 并删除;F-11 交换校验前 SIGKILL 后,发布二进制重启错误地跳过新目标、提交 round 2,陌生原目标留在隐藏 tmp。F-12 要求补齐目录 fsync、发布/回滚失败与更新路径信号窗口证据。详情和步骤见 TASK.md;原始复现见 `artifacts/validation-evidence/task-1.2-b-review-20261018-03/`,独立已有场景 3/3 与聚焦包/构建 PASS 证据见 `task-1.2-b-review-20261018-02/`。未将受控插桩写成未插桩二进制命中窗口。
- 2026-10-18：Developer 提交 `f4383f2f59adedadc9252e465c64580570a3bbcf` 后，B 核对完整 Task Base..Head 及纯文档后继，独立聚焦 Go 五包、amd64 发布／testhooks 构建、URL／SHA-1／Size 更新、零写、dry-run 与既有故障恢复最终 PASS；F-10 原双替换序列 PASS，新反例为三个 CLI FAIL 与两个确定性注入子测试 FAIL。证据在 `artifacts/validation-evidence/task-1.2-b-rereview-20261018-01/`；首轮并行启动验收驱动造成的 busy 保留，顺序重跑通过，不计产品缺陷。复核 Developer T4：Python 48、race 160 个测试／8 个包、隔离 CLI 9/9 和双架构构建通过，输入相同可复用，但未覆盖新失败，B 本轮未重跑完整 T4。F-12 新增故障注入及三个信号窗口的覆盖事实保留；修正候选未获 PASS，不创建新 Task 或 Closure Packet。
- Task 1.2 原候选 T4 证据保留在本机 `artifacts/validation-evidence/task-1.2-final/`（race 原始 JSON 为 147 个测试、8 个包通过，0 fail／skip；原报告的 155 混计包事件；隔离 CLI 9/9、双架构交叉构建与 testhooks 构建通过）。这些旧用例未覆盖本次失败窗口。首轮夹具错误及 B 的构建身份／屏障准备失败均保留原始记录；没有 Task 1.2 PASS 或 Closure Packet。
- 前一候选 T4 证据位于本机 `artifacts/validation-evidence/task-1.1-fix-20261018-05/`（race 134 pass）；Task 1.1 F-9 修正的权威 T4 证据位于 `artifacts/validation-evidence/task-1.1-b-review-fix-20261018-02/`（race 142 pass、隔离 CLI 9/9、故障恢复聚焦 3/3）。`task-1.1-fix-20261018-04/` 保留一项首轮断言失败及修正前后证据，不覆盖、不隐藏。
- SIGKILL 恢复场景观察到一个未引用的 `.strmgen-*.tmp`；已验证已有 STRM／归属／完整轮次恢复断言，恢复文档要求先核实而非盲目清理。真实外部 OpenList／无签名 `/d/`、固定插件解析、Jellyfin／代理、ARM64 实机及性能仍为 NOT RUN；无生产服务、媒体或配套仓库测试。
- 历史 B 独立复审证据位于 `artifacts/validation-evidence/task-1.1-b-review-20261018-02/`：首次完整隔离复跑在归属中断恢复处失败，随后单独 `fault_recovery` 重跑通过；失败由 SIGSTOP 落在“发布后、归属登记前”窗口导致，程序按合同正确保持未归属冲突，但驱动断言不稳定。该问题已在同一 Task 内改为确定性注入，并由后续 T4 证据闭环。
- 2026-10-18：Developer 在同一 Task 内修正 F-9：新增仅 `strmgen_testhooks` 构建可见的 `afterOwnershipRecord` 确定性屏障，进程在 `RecordOwnership` 成功后写标记并 `SIGSTOP`，驱动不再轮询；发布二进制为无环境开关的空实现（`strings` 核对无 `STRMGEN_TEST*`）。`fault_recovery` 聚焦重跑 5/5、隔离 CLI 9/9、race 142 pass 全 PASS，实现检查点 `15d749b3f151e94a6fb5e9850e6e21d7101aa603`；B 独立复审后 Task 1.1 PASS，Task 1.2 已规划，Slice 仍 ACTIVE，无 Slice PASS 或 Closure Packet。

## 开发准备

2026-10-17：按用户要求保留原始V4.3方案，复用配套项目Slice／Task流程，建立需求、用户旅程、架构、兼容性、测试矩阵、四个Slice路线、规划合同及角色指南。创建本地Go环境工具、合成OpenList夹具、配置示例和CI定义。

已验证：Go 1.27.2 本地安装及版本检查；标准库样例运行；Linux amd64／arm64 编译 smoke；29项Python工具回归；文档身份／链接／规划治理、Python／Bash语法与全部交付文件空白检查。原始V4.3内容SHA-256保持不变。

Compose夹具已在专用 `strm-generator-dev` 项目启动并通过health检查，地址 `http://127.0.0.1:15244`；容器端list分页、应用层500、HTTP503及合成不存在语义已验证。夹具仍运行，停止命令见环境说明；未访问或修改同机生产Jellyfin／OpenList。

用户补充Git身份后，已仅在本仓库配置 `kentinya <szd123956@outlook.com>`，开发准备文件保存为main的首个本地基线提交。真实完整提交号通过 `git log --reverse --format='%H %s'` 查看；未配置远程，未推送。基线提交后治理使用常规模式。CI定义尚未在GitHub运行，环境准备不计为产品Slice通过。

## 初始指针（历史，2026-10-17）

2026-10-17：用户明确要求激活第一个Slice；A提交Slice 1「安全的单轮生成闭环」ACTIVE合同及同步Roadmap，固定开发准备基线 `f90f9560e55b07fd2d6a20e8bac43741c39176c4`。本次只激活合同，没有产品实现或审查结论。

- [Slice 1](../SLICE.md)：ACTIVE；Base为 `f90f9560e55b07fd2d6a20e8bac43741c39176c4`，Implementation Head为NOT SET。
- [Task 1.1](../TASK.md)：PLANNED；首次生成、归属登记与安全重复运行，Task Base与Contract Checkpoint均为 `951aaeeeca966e8ef79e8ac9500827ce27d0f035`。尚无Developer实现，产品审查PENDING。
- 产品Go测试／构建、真实OpenList、插件解析、Jellyfin／代理、压力、产品发布：NOT RUN。
- B独立审查与A产品最终审查：PENDING。
- 下一步：Developer按Task 1.1实施并提交真实验证证据；B审查后重新核对全部RO。已拥有文件内容更新及恢复、真实OpenList／固定插件解析等缺口仍未完成，不预设Slice通过。

初始工具证据仅本机保存在 `artifacts/validation-evidence/preparation-20261017/`；Git身份配置、基线与常规治理验证保存在 `artifacts/validation-evidence/git-baseline-20261017/`；合同激活及提交后的治理／质量检查保存在 `artifacts/validation-evidence/slice-1-activation-20261017/`。旧证据中的无HEAD／身份缺失状态保留为当时事实，SDK与运行证据按可执行输入身份复用。后续Task／审查历史保存在可达Git检查点。

2026-10-17：B完成必读与实际仓库核查，规划前常规治理PASS，提交Task 1.1详细实施／验收／T4测试计划；规划验证证据见 `artifacts/validation-evidence/task-1.1-planning-20261017/`。本次没有产品实现或独立产品审查PASS，Slice／Roadmap／稳定需求保持不变。

2026-10-17：Developer按Task 1.1实现首个产品闭环并提交检查点，状态置为READY FOR B REVIEW（待提交）。交付：`cmd/strmgen` 的 `sync`／`sync --dry-run`；`internal/config` 严格YAML校验；`internal/openlist` 有界list/get与分页／应用层错误；`internal/pathmap`／`internal/strm` 纯路径、稳定URL、版本1协议与比较；`internal/storage` 用 `modernc.org/sqlite v1.60.1` 记录最小归属与完整轮次，dry-run对WAL库读私有快照不产生sidecar；`internal/reconciler` 有界遍历、同目标冲突、原子发布（同目录临时文件 + 不覆盖link）与跨进程flock互斥。依赖 `gopkg.in/yaml.v3 v3.0.1`；race门槛使用仓库固定Zig `cc`。

本机已执行（证据见 `artifacts/validation-evidence/task-1.1-final/`）：治理、mod download／verify、quality（gofmt／vet／Go test／build＋Python回归）、race、Linux amd64／arm64 CGO=0构建、以及隔离CLI全场景驱动（首次、重复与比较、归属与路径、API与限额、dry-run边界、多进程、故障恢复）全部PASS。产品未完成：已拥有文件内容替换、Mirror删除／移动、serve／定时、发布镜像；真实OpenList、固定插件解析、Jellyfin／代理、ARM64实机与性能仍NOT RUN。B独立审查PENDING，不预设Slice通过。


## B 审查记录（2026-10-18）

B 已读取仓库约束／必读文档并在审查前取得常规治理 PASS；独立审查 Task Base 951aaeeeca966e8ef79e8ac9500827ce27d0f035 至 Developer Head 4752ec2057f998d762829cdb352c8b75198ee900，核对纯报告后继和本机原始 quality／race／构建证据。现有实际 CLI 七类场景独立重跑 7/7 PASS，但新增隔离反例复现了根／祖先符号链接逃逸、锁命名空间分裂及预览锁写输出、多 source 归属覆盖、DB URI 路径误解、错误列表形状提交成功轮、可选 Size 阻断、诊断泄露、FIFO 阻塞及相对路径语义不符。

Task 1.1 为 **FIX REQUIRED**，详细 F-1..F-8 修正与复验步骤在 TASK.md；修正保留同一 Task／Base／T4，不新建 Task。Slice 1 仍 ACTIVE，Implementation Head 仍指向被审候选；RO-1..RO-5 均未全部满足，无 Closure Packet 或 A 最终 PASS。本次 B 未实施产品代码，未改 Slice／Roadmap／稳定需求，未触及配套仓库及生产服务，未推送。早先 PLANNED／NOT SET／PENDING 为历史记录，以本节及活动 Task 为当前状态。

审查证据仅本机：artifacts/validation-evidence/task-1.1-b-review-20261018/（index.json、原始日志、隔离输出／SQL、反例脚本）。B 未重跑完整 quality／race／双架构构建，发现阻断后先退回；Developer 修正后须用新证据目录重跑完整 T4。真实 OpenList／固定插件解析、Jellyfin／代理、ARM64实机、性能仍 NOT RUN。下一步：Developer 在 Task 1.1 增量修正并重新送 B 审查。
