import {readFileSync,writeFileSync} from 'node:fs';
// Run audit:contracts first. This inventory is source-derived; observation counts
// are deliberately separate from registration and from manual acceptance.
const catalog=JSON.parse(readFileSync('.artifacts/audit/catalog.json'));
const producers=JSON.parse(readFileSync('.artifacts/audit/producers.json'));
const groups=[
 ['AM-01','identity','当前契约','Repository 的账号、会话、密码与邀请事务；失败登录按真实 outcome；本人安全摘要仍独立裁剪。',null,null],
 ['AM-02','access','当前契约','角色／绑定／目录升级的实际事务出口；同一事务的不同效果独立保存，目录重放不重复变更。',null,null],
 ['AM-03','tenancy','当前契约','租户资料、成员、邀请、管理员交接；平台内部原因与 tenant_notice 分别授权。',null,null],
 ['AM-04','projects','当前契约','项目资料、成员、角色、管理员和归档；另写独立 tenant.project.governance 摘要。',null,null],
 ['AM-05','engineering,resources,devicecatalog,devices,sources','当前契约','定义、资源与设备／输出／安装、来源绑定、授权集合；快照由业务写锁内读取；链接保留父设备／设施等明确依据。',null,null],
 ['AM-06','monitoring','当前契约','原命令收据出口、采用事务、任务终态、导出和受控传输、数据授权、路由／保留／恢复维护。',null,null],
 ['AM-07','derived','当前契约','配置／通道／运行／分析的原事务出口；正式采用、发布、固定输入和成果关联；任务事实区分原发起人和本代请求人。',null,null],
 ['AM-08','spatial','当前契约','空间命令快照、处理终态、源文件传输、完整性维护；删除草稿保留处理任务和代次证据。',null,null],
 ['AM-09','display','当前契约','管理命令直接写公共事实；受限包处理事务仅写 display_audit_handoffs，BridgeAuditTx 核对来源后幂等翻译。',null,null],
 ['AM-10','files','当前契约','文件上传和附件事务，准备任务终态，修订关联，受控传输，清理／备份／恢复；分片不逐条写审计。',null,null],
 ['AM-11','tasks','当前契约','来源控制事实只在来源命令出口写；taskrecord.auditTransitionTx 仅写首次接受及真实终态；索引重放不追加。',null,null],
 ['AM-12','notifications','当前契约','原通知 source_key、指纹和接收人不变；保存 notification_event 显式关联，投递摘要只给状态与技术计数。维护重放有独立事实。',null,null],
 ['AM-13','audit','当前契约','统一 Writer、Reader、命令收据、快照导出／代次／受控下载；清理和恢复过程也写事实。普通浏览不递归写入。',null,null],
 ['AM-15','parsing','当前契约','目录同步、不可变配置、显式采用、样本与原响应读取、试解析提交和来源控制；原始字节与测量值不进入审计。',null,null],
 ['AM-14','runtime,config,migrations,CLI','当前契约','有限适配器、SQL 角色、协议／保留策略、worker、显式维护计划、恢复工具、静态目录及真实样本。',null,null],
];
const groupOf=m=>groups.find(g=>g[1].split(',').includes(m))??groups.at(-1);
const dynamic={parsing:['complete'],identity:['audit','ConfirmPassword','SetStatus'],access:['ChangeRole','SetAccountStatus'],tenancy:['ChangeStatus','ChangeMember','CreateTenantRole','ChangeTenantAuthorization','InvalidateTenantInvitations','AuditTenantAvailability'],projects:['complete','AuditProjectAvailability'],engineering:['complete'],resources:['recordChange','recordAccessChange'],devicecatalog:['complete'],devices:['complete','ChangeGrants'],sources:['Mutate'],monitoring:['auditCommandTx','bumpDataAccess','operatorAuditTx'],derived:['complete','analysisCompleteTx','configOperationTx','recordResourceControlTx','ControlRun'],spatial:['complete'],display:['Command'],files:['audit','maintenanceAuditTx'],audit:['exportFactTx','maintenanceFactTx','recordFailure'],notifications:['Replay','ReplayDisplayNotification'],tasks:['TaskControl']};
function owners(d){
 const event=d.event_type;
 if(event.includes('.download.'))return producers.filter(p=>p.kind==='producer'&&p.path.endsWith('/auditrecord/transfers.go'));
 if(event.includes('.execution.'))return producers.filter(p=>p.kind==='producer'&&p.function===(d.module==='display'?'BridgeAuditTx':'auditTransitionTx'));
 const exact=producers.filter(p=>p.kind==='producer'&&p.literal===event);if(exact.length)return exact;
 return producers.filter(p=>p.kind==='producer'&&!p.literal&&(dynamic[d.module]??[]).includes(p.function)&&((['tenancy','projects'].includes(d.module)&&p.path.includes('/access/'))||p.path.includes('/'+({derived:'derivedmonitoring',tasks:'monitoring'}[d.module]??d.module)+'/')||d.module==='audit'&&p.path.includes('/access/')));
}
const link=p=>`[${p.path.replace('packages/go/platform/','')} · ${p.function}](../../${p.path}#L${p.line})`;
const escape=s=>String(s).replaceAll('|','\\|').replaceAll('\n',' ');
const outcomes=d=>d.phase==='transfer'?'授权 success；结束按 sent/partial/aborted/failed/not_modified/head；未关闭 unknown':d.event_type.endsWith('.failed')||d.event_type.endsWith('.needs_action')?'failure':d.event_type.endsWith('.cancelled')?'cancelled':d.event_type==='login'?'success / failure / rate_limited':d.event_type.startsWith('audit.command.')?d.event_type.split('.').at(-1):'success（拒绝／冲突由命令失败出口独立记录）';
const relation=d=>d.phase==='transfer'?'transfer receipt → 固定 file/revision/generation':d.event_type.includes('.execution.')?'固定 task_execution；原始请求人与执行身份；完成时的明确结果':d.module==='audit'?'export/root + generation + task + notification_event；maintenance plan/hold':d.module==='files'?'upload/job/generation + attachment/revision（仅有实际依据时）':d.module==='display'?'instance + upload/job/generation；成功包的 package 引用':d.module==='spatial'?'asset/version + job/generation + 原件引用':d.module==='derived'?'channel/config/run + 固定 generation/artifact + 任务／结果':d.module==='monitoring'?'batch/job + 固定成果；维护目标 ID':'业务对象／固定修订；同事务 operation_id；明确产生的 notification_event';
let out='# 审计事件目录与生产路径账本\n\n';
out+='由 `node scripts/audit-event-catalog.mjs` 根据 `audit:contracts` 的有限注册表和 AST 生产路径生成。注册表是允许写入的词汇，不表示每个词条都已在本次测试触发。本机操作及人工接受以 [环境说明](../operations/local-environment.md) 为准。\n\n';
out+=`当前登记 ${catalog.length} 个事件词条，扫描 ${producers.length} 个生产／域内记录位置。目录描述当前源码的披露契约，不保存某次执行结果。\n\n`;
out+='## 公共字段、可见性与事务规则\n\n';
out+='所有事实保留服务端事件 ID、来源幂等键／效果、真实空间、动作／结果、发生与入库时间、operation_id、原发起人／本代请求人／执行身份、对象与修订、有限变化和显式关联。只有白名单字段可以进入快照；缺失、null、空集合、脱敏、截断分别表达。凭据、带凭据 URL、会话、文件正文、监测值与任意日志不进入事实。超过旧业务原因上限的审计摘要明确标记截断与摘要哈希，原业务收据协议不变。\n\n';
out+='`platform` 仅平台事实；`platform_tenant` 要求平台租户治理读取；`notice` 在租户空间只投影公开说明；`tenant`／`project` 精确成员资格；`governance` 为独立租户项目治理摘要；`entry`／`business` 使用实际来源归属，不能由前端指定。业务字段另外经过所属 Module 的当前元数据权限核对。technical 权限不增加业务可见性。`business_or_platform` 的平台任务仅允许 audit.export 和 parsing.test。\n\n';
out+='生产函数拥有业务事务；auditrecord 只加入该事务。任务、通知持久化失败与业务成功一起回滚。失败事实在已确认业务回滚后使用独立有界事务；提交未知先查原收据，不伪造失败。大屏受限处理以同事务 handoff 为来源事实，公共桥接不重做包处理。传输每个 GET/Range/HEAD/304 各有独立 receipt，开始和结束共享显式操作；超时未知不补造字节数。\n\n';
out+='## 模块与事务拥有者\n\n| 组 | 模块 | 实际行为／事务拥有者 |\n| --- | --- | --- |\n';
for(const g of groups)out+=`| ${g[0]} | ${g[1]} | ${g[3]} |\n`;
out+='\n## 不另写公共成功事实的路径\n\n- 普通列表／详情／搜索／筛选快照／续期、前端轮询、上传分片、心跳和索引重放：不记录，避免递归与噪声；敏感正文传输单独记录。\n- tasks 控制 Adapter：保持来源委托，公共成功只由原来源命令出口写；接受取消不等于已取消。\n- notifications 投递尝试、偏好、已读与归档：保持原投递事实，不把私人收件箱状态复制成业务审计。\n- display 订阅游标和遥测：保持域内运行记录，不将诊断日志升格为业务审计。包工作状态使用持久 handoff。\n- *_changes 与 *_events：保留原领域历史／发布依据。下表列出每个直接 INSERT 位置，只有明确公共 Writer／桥接生产事实；不通过扫描记录名猜测跨空间关联。\n- 目录中的旧动作别名和来源状态词汇保留有限兼容验证；无现存副作用路径的词条不作为新增功能或已执行证据。\n\n';
out+='## 每个事件的披露契约\n\n字段组按实际白名单内容去重编号，各组的完整字段在本节后列出。表内生产函数若为共享出口，所有该动作的实际调用均在业务事务内经过该出口。\n\n| 事件／动作 | 生产函数 | 可见性／投影适配器 | 结果 | 字段／关联 | 所属模块组 |\n| --- | --- | --- | --- | --- | --- |\n';
const fields=new Map();let next=0;
for(const d of catalog){const key=JSON.stringify(d.fields);if(!fields.has(key))fields.set(key,{id:['G','B','M','T'][next++]??`F${next}`,values:d.fields});const f=fields.get(key).id,g=groupOf(d.module),own=owners(d);out+=`| \`${d.event_type}\` · ${d.label} | ${own.map(link).join('<br>')||'有限状态／兼容词汇；无独立生产函数'} | ${d.scope_rule} / ${d.metadata} | ${outcomes(d)} | ${f}；${relation(d)} | ${g[0]} |\n`;}
for(const f of fields.values())out+=`\n### 字段组 ${f.id}\n\n${f.values.map(v=>'`'+v+'`').join('、')}。\n`;
out+='\n## 全部生产与直接写入位置\n\n扫描包括限定／同包 access.Audit、identity audit、devicekit.Audit/AuditSnapshots、Fact/LegacyInput 以及直接 INSERT 的 audit/events/changes/handoffs。静态字符串未登记即失败；动态动作仍由运行时有限目录拒绝。\n\n| 位置／函数 | 类型与表达式 | 处理规则 |\n| --- | --- | --- |\n';
for(const p of producers){let rule=p.kind==='producer'?'公共 Writer；实际副作用拥有事务，重放保持来源幂等键':'保持域内历史／状态；公共事件由同事务出口或持久桥接写入';if(/subscriptions|telemetry/.test(p.path))rule='不另记公共事件：运行游标／诊断，不是业务事实';if(p.path.endsWith('/auditrecord/writer.go'))rule='唯一事实持久化出口；触发器与 SQL 角色共同限制历史改写';out+=`| ${link(p)} | ${p.kind}：\`${escape(p.expression)}\` | ${rule} |\n`;}
writeFileSync('docs/development/audit-event-catalog.md',out);
console.log(`Audit event catalog: ${catalog.length} contracts, ${producers.length} source sites.`);
