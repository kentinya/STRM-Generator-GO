# Slice 1 — 安全的单轮生成闭环

```text
Slice ID: 1
Owner: A
Status: ACTIVE
Base SHA: f90f9560e55b07fd2d6a20e8bac43741c39176c4
Implementation Head: d0e2bacfb824b4876c647059e43c2996a9339417
```

## 用户目标

管理员对一个 OpenList 源运行一次同步或预览，得到可被 V4.3 插件读取的稳定地址 STRM，并能理解失败原因。

## Required Outcomes / Surfaces

- RO-1：目标 YAML、环境变量 token、sources 校验；`strmgen sync` 和 `sync --dry-run` 的明确成功／失败退出状态及脱敏摘要。
- RO-2：OpenList list／get 的分页、应用层错误、可用元数据与缺失信息处理；稳定 `/d/` URL 的逐段编码，不复制 raw_url／sign。
- RO-3：目录镜像映射、扩展名筛选与冲突报告；中文、空格、`#`、`%` 等名称处理；路径及符号链接边界检查。
- RO-4：版本 1 解析／序列化、现有文件比较、同目录原子写入；首次遇到未归属既有 STRM 不覆盖。SQLite 建立最小归属与可恢复写入顺序。
- RO-5：单轮互斥、有界目录工作与故障反馈；落实 SW-1..SW-6 单写者部署前提、已知条件校验与只读消费者说明；dry-run 无持久化副作用。提供可重复的临时根集成证据。

## 约束与不变量

遵守 [需求](docs/requirements.md) 的 R-01..R-12 与 AGENTS。无删除行为；不把共享目录旧文件自动认领。原子写入与数据库提交跨资源失败要能安全恢复。初版从同步／持久化闭环开始，不按类文件拆任务。

本地安全保证以需求 SW-1..SW-6 为部署前提：生成器独占输出／暂存／状态写入，消费者只读；生成器进程必须共享同一锁资源。前提贯穿轮次间与中断恢复，不保证任意不合作外部写者在 syscall 间隙替换名字时仍绝不误操作。既有未归属文件保护、未知残留跨重启冲突、程序自身失败恢复、路径边界、dry-run 和完整轮规则不变。只保护暂存目录或增加检查次数不能代替单写者部署；不修改生产权限或引入媒体状态表。

### A 合同修订决定（2026-10-18）

用户明确授权 A 限制外部写者，采纳上述单写者模型，取代旧合同对任意本地外部替换窗口的无条件保证。这是安全前提及验收含义的实质修订，不是已发现缺陷被修好，不追认旧失败为 PASS。Slice ID／Base／ACTIVE 不变，原始方案不变；本提交后由 B 重新判定 Task 1.2 的合法调整与具体测试映射，再开放实施准入。F-14、启动不明恢复事实保护、已知权限条件校验和完整 T4 仍须完成。

## 排除范围

Mirror 删除／移动旧路径清理、定时常驻、正式容器发布、百万级保证、真实 Jellyfin 播放验收；这些由后续 Slice 承接。实施单元由 B 按本合同规划，不因激活扩展到后续 Slice。

不开发抵御同 UID／root／任意不合作写者的文件系统仲裁、内核设施或自动权限／容器修复；不新增单写者确认开关来代替实际部署约束。

## 验收与最终验证

将一次真实本地临时根同步与 dry-run 作为入口，覆盖分页、应用错误、路径冲突、符号链接、无效哈希、无可比信息、相同 SHA-1 的 Size 异常、URL 改变、写入失败／恢复和跨进程互斥。由 B 在激活后以真实代码与风险填写准确命令；并发／持久化门槛为 T4。真实 OpenList 和插件解析实证不能由 Python 夹具替代。

另验 SW-6：只读消费者的实际写入拒绝、已知权限不合规／属性失败在修改前拒绝、同一锁命名空间多进程、无外部写者的所有故障恢复和跨多次重启的未知残留保护。任意外部写者反例按明确旧／新合同映射报告，不称通过或隐藏；完整 T4 不降低。权限夹具不代表生产部署已配置，也不代表真实 OpenList／插件通过。

## Closure Packet

NOT STARTED。Task 1.1 有历史 B PASS；最新实现候选为上列 Implementation Head，Task 1.2 仍 FIX REQUIRED。单写者合同修订不是产品修正或验收通过；B 调整 Task 后仍须实现、复审、逐项核对全部 RO，满足后才提交 Closure Packet。

## A Final Review

```text
Reviewed Range: NOT SET
Decision: PENDING
P0/P1 Blockers: TASK 1.2 FIX REQUIRED; REVISED CONTRACT VALIDATION PENDING
Next Action: B REASSESSES TASK 1.2 AGAINST COMMITTED SINGLE-WRITER CONTRACT
```
