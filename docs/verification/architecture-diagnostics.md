# 拆分架构的独立诊断与辅助作业自验

本页记录 ARC-016、ARC-026、ARC-027、ARC-032 中的诊断、辅助作业与资源预算检查。当前证据来自本机隔离环境。逐项自验及人工接受状态见[架构验收总账](system-architecture-refactor.md)。

## 进程身份、拓扑与故障

使用 `pnpm diagnostics:coverage` 检查 26 个拥有进程的 Go 入口、44 项运行目录、32 个部署角色及每种环境的预期探针。覆盖检查也会向目录校验器注入一个没有声明的入口，要求它报告遗漏。部署参数、UID、报告挂载及节点映射见[独立诊断手册](../operations/independent-runtime-diagnostics.md)。

```sh
run=.artifacts/architecture-refactor/run-20261118
node scripts/diagnostics-architecture-check.mjs "$run/diag-role-faults.json"
pnpm test:diagnostics:environment
pnpm test:diagnostics
```

| 检查 | 操作及观察 | 证据 |
| --- | --- | --- |
| 新增、兼容程序 | 按选定发行清单分别核对 CLI 和 13 个二进制的 SHA-256。启动各程序，在读取配置时观察启动身份及进程报告，随后触发配置失败，核实终止原因、退出码和停止状态。b6a60e55efd0 的 13 个程序均通过 | 主运行目录 `diag-role-faults.json` 及 `diag-role-faults-b6a60e55efd0.json` |
| 原生、容器与混合部署 | 对三种清单检查每个组件、副本、探针、依赖与归属的观察节点。在隔离容器实际运行 files-checker，核实 PID 1、运行 UID、程序路径、报告挂载，修改期望路径和停止容器分别产生不同结果 | `diag-role-faults.json` 的 coverage 与 container |
| 发布清单与诊断副本 | 用实际部署生成器分别生成本机、原生、Compose、混合 4 个发布目录，再交给诊断 CLI，逐一核对 31、31、30、31 个进程槽。两个网关、两个身份服务、三个 parser、独立边缘 relay 和兼容 worker 的报告文件、程序路径、入口和标签都与生成配置相符。原生及混合部署将身份副本和无监听的审计 worker 放在两个命名宿主，unit 的节点 ID、清单和对应观察器绑定一致 | 主运行目录 `diag-deployment-slots.json` |
| 多副本 TCP 代理 | 启动发布用 Python 代理，连接两个使用独立网络命名空间的 Linux 后端。分别收到两个后端的响应标记，停掉第一个后，连续两个请求命中幸存副本。独立探针确认代理的真实 PID、程序路径、UID 和监听可连接。优雅退出后观察到 PID 记录已清理、监听拒绝连接 | 主运行目录 `diag-replica-proxy.json` |
| 消息可用性 | 启动真正的 JetStream 进程，使用本机健康端点，停止后观察到连接拒绝。三副本部署从三个独立槽位计算门槛 2 | `diag-role-faults.json` 的 broker 数据。三副本与存储压力见主运行目录 `events/` |
| 独立性与恢复 | 业务整体离线时重新登录独立诊断，停掉诊断服务与观察器时核对业务就绪和 worker 工作时钟。另覆盖 PG、报告访问与身份、观察器断连、磁盘压力、陈旧上报、存储写失败和单个大屏端点故障 | 主运行目录 `diagnostics/faults.json`，共 15 项 |
| 底层适配器 | 原生进程及报告 8 项，固定协议 17 项，真实 Docker 观察 7 项，三个观察器跨进程网络及 TLS 27 项。核心 Go 竞态、契约与本机 HTTP 共 92 项 | 主运行目录 `diagnostics/{native-contract,fixed-protocols,docker-contract,network-contract,core-tests}.json` |
| 独立浏览器 | 登录与 16 个 worker 的真实筛选、报告及依赖抽屉、资源曲线、历史快照过期恢复、被移除目标、权限证据自动显示、窄屏与深色样式、业务掉线后的新会话、诊断断连、账号撤销及跨标签退出均按实际界面验证。另核对同源请求、浏览器存储和响应的敏感信息边界 | 主运行目录 `diagnostics/browser.json` 共 22 项。`diagnostics/summary.json` 登记 9 张截图及逐文件 SHA-256 |

本机活动诊断实验的目录由 `.artifacts/diagnostics/active.json` 指向。架构 B 的 `diag-live-probes.json` 另保留 51 次读取及各自的真实状态，其中包含按环境未启用的槽位。集群真实节点与生产容量需用对应实例的清单及权限另行核验。

### 本次功能验收的入口与凭证

本轮自验核对实际功能、授权和故障恢复。以下命令中的 `$run` 指 `.artifacts/architecture-refactor/run-20261118`。运维人员打开本次 A、B 环境时，可以按下面三项复核。验收总账单独记录人工接受状态。

| 复核内容 | 操作和预期结果 |
| --- | --- |
| 当前入口与版本 | 分别执行 `node scripts/architecture-walkthrough-control.mjs "$run" a status` 和同一命令的 `b status`。返回的版本、数据库、控制台、大屏及诊断地址应与本侧 `manual/walkthrough-<侧>.json` 一致，启动编号应等于描述符的 `execution.launchID`。两侧的 `sharedConsoleSHA256` 均应等于 `shared-console.json` 中的 `console.sha256`。 |
| 业务与诊断身份 | 从描述符的 `credentials` 获取相应本机账号材料。用业务管理员账号读取两版历史成果，各得十行。用受限工程账号查询获准工程的十行监测数据，再打开另一工程，后者应返回 404。用独立诊断账号登录诊断页面，核对本机观察到的服务启动编号与描述符的 `execution.serverBootID`。 |
| 同一批实例的故障恢复 | 在本侧已运行的环境执行 `node scripts/architecture-walkthrough-rehearsal.mjs "$run" a` 或 `b`。S0 演练解析与 ClickHouse 连接故障，候选再演练消息代理故障。每项都观察探针转为故障后恢复，在中断期间核对正常可读的设施和先前已接管报文。`manual/operator-<侧>.json` 的 `launchID`、`releaseSHA256`、`sharedConsoleSHA256`、`diagnosticBootID` 及 `descriptorSHA256` 绑定实际环境与相应描述符文件。 |

