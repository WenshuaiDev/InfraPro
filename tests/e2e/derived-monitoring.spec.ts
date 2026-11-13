import { mkdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { resolve } from 'node:path';
import { createHash, randomUUID } from 'node:crypto';
import type { APIRequestContext, Locator, Page } from '@playwright/test';
import type { MonitoringImportBatch, MonitoringMapping } from '../../packages/ts/api-client/src/monitoring';
import type { DerivedRule, DerivedRun } from '../../packages/ts/api-client/src/derived-monitoring';
import { test, expect, consoleURL, csrf, login } from './fixtures';
import { seedResources } from '../helpers/resource-fixture';

const origin=new URL(consoleURL).origin, api=`${origin}/api/v1`;
const wm=(revision='0')=>({request_id:randomUUID(),expected_revision:revision,reason:'派生监测真实浏览器自验'});
const start='2020-01-01T00:00:00.000000001Z',end='2020-01-01T00:03:00Z';
const longName='设施位移相邻变化与纳秒精确来源核验_abcdefghijklmnopqrstuvwxyz_ABCDEFGHIJKLMNOPQRSTUVWXYZ_0123456789';
test.use({actionTimeout:15000});
test.skip(process.env.INFRAPRO_MONITORING_E2E!=='1','requires the disposable PG, ClickHouse and S3 lab');

async function prepare(page:Page,request:APIRequestContext) {
 const f=await seedResources(request);await login(page,f.user);
 const headers={Origin:origin,'X-CSRF-Token':await csrf(page.request,origin)};
 const post=async(path:string,data:object,status:number|number[]=201)=>{const r=await page.request.post(api+path,{headers,data});expect(Array.isArray(status)?status:[status],await r.text()).toContain(r.status());return r.json();};
 const base=`/tenants/${f.tenant}`,monitoring=`${base}/monitoring`;
 const facility=await post(`${base}/engineering/resources/facilities`,{request_id:randomUUID(),code:'BRIDGE',name:'自验桥梁',type_version_id:f.type.version_id,values:{length:0}});
 const point=await post(`${base}/engineering/resources/facilities/${facility.id}/points`,{request_id:randomUUID(),code:'P1',name:'跨中测点'});
 const channel=await post(`${base}/engineering/resources/facilities/${facility.id}/points/${point.id}/channels`,{request_id:randomUUID(),code:'DISP',name:'原始位移',metric_version_id:f.metric.version_id,unit_version_id:f.unit.version_id,timestamp_meaning:'observation_time'});
 const csv=`observed_at,value,quality\n${start},10,good\n2020-01-01T00:01:00.000000001Z,12,good\n2020-01-01T00:02:00.000000001Z,15,good\n`,sha256=createHash('sha256').update(csv).digest('hex');
 let batch=await post(`${monitoring}/imports`,{...wm(),name:'derived-ui.csv',size_bytes:String(Buffer.byteLength(csv))}) as MonitoringImportBatch;
 const part=await page.request.put(`${api}${monitoring}/imports/${batch.id}/upload/parts/1`,{headers:{...headers,'Content-Type':'application/octet-stream','X-Expected-Revision':batch.revision,'X-Content-SHA256':sha256},data:Buffer.from(csv)});expect(part.status(),await part.text()).toBe(200);
 batch=await post(`${monitoring}/imports/${batch.id}/upload/complete`,{...wm(batch.revision),sha256},200);
 const mapping:MonitoringMapping={format:'csv',mode:'single',encoding:'utf-8',delimiter:',',header_row:1,time_column:'observed_at',value_column:'value',quality_column:'quality',timezone:'UTC',empty_wide:'error',channels:[{alias:'C',channel_id:channel.id,unit:'mm'}]};
 batch=await post(`${monitoring}/imports/${batch.id}/mappings`,{...wm(batch.revision),mapping},200);
 await post(`${monitoring}/imports/${batch.id}/validate`,wm(batch.revision),202);
 await expect.poll(async()=>{batch=await(await page.request.get(`${api}${monitoring}/imports/${batch.id}`)).json();return batch.state;},{timeout:30000}).toBe('ready');
 await post(`${monitoring}/imports/${batch.id}/publish`,wm(batch.revision),202);
 await expect.poll(async()=>{batch=await(await page.request.get(`${api}${monitoring}/imports/${batch.id}`)).json();return batch.state;},{timeout:30000}).toBe('published');
 return {...f,headers,post,base,monitoring,facility,point,channel,root:`${consoleURL}/t/${f.tenant}/monitoring`};
}
function section(page:Page,title:string) {return page.locator('section').filter({has:page.getByRole('heading',{name:title,exact:true})});}
async function range(area:Locator) {await area.getByLabel('起点（含）',{exact:true}).fill(start);await area.getByLabel('终点（不含）',{exact:true}).fill(end);}
async function configureRule(page:Page,f:Awaited<ReturnType<typeof prepare>>) {
 await page.getByLabel('物理量',{exact:true}).selectOption('unit');
 await page.getByLabel('固定单位版本搜索',{exact:true}).fill(`mm_${f.suffix}`);
 await page.getByLabel('固定单位版本定义',{exact:true}).selectOption(f.unit.definition_id);
 await page.getByLabel('固定单位版本版本',{exact:true}).selectOption(f.unit.version_id);
 await page.getByLabel('量值尺度',{exact:true}).selectOption('ratio');
 await page.getByLabel('允许来源',{exact:true}).selectOption('direct');
 await page.getByLabel('常用规则模板',{exact:true}).selectOption('difference');
 await page.getByRole('button',{name:'套用模板并替换步骤',exact:true}).click();
 await expect(page.getByLabel('计算方法',{exact:true})).toHaveValue('difference');
 await expect(page.getByRole('button',{name:'套用模板并替换步骤',exact:true})).toBeEnabled();
}
async function noOverflow(page:Page) {expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);}

