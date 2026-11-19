# 拆分架构的安装、切流及联合恢复

本手册用于 schema 59。升级前的业务基线由 `220aee11790d900af823b05b70796da1a4733c79` 构建，数据库版本为 58。用户完成隔离环境的人工接受后再安排生产窗口。原地址、静态应用路径、三处大屏 origin 和人员会话的配置沿用当前业务版本。

## 构建离线发行物

使用当前 Go workspace、仓库锁定版本的 pnpm 和 Node 构建。在构建机提前安装 `.go-version` 指定的工具链，按锁文件填充 pnpm store 和 Go 模块缓存（包括 broker 依赖）。broker 版本与模块摘要见 [版本锁](../../deploy/events/versions.json)。

```sh
node scripts/architecture-deploy.mjs build-release --target linux/amd64 --directory "$RELEASE" --parser-package "$RETAINED_METHOD_PACKAGE"
```

该命令将 Git 已跟踪文件和未忽略的新源码复制到临时私有目录，校验内容、符号链接与执行位后固定输入。三个 App、目标 Go 二进制和 broker 均从这份快照离线编译，成功后原子发布到空目录。已有经过外部流水线校验的二进制时，可用 `release --bin "$BIN" --broker "$BROKER" --directory "$RELEASE"` 组包。全新环境可以省略 `--parser-package`。升级环境应传入现用方法包，构建器先保留固定版本和旧 runner，再打入本版新增的 runner。包内包含三个 App、独立 CLI、parser／relay、空间转换脚本、部署模板和角色策略。允许随包交付的源码资源逐项登记在 deploy/architecture/offline-members.json，验收器会将包内目录与它逐文件对照。`release.json` 逐文件记录路径、字节数和 SHA-256，记录基准提交、是否有工作区修改、实际源树摘要、schema、目标 OS／架构。在目标侧以 `node scripts/architecture-deploy.mjs verify-release --directory "$RELEASE" --manifest-sha256 "$TRUSTED_HASH"` 核对所有成员后将发行目录只读挂载。按当前空间依赖锁预装 Python 转换环境。生产保持已选的单 ClickHouse 拓扑。broker 三副本需要独立故障域、容量和 I/O 实测，每机独立文件卷。原有业务和备份主机的余量须计入节点资源核算。

在准备了冻结版数据和候选发行物的隔离环境，运行 `node scripts/architecture-offline-check.mjs --output "$RUN" --host-release "$HOST_RELEASE"`。脚本从冻结源码复原已发布方法包，为目标平台构建并逐件核验发行物。它对主机上参与验收的所有二进制重新计算 SHA-256，同时要求两个平台的软件输入摘要、控制台、大屏运行壳和诊断静态文件逐项相等。结果在 `deploy/offline-release.json`。安装前再次校验已生成的包时，传入 `--bundle "$RELEASE" --manifest-sha256 "$TRUSTED_HASH"`。

## 本机、原生、Compose 和混合

```sh
pnpm setup:local --split
INFRAPRO_ROLE_CONFIG_ROOT="$HOME/.local/share/infrapro/development/split" pnpm dev
node scripts/architecture-deploy.mjs prepare --mode native --directory "$NATIVE"
node scripts/architecture-deploy.mjs prepare --mode compose --directory "$COMPOSE" --network "$PRIVATE_NETWORK"
node scripts/architecture-deploy.mjs prepare --mode mixed --container-roles monitoring-history-worker,monitoring-read-worker --directory "$MIXED"
```

每次 prepare 使用空目录。原生按照生成的 units 安装发行物到 `/opt/infrapro/current`，将各角色的专属配置安装到 `/etc/infrapro/roles/<角色>`，账户名使用角色清单的 `systemUser`。文件 worker、checker 和使用文件暂存的 HTTP 角色共享 `infrapro-files` 组和私有 socket 目录。按运营边界授权备份助手启停已登记的本机单元。parser、relay、独立诊断分别保留自己的 systemd 入口。大屏壳的 `display.shell_directory` 指向发行物的 `web/display-runtime`。

Compose 生成 `compose.json` 和三 origin Nginx 配置。默认只发布 Web 端口，控制台／运行壳／资产域名解析到该入口。将三个原域名和 TLS 终止方式配置为当前部署值。如使用现有 HTTPS 入口，可继续使用随包的 HTTPS Nginx 示例。私有后端使用服务 DNS。将各角色 runtime.yaml、runtime.env 和 tls 目录设为该进程可读，将该角色报告目录和暂存目录设为指定 UID 可写。容器默认 UID/GID 为 10001。配置目录内的密码、TLS 私钥、对象凭证权限为 0600。诊断 observer 读取单独挂载的报告目录。容器 node 默认 docker-observer。自定义主机节点须让进程中的 INFRAPRO_DIAGNOSTICS_NODE_ID 与生成探针时 --replica-nodes 的映射相同。`--network` 引用事先存在的私有 Docker 网络，网络内解析 PG、Redis、CH、对象和 parser 的地址。混合模板中的容器采用 Linux host 网络，本机 systemd 与容器后端共享主机的私有监听地址。

