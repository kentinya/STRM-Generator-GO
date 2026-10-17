# 目标包边界

产品代码未开始。`openlist` 负责 API；`pathmap` 负责路径与 URL；`strm` 负责协议与原子文件；`storage` 负责 SQLite 最小归属／轮次；`reconciler` 负责目录对账；`scheduler` 负责手动／定时的共同入口。

当前目录占位不表示这些包已实现。配置与 CLI 装配在后续 Task 确定。依赖应从接口注入，协议／路径纯函数不执行网络或数据库操作。