要复验两侧共同控制台的草稿与撤权，运行 `node scripts/architecture-console-drafts.mjs "$run"`。命令使用独立的本机测试租户和账号，同时打开成员暂停与邀请重新生成对话框。复核进行时两处确认按钮均不可用，同一成员和权限复核通过后两处都能继续编辑。由另一账号实际撤销两项权限后，两处操作都应消失，相应服务端命令均返回 403。恢复权限后重新打开表单，操作原因应为空。逐侧查看 `console-corrections/drafts.json` 给出的 HTTP 记录、浏览器观察、截图及逐项加载的前端文件摘要。

要复验列表与键盘焦点，打开本侧描述符的 `consoleCorrections.importFocus.listPath`，点击“下一页批次”，按 `target.name` 找到第二页原行。用 Enter 打开，切到“处理记录”，刷新后按 Esc。原行应重新获得焦点。若关闭时列表仍在读取，可以先点击搜索框并输入。列表返回后，输入内容及操作焦点应保留。命令 `node scripts/architecture-console-import-focus.mjs "$run"` 在两个后端分别控制真实列表响应的送达，逐项覆盖键盘、鼠标、筛选、切换租户和重新载入。运行记录 `console-corrections/import-focus.json` 关联每个用例的预期与实际焦点、HTTP 响应摘要、DOM 记录、截图及静态资源摘要。

C1 在两种后端上都曾出现同一现象，列表在抽屉关闭后才返回时焦点停在搜索框。C2 的 20 个真实浏览器场景均通过。关闭后收到的原始 HTTP 200 晚了 117 至 121 毫秒，两侧都能在原行重新出现时定位它。两侧还分别核对了主动点击或按 Tab 移焦，以及原行被真实取消、筛选变化、切换租户、文档卸载后的行为。每项都保留截图和 DOM 记录，两个后端载入的 131 个前端资源逐项相同。[C1 原浏览器失败的归档清单](../../.artifacts/architecture-refactor/run-20261118/regression/browser-diagnoses/b9-c1-1536661b539fc4d7/manifest.json) [C1 双侧时序复现](../../.artifacts/architecture-refactor/run-20261118/regression/browser-diagnoses/monitoring-import-escape-focus.json) [C2 的实测与资源摘要](../../.artifacts/architecture-refactor/run-20261118/console-corrections/import-focus.json)

两侧人工环境都已运行时，命令 `node scripts/architecture-console-import-focus.mjs "$run" --walkthrough` 直接使用保留的同一批次，复核快速响应、延迟响应与主动 Tab 移焦。两端保持在线，`manual/live-import-focus.json` 记录启动编号、实际发行摘要、同一原行和复核结果。

在保留的 A、B 环境中，样本索引 `manual/walkthrough-<侧>.json` 的 `consoleCorrections.drafts` 列有测试租户、两种角色的显示名以及相对页面地址。两位操作人员可按以下步骤走查同一组样本。

1. 使用两个独立的浏览器配置文件打开本侧控制台。从 `credentials.drafts` 指向的本机私有文件分别取操作人和租户管理员的登录资料。
2. 操作人在 `pages.members` 找到 `target.displayName`，从行菜单打开“暂停成员”。另开标签页，在 `pages.invitations` 找到 `invitation.displayName`，打开“重新生成”。在两个窗口分别填写不同的操作原因。切换窗口后再返回，等待资格复核完成，刚填写的文字应保留。
3. 租户管理员打开 `pages.operatorAuthorization`，选择“授权详情”“配置成员角色”。保留普通成员角色，移除索引所列的管理角色并选中只读角色，按界面完成影响预览和确认。操作人依次回到两个原标签页，两个确认框和对应行操作应消失。
4. 管理员在同一处移除只读角色并恢复管理角色。操作人重新打开两个表单，原因输入框都应为空。要自动复核当前两个在线环境和这批可继续交付的样本，可以执行 `node scripts/architecture-console-drafts.mjs "$run" --walkthrough`。命令完成后，两位操作人的原有编辑资格均恢复，证据位于 `manual/live-drafts.json`。

11 月 19 日 09:32，冻结后端 `cf757341bc3c` 和候选后端 `c0583da0ac5c` 分别通过两项原交互。两侧浏览器实际加载的 108 个文件逐一匹配共同控制台 `b5ded7f1404a` 的清单。复核中按 Enter 后，两侧均记录零次浏览器业务提交。真实撤权后，成员仍处于原活动状态，邀请仍在待领取状态，二者版本均保持原值。重新授权后两个输入框均为空。[两侧浏览器、HTTP 与截图记录](../../.artifacts/architecture-refactor/run-20261118/console-corrections/drafts.json)