```sh
docker build -f deploy/docker/api.Dockerfile -t infrapro-api:architecture .
docker build -f deploy/docker/infrapro.Dockerfile -t infrapro-ops:architecture .
docker build -f deploy/docker/display.Dockerfile -t infrapro-display:architecture .
docker build -f deploy/docker/web.Dockerfile -t infrapro-web:architecture .
docker build -f deploy/docker/spatial.Dockerfile -t infrapro-spatial:architecture .
docker build -f "$RELEASE/deploy/docker/broker.Dockerfile" -t infrapro-broker:architecture "$RELEASE"
docker compose -f "$COMPOSE/compose.json" config --quiet
docker compose -f "$COMPOSE/compose.json" up -d
docker compose -f "$COMPOSE/compose.json" --profile broker up -d
```

既有 Alpine API、运维 CLI 和大屏镜像中的 `infrapro` 使用 UID 100、GID 101。升级并复用原 MFA 密钥卷时，沿用该属主和 0600 权限。常规 API 与运维镜像默认使用此身份。新拆分 Compose 如果直接挂载原凭证卷，在该项目的 .env 同时设置 `INFRAPRO_SERVICE_UID=100`、`INFRAPRO_SERVICE_GID=101`，按此身份配置新报告及暂存目录，并把诊断探针的 `--runtime-uid` 设为 100。使用自定义应用属主的现场取原文件的实际数字 UID/GID。离线目标对同一密钥核对文件摘要和数字属主，再启动 identity-api 或同 schema 的 compat-api。

给已启用域单独启动对应的 worker、运行壳和资产进程。过渡期 `--profile rollback` 提供同 schema 的兼容进程。全新部署只按已启用域配置。在恢复业务前逐个确认其存储和程序包的就绪状态。故障时通过私有探针、进程报告及域收据判断受影响的入口。

## 各角色的依赖和资源

[角色清单](../../deploy/architecture/roles.json) 记录二进制、调用方、连接目的地、PG 角色、对象前缀和允许动作，以及 CPU、内存、临时盘、连接、在途数与关停时间。`layout.json` 的 `instances` 列出每个副本的实际额度。将同角色各槽的 PG 数量相加即为该角色总额。维护、备份、兼容重叠和故障接管需要另计余量，云中继的独立数据库单独核算。使用第 9 节完整负载、数据和同一配额测量现场资源。

增量计算和历史回填发布衍生结果时，共用租户的提交位置。数据库只允许两个角色更新 `commit_sequence`，并按本次已封存的计算结果核对通道、发布位置和输出范围。自动计算故障由增量进程记录，它只能锁定、合并和恢复 `derivedmonitoring` 的通知条件。部署自验在受限角色下实际执行这些操作，并验证越权写入返回 PostgreSQL `42501`。结果见 `deploy/domain-privileges.json`。

身份服务新建项目时写入租户和项目 ID。数据库设置首版资源策略的修订号为 1，并关闭设施创建。传输服务下载审计导出时会锁定导出、代次与对应对象。正文缺失或完整性校验失败时，传输服务可以把可读对象标记为缺失。使用已签发的传输登录身份，对象标回可读、替换摘要、改键或推进代次均返回 `42501`。部署检查把实际登录、三表锁定、事务回滚和拒绝的语句类别记录在 `deploy/domain-privileges.json` 的 `domainPaths.auditDownload`。

审计下载的临时磁盘按节点登记预留字节。传输服务在关闭对象后释放预留。相同预留再次执行释放命令会更新零行。数据库拒绝该角色把已释放用量改回在用、重写释放时间、修改预留字节或延长有效期。验收记录包含实际预留字节和关闭后的用量。

控制服务准备通知摘要时按邀请 ID、邀请人 ID 核对归属。审计 worker 回读分类发布事件时，通过已发布词典的 ID、版本号、正文、摘要、发布时间和原因重建当时的展示内容。回读工程事件时，按租户和设施查询工程结构视图。角色验收会核对结果，并检查邀请令牌与备注、词典草稿和发布人等私有字段的数据库拒绝记录。

### 每个副本的连接与在途上限

`prepare` 会生成 `resource-limits.json`。每行列出副本的 PG 连接数、物理 ClickHouse 池个数、每池与合计最大连接数、HTTP 同时处理数以及该角色的作业限制。生成器按实际挂载的 `architecture.json` 读取本槽在声明顺序中的份额。`slot_max_inflight` 是现有程序所读的配置份额。`http_inflight` 仅统计装有入口限流器的进程。解析、作业领取和本地文件检查另见 `work_limits`。带 `shared` 或 `per_owner` 的作业数由各槽共用。CPU、内存、暂存、网络和关停参数见同目录 `layout.json` 与角色清单。

生产默认清单按单物理端点和每角色一个副本计算。下表为程序在该配置下的上限。根据实际启用的角色合计，额外启动同库兼容进程时单独计入其池。

| 角色 | PG | ClickHouse | HTTP 同时处理 |
|---|---:|---:|---:|
| api | 0 | 0 | 92 |
| identity-api | 8 | 0 | 24 |
| control-api | 12 | 6 | 32 |
| monitoring-query-api | 10 | 6 | 24 |
| transfer-api | 10 | 6 | 12 |
| monitoring-publish-worker | 10 | 2 | — |
| monitoring-incremental-worker | 10 | 3 | 3，内部试算 |
| monitoring-history-worker | 6 | 2 | — |
| monitoring-read-worker | 6 | 2 | — |
| display-runtime / display-assets | 8 / 4 | 0 / 0 | 24 / 16 |
| 同库回退 compat-api | 12 | 6 | 24 |
| 同库回退 monitoring-worker / ingest-worker | 12 / 12 | 4 / 0 | — |