test('definition, facility-owned binding, trial, automatic publication and return to a run use real services',async({page,request},info)=>{
 test.setTimeout(240000);const f=await prepare(page,request);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(`${f.root}/derived-definitions?id=new`);
 await page.getByLabel('指标编码',{exact:true}).fill('DIFFERENCE');await page.getByLabel('指标名称',{exact:true}).fill(longName);
 await page.getByRole('navigation',{name:'派生监测与分析'}).getByRole('link',{name:'派生通道',exact:true}).click();await expect(page.getByRole('alertdialog')).toBeVisible();await page.getByRole('button',{name:'继续编辑',exact:true}).click();await expect(page.getByLabel('指标名称',{exact:true})).toHaveValue(longName);
 await configureRule(page,f);
 await page.getByLabel('操作原因',{exact:true}).fill('模板生成相邻差分指标');
 await page.getByLabel('指标编码',{exact:true}).fill('bad code');await page.getByRole('button',{name:'创建草稿',exact:true}).click();await expect(page.getByRole('alert')).toBeFocused();await page.getByRole('alert').getByRole('button').first().click();await expect(page.getByLabel('指标编码',{exact:true})).toBeFocused();await page.getByLabel('指标编码',{exact:true}).fill('DIFFERENCE');
 await page.getByRole('button',{name:'创建草稿',exact:true}).click();
 await expect(page.getByRole('button',{name:'校验已保存草稿',exact:true})).toBeEnabled();
 await page.getByRole('button',{name:'校验已保存草稿',exact:true}).click();
 await expect(page.getByText('校验通过，可发布当前已保存草稿',{exact:true})).toBeVisible();
 await page.getByRole('button',{name:'发布固定版本',exact:true}).click();
 await expect(page.getByText('已发布固定版本 V1。',{exact:true})).toBeVisible();
 const definition=new URL(page.url()).searchParams.get('id')!;await noOverflow(page);
 await page.emulateMedia({colorScheme:'dark'});await page.screenshot({path:info.outputPath('definition-dark.png'),fullPage:true});
 await page.goto(`${consoleURL}/t/${f.tenant}/resources/facilities/${f.facility.id}`);
 await page.getByRole('link',{name:'新建直属派生通道',exact:true}).click();
 await expect(page.getByLabel('归属类型',{exact:true})).toHaveValue('facility');
 await page.getByLabel('通道编码',{exact:true}).fill('CALC');await page.getByLabel('通道名称',{exact:true}).fill(longName);
 await page.getByLabel('指标定义',{exact:true}).selectOption(definition);
 await page.getByRole('button',{name:new RegExp(`^V1 · ${longName}`)}).click();
 await page.getByRole('button',{name:/跨中测点 · 原始位移 · DISP/}).click();
 await page.getByLabel('操作原因',{exact:true}).fill('设施直属差分持续监测');
 await page.getByRole('button',{name:'创建通道并进入试算',exact:true}).click();
 const trial=section(page,'试算固定配置');await expect(trial).toBeVisible();await range(trial);await trial.getByLabel('操作原因',{exact:true}).fill('检查首个空值及相邻差值');
 await trial.getByRole('button',{name:'对完整范围试算',exact:true}).click();
 await expect(trial.getByText('已完成',{exact:true})).toBeVisible({timeout:30000});await noOverflow(page);
 const control=section(page,'启用持续计算');await control.getByLabel('持续生效，不设终点',{exact:true}).uncheck();await range(control);
 await control.getByRole('button',{name:'预览启用影响',exact:true}).click();
 await expect(control.getByRole('button',{name:'确认执行此预览',exact:true})).toBeVisible();
 await control.getByRole('button',{name:'确认执行此预览',exact:true}).click();
 await expect(page.getByRole('heading',{name:'暂停持续计算',exact:true})).toBeVisible();
 const channelID=new URL(page.url()).pathname.split('/').at(-1)!;
 await expect.poll(async()=>{const response=await page.request.get(`${api}${f.monitoring}/derived/channels/${channelID}/generations?from_ns=1577836800000000001&to_ns=1577836980000000000`);if(!response.ok())return 0;return(await response.json()).items.length;},{timeout:30000}).toBeGreaterThan(0);
 await page.getByRole('navigation',{name:'派生监测与分析'}).getByRole('link',{name:'计算运行',exact:true}).click();
 await expect(page.getByRole('table',{name:'计算运行记录'})).toBeVisible();
 await page.getByRole('button',{name:/试算/}).first().click();
 await expect(page.getByText('已完成',{exact:true}).last()).toBeVisible();await noOverflow(page);
 await page.screenshot({path:info.outputPath('calculation-return.png'),fullPage:true});
 // A fresh document has no monitoring revision yet. Opening history immediately must survive its first authority response.
 await page.goto(`${f.root}/derived/${channelID}`);await page.getByRole('button',{name:'历史与来源',exact:true}).click();const publishedHistory=section(page,'发布历史与原依据');await range(publishedHistory);await publishedHistory.getByRole('button',{name:'查询此范围的发布历史',exact:true}).click();await expect(publishedHistory.getByRole('table',{name:'派生发布代次'}).getByRole('row')).toHaveCount(2);await publishedHistory.getByRole('table',{name:'派生发布代次'}).getByRole('button').first().click();
 const fullEvidence=section(page,'导出此范围的完整输入依据');await fullEvidence.getByLabel('输入依据导出原因',{exact:true}).fill('核对原始候选身份与质量');await fullEvidence.getByRole('button',{name:'后台生成完整输入依据',exact:true}).click();await expect(fullEvidence.getByRole('link',{name:/\.csv$/}).first()).toBeVisible({timeout:30000});const evidenceDownloadPromise=page.waitForEvent('download');await fullEvidence.getByRole('link',{name:/\.csv$/}).first().click();const evidenceDownload=await evidenceDownloadPromise;expect(await evidenceDownload.failure()).toBeNull();const evidenceText=readFileSync((await evidenceDownload.path())!,'utf8');expect(evidenceText).toContain('input_snapshot_id');expect(evidenceText).toContain('adoption_id');expect(evidenceText).toContain('10.000000000000');await fullEvidence.getByRole('link',{name:'在运行记录中保存此导出入口',exact:true}).click();await expect(page.getByRole('heading',{name:'派生历史输入依据导出',exact:true})).toBeVisible();await expect(page.getByRole('link',{name:/\.csv$/}).first()).toBeVisible();await page.goto(`${f.root}/derived/${channelID}`);
 await page.getByRole('button',{name:'退役通道',exact:true}).click();await page.getByRole('dialog').getByLabel('原因 *',{exact:true}).fill('保留历史，停止后续自动计算');await page.getByRole('dialog').getByRole('button',{name:'预览退役影响',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'确认保存',exact:true}).click();await expect(page.getByRole('button',{name:'恢复通道',exact:true})).toBeVisible();
 await page.getByRole('button',{name:'恢复通道',exact:true}).click();await page.getByRole('dialog').getByLabel('原因 *',{exact:true}).fill('恢复资源后仍保持计算暂停');await page.getByRole('dialog').getByRole('button',{name:'预览恢复影响',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'确认保存',exact:true}).click();await expect(page.getByRole('heading',{name:'恢复持续计算',exact:true})).toBeVisible();expect(errors).toEqual([]);
});

