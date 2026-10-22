# Task 4.5 B 审查（2026-10-20）

## 固定候选与决定

- Task Base：`5182443d0c34215deb7bb1e74a413564aa8e6422`。
- Implementation Head：`9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa`。
- 入口／纯文档后继：`9e37f4a6ecf9925e6e05a9acfbddd419867d960c`；入口main干净。
- Contract Checkpoint：`48ff46a2cfd742190de0e8a673a92ec61c086950`。
- **Decision: FIX REQUIRED**；完整审查Base..Implementation Head的12文件及入口后继，
  包括正式计划、配置、两辅助工具、直接测试、完成报告及全部关联文档，不仅最后修复提交。
- 证据根：`artifacts/validation-evidence/task-4.5-B-20261020T1237Z/`（本机、Git忽略，保留至A决定）。

## 阻塞与同Task修正

### F-45-01／P1：真实最少样本闭环缺失

AC-3／4／5 **BLOCKED**。已有10000生成与稳定重复不替代更新、移动／重命名、三次
不同完整轮缺失、删除前Get、再出现清零及场景恢复。当前配置cleanup=false。
B本轮只核对必要凭据存在性，均未注入；Task明确不继承4.4容器Env授权。历史guest
写请求403／匿名S3拒绝仅作已知限制，不重复无意义写请求，不读取容器Env／历史秘密，
不恢复账户。没有改变远端、服务、媒体、输出或生成器DB。

解除需要用户明确授权4.5在核对专用隔离身份后选择性内存取用必要凭据，或安全注入
专用写凭据；授权不等于通过。Developer继续同一Task、同一source identity／输出／DB，
用固定10000对象中的最少样本完成AC-3..5及恢复，不能删库、换路径、强刷或重建媒体
绕过。源端能提供的Size／URL变化仍须真实同步，缺元数据Go回归不得抵扣。

B随后独立执行同批样本dry-run、更新、移动／三完整轮清理、再出现清零及恢复确认。
**本轮B真实关键复现BLOCKED，未执行，不以只读检查或合成夹具冒充。**

### F-45-02／P1：完整候选门槛未齐备，B负责

本轮B已执行下表本地全量；固定真实Get／缓存、镜像／Compose／amd64容器／arm64 OCI／
重启证据的完整身份映射尚未完成，不能签当前完整T4 PASS。流程允许明确阻塞后先拒绝。
剩余门槛保持NOT RUN／未完成匹配，由B同Task重交时准确映射或补跑，不交回Developer，
不删除测试、不弱化断言，不要求机械重跑Final。

相对Slice3接受Head `410fc0e0a8964046de4707d58c0c7505385c6002`，当前产品／
依赖／Dockerfile／Compose／SDK入口／四驱动／合成夹具未变；这是源码层可复用事实，
不等于产物／版本／配置／原始结果已全部匹配。初查旧部署证据使用`e1f66bca7ff0`，
早于已接受CLI／Compose修正，已记录差异并拒绝直接抵扣。后续须选接受后的证据。

## B本轮门禁与独立核对

| 门槛 | 结果与边界 |
|---|---|
| 治理／合同链接 | 入口PASS；报告与提交后重查，见checks.jsonl |
| mod download／verify | 均PASS |
| quality.py | PASS，含Python全量、AST、Bash、gofmt、vet、Go test／build |
| Go完整非缓存／race | `test -count=1 ./...`及固定CC、CGO=1的`test -race -count=1 -json ./...`均PASS；未配置真实服务的SKIP不算真实PASS |
| amd64／arm64／amd64 testhooks构建 | 均PASS，产物置本轮证据bin，不覆盖旧发布包；arm64仅构建 |
| 辅助工具直接测试 | 15项PASS，ResourceWarning错误模式；两份副本已逐字节匹配，不重复跑相同输入 |
| 发布CLI合成同步 | 11/11 PASS，故障／取消／清理恢复／dry-run均覆盖 |
| serve合成集成 | 6/6 PASS，与同步串行 |
| 固定树独立只读验收 | 清单10000／regular STRM10000／检查10000，0问题，Size10000、SHA-1零、最大116字节 |
| 保留状态核对 | 当前path／size／mtime_ns等于旧稳定轮前后；生成器DB摘要等于旧populated dry-run前后 |
| 原始范围空白 | Base..Implementation Head `git diff --check`通过，本轮文档另查 |
| B真实变更复现／Slice Final | BLOCKED／NOT RUN，不将本地全绿称为Task通过 |

