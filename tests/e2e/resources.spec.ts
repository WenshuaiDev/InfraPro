import { readFileSync } from 'node:fs';
import { positionIssues } from '../../apps/console/src/features/resources/validation';
import { randomBytes, randomUUID } from 'node:crypto';
import type { APIRequestContext, Page } from '@playwright/test';
import { test, expect, consoleURL, csrf, login, password, createAccount } from './fixtures';
import { navigateTo } from '../helpers/navigation';
const origin=new URL(consoleURL).origin;const api=`${origin}/api/v1`;const wm=()=>({request_id:randomUUID(),reason:'资源自验'});
async function seed(request:APIRequestContext){
 const user=await createAccount(request,false);const headers={Origin:origin,'X-CSRF-Token':await csrf(request,origin)};expect((await request.post(`${api}/auth/login`,{headers,data:{username:'e2e_owner',password}})).status()).toBe(200);const suffix=randomBytes(5).toString('hex');
 const post=async(path:string,data:unknown,status=200)=>{const r=await request.post(`${api}${path}`,{headers,data});expect(r.status(),await r.text()).toBe(status);return r.json();};const publish=async(kind:string,code:string,content:object)=>{const d=await post(`/platform/engineering/${kind}`,{...wm(),code,content:{metamodel_version:'1',description:'',release_notes:'',...content}});return post(`/platform/engineering/${kind}/${d.definition_id}/publish`,{...wm(),revision:d.revision});};
 await post('/platform/engineering/unit-categories',{...wm(),key:`len_${suffix}`,name:'长度',description:'空间长度'});const unit=await publish('units',`mm_${suffix}`,{name:'毫米',unit:{symbol:'mm',category:`len_${suffix}`}});const metric=await publish('metrics',`disp_${suffix}`,{name:`位移_${suffix}`,metric:{shape:'decimal',category:`len_${suffix}`,standard_unit:unit.version_id,allowed_units:[unit.version_id],options:[]}});const enumMetric=await publish('metrics',`enum_${suffix}`,{name:`状态_${suffix}`,metric:{shape:'enum',category:'',standard_unit:'',allowed_units:[],options:[{key:'yes',name:'正常'},{key:'no',name:'异常'}]}});
 const type=await publish('type-definitions',`resource_${suffix}`,{name:`资源类型_${suffix}`,fields:[{key:'length',name:'设计长度',shape:'decimal',required:true,default:0,description:'',group:'',order:0,multiline:false}],components:[{key:'span',name:'跨',description:'',fields:[]},{key:'beam',name:'梁',description:'',fields:[]}],relations:[{parent:'root',child:'span',min_count:2,max_count:2,order:0},{parent:'span',child:'beam',min_count:0,max_count:3,order:0}]});
 const accounts=await(await request.get(`${api}/access/accounts?q=${user.username}`)).json();const tenant=await post('/platform/tenants',{name:`资源租户_${suffix}`,code:`res-${suffix}`,administrator_account_id:accounts.items[0].id,client_request_id:randomUUID(),reason:'资源验收'},201);
 return {user,headers,suffix,unit,metric,enumMetric,type,tenant:tenant.tenant_id as string,publish,post};
}
async function pick(page:Page,label:string,definition:string,version:string,q:string){await page.getByLabel(`${label}搜索`,{exact:true}).fill(q);await page.getByLabel(`${label}定义`,{exact:true}).selectOption(definition);await page.getByLabel(`${label}版本`,{exact:true}).selectOption(version);}
async function save(page:Page,ending:string,status=201){const response=page.waitForResponse((r)=>['POST','PATCH'].includes(r.request().method())&&(r.url().endsWith(ending)||(status>=400&&r.url().endsWith(ending+'/mutation-preview'))));await page.getByRole('button',{name:'保存',exact:true}).click();const r=await response;expect(r.status(),await r.text()).toBe(status);return r.json();}

