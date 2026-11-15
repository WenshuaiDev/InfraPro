# 统一任务中心实施与自验记录

日期：2026-11-13。状态：**Awaiting acceptance（待人工验收）**。用户已经确认[完整开发方案](../requirements/task-center-development-plan.md)；TF-01–TF-08、14 类实际来源及 TA-01–TA-24 的实现和必要自验均完成。浏览器最终检查于 2026-11-13 22:04（Asia/Shanghai）通过；人工接受及生产部署尚未执行。

## 基线、范围与实现

在当前 `/Users/hijacker/Documents/ChatGPT/InfraPro` main 工作区实施，未创建分支或 worktree。开工 HEAD 为 `ab8b2feee207b00829ab8765848d11af02c438b7`；`.artifacts/task-center/baseline-manifest.json` 记录当时含未提交改动的源码 SHA-256，完成快照及差异为 `final-manifest.json`、`final-delta.json`。快照包含本轮期间其他文档变动，不把整个工作区差异都归为任务中心改动。已核对 AGENTS.md 和既有 00034–00036 的 hash 与开工相同；既有大屏、空间、通知实现保留并在相应接缝接入。

| 交付 | 实现 |
| --- | --- |
| TF-01 | taskrecord 有限目录、稳定 SourceRef、十进制代次／修订、真实进度、执行／尝试／事件、来源 Adapter、契约与接入指南。 |
| TF-02 | `/tasks` 默认 mine、managed、空间／状态／类型／时间／检索、授权后分页、共享详情、执行历史、刷新及旧路由恢复。 |
| TF-03 | 14 类实际来源及真实回算展示分组；自动／内部工作在获准运行概况内。 |
| TF-04 | 原来源事务控制，当前资格、固定输入、预期代次／修订、理由与原回执，重复和并发重试保护。 |
| TF-05 | 固定业务目标；查询回看原快照；导出原文件检查；完成与结果可用／采用／发布分离。 |
| TF-06 | 来源终态通知 v2 固定代次，v1 指纹／接收人／已读兼容，旧通知历史提示，索引不发通知。 |
| TF-07 | 管理摘要与结果分权、当前资格、审计关联、索引诊断、受限 SQL 角色、恢复与重放。 |
| TF-08 | v37–v39 增量迁移、来源历史回填、独立索引进程／Compose／CLI、真实恢复、合成账号环境及浏览器材料。 |

事务登记采用来源白名单快照触发器加同事务 FlushTx/ApplyTx，覆盖既有直接 SQL 和内联成功出口；大屏用自有不可变 handoff。完整实现说明在[接入指南](../development/task-center-integration.md)。公共模块没有新增业务执行队列、任意任务创建或任意文件下载入口。

## 环境与命令

本机 Node 26.10.0、pnpm 12.18.0、仓库 Go workspace；真实隔离 PostgreSQL／Redis，`deploy/monitoring-contract/compose.yaml` 的 ClickHouse 与 S3，仓库空间转换器及生成样本。功能测试使用 `monitoringintegration` tag、race detector、每个 fixture 独立数据。未运行 load、soak、性能或压力路径。

| 检查 | 当前记录／证据 |
| --- | --- |
| pnpm check | 通过；`.artifacts/task-center/check.log`。包括 Go fmt/vet、lint、TS／路由、全部契约和权限政策覆盖。 |
| pnpm build | 通过；`.artifacts/task-center/build.log`。Go 二进制含 task-index-worker、所有前端应用。 |
| pnpm test:tasks | 通过；`test-tasks.log` 和 `sources.log`。31 个来源主场景通过、无跳过（581.280 秒）；宿主 fence、权限升级、HTTP（86.573 秒）、完整通知模块（61.726 秒）和 3 个客户端用例通过。 |
| pnpm tasks:browser | 通过，12 项旅程，未捕获页面异常；`browser/result.json`。已逐张检查桌面、375px 列表／详情／理由表单、200% 页面缩放和管理摘要截图。 |
| pnpm tasks:demo | 通过并保持本机运行；初始 57 个实际来源任务覆盖 14 类，浏览器真实操作后共 62 个。`lab/task-acceptance.json` 保存完整对应关系，临时账号仅在受保护本地文件。 |
| 现场分页与索引 | `live-pages.json`：A 16 项／2 页、B 46 项／5 页、M 管理摘要 12 项／2 页，当前精确计数一致，无重复或跨账号个人任务；M 无结果读取。`status-final.json` 为 ready、六个来源 complete、交接 pending/failed 均 0；`verify-final.json` 为 true。 |
| 最后查询修复回归 | `task-final.log`：9 个 Task 来源／权限／分页／迁移恢复场景全部通过（125.399 秒）；覆盖最终填页批次与事务内范围复用。 |
| 最终请求预算回归 | `final-budget-regression.log`：taskrecord/tasks 契约与任务／通知 HTTP 场景通过（HTTP 87.432 秒）；`config-final.log`：现有配置测试通过。 |
| 权限目录升级回归 | `catalog-upgrade.log`：监测旧版本、任务首次发布、合并升级在归档／停用情况下成功；已有目录扩权／部分发布拒绝；自定义角色与成员关系不变。 |
| 专项修复回归 | `repair-targeted.log`：历史分组迁移、旧／排队来源迁移；`query-pagination.log`：严格请求编码回看固定查询，以及 101 个候选的授权分页和并发新增。 |

