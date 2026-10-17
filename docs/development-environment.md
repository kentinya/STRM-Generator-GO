# 开发环境与交接

环境仅写本仓库 `artifacts/dev-env/`；系统没有 Go 时也无需修改全局 PATH 或系统安装。本次宿主为 Debian 12／Linux amd64，Python 3.11.2、Git、Docker 和 Compose 已存在。没有使用同机生产 Jellyfin 或配套项目的运行配置。

## 安装与检查

从仓库根目录执行：

```bash
python3 scripts/development_environment.py install
python3 scripts/development_environment.py doctor
bash scripts/go-local version
python3 scripts/development_environment.py smoke
python3 scripts/quality.py
```

Go 固定1.27.2，`.go-version`、go.mod 与环境规范保持一致。首次安装校验归档 SHA-256，安全解压至 `artifacts/dev-env/sdk/go1.27.2` 并检查版本；已有同版本安装复用。wrapper 禁止自动切换工具链，隔离 GOPATH／GOCACHE／GOMODCACHE，默认 CGO=0，不修改用户 Go 配置。安装器在 artifacts/dev-env 建立独立 go.mod，避免未来根模块 `./...` 遍历 SDK 和 smoke 源码。

官方 tar 端点在本机实测404，改用官方模块代理 ZIP。amd64 与 arm64 归档均已下载、按官方 checksum database 的 h1 比对并固定 SHA-256；tar 官方元数据保留作来源记录。安装器当前使用规范中的 ZIP，失败退出2，不自动降级或忽略校验。

离线安装复制目标平台对应的同一官方 ZIP，再运行：

```bash
python3 scripts/development_environment.py install --sdk-archive /path/to/v0.0.1-go1.27.2.linux-amd64.zip
```

smoke 在忽略目录生成标准库 Go 样例，运行 JSON Int64／net/http／slog 编译检查，分别构建 Linux amd64／arm64。ARM64产物没有在本机执行。没有产品包时 quality 显式报告产品 Go build/test/vet 为 NOT RUN；以后有包自动启用常规门槛，具体集成／T4要求仍由Task提供。

尚无 SQLite／YAML 第三方依赖，故当前没有 go.sum。SQLite驱动、YAML库、C编译器、race测试、正式打包和产品Dockerfile由活动Task确定，不能把编译 smoke 当成数据库或运行时兼容证明。CI定义已添加，尚未推送／远程运行。

## 原生 OpenList 夹具

```bash
python3 scripts/mock_openlist.py
```

默认只监听 `127.0.0.1:15244`，Ctrl-C停止。无数据库、无外部网络请求、无媒体下载，使用公开假 token `strmgen-fixture-only`。请求示例：

```bash
curl --fail http://127.0.0.1:15244/health
curl --fail http://127.0.0.1:15244/api/fs/list \
  -H 'Authorization: strmgen-fixture-only' \
  -H 'Content-Type: application/json' \
  -d '{"path":"/Movies","page":1,"per_page":2,"refresh":false}'
```

夹具数据位于 [openlist.json](../dev/fixtures/openlist.json)，覆盖分页、目录、中文特殊名、有效／缺失／无效SHA-1与大Size。list/get支持合成 `/error`（HTTP200/code500）、`/forbidden`（HTTP200/code403）、`/offline`（HTTP503）、不存在路径（HTTP200/code404）。这些错误码只为测试工具分支，不宣称真实驱动使用相同不存在语义。

get.raw_url 故意携带假签名，供未来验证不能复制该地址。哈希及文件是合成元数据，夹具没有媒体字节／`/d/`播放端点，不能用于播放或真实内容哈希验证。它不覆盖真实限流、token续期、驱动缓存或所有API字段。

目标 [配置示例](../config.example.yaml) 仅落在本地 artifacts 下，public_url 使用不可解析的示例域；真实配置复制为忽略的 config.yaml，再设置真实环境变量。不要将生产目录替换进夹具。当前没有CLI，示例不能直接执行同步。

## 可选 Docker Compose 夹具

先停止原生夹具，避免端口冲突；冲突时直接失败，不停止占用端口的其他服务。

```bash
docker compose -f dev/compose.yaml config --quiet
docker compose -f dev/compose.yaml up -d --wait
docker compose -f dev/compose.yaml ps
docker compose -f dev/compose.yaml stop
```

专用 project `strm-generator-dev`，仅发布本机15244；只读挂载夹具脚本和数据、非root、固定image digest、128MB限制。不挂载生产路径、Docker socket或Jellyfin数据。可用普通项目网络，不承诺阻断出网；程序本身不向外发请求。停止保留项目资源，不提供全局清理。另一工作副本应通过 `-p` 指定独立project和调整端口，避免共享同名项目。

固定镜像当前仅在amd64运行验证，ARM64主机可使用原生Python夹具；不能把Go的ARM64编译结论当作该镜像支持证明。后续真实OpenList／Jellyfin集成须另建固定隔离环境，不启动或接管同机生产容器。

## Git 与证据

本目录原先只有方案，无Git。用户提供身份后，已仅在本仓库配置 `kentinya <szd123956@outlook.com>`，开发准备文件保存为main的首个本地基线提交；提交号可通过 `git log --reverse --format='%H %s'` 查看。未配置远程，未推送。原始方案的Markdown换行空格按文件属性保留，内容摘要不变。

首次尚无HEAD的初始化阶段只使用治理／quality的 `--bootstrap`；基线提交后使用常规检查。当前Slice仍为PLANNED，Base／Implementation Head保持NOT SET；产品授权后由A选择真实基线并提交ACTIVE合同，不能把Git身份配置或初始化提交当成产品激活。

本机验证输出保存在 `artifacts/validation-evidence/preparation-20261017/`，包括输入SHA-256、精确命令、退出码与日志。文件被Git忽略，仅本机可读。当前交接事实见 [进度](progress.md)；没有产品代码、真实服务、性能或独立审查PASS。
