# 协议与兼容性

协议／决策规则的固定对照来源为配套插件提交 `1bdb79082c4945e126611f217200555d53c31367`，不是 Slice 4 实际部署包身份，也不是插件发布版本承诺。Slice 4 按合同从配套插件远程仓库获取锁定 SHA 的源码副本并构建插件包，记录远程 URL／分支／SHA、构建命令及产物 SHA-256。源码依据见 [references](references.md)。

```text
# strm-probe-version:1
# strm-probe-sha1:aabbccddeeff0011223344556677889900112233
# strm-probe-size:10737418240
https://openlist.example.invalid/d/Movies/Sample.mkv
```

生成器输出约束：UTF-8 无 BOM、LF，version 固定1、标记不重复、一个稳定 HTTP(S) URL；SHA-1 为真实40位hex，Size为非负Int64，缺失省略；最多65,536字节。插件 reader 可以读取 BOM／CRLF，对无效可选哈希／Size 按不可用处理，但生成器不能依赖宽松解析输出无效值。

插件决策先检查必要主流及原生 URL；首次历史为空时默认信任已有主流。URL 变化必探测；同 URL 两侧有效 SHA-1 相同忽略 Size，不同则探测；不可比才比较有效 Size；Modified 无作用。插件 `allowMediaProbe=false` 和成功后持久化指纹等规则不由生成器控制。

OpenList API 形状固定参考提交 `3b28270a4066ea52943a69ecddd850d83f7e8bdc` 的 `fsread.go`、`object.go`、`hash.go`，实测身份见 [开发环境](development-environment.md)（固定镜像 `sha256:11b49012…`、v4.2.7）；真实部署版本的兼容仍不由夹具证明。`hash_info` 是结构化映射，`hashinfo` 是历史字符串字段；具体兼容优先级在 API Task 固定并测试，不能误把整个字符串当 SHA-1。`raw_url`、`sign` 不写入 STRM。

稳定 `/d/` 地址需要目标挂载允许对应访问；夹具不证明真实挂载无需签名，也不提供可播放媒体。若挂载要求临时签名，应报告不满足当前产品前提，不自动绕过鉴权或改用临时 URL。

| 验证层 | 当前结论 |
|---|---|
| 解析器／决策源码人工对照 | 已完成源码核对 |
| Python list/get 合成夹具 | 可用于开发工具验证，不是 OpenList 兼容证据 |
| 生成器实际输出被远程仓库锁定 SHA 构建的插件包解析 | NOT RUN（已有生成器实现及本地协议回归，未获取／构建／运行 Slice 4 插件包） |
| 真实 OpenList 驱动／鉴权／无签名 `/d/` | NOT RUN（仅 Source Root／Target Get 判定在固定隔离 Local／WebDav 驱动上有实证） |
| Jellyfin 原生库扫描／客户端播放／插件检查 | NOT RUN |
| JellyfinProxy 等外部代理链 | 不在 Slice 4 范围；未验证 |
| ARM64 产品实机／压力与发布 | NOT RUN |
