# Slice 4 Closure Packet — B 交 A（2026-10-22）

```text
Slice ID: 4
Status: PASS / CLOSED
Base SHA: d1c95ad079a0e3f952d9e57ab50299c6ae93ad63
Implementation Head: 24e1ba0ab88173f2c7374f450c9750e5a1e9b504
B Task Decisions: 4.4 PASS / 4.5 PASS / 4.6 PASS
B Slice Final: COMPLETE (execution plus exact-input evidence reuse)
A Final Review: docs/slice-4-a-final-review.md (COMPLETE)
Decision: PASS
Next Action: A SELECTS THE NEXT LARGE SLICE
```

## 用户结果与固定范围

同一固定 10000 S3 文件经真实 OpenList／发布 CLI 生成 10000 STRM；最少样本已证明
同步、移动、三完整轮清理、再出现、dry-run 与恢复。原始生成字节经 Jellyfin 原生入口
读取，另用明确标注的同批副本完成 D-01..08 实际插件决定。无需探测结果成功或播放。
不重建已有环境、媒体或数据集，不新增代理、元数据服务或三组件编排。

A 审查原 Slice Base 到最终 Implementation Head 的完整范围，而非仅最后一次修复。
Task 4.6 的 `54f2dd2` 为纯文档后继；本次 B 报告／Closure 协调也不改变可执行输入。
SLICE 正文保留规划时点事实以保持既有能力合同；当前状态以顶栏及本 Packet 为准，
不把“当时未实施”当作当前无交付，也不由 B 改写合同追认权限。

## Task 与审查索引

