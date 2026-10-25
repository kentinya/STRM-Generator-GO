# 开发与维护

长期约束见 [AGENTS](../AGENTS.md)，业务规则见[需求](requirements.md)，
包职责见[架构](architecture.md)，状态见根 [SLICE](../SLICE.md)／[TASK](../TASK.md)。
本文件统一记录环境、测试、开发流程与历史查看，不再维护分散的角色经验、任务模板或进度账本。

## 环境安装

需要 Linux amd64／arm64、Python 3.11+、Git 和 HTTPS；Docker／Compose／Buildx 仅在
容器测试时需要。工具版本、源码 SHA、归档摘要和必需文件由[环境规范](../dev/environment-spec.json)
固定；Go 版本与 `.go-version`／`go.mod` 一致，不自动追随 latest。

```bash
python3 scripts/development_environment.py install
python3 scripts/development_environment.py install-cc
python3 scripts/development_environment.py doctor
bash scripts/go-local version
python3 scripts/development_environment.py smoke
python3 scripts/quality.py
```

安装器校验 SHA-256 后写入 `artifacts/dev-env/`；wrapper 隔离 Go 缓存，默认 CGO=0，
禁止自动切换工具链。固定 Zig C 工具链仅用于 race／CGO，不是产品运行依赖。
离线安装可传 `install --sdk-archive <固定ZIP>` 和 `install-cc --cc-archive <固定归档>`。
`smoke` 检查 SDK、标准库和双架构交叉编译，不生成产品二进制或代替 ARM64 原生运行。

```bash
CC="$PWD/artifacts/dev-env/cc/bin/cc" CGO_ENABLED=1 bash scripts/go-local test -race -count=1 ./...
```

## 测试入口

