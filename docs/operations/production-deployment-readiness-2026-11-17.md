# 首期正式部署前问题分析与执行清单（2026-11-17）

2026-11-17 部署更新：四机生产部署和自验已完成，状态为 **Awaiting acceptance**；实际服务、访问入口、备份恢复及人工验收见 [生产部署交接](production-deployment-2026-11-17.md)。


后续状态：业务生产实例已启动，发行包 `20261117-802a92f81b63` 与 Git 标签 `production-2026-11-17` 的代码对应关系已经实机核验，见[生产版本基线](production-version-2026-11-17.md)。下文保留部署前分析时点的记录；“尚未初始化”等表述不代表当前状态。

状态：**B1、B2、B3部署适配、全仓回归收尾及主机准备已完成，用户已确认。四服务器单实例方案已获确认（Accepted）；基础运行依赖已离线安装并实测，正式生产实例尚未初始化。** 回归采用完整遍历、修复后批次复验与专项补验，人工验收与生产验证另行记录；最新证据见[全仓回归修复报告](../verification/repository-regression-2026-11-17.md)。

本期按[已确认部署方案](production-four-server-deployment-plan-2026-11-17.md)启用当前已实现的全部功能：37放业务PG／Redis／业务S3，151放单CH／单Keeper／历史S3／relay及其独立PG，152放前端／API／全部内网worker／parser，38保留原业务并新增隔离诊断与备份。主备、第二个CH数据节点和自动故障切换不属于首期上线条件。

**结论：现在进入正式发行包和生产配置准备。** 部署适配、回归收尾、主机校时／重启／兼容性及依赖实测均已完成。需要冻结发行版本，生成Linux应用部署包、逐节点systemd和正式身份配置，再初始化并启动生产实例，完成目标环境业务与恢复验证。沿用[离线环境交接](../../.artifacts/offline-env-preparation-20261117/reports/environment-handover.md)中的实际目录、兼容程序与启动约定；基础软件验证通过不等同于正式业务实例已上线。

## 1. 本轮检查范围与已完成工作

首次问题分析通过SSH只读检查四台服务器和本地源码并执行 `pnpm check`。后续侧边任务已安装运行依赖并完成主机准备；本次状态同步读取其交接与验证清单，没有重复安装、校时或重启。下表明确区分初查与后续结果。

| 已完成事项 | 证据及边界 |
|---|---|
| 三台旧业务清理 | 37／151／152旧业务已清理。38原程序／配置／数据保留；后续准备核对六份配置摘要不变、原HTTP／HTTPS入口返回200，11883／11884仍监听，不等于旧业务全部流程验收。 |
| 两台数据卷准备 | 151／152各350GiB数据LV已在后续整机重启后按UUID自动挂载，原根卷及约48GiB卷组余量保留。四台data入口完成16MiB写入／fsync／摘要核对；151／152的env内data链接到 `/data/infrapro`。 |
| 主机与离线依赖 | 四台重启、NTPsec持续同步、目标CPU兼容程序、依赖真实读写／恢复／转换及16条跨机连接检查通过；验证实例已停止。见[环境验证汇总](../../.artifacts/offline-env-preparation-20261117/reports/verification-summary.json)。 |
| CH单节点支持 | 已取消至少两个节点的限制；单节点初始化、写入／发布／读取、原生备份恢复，以及双节点回归已通过。生产安装和全平台恢复仍待执行。 |
| 初次源码基础检查（历史） | 2026-11-17 11:17–11:18（本机时间）执行 `pnpm check`，退出0；当时没有运行全项目E2E或正式发行构建。后续结果见下一行。 |
| 全仓回归收尾 | 已核对“补齐部署前 HTTPS 支持方案”会话及[修复报告](../verification/repository-regression-2026-11-17.md)：Go清单493项中491项通过，浏览器269个项目实例中267项通过，JS103项通过；静态检查、完整本机构建及HTTPS／HTTP、大屏、解析采集、恢复、诊断专项通过。常驻夹具及重复场景单列，30分钟持续负载未运行。采用完整遍历与修复后复验汇总，不改写历史失败运行。 |
| 版本留痕与发行边界 | 初查快照记录 `main`、HEAD `9c744acfa26a3f754eb0710cd9052d5e5e8be9a7`、686项工作区变更及schema54；后续上传适配已推进到schema55。回归修复保存在工作区，尚未提交或推送；本机构建通过，冻结的Linux发行包仍需另行制作。 |

