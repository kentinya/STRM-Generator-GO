# Task 4.4 开发实施指南

本指南落实根 [TASK](../TASK.md) 的 AC-1..6，不增加 Slice 验收范围。当前仍为 PLANNED，
本文件中的新脚本参数、测试文件和清单格式是实施建议，尚未实现或运行。
开发者依次完成“核对输入 → 修改最小生成入口 → 小目录验证 → 固定本地数据 → S3 对账／
补传 → OpenList／环境连通核对 → 交付”。旧任务不构成前置。

## 1. 工作位置、修改范围和交付物

全部命令在仓库根目录执行。保留 [资产清单](slice-4-assets.md) 中的原位置；目录名称
包含旧 Task 编号不影响复用，无须搬迁环境或媒体。

| 路径 | 开发者怎样处理 |
|---|---|
| artifacts/task-4.1/isolated-s3/media/ | 现有 201 个媒体保持字节／mtime；只向预定新路径补文件 |
| 同目录 media-src/template.mkv | 已有 A 原件，24404 字节；只读使用 |
| 同目录 jellyfin-cache/B9-sha1-variant.mkv | 已发现 B 候选，24430 字节；先核对内容身份，再复制到 task-4.4/inputs/ 保留，避免依赖可清缓存目录 |
| 同目录 generate_media.py | 首先保存原版；去掉顶层执行和占位 payload 写入，改为由真实 A／B 原件补缺失。保留入口文件与既有特殊名 |
| 同目录 bootstrap.sh／teardown.sh | 优先不改；前者按需恢复既有环境，后者按需停止。禁止借停止脚本清空数据 |
| artifacts/task-4.4/inputs/ | 保存选定内容原件／变体及摘要，后续作为恢复依据；不导入 OpenList 数据根 |
| artifacts/task-4.4/dataset/ | 放候选／最终清单、数据集说明、样本角色表；不放到 media/ 内 |
| artifacts/task-4.4/tests/ | 可放本任务直接工具测试与临时小夹具；quality.py 不会自动发现这里的测试，须单独执行 |
| artifacts/task-4.4/strmgen.yaml | 后续 CLI 配置，只写 token_env 名称；输出／DB 指向专用新路径，本任务不运行生成器同步 |
| artifacts/validation-evidence/task-4.4-<run-id>/ | 原始命令／退出码、前后摘要、检查结果、脚本／测试准确输入、完成报告 |

不修改 Go 产品、插件源码、生产配置或 Jellyfin DB。确实需要改产品时按 TASK 的风险调整
规则处理。不要为了本任务建设新的媒体服务、S3 客户端框架或生命周期管理平台。

## 2. 进入实施并核对已有环境（AC-1）

先检查 Git，保留已有改动，运行治理。正式执行本指南时将根 Task 状态设为 IN PROGRESS，
保留已固定的 Task Base／Contract Checkpoint；本指南的文档提交不代表实施开始。

```bash
git status --short --branch
python3 scripts/check_governance.py
```

建立本次新证据目录，变量名称统一，防止混用旧输出：

```bash
umask 077
TASK44_ENV="$PWD/artifacts/task-4.1/isolated-s3"
TASK44_WORK="$PWD/artifacts/task-4.4"
TASK44_MEDIA="$TASK44_ENV/media"
TASK44_RUN="$(mktemp -d artifacts/validation-evidence/task-4.4-XXXXXX)"
mkdir -p "$TASK44_WORK/inputs" "$TASK44_WORK/dataset" "$TASK44_WORK/tests"
```

核对 Compose 指定的现有 project／服务和自有挂载目录。记录服务别名、固定镜像 digest、
健康结果、端口与配置摘要；镜像／插件身份以 SLICE 为准。仅选择性读取必要字段，不能
把 docker inspect 的 Env 或带秘密的 compose config 原样写入证据。

| 情况 | 应执行的动作 |
|---|---|
| 既有实例运行且可访问 | 直接复用；启动／停止脚本记录为“本轮无需执行” |
| 专用实例停止、配置／挂载身份一致 | 确认当前进程环境凭据后，按需运行已有 bootstrap.sh；保留退出码和健康结果 |
| 服务名／挂载／镜像与合同不匹配 | 停止相关操作并报告具体缺口，不接管同机其他实例 |
| 当前环境没有所需凭据 | 记录缺少的变量名称，不打印值、不从旧文件／容器 Env 恢复；不继续依赖步骤 |

bootstrap.sh 会建目录和调整自有目录属主，必须确认实际路径后才运行。本任务不要求
为了证明脚本有效而停止一个正常环境。teardown.sh 保留供按需停止使用，不加 -v、不删
媒体／数据／证据目录。记录 rclone／Python／Docker 版本，不追随 latest 更换环境。