test('numeric resource inputs preserve typed decimals and save their exact numeric values', async ({ page, request }) => {
 const f = await seed(request); await login(page, f.user);
 for (const [index, text] of ['0.05', '-0.5', '1.05', '1.00'].entries()) {
  await page.goto(`${consoleURL}/t/${f.tenant}/resources/facilities`);
  await page.getByRole('button', { name: '新增设施', exact: true }).click();
  await page.getByLabel('编号 *', { exact: true }).fill(`DECIMAL-${index}`);
  await page.getByLabel('名称 *', { exact: true }).fill(`数值输入 ${text}`);
  await pick(page, '设施类型', f.type.definition_id, f.type.version_id, f.suffix);
  const input = page.getByLabel('设计长度 *', { exact: true });
  await input.fill(''); await input.pressSequentially(text);
  await expect(input).toHaveValue(text);
  const saved = await save(page, '/facilities');
  const detail = await (await page.request.get(`${api}/tenants/${f.tenant}/engineering/resources/facilities/${saved.id}`)).json();
  expect(detail.values.length).toBe(Number(text));
 }
 const actions = page.getByRole('button', { name: '数值输入 1.00 操作', exact: true });
 await actions.click();
 await page.getByRole('menu').press('Escape');
 await expect(actions).toBeFocused();
 await expect(page.getByRole('dialog')).toHaveCount(0);
 await actions.click();
 await page.getByRole('menuitem', { name: '编辑设施', exact: true }).click();
 const input = page.getByLabel('设计长度 *', { exact: true });
 await input.fill('');
 await expect(input).toHaveValue('');
 await expect(input).toBeFocused();
 await input.pressSequentially('1e');
 await expect(input).toHaveValue('1e');
 await page.getByRole('button', { name: '保存', exact: true }).click();
 await expect(page.getByRole('heading', { name: /需要处理的问题/ })).toBeVisible();
 await expect(input).toHaveValue('1e');
 await page.getByRole('button', { name: '关闭面板', exact: true }).click();
 await expect(actions).toBeFocused();
});

test('component ownership tree never submits an edit until explicit save', async ({ page, request }) => {
 const f = await seed(request); await login(page, f.user);
 const headers = { Origin: origin, 'X-CSRF-Token': await csrf(page.request, origin) };
 const base = `${api}/tenants/${f.tenant}/engineering/resources`;
 const post = async (path: string, data: object) => { const r = await page.request.post(base + path, { headers, data: { request_id: randomUUID(), ...data } }); expect(r.status(), await r.text()).toBe(201); return r.json(); };
 const facility = await post('/facilities', { code: 'TREE', name: '树操作设施', type_version_id: f.type.version_id, values: { length: 0 } });
 const root = `/facilities/${facility.id}`;
 const span = await post(`${root}/components`, { code: 'SPAN', name: '第一跨', category_key: 'span', expected_structure_revision: 1 });
 const beam = await post(`${root}/components`, { code: 'BEAM', name: '第一梁', category_key: 'beam', parent_component_id: span.id, expected_structure_revision: 2 });
 const point = await post(`${root}/points`, { code: 'POINT', name: '归属测试点' });
 await page.goto(`${consoleURL}/t/${f.tenant}/resources/facilities/${facility.id}?point=${point.id}`);
 await page.getByRole('button', { name: '归属测试点 操作', exact: true }).click();
 await page.getByRole('menuitem', { name: '编辑测点', exact: true }).click();
 const panel = page.getByRole('dialog'); let writes = 0;
 page.on('request', (r) => { if (r.method() === 'PATCH' && r.url().endsWith(`/points/${point.id}`)) writes++; });
 await panel.getByLabel('名称 *', { exact: true }).fill('主动保存后改名');
 await panel.getByText('选择或纠正同设施归属', { exact: true }).click();
 const tree = panel.getByRole('navigation', { name: '设施构件', exact: true });
 await tree.getByRole('button', { name: '展开 第一跨', exact: true }).click();
 await expect(tree.getByRole('button', { name: '第一梁 BEAM', exact: true })).toBeVisible();
 await tree.getByRole('button', { name: '第一梁 BEAM', exact: true }).click();
 await expect(tree.getByRole('button', { name: '第一梁 BEAM', exact: true })).toHaveAttribute('aria-current', 'true');
 await tree.getByRole('button', { name: '设施整体', exact: true }).click();
 await expect(tree.getByRole('button', { name: '设施整体', exact: true })).toHaveAttribute('aria-current', 'true');
 await tree.getByRole('button', { name: '第一梁 BEAM', exact: true }).click();
 expect(writes).toBe(0);
 expect((await (await page.request.get(base + root + `/points/${point.id}`)).json()).name).toBe('归属测试点');
 await panel.getByLabel('修改原因（共享或归属修改必填）',{exact:true}).fill('纠正同设施归属');
 await page.getByRole('button',{name:'保存',exact:true}).click();await page.getByRole('button',{name:'确认保存',exact:true}).click();
 await expect(panel).toBeHidden(); expect(writes).toBe(1);
 const saved = await (await page.request.get(base + root + `/points/${point.id}`)).json();
 expect(saved.component_id).toBe(beam.id); expect(saved.name).toBe('主动保存后改名');
});

