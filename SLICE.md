# Slice 1 — 安全的单轮生成闭环

```text
Slice ID: 1
Owner: A
Status: ACTIVE
Base SHA: f90f9560e55b07fd2d6a20e8bac43741c39176c4
Implementation Head: 4752ec2057f998d762829cdb352c8b75198ee900
```

## 用户目标

管理员对一个 OpenList 源运行一次同步或预览，得到可被 V4.3 插件读取的稳定地址 STRM，并能理解失败原因。

## Required Outcomes / Surfaces

- RO-1：目标 YAML、环境变量 token、sources 校验；`strmgen sync` 和 `sync --dry-run` 的明确成功／失败退出状态及脱敏摘要。
- RO-2：OpenList list／get 的分页、应用层错误、可用元数据与缺失信息处理；稳定 `/d/` URL 的逐段编码，不复制 raw_url／sign。
- RO-3：目录镜像映射、扩展名筛选与冲突报告；中文、空格、`#`、`%` 等名称处理；路径及符号链接边界检查。
- RO-4：版本 1 解析／序列化、现有文件比较、同目录原子写入；首次遇到未归属既有 STRM 不覆盖。SQLite 建立最小归属与可恢复写入顺序。
- RO-5：单轮互斥、有界目录工作与故障反馈；dry-run 无持久化副作用。提供可重复的临时根集成证据。

## 约束与不变量

遵守 [需求](docs/requirements.md) 的 R-01..R-12 与 AGENTS。无删除行为；不把共享目录旧文件自动认领。原子写入与数据库提交跨资源失败要能安全恢复。初版从同步／持久化闭环开始，不按类文件拆任务。

## 排除范围

Mirror 删除／移动旧路径清理、定时常驻、正式容器发布、百万级保证、真实 Jellyfin 播放验收；这些由后续 Slice 承接。实施单元由 B 按本合同规划，不因激活扩展到后续 Slice。

## 验收与最终验证

将一次真实本地临时根同步与 dry-run 作为入口，覆盖分页、应用错误、路径冲突、符号链接、无效哈希、无可比信息、相同 SHA-1 的 Size 异常、URL 改变、写入失败／恢复和跨进程互斥。由 B 在激活后以真实代码与风险填写准确命令；并发／持久化门槛为 T4。真实 OpenList 和插件解析实证不能由 Python 夹具替代。

## Closure Packet

NOT STARTED。用户已要求激活第一个 Slice；A 以开发准备提交 `f90f9560e55b07fd2d6a20e8bac43741c39176c4` 固定 Base，本次提交 ACTIVE 合同及同步 Roadmap，B 随后规划首个 Task。当前没有产品实现，Implementation Head 保持 NOT SET，不得把环境准备或合同激活算作 Slice 完成。

## A Final Review

```text
Reviewed Range: NOT SET
Decision: PENDING
P0/P1 Blockers: NOT ASSESSED
Next Action: B PLANS THE FIRST TASK
```