## 3. 固定可复用内容并保存原件（AC-2／3）

先重新统计实际文件，不能把规划时的 201 当成当前事实。对现有 regular 媒体记录相对
路径、Size、SHA-256、mtime_ns；拒绝符号链接或越出自有目录的路径。检查名字经现有
pathmap 规则转换后不会同目标冲突，保留已有中文、空格、#、%、引号等名称。

当前可用内容身份如下；实施时再次计算，不只凭文件名：

| 内容 | 位置 | Size | SHA-256 |
|---|---|---|---|
| A | media-src/template.mkv | 24404 | `7f53d1542382ea9035ca3059315b2ec0d2efd47bc5d70b0cc36cfd24bf96f12d` |
| B 候选 | jellyfin-cache/B9-sha1-variant.mkv | 24430 | `e1aee32fbd9ddc13b970cbc644861b5fced7b2b8447542d9eedac8e02c4d7600` |

B 是已有的不同字节／不同大小候选，当前没有以它取得新任务的扫描／探测成功结论。
后续指纹变化验证要求真实字节、哈希和服务内容对应，不要求本任务验证播放或探测结果。
核对 A／B 后复制至 inputs/ 的稳定位置；目标已存在时必须先比摘要，相同则复用，
不同则冲突退出，禁止覆盖。保留源文件。B 不再存在或不能用于所需样本时，先找其他
自有变体，确实缺少才在首次建集补最少真实变体；不为每条决策重新生成媒体。

只读本地内容计算所需 SHA-1／SHA-256。允许 10000 个不同对象复用相同媒体字节，
不要求编码 10000 份不同视频；至少保留 A、B 两种实际内容用于后续变化。ETag／对象名
哈希不得填成内容 SHA-1。输入原件、候选清单和证据均放在 media/ 之外。

## 4. 最小改造 generate_media.py（AC-2）

### 4.1 建议接口（需要开发，不是现有参数）

保留原脚本路径；将可测试函数和 main 分开，使用 `if __name__ == "__main__"`。
导入模块、`--help` 和不带写入开关的调用都不能创建媒体或重写清单。
建议接口只包含以下参数：

| 参数 | 意义 |
|---|---|
| --source | 已核对的 A 原件 |
| --alternate-source | 已核对的 B 原件，用于预定一个或少量变化样本 |
| --media-dir | 自有媒体根；默认仍是已有 media/，测试可指定小目录 |
| --count | 总对象目标，真实固定为 10000；直接测试可用 8 等小数字 |
| --manifest | 最终清单位置，位于 dataset/，不能位于媒体根内 |
| --prepare | 明确允许按候选清单补缺失；不覆盖既有目标 |
| --verify-only | 只验证最终清单与当前文件，不创建文件、不改清单或 mtime |

`--prepare` 与 `--verify-only` 互斥；无两者时只输出计划摘要。成功退出 0，参数／
输入／冲突／I/O 失败非零并输出固定类别、相对路径和必要计数；不要回显秘密或私有绝对路径。
可以用少量函数完成：读取清单、计算文件身份、形成确定计划、补一个文件、验证全部文件。
不建立任务数据库、远端版本锁或自动补偿状态机。

下面展示改造完成后的调用形式，当前原脚本尚不支持这些参数。先按第 3 节准备
inputs/A.mkv 与 inputs/B.mkv，首次使用不带写入开关的预览，核对计划后才加 --prepare：

```bash
TASK44_MANIFEST="$TASK44_WORK/dataset/manifest.jsonl"
python3 "$TASK44_ENV/generate_media.py" \
  --source "$TASK44_WORK/inputs/A.mkv" --alternate-source "$TASK44_WORK/inputs/B.mkv" \
  --media-dir "$TASK44_MEDIA" --count 10000 --manifest "$TASK44_MANIFEST"
python3 "$TASK44_ENV/generate_media.py" \
  --source "$TASK44_WORK/inputs/A.mkv" --alternate-source "$TASK44_WORK/inputs/B.mkv" \
  --media-dir "$TASK44_MEDIA" --count 10000 --manifest "$TASK44_MANIFEST" --prepare
```

最终清单固定后，同一入口用 --verify-only 检查；没有最终清单时必须明确报告尚未固定，
不能仅因本地文件齐备就宣称远端和数据集均完成。

### 4.2 文件规划与补齐算法