同日 09:36 至 09:38，本机 Docker 的内存不足导致原合同环境的两个 ClickHouse 副本先后退出。候选诊断当时已完成 92 项核心检查和契约生成核对，镜像构建随后中断。诊断专属 7 个容器和 14 个原生进程已停车。原合同两个副本使用原卷恢复后，Keeper 状态均显示两个副本在线，跨副本读取的样本逐字节一致。冻结库的三张表与 53 个 S3 对象均重新读取并与原摘要核对。09:45 的恢复凭证和本次中断的原出口单独保存。[原环境数据回读](../../.artifacts/architecture-refactor/run-20261118/manual/original-storage-oom-recovery.json) [中断时的检查点](../../.artifacts/architecture-refactor/run-20261118/regression/candidate/attempts/c0583da0ac5c-interrupted-0939/archive.json)

原诊断中的事务提交失败用例先取一次帧时间，再给每个观察单独取时间。10:01 的原命令跨过了毫秒边界，服务按既有协议以 HTTP 400 拒绝晚于帧时间的观察。定向实测将观察推后一毫秒复现了 400，两侧共用的测试夹具现在从该帧时间构造所有观察。保持库已关闭的条件后，原用例连续五次得到预期的 503，且读侧状态为空，再握手也返回 503。共同夹具以 Go 测试 overlay 安装，冻结源码和两侧服务本身沿用原先的时间校验。各侧 `diagnostics-frame-clock.json` 记录源文件、原摘要和执行摘要。[原命令的真实返回](../../.artifacts/architecture-refactor/run-20261118/regression/candidate/test-diagnostics-unmodified.json) [跨毫秒红例与同步时刻的五次复验](../../.artifacts/architecture-refactor/run-20261118/regression/candidate/attempts/c0583da0ac5c-core-clock-1001/clock-check.json)

B9 的原命令通过前 11 组并恢复了 15 个真实故障后，10:25 的独立部署验证遇到 Docker 默认地址池耗尽。保留原配置再次执行 Compose 的容器创建步骤，守护进程返回同一错误。共同测试夹具为自己的临时项目从 `10.243.0.0/16` 选择可用的 `/24`。B 的独立验证在该网络上完成了真实 HTTPS 登录、页面和资源读取、错误请求及镜像内程序版本核对，随后按项目标签复查容器与网络均已删除。[Docker 原错误与独立恢复记录](../../.artifacts/architecture-refactor/run-20261118/regression/candidate/attempts/c0583da0ac5c-deployment/comparison.json)

### 运维人员在 A、B 对照环境中走查

从仓库根目录打开本次保留的数据。两个终端各运行一条启动命令，保持前台运行便于观察。当前运行目录的地址和专用账号文件由 `manual/walkthrough-a.json`、`manual/walkthrough-b.json` 给出。账号文件路径相对于本次运行目录。重新打开已准备的数据时，启动器复核运行程序、数据库及可供走查的样本。

```sh
run=.artifacts/architecture-refactor/run-20261118
node scripts/architecture-serve.mjs "$run" a --walkthrough
```

```sh
run=.artifacts/architecture-refactor/run-20261118
node scripts/architecture-serve.mjs "$run" b --walkthrough
```

启动器从运行目录的 `candidate.evaluated.json` 读取当前选定的候选。A 的业务后端沿用冻结发行。两侧从 `shared-console.json` 读取本次获准修正的控制台，启动时逐文件校验。B 的业务库和 relay 库独立保存。首次预热时，三个 ClickHouse 表及 53 个对象均与冻结样本重新核对。启动器用选定候选的 CLI 将最初的 58 版业务库迁移至 59。两个环境各自保存实际入口、诊断启动身份及业务样本。[预热时的数据库、挂载和实物摘要](../../.artifacts/architecture-refactor/run-20261118/manual/walkthrough-b8-prewarm.json)

S0 的原冻结清单未列独立诊断程序的摘要。人工环境优先使用通过 S0 原套件的程序，按该次运行记录的 SHA-256 复制到本环境的私有目录。新旧构建均已运行 `version --verify-assets`，返回相同的 UI 构建标识。`manual/walkthrough-a-diagnostic-identity.json` 记录两份程序摘要和原套件完成时间。当前描述符的 `execution.processes.diagnostics` 记录这次实际启动的进程与来源。

打开 B 样本索引的 `diagnostics` 地址，用 `credentials.diagnostics` 所指文件中的合成诊断账号登录。在“运行诊断”选择“查看全部组件与未启用项”，进入“运行组件”。选择类型后，按组件名称进入详情，打开末尾为 `-1` 的副本。在“副本身份”核对最近观察、进程启动标识、就绪和工作循环。打开“检查项”中的报告可以查看相应执行循环、最近的真实尝试和额度。在 A 重复相同步骤，原始角色和实际绑定见下表。保留的 1440、1024 和 375 像素视口截图与每个角色的实时观察位于 `visual/live-topology/`。

| A 中的职责 | B 中的组件 | 走查时关注的内容 |
| --- | --- | --- |
| `api` | `api` 承接原入口，`identity-api`、`control-api`、`monitoring-query-api`、`transfer-api` 分别显示身份、工程、查询及传输状态 | 查看四个业务角色各自的副本与进程启动标识，再从原控制台进入项目，读取设施树 |
| `ingest-worker` 与 `parser` | `ingest-custody-worker`、`parsing-dispatch-worker` 与 `parser` | 核对中继收据、原文摘要、解析结果。停止解析后，独立诊断的 `parser-health` 应从成功变成连接失败 |
| `monitoring-worker` | `monitoring-publish-worker`、`monitoring-incremental-worker`、`monitoring-history-worker`、`monitoring-read-worker` | 依次检查正式发布、增量、历史和读取的各自工作循环，按样本索引切换两代历史结果 |
| PG 事实对账 | `event-relay` 和 `nats-server` | 比对 `publish` 循环、待发送数、最老年龄及消费位置。MQ 停机后用 `nats-server-ready` 查看失败，恢复后查看新的成功观察 |

