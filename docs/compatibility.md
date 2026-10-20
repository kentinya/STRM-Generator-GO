# 协议与兼容性

当前 V4.4 协议／决策对照与 Slice 4 部署源码固定为配套插件提交 `2afc6517fda5a7b4170ebea625619a552c47dabc`；旧 `1bdb790…` 仅保留历史来源。已有构建包身份须与安装／实际加载文件核对，不自动追随远程 HEAD。源码与包摘要见 [references](references.md)。

```text
# strm-probe-version:1
# strm-probe-sha1:aabbccddeeff0011223344556677889900112233
# strm-probe-size:10737418240
https://openlist.example.invalid/d/Movies/Sample.mkv
```

协议要求 UTF-8，推荐无 BOM／LF，当前生成器按此序列化；version 固定1、标记不重复、一个稳定 HTTP(S) URL；SHA-1 为真实40位hex，Size为非负Int64，缺失省略；最多65,536字节。插件 reader 可以读取 BOM／CRLF，对无效可选哈希／Size 按不可用处理，但生成器不能依赖宽松解析输出无效值。

插件决策先检查必要主流及原生 URL；首次历史为空时默认信任已有主流。URL 变化必探测；同 URL 两侧有效 SHA-1 相同忽略 Size，不同则探测；不可比才比较有效 Size；Modified 无作用。插件 `allowMediaProbe=false` 和成功后持久化指纹等规则不由生成器控制。

OpenList API 形状固定参考提交 `3b28270a4066ea52943a69ecddd850d83f7e8bdc` 的 `fsread.go`、`object.go`、`hash.go`，实测身份见 [开发环境](development-environment.md)（固定镜像 `sha256:11b49012…`、v4.2.7）；真实部署版本的兼容仍不由夹具证明。`hash_info` 是结构化映射，`hashinfo` 是历史字符串字段；具体兼容优先级在 API Task 固定并测试，不能误把整个字符串当 SHA-1。`raw_url`、`sign` 不写入 STRM。

稳定 `/d/` 地址需要目标挂载允许对应访问；夹具不证明真实挂载无需签名，也不提供可播放媒体。若挂载要求临时签名，应报告不满足当前产品前提，不自动绕过鉴权或改用临时 URL。

| 验证层 | 当前结论 |
|---|---|
| 协议／决策源码对照与本地 Go 回归 | 已有验证，保留 Slice 1–3 结论 |
| Python list/get 合成夹具 | 工具／装配证据，不是实际 OpenList 兼容 |
| 真实 OpenList 源根／Get／缓存 | Task 2.1 已独立 B PASS；不等于全链路验收 |
| S3/RustFS／原始 STRM／插件读取 | Task 4.1 有 201 项分页、5 类字节与稳定复用 Developer 原件；最后 FIX REQUIRED |
| Jellyfin 原始产物扫描 | 旧 Developer 记录保留；新固定数据集最少扫描与独立 B 复现待验 |
| 插件完整比较表 | 当前 D-01..08 实际证据未齐，无旧暂缓例外 |
| 10000 个 STRM／实际耗时和 RSS | NOT RUN |
| ARM64 产品实机／正式发布 | NOT RUN；不在当前合同 |

## V4.4 验收边界

固定复用同批 10000 个隔离 S3 文件，以最少原始 STRM 验证原生扫描与插件读取；只核对
探测／复用决定。URL 改变、同哈希／异哈希、Size 不可比各行均须按
[测试矩阵](test-matrix.md) 实际观察；探测启动即可，复用需明确原因。
旧播放和旧暂缓记录不再作为当前门槛；没有客户端播放、代理或探测结果成功要求。

目标 get 未提供有效 SHA-1。测试侧可从同批保留的自有本地媒体计算真实 SHA-1，使用
明确标注的独立 STRM 副本；同哈希不同 Size 标注异常，异内容的真实哈希／Size／服务字节
必须一致。生成器不下载媒体补算哈希；副本不冒充 CLI 同步结果。原始生成字节单独保留。

哈希／Size 比较和不探测行须有历史指纹、必要主媒体流和当前原生地址匹配；URL 变化行
先建立匹配的旧基准，变更后可由 UrlChanged 或该变化导致的 NativeUrlMismatch 触发探测，
不能用无关地址／主流缺失代替该行。逐行恢复基准，用正确 item 的
当次日志窗口判定。首次信任或没有日志不足以证明比较通过。现有 Go 比较通过不能代替
真实插件证据。复用原件／摘要／限制见 [复用账本](slice-4-evidence.md)，实施交接见
[Slice](../SLICE.md)／[Task](../TASK.md)。

既有隔离环境、媒体原件和三个辅助脚本按 [资产清单](slice-4-assets.md) 保留复用；
最新任务顺序为 4.4 资产／建集、4.5 生成／同步、4.6 扫描／决策，旧 4.3 候选已撤回。
