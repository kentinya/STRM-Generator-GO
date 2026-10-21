# Task 4.5 B 授权登记与复测前置（2026-10-21）

## 当前决定

```text
Task Base: 5182443d0c34215deb7bb1e74a413564aa8e6422
Implementation Head: 9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa
Reviewed Documentation Successor: 8de3fde599f1c7e73ff0139ccabd7cfe1caef522
Decision: FIX REQUIRED
Next: SAME TASK 4.5; CREDENTIAL DISPOSITION AND B REAL REPLAY REMAIN
```

用户在 B 解释“同一隔离环境取用必要凭据、最少量样本变更并恢复，不碰生产／不重建
10000文件／不打印保存密码”后答复“可以”。已登记到 AGENTS 与 TASK；
**B 取密及样本操作的授权前置已解除，不再要求用户重复授权。**
这不是历史凭据落盘的例外接受，也不是账户恢复、服务变更或4.6授权。

## 已实际完成

证据根：`artifacts/validation-evidence/task-4.5-B4-20261021T075806Z/`（本机、Git忽略）。

- 入口 main 干净；治理通过。核对发布二进制、固定10000清单和B内容原件摘要，
  与已接受候选相同；产品／依赖／既有工具没有改动。
- 先核对同一 Compose project `strmgen-slice4-s3` 的 RustFS 与 OpenList：
  服务名、锁定镜像与实际image ID、artifacts内准确挂载全部匹配，之后才取必要键。
  未读取 Jellyfin、其他项目或生产容器 Env，未读历史秘密或临时登录helper。
- RustFS只选择 `RUSTFS_ACCESS_KEY`／`RUSTFS_SECRET_KEY`；OpenList只选择
  `OPENLIST_ADMIN_PASSWORD`，未取JWT秘密。值仅在Python内存及rclone子进程环境，
  rclone配置为 `/dev/null`，未创建凭据配置／临时文件，argv／结果只记录键名。
- S3现有 `rclone lsjson --stat` 对固定样本058鉴权成功，Size=24404／非目录。
  OpenList现有 `/api/auth/login`：guest尝试应用code401保留；admin使用上述密码键
  得到code200及仅内存token。没有恢复账户、改密码或设置，也不把新结果倒推为dev3来源。
- 使用token正常调用 `/api/fs/get`：配置源根为目录、样本058 Size24404；
  `/api/fs/list` 的所有请求明确 `refresh:false`，根页与最大子目录页均返回200，
  数量与固定清单推导的相应目录一致。未强刷，未下载媒体补算哈希。
- 本轮**远端媒体变更0，生成器输出／DB写入0，服务配置／生命周期变更0**。
  上述是前置连通／身份事实，不是更新、移动、清理、清零或完整dry-run的PASS。

## 首次失败与修正

第一次一次性preflight错误断言源根列表total应等于10000，实际根目录有202个直接项，
其余文件在清单已存在的bulk目录中。该断言把全树文件数混同直接项数，**不是产品或
数据集失败**。`preflight-first.py/json`完整保留；首次脚本虽输出BLOCKED但进程退出0，
也按真实情况记录，不冒充成功。

修正为从固定manifest计算每个实际目录的预期total，并使前置失败退出非零；
第二次前置通过，耗时约17.7s。源端、媒体、清单和产品没有为通过而修改。
`preflight.py/json`保留准确代码、请求、命令、退出码／耗时及非秘密结果。

## 为什么没有先改样本再启动长轮

原任务要求B使用同一源身份／原输出／原DB，不能换小数据集抵扣。现有发布CLI没有
单文件或排除bulk的验证入口，完整轮仍遍历同批10000；不能臆造过滤参数、修改配置
源路径／归属库或操纵计数来缩短验收。

dev3原始成功轮约753～1003s，当前最大目录单页实测及保留测量支持长轮预估。
关键闭环还需多轮缺失／阈值后清理、新路径恢复清理、同行清零及完整预览；仅一个
约15分钟完整轮已超本轮目前可用测试预算。当前用户要求仍是测试时间严格小于
当轮总时间50%，不能空等凑比、隐去重试或在授权后自行豁免。

因此在**任何媒体变更之前**停止长场景准入；没有把原Task门禁删掉或降级，
B独立完整关键复现保持 **NOT RUN（本轮测试预算不足）**，由B保留，不交回Developer。
这不是鉴权BLOCKED，亦不是产品FAIL。无需为此再申请同一取密授权。

## 同 Task 剩余工作

- [B3审查](task-4.5-b-review-3.md)已接受的真实移动／同行清零、指定全量精确复用保持；
  本轮不机械重跑Go／race／构建／镜像／合成集成或Slice Final。
- F-45-03仍需Developer交非秘密的历史凭据临时文件介质／清除范围／已知副本或未知项，
  以及dev3实际来源说明。新授权及本轮内存取密不抹去已记载的旧落盘事实；
  不要求重新取密或重复万文件测试以补说明，接受例外／轮换由用户／A处置。
- B在可满足当轮测试预算时继续原任务指定真实闭环，保留准确原始证据并确认恢复；
  AC-1..6未齐备前不PASS。若需要放宽用户的时间占比约束，只能由用户明确决定。
- RO-1／2保留，RO-3／6尚未齐备，RO-4／5仍NOT RUN；Task4.5 FIX REQUIRED，
  Slice4 ACTIVE，不激活4.6、不提交已完成Closure或自行关闭。

计时、前后生成器DB身份、源码／一次性输入摘要、治理／链接／空白结果见证据索引。