故障控制在第三个终端执行。先用 `status` 核对本次运行的 `releaseSHA256` 与 `launchID`。每次停止一个依赖，在相应副本“检查项”查看新观察，同时在业务控制台读取设施台账。恢复后再查看更晚的成功记录。A 可以操作 `parser`、`clickhouse`，B 还可以操作 `broker`。ClickHouse 操作切换该走查环境专用的数据库连接，使用 `clickhouse-protocol` 判定故障与恢复。

```sh
run=.artifacts/architecture-refactor/run-20261118
node scripts/architecture-walkthrough-control.mjs "$run" b status
node scripts/architecture-walkthrough-control.mjs "$run" b parser stop
node scripts/architecture-walkthrough-control.mjs "$run" b parser start
```

已在后台运行的人工环境也可由同一本机账号优雅停车。使用 `node scripts/architecture-walkthrough-control.mjs "$run" b park` 停止 B，对 A 改用 `a park`。命令核对正在运行的环境、版本和启动身份，再由该环境自己的主管进程收回下属监听。数据、账号和样本可以用上面的原启动命令再次打开。

需要保存一轮完整的操作和观察，使用 `node scripts/architecture-walkthrough-rehearsal.mjs "$run" b`，对 A 将最后一项改为 `a`。每个依赖的 `before`、`stopped` 和 `recovered` 保留探针 ID、原因及观察时间。两侧均通过业务账号重新打开设施台账与原报文，逐次核对同一收据、来源序列、解析结果和原始字节摘要。B 的消息故障还记录三个阶段的发布积压条数、最老等待时间以及消费进程的 PG 对账次数。仍未发生的发布确认或消费进度可保持缺项。各项记录存于 `manual/operator-{a,b}.json`。走查完毕后，对启动 A、B 的终端分别按 Ctrl-C，可保留下次重开的数据。

11 月 19 日的隔离预演中，S0 于 05:32:36 完成两项故障，b6a60e55efd0 于 05:36:11 完成三项。两侧在每次故障期间仍能读到三项设施，并核对同一份 81 字节原报文、收据及解析结果。parser 和 B 的 broker 均观察到连接拒绝，ClickHouse 的协议探针返回 `adapter_error`，恢复后均返回 `ok`。B 当时没有新增待发布记录，消息停机前、中、后实际积压均为零，任务索引的 PG 对账计数从 11 增至 15，再增至 18。两侧正常退出后，从操作系统复核 A 的 16 个原始及重启进程身份和 B 的 28 个身份均已退出。此次预演的详细现场与入口签名见 `manual/rehearsal-summary.json` 和 `manual/rehearsals/`。

再次启动时，设备的已发布监听、relay 身份与 spool 数据继续沿用。仅供本侧 custody 使用的 mTLS 控制地址在本次回环端口上生成。Docker 重新分配 PostgreSQL 发布端口时，启动器先核对实验容器标签和数据库中每个原登录的槽位标记，然后更新私有连接地址。`manual/walkthrough-republished-pg.json` 留存 22 个已发登录的原凭证复核，以及跨库和未知登录被拒的结果。

核对联合恢复时，打开本次运行的 `deploy/joint-recovery.json`，逐项比较 `keys.before` 与 `keys.after` 的业务及 relay 数据，再核对对象数、恢复 epoch 和从 `verified_read_only` 到 `ready` 的实际记录。核对拓扑切回时，打开 `deploy/rollback-readback.json`，按 `phases` 查看每次切回后的原收据及 GUI、SDK 和 CLI 回读。两份记录各自携带候选摘要，操作步骤和独立恢复目标的配置见[架构运维手册](../operations/system-architecture-refactor.md)。

也可以从已停止的走查环境运行单侧全程。启动器会按运行目录选择冻结 A 或当前 B，核对实际提供的三套入口 HTML 和 JavaScript 主文件，再完成样本复核、故障与恢复。命令退出后会关闭本次启动的进程，保留数据和恢复配置。

```sh
node scripts/architecture-walkthrough-rehearsal.mjs "$run" a --start
node scripts/architecture-walkthrough-rehearsal.mjs "$run" b --start
```

人工访问使用样本索引中的三个原地址。通用原生诊断配置默认只监听 `127.0.0.1:8096`，走查环境另行分配回环端口。部署清单的默认私有端口依次为身份 18441、工程 18442、监测查询 18443、传输 18444、增量试算 18445、兼容 API 18446、网关探针 18447。消息客户端和成员路由使用 4222、6222，单节点只在本机 8222 提供 JetStream 观察。各环境的最终值以生成的 `layout.json`、实际配置及诊断绑定为准。

11 月 18 日的一次完整回归在观察器恢复检查中发现，较早创建的隔离数据库已标为 59，却仍使用开发中间版 SQL。该库缺少 `event_publication_permit(boolean)` 和租户积压索引，PostgreSQL 日志也记录了前台监测 worker 所请求的函数不存在。旧实验保留容器、卷与失败日志。重新验收时从空库执行当前迁移，以实际函数和索引同时核对版本。数据库差异及迁移文件摘要见主运行目录 `diag-lab-schema-transition.json`。旧实验的创建时间、宿主调大内存前的 7 个容器状态、原客户端访问和角色报告另存 `diag-environment-ownership.json`，供恢复宿主设置时核对归属。

### 冻结 S0 和候选版的原套件