若现场调整 `max_inflight`，将角色总额写入一个 JSON 对象，传给 `prepare --max-inflight 文件`。例如 `{"transfer-api":2,"monitoring-incremental-worker":1}` 会在本例单端点、单槽下分别生成两条和一条 CH 连接，两处 HTTP 入口上限也分别变成二和一。网关取自身份额和四个下游份额之和中的较小值。自定义额度随角色私有配置及本机 TCP 代理一起生成。新分布应按现场吞吐、延迟和隔离结果验收。

每个配置的本地副本、每个物理分片的每个副本各产生一个连接池。同一台机器上两个分片数据库也占两个池。若本槽在途份额为 `H`、池数为 `E`，每池开到 `min(6, floor(H/E))` 条，角色本槽合计为池数乘以该值。余数留在池外。每个启用 CH 的槽至少须满足 `H >= E`。原 schema 58 的 API、监测 worker、大屏运行壳、接入 worker 和空间 worker 若获配监测存储，各为每池六条，原始无架构入口没有该 HTTP 限流器。原始大屏池以 `display.pool_size` 为准。运维 CLI 的 CH 操作也按每池六条单独列出。独立云中继的 spool 连接在 `independent_edge` 计入。

发布时在受保护目录提供进程实际配置的 JSON 或其资源字段投影。可直接传入发行模板使用的 JSON 格式 `runtime.yaml`。输入读取 `monitoring_clickhouse_replicas` 和 `monitoring_physical_shards` 的原有表示。需要按进程区别时用 `{"defaults":{…},"slots":{"transfer-west":{…}},"schema58":{"spatial-worker":{…}}}`。清单记录分组和数量。现有 YAML 包含锚点或进程环境覆盖时，提供合并后的同名字段。显式空副本列表表示该配置关闭监测存储。未提供现场配置时默认生产示例标记为 `production-single-endpoint`。

`resource-limits` 会重新读取已挂载的角色配置，并把结果原子写回同目录。可选报告来自对应槽实际运行时生成的自报文件。命令检查副本、进程身份、就绪状态和 `quota.ch_connections` 等已安装指标。输出保留核对时的 PID、启动标识、时间及驱动实际额度。报告尚未提供的槽只列配置值。角色散布在多台主机时，将报告路径按副本 ID 写成 JSON，传入 `--report-map 文件`。

```sh
node scripts/architecture-deploy.mjs resource-limits --directory "$NATIVE" --runtime-config "$RESOLVED_RUNTIME" --reports-directory /run
node --test scripts/architecture-capacities.test.mjs
```

### CPU 与运行时内存

向 `prepare` 或 `pnpm setup:local --split` 传入 `--process-budgets 文件`。每个角色可以声明总 CPU 核数 `cpu`、进程内存上限 `memory_mib`、Go 调度线程数 `go_threads`、Go 运行时软内存目标 `go_memory_mib`。多槽按声明顺序分摊总额。原生单元用 `CPUQuota`、`MemoryMax` 约束整个进程，Compose 用 `cpus`、`mem_limit` 实施相同限制。混合模式使用各进程所选运行方式的约束。直接在 macOS 上运行的本机进程获得 `GOMAXPROCS` 和 `GOMEMLIMIT`，进程实际 RSS 可超过 Go 的软内存目标。开发监督器与预览启动器读取同一份槽配置。

大屏负载配置包含 A 的三个原进程和 B 的十一个拆分进程。两侧各分配 13 个 Go 调度线程、2,048 MiB Go 运行时软内存、13 核 CPU 和 2,560 MiB 进程内存上限。设置同时为身份与查询各保留两个调度线程，保证传输的两个入口位置可同时占用。此配置供规格第 9 节的隔离复验。发布机器的实配与诊断取样应另外保留。测试宿主的 `hw.physicalcpu`、`hw.logicalcpu` 实测均为 14，内存 36 GiB。[CPU 与内存分配](../../deploy/architecture/display-processes.example.json) · [在途分配](../../deploy/architecture/display-inflight.example.json)

```sh
node scripts/architecture-deploy.mjs prepare --mode native --directory "$LAYOUT" \
  --process-budgets deploy/architecture/display-processes.example.json \
  --max-inflight deploy/architecture/display-inflight.example.json
```

B 各槽的实际配置与合计在 `resource-limits.json` 的 `process_resources`、`selected_process_profiles` 中。S0 原单元可安装 `schema58/systemd/<unit>.service.d/30-resource-limits.conf`。原 Compose 服务可叠加 `schema58/compose-limits.json`，直接启动 S0 进程时使用 `schema58/environment/<role>.env`。依原大屏配置设置 `display.pool_size`。第 9 节对照单独使用限制到 48 条 PG 连接的逐角色测试登录、实际 18 条 CH 池上限和 16 个对象请求名额。其登录签发记录、操作系统在运行 PID 上读出的调度参数、实际套接字和执行结果随该轮原始取证保存。常规发行角色的 PG 登录预算以该环境生成并签发的清单为准。

### 多副本配置

向 prepare 传入 `--replicas replicas.json`。本机开发可用 `pnpm setup:local --split --replicas replicas.json`。每个副本的槽名在清单内全局唯一。下例把身份服务的八条 PG 连接分为四条和四条，解析执行器的四个并发位置分为两个、一个和一个。

