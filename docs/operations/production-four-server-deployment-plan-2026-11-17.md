# InfraPro 首期全功能试运行部署方案（2026-11-17）

后续状态：业务生产实例已启动，当前版本映射和实机检查见[生产版本基线](production-version-2026-11-17.md)。下文为安装前确认的方案；不能用方案中的计划能力替代实际运行与验收结果。

状态：**部署方案 Accepted；用户已确认主机准备完成，基础运行依赖已安装并实测，正式生产实例尚未初始化。** ClickHouse 单节点支持、三项部署适配及回归收尾已完成。主机重启、校时、数据卷、兼容软件包和跨机连通性以[离线环境交接记录](../../.artifacts/offline-env-preparation-20261117/reports/environment-handover.md)为最新依据；此前清理见[历史记录](production-server-preparation-2026-11-17.md)。下一步制作正式发行包，随后配置并启动生产实例。

用户已明确本期采用单实例试运行，先启用当前已实现的全部功能，主备、数据副本与自动故障切换以后完善。本方案替代此前的 PostgreSQL 主备、ClickHouse 双副本和三 Keeper 方案；[原主备机制核查](production-failover-status-2026-11-17.md)保留为后续参考，不是本期实施要求。

## 1. 四台服务器的明确分工

| 服务器 | 实测配置与现有磁盘 | 本期部署服务 |
|---|---|---|
| **10.23.56.37：业务数据节点** | 8 vCPU／16 GiB；500 GiB 虚盘，约497 GiB根LV，清理后可用约454.6 GiB | **业务 PostgreSQL 17 单实例、Redis 7.4 单实例、业务 SeaweedFS／S3 单实例、observer**。保存账号／权限／租户／项目／设备／任务等业务元数据，以及文件、空间、大屏、解析、接入、监测源文件／摘要和审计对象。 |
| **10.23.52.151：监测与设备接入节点** | 12 vCPU／16 GiB；根LV100 GiB，已挂载 /data 数据LV350 GiB，另留约48 GiB VG余量 | **ClickHouse 单实例、同机单 Keeper、CH 在线历史专用 SeaweedFS／S3、ingest-relay、relay 专用 PostgreSQL 17 单实例、observer**。负责监测观测存储、设备入口和原文持久缓冲。 |
| **10.23.52.152：应用与后台计算节点** | 16 vCPU／32 GiB；根LV100 GiB，已挂载 /data 数据LV350 GiB，另留约48 GiB VG余量 | **Nginx／静态前端、API、全部内网 worker、独立 parser、大屏 runtime、observer**。下表列出12个业务常驻进程；按需执行解析子进程与Python转换器。 |
| **10.23.56.38：保留既有业务，新增隔离运维** | 8 vCPU／16 GiB；500 GiB虚盘；现有代理与Python服务保留 | 计划新增 **独立 diagnostics-server、observer、备份仓库／备份 S3**。运行用户沿用已准备的gzzx，应用身份、配置、目录、服务名和端口独立；不改原Nginx配置，不占用9000、8555–8560、11883／11884等既有端口。 |

37 的 PostgreSQL 与151 的 relay PostgreSQL **保存不同业务的数据，是两个独立实例，不组成主备关系**。业务Redis和S3也各只有一个在线实例。151 的 Keeper 是现有表引擎所需的元数据协调服务，只部署一个并绑定本机；它不存第二份监测数据。

选择这一分工是为了让37承担业务数据库与文件I/O，151承担监测库及接收缓冲，把内存最多的152留给API、解析、转换和后台计算。38继续保留现有业务，同时让诊断与备份独立于新应用节点。

## 2. 全功能进程清单

| 业务进程 | 服务器 | 职责 |
|---|---|---|
| api | 152 | 人员与业务API、权限、会话和文件上传入口 |
| monitoring-worker | 152 | 监测导入／摘要／导出／维护、派生计算与分析、人工试解析、大屏后台读取协调 |
| notification-worker | 152 | 通知投递 |
| task-index-worker | 152 | 任务索引、回填与对账 |
| spatial-worker | 152 | 空间任务及按需Python转换 |
| display-package-worker | 152 | 大屏包检查、处理与维护 |
| display-runtime | 152 | 可信运行域与不可信资源域两个独立端点 |
| files-worker | 152 | 附件准备、交接与清理 |
| files-checker | 152 | 无存储凭据、禁网的本机文件检查器 |
| audit-worker | 152 | 审计交接、导出与维护 |
| parser | 152 | 固定版本解析服务，按请求启动受限parser-runner |
| ingest-worker | 152 | 主动从151拉取原文、接管确认、识别、规划、解析与恢复 |
| ingest-relay | 151 | 设备接收、持久spool、控制接口与接收维护 |

