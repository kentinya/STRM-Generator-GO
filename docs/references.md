# 固定参考来源

初始采集日期：2026-10-17；2026-10-20 按 V4.4 协调。以下固定输入用于复现，不自动追随 main 或 latest。环境 [规范](../dev/environment-spec.json) 保存版本、来源、SHA 和维护中的文件清单。

| 输入 | 固定身份／用途 |
|---|---|
| 当前行为需求 | [需求](requirements.md)（V4.4）、[范围](scope.md)和[架构](architecture.md)；本地参考资料不纳入治理输入 |
| 当前插件参考／部署源码 | `kentinya/jellyfin-strm-probe@2afc6517fda5a7b4170ebea625619a552c47dabc`；包与实际加载身份仍须核对 |
| OpenList 源码 | `OpenListTeam/OpenList@3b28270a4066ea52943a69ecddd850d83f7e8bdc`，API 数据形状参考；不是已实测服务版本 |
| Go SDK | `1.27.2`，官方模块代理 ZIP 与固定 SHA-256；官方 checksum database 内容摘要已比对 |
| Python 容器夹具 | `python@sha256:0a310eeecf4e1f5a0743f9a6520c90c88d089c903ca5fd283f501e3a805f5f89`，本机 amd64；ARM64镜像运行未验 |

## 配套项目

Slice 4 部署包按合同从 `https://github.com/kentinya/jellyfin-strm-probe.git` 远程仓库获取
并构建，已锁定 main 的 `2afc6517fda5a7b4170ebea625619a552c47dabc`，不重新解析远程 HEAD。
已有包 SHA-256 为 `382d3ef106ed8534b06ab5bff614d826e602a29703dadddbe3ef2e9864bd06d8`。
实施只在本仓库 `artifacts/` 下核对该 SHA 的源码副本、构建记录、包及实际加载身份。
不得使用本地配套仓库 checkout 或把下方历史参考提交误称为部署包，也不得用来源不明的预构建包替代。

- [AGENTS](https://github.com/kentinya/jellyfin-strm-probe/blob/1bdb79082c4945e126611f217200555d53c31367/AGENTS.md) 与 [开发流程](https://github.com/kentinya/jellyfin-strm-probe/blob/1bdb79082c4945e126611f217200555d53c31367/docs/development-workflow.md)：复用 Slice／Task、三职责、状态、Git、分级测试、证据与关闭门槛；本地重新编写，不迁入插件历史授权。
- [STRM reader](https://github.com/kentinya/jellyfin-strm-probe/blob/1bdb79082c4945e126611f217200555d53c31367/src/Jellyfin.Plugin.StrmProbeOptimizer/Services/StrmManifestReader.cs)：版本、UTF-8、65,536字节、SHA-1、Int64、URL 与重复标记。
- [决策器](https://github.com/kentinya/jellyfin-strm-probe/blob/1bdb79082c4945e126611f217200555d53c31367/src/Jellyfin.Plugin.StrmProbeOptimizer/Services/ProbeDecisionService.cs)：主流、原生地址、首次信任、URL优先和哈希／Size比较。

以上 `1bdb790…` 链接保留初始流程／协议依据；当前运行对照为锁定部署源码的
[reader](https://github.com/kentinya/jellyfin-strm-probe/blob/2afc6517fda5a7b4170ebea625619a552c47dabc/src/Jellyfin.Plugin.StrmProbeOptimizer/Services/StrmManifestReader.cs) 与
[决策器](https://github.com/kentinya/jellyfin-strm-probe/blob/2afc6517fda5a7b4170ebea625619a552c47dabc/src/Jellyfin.Plugin.StrmProbeOptimizer/Services/ProbeDecisionService.cs)。

## OpenList

- [fsread.go](https://github.com/OpenListTeam/OpenList/blob/3b28270a4066ea52943a69ecddd850d83f7e8bdc/server/handles/fsread.go)：list/get 请求与响应、分页、hash_info／hashinfo、错误边界。
- [object.go](https://github.com/OpenListTeam/OpenList/blob/3b28270a4066ea52943a69ecddd850d83f7e8bdc/internal/model/object.go) 与 [hash.go](https://github.com/OpenListTeam/OpenList/blob/3b28270a4066ea52943a69ecddd850d83f7e8bdc/pkg/utils/hash.go)：Size、可选 HashInfo、`sha1` 名称及历史 JSON 字符串。
- [官方 fs 文档](https://docs.openlist.team/guide/api/fs)：实机 Task 应再固定目标版本／驱动和脱敏响应。当前夹具不是完整 API 模拟器。

## Go

- [官方发布元数据](https://go.dev/dl/?mode=json) 与 [安装说明](https://go.dev/doc/install)。本机访问 tar 下载最终返回404，不能把“查到版本”写成“已安装”。
- [官方工具链模块代理](https://proxy.golang.org/golang.org/toolchain/@v/v0.0.1-go1.27.2.linux-amd64.zip)；[amd64 checksum 记录](https://sum.golang.org/lookup/golang.org/toolchain@v0.0.1-go1.27.2.linux-amd64) 与 [arm64 checksum 记录](https://sum.golang.org/lookup/golang.org/toolchain@v0.0.1-go1.27.2.linux-arm64)。先按 Go module dirhash 算法比对 h1，再固定各 ZIP 的 SHA-256；安装器使用仓库已固定摘要。
- [Go 模块校验说明](https://go.dev/ref/mod#authenticating)：本次校验记录通过官方 HTTPS 读取，未自行实现 checksum database 的完整透明日志客户端。

升级任何固定输入应显式更新规范、引用与相关证据，不能复用旧兼容／运行结论。