test('actual resources: direct save, min count reference, point/channel, positions, history, retirement and deep link',async({page,request},info)=>{
 test.setTimeout(180_000);const f=await seed(request);const errors:string[]=[];page.on('pageerror',(e)=>errors.push(e.message));await login(page,f.user);await page.goto(`${consoleURL}/t/${f.tenant}/resources/facilities`);await page.getByRole('button',{name:'新增设施',exact:true}).click();await page.getByLabel('编号 *',{exact:true}).fill(`F-${f.suffix}`);await page.getByLabel('名称 *',{exact:true}).fill('测试桥梁');await pick(page,'设施类型',f.type.definition_id,f.type.version_id,f.suffix);await expect(page.getByLabel('设计长度 *',{exact:true})).toHaveValue('0');const facility=await save(page,'/facilities');await expect(page.getByRole('heading',{name:'测试桥梁',exact:true})).toBeVisible();await expect(page.getByText('暂无测点',{exact:true})).toBeVisible();await expect(page.getByText(/待完善|完成度|发布后启用/)).toHaveCount(0);
 await page.getByRole('button',{name:'新增构件',exact:true}).click();await page.getByLabel('编号 *',{exact:true}).fill('C-1');await page.getByLabel('名称 *',{exact:true}).fill('第一跨');await page.getByLabel('构件类别 *',{exact:true}).selectOption('span');const component=await save(page,'/components');await expect(page.getByRole('heading',{name:'第一跨',exact:true})).toBeVisible();await page.getByRole('button',{name:'添加测点',exact:true}).click();await page.getByLabel('编号 *',{exact:true}).fill('P-1');await page.getByLabel('名称 *',{exact:true}).fill('跨中位移测点');await page.getByText('位置资料（可选）',{exact:true}).click();await page.getByRole('button',{name:'填写地理坐标',exact:true}).click();await page.getByLabel('经度（度）',{exact:true}).fill('0');await page.getByLabel('纬度（度）',{exact:true}).fill('0');await page.getByRole('button',{name:'保存',exact:true}).click();await expect(page.getByRole('heading',{name:/需要处理的问题/})).toBeVisible();await page.screenshot({path:info.outputPath('resource-position-error.png'),fullPage:true});await page.getByLabel('坐标系依据',{exact:true}).fill('EPSG:4326');await page.getByRole('button',{name:'添加路线参照',exact:true}).click();await page.getByLabel('路线编号',{exact:true}).fill('G109');await page.getByLabel('起点或点位桩号',{exact:true}).fill('K123+450');await page.getByRole('button',{name:'添加断面参照',exact:true}).click();await page.getByLabel('断面种类',{exact:true}).selectOption('facility_monitoring_section');await page.getByLabel('断面编号',{exact:true}).fill('DM-01');await page.getByLabel('参照说明',{exact:true}).fill('主梁跨中断面');const point=await save(page,'/points');await expect(page.getByRole('heading',{name:'测点：跨中位移测点',exact:true})).toBeVisible();await expect(page.getByText('暂无业务通道',{exact:true})).toBeVisible();
 await page.getByRole('button',{name:'添加通道',exact:true}).click();await page.getByLabel('编号 *',{exact:true}).fill('CH-1');await page.getByLabel('名称 *',{exact:true}).fill('横向位移');await pick(page,'监测指标',f.metric.definition_id,f.metric.version_id,f.suffix);await page.getByLabel('固定单位 *',{exact:true}).selectOption(f.unit.version_id);const channel=await save(page,'/channels');await expect(page.getByRole('heading',{name:'通道：横向位移',exact:true})).toBeVisible();await expect(page.getByText(/固定单位：毫米/)).toBeVisible();
 await page.screenshot({path:info.outputPath('resource-workspace.png'),fullPage:true});await page.reload();await expect(page.getByRole('heading',{name:'测点：跨中位移测点',exact:true})).toBeVisible();expect(page.url()).toContain(`component=${component.id}`);expect(page.url()).toContain(`point=${point.id}`);
 const headers={Origin:origin,'X-CSRF-Token':await csrf(page.request,origin)};const root=`${api}/tenants/${f.tenant}/engineering/resources/facilities/${facility.id}`;const detail=await(await page.request.get(`${root}/points/${point.id}`)).json();expect(detail.position.geographic.longitude).toBe(0);expect(detail.component_id).toBe(component.id);expect((await page.request.get(`${root}/points/${point.id}/channels/${channel.id}`)).status()).toBe(200);
 await page.getByRole('button',{name:'第一跨 操作',exact:true}).click();await page.getByRole('menuitem',{name:'删除构件',exact:true}).click();await page.getByLabel('原因 *',{exact:true}).fill('依赖保护');await page.getByRole('button',{name:'预览删除影响',exact:true}).click();await expect(page.getByText('仍有 1 个测点',{exact:true})).toBeVisible();await page.getByRole('button',{name:'关闭面板',exact:true}).click();
 await page.getByRole('button',{name:'测试桥梁 操作',exact:true}).click();await page.getByRole('menuitem',{name:'退役设施',exact:true}).click();await page.getByLabel('原因 *',{exact:true}).fill('暂停使用');await page.getByRole('button',{name:'预览退役影响',exact:true}).click();await page.getByRole('button',{name:'确认保存',exact:true}).click();await expect(page.getByRole('button',{name:'添加测点',exact:true})).toHaveCount(0);await expect(page.getByText('祖先已退役，请先恢复祖先。').first()).toBeVisible();await page.getByRole('button',{name:'测试桥梁 操作',exact:true}).click();await page.getByRole('menuitem',{name:'恢复设施',exact:true}).click();await page.getByLabel('原因 *',{exact:true}).fill('恢复使用');await page.getByRole('button',{name:'预览恢复影响',exact:true}).click();await page.getByRole('button',{name:'确认保存',exact:true}).click();await expect(page.getByRole('button',{name:'添加测点',exact:true})).toBeVisible();
 // Conflict keeps input; a failed write is not automatically replayed.
 await page.getByRole('button',{name:'第一跨 操作',exact:true}).click();await page.getByRole('menuitem',{name:'编辑构件',exact:true}).click();await page.getByLabel('名称 *',{exact:true}).fill('我的未提交名称');expect((await page.request.patch(`${root}/components/${component.id}`,{headers,data:{request_id:randomUUID(),expected_revision:1,name:'另一窗口已修改'}})).status()).toBe(200);await save(page,`/components/${component.id}`,409);await expect(page.getByLabel('名称 *',{exact:true})).toHaveValue('我的未提交名称');await page.getByRole('button',{name:'关闭面板',exact:true}).click();
 await page.getByRole('button',{name:'设施变更记录',exact:true}).click();await expect(page.getByRole('heading',{name:'变更记录',exact:true})).toBeVisible();await expect(page.getByText('另一窗口已修改 · C-1',{exact:true})).toBeVisible();for(const theme of ['light','dark']){await page.evaluate((theme)=>document.documentElement.classList.toggle('dark',theme==='dark'),theme);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:info.outputPath(`resources-${theme}.png`),fullPage:true});}
 expect(errors).toEqual([]);
});