test('analysis multi-output version, result evidence, comparison and exact export survive navigation',async({page,request},info)=>{
 test.setTimeout(240000);const f=await prepare(page,request);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(`${f.root}/analysis?id=new`);await page.getByLabel('分析编码',{exact:true}).fill('ANALYSIS');await page.getByLabel('分析名称',{exact:true}).fill('位移差分与复核输出');await configureRule(page,f);
 await page.getByRole('button',{name:'添加分析输出',exact:true}).click();await page.getByLabel('输出名称',{exact:true}).nth(1).fill('复核输出');
 await page.getByRole('button',{name:/跨中测点 · 原始位移 · DISP/}).click();
 await page.getByLabel('操作原因',{exact:true}).fill('建立多输出历史分析');await page.getByRole('button',{name:'保存分析草稿',exact:true}).click();
 await page.getByRole('button',{name:'校验已保存草稿',exact:true}).click();await expect(page.getByText('校验通过，可生成固定执行版本',{exact:true})).toBeVisible();
 await page.getByLabel('操作原因',{exact:true}).fill('核对后固定本次分析规则');await page.getByRole('button',{name:'保存固定执行版本',exact:true}).click();
 const execution=section(page,'运行固定配置 V1');await range(execution);await execution.getByLabel('运行原因',{exact:true}).fill('固定原始快照，发布两项输出');
 await execution.getByRole('button',{name:'运行并生成完整成果',exact:true}).click();await expect(execution.getByRole('link',{name:'查看本次完整成果',exact:true})).toBeVisible({timeout:30000});
 await execution.getByRole('link',{name:'查看本次完整成果',exact:true}).click();
 await expect(page.getByRole('heading',{name:'固定成果明细',exact:true})).toBeVisible();await expect(page.getByRole('table',{name:'计算输出与匹配解释'})).toContainText('2');
 await page.getByRole('button',{name:'查看原固定来源',exact:true}).click();await expect(page.getByText(/原始快照：/)).toBeVisible();
 await page.getByRole('button',{name:'与另一成果版本比较',exact:true}).click();
 const compare=section(page,'比较固定成果：左侧 V1');await compare.getByLabel('右侧固定成果版本',{exact:true}).selectOption({index:1});await compare.getByLabel('比较原因',{exact:true}).fill('同版本逐项比较应一致');
 await compare.getByRole('button',{name:'提交完整版本比较',exact:true}).click();await expect(compare.getByText('已完成',{exact:true})).toBeVisible({timeout:30000});
 await page.getByLabel('导出或任务操作原因',{exact:true}).fill('下载固定成果精确值与来源');await page.getByRole('button',{name:'生成完整导出',exact:true}).click();
 await expect(page.getByRole('link',{name:/\.csv$/}).first()).toBeVisible({timeout:30000});
 const downloadPromise=page.waitForEvent('download');await page.getByRole('link',{name:/\.csv$/}).first().click();const download=await downloadPromise;expect(await download.failure()).toBeNull();const downloaded=readFileSync((await download.path())!,'utf8');expect(downloaded).toContain('artifact_version_id');expect(downloaded).toContain('half_even');expect(downloaded).toContain('2.000000000000000000');
 await page.getByLabel('导出内容',{exact:true}).selectOption('inputs');await page.getByLabel('导出或任务操作原因',{exact:true}).fill('下载完整固定输入候选和基准');const previousExport=new URL(page.url()).searchParams.get('export_id');await page.getByRole('button',{name:'生成完整导出',exact:true}).click();await expect.poll(()=>new URL(page.url()).searchParams.get('export_id')).not.toBe(previousExport);await expect(page.getByRole('link',{name:/\.csv$/}).first()).toBeVisible({timeout:30000});const inputPromise=page.waitForEvent('download');await page.getByRole('link',{name:/\.csv$/}).first().click();const inputDownload=await inputPromise;expect(await inputDownload.failure()).toBeNull();expect(readFileSync((await inputDownload.path())!,'utf8')).toContain('evidence_path');
 await page.reload();await expect(page.getByRole('link',{name:/\.csv$/}).first()).toBeVisible({timeout:30000});const exportID=new URL(page.url()).searchParams.get('export_id')!;await page.goto(`${f.root}/derived-runs?export_id=${exportID}`);await expect(page.getByRole('table',{name:'历史分析导出记录'})).toContainText('CSV');await expect(page.getByRole('link',{name:/\.csv$/}).first()).toBeVisible({timeout:30000});
 await page.emulateMedia({colorScheme:'dark'});await noOverflow(page);await page.screenshot({path:info.outputPath('analysis-artifact-dark.png'),fullPage:true});expect(errors).toEqual([]);
});