实际主机准备见[清理与数据卷记录](production-server-preparation-2026-11-17.md)，单节点证据见[CH验证记录](../verification/single-clickhouse-deployment.md)。本轮证据位于 [`.artifacts/production-planning-20261117/`](../../.artifacts/production-planning-20261117/) 中的 `predeployment-current-state.json`、`predeployment-source-snapshot.json`、`predeployment-pnpm-check.json` 和同名日志。源码与配置继续变化时，以正式冻结版本重新核验。

## 2. 安装前部署适配问题与处理状态

| 编号 | 已确认的问题与影响 | 实施内容 | 完成标准 |
|---|---|---|---|
| B1 数据库版本不一致 | 原问题：迁移版本54，接入运维工具仅接受并报告45，阻断 `manifest`、`release`、`objects`。2026-11-17 已完成修复及隔离自验，**Awaiting acceptance**。 | 运维工具改用统一的 `migrations.Version`，清单与发行报告返回实际核验版本；保留严格匹配、缺产物及坏对象拒绝，接入手册已更新。 | 空库首装及真实 PG／S3 恢复目标上三个 CLI 命令均成功；旧版／未来版、缺产物、缺对象和同长度篡改均明确失败。接入常规集、历史／并发迁移及 `pnpm check` 通过；生产安装迁移未执行。见[B1验证记录](../verification/deployment-schema-consistency.md)。 |
| B2 应用 HTTPS 适配 | 原 HTTP-only 配置限制已解除；真实 Nginx HTTPS 专项和相关回归通过，**Awaiting acceptance**。 | 应用 URL／三 origin／Secure Cookie 联动校验；开发工具支持 HTTPS 与正常 CA 验证；交付原生 Nginx 模板、应用配置和运维手册。 | 真实代理登录／写入／邀请／重置／退出、大屏发布／预览／工具续传、错误来源与证书拒绝均通过；运行上游仅接收自身 Cookie，资源上游不接收 Cookie／Authorization。首次扩大回归的问题已另项修复并复验；正式证书安装、目标主机与人工验收未执行。见[B2 验证记录](../verification/https-deployment.md)及[全仓回归报告](../verification/repository-regression-2026-11-17.md)。 |
| B3 代理请求体限额不匹配 | 原 16 KiB 默认限制会阻断合法分片。现三份模板已统一上传／业务请求策略，整文件大小改为部署配置，**Awaiting acceptance**。 | 保留附件／空间 8 MiB、监测 32 MiB、大屏 4 MiB 分片；配置、API、前端、开发工具与 worker 联动；schema 55 放宽旧大小及分片数约束；补齐超时、流式传输和 JSON 错误。 | 126 项真实 Nginx 检查、四类真实上传／续传及适用的下载、旧上限以上样本、调低配额、54→55 升级、101 MiB 附件独立恢复、HTTPS 和模块回归、`pnpm check` 均通过。生产安装和人工验收未执行，见[B3 验证记录](../verification/upload-deployment.md)及[配置手册](upload-limits-and-proxy.md)。 |

源码依据：

- B1：[迁移版本](../../packages/go/platform/migrations/migrations.go)、[接入运维实现](../../packages/go/platform/ingestion/operator.go)、[运维命令入口](../../tools/infrapro/cmd/ingestion-operator/main.go)。
- B2：[业务配置](../../packages/go/platform/config/config.go)、[大屏配置](../../packages/go/platform/config/display.go)、[会话Cookie](../../services/api/internal/transport/http/identity/handler.go)。
- B3：[基础代理](../../deploy/nginx/default.conf)、[大屏代理](../../deploy/display/nginx.conf)、[普通文件handler](../../services/api/internal/transport/http/files/handler.go)、[空间分片](../../packages/go/platform/spatial/uploads.go)、[监测分片](../../packages/go/platform/monitoring/import_model.go)、[大屏分片](../../packages/go/platform/display/model.go)。

## 3. 安装前的主机和发行准备

### 3.1 主机准备已完成：正式发行必须继承的结果

用户已确认侧边对话完成主机准备。本次核对[环境交接](../../.artifacts/offline-env-preparation-20261117/reports/environment-handover.md)、[验证汇总](../../.artifacts/offline-env-preparation-20261117/reports/verification-summary.json)及四台environment清单；以下取代初查的时钟异常、待重启和兼容性待定结论。

