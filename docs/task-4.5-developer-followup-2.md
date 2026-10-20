# Task 4.5 Developer 续修交接 — 真实样本闭环（2026-10-20，第二轮）

```text
Task: 4.5
Status: READY FOR B REVIEW
Task Base: 5182443d0c34215deb7bb1e74a413564aa8e6422
Prior Implementation Head: 9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa
Implementation Head: 9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa (unchanged; no product/tool change this round)
Document Successor chain: 19b2ab1 → 6f54c5c → 391040a → 9d71c1c (handoff commits; any later pointer-text fix does not change this chain)
B Decision: PENDING
```

本轮入口为 FIX REQUIRED（B 审查 `46c28c3`＋授权登记 `f6341ec` 之后干净 main），
治理通过。以根 [TASK](../TASK.md) 当前 4.5 任务为准；用户 2026-10-20 独立容器
Env 取用授权（本 Task 专用）已生效并仅用于本轮 AC-3/4/5 最小样本闭环。
用户本轮 30% 计时限制已如实核算但**未达标**（见下），不倒改时间、不空等凑比。

## 交付（无产品改动）

- 未改 Go 产品、依赖、镜像、插件源码、生产配置、Jellyfin DB、bootstrap/teardown、
  固定清单、本地媒体原件。仅新增本机证据（Git 忽略）与本续修文档（tracked）。
- 派生清理配置只存在于证据根（`strmgen-cleanup.yaml`，同源 identity 仅开
  cleanup=true／阈值3）；`artifacts/task-4.5/strmgen.yaml`（cleanup=false）未动。
- 凭据仅内存／子进程使用：核对 project `strmgen-slice4-s3`、rustfs 服务、锁定镜像
  及 artifacts 内挂载后，从 rustfs 容器 Env 取两键至 0600 临时文件，
  注入 rclone 子进程，用后 shred。Guest 登录 token 逐次内存获取、从不落盘。
  证据仅记键名与存在性。无生产／其他项目容器读取，无历史秘密恢复。

## 真实样本闭环（固定 10000 内最少自有样本，原件逐场景恢复）

样本：`Synthetic 050.mkv`（UPDATE：A→B→A）、`Synthetic 052.mkv`
（MOVE-ASIDE→三轮→删除→REAPPEAR）、`Synthetic 051/053.mkv`（SKIP 稳定对照）。
B 内容取固定清单内 `artifacts/task-4.4/inputs/B.mkv`；无新媒体集。

| 步骤 | 结果 |
|---|---|
| 基线 | DB 10000 行／epoch 2／0 缺失；输出 10000 文件聚合 `bca149cd…`；050/052 备份 0400 |
| S3 UPDATE 050 | A(24404)→B(24430)，S3 `lsl`＋OpenList 精确 Get（refresh:false）均为 24430 |
| S3 MOVE-ASIDE 052 | 字节与备份 sha256 一致后 `deletefile`；S3 9999；OpenList Get code 500 |
| sample-update-round1 | exit 0，updated=1（050 STRM 变 24430），1110.8s／29000KiB |
| cleanup-round1 | **首败保留**：exit 1，errors=1 partial（慢速分页传输超时）；轮未提交（仍 epoch 3、052 计数 0） |
| cleanup-round1-retry | exit 0，052 计数 1／epoch 4，820.5s |
| cleanup-round2／3 | exit 0，计数 2／3（epoch 5／6），三轮前 STRM 保留 |
| cleanup-round4-delete | exit 0，deleted=1；052 STRM 删除、归属 dropped、9999 行 |
| 恢复 | 050/052 由证据备份重传；S3 10000；尺寸皆 24404；下载字节 sha256 与 A 一致 |
| reappear-round | exit 0，created=1 updated=1；10000 行、全计数 0 |
| populated dry-run | exit 0，skipped=10000；DB sha256 与输出聚合不变、轮次停留 8 |
| final-stable-repeat | exit 0，0/0/10000；树校验 10000/10000、0 问题 |

失败／重跑／只读验收全部计入测试时间（见证据 INDEX 计时节）。
本轮真实验证墙钟合计约 7489.8s，测试类合计上限约 7610s，占回合
（13:10:13Z→约16:36Z，≈12390s）约 **61%——不满足用户 30% 上限（亦超计划
50%）**。超支结构性原因：AC-3/4/5 要求整树真实同步，每轮在慢速隔离 S3 分页上
约 13–18 分钟，八轮成功＋一轮保留失败是闭环最小值。未空等、未跳门禁；
是否接受由 B 裁定，不自判 PASS。

## 验证（受影响范围）

- `go test ./internal/strm ./internal/pathmap` PASS；`go test ./internal/reconciler
  ./internal/storage ./internal/openlist ./internal/config ./cmd/strmgen` PASS。
- `go vet ./...` PASS；`gofmt -l cmd internal` clean；辅助工具 15/15 PASS；
  治理 PASS；`git diff --check` clean。
- 最终树校验 PASS（size 10000、sha1 0——真实 hashinfo=null，无伪造）。
- 完整 T4 全量与独立关键复现仍由 B 负责；RO-4/5（4.6）NOT RUN。

## 风险与剩余

- B 仍须审查完整 Task Base..新 Head、指定全量门槛、独立复现最小样本
  （dry-run／更新／移动三轮清理／再出现），逐项重评 RO-1..6（AC-6）。
- 慢速分页超时在本 Task 出现两次（历史首轮＋本轮 cleanup-round1），均保留、
  均同库重试恢复；非产品缺陷证据，按未知／未确认停止、无自动补偿。
- restore 首个 copyto 报告 `Bucket "slice4-media" created`；即时重数 10000 且
  尺寸／字节正确，无数据损失，仅作观察记录。
- 原始证据根：`artifacts/validation-evidence/task-4.5-dev2-20261020T1345Z/`
  （本机、Git 忽略，INDEX 含命令／退出码／计数／耗时／RSS／首败／恢复映射）。
