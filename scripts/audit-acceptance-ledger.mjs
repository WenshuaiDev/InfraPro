import {readFileSync,writeFileSync,existsSync} from 'node:fs';
const root='.artifacts/audit/';
const commands={core:'pnpm test:audit',governance:'pnpm test:audit:regression governance',resources:'pnpm test:audit:regression resources',repaired:'pnpm test:audit:regression repaired',metadata:'pnpm test:audit:regression metadata',derived:'pnpm test:audit:regression derived-metadata',source:'pnpm test:audit:regression source-repair',compatibility:'pnpm test:audit:regression compatibility',filesRestore:'pnpm test:audit:regression files-restore',resourceRepair:'pnpm test:audit:regression resource-repair'};
const logs={core:'domain.log',governance:'governance-final.log',resources:'resources-final.log',repaired:'regression-repaired.log',metadata:'regression-metadata.log',derived:'regression-derived-metadata.log',source:'regression-source-repair.log',compatibility:'regression-compatibility.log',filesRestore:'regression-files-restore.log',resourceRepair:'regression-resource-repair.log'};
const evidence=(log,...names)=>names.map(name=>({log:logs[log],command:commands[log],test:name}));
const c=(...n)=>evidence('core',...n.map(s=>s.startsWith('Test')?s:'TestAudit'+s));
const r=(...n)=>evidence('resources',...n);
const g=(...n)=>evidence('governance',...n);
const b={json:'browser/result.json',command:'pnpm audit:browser'};
const h={json:'history-browser/result.json',command:'pnpm audit:history'};
const contract={json:'contract-guard.json',command:'node scripts/audit-contract.mjs --check（含未登记 producer 拒绝探针）'};
const deliver={json:'delivery-checks.json',command:'pnpm check / pnpm build / pnpm audit:contracts / 演示与文档核对'};
const rows=[
 [1,c('SourceIdentityAtomicityAndBounds','ConcurrentReplayReturnsOneFact'),'Writer 并发事务／平台','相同来源重放同一 ID；不同内容冲突并回滚。'],
 [2,c('InvitationEffectsAndLegacyUnknown'),'InviteAudit／租户邀请','接受与成员加入分别存在，operation 相同；重放不新增成功事实。'],
 [3,[...c('EmptyExportCancelRetryAndOwnerIsolation'),...r('TestTaskSpatialManagersRetryHistoryAndOriginalIdentity','TestFilesTaskActorAndLegacyFingerprint')],'所有人、管理员、worker／三空间','原发起人、本代请求人、代办人和执行进程保持独立身份。'],
 [4,c('SecretsProjectionCSVLegacyAndCausality','UnavailableDatabaseHasBoundedSafeDiagnostic'),'AuditPlatform／平台；注入禁存标记','存储、三类投影、technical CSV 无禁存标记；诊断不输出连接密码。'],
 [5,c('AllowListedValuesAndMissingStates','TruncatedValuesAndBoundedCausalChain','SecretsProjectionCSVLegacyAndCausality'),'Writer／平台历史对象','null、空集合、缺失、脱敏、截断及名称来源分别断言。'],
 [6,[...c('AtomicTaskAndNotificationFailures','BusinessCommandReceiptsConflictsAndAtomicFailure'),...g('TestFailedAuditRollsBackWholePublication')],'平台导出／定义发布；写入故障','根对象、审计、任务、通知与收据没有半提交；恢复后完成一次。'],
 [7,c('BusinessCommandReceiptsConflictsAndAtomicFailure','StorageFailureSnapshotReuseRevocationAndUnknownTransfer'),'平台两名操作人；冲突／执行故障','拒绝、冲突、执行失败分开；失败事实没有伪造生效差异。'],
 [8,c('CommandReceiptsAndTombstones','BusinessCommandReceiptsConflictsAndAtomicFailure','HTTPCommandsStrictQueryAndMetadataWithoutStorage'),'真实 HTTP／平台命令','同键同载荷重放；换载荷冲突；无键 428；tombstone 不重新执行。'],
 [9,c('LostCommitAcknowledgementUsesOriginalReceipt'),'平台命令；丢失 commit 响应','新连接核对原收据；未知结果不作为失败重做业务。'],
 [10,c('BusinessCommandReceiptsConflictsAndAtomicFailure'),'两个事务／相同角色修订','一个成功、一个冲突；冲突没有已生效 before/after。'],
 [11,c('ThreeScopesAndBoundCursors'),'OnlyPlatform／双租户双项目','无空间资格无法读详情或隐藏名称；平台无完整项目事实。'],
 [12,c('MembershipRejoinNoticeAndPermissionSeparation'),'平台管理员／租户审计员','租户启停只给公开说明；技术及嵌套投影未泄漏内部原因。'],
 [13,[...c('ThreeScopesAndBoundCursors'),...g('TestProjectAuditIsolationAndTenantRoleIndependence')],'平台／租户审计／项目审计','项目完整审计独立授权；租户治理摘要不授予项目入口。'],
 [14,[...c('MetadataRedactionAndQueryFailures'),...r('TestFilesPermissionsCurrentReadBoundaryAndArchivedHost')],'审计员撤销业务元数据权限','保留最小审计摘要；业务名、说明、差异及文件正文不能被审计恢复。'],
 [15,c('MembershipRejoinNoticeAndPermissionSeparation'),'RejoinAuditor／同账号新成员关系','旧游标、委托、下载及通知链接按新关系拒绝。'],
 [16,c('MembershipRejoinNoticeAndPermissionSeparation','EmptyExportCancelRetryAndOwnerIsolation'),'read／technical／export／manage 四类账号','管理者可取消但不能下载或冒名 retry；technical 不扩展业务读取。'],
 [17,c('ThreeScopesAndBoundCursors','MetadataRedactionAndQueryFailures'),'无业务权审计员／跨空间 ID','隐藏内容搜索、错误空间与人员筛选不回显未授权数据。'],
 [18,c('ThreeScopesAndBoundCursors','MembershipRejoinNoticeAndPermissionSeparation','FrozenBackupIndependentRestore'),'跨账号／空间／权限修订／恢复 epoch','非法和过期游标拒绝；合法分页无重复。'],
 [19,[...c('MetadataRedactionAndQueryFailures','SecretsProjectionCSVLegacyAndCausality'),b],'查询空集、拒绝、来源故障、2001 候选','空集与错误分开；预算耗尽不返回部分成功；UI 故障清掉旧行。'],
 [20,c('InvitationEffectsAndLegacyUnknown','SecretsProjectionCSVLegacyAndCausality'),'旧已知／未知事件；历史名称','缺失如实标记；未知不归平台；当前名称标为 current。'],
 [21,c('SecretsProjectionCSVLegacyAndCausality','TruncatedValuesAndBoundedCausalChain'),'同请求号／同名对象／有界因果链','显式链接决定操作链；相同请求号和时间不自动关联。'],
 [22,r('TestTaskSpatialManagersRetryHistoryAndOriginalIdentity','TestFilesTaskCancellationKeepsExecutionRequesterAndRejectsOldCommands'),'来源页与统一任务命令','来源唯一控制事实；终态另记；旧命令不能重复成功。'],
 [23,[...c('WorkerKilledAndLateFenceCannotPublishTwice'),...r('TestFilesSIGKILLWorkerRecoversSameGeneration','TestTaskSpatialManagersRetryHistoryAndOriginalIdentity')],'worker 自动恢复／本人和管理员人工 retry','自动恢复保留代次；人工重试新代；旧代身份与结果保持。'],
 [24,c('EmptyExportCancelRetryAndOwnerIsolation','StorageFailureSnapshotReuseRevocationAndUnknownTransfer'),'排队／运行／已完成导出','请求接受、取消中、实际取消和终态竞争按真实状态表达。'],
 [25,[...c('MetadataRedactionAndQueryFailures'),b],'来源暂不可用后恢复','不可核实与无权分别投影；恢复后链接可重新解析。'],
 [26,evidence('metadata','TestAuditRealDisplayHandoffExhaustionAndMaintenanceReplay'),'真实 display 包＋受限处理／桥接账号','八次公共写入失败仍保留 handoff；维护重试只翻译一次，不重跑包检查。'],
 [27,g('TestIntegrationSessionRevocationAndRecovery','TestIntegrationLimitsAuditCleanupAndRollback','TestAuditNamesOutcomesAndScope','TestAuditChronologicalPagination'),'identity/access 真实事务／平台及本人','身份、权限、失败登录和本人安全摘要通过原领域回归。'],
 [28,[...g('TestProjectMembersRolesUnionHistoryAndParentCascade','TestTenantLifecycleIsolationPreservationAndPrivacy'),...evidence('repaired','TestLifecycleIsolationHistoryAndRecovery','TestProjectCatalogPublicationAuditCleanupAndPaging')],'租户／项目管理员、成员与邀请目标','治理各效果和公开投影通过；旧审计夹具已按不可变事实修正并复验。'],
 [29,[...g('TestCatalogOwnershipPublicationAndReplay','TestDeviceOutputsAndInstallationPeriods','TestSourceAtomicSwitchOccupancyAndRollback'),...r('TestResourceLifecycleAndStableHistory','TestProjectResourceExplicitGrantsContextAndCreation')],'工程／设备管理员及项目使用者','定义、台账、安装、来源绑定与项目授权均产生真实事务事实。'],
 [30,[...r('TestMonitoringCSVPublicationAndFixedDuplicateSet','TestMonitoringActualAdoptionsRevokeAndReadopt','TestMonitoringExportFixedSnapshotAndTypedParquet'),...evidence('source','TestMonitoringRouteCopyPauseTailSwitchAndReverseCopy')],'真实 PG/ClickHouse/S3／租户与项目','导入、校验、采用、撤销、导出与维护可追踪；无逐行观测审计。'],
 [31,evidence('derived','TestDerivedFormalPublicationUsesSealedAttempts','TestAnalysisAtomicMultiOutputVersionsAndChangedInputs'),'真实派生／分析账号、固定输入、受限 audit worker','试算、正式计算、回放、分析、比较与导出经过真实来源投影和受限 CSV。'],
 [32,[...evidence('source','TestSpatialLifecycleProjectDefaultsAndConcurrentDraft'),...evidence('metadata','TestDisplayLifecycleAndSubjectBoundary'),...r('TestDisplayTerminalPublicationAndRestore')],'空间／大屏管理者及项目读者','版本、发布、撤回、授权与终态通过；没有新增包正文下载。'],
 [33,r('TestFilesUploadFactsAndImmutableAttachment','TestFilesReplacementConflictReconfirmHistoricalTargetAndReads','TestFilesRegisteredCleanupRespectsHistoryReadAndBackupHold'),'租户／项目附件所有人、管理员','上传、准备、待绑定、替换、还原、回收恢复与固定修订对齐。'],
 [34,[...c('ExportSnapshotTaskNotificationAndDownload'),...evidence('repaired','TestFilesHTTPRealUploadStrictInputAndGuardedContent'),...r('TestMonitoringMultipartResumeAndOriginalDownload','TestSpatialDownloadRevocationAndBasisChange')],'真实敏感下载；GET/Range/HEAD/304','每次传输独立开始／结束；HEAD 和条件响应不冒充完整正文。'],
 [35,c('StorageFailureSnapshotReuseRevocationAndUnknownTransfer'),'未关闭 receipt；故障与过期时钟','补记 interrupted_unknown；未知字节数保留空值，不伪造成功。'],
 [36,c('SnapshotExcludesLateCommittedFact'),'导出冻结期间迟提交事务','固定 MVCC 视图排除迟提交事实，snapshot_at 和集合一致。'],
 [37,c('SecretsProjectionCSVLegacyAndCausality','TestCSVFormulaAndQuotedNewlineCells'),'中文／引号／CRLF／公式前缀／technical 档','CSV BOM、引号、换行和公式安全通过；敏感字段不旁路。'],
 [38,c('FiftyThousandRowBoundaryAndMidstreamRevoke','ConfiguredByteCeilingHasNoPartialSuccess','TestCSVExactHundredMiBBoundary','EmptyExportCancelRetryAndOwnerIsolation'),'0／50000／50001 条；100 MiB 编码边界','空集仅表头；最大行数成功；超行／超字节不产生部分成功文件。'],
 [39,c('LowerConfiguredScopeAndInstanceQuotas','EmptyExportCancelRetryAndOwnerIsolation'),'本人／空间／实例并发限额','同命令重放不重复占名额；超配额明确拒绝。'],
 [40,c('WorkerKilledAndLateFenceCannotPublishTwice','StorageFailureSnapshotReuseRevocationAndUnknownTransfer'),'真实 SIGKILL 子进程／过期租约／旧 fence','恢复同代原快照；旧进程不能重复发布，手工 retry 新代。'],
 [41,c('StorageFailureSnapshotReuseRevocationAndUnknownTransfer','ExportSnapshotTaskNotificationAndDownload'),'冻结／写入／发布期间权限和版本变化','失去当前资格的成果不发布为可下载。'],
 [42,c('ExportSnapshotTaskNotificationAndDownload','MembershipRejoinNoticeAndPermissionSeparation'),'成功导出所有人撤权后恢复','旧成果仍失效；重新申请生成新快照和授权向量。'],
 [43,[...c('FiftyThousandRowBoundaryAndMidstreamRevoke'),b],'下载中途撤权；真实 HTTP 条件读取','后续未放行块被阻止；GET/HEAD/Range/304 均重新核对所有人权限。'],
 [44,c('EmptyExportCancelRetryAndOwnerIsolation'),'所有人和 exports.manage 管理者','管理者取消成功；他人下载／retry 拒绝；本人 retry 新代。'],
 [45,c('ObjectCleanupHonorsReadLease','ExportSnapshotTaskNotificationAndDownload'),'24h 成果到期／活动读取租约','拒绝过期正文；历史成功保持；有效 read lease 阻止物理删除。'],
 [46,c('ObjectDeletionTimeoutMissingBodyAndUnknownKey','StorageFailureSnapshotReuseRevocationAndUnknownTransfer'),'S3 写后失败／删除超时／缺失与损坏对象','登记对象可对账；未知键保持；缺失正文不返回伪空 CSV。'],
 [47,[...c('ExportSnapshotTaskNotificationAndDownload'),b],'无租户成员的 audit_platform','提交、任务、CSV 全链路通过；platform 任务只登记 audit.export。'],
 [48,[...r('TestTaskMigrationWithRealRetainedAndQueuedSources','TestTaskIndependentBackupRestoreAndLegacyNotification','TestFilesTaskActorAndLegacyFingerprint'),...evidence('compatibility','TestTaskContractRejectsUnsafeFacts')],'既有任务类型和 files.prepare','旧序列化／指纹／命令收据／固定代次／索引恢复回归通过。'],
 [49,c('ExportSnapshotTaskNotificationAndDownload','AtomicTaskAndNotificationFailures'),'导出所有人／重试新代／worker','同代一条规定终态通知；接收人为原相关人员，无管理员广播。'],
 [50,c('AtomicTaskAndNotificationFailures'),'通知停机、偏好关闭、恢复 worker','业务提交和任务终态独立；投递恢复不重复生成事实。'],
 [51,[...c('AtomicTaskAndNotificationFailures'),...evidence('compatibility','TestExpiredClaimFencingPoisonIsolationAndReplay')],'待投递／抑制／失败投递','仅公开受控摘要；无他人正文、偏好、已读或归档数据。'],
 [52,[...c('MembershipRejoinNoticeAndPermissionSeparation','ExportSnapshotTaskNotificationAndDownload'),b],'固定 generation 的通知／任务／文件入口','跳转固定代次；过期和撤权不能绕过源授权。'],
 [53,[...c('RetentionBackfillProtectedCleanupAndRoles'),...g('TestIntegrationLimitsAuditCleanupAndRollback')],'旧保留配置／身份维护／项目历史','实际旧保留值保留；身份清理不再删审计；项目不新增自动 90 天删除。'],
 [54,c('RetentionBackfillProtectedCleanupAndRoles','ObjectCleanupHonorsReadLease'),'plan/apply 之间新增 hold 或 read lease','执行时再次核对保护；无任意 ID 删除或 force 入口。'],
 [55,c('FrozenBackupIndependentRestore','RestoreFencesWithoutReissuingNotifications'),'真实 pg_dump/独立数据库与 S3 恢复','新 epoch 使旧游标、fence、read lease 失效；中断留痕、不补发旧通知。'],
 [56,c('RetentionBackfillProtectedCleanupAndRoles'),'旧已知／未知事件，重复回填','原 ID、时间、原因、details 保持；缺失关联不猜测；未分类计数可查。'],
 [57,c('HTTPCommandsStrictQueryAndMetadataWithoutStorage','SecretsProjectionCSVLegacyAndCausality','ThreeScopesAndBoundCursors'),'新旧三空间 API／本人安全页','所有路径统一投影；旧 details 无原样旁路，本人摘要保持。'],
 [58,c('NamedRolesRunAndDenyUnrelatedData'),'observer／maintenance／content／restore SQL 角色','观察、apply、CSV 内容凭据分离；普通人员无本地数据库维护权。'],
 [59,[b],'浏览器 owner／platform／tenant／project','三空间列表、差异、名称来源、导出及返回通过真实页面检查。'],
 [60,[h,b],'全部 §15.2 对象／两种业务账号／无审计账号','每类来源历史链接真实可达，固定对象／修订可返回；缺审计权不阻断业务。'],
 [61,[b],'1440px／375px／200%／纯键盘','详情可开关并归还焦点；无水平溢出，状态有文字，截图人工复核。'],
 [62,c('HTTPCommandsStrictQueryAndMetadataWithoutStorage','AtomicTaskAndNotificationFailures'),'无 S3/checker/ClickHouse 装配；通知停机','审计元数据仍启动和查询；依赖动作按明确不可用处理。'],
 [63,[contract,...evidence('metadata','TestAuditRealDisplayHandoffExhaustionAndMaintenanceReplay'),...evidence('derived','TestAnalysisAtomicMultiOutputVersionsAndChangedInputs')],'AM-01–14 源码扫描及真实生产路径','事件目录列出生产函数、事务、字段与关联；未登记新事件检查失败。'],
 [64,[...c('NamedRolesRunAndDenyUnrelatedData'),...r('TestDisplayPackageWorkerLeastPrivilege','TestFilesWorkerObserverAndBackupDatabaseRoles'),...evidence('derived','TestDerivedFormalPublicationUsesSealedAttempts')],'SQL 受限 worker／维护／观察账号','实际受限任务可运行；历史 UPDATE/DELETE、凭据、业务正文列读取拒绝。'],
 [65,c('SourceIdentityAtomicityAndBounds','TruncatedValuesAndBoundedCausalChain'),'超限字段／关联、恶意 request_id／循环因果','拒绝非法类型与循环；长值按明确规则截断，有界遍历。'],
 [66,c('UnavailableDatabaseHasBoundedSafeDiagnostic'),'断开 PostgreSQL；合成连接密码标记','业务返回失败；诊断含排障号、无密码且时间有界。'],
 [67,[...c('MembershipRejoinNoticeAndPermissionSeparation'),b],'归档项目、禁用租户、失效成员、ReadSubject','归档资格保留；禁用／失效／机器 ReadSubject 不获得审计入口。'],
 [68,[...evidence('compatibility','TestOptionalEventActorPreservesLegacyFingerprint','TestUpgradeFrom35AndIndependentNotificationRestore','TestTransactionalDeliveryWatermarkIsolationAndPreferences'),...r('TestTaskIndependentBackupRestoreAndLegacyNotification')],'原通知接收人、历史已读／归档和旧 source_key','原指纹与私人收件箱状态保持，恢复／回填不补发。'],
 [69,[b],'账号／空间切换；迟到响应／blob URL','中止旧请求、清空旧详情并回收 URL；无跨空间 CSV 缓存。'],
 [70,[deliver,...c('HTTPCommandsStrictQueryAndMetadataWithoutStorage')],'完整本地构建、合成演示、恢复与维护命令','OpenAPI／TS／权限一致；运行手册、真实样本和维护入口齐备。'],
];
const read=p=>existsSync(root+p)?readFileSync(root+p,'utf8'):'';
const passed=e=>e.json?(()=>{try{return JSON.parse(read(e.json)).passed===true;}catch{return false;}})():new RegExp('^--- PASS: '+e.test+' \\(','m').test(read(e.log));
const plan=readFileSync('docs/requirements/audit-and-operation-tracing-development-plan.md','utf8');
const requirements=new Map([...plan.matchAll(/^\| (AT-\d+) \| ([^|]+) \| ([^|]+) \|$/gm)].map(m=>[m[1],{scope:m[2].trim(),criterion:m[3].trim()}]));
if(rows.length!==70||requirements.size!==70)throw Error('Expected exactly 70 distinct acceptance requirements');
const results=rows.map(([n,proof,input,result])=>({id:'AT-'+String(n).padStart(2,'0'),...requirements.get('AT-'+String(n).padStart(2,'0')),input,result,proof,passed:proof.every(passed)}));
const complete=results.every(r=>r.passed);
let doc='# 审计与操作追踪自验记录\n\n';
doc+=`日期：2026-11-14。软件状态：**${complete?'Awaiting acceptance':'In progress'}**。逐项软件自验：**${results.filter(r=>r.passed).length}/70**；人工验收：**未进行，待用户确认**。\n\n`;
doc+='依据[详细方案](../requirements/audit-and-operation-tracing-development-plan.md)，在当前 main checkout 实施 AU-01–08、AM-01–14、AD-01–12。未创建分支或 worktree；原在途任务、文件、通知、空间与大屏改动保留，基线见 `.artifacts/audit/baseline-status.txt`。无生产部署、生产迁移、硬件接入或长时压力测试结论。\n\n';
doc+='## 环境、命令与证据解释\n\n真实 PostgreSQL 隔离 schema／独立恢复数据库、Redis、ClickHouse 与 SeaweedFS S3；真实公开业务接口、事务、受限 SQL 角色、独立 worker 与浏览器。所有账号、数据和附件为合成样本。演示入口与账号路径见[人工验收手册](../operations/audit-and-operation-tracing-acceptance.md)，运维见[运行手册](../operations/audit-and-operation-tracing.md)，生产路径见[事件目录](../development/audit-event-catalog.md)。\n\n';
doc+='表中的 `Test...` 是日志中实际出现 `PASS` 的用例，不以“存在测试函数”作证。组合回归保留最初失败日志：governance 的三项旧夹具／环境问题经 repaired 复验；resources 的历史删除、监测维护 retry、空间生命周期三项经 resource-repair/source-repair 复验。metadata 的派生受限角色缺项与 NULL 运行标识经 derived-metadata 复验。初始日志中的失败不会被改写，修复结果单独列出。\n\n';
doc+='resources 中三个专用恢复／旧版本用例跳过不算通过：文件恢复另执行 files-restore，监测恢复和旧版本升级由 `pnpm test:monitoring:restore` 及 `--legacy` 单独执行。核心审计命令拒绝关键测试 skip；其余进程 helper／专用 E2E server 是启动入口，不是跳过的业务验收。\n\n';
doc+='浏览器按 §15.2 执行 36 条真实来源往返，包括设施授权详情及两空间附件的两个修订。首轮发现并修复平台租户路由、监测任务历史筛选、资源授权集合筛选与详情入口、空间处理完成的迟到导航及固定版本深链接；最终完整结果见 history-browser/result.json。每条断言实际对象／关联 ID，并点击获准来源返回；任务还核对固定 execution，附件核对固定 revision。\n\n';
doc+='## 70 项逐项核对\n\n| 编号／范围 | 验收要求 | 输入账号／空间／故障 | 实际结果 | 命令／用例及证据 | 自验 |\n| --- | --- | --- | --- | --- | --- |\n';
for(const r of results){const proof=r.proof.map(e=>e.json?`\`${e.command}\` · [结果](../../.artifacts/audit/${e.json})`:`\`${e.command}\` · \`${e.test}\` · [日志](../../.artifacts/audit/${e.log})`).join('<br>');doc+=`| ${r.id} · ${r.scope} | ${r.criterion} | ${r.input} | ${r.passed?r.result:'仍需完成本行全部证据；不宣称通过。'} | ${proof} | ${r.passed?'通过':'待完成'} |\n`;}
doc+='\n## 交付与人工接受\n\n新迁移为 41–43；保留策略须按实际旧配置显式 plan/apply，默认 plan_only。恢复先协调维护状态、epoch 和未完成代次再开放服务。普通控制台没有永久删除审计的按钮。\n\n本地软件自验完成不等于人工接受。人工逐项操作按验收手册核对，用户明确确认后才可改为 Accepted。没有通过的行必须继续处理，不以全量构建或其他领域通过替代。\n';
writeFileSync('docs/verification/audit-and-operation-tracing.md',doc);
writeFileSync(root+'acceptance-70.json',JSON.stringify({passed:complete,completedAt:new Date().toISOString(),manualAcceptance:'pending',rows:results},null,2));
console.log(`${results.filter(r=>r.passed).length}/70 software acceptance rows have all recorded evidence.`);
if(process.argv.includes('--require-complete')&&!complete)throw Error('Incomplete: '+results.filter(r=>!r.passed).map(r=>r.id).join(', '));