| 项目 | 已完成结果及后续约束 |
|---|---|
| 重启与挂载 | 四台已逐台重启，SSH恢复、无待重启标记，systemctl失败项为0；151／152数据卷自动挂载通过。持久服务unit仍需配置挂载依赖。 |
| 持续校时 | 四台NTPsec开机启用并持续选中10.23.52.102，leap00、stratum4。该源自报约10秒root dispersion，用户已确认继续使用；maxdist16是接收源的距离阈值，不是系统精度承诺，不能套用初查时建议的绝对时间精度作为已验证结果。 |
| CPU兼容 | 保留公司分配的CPU模式。CH／Keeper改用26.8.38.9发布分支的官方amd_compat CI产物，已固定提交／摘要并完成TLS读写、单Keeper单副本、备份恢复和重启读回。NumPy2.5.4改为通用x86-64源码编译wheel，其余锁定依赖通过10项实际转换。 |
| 依赖及网络 | PG、Redis、S3、Nginx、CH和Python按角色完成真实功能验证；16条跨机连接通过，临时服务已停止。现场设备到151的网络尚未测试，正式业务TLS连接随实例配置验收。 |
| 38原业务 | 六份原配置摘要保持；原HTTP／HTTPS站点返回200，11883／11884仍监听。原Nginx重启后手动恢复，本次未补其旧业务自启动编排。 |

发行必须使用 `GOOS=linux GOARCH=amd64 GOAMD64=v1`，保留已适配的CH与NumPy输入；不能被默认amd64包或默认NumPy wheel覆盖。Python源码依赖锁与兼容包清单的源锁摘要在本次交接核对中一致；实际发行仍需锁定最终转换器源码。依赖验证与容量压测分开，已有结果不代表生产规模吞吐保证。

### 3.2 全功能发行包与systemd

现有 [Go构建脚本](../../scripts/go.mjs)能发现各模块命令入口，但本机构建不自动等于Linux发行；现有 [API镜像](../../deploy/docker/api.Dockerfile)也只包含部分入口。仓库已有分模块systemd样例，尚无覆盖四台的完整安装配置，尤其缺少API、monitoring-worker、task-index-worker、spatial-worker对应的unit模板。

需要交付：

1. **一个可追踪的首期发行版本**：固定源码、schema、依赖锁、程序／静态网页／诊断UI／解析包摘要；包含13个业务常驻入口、诊断程序和实际需要的维护工具。保留各平台历史解析产物，避免重编译覆盖已登记的方法身份。
2. **复用已验证的离线依赖**：PG17.12、Redis7.4.12、SeaweedFS4.53、CH／Keeper26.8.38.9 amd64compat、Nginx1.30.6及Python转换环境已按角色安装。将角色清单、兼容NumPy wheel及摘要纳入发行记录；采用现有封装入口。正式CA／证书／密钥仍需准备，不能复用临时测试材料。
3. **逐节点安装与检查清单**：沿用37／38的 `/home/gzzx/env/infrapro` 和151／152的 `/home/uadmin/env/infrapro`，在其中增加应用releases及逐服务config，使用现有bin、run、logs、data。systemd运行用户按交接使用gzzx／uadmin，逐服务限制文件访问及存储身份；补齐重启、退出等待、挂载、日志和资源限制。
4. **真实systemd验证**：检查unit语法，再以正式运行身份启动；验证报告文件、Unix socket、本机暂存、parser-runner和空间转换权限。API／files-worker／files-checker保持152同机、同node_id及匹配目录；`ProtectHome`／`ProtectSystem`等限制须与home内安装路径匹配。SeaweedFS按master、volume、filer、S3四组件编排，使用实际S3读写判定就绪。
5. **全功能逐进程配置**：不能复制同一完整配置给所有worker；ingest-worker的通用runtime在配置CH时仍会连接并检查CH，按它实际使用的PG／S3／parser提供配置，避免引入额外启动依赖。为每个需要的模块显式启用真实存储和机器身份。

版本与入口依据：[go.work](../../go.work)、[package.json](../../package.json)、[运行目录](../../services/diagnostics/settings/catalog.go)、[通用runtime](../../packages/go/platform/runtime/resources.go)、[ingest-worker](../../services/api/cmd/ingest-worker/main.go)、[文件unit](../../deploy/files/systemd/infrapro-files-checker.service)、[Python依赖锁](../../tools/spatial-converter/requirements.lock)。

## 4. 安装时需要完成的配置与初始化

