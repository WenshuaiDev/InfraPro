# 独立运行诊断


诊断地址、账号、观察器密钥、状态库和生命周期都独立于业务 API。它只能读取配置中登记的目标，不接收浏览器传入的探测地址或控制命令。诊断与业务同宿主时，宿主整体故障仍会使二者同时不可访问；需要异地可用性时，应把 server 放在独立主机。

## 本机人工验收

在仓库根目录执行：

```sh
pnpm test:diagnostics:environment
pnpm diagnostics:lab -- status
```

测试环境构建生产 UI 和 Go 二进制，创建独立 Compose project、随机 loopback 端口和私有临时目录，启动真实业务依赖和全部 10 个 owning process。终端只显示访问地址与 `accounts.json` 的绝对路径；用该文件内的独立账号登录。文件为 0600，父目录为 0700。`.artifacts/diagnostics/active.json` 只保存 run_id 和私有目录位置，不保存密码。

没有 Docker 时可执行 `pnpm test:diagnostics:environment -- --native-only`；此模式验证独立 server、三个原生 observer 和宿主资源，不代表已安装全部原生数据库。完整 L2–L4 自验需要本机 Docker。已有运行中的测试环境 会复用。

人工步骤与预期：

1. 登录后查看运行概览、所有组件、未启用项和 Python 按需工具。期望槽位来自 manifest，进程缺失不会从列表消失。
2. 在节点页打开 `native-a`，进入一个 worker 副本，打开 report 检查抽屉。确认实际 lane、最近尝试、成功、截止时间及下一次计划；按 Escape 返回原按钮。
3. 查看 ClickHouse 两个副本，分母保持为 2。资源页分开显示宿主、容器与未接入的 VM；`logical_test` 不计为三台服务器。
4. 执行 `pnpm diagnostics:lab -- fault business-offline`。业务 web、API、PG 停止后，已有诊断会话仍可访问，新无痕窗口仍可登录，目标失败在 HTTP 200 的证据内呈现。
5. 执行 `pnpm diagnostics:lab -- recover`，等待一个采样周期，核对新证据恢复。每次只注入一种故障，之后先恢复再换下一项。
6. 在有限历史查看变化事件，进入资源卡查看 6 小时趋势。缺口不补零，每分钟最多一个点，没有导出或治理按钮。
7. 缩窄窗口，使用 Tab、Enter、Escape，检查文字状态、焦点、表格水平滚动；刷新不抢焦点。

固定故障：`business-offline`、`postgres-offline`、`worker-stop`、`worker-loop-failure`、`observer-disconnect`、`stale-replay`、`container-restart`、`permission-denied`、`disk-pressure`、`store-unwritable`、`report-corrupt`、`credential-revoke`、`clock-skew`。

`disk-pressure` 只提高本 lab 阈值，用真实磁盘采样触发提示；不写满用户磁盘。`store-unwritable` 使用新的只读目录，完整保留原库。`stale-replay` 和 `clock-skew` 是带标签的故障发布进程，不冒充业务采样。`credential-revoke` 会使当前诊断会话失效；recover 重新启用 lab 用户并轮换对应 observer generation。

```sh
pnpm diagnostics:lab -- recover
pnpm diagnostics:lab -- down
```

recover 可重复执行；down 仅停止拥有匹配 PID 启动身份／run_id 标签的资源并删除本 project volume，保留证据。不会删除其他 Compose project、数据库、用户进程或证书。不要复制活动 lab 的密钥进入真实环境。

## 独立原生部署

```sh
pnpm build:diagnostics
go build -o build/bin/infrapro ./tools/infrapro/cmd/infrapro
build/bin/infrapro diagnostics config init --directory /absolute/new/diagnostics --instance-id production-a --profile native
build/bin/infrapro diagnostics auth init --server /absolute/new/diagnostics/server.json --username operator
build/bin/infrapro diagnostics config validate --server /absolute/new/diagnostics/server.json
build/bin/infrapro-diagnostics server --config /absolute/new/diagnostics/server.json
```

密码通过交互终端输入，不支持参数、环境变量或管道。`config init` 要求新目录，不覆盖旧配置。macOS 可在专用终端中直接运行，Ctrl-C 优雅停止；无需 systemd。默认仅监听 `127.0.0.1:8096`。诊断启动不运行任何业务 migration，不读业务 YAML 或业务身份配置。