在已有基线和候选发行物的架构运行目录中依次执行两侧检查。每侧使用其源码目录中的 `pnpm test:diagnostics:environment` 和 `pnpm test:diagnostics`。S0 启动前按冻结清单逐文件验证应用、服务、共享包、CLI、契约和诊断测试源。验收端核对常驻角色的发行二进制并实际重启独立实验中的对应进程，PID 和 SHA-256 存于 `regression/<side>/diagnostics-residents.json`。其他记录包含实际数据库 schema、发布函数和索引、原生服务及 CLI 摘要、目录摘要和最终运行的 Docker 镜像 ID。两个原总命令入口的内容相同。

```sh
run=.artifacts/architecture-refactor/run-20261118
node scripts/architecture-regression.mjs "$run" candidate test:diagnostics
node scripts/architecture-regression.mjs "$run" baseline test:diagnostics
```

每侧的总命令退出码、输出和时长存于 `regression/<side>/test-diagnostics.{json,log}`。只有同一实验的 13 项收据都在本次命令执行期间生成并通过，且 9 张 PNG 检验通过，才会更新 `diagnostics/summary.json`（候选）或 `diagnostics-baseline/summary.json`（S0）。打开相应摘要可以核对逐文件 SHA-256，`deployment.json` 可以核对运行过的镜像。两侧通过后，`diagnostics-comparison.json` 另核对独立实验身份、相同原总命令及 22 项浏览器和 15 项故障的逐项结果。

冻结版另保存一次其原浏览器驱动的独立执行及日志。原驱动的页面截图、场景记录及文件摘要保存在 `regression/baseline/frozen-diagnostics-browser/`。共同验收驱动按各自清单计算 worker 数，检查实际分页文案，并等待权限故障先到达真实 API，再验证已打开的页面自动更新。故障套件跟踪一个独立控制容器的标签、运行状态与原始启动时间，以便同一宿主上的其他实验能独立开始或结束。每次恢复都实际查询原会话，收到 401 时重新认证。业务整体掉线的场景另行强制新登录。遇到 429 时，测试客户端按服务端给出的 `Retry-After` 在 75 秒总时限内等待，并记录等待时间。旧版实验建库脚本还需给人员 API 提供 S0 本身要求的 MFA 密钥文件。验收启动器在各自实验目录生成独立的 0600 私钥文件。两份驱动的原判断、共同判断、完整场景列表、测试客户端和启动配置的文件摘要位于 `regression/baseline/diagnostics-harness.json`。这些调整只加载到验收进程，S0 源码和服务文件均来自冻结归档。

2026 年 11 月 19 日在真正的 S0 实验 `diag-b2e7d4d175b633ac` 重现了三处冻结交付差异。

| 冻结项 | 原始结果 | 同口径复核 |
| --- | --- | --- |
| 默认部署模板 | 最后一轮完全使用原驱动的 `pnpm test:diagnostics` 于 04:39:38 在模板字节比较时退出 1，用时 28.861 秒。原生、Docker、混合模板都只登记 24 个组件和 24 个副本。冻结 CLI 和冻结运行目录均含 28 个，还将 `monitoring-worker` 命名为“监测、派生计算与试解析”。签入的名称为“监测与派生计算” | 从冻结运行目录独立取出 `ingest-worker`、`ingest-relay`、`ingest-relay-postgres`、`parser` 四个条目，按模板原有的禁用位置重建它们的组件与副本，并应用冻结目录中原有的标题。三套配置和其余模板文件随后通过原生成器的完整字节比较。原始退出见 `regression/baseline/test-diagnostics-unmodified.json`，三份输入的摘要和对照见 `regression/baseline/frozen-diagnostics-templates/comparison.json` |
| 组件筛选 | 冻结浏览器驱动预期“共 8 项 · 当前快照”，在第 4 项退出 1。原浏览器实测为 9 个工作进程，筛选后分页显示“共 9 条”，API 链接数为 0 | 共同驱动在每代真实页面上复核本代运行目录的 worker 数、分页文本及 API 行过滤。冻结页面、另取的筛选截图及实际文本见 `regression/baseline/frozen-diagnostics-browser/` |
| 接入 worker | 原实验清单把 `ingest-worker` 登记为必须常驻，生成的业务配置缺少其启动时必须读取的 `ingestion_file`。冻结二进制实际退出并上报 `dependency_unavailable` | 在该实验的私有目录配置冻结服务原本支持的空中继列表、预算及其真实 PG 和对象存储。重新启动后验证所有预期探针新鲜、无异常，保留该工作进程及其所有诊断检查。见 `regression/baseline/diagnostics-ingestion-fixture.json` 和 `regression/baseline/diagnostics-residents.json` |

S0 的第一次共同回归在 04:27:40 退出 0，用时 1,244.842 秒，13 组收据、22 个浏览器场景、15 个故障和 9 张截图均通过。停车时 46 个探针全部更新，19 个副本就绪。随后核对发现，原进程集成测试的 `go build` 曾把 7 个常驻程序路径覆盖为同一份源码的重编译文件。七组 Go 构建元数据均匹配工具链、模块路径、依赖和提交，变化的构建设置为 `vcs.modified`。实际有 8／15 个文件匹配封存摘要，本轮只作为功能证据。每个原始和重建摘要见 `regression/baseline/frozen-diagnostics-templates/owning-rebuild-drift.json`。

架构回归现在将这些 Go 编译产物放入实验的私有临时目录，原进程集成测试从选定发行目录的核对副本启动。命令结束时从操作系统重查 PID、启动时间和程序，再逐个计算文件摘要。需要停车时还会核对副本、容器和卷，并在停车后再次核对文件。单侧的 `regression/<side>/diagnostics-binaries-after_command.json` 和 `diagnostics-binaries-parked.json` 记录每个身份，跨代汇总的 `release_binaries` 只收录同一源码、同一命令结束时间的凭证。

