# Task 4.6 Jellyfin 临时认证交接（2026-10-21）

> 2026-10-22 B2当前核对：以下是首次交接时点，不代表当前仍有可用租约。
> 首次租约已到期自撤销；Developer第二次租约的最终结果于
> **2026-10-22 07:39:11（Asia/Shanghai，2026-10-21 23:39:11Z）**捕获，记录已显式结束。
> 原始`auth-lease-2-final-revoked.json`及当前`artifacts/task-4.6/auth/result.json`
> 记录`temporary_key_revoked=true`、`revoked_key_rejected=true`、退出会话HTTP204、
> `lease_active=false`。B2只读核对这些非秘密结果，未新登录／恢复账户／重发Key；
> 当前没有活跃内存Key可沿用。后续需要时仍由B按AGENTS既有授权核对身份并协调，
> Developer不得自行恢复账户；本次已发生的角色／范围偏差见[第二次B审查](task-4.6-b-review-2.md)。

## 2026-10-22 B3 最终交接（取代上文历史可用性描述）

B为独立原始扫描／八行复现再次核对专用project、服务、锁定镜像与全部挂载后，按既有
明确授权经同一官方流程取得本次仅内存临时Key；未读历史秘密或Jellyfin／插件DB。
本次业务验证结束，**Key已撤销，同Key返回401，恢复会话退出204，保管进程退出0**。
当前没有活跃租约，不应再将下面首轮示例／过期时点理解为仍可使用的认证。
证据：`artifacts/validation-evidence/task-4.6-B3-20261022T010708Z/auth-B3-final-revoked.json`。
本次实际更改了专用隔离管理员密码，随机新值未保存；没有扩大未来取密／恢复权限。
Task4.6已B PASS，Slice交A审查；如需后续认证仍先遵守现有授权职责，不读历史秘密。

## 当前结果与边界

用户明确允许仅对专用隔离Jellyfin执行官方账户恢复、更改管理员密码并创建4.6临时验收Key。
授权先提交于 `55a08d0a8d08c572aadb233ce048d012d32b2b0f`，随后才执行恢复。
B重新核对 `strmgen-slice4-s3`、Jellyfin服务、锁定镜像及全部五个专用bind挂载；未读Env。
当前实例公开API描述确认方法／字段，再使用此前已核对的官方恢复与认证入口，没有猜路由。

- 官方恢复请求、新生成PIN文件核对、PIN兑换及管理员认证成功；仅读取本次新文件，
  路径／regular类型／时间／admin均匹配，服务已删除该恢复文件，未读历史秘密或DB。
- 隔离admin密码已实际改为内存生成的非空强随机值；没有把它写入交接材料。
- 仅创建本次Key，AppName为 `Slice4-Task46-validation-20261021T144955Z`；
  该名字不是Key。使用Key读取原生任务200，21项中目标任务存在／Idle。
- 独立子进程经环境注入Key后，受保护 `/System/Info` 返回200、Slice4／12.2.0匹配。
  这证明交接入口实际可用，不是把Key明文交给聊天或写进配置。
- 未创建STRM副本／媒体库、未改插件配置、未触发扫描／计划任务／播放；未改S3或服务生命周期。
  **通过的只有认证与内存交接，不是AC-1..6、独立矩阵或Task／Slice PASS。**

## Developer／B 的实际使用入口

Key由本机内存保管进程持有，位于已seal的匿名memfd；普通文件只保存非秘密的进程／fd／
有效期元数据。启动器核对进程启动标识、匿名fd、0600权限、seal及有效期，再将Key放入
一个子进程的 `JELLYFIN_API_KEY`，同时设置 `JELLYFIN_BASE_URL`。Key不出现在argv或stdout。
没有新增HTTP代理／插件入口；启动器只负责凭据交接，不编排媒体操作。

在本仓库根运行已验证的安全检查：

```bash
python3 artifacts/task-4.6/auth/with_jellyfin_key.py --check
```

它只输出HTTP状态及目标原生任务Name／Key／State，不执行该任务，不输出认证Key。
执行本Task实际命令时使用启动器的 `--` 子进程形式；下面是已验证的受保护只读请求：

```bash
python3 artifacts/task-4.6/auth/with_jellyfin_key.py -- python3 - <<'PY'
import json, os, urllib.request
key = os.environ["JELLYFIN_API_KEY"]
authorization = ('MediaBrowser Client="STRM-isolated-recovery", Device="B-verification", '
                 'DeviceId="slice4-task46-recovery", Version="1.0", Token="' + key + '"')
request = urllib.request.Request(os.environ["JELLYFIN_BASE_URL"] + "/System/Info",
                                 headers={"Authorization": authorization})
with urllib.request.urlopen(request, timeout=10) as response:
    info = json.load(response)
    print(json.dumps({"http": response.status, "server": info["ServerName"],
                      "version": info["Version"]}))
PY
```

该实例实际接受上述既有MediaBrowser认证头；第一次启动器只带 `X-Emby-Token` 返回401，
已保留失败并修正，不能原样沿用失败头。不要输出env／header／完整响应，不把Key拼进
curl参数、URL、日志或脚本文件。媒体操作仍从根TASK要求的原生入口执行，不调用内部接口。

## 有效期与结束处理

本次保管进程PID为3487609；最长8小时，截止 **2026-10-22 06:49:55（Asia/Shanghai）**，
即2026-10-21 22:49:55Z。时间上限是此临时认证的保管安排，不是产品要求或测试豁免。
当前Key及恢复会话仍有效，**尚未撤销／退出**，否则Developer无法继续使用。

Task验收使用结束时由B执行：

```bash
python3 artifacts/task-4.6/auth/with_jellyfin_key.py --close
```

该命令仅发送结束信号；保管进程随后撤销本次Key、验证同Key返回401、退出恢复会话。
八小时截止也会执行相同清理。必须核对 `artifacts/task-4.6/auth/result.json` 的实际
`temporary_key_revoked`／`revoked_key_rejected`／退出状态再宣称清理完成；不能仅凭发送信号。
若保管进程被强杀／宿主重启，自动清理不保证执行，需B核对服务器侧本次AppName后处理，
不得误撤其他Key。启动器失败就报告B，不从历史文件找秘密或擅自再恢复账号。

## 证据

`artifacts/validation-evidence/task-4.6-jellyfin-access-20261021T144611Z/` 保留授权检查点、
当前API定义、身份检查、辅助代码摘要、6项直接检查、恢复步骤／HTTP状态、子进程交接
验证、首次失败和修正、当前限定范围秘密扫描及计时。实际辅助脚本保留在
`artifacts/task-4.6/auth/`，摘要见 `input-identity.json`；脚本不含凭据。
本机证据不是远端CI产物，保留至A决定。原4.5 UNKNOWN不因此改写或追认。
