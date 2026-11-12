# Task FIX-TG-01 — Telegram 通知排版与图标

Slice 1–6 均已 PASS / CLOSED，历史结论见 [Slice 6 Closure Packet](SLICE.md#closure-packet)。
独立小版本修复 `FIX-TG-01` 已完成本地实现与独立 B 审查，结论为 FIX REQUIRED。
本地验收通过；剩余交付是 TG-6 的新版真实客户端视觉与接收证据，开发指导见本页审查报告。

## 独立修复 Task FIX-TG-01 — Telegram 通知排版与图标

```text
Task ID: FIX-TG-01
Task Type: standalone-fix
Parent Slice: NONE
Related Slice: 6
Status: FIX REQUIRED
Task Base: 959451530efbfe8a646f80ce325b4f789f85e770
Contract Checkpoint: e1f7bb5de5c61953bda42c58721530842f982a94
Implementation Head: 57d1436456189576e740b4f112a43bd0b8cd94b7
Difficulty: Medium
Test Level: T3
Independent B Review: FIX REQUIRED
Real Telegram Visual Check: NOT RUN
```

用户在 2026-11-12 要求改善 Telegram 通知外观、增加图标，并规划一版独立修复 Task。
本任务是通知展示的小版本修复，独立于已完成的 Task 6.1，不重开 Slice 6、不要求新的 A
大版本合同；后续产品审查仍须独立 B 完成，实施者不能自签独立 PASS。
Base 固定为规划前真实 HEAD；Contract Checkpoint 指向已提交的本计划，提交前为 NOT SET，
不使用旧 Slice 的 ACTIVE 提交冒充新展示约定，也不预填实现 Head。

### 问题、目标与依据

现有 `internal/notify/message.go` 将九项统计排成一长行，标题、时间、结果没有视觉层级，
最多二十条文件事件全部展开；`internal/notify/sender.go` 只传 `chat_id` 和 `text`，
没有设置 `parse_mode`。用户提供的既有接收截图能说明原版样式，不能作为新版验收证据。

目标是让用户先看清结果和实际变更，再按需展开长明细。采用 Telegram 原生 HTML 文本格式、
少量固定 Unicode 图标与可折叠引用块；不引入图片生成、外部字体或模板依赖。
参考已查阅的 [Telegram HTML 格式](https://core.telegram.org/bots/api#html-style)及
[sendMessage](https://core.telegram.org/bots/api#sendmessage)，接口文档不代替真实接收证据。

### 展示约定与兼容边界

本计划显式提出一项展示合同修订：将 Slice 6 历史合同的“中文纯文本正文”改为
“中文 HTML 格式正文”。实施时在 [需求 R-14](docs/requirements.md)、README 和架构中
记录此次增量及长度口径；旧 [SLICE](SLICE.md) 的原合同、Closure Packet、Base、Head、
审查决定和真实送达记录保持原样，不把富文本能力追认为旧版已验收。

1. **结果置顶。** 使用粗体中文标题，成功为 `✅ STRM 同步完成`，部分来源未完成为
   `⚠️ STRM 部分来源未完成`，全部来源未完成及初始化失败使用 `❌` 和对应明确文字。
   继续使用已有 Outcome 判断，冲突不能因处理错误为零而被标成成功。
2. **固定图标、保留文字。** 实例 `🏷️`、来源 `📂`、耗时 `⏱️`、时间 `🕒`、
   文件事件标题 `📋`、附件 `📎`；不单独靠颜色、图标或 Premium 自定义 emoji 传递含义。
   九项统计使用下表图标，分成短行，每行至多三项，保留零值及原计数含义。
3. **信息完整。** 正文保留实例、sync／serve、来源总数与完成／未完成数、同步耗时、
   完成时间，以及原本可用的开始时间；时区明确，不读取新的时区配置或假定 UTC+8。
   初始化失败保留已知来源总数，完成数及统计标为“不可用”，不渲染九个零冒充报告。
4. **明细适量折叠。** 以正文实际入选的事件预览条数为准，不超过五条时直接展示；
   六条及以上使用一个 `<blockquote expandable>` 包住文件预览。标题和正文预览／省略说明在块外，
   超量仍最多二十条。折叠只改变显示，不减少所计入的正文长度，不替代附件，也不依赖
   Bot 按钮、回调或 webhook。客户端即使展开全部内容也应顺序可读。
5. **异常优先可见。** 已有来源失败／冲突／安全保留诊断在事件预览之前展示，不放入
   折叠块；诊断继续采样、有界，装不下的来源明细进入同一附件，并显示省略数。
   明细不可用、写入失败或达到大小上限的警示始终在折叠块外。
6. **事件身份和顺序。** 每条保留操作文字、原 source 编号及输出根内完整相对路径，
   路径可用 `<code>` 呈现；不改成 basename、不重编号、不按图标分组排序或合并事件。
   路径过长可整条移到附件，不能用截断路径冒充完整身份。
7. **清理事实清楚区分。** 待删计数推进用 `⏳`，恢复清零用 `↩️`，实际删除用 `🗑️`，
   归属记录回收用 `♻️`；保留已提交前后值、阈值、固定原因和原阶段说明。
   `2→3/3` 必须说明等待后续清理重核，不能显示成已删除；归属回收不能冒充文件删除。
8. **附件说明真实。** 仅在确有可用待上传清单时显示附件说明；摘要阶段使用“清单随附上传”
   等措辞，不声称附件已经送达。全部事件已容纳时不为美观强制发附件；写失败不得承诺完整。
   清单继续为原 UTF-8 纯文本，HTML 标签和实体不能污染附件中的普通文件名或诊断。

| 统计／操作 | 固定图标与文字 |
|---|---|
| 新增、更新 | ➕ 新增；✏️ 更新 |
| 实际删除、归属回收 | 🗑️ 实际删除；♻️ 归属回收 |
| 未变化跳过、扩展名过滤 | ⏭️ 未变化跳过；🔎 扩展名过滤 |
| 安全保留、冲突、处理错误 | 🛡️ 安全保留；⚠️ 冲突；❌ 处理错误 |
| 已提交计数推进、恢复清零 | ⏳ 待删除计数；↩️ 恢复保留 |

正常二十五条新增的排版示意如下，数值仅为合成样例；文件预览在真实客户端中折叠，
省略说明与附件说明不折叠。所有运行字段取实际 RunInfo，不以示例值作为默认值。

```text
✅ STRM 同步完成                         [粗体标题]
🏷️ 家庭媒体库 · sync

➕ 新增 25 · ✏️ 更新 0
🗑️ 实际删除 0 · ♻️ 归属回收 0
⏭️ 未变化跳过 0 · 🔎 扩展名过滤 0
🛡️ 安全保留 0 · ⚠️ 冲突 0 · ❌ 处理错误 0

📂 来源 1 · 完成 1 / 未完成 0
⏱️ 同步耗时 403 毫秒

📋 文件事件                             [粗体小标题]
  ➕ 新增 · 来源 0 · 样例 01.strm          [以下为可展开引用块]
  ➕ 新增 · 来源 0 · 样例 02.strm
  ……其余入选预览，合计不超过 20 条

正文预览 20 条，另有 5 条未展示。
📎 完整清单 25 条，随附上传。
🕒 开始 2026-11-12 13:33:44 UTC
🕒 完成 2026-11-12 13:33:45 UTC
```

### 技术范围与不变量

- 产品改动以 `internal/notify/message.go`、`sender.go` 及其测试为主；可按需调整
  `notify.go` 中的展示辅助函数／注释。HTML 转义属于正文展示层，不能直接把 Collector
  共用的纯文本 `EscapeText` 改成 HTML 转义。`cmd/strmgen/notify.go` 仅允许必要的消息交接调整。
- `sendMessage` 固定发送 `parse_mode=HTML`，只生成 `<b>`、`<code>`、
  `<blockquote expandable>` 所需的受控标签，不支持配置或文件名传入任意 HTML。
  所有动态实例名、路径和诊断先按原规则转义控制／换行／双向控制字符，再转义 HTML
  的 `&`、`<`、`>`；例如字面文件名 `&lt;b&gt;.strm` 显示后仍保持该字面值。
- 沿用 `BodyMaxUTF16Units=3500`，采取保守口径：**完整待发送 HTML 字符串连标签和实体
  也不得超过 3500 个 UTF-16 code units**，因此解析后正文同样受限。emoji、标签、
  小标题、块闭合及省略／不完整说明均占预算；最多二十条不是保证一定能容纳二十条。
  在动态文本和完整区块层分配预算，禁止对最终 HTML 串硬截断、留下半个实体／标签或拆开
  Unicode 字符。超长合法实例名仍由展示层有标记地缩短，不新增配置拒绝规则。
- 所有未入选正文的事件与来源明细须在可用附件中保留；附件仍单个、流式、最多 8 MiB。
  选取／退回区块、补写来源明细、Finalize 和完整性提示的顺序必须一致，不能在清单
  已结束后丢弃正文来源明细又漏掉附件补写。不把折叠的事件算成未预览事件。
- 不改 Collector 的事件事实、暂存／提交／撤销及附件持久语义，不改 reconciler、storage、
  STRM 内容、数据库 schema、清理资格／计数／时机、同步退出码或调度。需要触及这些内容时
  应先重新评估范围与 T4 要求，不能夹带在排版修复中。
- 通知默认关闭、三策略、dry-run／busy／取消静默、锁外串行发送、共用三十秒预算、
  单条摘要及可选单附件、无重试、TLS／重定向／响应上限、凭证与路径脱敏保持。
  Telegram 拒绝 HTML 时只走既有脱敏降级，不追加纯文本重发或另发状态消息。
- 不新增配置项、外部依赖、交互按钮、链接、媒体封面、图片通知、模板系统、通知历史或队列。

### 独立任务的治理接入

`scripts/check_governance.py` 为独立小版本修复提供 `standalone-fix` 路径，规则见
[开发流程](docs/development-workflow.md#任务与审查)。规划阶段可保留
`NO ACTIVE IMPLEMENTATION TASK` 和 NOT SET 检查点；这只表示规划有效，不授权产品实施。
激活时移除无活动标记，固定已提交 PLANNED 计划的真实检查点；校验 Task Base 的祖先关系、
任务类型／状态、与检查点一致的完整范围及已关闭关联 Slice 的原文。
后续执行证据写入 `Task Report`，验收表保留规划时状态；不得在报告外悄悄调整验收范围。
新大版本仍走 A 合同／B Task，原数字子任务的父级、合同一致性和关闭校验不变。
治理回归须覆盖合法／非法 Base、未提交或不匹配计划、父级混用和旧路径；不得继续写
“无活动”而实际开始产品实施，不另建通用任务框架。

### 验收条件

| ID | 必须满足的结果 | 计划中的证据 | 规划时状态 |
|---|---|---|---|
| TG-1 | 四种结果标题、九项分行统计及固定图标正确；未知计数不伪造零 | 渲染用例与合成样例 | NOT RUN |
| TG-2 | 五条直接显示、六条起折叠、最多二十条；来源、顺序、路径、计数原因及阶段完整 | 边界用例、CLI 清理进度回归 | NOT RUN |
| TG-3 | 所有动态字段防 HTML／换行／双向控制注入；长实例、长路径及 emoji 下完整 HTML ≤3500 UTF-16，标签与实体完整 | 转义、结构和长度边界用例 | NOT RUN |
| TG-4 | 来源／事件溢出使用原单清单；无事件、多来源、收集不可用、写失败和超限提示真实且在折叠块外 | 原清单失败回归与新增格式断言 | NOT RUN |
| TG-5 | HTML 请求参数正确；摘要／附件失败无重试且隔离；开关、策略、锁、取消、产物和数据库结果不变 | 传输、tagged CLI、sync／serve 通知场景 | NOT RUN |
| TG-6 | 新版真实客户端中图标、粗体、折叠和特殊文件名正确，超量清单实际接收且与运行匹配 | 新授权的隔离 Bot／聊天接收截图、运行身份及附件 SHA-256 | NOT RUN |
| TG-7 | 独立治理接入与当前文档一致，固定 Base／计划检查点／实现 Head，独立 B 全范围审查 | 治理回归、T3 报告、独立 B 决定 | PENDING |

### 验证计划与证据边界

规划维护按 T0 检查治理、链接和补丁空白；治理接入另跑受影响的 Python 回归。
产品实现的 T3 验证如下，不能将规划／激活检查写成产品或新版真实渲染 PASS。

1. **聚焦检查。** 扩展现有 `internal/notify` 渲染和发送测试，保留原业务断言，
   改为同时核对 HTML 结构／转义及解析后可见文本。覆盖正常／无变化、部分／全部失败、
   初始化失败、六种事件、五／六／二十／二十一条、中文／emoji、长实例／路径／诊断、
   `&<>`／引号／伪标签／字面实体、混合原因、多来源溢出及明细故障。
   断言失败和不完整提示在可折叠块外；补发不发生，摘要失败不继续附件。
2. **传输与 CLI。** 沿用现有 mock，扩展 `scripts/telegram_receiver.py` 以记录
   `parse_mode`，在测试中核对原始 HTML 和实际可见文字，mock 不伪装为 Telegram 渲染器。
   运行以下现有入口；证据目录均新建在 `artifacts/validation-evidence/`。

   ```bash
   bash scripts/go-local test ./internal/notify ./cmd/strmgen
   bash scripts/go-local test -tags strmgen_testhooks ./cmd/strmgen
   bash scripts/go-local build -trimpath -o artifacts/bin/strmgen ./cmd/strmgen
   bash scripts/go-local build -tags strmgen_testhooks -trimpath -o artifacts/bin/strmgen-testhooks ./cmd/strmgen
   python3 scripts/run_isolated_sync_integration.py --binary artifacts/bin/strmgen --fault-binary artifacts/bin/strmgen-testhooks --scenario notifications,notifications_cleanup --output-dir "$TG_SYNC_RUN_DIR"
   python3 scripts/run_serve_integration.py --binary artifacts/bin/strmgen --fault-binary artifacts/bin/strmgen-testhooks --scenario serve_notifications --output-dir "$TG_SERVE_RUN_DIR"
   ```

   `TG_SYNC_RUN_DIR`／`TG_SERVE_RUN_DIR` 在执行时分别用 `mktemp -d` 创建，不复用旧产物。
   检查通知开／关的 STRM 字节、归属、轮次和缺失计数等价；现有通知相关业务断言继续执行，
   不能只改预期文案后删除安全断言。
3. **候选质量。** 运行 `python3 scripts/quality.py`，包含治理／Python／gofmt／vet／
   全包 Go 测试／tagged CLI／build；运行受影响 `internal/notify`、`cmd/strmgen` 的
   race 检查，使用仓库固定 C 工具链。完整套件已覆盖聚焦检查时不机械重复。
   未改存储／删除接口时不要求重跑真实 OpenList、Jellyfin、插件或固定万文件链路；
   仅展示改动不增加容器全套和 ARM64 原生实机门槛。
4. **客户端视觉。** 保存可核对的合成 HTML／可见文本样例。真实验证仅在届时明确授权的
   专用 Bot／聊天中，从正常产品 CLI 发送最少两类通知：含二十五条事件及特殊字符文件名的
   成功轮（正文加单附件）、有来源诊断的失败轮。核对标题／统计／路径未显示裸标签，
   折叠前后内容和展开操作正确，警示可见，附件条目与运行一致；记录客户端／版本、
   脱敏截图、运行身份和接收附件 SHA-256。
   本轮没有发送授权，不读取旧凭证或复用旧真实接收结果；缺条件时保持 TG-6 NOT RUN／
   整体验收 PENDING，可先提交本地结果供 B 审查，不将 mock／本地 HTML 预览写成真实 PASS。
5. **交付与独立审查。** 实施者记录准确 Base..Head、未提交输入、命令／退出码、
   原始日志／产物摘要、首次失败及重跑、逐项验收证据，提交状态为 READY FOR B REVIEW。
   独立 B 核对整个范围与新视觉证据后决定 PASS 或 FIX REQUIRED；小版本修复不追加 A
   验收。文档与审查完成后结束本独立任务，原 Slice 历史结论不变。

## Task Report

### 激活记录（2026-11-12）

用户明确要求激活本 Task。计划检查点为 `e1f7bb5de5c61953bda42c58721530842f982a94`，
Task Base 保持不变；独立任务校验与流程已接入，状态进入 IN PROGRESS。
本轮只完成激活前置工作，Implementation Head 仍为 NOT SET；通知格式、发送器和同步逻辑
未改，TG-1..6 产品验证未运行，TG-7 的独立 B 审查保持 PENDING。
Slice 1–6 的状态、历史合同、实现 Head 和验收决定保持原样。

验证证据位于 `artifacts/validation-evidence/fix-tg-activation-0757z5nc/`：

- Python 回归按全部七个测试模块执行，合计 **125/125** 通过，其中治理 **31/31**；
  命令、逐模块数量与退出码见 `python-summary.json`，原始输出见各 `test_*.log`。
  所有临时夹具均位于本仓库 artifacts 内。目录边界用例的正／负夹具要求不同，
  OpenList 驱动单元使用 artifacts 内的非证据临时目录，其余模块使用本轮证据目录。
- 首次治理运行 `governance-tests.log` 有一项失败：无 HEAD 夹具位于仓库下时向上发现父库；
  已在该用例显式初始化空仓库，保留原断言后通过。首次全套
  `python-regressions.log` 和再次全套 `python-regressions-rerun.log` 分别有一项失败、
  三项错误，原因均为统一 TMPDIR 与既有目录边界夹具的假设冲突；改为上述分模块目录后
  所有用例通过，未删除、跳过测试或放松产品目录边界。
- 计划提交前治理、文档链接、Python 语法及补丁空白检查通过；激活提交后的检查与
  实际提交身份记录在 `activation-checks.log`／`activation-evidence.json`。
- Go 产品回归、CLI／serve 产品集成、race、真实 Telegram、OpenList、Jellyfin、
  插件与容器运行本轮 NOT RUN；Python 中模拟的缺服务诊断不是实际服务运行结果。
  激活检查不代替本计划的 T3 产品验证或独立 B 审查。

### 实施报告（2026-11-12）

按激活计划完成 FIX-TG-01 的产品实现与受影响验证，状态提交为 READY FOR B REVIEW。
Task Base 与 Contract Checkpoint 保持不变；Implementation Head 为
`57d1436456189576e740b4f112a43bd0b8cd94b7`（含折叠块内不产生空首行的展示修正与超限提示回归）。独立 B 审查仍为 PENDING；本轮没有真实
Telegram 发送授权，因此 TG-6 为 NOT RUN，整体验收不能由以下本地／合成证据代替。

实现范围：

- `internal/notify/message.go` 改为受控 HTML 渲染：四种结果粗体标题、固定图标、
  九项统计分行、实例／来源／耗时／开始完成时间、来源诊断先于事件、5 条直接展示／
  6 条起单个 `<blockquote expandable>`、最多 20 条预览、正文预览／省略与附件说明。
  动态值先做原控制／换行／双向字符转义，再做 `&`、`<`、`>` HTML 转义；完整待发送
  HTML 连标签和实体共享 3500 UTF-16 预算，长实例名按完整 rune／实体缩短，路径不截断。
- `internal/notify/sender.go` 的 `sendMessage` 固定传递 `parse_mode=HTML`；其余传输、
  预算、重定向、响应上限、无重试与脱敏逻辑未改。
- Collector、reconciler、storage、STRM／清理／调度、配置 schema 与 Slice 6 历史合同未改。
- README、需求 R-14、架构与路线图记录本次 HTML 增量及长度口径；旧 SLICE 原文未改。
- `scripts/telegram_receiver.py` 增加 `parse_mode` 记录与测试用可见文本读取；sync／serve／
  container 集成断言改为同时核对原始 HTML 与可见文字，未删除原安全断言。

验证与证据（本机，T3）：

| 检查 | 命令 | 结果 |
|---|---|---|
| 聚焦格式化／发送／CLI 测试 | `bash scripts/go-local test ./internal/notify ./cmd/strmgen` | PASS |
| tagged CLI 测试 | `bash scripts/go-local test -tags strmgen_testhooks ./cmd/strmgen` | PASS |
| sync 通知与清理实 CLI | `python3 scripts/run_isolated_sync_integration.py --scenario notifications,notifications_cleanup` | 2/2 PASS |
| serve 通知生命周期 | `python3 scripts/run_serve_integration.py --scenario serve_notifications` | PASS |
| 受影响包 race | 固定 CC／CGO=1，`go-local test -race -count=1 ./internal/notify ./cmd/strmgen` | PASS |
| 常规质量 | `python3 scripts/quality.py` | PASS（治理、Python 125/125、语法／空白、gofmt、vet、全包 Go、tagged CLI、build） |

实现 Head 之后有一次展示修正：`blockquote expandable` 开标签不再带入空首行，事件预览
紧跟标签内容；同时补充了附件达到大小上限时“提示在折叠块外且不承诺完整”的回归。
聚焦 Go 测试、sync／serve 通知集成与受影响 race 已对最终源码重跑 PASS。

证据位于 `artifacts/validation-evidence/fix-tg-01-sync-R7W9DH/`：`run.log`、`serve.log`、
`race.log`、`quality.log`、`sample.log`（合成 25 条事件的原始 HTML 与可见文本），以及
驱动各自输出目录与 `summary.json`。构建产物为 `artifacts/bin/strmgen` 与
`artifacts/bin/strmgen-testhooks`。

首次失败与修正：

- 原测试仍断言纯文本标题／无标签事件，改为同时核对 HTML 结构与解析后可见文字；
  没有删除业务或安全断言。
- 诊断顺序用例首次复用了已 Finalize 的 collector，关闭文件后未触发新失败；改为独立
  故障 collector 后通过。
- 长路径用例的合成路径仍能容纳于正文，改为更长路径后验证整条移入附件。
- 单来源失败被正确地归为“全部来源未完成”，测试预期由部分完成改为该标题。
- 首次 `quality.py` 在实现提交前因 `docs/roadmap.md` 尚未提交而被治理拒绝
  （`Working roadmap differs from committed HEAD`）；提交实现后同一质量套件 PASS。
  这是工作区提交顺序问题，不是产品失败。

未运行与限制：

- 真实 Telegram 客户端视觉检查、真实发送与接收附件核验：NOT RUN；需另行授权的
  Bot／聊天。mock 只记录请求，不证明 Telegram 的 HTML 渲染、折叠展开或 emoji 宽度。
- 容器全套、真实 OpenList／Jellyfin／插件、ARM64 原生与 T4 恢复链路本轮未重跑；
  本次未改持久化、删除、清理资格、STRM 字节或调度接口，按计划采用受影响回归。
- 未提交输入：实现提交前无额外未提交文件；本报告在实现提交后追加，审查范围应覆盖
  实现 Head 与本报告提交。剩余阻塞为 TG-6 真实视觉证据与独立 B 决定。

### 独立 B 审查（2026-11-12）

```text
Reviewer: B（本轮独立审查，未参与被审产品实现）
Decision: FIX REQUIRED
Reviewed Range: 959451530efbfe8a646f80ce325b4f789f85e770..f8a8d7b12bd7781736b2ed7f73771475ad40d5e3
Reviewed Implementation: 57d1436456189576e740b4f112a43bd0b8cd94b7
Blocker: B-TG-01 — TG-6 新版真实客户端视觉与附件接收尚未验证
Visual Evidence: NOT RUN
```

审查覆盖上述完整范围的 21 个变更文件，包括独立任务治理接入、产品、测试、驱动、
文档和实现后的报告；`57d1436..f8a8d7b` 仅改 TASK 报告，产品源码一致。
复核了当前流程及固定历史检查点中的 B 角色指南。Slice 6 原文与 Task Base 完全一致，
Base、计划检查点和实现 Head 的祖先关系成立。产品行为变化集中在正文展示及
`sendMessage` 的 HTML 参数；Collector 仅改注释，reconciler、storage、配置与调度未改。

**本地范围通过，整项任务尚不能关闭。** 本轮未发现需要修改产品代码的阻塞问题。
唯一阻塞 B-TG-01 对应原计划 TG-6：用户完成同步后，在 Telegram 中看到正确的粗体与
图标，点击展开查看文件，再下载清单核对全部文件，这段实际使用过程尚无新版证据。
现有 mock 能证明请求内容与附件字节；它不执行客户端展开操作。旧版截图与接收附件
属于已关闭的 Slice 6，不能证明本次新增的 HTML 展示。

| 验收项 | B 本次结果 | 核对依据 |
|---|---|---|
| TG-1 | 本地 PASS | 四种结果标题、九项分行统计、初始化失败计数不可用；原 Outcome 接入保留 |
| TG-2 | 本地 PASS | 五／六条折叠边界、二十条预览及溢出清单；六类事件保留原来源、路径与清理阶段；CLI 验证三次推进、恢复清零及实际删除 |
| TG-3 | 本地 PASS | 动态值转义、字面实体、长实例／路径及 UTF-16 预算回归；正文按完整区块计长，路径整体转入附件 |
| TG-4 | 本地 PASS | 多来源与无事件溢出、收集不可用、正文外写失败／大小上限提示；来源补写在 Finalize 前完成，附件保持纯文本 |
| TG-5 | 本地 PASS | HTML 参数、发送失败隔离及无重试；tagged CLI 与实 CLI 验证开关、策略、dry-run、锁忙、取消、清理与后续恢复 |
| TG-6 | NOT RUN／阻塞 | 尚无新版成功轮、来源失败轮的真实客户端检查与接收附件核对 |
| TG-7 | 治理与本地证据 PASS；收尾未完成 | 完整 Base..Head 已独立审查，最终决定 FIX REQUIRED；待 TG-6 补齐后由 B 续审关闭 |

关联 Slice 的影响也已重评：RO-1 的配置／触发、RO-2 的结果事实、RO-4 的已提交清理
进度、RO-5 的锁外发送与失败隔离由本轮回归确认；RO-3 的新展示本地通过，真实展示
随 TG-6 待验；RO-6 的当前文档已核对，新版接收证据待补；RO-7 的本独立修复尚未闭环。
这些是本修复的影响判断，Slice 1–6 的历史 PASS / CLOSED 保持原结论。

#### B 验证与证据

本轮证据：`artifacts/validation-evidence/fix-tg-01-b-dnge8vhf/`。
命令、cwd、退出码和耗时见 `review.json`，原始输出见同名日志；所有新增夹具在该目录。

| 独立执行 | 结果 |
|---|---|
| `bash scripts/go-local test -count=1 ./...` | 全部 10 包 PASS |
| `bash scripts/go-local test -count=1 -tags strmgen_testhooks ./cmd/strmgen` | PASS |
| `bash scripts/go-local vet ./...` | PASS |
| `python3 -m unittest discover -s tests -p test_governance.py -v` | 31/31 PASS |
| 现有 sync 驱动 `--scenario notifications,notifications_cleanup` | 2/2 PASS |
| 现有 serve 驱动 `--scenario serve_notifications` | 1/1 PASS |

CLI 使用现有两份二进制，已核对其摘要与 Developer 原记录一致；发布二进制的
`go version -m` 为 `f8a8d7b12bd7781736b2ed7f73771475ad40d5e3`、`vcs.modified=false`。
发布／测试钩子二进制 SHA-256 分别为
`b50142ef499cbd969aace468d941762454fec664f254f86782dc8ff5503bfa1e`／
`0f83ed05c63d48ddbbc85f025b2b19154a2081226aab3d4b3bcedca28e8d118e`。
复用 Developer `fix-tg-01-sync-R7W9DH/final-quality3.log` 的完整质量结果与
`race3.log` 的受影响两包 race 结果；24 项日志、二进制及源码关系核对全部一致，
详见 `reused-evidence-check.json`。复用结果不写成本轮独立重跑。

本轮上述独立检查首次均通过，未修改产品代码或测试。审查文档更新后的提交前治理
检查因路线图尚未提交而拒绝，原始输出保存在 `governance-working.log`；提交后另行复核。
测试及静态检查命令合计不足一分钟；连续计时审查超过三分钟（此前文档与代码阅读
未计入），测试占本轮时间低于
50%。真实 Telegram 发送／客户端检查仍 NOT RUN；本轮未读取历史凭证。
容器全套、真实 OpenList／Jellyfin／插件及 ARM64 原生运行未重跑，沿用本任务 T3 边界。

#### 开发指导：补齐 B-TG-01 后交回 B

继续原 FIX-TG-01，保留 Base、计划检查点和当前实现 Head。剩余工作以现有产品入口及
接收证据为主；无需新增测试平台或扩大功能。真实发送须有用户当次明确授权的专用
Bot／聊天及当前环境凭证，当前会话尚未提供该授权。

1. 在新建的 `artifacts/validation-evidence/` 隔离目录内准备配置、输出与状态，复用
   `scripts/mock_openlist.py` 的现有夹具能力。用当前发布二进制的正常
   `strmgen sync --config <隔离配置>` 发送一次 25 文件成功轮；首批预览纳入中文、
   emoji、`&<>` 和字面 `&lt;b&gt;` 文件名，使接收端能直接核对真实文件身份。
2. 在 Telegram 中记录客户端名称／版本，查看摘要标题、分行统计和特殊文件名，
   保存折叠前及点击展开后的脱敏截图；下载实际接收的 `strm-events.txt`，核对 25 条
   路径、顺序、运行时间标识、总数与输出，并记录接收文件 SHA-256。
3. 使用同一现有夹具的列表失败能力，通过正常 CLI 再运行一次来源失败轮；在客户端
   核对失败标题和来源诊断，保存接收截图与对应退出码／日志。由正常同步形成报告，
   不直接调用发送 API 拼装成功消息。
4. 在 Task Report 追加当前源码／二进制身份、两轮命令、结果、消息关联信息、截图与
   接收附件索引，更新真实视觉字段并交回 READY FOR B REVIEW。产品源码未改时复用
   本轮本地回归；若实际展示发现问题，只修对应渲染／交接并运行直接相关检查。
   B 核对新增接收证据及后续完整差异后决定是否关闭，本独立修复无需追加 A 验收。

## 已完成 Task 6.1

- Task Base／Contract Checkpoint：`1bfac55136dc4d3137d9ee9d470ec5c6ba840749`。
- Implementation Head：`757b082855974c43498ff7e3f5896b31d657a4ea`。
- 独立 B 本地 PASS 范围至 `7c54baf660150ced9ac87e41d34dc4631f923fa1`；
  后续 P2 日志小修复按用户要求自复核，A 在完整实现审查中接受，未重标为独立 B 审查。
- 真实 Telegram 文本／附件送达已确认，用户直接补交截图与 `strm-events.txt`，
  A 核对运行身份、25 条记录与接收文件 SHA-256，A-01 关闭。
- 完整计划、首次失败／修复、历次 B 审查、P2 自复核及 A 首次退回记录保留于
  `89bf67b7868561ab27f307ea88604a65136f4f7f:TASK.md`；本次 A 最终结论见当前 SLICE。

历史 Task 5.1 及更早工作从[固定 Git 检查点](docs/development-workflow.md#历史记录)查看，
不复制归档目录。新大版本需求遵循 A 合同 → B Task → Developer；小版本 bug 无需 A 介入。