| 工作 | 具体交付及验收 |
|---|---|
| 网络与入口 | 16条计划连接已用临时监听和校验令牌实测通过。安装正式实例时落实监听、来源范围及正式TLS／mTLS握手，验证三个独立origin和Cookie／资源域授权剥离；临时连通性结果不替代真实业务入口验收。 |
| 设备现场链路 | 明确首批设备通过什么路径到151、批准哪些端口／协议／来源以及机器证书。四台都是内网IP；若设备只能连公网，需落实网关或转发路径再验收。当前没有现场设备网络已接通的证据。 |
| 正式身份与配置 | 建立运行、迁移、备份、恢复等身份及对象访问策略；生成正式密钥、数据库口令和CA／mTLS材料，按服务受限保存。诊断身份独立；不复用实验室账号、两天测试证书或示例密钥。配置不提交凭据到仓库。 |
| 两套独立PG及存储初始化 | 复用已安装程序，创建37业务库及151 relay正式实例并分别迁移；业务库配置扩展／角色，执行权限目录sync/check。显式初始化单CH布局、历史对象层、业务对象前缀及备份配置，再用实际应用账号验证。两套PG没有主备关系，临时验证库不转为正式库。 |
| 正式首个管理员和业务基础数据 | 在空业务库完成一次bootstrap，准备真实租户／项目、工程与设备、产品／协议／解析方法采用及机器授权。保持测试数据与正式数据边界；已初始化环境走账号恢复流程。 |
| 资源预算 | 37核对各应用PG连接池总和，152核对worker及子进程预算；151的CH、对象存储和relay PG共享16GiB内存及数据盘，需要按低并发试运行调参，验证合并／备份／接收并行时的峰值。预算表不是实测容量保证。 |
| 38隔离安装 | 沿用gzzx和env目录，配置独立诊断／备份身份、目录和unit，只使用规划的新端口；原Nginx和Python服务不改配。原Nginx在主机准备重启后已手动恢复，未来整机重启应纳入恢复清单；新增进程限制内存、CPU和备份I/O。 |

首装使用同一冻结发行的CLI和应用，按“依赖存储 → 迁移及权限 → 存储布局／角色 → 首个账号 → parser／checker等依赖 → API及workers → 代理入口”执行。[迁移命令](../../tools/infrapro/internal/command/database.go)、[权限命令](../../tools/infrapro/internal/command/access.go)、[账号初始化](../../tools/infrapro/internal/command/account.go)