每个需要 ClickHouse 的进程至少要有一条到每个物理端点的连接。在多端点测试环境中，该角色用于 ClickHouse 的总额度至少为配置槽数乘以物理端点数，保证每个独立连接池能够就绪。生产的一期配置使用一个物理端点。

```json
{
  "identity-api": ["identity-east", "identity-west"],
  "parser": ["parser-east", "parser-central", "parser-west"]
}
```

分配顺序决定余数。每槽具备独立的报告 ID、配置目录、机器证书和数据库登录。跨主机安装时使用形如 `{"id":"history-west","host":"worker-west.internal","diagnostic_node":"west"}` 的副本声明。每个槽的实际节点名写入 layout.json 的 diagnostic_node，供启动器和诊断 observer 同时读取。原生部署为 `instances[].systemUser` 创建系统账户，将 `instances[].directory` 保持原相对路径安装在 /etc/infrapro 下，顶层 /etc/infrapro 及中间目录 roles、replicas、replicas/<角色> 由 root 持有，使用 0711 允许各槽遍历到自身路径。每个最终槽目录设置为该账户独享的 0700，私钥使用 0600。TCP 代理配置仅含监听和后端地址，安装为 root 持有的 0644 文件。TLS 叶证书含角色与槽位两个 DNS SAN。例如身份服务第一个副本需要 `identity-api.infrapro.internal` 和 `identity-east.infrapro.internal`。本机生成器自动签发测试证书。将 `layout.json` 安装到备份助手的 `config/architecture/layout.json`，保留已部署的副本名单和冻结次序。

Compose 在私有网络为各槽创建单独容器和固定容器名，各槽共享原角色的 DNS 别名。扩容时在清单内划分新槽，再生成对应服务。原生与混合部署使用 `replica-proxy.json` 保持客户端原地址，将 TCP 流量转交到各槽的独立端口并保留端到端 mTLS。在入口主机安装生成的 infrapro-replica-proxy.service，将 `replica-proxy.json` 放入 /etc/infrapro，启用 Python 3.8 或更新版本。每个角色的槽位都同机时保留默认 loopback 地址。跨主机的槽位可写成 `{"id":"identity-west","host":"10.0.0.12"}`，把 prepare 的 `--host` 设为入口主机的私有地址，并限制各后端端口仅对入口与授权诊断来源开放。通过 `--public-listeners 文件` 覆盖网关、大屏两个 origin 和 parser 原有的监听地址，JSON 使用角色到 host:port 的映射。将独立 parser 的工作包和只读 token 装入其配置文件指定的位置。

在应用数据库完成 schema 59 迁移后，以维护身份执行下面的命令，为本次清单逐槽签发 PostgreSQL LOGIN，写入角色私有配置。PostgreSQL 的 CONNECTION LIMIT 与应用池使用相同的物理额度。已运行的进程占用其槽，重复启动同一份角色配置时由业务连接上的 PG 锁拒绝。退出释放最后一条连接后可以接替。网关使用端口和独立证书启动。变更清单时保留仍在用槽的受保护 environment.json，先排空将下线的副本，再追加 `--retire-slots`，签发器会拒绝仍有连接的旧槽，并在同一事务内禁用已撤下的登录。

```sh
node scripts/architecture-postgres.mjs --issue-logins "$LAYOUT"
```

每个云中继拥有一套独立的 RelayID、spool 和登录。多台边缘安装时传入 `--relay-configs 文件`，将各槽名映射到其现用 relay.json 的受保护路径。将生成的每份 relay.json 与 edge/units 中对应单元发至各自边缘节点，在该槽的云库上执行同目录 postgres-limit.sql。生成器保留 relay 身份与证书位置，限定池大小和登录上限，并为既有 spool 独占锁保留连接。单槽首次安装可从 relay.example.json 设置实际的云库、身份与证书。

| 进程 | 监听和访问边界 |
| --- | --- |
| api | 现有 HTTP 对外入口。私有 mTLS 探针使用 endpoints.api。公开代理只转发既有业务路由。网关没有业务库或身份密钥 |
| 四个 HTTP owner | endpoints 中分别配置 HTTPS 地址。服务证书 DNS SAN 为 `<角色>.infrapro.internal`。不同角色独立私钥，CA 私钥离线保存。identity 拥有 MFA 密钥及会话库，其他角色仅获取完成事务复验所需状态 |
| display-runtime、display-assets | 分别绑定原壳与资产地址，运行壳通过私有 API 请求受限查询。资产代理剥离 Cookie 和 Authorization，资产角色的对象身份只读取已封存包 |
| 监测与解析 | 试算端口只接受 control-api 的 mTLS 调用。历史、后台查询、发布与增量分别使用独立连接池及领取预算，执行领取同时核验 PG 共享预算与代次。parser 和 runner 使用固定方法包，接管角色保持 relay、收据和对象所有权 |
| 其他 worker | 索引、通知、审计有独立连接和工作预算。文件 checker 仅绑定本机 socket，空间转换脚本使用原沙箱配额 |
| event-relay 与 broker | relay 使用专用 PG 与发布身份。每个 broker 的 4222、6222 只允许应用和成员网络，8222 只监听 broker 本机。每个角色独立 mTLS 身份和主题权限。只在诊断 observer 上提供只读状态 |