1. 枚举并核对现有自有 regular 媒体；已有数超过目标或有明确冲突时失败，不删除凑数。
2. 原有路径全部保留；新路径可统一为 `bulk/000001.mkv` 等固定编号。一个新路径分配 B，
   其余分配 A。若当前恰有 201 文件，则新增共 9799 个，B 包含在这 9799 内，不额外加第 10001 个。
3. 名称由稳定顺序确定；计划中的相对路径、内容类型和预期摘要在第一次写媒体前保存为
   候选清单。候选已存在时读取核对后沿用，不因为部分完成或重跑改变文件名／内容分配。
4. 逐个处理计划项：目标不存在才从对应原件复制；目标存在且类型、Size、内容摘要一致
   就跳过并保留 mtime；类型或摘要不同立即失败。相同 Size 不能代替内容比较。
5. 发布新文件必须不覆盖已存在目标；可用同目录临时文件写完、验证后 link 到未占用
   目标的简单实现，或等效不覆盖操作。只清理本次明确创建的临时文件，不能删既有媒体。
6. 全部补齐后再枚举，确认恰好 count 个媒体、路径集合及摘要符合候选；原有媒体的
   字节／mtime 与步骤 3 的初始记录相同。不把 JSON 清单／源原件计入媒体数。

同一任务使用单写者顺序执行即可，不要求并发复制或复杂故障平台。目录中若发现不在
计划内的新对象，记录冲突并停止，不静默纳入或删除。后续恢复继续使用同一份计划。

### 4.3 清单格式与数据集固定

建议使用 UTF-8 JSONL，相对路径按原字符串稳定排序，每行紧凑 JSON、结尾 LF。字段示意：

```json
{"path":"bulk/000001.mkv","size":24430,"sha256":"<从实际本地字节计算>","sha1":"<从实际本地字节计算>","content_id":"B","origin":"added"}
```

示意中的占位值不能进入实际清单。path 相对 media/，也对应选定 S3 前缀下的对象相对键；
不写完整私有源根或带凭据 URL。origin 只区分 preserved／added，是夹具来源说明，
不构成生成器的 STRM ownership。正文不放运行时间，保证同输入重跑字节不变。

本地补齐后先保留 `manifest.candidate.jsonl`（由 --manifest 的目录派生）；第 6 节
远端核对通过后，将其准确字节固定为 `manifest.jsonl`，不覆盖已有最终文件，另存
`dataset.json`（count、清单 SHA-256、由清单摘要得到的稳定 dataset_id、源内容身份）
和 `samples.json`（样本 A／B 相对路径、内容、恢复来源）。例如 dataset_id 可为
`slice4-v44-` 加清单 SHA-256；以同一份最终字节计算，不能在正文内自引用摘要。
生成脚本只负责本地候选与验证，最终固定在远端核对后用简单本地文件操作完成，
无需把 S3 管理加入生成器产品或媒体复制脚本。最终清单已存在时只能验证，
重复执行不能重写或另生成一批媒体。后续 4.5／4.6 的临时变更写各自运行记录，恢复后
再验证固定清单，不随实验修改固定基准。

## 5. 先用最少夹具验证改造（AC-2／5）

在 task-4.4/tests/ 中使用临时小目录和明确的 A／B 测试字节，目标 count=8；
这些夹具只验证工具逻辑，不作为真实媒体或 OpenList 证据。不要先拿真实 media/ 试错。

| 直接验证 | 必须断言 |
|---|---|
| import／--help／默认预览 | 媒体根和清单不存在时仍零写，已有文件／mtime 不变 |
| 首次补齐 | 原有两个媒体保留，合计恰为 8，每个新文件与分配原件字节相同，B 包含在总数内 |
| 同输入重跑／verify-only | 新增为零，所有媒体字节／mtime 与清单字节不变 |
| 既有目标冲突 | 同大小不同内容、非 regular 或符号链接均非零失败，既有对象不被覆盖／删除 |
| 参数／计数／名称错误 | 源缺失、目标数小于既有数、同目标映射或越界路径，在媒体写入前拒绝 |
| 补齐中断后重试 | 同一候选继续，已正确文件跳过；不生成第二份计划、不把失败写成已完成 |

测试文件若在 artifacts 下，最终必须显式运行并保存原文与结果；不能因为 quality
通过就认为这些测试执行了。也不要给仓库全局测试加依赖本机私有 assets 的隐式 import。
通过后才对真实原件／媒体执行相同入口；保存脚本原版、修改后源码／diff、测试源码与
实际命令，供 B 审查。无需新增通用测试 runner。

## 6. 复用 S3，只补传缺失对象并验证对应（AC-3）