合计13个业务常驻入口。迁移／维护CLI、parser-runner和Python转换器按需运行；前端与诊断UI发布静态构建产物，不常驻Node/Vite。

API、files-worker、files-checker保持同机，配置相同node_id和匹配的本机暂存目录／Unix socket权限；远端S3不会替代这份本机暂存。其他worker采用各自最小权限配置。ingest-worker单独配置其实际需要的PG／S3／parser，避免继承无关CH启动检查。

## 3. 服务之间怎样连接

- 用户 → 152 Nginx／前端／API → 37业务PG、Redis、业务S3。
- 152监测与计算进程 → 151 ClickHouse；151 CH访问同机在线历史S3。
- 设备 → 151 ingest-relay → 151独立relay PG；152 ingest-worker主动调用151控制接口拉取、确认，再调用同机parser并写入业务存储。
- 每台observer → 38独立诊断服务；备份材料在维护窗口封存到38。
- 业务S3和CH在线历史S3使用独立实例／身份／目录；38备份使用第三套独立身份和存储目录。

当前已实现功能全部纳入安装清单，但仍以各模块真实功能范围和实际生产配置为准。设备解析、正式观测发布、质量规则和业务告警之间的能力边界以[接入运行手册](device-ingestion-and-raw-messages.md)为准，不能用进程启动代替业务链路验收。

## 4. ClickHouse 单节点支持：本轮已完成

默认存储及附加物理分片已允许一个或多个真实endpoint。例如本期配置：

    monitoring_clickhouse_replicas: '10.23.52.151:9440'
    monitoring_physical_shards: '[]'

真实存储测试已证明单节点表的total_replicas和active_replicas均为1，能够初始化、检查、写入、核验发布清单、读取并进行原生备份恢复；业务CSV上传／发布／查询及在线历史分层也通过。双节点回归保持通过。详见[验证记录](../verification/single-clickhouse-deployment.md)。

继续使用现有ReplicatedMergeTree、一个Keeper和既有S3分层／恢复协议；单节点没有第二份数据。检查全部实际配置节点，保留类型、TTL、权限、对象与载荷完整性保护。以后可以显式增加节点；不能重复填写同一实例绕过多副本身份检查，也不在服务启动时自动改动既有表。

## 5. 技术与运行方式

采用现有Ubuntu 24.04上的 **systemd原生部署**，每个进程独立服务与配置。按主机准备结果，37／38使用gzzx、151／152使用uadmin；数据库／对象／机器身份按实际职责配置最小权限。服务按依赖顺序启动，异常退出受控重启，日志进入journald并设置保留上限。

| 技术 | 版本／用途 |
|---|---|
| Go | 按仓库锁定Go1.27.0，使用GOOS=linux、GOARCH=amd64、GOAMD64=v1；承载API、各worker、parser、relay及诊断 |
| React／TypeScript、Nginx | 前端静态文件在152发布；已安装Nginx1.30.6，生产不运行开发服务器 |
| Python | 152使用已有Python3.12.3及独立转换环境；保留实测的NumPy2.5.4通用x86-64兼容wheel与13包锁定清单 |
| PostgreSQL | 已安装17.12；37业务实例、151独立接入缓冲实例；38／152有同主版本客户端 |
| Redis | 已安装7.4.12；37的会话缓存、活动合并与限流 |
| ClickHouse／Keeper | 已安装并实测26.8.38.9的amd64compat版本，官方发布分支CI产物，固定提交71391c9b2bbc4d2c104996451586a396138a4785及摘要；151单CH＋单Keeper |
| SeaweedFS | 已安装4.53；37业务S3、151历史S3及38备份S3各由master、volume、filer、S3四个独立进程组成一个存储实例 |

基础依赖已按主机角色离线安装，不依赖Docker或在线拉镜像。正式应用发行仍需构建匹配Ubuntu24.04／Linux amd64的产物，保存版本和摘要。兼容软件包以环境交接中的清单为准：原26.8.36.3标准包不能在当前CPU运行，已移除；NumPy必须沿用已验证的兼容wheel。不能将本机研究目录中的原始包重新覆盖到已适配的环境。

统一目录沿用用户在主机准备中指定的 `~/env/infrapro`：37／38为 `/home/gzzx/env/infrapro`，151／152为 `/home/uadmin/env/infrapro`。

- `releases/<release-id>`：待发行的应用程序、静态文件及完整历史解析包。
- `packages/`、`software/`、`bin/`：已安装的角色离线包、版本化依赖及含库路径的封装入口。
- `config/<process>`：受限配置、身份与正式证书；`logs/`、`run/`：日志、报告、socket及受控暂存。
- `data/<component>`：持久数据；151／152的data入口已链接到 `/data/infrapro`，37／38使用各自env内的根文件系统目录。
- 38在自身data下设置独立备份与诊断状态目录，保留原服务目录。