test('resources HTTP contracts: nested paths, strict writes, independent channels, replay and deleted history',async({page,request})=>{
 const f=await seed(request);await login(page,f.user);const headers={Origin:origin,'X-CSRF-Token':await csrf(page.request,origin)};const base=`${api}/tenants/${f.tenant}/engineering/resources`;const post=async(path:string,data:unknown,status:number)=>{if(/\/(retire|restore|delete)$/.test(path)){const action=path.split('/').pop();const pr=await page.request.post(base+path.replace(/\/(retire|restore|delete)$/,'/mutation-preview'),{headers,data:{...(data as object),action}});if(pr.status()===200)data={...(data as object),preview_hash:(await pr.json()).preview_hash};}const r=await page.request.post(base+path,{headers,data});expect(r.status(),await r.text()).toBe(status);return r.json();};
 const input={request_id:randomUUID(),code:'F-1',name:'设施',type_version_id:f.type.version_id,values:{length:0}};const facility=await post('/facilities',input,201);expect((await post('/facilities',input,201)).replayed).toBe(true);await post('/facilities',{...input,name:'冲突'},409);await post('/facilities',{...input,request_id:randomUUID(),status:'retired'},400);const root=`/facilities/${facility.id}`;
 const point=await post(`${root}/points`,{request_id:randomUUID(),code:'P1',name:'根测点'},201);const channels=`${root}/points/${point.id}/channels`;for(const code of ['A','B'])await post(channels,{request_id:randomUUID(),code,name:code,metric_version_id:f.metric.version_id,unit_version_id:f.unit.version_id,timestamp_meaning:'observation_time'},201);
 const numeric={code:'INVALID',name:'拒绝不合法语义',metric_version_id:f.metric.version_id,unit_version_id:f.unit.version_id,timestamp_meaning:'observation_time'};
 for(const sampling of [{mode:'periodic',interval_seconds:0},{mode:'event',interval_seconds:60},{mode:'event',interval_seconds:null},{mode:'manual',interval_seconds:1}])await post(channels,{...numeric,request_id:randomUUID(),sampling},422);
 await post(channels,{...numeric,request_id:randomUUID(),timestamp_meaning:'interval_end'},422);
 await post(channels,{...numeric,request_id:randomUUID(),unit_version_id:null},422);
 const incompatible=await f.publish('units',`other_${f.suffix}`,{name:'不在允许列表的单位',unit:{symbol:'cm',category:`len_${f.suffix}`}});
 await post(channels,{...numeric,request_id:randomUUID(),unit_version_id:incompatible.version_id},422);
 await post(channels,{request_id:randomUUID(),code:'C',name:'枚举',metric_version_id:f.enumMetric.version_id,unit_version_id:f.unit.version_id,timestamp_meaning:'observation_time'},422);const enumCh=await post(channels,{request_id:randomUUID(),code:'C',name:'枚举',metric_version_id:f.enumMetric.version_id,unit_version_id:null,timestamp_meaning:'interval_end',aggregation_description:'每日状态'},201);
 expect((await page.request.get(`${base}${root}/points/${randomUUID()}/channels/${enumCh.id}`)).status()).toBe(404);expect((await request.get(base+root)).status()).toBe(404);
 const changed=await page.request.patch(`${base}${channels}/${enumCh.id}`,{headers,data:{request_id:randomUUID(),expected_revision:1,timestamp_meaning:'observation_time'}});expect(changed.status()).toBe(400);
 const leaf=await post(`${root}/points`,{request_id:randomUUID(),code:'DELETE',name:'误建测点'},201);await post(`${root}/points/${leaf.id}/delete`,{request_id:randomUUID(),expected_revision:1,reason:'误建'},200);expect((await page.request.get(`${base}${root}/points/${leaf.id}`)).status()).toBe(404);await post(`${root}/points`,{request_id:randomUUID(),code:'delete',name:'保留编号'},409);expect((await(await page.request.get(`${base}${root}/changes?id=${leaf.id}`)).json()).items).toHaveLength(2);
 const huge=await page.request.post(base+'/facilities',{headers,data:{...input,request_id:randomUUID(),description:'x'.repeat(260*1024)}});expect(huge.status()).toBe(413);
 await post(`${channels}/${enumCh.id}/delete`, { request_id: randomUUID(), expected_revision: 1, reason: '保留历史枚举解释' }, 200);
 expect((await page.request.get(`${base}${channels}/${enumCh.id}`)).status()).toBe(404);
 await page.goto(`${consoleURL}/t/${f.tenant}/resources/facilities/${facility.id}`);
 await page.getByRole('button', { name: '设施变更记录', exact: true }).click();
 const row = page.getByRole('row').filter({ hasText: '枚举 · C' }).first();
 await row.getByText('查看前后值', { exact: true }).click();
 await expect(row.getByText(`状态_${f.suffix} · V1`, { exact: true })).toBeVisible();
 await expect(row.getByText('yes：正常', { exact: true })).toBeVisible();
 await expect(row.getByText('no：异常', { exact: true })).toBeVisible();
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
 await page.screenshot({ path: test.info().outputPath('deleted-channel-history.png'), fullPage: true });
});