该轮 S0 的完整共同回归于 04:54:42 退出 0，用时 890.097 秒。11 个原 Go 命令均在独立目录实际编译，领域用例使用的 11 个业务程序全部匹配冻结发行物。核对结果包括 82 个核心测试、15 个进程场景、17 个固定协议场景、27 个网络场景、15 个已恢复故障和 22 个浏览器场景。13 组同次收据和 9 张截图的摘要均已复算。结束时逐个核对 15 个进程的操作系统身份和程序文件，全部匹配。04:55:07 停车前 46／46 个探针更新、19 个副本就绪，停车后 15 个进程和 7 个容器均已停止，6 个原卷保留，15 份程序摘要再次全部匹配。与 b4a1f272c809 的比较已归档在 `regression/candidate/attempts/b4a1f272c809-qualified/diagnostics-comparison.json`。双方共同夹具的退出码为 0／0，S0 的三条原始检查退出码为 1／1／1，b4 的 14／14 份封存程序均匹配。

需要暂停实验时，可核对已归档的实验身份后停车，再恢复同一实验。两条命令均记录探针客户端的实际可达性、各角色的进程状态、7 个容器及其具名卷。接受 `baseline` 或 `candidate` 作为最后的版本参数。

```sh
node scripts/architecture-diagnostics-harness.mjs --park "$run" candidate
node scripts/architecture-diagnostics-harness.mjs --resume "$run" candidate
```

11 月 19 日的 b315 候选原总命令曾在第 4 项故障恢复后因验收客户端反复登录而收到 429。修正客户端后，15 项真实故障全部恢复，11 项复用了仍有效的会话。磁盘压力、状态目录不可写、账号撤销和诊断服务停机这 4 项均先观察到 401，再成功重新认证。全组实际登录 8 次，逐项记录、原失败和脱敏观测的摘要在 `diag-fault-sessions.json`。再启动时按原实验身份复核同一组容器、卷与进程，见 `regression/candidate/diagnostics-resumed.json`。

前一候选 b4a1f272c809 的原总命令于 11 月 19 日 02:25:12（北京时间）退出 0，用时 928.956 秒。归档核对了本次 13 组收据、22 个浏览器场景、15 个已恢复的故障以及 9 张截图。该次故障组实际登录 8 次，服务端未要求重试。完成时的应用源摘要与 b4 封存发行一致。02:25:30 再次停车前，44 个探针全部更新，18 个副本就绪。停车后核实 14 个原生进程和 7 个容器均已停止，6 个具名卷及容器身份完整。其原命令、套件摘要和停车凭证保存在 `regression/candidate/attempts/b4a1f272c809-qualified/`。

中间候选 b6a60e55efd0 于 05:20:31 完成同一原总命令，退出 0，用时 1,032.409 秒。复算了本次 13 组收据和 9 张截图的文件摘要。22 个浏览器场景通过，15 个真实故障均已恢复，故障组使用 8 次登录。测试结束与 05:21:22 停车时的 14 份程序均匹配新发行。停车前独立客户端读到 44／44 个新鲜探针和 18 个就绪副本，停车后 14 个本机进程和 7 个专用容器均已停止，6 个数据卷保留。本次 36 份文件和摘要已归档在 `regression/candidate/attempts/b6a60e55efd0-qualified/`。

候选 b7 的核心、原生、进程归属及契约四组完成后，空集群升级暴露了审计组缺失的错误。原总命令在阶段边界停止，14 个进程和 7 个容器全部停车。本次局部结果单独保存在 `regression/candidate/attempts/97f6d452475e-interrupted/`。

候选 B8（`59a985d1a8ad`）于 06:38:18 完成原总命令，耗时 921.698 秒，退出 0。13 组收据中包括 92 个核心测试、22 个浏览器场景、15 个已恢复故障和 9 张截图。06:38:57，另行启动的 13 个拆分进程及 JetStream 完成故障验证，两个后端网络命名空间均实收代理连接，停用一个后端后由另一个继续承接。四种部署布局实际核对了 31、31、30、31 个槽位。[拆分进程、消息与代理实测](../../.artifacts/architecture-refactor/run-20261118/diag-role-faults-59a985d1a8ad.json)

停车前，独立客户端读到 44／44 个新鲜探针和 18 个就绪副本。06:39:41 核实诊断专属 14 个原生进程和 7 个容器均已停车，6 个数据卷保留。结束与停车时 14／14 份程序均匹配 B8 发行。冻结 S0 的 15／15 份程序在各自两个阶段也均匹配。[B8 完整归档](../../.artifacts/architecture-refactor/run-20261118/regression/candidate/attempts/59a985d1a8ad-qualified/archive.json)

B9 的 13 个拆分业务进程已分别从该次发行启动，逐进程验证身份、探针失效和恢复。消息节点另行真实停启，双后端代理在一端停机后继续提供服务。四种部署布局核对 31、31、30、31 个槽位。[B9 故障实测](../../.artifacts/architecture-refactor/run-20261118/diag-role-faults-c0583da0ac5c.json)

B9（`c0583da0ac5c`）使用共同夹具的原总命令于 11:51:29 退出 0，耗时 1,204.547 秒。13 组均通过，其中包括 92 个核心检查、22 个浏览器场景、15 个故障及其恢复。9 张截图的摘要也已逐一核对。独立部署的容器实际使用 `10.243.254.2`，退出后释放了容器和子网。命令结束时和 11:51:52 停车时，14 个常驻程序的摘要均与发行匹配。停车前 44／44 个检查新鲜、18 个副本就绪，停车后 7 个容器全部停止、6 个卷保留。[35 份原件与 SHA-256 清单](../../.artifacts/architecture-refactor/run-20261118/regression/candidate/attempts/c0583da0ac5c-common-clock-network-2026-11-19T03-31-25.428Z/archive.json)