业务角色配置通过 `INFRAPRO_CONFIG`、`INFRAPRO_ARCHITECTURE_FILE` 和需要消息的角色自己的 `INFRAPRO_EVENTS_FILE` 挂载。生产 CA 颁发服务端／客户端用途证书，信任链文件名为 `tls/ca.pem`，证书和私钥分别为 `tls/<角色>.pem` 和 `tls/<角色>.key`。测试可用 `--local-ca`，私有 probe 的观察角色证书为 `diagnostics-observer`。七个内网端口的默认值为 18441 至 18447。跨主机时逐个改为私有 DNS 地址。容器与原生的生成配置均可调整独立监听和资源。模拟数据以外的环境应从密码管理系统发放独立登录，按各角色生成的 `objects.iam.json` 替换桶名，在对象存储授予所列前缀，再配置对应域的连接。

已有接入入口可以在 prepare 时传入 `--ingestion "$RELAY_PROFILE"`。生成器为控制 API 保留节点 ID 与配置引用，解析调度和传输仅接收域预算，设备拉取与控制客户端的证书路径仅出现在接管进程和回退接入 worker 的配置中。将这些证书和密钥按所列绝对路径单独挂载到相应进程。对象读取就绪通过命名空间内的有界列举检测，S3 地域查询仅授予对应桶。

生成权限后，运行 `python3 scripts/architecture-object-profile-check.py --layout "$NATIVE" --output "$EVIDENCE/object-privileges.json"`。此命令在独立的临时对象存储签发六个角色的测试身份，实际执行 24 次签名请求。验收记录包含各角色的对象读取、写入、删除和前缀列举结果。结果应显示封存大屏包可读、其他大屏前缀被拒绝、接管角色可处置原文、查询和传输角色无权改写原文。

prepare 的 `--clickhouse-database NAME` 指定本环境的监测分析库。省略时需先将生成 SQL 中的 `REPLACE_MONITORING_DATABASE` 换为实际库名。用维护身份完成物理存储初始化，再对每个 ClickHouse 副本和计算分组执行角色目录中的 `clickhouse.sql`。数据库及角色均以环境库名限定。为每个进程发放独立的 ClickHouse 登录，只授予该文件创建的角色，通过本角色的 `monitoring_clickhouse_user` 和 `monitoring_clickhouse_password` 挂载凭据。读取者能查询正式数据和就绪所需元数据。发布角色可以写原始表，计算增量及历史角色可以写成果表，发布角色验证后可清除已取消选择的计算尝试。schema 建立、备份及恢复仍通过维护登录运行。联合演练会用实际角色登录读取、尝试零行写入并检查禁止的 DDL。

公开回放生成派生、空间和分析样本后，运行 `node scripts/architecture-display-scope-permissions.mjs "$RUN"`。它在原 PostgreSQL 实例上克隆样本，迁移到发行版要求的 schema，再使用部署器签发的两个独立账号。`deploy/display-scope-permissions.json` 记录运行服务解析出的工程、设备、监测、空间成员摘要、递归通道资格，以及资产账号的版本核验和私有列拒绝。`deploy/display-artifact-permissions.json` 记录一个固定分析版本的读取资格、实际大屏绑定、终端首次启动、来源撤权后的配对与续期结果，以及 ClickHouse 对物理观察表的访问结果。每个临时库与登录在用例退出时清理。可用 `--scopes-only` 或 `--artifacts-only` 重放对应部分。

## 数据库升级及兼容窗口

放行发布 worker 前，用它实际领取到的 PostgreSQL 和 ClickHouse 登录核对计算目录版本、权限及原始／计算表，运行 `infrapro monitoring storage check`。库表结构、只读副本状态或访问权限未就绪时，保留当前领取归属。隔离演练的 `role-cold-start.json` 同时记录预检时刻和真实进程就绪结果。

早期隔离环境即使显示 schema 59，也应先运行本次发行的 `infrapro database migrate`，再安装角色权限。迁移会在同一 schema 的事务锁内检查固定基准和个人租户邀请的只读函数，缺失时补装，保留现有事件恢复代号、租约和队列。大屏可信运行账号通过固定基准函数核对原证据摘要，只能取得快照元数据和输入时间范围。固定成果配对、来源撤权与续期的实测结果见 `deploy/display-artifact-permissions.json`。

通知收件箱使用 `control-api` 的数据库登录读取自己的个人通知。数据库按接收账号、租户和真实通知事件核对邀请身份，已接受的邀请仅在原成员关系和租户仍有效时返回接受时的租户名称。邀请令牌、完整接受快照与关闭原因由身份服务管理。可用 `pnpm test:notifications` 运行真实受限登录的过期、改名、停用和撤权用例。

从 `pg_dump --no-acl` 还原到新 PostgreSQL 集群时，原集群的 NOLOGIN 角色和函数授权需要重新装配。升级命令支持先还原 schema 58 的业务数据再签发角色。还原 schema 59 后，升级命令收回 PostgreSQL 默认给 `PUBLIC` 的两个元数据函数执行权，随后部署权限脚本向对应角色授予调用权。`node scripts/architecture-cold-grants.mjs "$RUN"` 在新建的 PostgreSQL 17 实例重走两种还原，逐项核对账号摘要、事件恢复代号，以及审计、大屏、收件箱和普通账号的实际执行权。记录位于 `$RUN/deploy/cold-restore-grants.json`。