test('position validation shares the Go boundary samples',()=>{const positions=JSON.parse(readFileSync('tests/fixtures/resource-positions.json','utf8')) as {name:string;position:unknown;valid:boolean}[];for(const sample of positions)expect(positionIssues(sample.position).length===0,sample.name).toBe(sample.valid);});

test('resource lost response uses a receipt without replay and keeps inputs during outage',async({page,request})=>{
 const f=await seed(request);await login(page,f.user);const headers={Origin:origin,'X-CSRF-Token':await csrf(page.request,origin)};const base=`${api}/tenants/${f.tenant}/engineering/resources`;const r=await page.request.post(base+'/facilities',{headers,data:{request_id:randomUUID(),code:'LOST',name:'原名称',type_version_id:f.type.version_id,values:{length:0}}});expect(r.status()).toBe(201);const facility=await r.json();await page.goto(`${consoleURL}/t/${f.tenant}/resources/facilities/${facility.id}`);await page.getByRole('button',{name:'原名称 操作',exact:true}).click();await page.getByRole('menuitem',{name:'编辑设施',exact:true}).click();await page.getByLabel('名称 *',{exact:true}).fill('已提交但响应丢失');let writes=0;await page.route(`${base}/facilities/${facility.id}`,async(route)=>{if(route.request().method()!=='PATCH')return route.continue();writes++;const response=await route.fetch();expect(response.status()).toBe(200);await route.abort('failed');});await page.getByRole('button',{name:'保存',exact:true}).click();await expect(page.getByText('结果尚未确认，请查验对应记录后再决定是否重试。',{exact:true})).toBeVisible();await expect(page.getByLabel('名称 *',{exact:true})).toHaveValue('已提交但响应丢失');await expect(page.getByRole('button',{name:'保存',exact:true})).toBeDisabled();await page.unroute(`${base}/facilities/${facility.id}`);await page.getByRole('button',{name:'核对本次提交结果',exact:true}).click();await expect(page.getByRole('heading',{name:'已提交但响应丢失',exact:true})).toBeVisible();expect(writes).toBe(1);expect((await(await page.request.get(`${base}/facilities/${facility.id}/changes`)).json()).items).toHaveLength(2);
});