模板先把未知业务组件置为 disabled，并说明原因。按实际部署启用 component/replica，声明 required probes 和 node assignment，配置精确路径、UID、进程启动身份、容器标签或 systemd unit。未配置目标不能用模板默认值当作部署已验证。

`config validate --probe` 执行一次固定只读检查，退出码 0 表示配置及全部检查通过、2 表示配置失败、3 表示检查未全部通过。该命令不安装服务，不启动业务，不接受任意 SQL 或命令。

## Compose 与 systemd

交付文件位于 [deploy/diagnostics](../../deploy/diagnostics/)。`native/`、`docker/`、`mixed/` 是严格 JSON 示例；使用 `config init` 为实际 UID 和绝对路径生成配置通常更方便。仓库中的空 auth/keyring 不包含默认凭据。Docker 示例使用 `/etc/infrapro/diagnostics` 和 `/var/lib/infrapro-diagnostics`，需要按实际服务 UID 创建目录、设置文件 0600／目录 0700。

```sh
docker build -f deploy/diagnostics/Dockerfile -t infrapro-diagnostics:local .
docker run --rm --network none infrapro-diagnostics:local version --verify-assets
DIAGNOSTICS_CONFIG_DIR=/absolute/server-config DIAGNOSTICS_STATE_DIR=/absolute/server-state \
  docker compose -p infrapro-diagnostics -f deploy/diagnostics/compose.yaml up -d server
```

docker 示例直接使用 TLS，不经业务 Nginx。配置 `public_origin`、精确 `allowed_hosts` 和证书；示例中的 `example.invalid` 必须替换。证书应覆盖浏览器域名和本地 ready 探针使用的 IP／名称；私有 CA 通过 `tls_ca_file` 提供。默认端口只发布在 loopback；远端访问才按实际安全网络设置 `DIAGNOSTICS_BIND`，始终保留独立身份与 TLS。

server 无 Docker socket、无业务 depends_on，只挂自己的配置和可写状态目录。`observer` profile 是独立低权限网络探测角色；`compose.docker-observer.yaml` 是可选 daemon 观察角色，配置不同的 key、state、UID 和 socket 所属 GID。socket 的只读挂载本身不能限制 Docker API 操作，适配器在代码中只允许固定 GET；不要把该 socket 挂到浏览器 server。只挂该观察器分配的报告目录和凭据。

systemd 两个 unit 分别使用 `infrapro-diagnostics` 与 `infrapro-observer` 用户。先建立专用账号、配置和证书，复制 unit，再执行 `systemctl daemon-reload` 和启用对应 unit。它们只有 `After=network.target`，没有业务 unit 的 Requires/PartOf。读取 systemd 属性需要本机 D-Bus 只读权限；Docker 组权限只授予单独 daemon observer。`PrivateTmp`、`ProtectHome` 等沙箱限制会影响可见路径，应把报告放在专用 `/run` 目录，资源采样只登记可见的必要挂载点。

## 观察器与配置变更

server 管理 manifest 和节点密钥摘要；observer 只持有自己的 key、bindings、manifest 副本和 epoch 状态，不持有人员账号或其他节点密钥。远端 `server_url` 必须 HTTPS，校验 CA 和 hostname，拒绝跳转。协议／主机白名单拒绝 metadata、用户信息和带 query 的目标 URL。

```sh
build/bin/infrapro diagnostics observer-key issue --server /absolute/server.json --node-id node-a --output-file /absolute/new-node-key.json
build/bin/infrapro-diagnostics observer --config /absolute/observer.json
build/bin/infrapro diagnostics observer-key revoke --server /absolute/server.json --node-id node-a
build/bin/infrapro diagnostics observer-key rotate --server /absolute/server.json --node-id node-a --output-file /absolute/new-generation-key.json
```

key 只写到新的 0600 文件，输出不含 secret。issue/rotate/revoke 更新 keyring revision；受控轮换后将新 key 放到该 observer 配置的 credential 文件，保留安全权限。state 丢失／回滚时不要删除 server fence；通过轮换 generation 恢复。保留原 key 文件直到新 generation 配置完成，随后按本地凭据管理流程移除。

