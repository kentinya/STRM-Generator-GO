# Slice 4 历史证据复用账本（V4.4）

2026-10-20 核对本机原件存在、JSON 结构／计数及 SHA-256；未运行真实服务，未签独立 B PASS。
旧 Task 4.1 最后 FIX REQUIRED，完整报告／审查在 `f0767f0:TASK.md`；旧三项暂缓与播放
结论保留为历史。用户授权 V4.4 后，插件 URL／Size 等全部比较行重新成为必验项。

原件根：`artifacts/validation-evidence/task-4.1-fix2-20261019-215918/`（Git 忽略、本机）。
原件不删、不改，不读取历史凭据。旧记录源码为 `83e5b1901e1c306d1f48fcde92786b353eb3d5e3`；
后继未提交输入身份须结合该目录 `input-sha256.txt` 和 `binaries-sha256.txt` 复核，不能只凭源码 SHA。
这两份身份文件 SHA-256 分别为
`e998ab43651f01004044fb72d208cea575ab303de43ca2e89c64b29396c8732f`、
`f0437871d29b20035cf1d26a98ffebe677363e20bdd7d0f638007ec57af50f71`。
INDEX.md SHA-256：`9b2787b0955f37d42c318ee451e697dad93cfdbb975440a181178fecf7ba2c61`。

| 相对原件 | 当前映射与可保留事实 | SHA-256 |
|---|---|---|
| `target/ro2_metadata.json` | RO-2／RO-3：201 项、分页 200+1；仅复用分页兼容，不证明 10000 | `07071632e6b50fb0e2d2a0239c51a0b6fafb9202cc070373c58c7ca229cc894d` |
| `target/ro2_byte_equality.json` | RO-2／RO-4：5 类原始 STRM／稳定 URL 字节；只限旧样本 | `fc3fde175f112aacc41cf15b9a4908f25d83fa3c169107f00144e30084ad9755` |
| `target/ro5_stable_sync.json` | RO-2／RO-3：旧 CLI 重复零重写；新数据集仍须测量 | `4b1991934adcd7a9cc36e3771c932d2743accb861df7571b546d540e8dfd0993` |
| `target/ro5_move_closure.json` | RO-3：旧移动、4 轮清理与恢复；只复用生成／清理部分 | `421fdfa8792f2ccb45764fad7afe1d70a5f54022b56379bf297fd2bf879456eb` |
| `target/ro4_native_playback.json` | RO-4：只复用原生扫描、条目与插件读取记录；播放不参与新门槛 | `2317d24d43359a67de81608298bd16038b376bfb6bed163eff70bb7156fba02d` |
| `target/ro3_decision_rows.json` | RO-5：5 个窗口内决定、0 次探测；仅保留已有复用观察，不能充当完整决策表 | `ef16cccb44759d94aa714bdadae1ca1af875d6f33a994aa766788c33c5b03755` |

复用只限已记录输入与行为：下一 Task 逐项核对源码／二进制／工具、固定镜像、插件包／加载
文件、脱敏配置和媒体身份。撤下运行器不使旧记录失效，也不使新流程自动通过。B 对关键
真实链路和完整决策表仍须独立复现；未匹配输入的行为按 NOT RUN 处理。

固定 10000 数据集、全量生成／RSS、同批最少同步样本、原始产物最少扫描及 D-01..08
决策的当前证据尚缺。旧扫描库有 201 条，不能宣称已按新方案完成最少样本验收；旧两行
SHA-1 实际验证缺口及旧 BLOCKED 记录均未改称 PASS。历史凭据事件仍开放，未宣称处置。

Slice 1–3 的通过结论与完整 T4 输入身份保留在各关闭检查点；当前工具治理只撤旧 Slice 4
入口，不删除其产品测试、四个驱动或已有原始证据。后续执行与 RO 映射见
[测试矩阵](test-matrix.md)、[Slice](../SLICE.md) 和 [Task](../TASK.md)。