test('a restricted project artifact supports its authorized slice and clears widened or revoked results',async({page,request},info)=>{
 test.setTimeout(240000);const f=await prepare(page,request);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const tenantContext=await(await page.request.get(api+f.base+'/context')).json();
 const project=await f.post(f.base+'/projects',{client_request_id:randomUUID(),reason:'局部成果真实权限验收',name:'局部成果验收',code:'artifact-slice',purpose:'指定范围及输出',owner_tenant_membership_id:tenantContext.membership_id,administrator_tenant_membership_id:tenantContext.membership_id},200);
 const pb=f.base+'/projects/'+project.project_id,resource=pb+'/resource-access/facilities/'+f.facility.id,monitoring=pb+'/monitoring/derived',grants=pb+'/monitoring-access';
 const state=await(await page.request.get(api+resource)).json();
 const change={request_id:randomUUID(),reason:'授权分析输入通道',action:'create_link',expected_access_revision:state.policy.revision,add:[{kind:'facility',capabilities:['read']},{kind:'channel',channel_id:f.channel.id,capabilities:['read']}]};
 const impact=await f.post(resource+'/preview',change,200);await f.post(resource+'/changes',{...change,preview_hash:impact.preview_hash},200);
 const granted=await(await page.request.get(api+resource)).json();
 const resourceGrant=granted.grants.find((v:{channel_id?:string})=>v.channel_id===f.channel.id);
 expect(resourceGrant).toBeTruthy();
 const dataAccess=await(await page.request.get(api+grants)).json();
 const original=await f.post(grants,{...wm(dataAccess.access_revision),channel_id:f.channel.id,resource_grant_id:resourceGrant.id,range:{from:null,to:end},capabilities:['read','history','export']},200);
 const rule:DerivedRule={schema_version:'derived-rule/1',inputs:[{key:'x',shape:'numeric',source_kinds:['direct'],metric_version_id:f.metric.version_id,quantity_spec:{base_unit_version_id:f.unit.version_id,base_power:1,time_power:0,quantity_mode:'level',level_scale:'ratio'},direction:'not_applicable',reference:'not_applicable',time_semantics:{kind:'instant'}}],parameters:[],clock:{kind:'primary_input',input:'x'},nodes:[{id:'point',op:'formula',params:{expression:'x',variables:{x:'input:x'}}},{id:'delta',op:'difference',inputs:['input:x'],params:{difference_mode:'adjacent',invalid_policy:'break',max_gap_ns:'1800000000000'}}],outputs:[{key:'point',node:'point',semantic_key:'selected_point',name:'原值复核',numeric_contract:{scale:18,rounding:'half_even'}},{key:'delta',node:'delta',semantic_key:'selected_delta',name:'相邻差分',numeric_contract:{scale:18,rounding:'half_even'}}],policies:{quality:['good','unmarked'],config_boundary:'break',allowed_lateness_ns:'0'}};
 const created=await f.post(monitoring+'/analysis/configs',{...wm(),code:'SELECTED',name:'部分授权的固定分析',body:{rule,input_bindings:{x:{channel_id:f.channel.id}},parameters:{}}});
 const config=created.config,validation=await f.post(monitoring+'/analysis/configs/'+config.id+'/validate',{draft_revision:config.draft.revision},200);
 const published=await f.post(monitoring+'/analysis/configs/'+config.id+'/versions',{...wm(config.revision),draft_revision:config.draft.revision,validation_hash:validation.validation_hash});
 const run=await f.post(monitoring+'/analysis/runs',{...wm(),config_version_id:published.version.id,config_hash:published.version.config_hash,output_range:{from_ns:'1577836800000000001',to_ns:'1577836980000000000'}},202);
 await expect.poll(async()=>{const current=await(await page.request.get(api+monitoring+'/runs/'+run.id)).json();return current.job.state;},{timeout:30000}).toBe('succeeded');
 const revoke=async(grant:{id:string;revision:string})=>{const preview=await f.post(grants+'/'+grant.id+'/revoke-preview',{expected_revision:grant.revision},200);return f.post(grants+'/'+grant.id+'/revoke',{...wm(preview.grant.revision),preview_hash:preview.preview_hash},200);};
 const revoked=await revoke(original),sliceFrom='2020-01-01T00:01:00Z',sliceTo='2020-01-01T00:02:00Z';
 const partial=await f.post(grants,{...wm(revoked.access_revision),channel_id:f.channel.id,resource_grant_id:resourceGrant.id,range:{from:sliceFrom,to:sliceTo},capabilities:['read','history','export']},200);
 const root=`${consoleURL}/t/${f.tenant}/p/${project.project_id}/monitoring`,artifactURL=`${root}/analysis/${run.analysis.artifact_id}?version=${run.analysis.artifact_version_id}`;
 await page.goto(artifactURL);
 const selection=section(page,'按获准片段读取成果');await expect(selection).toBeVisible();
 await selection.getByLabel('起点（含）',{exact:true}).fill(sliceFrom);await selection.getByLabel('终点（不含）',{exact:true}).fill(sliceTo);await selection.getByLabel('输出标识（逗号分隔，可留空）',{exact:true}).fill('point');await selection.getByRole('button',{name:'读取获准片段',exact:true}).click();
 const values=page.getByRole('table',{name:'计算输出与匹配解释'});await expect(values).toContainText('12.000000000000000000');await expect(values.getByRole('row')).toHaveCount(2);await expect(page.getByText('当前为所选范围及输出的获准片段；整份成果行数不在此片段中披露。',{exact:true})).toBeVisible();
 await page.getByRole('button',{name:'与另一成果版本比较',exact:true}).click();const comparison=section(page,'比较固定成果：左侧 V1');await comparison.getByLabel('比较原因',{exact:true}).fill('只比较获准原值片段');await comparison.getByRole('button',{name:'提交所选片段比较',exact:true}).click();await expect(comparison.getByText('已完成',{exact:true})).toBeVisible({timeout:30000});await expect(comparison.getByText('输出范围：2020-01-01T00:01:00.000000000Z 至 2020-01-01T00:02:00.000000000Z（终点不含）',{exact:true})).toBeVisible();
 await page.getByLabel('导出或任务操作原因',{exact:true}).fill('导出一分钟的获准原值');await page.getByRole('button',{name:'生成完整导出',exact:true}).click();const link=page.getByRole('link',{name:/\.csv$/}).first();await expect(link).toBeVisible({timeout:30000});const downloadPromise=page.waitForEvent('download');await link.click();const download=await downloadPromise;expect(await download.failure()).toBeNull();const csv=readFileSync((await download.path())!,'utf8');expect(csv).toContain('12.000000000000000000');expect(csv).not.toContain('15.000000000000000000');
 await page.emulateMedia({colorScheme:'dark'});await noOverflow(page);await page.screenshot({path:info.outputPath('analysis-authorized-slice-dark.png'),fullPage:true});
 await selection.getByLabel('输出标识（逗号分隔，可留空）',{exact:true}).fill('delta');await selection.getByRole('button',{name:'读取获准片段',exact:true}).click();await expect(selection.getByRole('alert')).toBeVisible();await expect(values).toHaveCount(0);
 await selection.getByLabel('输出标识（逗号分隔，可留空）',{exact:true}).fill('point');await selection.getByLabel('起点（含）',{exact:true}).fill(start);await selection.getByRole('button',{name:'读取获准片段',exact:true}).click();await expect(selection.getByRole('alert')).toBeVisible();await expect(values).toHaveCount(0);
 await selection.getByLabel('起点（含）',{exact:true}).fill(sliceFrom);await selection.getByRole('button',{name:'读取获准片段',exact:true}).click();await expect(values).toContainText('12.000000000000000000');await revoke(partial);await page.evaluate(()=>{window.dispatchEvent(new Event('focus'));document.dispatchEvent(new Event('visibilitychange'));});await expect(values).toHaveCount(0,{timeout:35000});await expect(page.getByRole('link',{name:/\.csv$/})).toHaveCount(0);await noOverflow(page);expect(errors).toEqual([]);
});