| Task | Base → Implementation Head | 独立 B 决定／证据 |
|---|---|---|
| 4.4 资产复用／固定数据集 | `77fc768790a1c1ba6f0700c0aab294d3237cf7e4` → `12a9c4f273efbdb9856a85b9a335528c32585d57` | [B4 PASS](task-4.4-b-review.md#原生可用性补证复审2026-10-20-b4)，保留 B1..3 首败及修正历史 |
| 4.5 万文件／最少同步 | `5182443d0c34215deb7bb1e74a413564aa8e6422` → `9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa` | [B7／B8 PASS](task-4.5-b-review-5.md#2026-10-21-续审用户接受历史-unknowntask-pass)，文档后继 `481be61b158542c47d6baa6bf3e32a9ed4d7ba7b`，接受检查点 `b599dbc5c0e6bd394ee361343d74ebbb12d1ecab` |
| 4.6 原始扫描／完整决定 | `b599dbc5c0e6bd394ee361343d74ebbb12d1ecab` → `24e1ba0ab88173f2c7374f450c9750e5a1e9b504` | [B3 PASS](task-4.6-b-review-3.md)，包含原实现 `69d022e2f7acb41fe1002fad9652386e4460cc75`、B2 修正及全部后继 |

## RO 与最终证据

| RO | 最终 B 验收面与证据 |
|---|---|
| RO-1 | 环境／媒体／三脚本及工具追溯：4.4 B3／B4；本轮八项操作副本、三服务锁定镜像／全部挂载、插件源码／ZIP／DLL 一致 |
| RO-2 | 清单 SHA-256 `378068e317ca30787c3fea5bfbefa298a27ad4c430cfa149cd6eeccc8479d3c9`；本轮 final-s3-check 对 10000 路径／Size 完全匹配，同批 A/B 原件保持 |
| RO-3 | 4.5 首轮测量与独立 B7 十轮真实闭环；本轮 94 份 B7 原始摘要匹配、10000 输出校验 0 问题、10001 条输出／状态字节与 mtime 相同 |
| RO-4 | B3 original-native-refresh／01-ac1：原字节 SHA-256、原生 item／source、扫描请求及本次明确探测启动；原 URL 不可达如实保留 |
| RO-5 | B3 02-ac2、03-matrix、D-01..08 与 windows：B 自己观察的 8 行，当前主流／地址匹配，原生复用连续历史、单变量与逐行恢复；无 DB 依赖 |
| RO-6 | B3 完整 Task Base..Head 审查、gate-reuse-confirmed／当前文档检查、独立行为及本 Packet；A 最终决定仍待完成，不由 B 代签 |

当前主要证据根：`artifacts/validation-evidence/task-4.6-B3-20261022T010708Z/`。
精确复用的上游：4.4 B3／B4、4.5 B1／B2／B3／B5／B7／B8；路径、原命令、
退出码、摘要、二进制／镜像／OCI 及输入匹配见 [B3 门槛表](task-4.6-b-review-3.md#指定全量与精确复用)。
原始证据为本机 Git 忽略文件，A 在本机可核验；没有声称已上传远端，保留至 A 最终决定。

Slice Final 使用当前固定合同及上述完整身份链；没有为了换阶段名重跑同输入 Final。
本轮新增的直接工具测试 25 项、仓库 Python 111 项、原始扫描及八行由 B 执行；Go／
race／构建／11 同步／6 serve／固定 Get／容器等准确匹配复用，不交回 Developer。

## 不变量与恢复

- 产品协议、URL→SHA-1→Size 优先级、缺失元数据、归属／路径边界、SQLite 最小事实、
  dry-run、互斥及完整轮条件不变；当前 Task 未修改 Go 产品或锁定插件。
- 仅对同批已确认自有最少样本作必要变化；最终固定清单／S3 集合、样本 A 内容、
  原输出／状态库与必要插件配置均已确认恢复。保留两个专用测试库与原始／A 标注副本，
  不删除未知对象。LastModified 不承诺回到旧值。
- 本轮临时 Key 已撤销，同 Key 验证 401，恢复会话退出 204；无活跃交接租约。
  B 官方恢复确已改变隔离管理员密码，随机新值未保存，不将认证副作用写成“无变更”。
- 不访问 Jellyfin／插件 DB、不调用内部接口、不改生产／其他项目／服务生命周期；
  4.6 历史曾直接读取插件 DB 的证据保留并标为不合规，当前依赖已退役。

## 历史处置、P2 与限制

- F-46-02 已修正并经无 DB 的 B 原生复现闭合；F-46-03 原 P2 不单独阻断，最小修正
  及本次实际配置恢复已核对，不追加恢复平台门槛。
- F-46-04 两项已披露历史范围／角色操作、F-45-03 历史凭据 UNKNOWN 均已由用户
  接受，仍保留原始事实，不改写为当时授权／无泄露／已清除的证明，不扩大未来权限。
- 无待 Developer 修复的 P0／P1；不为非阻塞润色新增 Task。原始 URL 在容器内不可达，
  因而原始副本不作为正常比较基准；决定矩阵使用明确标注的稳定同网络 URL 副本。
- 仅接受实际探测／复用决定，未要求或宣称探测成功。ARM64 实机、正式发布、播放／
  会话／进度、代理及更高规模 NOT RUN／合同排除，不列为当前延期阻塞。
- 本轮测试占比严格小于 50%，含真实等待／失败／恢复／只读验收，详见 B3 timing。
  4.5 历史长场景有其单独用户豁免，本轮没有沿用。

## A 关闭协调与停止点

本 Packet 的 B 原始交接已完成；上文“A 待完成”等文字保留 B 时点，不是当前待办。
A 已独立审查完整 Slice 范围、后继、用户结果与原始证据，决定 [PASS](slice-4-a-final-review.md)。
本关闭检查点协调 README、CURRENT／范围／用户旅程／兼容性／环境／测试矩阵、
Roadmap／Progress、SLICE／TASK 和三角色经验，不改稳定需求、V4.4 摘要或合同能力。
Task 4.6 保留 PASS，根 TASK 撤为无活动；历史接受不追认权限，原证据不删除。

**Slice 4 PASS / CLOSED。关闭后停止，不自动选择或启动下一 Slice。**
