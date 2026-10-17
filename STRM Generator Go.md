---
title: "STRM Generator Go"
---

# STRM Generator Go — 开发方案 V4.3

**配套项目：** [jellyfin-strm-probe](https://github.com/kentinya/jellyfin-strm-probe)  
**开发语言：** Go  
**数据源：** OpenList  
**核心目标：** 高性能、低内存、增量生成、目录镜像、最终一致性。

已核对插件当前 `main` 分支，开发基准为 **V4.3**，Slice 1～3 已完成，Slice 4 正在开发。因此生成器直接适配其现有 STRM 协议和探测规则。

## 一、整体架构

```text
                  OpenList
                     │
                     ▼
             Directory Reconciler
                     │
              读取目录与文件
                     │
                     ▼
               SHA-1 / Size
                     │
                     ▼
                 STRM Sync
                     │
             ┌───────┼───────┐
             ▼       ▼       ▼
            新增    更新     跳过
                     │
                     ▼
               本地 STRM
                     │
                     ▼
                  Jellyfin
                     │
                     ▼
             jellyfin-strm-probe
                     │
              按需原生 ffprobe
```

生成器不运行 ffprobe、不操作 Jellyfin 数据库，也不调用插件内部接口。

## 二、STRM 文件格式

严格使用插件现有协议：

```text
# strm-probe-version:1
# strm-probe-sha1:aabbccddeeff0011223344556677889900112233
# strm-probe-size:10737418240
https://openlist.example.com/d/Movies/movie.mkv
```

要求：

- SHA-1 为有效的 40 位十六进制字符串。
- Size 为字节数。
- 缺少 SHA-1 或 Size 时省略对应字段。
- 使用 UTF-8 编码，URL 必须是有效 HTTP/HTTPS 地址。
- 不写入 Modified。
- 不生成临时签名播放 URL。
- 单个 STRM 不超过插件的 64 KB 解析限制。

协议依据：[StrmManifestReader.cs](https://github.com/kentinya/jellyfin-strm-probe/blob/main/src/Jellyfin.Plugin.StrmProbeOptimizer/Services/StrmManifestReader.cs)。

## 三、媒体变化规则

生成器负责更新 STRM，插件负责决定是否 ffprobe。

| 变化情况 | 生成器行为 | V4.3 插件行为 |
|---|---|---|
| URL、SHA-1、Size 均不变 | 跳过 | 不探测 |
| URL 不变，SHA-1 相同，Size 变化 | 跳过，记录异常 | 不探测 |
| URL 不变，SHA-1 不同 | 更新 STRM | 探测 |
| URL 不变，SHA-1 不可比，Size 变化 | 更新 STRM | 探测 |
| **URL 变化** | **更新 STRM** | **始终探测** |
| 仅 Modified 变化 | 跳过 | 不探测 |

首次遇到已有主媒体流的 STRM，插件默认信任现有信息；缺少必要主媒体流时仍需要探测。

**注意：V4.3 已明确修改为 URL 变化一律 ffprobe，即使 SHA-1 相同。**

依据：[ProbeDecisionService.cs](https://github.com/kentinya/jellyfin-strm-probe/blob/main/src/Jellyfin.Plugin.StrmProbeOptimizer/Services/ProbeDecisionService.cs)。

## 四、目录同步方案

采用 **Directory Reconciler + Mirror 模式**。

不再引入独立的文件发现状态机。

例如：

```text
OpenList：
/Movies/动作/A.mkv

本地：
/strm/Movies/动作/A.strm
```

远程移动：

```text
OpenList：
/Movies/科幻/A.mkv

本地：
/strm/Movies/科幻/A.strm
```

同步原则：

**1. 文件新增：** 生成 STRM。

**2. 文件变化：** 比较现有 STRM，必要时原子更新。

**3. 文件移动或重命名：** 新路径生成 STRM，旧路径安全清理。

**4. 文件删除：** 连续两次完整成功对账未发现，且再次确认不存在后删除。

**5. 扫描期间文件变化：** 不处理竞态，下一次同步修正。

**6. OpenList 查询失败：** 保留旧 STRM，不根据失败结果执行清理。

不需要在每次写入前再次请求 OpenList，也不需要为远程文件维护复杂版本锁。

## 五、Go 程序结构

```text
strm-generator-go/
├── cmd/
│   └── strmgen/
│
├── internal/
│   ├── openlist/       # API 客户端
│   ├── reconciler/     # 目录对账
│   ├── strm/           # 读取、生成、原子更新
│   ├── pathmap/        # 目录镜像
│   ├── storage/        # SQLite
│   └── scheduler/      # 手动、定时运行
│
├── config.yaml
├── Dockerfile
└── go.mod
```

技术选择：

| 模块 | 技术 |
|---|---|
| HTTP | Go `net/http` |
| JSON | `encoding/json` |
| 并发 | Goroutine + 有界 Worker Pool |
| 数据库 | SQLite |
| 日志 | `log/slog` |
| 配置 | YAML |
| 文件写入 | 原子替换 |
| 部署 | Docker / Linux / ARM64 |

SQLite 只保存文件归属、路径映射、最近完整对账轮次和连续缺失次数。

**不保存独立的媒体版本状态机。** URL、SHA-1、Size 已经存在 STRM 中，可以直接读取比较。

## 六、运行与性能

建议第一版支持：

```bash
strmgen sync             # 手动同步
strmgen sync --dry-run   # 预览变化
strmgen serve            # 定时同步
```

配置示例：

```yaml
openlist:
  api_url: "http://openlist:5244"
  public_url: "https://openlist.example.com"
  token_env: "OPENLIST_TOKEN"

sources:
  - remote: "/115/Movies"
    output: "/strm/Movies"

sync:
  interval_minutes: 60
  directory_workers: 4
  atomic_write: true
  skip_unchanged: true
  mirror: true

cleanup:
  enabled: true
  missing_confirmations: 2

database:
  path: "/data/strmgen.db"
```

同一输出范围禁止多轮同步重叠，但同一轮内可以有限并发读取目录。

目标支持十万至百万级 STRM，采用有界内存处理；512 MB 可以作为初始压测限制，而非保证值。

## 七、与 Jellyfin 插件配合

```text
STRM Generator
      ↓
生成或更新 STRM
      ↓
Jellyfin 媒体库扫描
      ↓
jellyfin-strm-probe
      ↓
播放自动检查 / 原生批量检查
      ↓
需要时执行 ffprobe
```

插件 V4.3 已经支持 Jellyfin 原生计划任务：

**控制台 → 计划任务 → STRM 媒体信息检查**

因此生成器不需要控制插件的任务执行。

管理员可以分别设置生成器同步、Jellyfin 媒体库扫描和插件检查的时间，但三者目前不保证自动串行完成。

如果本地 STRM 路径因目录移动发生变化，Jellyfin 可能将其重新入库，原有媒体流和 Item ID 不保证保留。

## 八、开发优先级

| 阶段 | 内容 |
|---|---|
| P0 | OpenList API、STRM 协议、目录映射 |
| P1 | 增量生成、SQLite、原子更新 |
| P2 | Mirror 同步、安全清理 |
| P3 | 定时任务、并发优化、Docker |
| P4 | 与 jellyfin-strm-probe V4.3 联动测试 |

---

### 最终方案

**Go + OpenList + Directory Reconciler + SQLite + Mirror + 最终一致性。**

我建议坚持三个核心原则：

**第一，不追求实时一致性。** 远程文件在同步期间变化是正常情况，下一轮同步修正即可。

**第二，不设计多余的状态机。** 生成器只保存必要的文件映射与清理依据，探测状态由 Jellyfin 插件负责。

**第三，保持组件独立。** Go 生成器维护 STRM，Jellyfin 扫描媒体库，V4.3 插件负责按需 ffprobe。

这样可以避免重复 MediaFlow 中较复杂的文件发现、执行冻结和竞态管理设计，同时满足之前确定的全部核心需求。