1. 记录已受理命令、relay 两端收据、发布位置、在途作业、对象清单及会话状态。封存当前业务发行物和通过验证的兼容发行物。先关闭新写入及领取，核对在途结果。若在 schema 58 上放弃升级，可恢复冻结版原拓扑。
2. 使用维护身份运行 `infrapro database migrate`，再执行 `infrapro access sync`、`infrapro access check` 和 `node scripts/architecture-postgres.mjs`。后者使用 `INFRAPRO_DATABASE_URL` 及可选本地 PG 工具容器。先恢复原域的角色权限，再安装验包角色、拆分角色和事件授权。数据库升级、应用登录、运维登录三类身份分别管理。账号只加入角色清单指定的 NOLOGIN 组。`infrapro_event_admin` 用于有审计记录的切换和恢复。目标库必须支持本版本的精确 schema 检查。
3. 使用同 schema 的兼容 API 及原 worker 确认旧数据、会话、权限、未完成任务和业务入口，恢复受控窗口。旧 58 二进制访问 59 必须明确退出。旧 relay 协议仍可在自身保管预算内缓存上报。
4. 先测试只读操作，再按 owner 变更网关 `roles/api/architecture.json` 的 `route_targets`，例如 `{"identity-api":"compat-api"}`。原子替换文件，逐副本优雅重启网关。删除对应覆盖项后该组回到拆分进程。带会话活动、查询快照、传输登记等副作用的请求仅有一个执行者。冷却期保留两种拓扑、旧对象和旧载荷。
5. 在受限维护环境运行 `event-relay status`，记录 `restore_epoch`、逐消费者位置、每个 pool 的 `generation`。通过 `event-relay switch --role ROLE --expected GENERATION --mode draining --epoch EPOCH --reason REASON` 停止新领取，在途完成或按租约安全转交后再按最新代次切换为 `split`。回退同样先 draining，再选择 `compat`。所有操作使用返回的当前代次。原 worker 首先接管已有待办，其后再开启所归属的公开路由。
6. broker 每节点使用独立的配置、服务器私钥、节点互信私钥和持久卷。用 `node deploy/events/render.mjs PROFILE EPOCH OUT` 为每台生成 `nats.conf`，再用事件运维身份运行 `event-relay provision` 和 `event-relay enable --epoch EPOCH --reason REASON`。程序消费使用私有客户端配置，`mode=pg` 或 `mode=jetstream` 选择原事实对账或消息唤醒。每次切换只处理所属作业／位置，保留未发送及隔离记录。角色积压、PG/CH/S3 限额及 hard limit 见诊断和事件状态。

切回兼容 worker 前，核对已安装的事件权限脚本摘要与此次验收的 `deploy/rollback-readback.json` 一致。兼容身份只能按当前归属重取同一恢复代号和分区的旧租约。续领写入的代次必须是当前代次。按原请求 ID 回查切换时已排队的作业，确认它以原作业 ID 在新代次下完成。验收报告按三个切换时点保存 GUI 截图、SDK 和 CLI 读回及每个副作用的业务键。

## 联合备份和恢复放行

安装匹配发行物的 [备份协调器](../../deploy/production/production-backup.py)、[每机助手](../../deploy/production/backup-node.py)、[清单读取器](../../deploy/production/checkpoint_roles.py) 和 [对象归档库](../../deploy/production/s3_archive.py)。每机原生服务通过真实 systemd 单元发现。容器主机的 `config/architecture/checkpoint.json` 设置 `docker=true`、实际 `instance`、broker 的 `jetstream_monitor_port` 和 `jetstream_store_dir`。自定义原生命名在 `units` 显式映射到角色。仅允许清单内角色、实际存在的单元及相同实例的容器。发布期间与调度器共用受控窗口保持冻结成员稳定。四机保留现有 SSH 强制命令、备份流量限制、容量和保留策略。

协调器顺序停止入口、relay、业务与后台写入、消息发布消费、checker、broker 及独立诊断。按实际初态记录停止服务和时点。采集两端收据、event 状态、PG／CH 快照、对象清单、固定程序包、运行设置、受保护密钥、诊断和各 broker 的流／消费者及离线卷。核对冻结状态后按逆序恢复原先活动的实例。完整归档通过仓库回执的字节数／摘要后才进入保留轮换。恢复点和密钥只在受保护存储中传递。

```sh
python3 deploy/production/architecture-restore.py verify --checkpoint "$CHECKPOINT" --manifest-sha256 "$VERIFIED_HASH"
python3 deploy/production/architecture-restore.py apply --checkpoint "$CHECKPOINT" --manifest-sha256 "$VERIFIED_HASH" --target "$TARGET" --reason '独立恢复演练'
python3 deploy/production/architecture-restore.py open --checkpoint "$CHECKPOINT" --manifest-sha256 "$VERIFIED_HASH" --target "$TARGET" --reason '完成逐域对账'
```

`VERIFIED_HASH` 从已核对外层归档回执的 `archive-manifest.json` 取得。目标配置见 [恢复目标示例](../../deploy/production/architecture-restore.example.json)。prepare/stop/frozen 先在恢复目标逐机检查，`writer_hosts` 填每台目标记录及其批次。原生产地址、角色登录、旧 broker 工作卷和旧消息主题仅作受保护的对账资料。为恢复目标分别提供独立的业务 PG、relay PG、审计内容核验只读账户、CH 测量表、源文件桶、各私有桶和备份暂存前缀。应用域恢复材料按同一方法包和密钥版本提供。目标系统管理员保管数据库维护身份、各域运维角色和读写测试桶的凭证。