冻结 S0（`cf757341bc3c`）使用相同夹具的总命令于 12:17:16 退出 0，耗时 1,141.478 秒。13 组通过，包含 82 个核心检查、与 B9 相同的 22 个浏览器场景和 15 个已恢复故障。9 张截图均已核对。其独立容器使用 `10.243.61.2`，容器与网络均已释放。结束时和 12:17:45 停车时，15 个程序均匹配冻结发行。停车前 46／46 项检查新鲜、19 个副本就绪，停车后 7 个容器全部停止、6 个卷保留。两侧共同夹具的完整退出码为 0／0。S0 未改总命令、模板和浏览器原检查的退出码为 1／1／1，B9 未改总命令的跨毫秒失效退出码为 1。[S0 40 份原件](../../.artifacts/architecture-refactor/run-20261118/regression/baseline/attempts/cf757341bc3c-common-clock-network-2026-11-19T03-58-14.790Z/archive.json) [两侧逐项对照与运行程序身份](../../.artifacts/architecture-refactor/run-20261118/diagnostics-comparison.json)

最终候选 B10（`b09feb944c39`）的原始 `pnpm test:diagnostics` 于 13:50:25 退出 0，耗时 18 分 09.897 秒。紧接着使用 A、B 共同夹具运行同一命令，14:07:49 退出 0，耗时 17 分 24.168 秒。两次分别留下 13 组报告，其中有 92 个核心检查、22 个浏览器场景、15 个故障及恢复，另各保留 9 张截图。原命令的每份报告与截图都在本次原命令的实际时间窗内，其摘要另经离线归档复核。共同命令结束时和停车时，两次均核对到 14/14 个选定程序的启动身份与文件摘要。14:08:24 停车时保留了七个已停止的原容器和六个卷，最后一帧报告 44/44 份观察新鲜、18 个组件就绪。[未覆盖原命令与 13 组原件](../../.artifacts/architecture-refactor/run-20261118/regression/candidate/attempts/b09feb944c39-raw-2026-11-19T05-32-15.749Z/summary.json) [共同命令、发布身份和停车收据](../../.artifacts/architecture-refactor/run-20261118/regression/candidate/attempts/b09feb944c39-common-clock-network-2026-11-19T05-50-25.658Z/archive.json) [与冻结 S0 的逐项对照](../../.artifacts/architecture-refactor/run-20261118/diagnostics-comparison.json)

14:09:00 的 B10 独立故障核验使用同一发行的 13 个拆分及备用程序，依次观察真实进程、启动报告和退出结果。另有 Linux 容器内的 PID 1 与 UID 观察、JetStream 从可用到连接拒绝、两个网络命名空间的实际代理转发及失去一个后端后的继续路由。四套布局分别识别 31、31、30、31 个部署槽位。[B10 分角色、代理与消息代理原件](../../.artifacts/architecture-refactor/run-20261118/diag-role-faults-b09feb944c39.json)


## 领域事实与受限度量

| 检查 | 实际结果 | 主运行目录证据 |
| --- | --- | --- |
| 通知、审计、任务 | 按原始事实身份、范围及代次推进指定操作。错误范围或旧代次无法推进另一条作业，任务投影保留既有乱序和原子历史语义。受限大屏交接继续经过原授权翻译 | `diag-aux-go-tests.jsonl` |
| 分租户积压 | 新建隔离 PG schema 并迁移至 59，单连接读取 15 个合成租户。首轮 12 组，下一轮 3 组。逐组核对待发和租用中的总数，排除平台、已发和其他恢复代号。另有 8,192 条已发送记录时，实际执行计划使用 `event_outbox_tenant_pressure` 部分索引。冻结后返回空组。报告将租户 UUID 转为固定长度摘要 | `events/tenant-queue.json`、`diag-tenant-queue.jsonl` 和 `diag-aux-go-tests.jsonl` |
| ClickHouse | 两个物理端点分别持有一个连接额度。其中一个端点被慢查询占用时，同端点的第二次调用按期超时，另一端点仍可用。慢查询完成后原端点再次可用 | `diag-storage-budgets.jsonl` |
| 对象存储 | 两个真实对象客户端共享一个限额。读取一个 2 MiB 对象并保持响应体打开时，另一客户端的 bucket 检查超时。流关闭后检查成功。对象与私有 bucket 在用例结束后清理 | `diag-storage-budgets.jsonl` |
| 同角色两个副本 | 将 3 个 PG 连接及 5 个在途额度分成 2/1 与 3/2。真实子进程占用首槽，重复进程被拒，次槽可用。终止次槽最后一个数据库会话后，存活进程失去所有权。终止首槽进程后，新进程成功取得相同槽 | `diag-replica-budgets.jsonl` 中的 `TestReplicaBudgetRejectsDuplicateAndRecovers` |
| PG 停顿与终止 | 真实 PG 连接的写通路短暂停顿 4.25 秒。空闲守卫未写入探活请求，恢复后仍使用原 PostgreSQL 后端 PID。使用独立管理连接终止真实后端时，进程仍按预期失权 | `diag-passive-pg.jsonl` |
| 浏览器取消并行请求 | PG 配额 12，使用真实 HTTP 和 `access.Transact` 同时取消 3 条已在执行的事务。b315 的下次就绪请求实际返回 503。修复后就绪返回 200，取消前实际已分配的所有后端 PID 均保留。将 PG 的 57014 取消确认延迟 1.25 秒，后续事务仍使用原 PID。管理员终止全部真实后端后，存活进程确实失权 | `diag-browser-cancellation.json` 和 `diag-browser-cancel-final.jsonl`，5 项真实 PG 竞态专项各重复 3 次 |
| 原控制台工作空间导航 | 在 1440、1024 和 375 像素视口中，b315 的项目工程树均返回 503，导航停在第 5 步。相同浏览器操作在 b4a1f272c809 的三个视口均收到 200 并完成 24 步。在各视口的独立诊断中，8 个角色都与实际进程身份相符，报告已更新，控制服务就绪 | `diag-browser-cancellation.json` 的 `live_browser` 含修复前后各 3 份导航记录的摘要、控制服务实际失权时间和新拓扑记录 |
| 长闲连接和缩池 | 连接闲置超过 1 秒后借出，普通 pgx 默认执行一次协议写，登记槽借出时为 0。保持 3 个空闲后端时，原空闲检查持续刷新每个后端的使用时间。修复后按配置回收其中 2 个，余下 1 个继续承载真实事务和所有权，单连接角色空闲探针始终只读 | `diag-idle-shrink-before.jsonl`、`diag-browser-cancel-final.jsonl` |
| HTTP 在途数 | 同时通过三个真实 HTTP 监听占满 5 个额度。首副本的两个监听共享 3 个额度，次副本可用 2 个。三个监听的超额请求均超时，完成在途请求后各监听重新获得容量 | `diag-replica-budgets.jsonl` 中的 `TestReplicaHTTPBudgetAcrossListeners` |
| 角色与槽位证书 | 两个服务通过真实双向 TLS 和人员会话核验业务路由。反序列出的角色／槽位 SAN 成功核验。带两个角色、外部槽、缺少槽及服务端槽错误的证书均未获准。有效控制角色不能代查身份 owner 的另一条操作 | `diag-replica-budgets.jsonl` 中的 `TestArchitectureReplicaMTLS` |

