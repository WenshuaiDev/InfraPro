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


## 业务架构拆分后的部署

业务角色、可执行文件、部署模式和系统账号以 `deploy/architecture/roles.json` 为准。standalone 继续只启用诊断本身。按实际工作负载启用 split 时，用发布包同版本的维护程序生成清单和绑定。下例将容器内报告目录绑定到宿主机的私有卷。

```sh
infrapro diagnostics config init --directory /srv/diag/split --instance-id infrapro-architecture \
  --topology split --profile docker --container-report-directory /var/lib/infrapro-architecture/reports \
  --runtime-uid 10001 --broker-replicas 3
```

| 环境 | 报告与探针 |
| --- | --- |
| native | 默认一个 broker，原生程序以 `/opt/infrapro/current/bin/<role>` 定位。报告默认在 `/run/infrapro-<role>/process.json`。可选 `--bin-directory`、`--report-directory`、`--role-uids <JSON文件>`。如部署了三个 broker，使用 `--broker-replicas 3` 映射至三个不同观察节点 |
| docker | 默认登记三个 `nats-server-1/2/3` 副本，最低健康数为 2。容器观察器按部署和副本两个标签匹配，并校验 PID 命名空间、挂载、UID 和实际程序路径。parser 镜像的入口为 `/parser`。独立边缘 relay 由 `edge` 的原生观察器检查 systemd 程序报告 |
| mixed | 部署默认将 `monitoring-history-worker`、`monitoring-read-worker` 放在容器内，其余程序按原生方式检查，原生 broker 默认为一个。自定义混合部署可传 `--container-roles` 逗号分隔名单。使用 `--container-report-directory` 单独设置容器的宿主挂载根目录，原生 unit 继续使用实际的运行目录 |

`--replica-nodes <JSON文件>` 接受副本 ID 到观察节点 ID 的映射，例如 `{"ingest-relay-1":"edge","nats-server-1":"msg-a","nats-server-2":"msg-b","nats-server-3":"msg-c"}`。不同宿主上的原生 broker 分别检查本机 systemd unit 和 loopback 的 JetStream 健康端点。容器模板可在单机演练时观察三个容器，实际三故障域需要分别配置节点映射并完成资源验收。兼容 API 已启动时加入 `--compat-api`。兼容旧 worker 由原部署 profile 登记，各自只在实际启用时计入连续服务分母。

有多个业务副本时向 `config init` 传入部署生成器使用的 `--replicas <JSON文件>`。每个角色包含有序的副本 ID 或声明，例如 `{"id":"control-api-2","host":"10.24.8.2","diagnostic_node":"app-b"}`。省略 `diagnostic_node` 时，生成器使用合法宿主名或为该地址计算固定节点 ID，并记录在 `layout.json`。诊断 CLI、systemd unit 和应用报告使用同一节点 ID。实际报告节点另有配置时，用 `--replica-nodes` 显式映射。每个副本分别登记进程、报告及源依赖探针。多副本角色按 `/run/infrapro-<副本ID>/process.json` 或指定报告根目录的 `<副本ID>/process.json` 定位。单副本按角色名定位。UID 映射支持角色名和副本 ID，后者优先。回退启用原 `ingest-worker` 和 `monitoring-worker` 后，用 `--compat-workers` 登记两个实际进程。

生成目录中 `bindings.json` 给 server 自有的本机 observer，`observer-bindings.json` 给默认 Docker observer，`observer-<node>-bindings.json` 给其他分配节点。每个 observer 单独签发凭据并绑定 node 和 assignment_revision，沿用既有 TLS 流程。应用的 `INFRAPRO_DIAGNOSTICS_INSTANCE_ID`、`INFRAPRO_DIAGNOSTICS_NODE_ID` 和 `INFRAPRO_DIAGNOSTICS_REPLICA_ID`必须匹配 manifest。容器设置 `INFRAPRO_DIAGNOSTICS_PID_NAMESPACE=container`，应用报告写入 `/run/infrapro-report/process.json`，daemon observer 以只读方式在与宿主一致的绝对路径挂载报告目录。以共享组访问报告时，在业务进程设置 `INFRAPRO_DIAGNOSTICS_REPORT_SHARED_GROUP=true` 并按文件属主和观察者组设置目录访问权。

也可直接将发布生成的 `layout.json` 传给 `--replicas`。使用本机启动器时，选择 `--profile native --local-reports --report-directory "$INFRAPRO_LOCAL_DIRECTORY/reports"`。这会读取启动器的 `<角色或多副本ID>.json`。将 `--runtime-uid` 和 `--bin-directory` 分别设为运行账号 UID 和实际发布二进制目录。

本机单个 broker 通过 `nats-server -P <私有PID文件>` 启动时，可向 `config init` 传 `--broker-pid-file`、`--broker-executable` 和 `--broker-monitor-port`。PID 文件设置为 0600，观察器分别核对实际进程身份和本机 JetStream 健康端点。原生 systemd 单元可直接使用模板默认绑定。

`layout.json` 登记了 `tcp_proxies` 时，诊断会显示 `replica-proxy` 和各个监听。原生／混合部署由 systemd 提供活跃状态、主 PID 和启动时刻。本机启动器使用受保护的 PID 记录、实际程序路径和 UID 匹配 Python 进程。若监听使用本机启动时的端口覆盖，传入已生效的 `--proxy-listeners <文件>`。Python 路径可通过 `--proxy-executable <绝对路径>` 设置。每个 TCP 探针报告该监听能否建立连接，后端业务的就绪由其专属探针核实。

打开每个启用角色的副本与检查。进程存在、报告身份一致和真实循环是两个独立的必需检查。依赖状态来自实际业务调用。MQ 延迟、隔离与 PG 回补见相应事件循环和报告指标；低频兜底继续运行时仍能观察 MQ 故障。事件发布、重复消息、过时代号和定向回补的演练步骤见[消息运维说明](architecture-events.md)。