151的内存需要特别实测：ClickHouse官方说明小内存环境需降低并发和缓存等设置。此处据此提出调参与验证要求，没有把推荐大内存配置作为本期新增硬件门槛。[ClickHouse运行建议](https://clickhouse.com/docs/guides/oss/best-practices/tips)

## 5. 正式开放前的验证和运行保障

### 5.1 联合备份与可恢复性

本期保留每日低峰及升级前备份，落到38；首期目标保留两份核验通过的完整恢复点，预算和实际增长匹配后实施。38是另一台虚拟机，底层物理存储是否独立尚未核实，不宣称已经具备异地容灾能力。

- 冻结业务写入、入口配置和接入确认边界，协调业务PG、relay PG、CH原生备份及所有业务对象；配置／证书／密钥、数据库角色与扩展、对象策略、完整历史解析包和发行摘要一并纳入保护清单。
- 监测backup工具会导出业务schema，但它的对象复制范围是监测租户前缀和CH原生备份；文件、空间、大屏、解析、接入及审计有自己的对象和恢复协议，必须组成全平台清单。不能把已有单CH恢复用例记作整个平台恢复通过。
- 在隔离空目标实际恢复一份联合恢复点，执行各模块核对。relay恢复与正常重启区别处理，保留旧收据身份，避免云端已确认但业务快照缺收据；任务／审计／文件的恢复代次与租约按各自流程处理。
- 记录备份耗时、体积、失败处理、恢复耗时和实际数据缺口。每日备份意味着故障时可能丢失最近成功恢复点之后的数据；可接受损失和停机时间须在正式开放前确认。定时备份成功只算一项证据，恢复完成才构成可用恢复点。
- 先按代表性数据测算两份备份能否放入38的300GiB初始预算，同时监控原业务增长。容量不够时调整保留安排或介质，不能自动挤占38原业务空间。

依据：[监测备份实现](../../packages/go/platform/monitoring/backup.go)、[监测运维](monitoring-data.md)、[接入联合恢复](device-ingestion-and-raw-messages.md)、[文件恢复](files-and-attachments.md)、[空间恢复](spatial-visualization.md)、[大屏恢复](visualization-display.md)、[解析发行](parsing-service.md)、[审计恢复](audit-and-operation-tracing.md)。

### 5.2 全功能业务与实机故障验收

| 验收场景 | 可观察的通过条件 |
|---|---|
| 登录、租户／项目与权限 | HTTPS登录、退出及写操作正常；跨租户、未授权项目和撤权后的访问明确拒绝；正式管理员可正常维护。 |
| 工程、设备、文件、空间、大屏 | 代表性业务可以创建／查询；普通文件分片上传与下载、空间转换、大屏包发布及运行有真实结果，后台任务不长期停在排队。 |
| 监测、派生计算、分析 | 单CH配置下完成导入／发布／曲线／精确明细／历史层读取／派生与分析／导出。没有缺少第二节点的错误提示。 |
| 接入与解析 | 使用代表性协议和真实授权，逐段核对接收、内网接管确认、识别和解析；断开worker后relay保管，恢复后积压排空，重复上报按协议幂等。当前已实现接入与人工解析功能全部验收；正式观测、质量规则及业务告警的边界仍按现有规格，不据此承诺未实现的自动链路。 |
| 通知、任务和审计 | 任务推进及失败／重试可见，通知按现有渠道送达，审计可追溯并完成受权导出；运维告警渠道另行验证。 |
| 首批规模与资源 | 按实际设备数、报送频率／字节量、用户并发和文件规模执行有界负载；记录持续积压、排空时间、响应延迟、内存及磁盘增长。历史100帧/秒测试未完整排空，不能替代本次目标机证据，也不恢复用户已停止的历史测试。 |
| 重启与依赖异常 | 进程异常退出后受控重启；依赖停止时状态明确、无假成功。机器维护重启后挂载和服务恢复；单实例机器故障按人工恢复手册处理。 |
| 38保留业务 | 原代理与Python链路安装前后正常；新增诊断和备份的峰值不影响原有业务。 |

### 5.3 诊断、容量和日常操作

逐节点建立真实manifest、实例身份和报告路径，将全功能进程从模板的disabled改为实际运行预期。诊断展示一个CH数据节点和一个Keeper；4个observer有新鲜报告。基础 `SELECT 1`／PING／HeadBucket只证明连通性，还要核验任务进度、接入积压、存储布局和最近可恢复备份。

至少落地：磁盘／WAL／日志／暂存上限和告警、worker任务失败与积压观测、备份失败及证书到期检查、值守联系人、启动／停止／恢复／升级步骤。独立诊断页面和业务站内通知不自动等于运维外部告警已接通。诊断状态库及observer代次按[独立诊断手册](independent-runtime-diagnostics.md)处理，恢复时不能复用旧身份的空状态库绕过检查。

## 6. 建议实施顺序与待填现场参数

1. **已完成部署适配和回归收尾**：B1、B2、B3已修复并完成自检；全仓失败和超时已按独立报告修复及复验，用户已确认收尾完成。人工验收、正式发行冻结及生产安装分别跟踪。
2. **主机准备已完成，制作正式发行包**：复用离线环境交接中的目录、兼容软件包、用户与封装入口；冻结当前源码和schema，生成可校验Linux应用发行及全功能systemd／配置。最终发行在已准备的主机上验证，不重复基础软件安装。
3. **配置并首装初始化**：按已确认分工配置数据实例、安装诊断与应用发行；初始化业务库／relay库、权限、CH及对象存储，建立正式账号和首批配置。
4. **目标环境验收**：通过真实HTTPS入口验证全功能、设备链路、代表性负载及停启恢复；修复实机问题。
5. **备份演练与开放**：联合备份／隔离恢复、诊断和值守检查通过，保留报告与回退步骤，再开放首批正式使用。后续升级先备份；schema不兼容时使用受控数据恢复，不能只替换旧二进制。

以下参数不阻止本轮问题分析；进入对应实施步骤前需要落实，不使用未经确认的值作容量或可用性承诺：

| 参数 | 用途 |
|---|---|
| 首批设备数量、采样／上报频率、报文大小、最长离线时间，用户／文件规模 | 确定relay缓冲、解析吞吐、CH与对象空间、备份预算及负载验收量。 |
| 设备到151的实际网络路径、网段、批准端口及协议／身份 | 完成网络开放和第一条真实设备链路。 |
| 正式管理员、租户／项目和首批设备／方法清单；证书信任分发方式 | 完成正式初始化和可用业务入口。IP加端口可沿用已确认方案，不要求等待购买域名。 |
| 后续安装／备份低峰窗口、允许丢失的数据时间范围、允许停机时间和值守人 | 主机准备的维护窗口已执行；落实后续兼顾38原业务的安装和人工恢复安排。 |

主备、CH第二数据节点、多Keeper仲裁、自动切换、跨机房容灾及弹性扩容继续作为后续完善项；本期只验收单实例实际提供的能力。全项目静态检查已通过，全部生产运行、容量和恢复验证仍需在上述实施步骤中取得证据，分析完成不代表生产上线完成。