test('the actual CLI authenticates through a terminal and operates the live API with reviewable previews',async({page,request},info)=>{
 test.skip(!process.env.INFRAPRO_E2E_OPS_CONFIG||info.project.name!=='desktop','explicit disposable operator configuration required');
 test.setTimeout(240000);const f=await prepare(page,request),root=f.monitoring+'/derived',execute=promisify(execFile);
 const binary=info.outputPath('infrapro'),session=info.outputPath('session.json');mkdirSync(info.outputPath(),{recursive:true});
 await execute('go',['build','-o',binary,'./tools/infrapro/cmd/infrapro'],{timeout:90000});
 const rule:DerivedRule=await f.post(root+'/templates/difference/expand',{inputs:[{key:'x',shape:'numeric',source_kinds:['direct'],metric_version_id:f.metric.version_id,quantity_spec:{base_unit_version_id:f.unit.version_id,base_power:1,time_power:0,quantity_mode:'level',level_scale:'ratio'},direction:'not_applicable',reference:'not_applicable',time_semantics:{kind:'instant'}}]},200);
 const created=await f.post(root+'/metric-definitions',{...wm(),code:'CLI_DELTA',name:'命令行差分验收',description:'',rule});
 const validation=await f.post(root+'/metric-definitions/'+created.definition.id+'/validate',{draft_revision:created.definition.draft.revision},200);
 const published=await f.post(root+'/metric-definitions/'+created.definition.id+'/publish',{...wm(created.definition.revision),draft_revision:created.definition.draft.revision,validation_hash:validation.validation_hash});
 let channel=(await f.post(root+'/channels',{...wm(),code:'CLI_DELTA',name:'命令行差分验收',owner:{kind:'point',id:f.point.id,facility_id:f.facility.id},definition_version_id:published.version.id,input_bindings:{x:{channel_id:f.channel.id}},parameters:{}})).channel;
 const path=root+'/channels/'+channel.resource.id,config=(await(await page.request.get(api+path+'/configs')).json()).items[0],output={from_ns:'1577836800000000001',to_ns:'1577836980000000000'};
 let trial:DerivedRun=await f.post(path+'/trials',{...wm(channel.config_revision),config_id:config.id,config_hash:config.config_hash,output_range:output},[200,202]);
 await expect.poll(async()=>{trial=await(await page.request.get(api+root+'/runs/'+trial.id)).json();return trial.job.state;},{timeout:30000}).toBe('succeeded');
 const preview=await f.post(path+'/control-preview',{expected_revision:channel.control_revision,action:'enable',auto_start_ns:output.from_ns,stop_ns:output.to_ns,trial_receipt_id:trial.trial!.trial_receipt_id},200);
 channel=(await f.post(path+'/control',{...wm(channel.control_revision),preview_id:preview.id,preview_hash:preview.hash},200)).channel;
 let generations:{items:{id:string}[]}={items:[]};await expect.poll(async()=>{generations=await(await page.request.get(api+path+'/generations?from_ns='+output.from_ns+'&to_ns='+output.to_ns)).json();return generations.items.length;},{timeout:30000}).toBeGreaterThan(0);
 const evidence:Record<string,unknown>={};
 const invoke=async(action:string,args:string[]=[],business=true)=>{const result=await execute(binary,['monitoring','derived',action,...(business?['--session-file',session,'--tenant',f.tenant]:[]),...args],{env:{...process.env,INFRAPRO_CONFIG:process.env.INFRAPRO_E2E_OPS_CONFIG!},timeout:90000,maxBuffer:8*1024*1024});return JSON.parse(result.stdout);};
 try{
 const authenticated=await execute('python3',[resolve('scripts/fixtures/derived-cli-login.py'),binary,origin,f.user.username,session],{env:{...process.env,INFRAPRO_CLI_TEST_PASSWORD:f.user.password},timeout:45000});expect(JSON.parse(authenticated.stdout).state).toBe('authenticated');expect(statSync(session).mode&0o777).toBe(0o600);evidence.login={state:'authenticated',mode:'0600',transport:'live HTTP with terminal password'};
 const pause=await invoke('pause',['--channel',channel.resource.id,'--expected-revision',channel.control_revision]);evidence.pause_preview=pause;expect((await(await page.request.get(api+path)).json()).control_state).toBe('enabled');
 channel=(await invoke('pause',['--channel',channel.resource.id,'--expected-revision',channel.control_revision,'--apply','--preview-id',pause.id,'--preview-hash',pause.hash,'--reason','命令行实际暂停自验'])).channel;expect(channel.control_state).toBe('paused');
 const resume=await invoke('resume',['--channel',channel.resource.id,'--expected-revision',channel.control_revision,'--resume-mode','catch_up','--to-ns',(BigInt(pause.effective_at_ns)+60_000_000_000n).toString(),'--trial-receipt-id',trial.trial!.trial_receipt_id]);evidence.resume_preview=resume;
 channel=(await invoke('resume',['--channel',channel.resource.id,'--expected-revision',channel.control_revision,'--apply','--preview-id',resume.id,'--preview-hash',resume.hash,'--reason','命令行按预览恢复自验'])).channel;expect(channel.control_state).toBe('enabled');
 const backfill=await invoke('backfill',['--channel',channel.resource.id,'--expected-revision',channel.config_revision,'--from-ns',output.from_ns,'--to-ns',output.to_ns]);evidence.backfill_preview=backfill;
 const computed=await invoke('backfill',['--channel',channel.resource.id,'--expected-revision',channel.config_revision,'--apply','--preview-id',backfill.id,'--preview-hash',backfill.hash,'--reason','命令行固定范围回算自验','--wait']);expect(computed.runs.every((r:{job:{state:string}})=>r.job.state==='succeeded')).toBe(true);evidence.backfill=computed;
 const replay=await invoke('replay',['--generation',generations.items[0]!.id,'--compare','--apply','--reason','命令行固定原依据回放自验','--wait']);expect(replay.historical_replay.matched).toBe(true);expect(replay.job.state).toBe('succeeded');evidence.replay=replay;
 for(const action of ['check','status','reconcile','recovery'])evidence[action]=await invoke(action,action==='check'?[]:['--tenant',f.tenant],false);
 expect((evidence.check as {state:string}).state).toBe('passed');expect((evidence.reconcile as {state:string}).state).toBe('passed');
 writeFileSync(info.outputPath('cli-evidence.json'),JSON.stringify(evidence,null,2));await info.attach('actual-cli-evidence',{path:info.outputPath('cli-evidence.json'),contentType:'application/json'});
 }finally{rmSync(session,{force:true});}
});