| 检查 | 命令或驱动 |
|---|---|
| 合同、必需路径和文档链接 | `python3 scripts/check_governance.py` |
| Python 工具回归 | `python3 -m unittest discover -s tests -v` |
| 常规质量 | `python3 scripts/quality.py`：治理、Python、语法／空白、gofmt、vet、Go test／build |
| Go 产品回归 | `bash scripts/go-local test ./...`；覆盖配置、API、协议、路径、SQLite、同步／清理、恢复、锁和调度 |
| CLI／Mirror／故障恢复 | `scripts/run_isolated_sync_integration.py` |
| 真实 OpenList Get／缓存／权限边界 | `scripts/run_openlist_absence_integration.py` |
| 定时、多进程和取消 | `scripts/run_serve_integration.py` |
| 容器、挂载、重启和共享锁 | `scripts/run_container_integration.py`，镜像构建见[部署](deployment.md#构建) |

构建发布与故障注入二进制；后者只供隔离测试，不用于生产：

```bash
mkdir -p artifacts/bin artifacts/validation-evidence
bash scripts/go-local build -trimpath -o artifacts/bin/strmgen ./cmd/strmgen
bash scripts/go-local build -tags strmgen_testhooks -trimpath \
  -o artifacts/bin/strmgen-testhooks ./cmd/strmgen
RUN_DIR="$(mktemp -d artifacts/validation-evidence/sync-local-XXXXXX)"
python3 scripts/run_isolated_sync_integration.py --binary artifacts/bin/strmgen \
  --fault-binary artifacts/bin/strmgen-testhooks --scenario all --output-dir "$RUN_DIR"
SERVE_RUN_DIR="$(mktemp -d artifacts/validation-evidence/serve-local-XXXXXX)"
python3 scripts/run_serve_integration.py --binary artifacts/bin/strmgen \
  --fault-binary artifacts/bin/strmgen-testhooks --output-dir "$SERVE_RUN_DIR"
```

四个集成驱动的 `--output-dir` 必须位于 `artifacts/validation-evidence/` 且为新空目录。
真实 OpenList 驱动只创建自有专用容器；先检查其 `--help` 和固定镜像身份，再运行：

```bash
mkdir -p artifacts/task-2.1/bin artifacts/validation-evidence
bash scripts/go-local test -c -tags openlist_integration \
  -o artifacts/task-2.1/bin/openlist-integration.test ./internal/openlist
OPENLIST_RUN_DIR="$(mktemp -d artifacts/validation-evidence/openlist-local-XXXXXX)"
python3 scripts/run_openlist_absence_integration.py \
  --test-binary artifacts/task-2.1/bin/openlist-integration.test \
  --output-dir "$OPENLIST_RUN_DIR/real-openlist"
```

真实服务未配置时的 SKIP 不算真实兼容 PASS；Python 测试不启动真实外部服务。
固定万文件和 Jellyfin／插件八行实际决定属于独立验收，不能用预测模型或合成夹具替代，
预期矩阵见[需求](requirements.md#插件决策验收矩阵)。

## 合成夹具

```bash
python3 scripts/mock_openlist.py
```

默认只监听 `127.0.0.1:15244`，公开假 token 为 `strmgen-fixture-only`，Ctrl-C 停止。
数据见 [openlist.json](../dev/fixtures/openlist.json)，包含分页、中文特殊名、可选哈希／Size
和合成错误。夹具无真实媒体、稳定播放端点或对外请求；`raw_url` 中的假签名用于防止误复制。

```bash
curl --fail http://127.0.0.1:15244/api/fs/list \
  -H 'Authorization: strmgen-fixture-only' -H 'Content-Type: application/json' \
  -d '{"path":"/Movies","page":1,"per_page":2,"refresh":false}'
```

可选 [dev/compose.yaml](../dev/compose.yaml) 使用同一端口，运行前停止原生夹具。
不要停止占用端口的其他服务；另一工作副本使用独立 project／端口。该夹具不挂生产路径。

## 任务与审查

| 职责 | 工作 |
|---|---|
| A | 定义完整用户结果、RO、边界与 Base；最终审查和关闭 Slice |
| B | 基于真实代码规划 Task、独立审查完整 Base..Head、重评全部 RO |
| Developer | 在活动 Task 内实现／修复，提供真实实现 SHA、验证和剩余限制 |

职责不自动授权多代理。实施者不能自签独立审查 PASS，没有独立审查保持 PENDING。
按完整成功／失败／恢复闭环规划，不按文件或类拆任务；每次通过后重评剩余 RO，
不为非阻塞润色无限追加任务。

```text
Task: PLANNED -> IN PROGRESS -> READY FOR B REVIEW -> PASS
                              -> FIX REQUIRED -> IN PROGRESS
Slice: PLANNED -> ACTIVE -> READY FOR A REVIEW -> PASS / CLOSED
                        -> FIX REQUIRED -> 原 Slice 内修正
```

新增产品实施顺序：真实基线提交 → A 提交 ACTIVE 合同及路线图 → B 提交 Task → Developer 实施。
Base 是实施前真实可达完整 SHA，激活后固定；Implementation Head 指向实际实现检查点。
规划阶段可为 NOT SET，不能伪造。Task 计划在根 TASK 中记录 ID、Parent Slice、Status、Task Base、
Contract Checkpoint、Goal／Scope、Difficulty、Test Level、验收条件、测试、非目标及完成／审查报告。

改变 RO、安全不变量或验收含义时先显式修订合同，不把需求变更伪装成纯文档维护。
修正在原 Task 增加检查点，不重写已审历史。用户另行授权的文档维护可以独立执行，
不重新激活已关闭的产品 Slice，也不签发新的产品 PASS。

| 等级 | 适用变更 | 检查 |
|---|---|---|
| T0 | 事实文档 | 相关链接、治理、补丁空白 |
| T1 | 增加已有行为测试 | 新增与受影响测试 |
| T2 | 单一低风险模块 | 聚焦测试与静态检查 |
| T3 | API／CLI／配置等跨模块接入 | 聚焦、相关集成与受影响回归 |
| T4 | 持久化、原子写入、删除、并发、迁移和真实兼容 | 全部已有回归及适用真实运行／故障恢复／打包验证 |

先运行新增、已知失败及直接影响检查；最终候选执行对应等级门槛。
完整套件覆盖聚焦检查时不机械重复；真实服务、重启或删除恢复的独立要求仍执行。
证据复用须匹配准确输入和原始产物，不只看报告标签。不能删测试、弱化断言或隐藏首次失败换取通过；
已有 Task 测试计划由 B 显式调整并说明原因。

所有 Task 通过后汇总 Base／Head、RO、证据、不变量、P2、延期与未运行项，交 A 审查完整范围。
A 记录 Reviewed Range、Decision、Blockers，判 PASS、FIX REQUIRED 或 PARTIAL / RESCOPE。
通过后同步 SLICE／TASK／路线图，根 TASK 撤为无活动；无剩余已授权需求时停止。
关闭后原范围 P0／P1 回原 Slice 修正，P2 不无限延长交付。

## 证据与 Git

记录源码 SHA、未提交输入摘要、命令／cwd、工具与依赖、配置／夹具身份、退出码、计数、耗时、
原始日志、产物摘要、首次失败／重跑、验收映射、复用和未运行项。证据放
`artifacts/validation-evidence/<run-id>/` 或 CI artifact，保留至最终决定；本机证据不等于远端可读。
正常恢复应独立枚举目录并传播 sync 失败；出现观察清除待提交缺失；样本恢复必须使用原始前值，
不能拿已修改配置冒充。外部决定须关联原始字节、正常基准、单变量和当次日志，预测、启动、成功分别报告。

提交前检查 status、完整 diff、未跟踪文件、秘密与空白；显式暂存，不自动 push 或改写历史。
没有真实 Git 身份时保留未提交状态。完整克隆用于核对历史 SHA，质量 CI 使用 `fetch-depth: 0`。
治理检查允许把合同链接改为本仓库固定祖先提交的同内容文件；不允许改变正文、锚点或历史内容。

## 历史记录

旧计划、审查、角色经验和进度不再放在当前目录。原始记录位于清理前固定检查点
`4d16846722f31aa9615494b90df715cf4b5bb6df`，可离线查看：

```bash
git ls-tree -r --name-only 4d16846722f31aa9615494b90df715cf4b5bb6df -- docs Task
git show 4d16846722f31aa9615494b90df715cf4b5bb6df:docs/slice-4-a-final-review.md
```

历史“当前”、授权与 UNKNOWN 仅属原时点，不转成新权限或无泄露证明；Slice 4 临时认证已撤销。
`artifacts/` 中的媒体、隔离服务数据、原始日志与专用工具不随克隆提供。
原开发机的固定数据集和三个脚本继续保留，本次清理不删除或重建这些资产。