已发现并修复：契约生成漏同步来源 TaskRef、权限目录断言新增项、迁移夹具复制 range／identity 列、分析试算过期元数据字段、管理控制权限与设备事务接缝、旧代乱序尝试保存、丢失响应时的命令幂等和刷新状态核验、查询导航中的可选空字段编码、数字代次深链接、归档项目／停用租户的首次任务权限目录发布、375px 工作空间筛选溢出及窄屏列表、分页不必要地核验整批候选导致超时，以及任务页与顶栏重复全范围计数竞争请求预算。测试失败按修复后重跑记录，不作为通过或跳过。

元数据超时从方案的 2 秒初值有界调整为 5 秒：浏览器并发的通知摘要会在既有身份检查中持有同账号锁，任务请求因此需要授权等待时间。未修改身份锁语义；候选上限 2000、单页上限 100 和明确超时错误保留。这是功能调试结果，不是规模或性能验收成绩。

## TA-01–TA-24 对照

下列为已通过的证据位置；最终命令成功且浏览器 result.passed=true，整体软件自验通过，人工接受另行记录。

| 条件 | 自动验证与人工可观察结果 |
| --- | --- |
| TA-01 | tasks-acceptance 通过真实命令生成 A 两租户两项目、B 独立租户／项目；浏览器 mine 与当前空间切换。 |
| TA-02 | TaskMonitoringManagerBackgroundQueryAndPagination、TaskCenterDisplayRealLifecycleAndIndexIndependence、TaskHTTPAuthenticationCSRFStrictInputAndIdle；A/B/M 浏览器切换。 |
| TA-03 | TaskPostAuthorizationPageBudgetAndStaleCursor：101 个候选中 4 个可见、授权后满页、翻页中新增仅首页刷新出现、签名篡改、明确扫描预算；浏览器 10 条分页无重复。 |
| TA-04 | 同上和 TaskMonitoringManagerBackgroundQueryAndPagination：撤权后游标／详情拒绝；恢复测试废弃旧 epoch；账号变更清缓存。 |
| TA-05 | MonitoringCSVPublicationAndFixedDuplicateSet、MonitoringActualAdoptionsRevokeAndReadopt、PublicationCommitFailureAndRecovery：真实校验／采用／noop／撤销。 |
| TA-06 | TaskMonitoringManagerBackgroundQueryAndPagination：实际 background query、固定快照回看及管理者不能读查询参数；MonitoringExportFixedSnapshotAndTypedParquet 和过期断言。 |
| TA-07 | DerivedExplicitDependencyBackfillWaitsForSelectedUpstream、DerivedExplicitPrestartBackfillAndRetiredCorrection、DerivedSixteenDistinctInputsFormalAndReplay：依赖、分组、回放、原代、回填恢复。 |
| TA-08 | AnalysisAtomicMultiOutputVersionsAndChangedInputs、AnalysisWholeRangeProjectOwnershipHistoryAndArchive，以及 fixture 各类分析与两类导出；新鲜试算结果立即按元数据检查。 |
| TA-09 | TaskSpatialManagersRetryHistoryAndOriginalIdentity：真实无效 GLB、另一管理者重试、初始人保留、旧报告封存、执行人改变、结果不自动发布。 |
| TA-10 | TaskCenterDisplayRealLifecycleAndIndexIndependence、DisplayPackageWorkerLeastPrivilege、DisplayLifecycleAndSubjectBoundary：真实 ZIP 校验、取消／重试、尝试上限／租约和受限角色。 |
| TA-11 | TaskMonitoringManagerBackgroundQueryAndPagination、TaskDerivedManagerCannotReplaceLostExecutor、TaskSpatialManagersRetryHistoryAndOriginalIdentity：最小停止权限不变成数据／成果／源文件权限。 |
| TA-12 | TaskDerivedManagerCannotReplaceLostExecutor：禁用原执行账号后能力明确禁用、命令拒绝且未增加代次。 |
| TA-13 | TaskRecordAtomicHistoryAndOutOfOrder、TaskCenterDisplayRealLifecycleAndIndexIndependence、TaskSpatialManagersRetryHistoryAndOriginalIdentity：内部／人工代次、双管理员并发、一键重放和异载荷冲突；浏览器丢失响应重放。 |
| TA-14 | CalculationSharedTaskLeaseFenceAndCancellation、MonitoringExpiredWorkerAndWholeBatchInvalidInput、MonitoringUnsealedPhysicalWriteUsesNewAttempt、SpatialUploadCancelAndProcessingFences；清理未知不宣称完成。 |
| TA-15 | TaskRecordAtomicHistoryAndOutOfOrder、DisplayNotificationLateCommitAndFailedReplay、TaskIndexRestrictedRole：重复／乱序／迟提交、旧代关闭记录不改写。 |
| TA-16 | TaskCenterDisplayRealLifecycleAndIndexIndependence、全部 notifications 及 HTTP Notification 回归：索引／投递独立，现场读取来源真相，偏好／后续投递不重复执行业务。 |
| TA-17 | notificationevent 的 v1 固定序列化、TaskIndependentBackupRestoreAndLegacyNotification、通知 HTTP 回归、浏览器旧通知打开旧代。 |
| TA-18 | verifyTaskSources 用不配 S3／ClickHouse／转换器的 ComposeTasks 读取真实已生成来源；源故障注入和结果访问错误保持独立。 |
| TA-19 | TaskMigrationWithRealRetainedAndQueuedSources、空间和真实依赖回算的 verifyTaskLegacyUpgrade：v36→v39 原表字段 hash 不变，单行分页断点续扫，无伪造尝试／通知／业务重跑。 |
| TA-20 | TaskIndependentBackupRestoreAndLegacyNotification：真实 pg_dump／隔离 restore，20 类持久表一致，旧游标作废、旧失败／回执／v1 指纹不变，无成功业务再执行。 |
| TA-21 | MonitoringExportCommitFailureAndArchivedProject、AnalysisWholeRangeProjectOwnershipHistoryAndArchive、原项目／租户权限回归和输入撤销后 ResultState=invalidated；导出过期与成功状态分离。 |
| TA-22 | TaskHTTPAuthenticationCSRFStrictInputAndIdle、实际 automatic／display ReadSubject 来源、索引事实过滤及运行概况；不存在个人列表中的维护／投递工作单元。 |
| TA-23 | tasks-browser-check：1440px、375px、键盘／Escape 焦点、CSS zoom=2 的 200% 页面缩放、理由保留、刷新失败隐藏旧行和恢复。 |
| TA-24 | 原 monitoring jobs `?job=`／批次／计算／空间／大屏 TaskLink 和共享 TaskSourcePanel；浏览器旧链接、原业务目标、通知固定代次。 |

## 交付与限制

运行与恢复：[task-center.md](../operations/task-center.md)；人工步骤：[task-center-acceptance.md](../operations/task-center-acceptance.md)。本机验收入口 `http://127.0.0.1:15573/tasks`，合成凭证在 `.artifacts/task-center/lab/accounts.json`（0600）。完整自验通过，状态为 Awaiting acceptance；用户明确确认后才能改为 Accepted。

本轮未生产部署、未提交或推送 Git、未代替用户人工接受。性能／压力／长时观察按既定范围暂缓；Safari、物理手机及生产网络未实测。原业务对象真实持久化和元数据备份恢复分别验证，不将隔离恢复等同于生产灾备演练。