本机已核对 rclone 1.74.1 的 copy／check／lsjson 帮助。执行时再次记录实际版本，
使用当前进程环境注入的 S3 remote，下面 `TASK44_S3` 表示已核对的专用 bucket／前缀，
不得填生产地址。可用 `--config /dev/null` 加环境 remote，避免读取默认历史配置；
remote 名与 RCLONE_CONFIG_<NAME>_* 的映射按已有专用凭据设置，不从历史文件恢复。

1. 先读取实际对象清单，保存相对键／Size 等必要字段；以候选清单逐项对账。
2. 已有且身份一致的对象复用；已有但 Size／内容明确冲突的对象停止处理，不能用上传
   覆盖。同大小只证明大小一致；需要确认已有内容时使用可用的实际内容校验和，或对
   自有对象做只读字节比较，记录验证方式。不能把 ETag 当 SHA-1，也不能把 size-only
   结果标为完整内容验证。检查工具若报告不支持 hash／仅比较大小，应如实降为大小证据。
3. 只为真正缺失的相对键生成 `missing-files.txt`，每行一个原始路径，无多余转义；
   路径不允许换行等控制字符。已有未知对象不删除，不在未知前缀试验 purge／sync。
4. 用 copy 而非同步删除补传；并行数有界即可。上传失败／超时保留候选和已确认结果，
   重新列举确认实际状态后续传，不假设远端没有提交，不重新生成本地数据。

missing-files.txt 为空时跳过上传并记录零缺失，不能改用无过滤复制。rclone 的输出也只
保存必要的相对键、状态与计数；错误若含私有端点或凭据，先脱敏再进入证据，不打开 HTTP dump。

以下是执行形式；变量必须由实际资产核对后赋值，示例不代表命令已经运行：

```bash
: "${TASK44_S3:?请设置专用环境 remote 与 bucket/前缀}"
TASK44_MISSING_LIST="$TASK44_WORK/dataset/missing-files.txt"
rclone --config /dev/null lsjson "$TASK44_S3" --recursive --files-only
rclone --config /dev/null copy "$TASK44_MEDIA" "$TASK44_S3" \
  --files-from-raw "$TASK44_MISSING_LIST" --ignore-existing \
  --no-update-modtime --transfers 4 --checkers 4
rclone --config /dev/null lsjson "$TASK44_S3" --recursive --files-only
```

`--ignore-existing` 只是避免覆盖，不验证已有内容。上传前后的 lsjson 路径是相对该前缀的
对象键；不得只看命令退出码。对象集合与候选逐项一致且恰为 10000，Size 对应；内容校验
与上传来源证据明确，再固定最终清单。若该前缀存在未知多余对象，不删除凑数；先明确
正确自有范围，不能把额外对象过滤隐藏后宣称该源根恰为 10000。

按需用 `rclone check` 比较只读内容；远端无可用 hash 时 `--download` 可以读取字节比较，
但这是验证侧对自有对象的操作，不把下载结果注入 OpenList 或生成器。只验证集合内
所需对象；`--one-way` 会忽略目标多余对象，不能代替上面的完整对象集合核对。无需为了
本任务引入全量媒体下载平台。保留非零退出和 mismatch／error 数，不能用空报告当成功。

## 7. 经真实 OpenList 和原生入口确认可使用（AC-4）

### 7.1 OpenList 分页与样本

使用当前专用进程环境 token；HTTP Authorization 仅在内存请求头传递，不用带 token
参数的 curl 示例，不打印请求头／响应里的 raw_url／sign／token。可写一个本任务小型
取证脚本用 Python 标准库发请求，或复用现成安全入口；不另建元数据服务。

核对当前配置源根 `/api/fs/get` 为目录。所有 `/api/fs/list` 请求体都显式包含：

```json
{"path":"<专用源根或当前子目录>","page":1,"per_page":200,"refresh":false}
```

同时检查 HTTP 及 JSON 外层 code／data；列表失败不是空目录。每目录完整分页：total
必须稳定，名称不重复，累计数与 total 一致，不把空中间页算完成。按目录相对路径累计
regular 媒体，排除目录数量，和固定清单逐项比较。平铺 10000 文件、每页 200 时有 50
个非空页；若有子目录则分别分页，不能用“根目录 total=10000”替代递归对象数。

现有产品每目录最多 10000 项、名称累计 2 MiB，见 internal/openlist/openlist.go。
使用 bulk/ 等布局时根目录的目录项也占根的 total；清单、原件和说明不得放进媒体根，
避免把 10000 文件加额外目录／文件做成超限夹具。普通有界重试等待缓存自然更新，仍
使用 refresh:false；缓存迟滞未消失时记录未确认，不能强刷绕过或改夹具计数。