系统级unit等注册文件仍由systemd管理。151／152的持久数据服务设置 `RequiresMountsFor=/data`；逐服务核对实际可读／可写路径，模板中的 `ProtectHome` 等限制须与home内安装路径适配。复用 `bin/postgres`、`bin/weed`、`bin/clickhouse-server`、`bin/spatial-python` 等封装入口；Nginx的pid、日志和临时目录显式指向env。SeaweedFS按四组件顺序启停，等待master选主及真实S3请求通过，不以端口监听代替就绪。

## 6. 建议端口及访问范围

| 服务 | 计划地址 | 访问范围 |
|---|---|---|
| 人员站点／API | 152:18443，HTTPS | 用户访问；API实际监听152 loopback:18000 |
| 大屏可信运行域／不可信资源域 | 152:18444／18445，HTTPS | 保持三个站点origin隔离；后端绑定loopback |
| parser | 152 loopback:18091 | 同机API／worker，保留机器身份认证 |
| 业务PG | 37:55432，TLS | 152业务进程及指定维护来源 |
| Redis | 37:16379，TLS | 152业务进程及指定维护来源 |
| 业务S3 | 37:18333，TLS | 152业务进程及指定备份／维护来源 |
| ClickHouse native | 151:9440，TLS | 152监测／计算及指定维护来源 |
| 单Keeper | 151 loopback:9181／9234 | 同机CH和本机诊断；不开放跨服务器仲裁端口 |
| CH历史S3 | 151 loopback:18333 | 同机CH；master／filer／volume等管理口均受限 |
| relay专用PG | 151 loopback:55432 | 仅relay及受控备份维护 |
| relay控制接口 | 151:19443，mTLS | 152 ingest-worker及指定维护来源 |
| 设备入口 | 151:20000–20999中的实际批准端口 | 逐入口配置协议、身份、设备网段；按需开通 |
| 独立诊断 | 38:18096，HTTPS | 运维及observer，使用独立身份和入口 |
| 备份S3 | 38:18334，TLS | 指定备份／恢复来源，不经既有Nginx公开 |

首期人员入口可使用152 IP和独立端口、含IP SAN的内部CA证书，访问终端安装信任链；已有正式DNS时使用对应三个域名。单实例选择不改变入口身份、租户隔离和文件访问保护。

应用已支持 Nginx 终止 TLS 的 HTTPS，URL／Secure Cookie／Origin／CSRF／代理和大屏工具已适配，真实代理专项自检通过；首次扩大回归发现的问题已修复并复验，见[HTTPS验证记录](../verification/https-deployment.md)及[全仓回归修复报告](../verification/repository-regression-2026-11-17.md)。正式证书、信任链及目标入口仍须安装验证。上述端口是待安装值，没有声称当前已经可访问。设备到151的现场网络／网关映射需在实际接入时核验。

## 7. 资源初值与基本备份

| 节点 | 内存初始预算 | 约束 |
|---|---|---|
| 37，16 GiB | 业务PG约5 GiB，业务S3约2 GiB，Redis约512 MiB，observer约128 MiB；至少留约4 GiB给OS／缓存 | 按实际对象、PG及WAL增长设置空间告警，限制备份I/O |
| 151，16 GiB | CH约6 GiB，历史S3约1 GiB，relay PG约1.5 GiB，relay约512 MiB，Keeper约512 MiB，observer约128 MiB；至少留约4 GiB给OS／缓存 | 接入缓冲与CH共享数据卷，分别设预算；初期CH查询／合并和接收并发受控 |
| 152，32 GiB | 应用／worker／parser／子进程总预算先控制在约16–20 GiB，至少留约8 GiB给OS／缓存／峰值 | 初始派生前台1／历史1、解析全局4／每租户2、空间转换1；根据实测调整 |
| 38，保留既有业务 | 新诊断与备份服务合计先控制在约2 GiB／1 vCPU | 复制限带宽和I/O，备份目录预算暂不超过300 GiB，保留空间供原业务增长 |

这些是首期预算，不代表已经验证设备容量、吞吐或生产SLA。磁盘目标保留约20%可用空间，限制WAL、日志、缓存、导出与暂存增长；不把350 GiB数据LV理解为350 GiB全部可写业务净容量。单节点CH无需为第二份在线副本预留同等空间。

**首期仍做独立备份。** 每天低峰和升级前生成联合恢复点，先保留两份校验通过的完整恢复点，容量超过预算时调整介质或保留安排。备份覆盖37业务PG、151 relay PG、151 CH原生备份、业务对象、完整历史解析包、发行物与受保护配置。38的备份材料不构成运行中的备库，不需要主备仲裁。