test('late resource write cannot reopen an object after switching tenants',async({page,request})=>{
 const f=await seed(request);
 const accounts=await(await request.get(`${api}/access/accounts?q=${f.user.username}`)).json();
 const second=await f.post('/platform/tenants',{name:`隔离租户_${f.suffix}`,code:`other-${f.suffix}`,administrator_account_id:accounts.items[0].id,client_request_id:randomUUID(),reason:'迟到响应隔离'},201);
 await login(page,f.user);
 const headers={Origin:origin,'X-CSRF-Token':await csrf(page.request,origin)};
 const base=`${api}/tenants/${f.tenant}/engineering/resources`;
 const response=await page.request.post(base+'/facilities',{headers,data:{request_id:randomUUID(),code:'PRIVATE',name:'第一租户私有设施',type_version_id:f.type.version_id,values:{length:0}}});expect(response.status()).toBe(201);
 const facility=await response.json();
 await page.goto(`${consoleURL}/t/${f.tenant}/resources/facilities/${facility.id}`);
 await page.getByRole('button',{name:'第一租户私有设施 操作',exact:true}).click();
 await page.getByRole('menuitem',{name:'编辑设施',exact:true}).click();
 await page.getByLabel('名称 *',{exact:true}).fill('第一租户迟到内容');
 let release!:()=>void;const held=new Promise<void>((resolve)=>{release=resolve;});
 let fetched!:()=>void;const ready=new Promise<void>((resolve)=>{fetched=resolve;});
 await page.route(`${base}/facilities/${facility.id}`,async(route)=>{
  if(route.request().method()!=='PATCH')return route.continue();
  const result=await route.fetch();expect(result.status()).toBe(200);fetched();await held;
  try{await route.fulfill({response:result});}catch{/* Unmount may cancel this completed write. */}
 });
 await page.getByRole('button',{name:'保存',exact:true}).click();await ready;
 await page.getByRole('button',{name:'关闭面板',exact:true}).click();
 const navigation=page.getByRole('navigation',{name:'主导航',exact:true});
 if(!await navigation.isVisible())await page.getByRole('banner').getByRole('button',{name:'切换侧栏',exact:true}).click();
 await page.getByRole('button',{name:/^切换工作空间：/}).click();
 await page.getByRole('menuitem',{name:new RegExp(`^隔离租户_${f.suffix} 租户`)}).click();
 await expect(page).toHaveURL(`${consoleURL}/t/${second.tenant_id}/profile`);
 await expect(page.getByRole('heading',{name:'租户资料',exact:true})).toBeVisible();
 if((page.viewportSize()?.width??1440)<768)await expect(page.getByRole('dialog',{name:'主导航',exact:true})).toBeHidden();
 release();await page.unroute(`${base}/facilities/${facility.id}`);
 await navigateTo(page,'设施管理');
 await expect(page).toHaveURL(`${consoleURL}/t/${second.tenant_id}/resources/facilities`);
 await expect(page.getByText('第一租户迟到内容',{exact:true})).toHaveCount(0);
 await expect(page.getByText('第一租户私有设施',{exact:true})).toHaveCount(0);
 await expect(page.getByRole('dialog')).toHaveCount(0);
 expect((await(await page.request.get(`${base}/facilities/${facility.id}`)).json()).name).toBe('第一租户迟到内容');
});