预期证据只保留归属角色、技术原因码、状态、单调水位和有限维度。业务操作员可用原业务操作记录中的请求、命令、收据与作业身份另行对账。诊断账号及恢复令牌使用相应隔离实验的私有文件。

### 大屏核对固定基准

冻结 S0 用 Go 的 `json.Marshal(fixedBaseline)` 生成证据与 SHA-256。自引入固定基准的提交 `525cee502996` 至 S0，写入结构、快照和输入范围使用的 JSON 字段与顺序相同。标记为 `omitempty` 的四个观测或结果标识在空值时省略。未设置的输入范围切片写作 `null`，已分配的空切片写作 `[]`。未设置的范围端点写作 `null`。复制配置和分析版本时，数据库原样复制证据及摘要。

迁移 59 在同一事务内按原有字段顺序和范围顺序重建上述字节，核对完整摘要、引用和快照编号。大屏运行角色可取得快照和输入范围。原始分数的分子和分母只参与库内摘要核对。损坏的已存行返回完整性错误，避免资格检查进入缺少固定基准时的默认选取逻辑。

重新打开已停车的旧版 59 诊断库时，回归入口会比对实验标签、PostgreSQL 容器身份和实际发布的回环端口。独立回归只对该容器内的 `diagnostics_lab` 库执行选定发行的 `infrapro database migrate`，补齐函数或修复随 `--no-acl` 恢复的默认执行权限。迁移撤销 PUBLIC 调用权限，原审计组存在时授予该组 EXECUTE。检查随后用可用的审计组或迁移账号，在只读事务内实际调用。数据库版本、原恢复代次、权限检查和 CLI 摘要记录在 `regression/candidate/diagnostics-schema.json`。

B10 的绑定同时检查 `architecture_notification_invitation_targets` 的存在性、PUBLIC 执行权以及已安装的 control 角色授权。13:32 的自有旧 59 库仍缺这个函数。选定的 B10 CLI 补装后保留原恢复代次，两组函数均通过只读事务，邀请投影以空收据集合返回零行。独立的分角色实验库也逐项证实 control 可执行、PUBLIC 无执行权。[本次旧库补装记录](../../.artifacts/architecture-refactor/run-20261118/regression/candidate/diagnostics-schema.json) [冻结与候选库的权限对照](../../.artifacts/architecture-refactor/run-20261118/regression/candidate/attempts/b09feb944c39-projection-probe/qualification.json)


`deploy/fixed-baseline-authority.jsonl` 记录 PostgreSQL 17 自定义 schema 从 58 升到 59 后的十组实测，包括空范围、中文及转义字符、范围换序、分子或分母变化、引用与快照不一致，以及两个实际角色的权限检查。`deploy/fixed-baseline-install.jsonl` 记录并发首次迁移、早期 59 的正常补装及恢复代次复核。`deploy/fixed-baseline-restored-acl.jsonl` 记录已存在的函数恢复 PUBLIC 权限后的修复，以及迁移对已命名的其他调用角色授权的保留。三份实测记录均退出 0。

空 PostgreSQL 集群提供了另一组还原证据。b7 的 S0 转储在 58 缺少全局审计组，迁移报 `42704`。候选 `59a985d1a8ad` 在相同条件下从 58 升至 59。恢复已有 59 时，普通迁移和直接安装部署权限两条路径均撤回了默认 PUBLIC EXECUTE，读回相同的账号集合与原事件代次。获准调用方的空查询成功，其他角色收到 `42501`。[原失败记录](../../.artifacts/architecture-refactor/run-20261118/deploy/cold-grant-attempts/cdd31ec76c7f5df9.json) · [B8 三种还原路径实测](../../.artifacts/architecture-refactor/run-20261118/deploy/cold-restore-grants.json)