现有联合备份需要短维护窗口冻结写入和接入确认边界，完成后恢复；不直接拷贝正在写入的数据目录冒充一致备份。正式接入前至少验证一个完整恢复点在独立空目标可恢复。详细边界沿用各模块运维手册；本轮单CH原生恢复验证没有代替全平台恢复演练。

## 8. 实施顺序与剩余事项

1. **已完成**：三台旧业务清理、151／152数据卷、ClickHouse单节点支持；B1版本检查、B2 HTTPS、B3上传适配，以及全仓回归失败／超时修复和复验。各项自检与生产验收分别记录。
2. **主机准备已完成**：四台已重启并同步到用户确认的内网时间源10.23.52.102；151／152开机挂载、各组件真实功能与16条跨机连接检查通过。38原配置摘要保留，已核对原站点和端口；原Nginx重启后手动恢复，自动启动编排不在本次准备范围。详见环境交接，不重复安排基础依赖安装。
3. **制作正式发行包**：冻结源码及产物摘要，按当前schema55制作同一版本的Linux程序、前端、解析包与维护工具；补齐全13入口systemd配置、逐节点最小权限配置和离线依赖，准备HTTPS正式证书及入口安装材料。已完成的部署适配及回归收尾不再重复列为待修复项。
4. **配置生产数据实例**：复用37的PG／Redis／业务S3和151的CH／单Keeper／历史S3／relay PG程序，创建正式实例、证书和身份并注册unit；配置38备份S3及新增诊断程序。环境准备的临时验证实例已停止，不作为正式业务库使用。
5. **首装初始化**：使用同一冻结发行CLI迁移空业务库、access sync/check、初始化监测存储和模块角色／对象策略，创建正式首个管理员；relay执行独立迁移。使用正式身份及证书，不搬用合成验收数据库或测试账号。
6. **启动全功能应用**：152先启动checker／parser等依赖，再启动API、各worker、大屏和Nginx；151启动relay。配置实际租户／项目、设备入口、关联、方法采用和机器授权；4个observer按真实服务与单CH登记。
7. **试运行验收**：验证下列业务和运行条件，形成首个联合恢复点并演练后开放正式使用。故障时采用受控重启或从备份恢复；本期不安装Patroni、etcd、Keepalived，不进行自动主备切换。

## 9. 手工验收及交付边界

- 152人员站点可登录，租户／项目权限、工程／设备、普通文件、空间、大屏、通知、任务、审计正常；解析／接入按照已实现契约运行。
- 只配置151一个CH地址，storage init/check成功；监测导入、发布、曲线、精确明细、派生／分析与导出有实际结果，状态页面不再以缺少第二个副本判故障。
- 151接收代表性设备／模拟报文，152自动拉取、确认、识别和解析；分别核对接收收据与处理结果。
- 全部常驻进程由systemd管理，重启恢复正常；文件暂存与checker权限正确，数据卷挂载有效，诊断能看到四个真实节点和实际配置数量。
- 38原代理及Python服务保持可访问；新服务端口和数据目录隔离。
- 备份封存与恢复核验通过，记录实际恢复耗时；日志、容量及任务失败可观察。单实例故障会造成对应功能暂停，这是用户已选择的首期运行方式。

部署方案已获用户确认。此次部署前分析没有安装生产软件、变更四台服务器配置、执行生产迁移或做全系统压力测试；单节点代码自检、方案确认与生产部署验收分别记录。安装和正式开放前的具体问题、交付物与验收条件见[部署准备分析](production-deployment-readiness-2026-11-17.md)。

## 10. 主要依据

- [运行入口目录](../../services/diagnostics/settings/catalog.go)：13个业务常驻进程及按需工具。
- [单节点配置校验](../../packages/go/platform/config/monitoring.go)、[存储初始化与读写](../../packages/go/platform/monitoring/storage.go)、[布局核验](../../packages/go/platform/monitoring/storage_checks.go)。
- [文件同机边界](files-and-attachments.md)、[监测运行与恢复](monitoring-data.md)、[派生分析](derived-monitoring-and-analysis.md)、[设备接入与独立relay](device-ingestion-and-raw-messages.md)。
- [业务HTTPS配置](../../packages/go/platform/config/config.go)、[大屏origin校验](../../packages/go/platform/config/display.go)、[代理请求体设置](../../deploy/nginx/default.conf)。
- [迁移头](../../packages/go/platform/migrations/migrations.go)、[接入operator版本检查](../../packages/go/platform/ingestion/operator.go)。
- ClickHouse官方[复制表说明](https://clickhouse.com/docs/reference/engines/table-engines/mergetree-family/replication)：复制按表管理并由Keeper保存协调元数据；本期保持现有引擎，但只建立一个真实数据节点。