test('quantile editor uses percentage positions and sample-window counts stay exact',async({page,request})=>{
 test.setTimeout(180000);const f=await prepare(page,request);
 await page.goto(`${f.root}/analysis?id=new`);
 await page.getByLabel('分析编码',{exact:true}).fill('PERCENTILE');
 await page.getByLabel('分析名称',{exact:true}).fill('界面 P95 与样本窗门槛');
 await configureRule(page,f);
 await page.getByLabel('计算方法',{exact:true}).selectOption('window_count');
 await expect(page.getByLabel('最少合格样本',{exact:true})).toHaveValue('0');
 await page.getByLabel('窗口类型',{exact:true}).selectOption('trailing_count');
 await page.getByLabel('样本数量 N',{exact:true}).fill('3');
 await expect(page.getByLabel('最少合格样本',{exact:true})).toHaveValue('3');
 await page.getByLabel('计算方法',{exact:true}).selectOption('window_quantile');
 const percentile=page.getByLabel('分位位置（0 到 100，P95 填 95）',{exact:true});
 await expect(percentile).toHaveValue('50');await percentile.fill('95');
 await page.getByLabel('窗口时长（秒）',{exact:true}).fill('180');
 await page.getByLabel('最少合格样本',{exact:true}).fill('2');
 await page.getByRole('button',{name:/跨中测点 · 原始位移 · DISP/}).click();
 await page.getByLabel('操作原因',{exact:true}).fill('验证百分位输入95对应R7 P95');
 await page.getByRole('button',{name:'保存分析草稿',exact:true}).click();
 await page.getByRole('button',{name:'校验已保存草稿',exact:true}).click();
 await expect(page.getByText('校验通过，可生成固定执行版本',{exact:true})).toBeVisible();
 await page.getByLabel('操作原因',{exact:true}).fill('固定已校验的R7 P95规则');
 await page.getByRole('button',{name:'保存固定执行版本',exact:true}).click();
 const execution=section(page,'运行固定配置 V1');await range(execution);
 await execution.getByLabel('运行原因',{exact:true}).fill('从实际原始输入核对14.7');
 await execution.getByRole('button',{name:'运行并生成完整成果',exact:true}).click();
 await expect(execution.getByRole('link',{name:'查看本次完整成果',exact:true})).toBeVisible({timeout:30000});
 await execution.getByRole('link',{name:'查看本次完整成果',exact:true}).click();
 const rows=page.getByRole('table',{name:'计算输出与匹配解释'});
 await expect(rows).toContainText('11.900000000000000000');
 await expect(rows).toContainText('14.700000000000000000');
 await noOverflow(page);
});