恢复器在业务库进入已验证的 `read_only` 状态后，调用目标发行物的 `infrapro database migrate`，使早期同版本备份中的函数符合当前 schema 59。`recovery.json` 的 `steps.role_grants.result.schema_installation` 保存迁移 CLI 的 SHA-256、迁移前后备份 ID、访问撤销状态、四个域的恢复代号及函数签名。查证这些状态与备份相符后，恢复器依序重装四份权限脚本并记录各自 SHA-256。

恢复先核对每个成员的摘要、每台冻结清单和两个域引用的控制库备份。监测恢复通过原守卫恢复 PG、CH、源对象与观察结果，撤销旧会话和旧凭据。附件、审计、relay 和事件分别使用各域恢复方式重建代号和可恢复状态。大屏先按冻结的 `display-files.json` 对照 PG 的固定文件清单，调用 `display verify-storage` 核对包内容，再用 `display restore-invalidate` 更换环境代次、撤销会话及终端配对。较早的完整归档使用同一控制库快照内的包清单逐文件核验。空间资料使用 `spatial verify` 按租户核对 PG 版本、原件和浏览文件。对应结果写入恢复记录的 `display` 与 `spatial` 步骤。新 JetStream 命名空间按新 epoch 重新生成配置、加载并准备流。归档的旧卷用于对照。每种对象逐字节回读，relay 云侧和内网收据按身份核对。对账未完成时保留停写状态和失败目标，另选一个独立空环境重试。操作员检查恢复证据并显式 open，再按目标角色清单放行业务。

遇到权限越界、重复效果、未收敛缺口、错误发布、性能或可见页面门槛失败时停止扩大流量。优先保留切流后已确认的 PG、CH 与对象数据，通过同 schema 兼容程序和递增代次切回。重新开放之前，逐键复核新增命令、任务、云端确认、发布／撤销水位及通知／审计。原库快照只用于独立恢复验收。

## 隔离自验

对有冻结 S0 的验收目录运行 `node scripts/architecture-offline-check.mjs --output "$RUN"`。该命令从冻结基线构建同一目标的旧方法包，将其与本版发行物一起核验，并将可信 manifest 摘要、逐方法兼容结果和文件数写入 `deploy/offline-release.json`。可通过 `--parser-package "$RETAINED_METHOD_PACKAGE"` 指定已经使用的版本。复核外部构建的包时，另加 `--bundle "$RELEASE" --manifest-sha256 "$TRUSTED_HASH"`。

`node scripts/architecture-deployment-check.mjs --output "$RUN"` 创建独占 Docker 网络、PG 和 Redis，使用冻结版创建会话、租户与待办，执行升级、切流、恢复，并核对容器与原生入口。选用已有候选时，`--current-bin` 指向发行目录的 `candidate-bin`。部署检查校验相邻的源码归档与候选清单，使用归档针对 Docker 主机架构编译六个 Linux 二进制，并逐字节比对 Nginx 返回的控制台首页和候选静态文件。`$RUN/deploy/deployment.json` 记录检查点、Linux 二进制摘要和结果。`pnpm test:architecture` 执行综合验收，按 ARC 编号关联版本、域证据、故障和实测数据。人工接受记录与软件自验分别签记。

同一文件的 `replica-quotas` 记录身份服务合计八条数据库连接、每个槽实际占用四条、同槽重复启动时的结果及原进程退出后的接管。每个槽在进程报告中登记十二个 HTTP 在途请求的限额。解析执行器把四个位置分为 2、1、1。`live-native`、`live-compose` 和 `live-mixed` 记录代表部署的原生进程和容器实际资源用量。逐角色的日志与诊断样本可在[架构自验](../verification/system-architecture-refactor.md)和[诊断自验](../verification/architecture-diagnostics.md)中查看。

联合恢复的 `deploy/joint-recovery.json` 包含冻结时点与空环境恢复后逐键核对的 `keys.before` 和 `keys.after`。云库收据与本地 ACK 意图按同一接收 ID 和原文摘要关联。对照发布作业、CH 物理写入及 PG 正式选中数，核实在途写入的可见性。部分文件上传按旧恢复代号到期，相同分片收据和对象仍可核对。审计导出按代次、对象及 CSV 摘要核验。逐项核对完成后才执行恢复开放命令。

已准备冻结 S0 的环境另运行 `node scripts/architecture-rollback-readback.mjs "$RUN"`。脚本在单独的 PG、CH 和对象命名空间保留一份业务库，依次覆盖切流前、部分 HTTP 切流、worker 切换后三个时点。在每个时点通过原公开接口接受新数据、切回兼容角色，用同一已授权大屏查询新旧值、真实浏览器检查批次、运维 CLI 查原作业，再逐键核对发布、变更、审计、通知和重复命令回执。记录在 `deploy/rollback-readback.json`，浏览器截图按阶段保存到 `deploy/rollback-gui`。

回退完成后，脚本还使用已发布的数据核对摘要修复权限。三个 worker 只可清除摘要头水位已覆盖的同租户、同通道脏区间。历史 worker 登记逐块替换对象要匹配仍有效的重建作业及已完成的分块，解除通道隔离还要有对应版本的修复收据。十项允许与拒绝的实际结果见 `deploy/summary-repair-check.json`。