精确argv、cwd、环境覆盖、退出码、耗时见checks.jsonl；原始summary及场景日志保留。
本轮构建buildinfo为入口后继加dirty，当时仅B审查文档未跟踪，产品输入未变；
tracked摘要、工作区diff及buildinfo已保存。不同产物SHA不称为相同二进制，
旧真实测量仍归原发布包，不重标为本轮产物执行。

## 原始证据复用与AC

- 四份tracked快照、artifacts副本和实施Head blob逐字节匹配；Developer续修
  input-identities.json内工具／配置／清单／发布包／依赖／环境规范摘要全部匹配。
- 原始真实运行位于`task-4.5-dev-20261020T093928Z/`；五个JSON逐项与results.json
  及CLI stdout核对。旧二进制buildinfo指向`519bf2d1625e`，其产品／依赖与当前相同。
- 旧测量包装器准确字节从`0a6f8e9`保留，SHA-256为
  `6487d820e3f124d6fe7a727e586f8da71a3564d01468e49f2e8e7710bed6c31b`。
  修复仅同步Popen.returncode；旧结果不改称以修复版包装器执行。
- **AC-1已支持**：初次dry-run退出0、树相同；首轮创建201后超时失败保留，
  同DB／输出重试退出0（created9799、skipped201），当前精确10000树通过。
  真实hashinfo=null，只写有效Size，无伪造SHA-1。
- **AC-2已有证据**：成功重试946.561s／28684KiB，稳定重复982.478s／29396KiB，
  created0／updated0／skipped10000。本轮不重复17分钟真实全量。path／size／mtime
  证明零重写，不单独冒充历史字节快照；当前字节经精确URL／协议／Size验证，另记有序
  内容摘要。恢复后最终稳定重复仍属AC-5，不提前通过。
- populated dry-run退出1（传输超时），只支持失败预览零写，不是成功完成预览。
- **AC-3／4／5 BLOCKED，AC-6未满足**；首败、重跑与未执行项保留。

## 全RO重评与下一步

| RO | 当前判断 |
|---|---|
| RO-1／2 | 4.4独立B PASS保留；本轮清单身份匹配，无重生成／远端写，不声称新做远端全量对账 |
| RO-3 | PARTIAL：10000生成／测量／稳定重复已有，真实样本闭环BLOCKED |
| RO-4／5 | NOT RUN，4.6未激活，无原始扫描／D-01..08实际决定 |
| RO-6 | PARTIAL：B完整范围审查与本地全量已做，真实复现／剩余全量／收尾仍缺 |

既有4.5完整行为计划仍适用，Developer受影响验证和B最终门禁分工不变；
保留原Task Base／实施Head，不新造Task，不私改RO／安全边界。范围／验收含义
若需改变，交A先协调合同。若全部RO未来满足，才运行Slice Final、提交Closure Packet，
返回SLICE READY FOR A REVIEW，不自行关闭。当前不具备该条件。

多处长文含重规划／规划时“均未实施”的旧概述，当前事实以根TASK及本审查为准；
后续协调当前状态表述，不因此新建行为Task或改写历史完成报告。

## 计时及诚实记录

本轮用户上限严格小于50%，不拿上一轮30%覆盖本轮。失败、重跑、只读验收、
治理、构建与证据核对计入测试；起止、原始秒数及保守补计见timing.json，
使用短于完整回合的记录阶段验证比例，不空等凑比、不跳过门禁伪造PASS。

历史五次真实验证合计2899.638s／5533s，至少52.406%；旧“<1%”排除真实验证
不成立。Developer已公开纠正，B复核确认，不倒改旧日志或重跑修复历史比例。
本轮time缺失／shell括号／初始化及报告脚本SyntaxError均如实保留说明；
它们未完成所声称动作，不作产品FAIL或测试PASS。

**返回FIX REQUIRED，继续Task 4.5。Slice ACTIVE，A Final Review／Closure Packet PENDING。**

## 审查后用户明确授权（2026-10-20）

用户随后明确给开发者授权。**Developer**可按当时根 TASK 授权（现见[历史授权记录](authorization-history.md#原授权与交接记录)），
核对专用隔离project／RustFS服务／锁定镜像／挂载身份后，从该RustFS/S3容器Env选择性
内存取用必要凭据，完成同Task AC-3／4／5最少自有样本的S3变更与逐场景恢复。
这解除F-45-01的“尚无授权”前置；本次仅登记授权，未读取凭据、验证鉴权或操作远端，
不关闭F-45-01、不改写上述历史观察，也不授予4.6、生产操作或账户恢复权限。
Task仍FIX REQUIRED；Developer交付后B继续完整范围审查、指定全量及独立关键复现，
F-45-02与其余门槛不变，不将B职责交回Developer。