manifest 内容改变必须递增 revision；节点分配改变同时递增对应 assignment_revision 并更新 bindings、observer 配置及活动 key 的 assignment。重载约 5 秒；非法内容保留最后有效拓扑并显示配置错误。旧观察和 cursor 失效，新目标从 never 开始。绑定、凭据和账号文件均拒绝未知键、symlink、危险权限及半写文件。

人员账号可本地 `auth add`、`auth reset-password`、`auth set-enabled --enabled false`；修改后在重载边界撤销旧会话，浏览器清空缓存。轮询不续空闲期，受信的人类键盘／指针活动才可续期；30 分钟空闲、8 小时绝对过期。诊断账号没有业务权限，observer token 不能查询人员 API。

## 状态库与恢复

bbolt 独占单 writer，实例 ID、schema、manifest/keyring revision 和 generation/epoch/boot/sequence fence 都有持久约束。历史有限：变化事件 24 小时、分钟资源点 6 小时、默认总库 256 MiB；低于预算时先淘汰历史，保留当前 fence。页面显示实际保留起点与累计淘汰数，不把配置窗口假装成已采满。

库不可写时，受保护页面仍可查询退化原因，本机新样本明确为 volatile，远端写入返回 503。失败写入不确认；旧数据自然过期。恢复约每 15 秒完整校验并实际提交后重新接收。损坏库保留现场，不自动删除、不自动空库、不回滚 fence。

```sh
# 先停止本实例 server；inspect/compact 都需要离线独占锁。
build/bin/infrapro diagnostics state inspect --server /absolute/server.json
build/bin/infrapro diagnostics state compact --server /absolute/server.json
```

compact 使用临时完整库校验、同步和原子替换；保留序号屏障。失败不覆盖原文件。磁盘满先由运维恢复可写空间；确认文件权限、实例 ID 与配置 revision，保留原库副本并用 inspect 核对。若确实无法恢复，应显式建立新 instance_id 和新配置／密钥，不复用旧身份的空库。

升级先保留配置、状态库和旧二进制。执行生产构建和 `version --verify-assets`，停止旧 server 后替换；重启撤销人员／观察器会话，缓存先标 historical，新 attempt 才恢复 fresh。回退只在 schema/protocol major 兼容且保留现有状态和 revision 的前提下替换二进制，不能恢复旧状态库以绕过 fence。没有远程安装或滚动升级调度器。

## 范围与验证解释

完整自验按以下顺序执行。测试环境提供隔离资源；构建检查覆盖全仓静态检查及原生程序，诊断测试另构建 Linux 程序与容器镜像并执行 L1–L4 检查。故障套件会反复停止及恢复本 lab 的服务，不应同时用该 lab 做人工操作。

```sh
pnpm test:diagnostics:environment
pnpm check
pnpm build
pnpm test:diagnostics
pnpm diagnostics:lab -- down
```

单独复查核心契约可执行 `pnpm test:diagnostics -- --core`；浏览器复查用 `pnpm diagnostics:browser`。开发启动用 `pnpm dev:diagnostics`（自动建立独立开发配置；自定义时用 `-- --config /absolute/server.json`，并在该配置显式允许 `http://127.0.0.1:5196`），其 Vite 和 Go 服务独立退出，不参与业务 dev 进程组。

CPU 首样本、计数回绕和无法获取的值为 null；容器 unlimited 不能表示为 0。磁盘仅采集明确声明的挂载点，网卡只采集声明项。logical_test 不提供宿主容量，同一文件系统不会按路径重复汇总。Docker Desktop 的 macOS 宿主与 Linux VM／容器必须分列，未接入 VM 显示缺项。

PG 执行固定 `SELECT 1`，Redis 只 PING，CH 固定 `SELECT 1`，S3 只对指定 bucket 发一次 HEAD，Keeper 只发 ruok。S3 HEAD 的部分实现把错误凭据和缺少权限都返回无 body 的 403，因此显示拒绝访问，不凭空判别根因。CH 固定常量查询不证明业务表授权；权限拒绝契约另以真实用户的固定设置约束检查。依赖网络视角成功不能替代源进程实际调用证据。

L1–L4 自验覆盖本机原生、真实容器、三个观察器 TLS 上报和 Linux 私有 D-Bus 契约。真实独立服务器、真实 systemd unit、其他 Linux 发行版与供应商版本组合、负载和长时间稳定性单列为未验证；私有 D-Bus fixture 不等于真实 systemd 部署。