`pnpm test:architecture restore --run-dir "$RUN" --release-dir "$HOST_RELEASE"` 还会依次运行 A、B 两侧冻结套件已有的监测、文件、大屏、通知和审计恢复命令。每侧使用自己的 restore 源存储、账户数据库和 Redis，配置保存在 `$RUN/private/regression-storage/{baseline,candidate}-restore`。监测和大屏原用例每次创建一个新的恢复目标。两侧共享该目标的固定端口，统一入口用同一把锁串行启动。每项保留原测试名、退出码和完整日志。监测及大屏另保留独立目标的恢复清单、校验报告和服务日志，见 `$RUN/regression/{baseline,candidate}/restore`。

恢复套件使用 `--resume` 时会核对本次选中的发行源码、原命令日志、归档文件以及监测和大屏的备份清单。发行物已换版或原证据缺失时重新执行对应命令。日常文件功能的 `test:files`、`test:files:http`、`test:uploads` 共用本侧独立的 `files` 存储组。三项在同一把锁内依次使用原 PostgreSQL、ClickHouse、Redis 和对象存储。每次退出后保留命名卷，将容器停车记录写入 `$RUN/private/regression-storage/{baseline,candidate}-files/parking/`。

### 功能自验范围

本次验收要求 ARC-001 至 ARC-036 及 ARC-039 的功能检查全部通过。运行 `pnpm test:architecture --run-dir "$RUN" --release-dir "$HOST_RELEASE" --resume` 接续功能验证。A、B 两侧各保留控制台、大屏和独立诊断入口供人工操作。

项目治理分页、成员与邀请草稿、监测导入详情回焦使用用户批准的共用控制台修正版。统一入口对照冻结源码与候选发行清单，生成 `$RUN/shared-console.json`，记录源文件差异及整份控制台的逐文件摘要。旧后端和新后端加载同一静态目录，各自处理实际的登录、授权、分页和写入。原始失败及当时使用的控制台可在 `$RUN/regression/browser-diagnoses/` 和 `$RUN/console-corrections/` 复查。每侧人工样本索引的 `consoleCorrections.importFocus` 记录了可重新打开的第二页批次。进入处理记录后刷新并按 Esc，列表返回时焦点应回到原文件；已手动选中其他控件时，焦点应保留在那里。

两侧的会话测试使用真实 PostgreSQL 和 Redis，结果及原始 Go 输出在 `$RUN/identity/session-activity/{baseline,candidate}/`。记录包含数据库时钟落后时的活动缓存、清空缓存后的读回、两小时空闲、十二小时到期以及撤销后的公开请求。解析服务的构建身份也逐侧核实。公开解析结果的 `service_build_digest` 须等于该侧实际执行的 `parser` 二进制 SHA-256，方法、输入和业务结果按各自契约核对。

ARC-037 九轮接入压测、ARC-038 十八场大屏负载对照，以及 ARC-039 中的持续接入、百万行计算和端到端负载、监测容量采样、十实例大屏压力及派生 2 小时混合运行与 24 小时观察均按用户要求跳过。`load` 子套件只登记此次选择。原始测量结果和日志保留在各自活动目录。大屏成员范围继续使用原命令的 `--boundary` 模式，校验 50,000 个成员的准确总数及 50,001 个成员时的拒绝行为。

原领域回归使用专属契约测试存储时，统一入口在命令结束并归档证据后，持有该组独占锁停止其 Compose 服务。对象和 ClickHouse 的命名卷可供后续重启。使用 tmpfs 的测试 PG 会在下次启动时重新创建合成数据，需复查的恢复清单、日志和收据在停车前已归档至运行目录。

本机并行跑不同子套件时，原 HTTP 回归与独立诊断回归共用一把镜像构建锁。HTTP 回归先逐个构建原 Compose 声明的运维、API 和 Web 镜像，再用原命令的 `up --build` 启动。每次构建分别留有日志和退出码。同机同时编译多个 Linux 镜像曾使 Docker 的内存耗尽，引发原监测契约库的两个 ClickHouse 容器退出。恢复时使用原容器和原卷，分别读回两台副本、冻结表与对象摘要。相关窗口内失败的命令须在资源恢复后使用原时限复验。

公开 HTTP 和 Playwright 各自保存该次 Go 请求录制夹具。两个独立存储组分别替换测试用的 ClickHouse 读取地址和清场端口，同一版本的两组可以同时录制。原请求和响应按各自运行目录归档。

派生回算的取消操作保留独立功能用例。在 A、B 各创建一个回算作业，观察该作业正在执行的 PostgreSQL 查询，通过原业务命令取消，再读回作业状态并重放同一命令。两侧均须确认查询已停止、作业为 `cancelled`、同一命令只有一张持久回执。两份 Go 原始日志和运行批次见 `regression/derived-cancelled-query.json`。

S-01 的 A、B 各用一个独立的 SeaweedFS 主节点，选举超时均为 3 秒。停机前记录两个卷服务各自持有的卷号。重启后，从主节点核实这些卷已重新注册，再用原用例读取停机前写入的完整对象及 ClickHouse 数据分区。自发出启动命令到原用例读回成功合计最多 20 秒。命令收据保存实启镜像、选举参数、持久卷、恢复时序和原始 Go 结果。
