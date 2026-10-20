---
title: "STRM Generator Go"
---

# STRM Generator Go — 开发方案 V4.4

**配套项目：** [jellyfin-strm-probe](https://github.com/kentinya/jellyfin-strm-probe)  
**开发语言：** Go  
**数据源：** OpenList  
**核心目标：** 高性能、低内存、增量生成、目录镜像、最终一致性。

本文件定义生成器当前的业务规则、运行边界和最终集成验收。生成器适配配套插件的 STRM 协议和探测规则；实际验收使用已锁定源码 SHA 构建的插件包。

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
              按规则探测／复用
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

- SHA-1 为真实内容的 40 位十六进制哈希，输出小写。
- Size 为非负 Int64 字节数。
- 缺少或无效的 SHA-1／Size 省略对应字段。
- 使用 UTF-8 编码，URL 必须是有效 HTTP/HTTPS 地址。
- 不写入 Modified。
- 不生成临时签名播放 URL。
- 单个 STRM 不超过插件的 64 KB 解析限制。

协议依据：[StrmManifestReader.cs](https://github.com/kentinya/jellyfin-strm-probe/blob/main/src/Jellyfin.Plugin.StrmProbeOptimizer/Services/StrmManifestReader.cs)。

## 三、媒体变化规则

生成器负责更新 STRM，插件负责决定是否 ffprobe。

哈希／Size 比较和不探测行以已有历史指纹、必要主媒体流和当前原生地址匹配为前提。
URL 变化行先建立地址匹配的旧基准；变更后由 URL 历史变化或本次变化导致的原生地址不匹配触发探测，不能以无关主流／地址缺失冒充该行通过。

| 变化情况 | 生成器行为 | 配套插件行为 |
|---|---|---|
| URL、SHA-1、Size 均不变 | 跳过 | 不探测 |
| URL 不变，SHA-1 相同，Size 变化 | 跳过，记录异常 | 不探测 |
| URL 不变，SHA-1 不同 | 更新 STRM | 探测 |
| URL 不变，SHA-1 不可比，两侧有效 Size 不同 | 更新 STRM | 探测 |
| URL 不变，SHA-1 不可比，有效 Size 相同 | 跳过 | 不探测 |
| URL 不变，SHA-1 不可比，Size 信息不足 | 记录限制，不据此推断变化 | 不凭空探测 |
| **URL 变化** | **更新 STRM** | **始终探测** |
| 仅 Modified 变化 | 跳过 | 不探测 |

首次遇到已有主媒体流的 STRM，插件默认信任现有信息；缺少必要主媒体流时仍需要探测。

URL 变化优先于 SHA-1／Size 比较，即使 SHA-1 相同也必须更新 STRM，插件要求探测。

依据：[ProbeDecisionService.cs](https://github.com/kentinya/jellyfin-strm-probe/blob/main/src/Jellyfin.Plugin.StrmProbeOptimizer/Services/ProbeDecisionService.cs)。

## 四、目录同步方案

采用 **Directory Reconciler + Mirror 模式**。

目录遍历、生成、更新和清理由对账轮次统一处理，不维护独立的文件发现状态机。

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

**4. 文件删除：** 源根成功读取为目录后，已拥有目标 Get 未读到才按缺失处理；至少连续三次完整成功轮缺失，删除前按同一源根／目标 Get 规则重新核对，再检查归属与输出边界后安全清理。目标再出现则清零。

**5. 扫描期间文件变化：** 由下一次成功同步纠正。

**6. OpenList 查询失败：** 源根未成功读取为目录时保留旧 STRM，不增加缺失次数；列表失败／分页不完整、整轮取消或必要本地处理失败不推进缺失计数或启动清理。源根可读取时，目标 Get 未读到按第 4 条计缺失，不再区分权限、驱动或请求失败等相同响应的原因。

所有列表请求均使用 `refresh:false`。清理默认关闭，仅清理本程序可靠拥有的 STRM。

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

命令入口：

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
  enabled: false
  missing_confirmations: 3

database:
  path: "/data/strmgen.db"
```

同一输出范围禁止多轮同步重叠，但同一轮内可以有限并发读取目录。

dry-run 仅预览，不修改 STRM、数据库、完整轮次或缺失计数。

采用有界内存处理，规模目标和最终集成验收均为 **10000 个 STRM**，记录实际运行环境、耗时和内存用量。

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
原生批量检查
      ↓
按规则决定探测／复用
```

通过 Jellyfin 原生计划任务检查：

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
| P4 | 第九节的 10000 个 STRM 生成、少量样本同步、Jellyfin 扫描与插件决策验证 |

## 九、最终集成验证

最终集成验证包含以下两项，均使用专用隔离环境。

### 1. 10000 个 STRM 生成与少量样本同步验证

- 复用已经完成的隔离环境、媒体原件及媒体生成／启动／停止脚本；保留现有媒体，仅一次补齐并固定隔离 S3 中的 **10000 个媒体文件**，经 OpenList 供本程序读取，验证能够生成一一对应的 **10000 个 STRM**。脚本在不覆盖原件的前提下复用，必要适配留存原版；后续不重新生成整批数据。
- 生成及后续同步过程中，从这批文件中选取最少量样本，顺序验证新增、更新、跳过、移动／重命名和安全清理等同步规则，其余文件保持稳定；不为每条规则另建一套数据。
- 后续所有需要媒体文件的验证、复核和重跑均复用这 **10000 个文件**，不重新生成媒体。样本需要变更时仅调整必要对象，并保留原始内容以便恢复。
- 记录实际生成数量、同步结果、运行环境、耗时与内存。本次规模验收为 10000 个 STRM。

### 2. 最少 STRM 验证 Jellyfin 与插件连通性

从上述数据集中选取最少量样本，路径不变时优先复用同一个 STRM／媒体库条目顺序验证，无需把 10000 个 STRM 全部导入 Jellyfin。移动／重命名后允许重新入库，不要求延续原 Item ID 或媒体流。

1. **Jellyfin 扫描入库：** Jellyfin 原生媒体库扫描能够收录本程序直接生成的 STRM，确认文件与媒体库条目对应。
2. **插件是否需要重新读取媒体信息：** 插件读到 STRM 后，决定“探测”或“不探测”。探测表示需要重新读取媒体的编码、时长等信息；不探测表示继续使用已经保存的信息。

先完成正常扫描／检查，使插件有可以复用的媒体信息和上一次记录，满足第三节的比较条件。然后复用同一个样本，每次调整一个条件，再运行插件原生检查，核对日志中的实际决定：

| 样本发生的变化 | 插件应作出的决定 |
|---|---|
| 输入不变，或远端仅 Modified 变化 | 不探测 |
| URL 不变、有效 SHA-1 相同，即使 Size 元数据不同 | 不探测 |
| URL 不变、实际内容及有效 SHA-1 改变 | 探测 |
| URL 改变，即使 SHA-1／Size 不变 | 探测 |
| URL 不变、一侧缺少有效 SHA-1，两侧有效 Size 不同 | 探测 |
| URL 不变、一侧缺少有效 SHA-1，Size 相同或信息不足 | 不探测 |

例如，把同一份内容的播放地址从 A 改成 B，插件应记录“需要探测”；地址和 SHA-1 都相同，只把 Size 元数据改成另一个值，插件应记录“复用”。所有样本仍来自固定的 10000 个文件。

通常使用程序生成的 STRM。如果 OpenList 没有提供 SHA-1，或需要制造“同 SHA-1、不同 Size”的测试输入，测试侧可以使用明确标注的 STRM 副本：SHA-1 从这批文件保留的自有本地内容计算；故意修改的 Size 标注为异常元数据。不同内容使用对应真实 SHA-1、实际 Size 和匹配的服务内容。原始生成文件保留，用于证明扫描入库和插件读取；测试副本只用于验证插件规则，不作为生成器同步结果。生成器不下载媒体补算哈希。

每项只记录“改了什么、预期决定、插件实际决定、对应条目及日志”。日志明确要求探测或记录探测启动即可判定“探测”；明确记录复用／跳过才能判定“不探测”，不能根据没有探测日志推断。只核对决定是否正确，无需验证探测结果、客户端播放、会话／进度或代理。缺少实际决策证据时记录为未完成。

---

### 最终方案

**Go + OpenList + Directory Reconciler + SQLite + Mirror + 最终一致性。**

三个核心原则：

**第一，不追求实时一致性。** 远程文件在同步期间变化是正常情况，下一轮同步修正即可。

**第二，不设计多余的状态机。** 生成器只保存必要的文件映射与清理依据，探测状态由 Jellyfin 插件负责。

**第三，保持组件独立。** Go 生成器维护 STRM，Jellyfin 扫描媒体库，配套插件负责按规则探测或复用。