至少选 A、B 和已有特殊名样本作精确 Get，核对名称／Size／可用元数据；稳定 URL 使用
已核对的 public_url 和逐段编码路径，不拿 raw_url／sign 代替。验证侧读取这些自有样本
URL 返回的原始媒体字节并与本地对应内容比摘要；检查状态和响应字节，不能把登录 HTML
或错误 JSON 当媒体。缺 SHA-1 如实记录，后续插件副本用保留本地内容的真实 SHA-1。

输出只保留必要的相对路径、HTTP／应用状态、页数／总数／摘要和是否匹配，排除带签名
字段和凭据。精确复现命令的秘密参数用变量名表示，不启用 set -x 或 HTTP dump。

### 7.2 Jellyfin／插件可用性

核对专用 Jellyfin 原生入口可访问、固定插件包与实际安装／加载身份一致，原生计划任务
中可找到 STRM 媒体信息检查。可用公开管理入口和必要脱敏启动日志，不查 Jellyfin DB、
不调用插件内部接口，不改插件。这里只确认后续可以操作，扫描入库和 D-01..08 决策
实际通过都留给 4.6；“任务存在”不能写成“插件决策通过”。

## 8. 失败处理与后续交接

| 失败位置 | 开发者应保留／确认的状态 |
|---|---|
| 输入／环境核对失败 | 不写媒体；记录具体缺口和已核对项 |
| 生成中断 | 候选清单、正确文件和原件保留；仅清理本次临时文件，按同一计划续做 |
| 上传超时或失败 | 记录可能已提交；重新列举确认，不默认未上传／已回滚 |
| 远端冲突／多余未知对象 | 停止，不覆盖／清空；核对专用范围和来源后处理 |
| OpenList 缓存／列表未对齐 | 不改变固定数量或强刷；保留 S3 成功与 OpenList 未确认的分层结果 |
| 复核失败 | 不写“数据集固定成功”；保持真实未完成状态，修正后补跑直接受影响项 |

交接给 4.5／4.6：dataset_id／最终清单摘要、10000 个对象的集合与本地内容对应、
A／B 和特殊名样本角色、恢复原件位置、S3 与 OpenList 源根的本地配置身份、稳定 URL
构造方式、固定镜像／插件身份、三个脚本和实际使用方式。只在本地配置保留私有完整
地址，报告用别名／摘要。新的生成器输出／DB 使用专用新路径，不接管旧归属库。

## 9. 检查与完成报告（AC-5／6）

先执行第 5 节直接测试；最终 quality 包含仓库 Python／Go 基础回归，不重复同一套
聚焦测试。artifacts 内的脚本和测试仍要显式校验／执行。正式门槛以 TASK 为准：

```bash
python3 scripts/check_governance.py
python3 scripts/quality.py
bash -n artifacts/task-4.1/isolated-s3/bootstrap.sh artifacts/task-4.1/isolated-s3/teardown.sh
python3 -c 'import ast; from pathlib import Path; p=Path("artifacts/task-4.1/isolated-s3/generate_media.py"); ast.parse(p.read_text(), filename=str(p))'
git diff --check
```

本任务的直接测试建议用标准 unittest，具体命令在测试文件创建后写进报告，例如：

```bash
python3 -m unittest discover -s artifacts/task-4.4/tests -p 'test_*.py' -v
```

未实现的接口／测试文件不能报已执行。报告逐项对应 AC：

| AC | 报告最少应给出的原件 |
|---|---|
| AC-1 | 当前环境／镜像／插件身份和访问结果，实际是否启停 |
| AC-2 | 原件与既有媒体前后摘要／mtime，三个脚本原版／修改文本、直接测试结果 |
| AC-3 | 候选／最终清单、dataset_id、10000 本地／远端对象对账和内容验证方式 |
| AC-4 | 真实 OpenList 分页／精确 Get／稳定样本字节，以及 Jellyfin／插件入口结果 |
| AC-5 | 所用工具／版本、实际命令／cwd、退出码、计数／耗时、首败／重跑／未运行项 |
| AC-6 | 配置身份、A／B 与特殊样本、恢复来源及 4.5／4.6 可直接使用的说明 |

普通本地检查点记录源码完整 SHA；artifacts 中修改的脚本／测试必须交付准确文本和
摘要，不能只有一个 Git 忽略路径。固定三脚本只显式纳入需要的文件或证据快照，禁止
把整个隔离配置／数据树强行加入 Git。完成后根 Task 改为 READY FOR B REVIEW，保留
实际实现 Head、未满足项和分层结果。由独立 B 审查本任务，旧任务没有审查前置。
