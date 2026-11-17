import { randomBytes, randomUUID } from 'node:crypto';
import type { APIRequestContext, Page } from '@playwright/test';
import { test, expect, consoleURL, csrf, login, password, createAccount } from './fixtures';
import {navigateTo,openWorkspacePicker,browseWorkspaceTenant} from '../helpers/navigation';
const origin=new URL(consoleURL).origin;const api=`${origin}/api/v1`;const meta=()=>({request_id:randomUUID(),reason:'设备产品自验证'});
async function seed(request:APIRequestContext){
 const user=await createAccount(request,false);const headers={Origin:origin,'X-CSRF-Token':await csrf(request,origin)};
 const post=async(path:string,data:unknown,status=200)=>{const r=await request.post(`${api}${path}`,{headers,data});expect(r.status(),await r.text()).toBe(status);return status===204?{}:r.json();};
 await post('/auth/login',{username:'e2e_owner',password});const suffix=randomBytes(5).toString('hex');
 const publish=async(kind:string,content:object)=>{const d=await post(`/platform/engineering/${kind}`,{...meta(),code:`dev_${kind.replaceAll('-','_')}_${suffix}`,content:{metamodel_version:'1',description:'',release_notes:'',...content}});return post(`/platform/engineering/${kind}/${d.definition_id}/publish`,{...meta(),revision:d.revision});};
 const metric=await publish('metrics',{name:`开关状态_${suffix}`,metric:{shape:'enum',category:'',standard_unit:'',allowed_units:[],options:[{key:'normal',name:'正常'},{key:'abnormal',name:'异常'}]}});
 const type=await publish('type-definitions',{name:`设备验收设施_${suffix}`,fields:[],components:[],relations:[]});
 const manufacturer=await post('/platform/device-definitions/manufacturers',{...meta(),code:`factory_${suffix}`,name:`设备厂家_${suffix}`});
 const draft=await post('/platform/device-definitions/products',{...meta(),manufacturer_id:manufacturer.id,code:`product_${suffix}`,name:`状态采集仪_${suffix}`,model:'D-01',output_slots:[{key:'state',name:'固定状态',mode:'fixed',metric_version_id:metric.version_id,unit_version_id:null,timestamp_meaning:'observation_time'},{key:'extension',name:'扩展槽位',mode:'configurable',allowed_shapes:['enum']}]});
 const product=await post(`/platform/device-definitions/products/${draft.id}/publish`,{...meta(),expected_draft_revision:draft.revision,classification:{mode:'unclassified'}});
 const accounts=await(await request.get(`${api}/access/accounts?q=${user.username}`)).json();const tenant=await post('/platform/tenants',{name:`设备租户_${suffix}`,code:`device-${suffix}`,administrator_account_id:accounts.items[0].id,client_request_id:randomUUID(),reason:'设备验收'},201);
 await post('/auth/logout',undefined,204);await post('/auth/login',user);
 const tid=tenant.tenant_id as string;const tenantContext=await(await request.get(`${api}/tenants/${tid}/context`)).json();
 const project=await post(`/tenants/${tid}/projects`,{name:`设备项目_${suffix}`,code:`DP-${suffix}`,purpose:'设备授权验证',description:'',owner_tenant_membership_id:tenantContext.membership_id,administrator_tenant_membership_id:tenantContext.membership_id,client_request_id:randomUUID(),reason:'设备验收'});
 const facility=await post(`/tenants/${tid}/engineering/resources/facilities`,{request_id:randomUUID(),code:'FAC',name:'设备验收设施',type_version_id:type.version_id,values:{},description:''},201);
 const point=await post(`/tenants/${tid}/engineering/resources/facilities/${facility.id}/points`,{request_id:randomUUID(),code:'PT',name:'状态测点'},201);
 const channel=await post(`/tenants/${tid}/engineering/resources/facilities/${facility.id}/points/${point.id}/channels`,{request_id:randomUUID(),code:'CH',name:'状态通道',metric_version_id:metric.version_id,unit_version_id:null,timestamp_meaning:'observation_time'},201);
 return {user,headers,post,suffix,tenant:tid,project:project.project_id as string,manufacturer,product,metric,facility,point,channel};
}
async function confirm(page:Page){const button=page.getByRole('button',{name:'确认提交',exact:true});await expect(button).toBeVisible();await button.click();}
async function select(page:Page,label:string,text:string){await page.getByRole('combobox',{name:label,exact:true}).click();await page.getByRole('option',{name:text,exact:true}).click();}

test('tenant device workspace preserves navigation and filters current locations across pages', async ({ page, request, isMobile }, info) => {
  test.setTimeout(150_000);
  const f = await seed(request);
  const devices: { id: string; name: string }[] = [];
  for (let index = 0; index < 27; index++) {
    const name = `工作区设备 ${String(index).padStart(2, '0')}`;
    const result = await f.post(`/tenants/${f.tenant}/devices`, { ...meta(), code: `WORK-${index}`, name, product_version_id: f.product.version_id });
    devices.push({ id: result.id, name });
  }
  for (const [index, target, description] of [[1, { kind: 'location' }, '独立机柜'], [24, { kind: 'facility', id: f.facility.id }, ''], [26, { kind: 'point', id: f.point.id }, '']] as const) {
    await f.post(`/tenants/${f.tenant}/devices/${devices[index]!.id}/installations`, { ...meta(), expected_installation_revision: '0', target, location_description: description });
  }
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await login(page, f.user);
  await page.goto(`${consoleURL}/t/${f.tenant}/devices?q=WORK-`);
  const list = page.getByRole('navigation', { name: '选择设备', exact: true });
  const detail = page.getByRole('region', { name: '设备详情工作区', exact: true });
  await expect(list.getByRole('link')).toHaveCount(25);
  await page.getByRole('button', { name: '下一页设备', exact: true }).click();
  await expect(list.getByRole('link')).toHaveCount(2);
  await list.getByRole('link', { name: devices[26]!.name, exact: true }).click();
  await expect(detail.getByRole('heading', { name: devices[26]!.name, exact: true })).toBeVisible();
  await page.getByRole('tab', { name: '输出槽', exact: true }).click();
  await page.getByRole('button', { name: '固定状态 · 第 1 代', exact: true }).click();
  expect(new URL(page.url()).searchParams.get('q')).toBe('WORK-');
  expect(new URL(page.url()).searchParams.get('output')).toBeTruthy();
  if (isMobile) await page.getByRole('button', { name: '返回设备列表', exact: true }).click();
  await expect(list.getByRole('link')).toHaveCount(2);
  await list.getByRole('link', { name: devices[25]!.name, exact: true }).click();
  await expect(page.getByRole('tab', { name: '输出槽', exact: true })).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByRole('table', { name: '产品输出槽', exact: true })).toBeVisible();
  expect(new URL(page.url()).searchParams.has('output')).toBe(false);
  if (!isMobile) {
    await page.goBack();
    await expect(detail.getByRole('heading', { name: devices[26]!.name, exact: true })).toBeVisible();
    await expect(detail.getByRole('heading', { name: '固定状态 · 第 1 代', exact: true })).toBeVisible();
    await page.goForward();
    await expect(detail.getByRole('heading', { name: devices[25]!.name, exact: true })).toBeVisible();
  }
  if (isMobile) await page.getByRole('button', { name: '返回设备列表', exact: true }).click();
  await page.getByRole('button', { name: '安装位置', exact: true }).click();
  await page.getByRole('menuitemradio', { name: '独立位置', exact: true }).click();
  await expect(list.getByRole('link')).toHaveCount(1);
  await expect(list).toContainText('独立位置 · 独立机柜');
  await page.getByRole('button', { name: '所属设施', exact: true }).click();
  const picker = page.getByRole('dialog', { name: '筛选所属设施', exact: true });
  await picker.getByRole('button', { name: /设备验收设施 FAC/ }).click();
  await expect(list.getByRole('link')).toHaveCount(2);
  await expect(list.getByRole('link', { name: devices[24]!.name, exact: true })).toBeVisible();
  await expect(list.getByRole('link', { name: devices[26]!.name, exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '上一页设备', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: '厂家', exact: true }).click();
  const makers = page.getByRole('dialog', { name: '筛选厂家', exact: true });
  await makers.getByRole('textbox').fill(f.suffix);
  await makers.getByRole('button', { name: new RegExp(`设备厂家_${f.suffix}`) }).click();
  await page.getByRole('button', { name: '产品', exact: true }).click();
  await page.getByRole('dialog', { name: '筛选产品', exact: true }).getByRole('button', { name: new RegExp(`状态采集仪_${f.suffix}`) }).click();
  await expect(list.getByRole('link')).toHaveCount(2);
  await page.reload();
  await expect(page.getByRole('button', { name: '所属设施', exact: true })).toContainText('设备验收设施');
  await expect(list.getByRole('link')).toHaveCount(2);
  await page.screenshot({ path: info.outputPath('device-workspace-filtered.png'), fullPage: true });
  await page.getByRole('button', { name: '产品来源', exact: true }).click();
  await page.getByRole('menuitemradio', { name: '本租户私有', exact: true }).click();
  await expect(list.getByRole('link')).toHaveCount(0);
  await expect(list.getByText('没有符合条件的设备', { exact: true })).toBeVisible();
  await list.getByRole('button', { name: '清空筛选', exact: true }).click();
  await expect(list.getByRole('link')).toHaveCount(25);
  await page.evaluate(() => document.documentElement.classList.add('dark'));
  if (isMobile) await list.getByRole('link').first().click();
  await page.screenshot({ path: info.outputPath('device-workspace-dark.png'), fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  if (!isMobile) {
    await page.setViewportSize({ width: 1024, height: 768 });
    await expect(list).toBeVisible();
    await expect(detail).toBeHidden();
    await list.getByRole('link').last().click();
    await expect(detail).toBeVisible();
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0);
    await expect(page.getByRole('button', { name: '返回设备列表', exact: true })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath('device-workspace-tablet.png'), fullPage: true });
  }
  expect(errors).toEqual([]);
});

test('device registration, independent installation, immediate source and state cleanup',async({page,request},info)=>{
 test.setTimeout(120_000);const f=await seed(request);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await login(page,f.user);
 await page.goto(`${consoleURL}/t/${f.tenant}/devices`);await page.getByRole('button',{name:'登记设备',exact:true}).click();const dialog=page.getByRole('dialog');await dialog.getByLabel('设备编号 *',{exact:true}).fill('DEV-01');await dialog.getByLabel('搜索产品名称、型号或编号',{exact:true}).fill(f.suffix);await dialog.getByRole('button',{name:`状态采集仪_${f.suffix} · D-01 · v1`,exact:true}).click();await dialog.getByLabel('设备名称 *',{exact:true}).fill('现场状态设备');await dialog.getByLabel('序列号（可选）',{exact:true}).fill('private-serial');const created=page.waitForResponse(r=>r.url().endsWith(`/tenants/${f.tenant}/devices`)&&r.request().method()==='POST');await dialog.getByRole('button',{name:'登记设备',exact:true}).click();const response=await created;expect(response.status(),await response.text()).toBe(200);const device=await response.json();await expect(page.getByRole('heading',{name:'现场状态设备',exact:true})).toBeVisible();await expect(page.getByRole('region',{name:'设备详情工作区'}).getByText('无当前安装记录',{exact:true})).toBeVisible();
 await page.getByRole('tab',{name:'输出槽',exact:true}).click();await expect(page.getByRole('button',{name:'固定状态 · 第 1 代',exact:true})).toBeVisible();await expect(page.getByText('未配置',{exact:true})).toBeVisible();await page.getByRole('button',{name:'固定状态 · 第 1 代',exact:true}).click();await expect(page.getByRole('heading',{name:'固定状态 · 第 1 代',exact:true})).toBeVisible();await page.getByRole('tab',{name:'来源关联',exact:true}).click();await page.getByRole('button',{name:'状态通道 · CH',exact:true}).click();await expect(page.getByText('未配置来源',{exact:true})).toBeVisible();await page.getByRole('button',{name:'配置来源',exact:true}).click();await page.getByLabel('变更原因 *',{exact:true}).fill('直接来源，无安装前置');await page.getByRole('button',{name:'预览来源变更',exact:true}).click();await confirm(page);await expect(page.getByRole('button',{name:'解除来源',exact:true})).toBeVisible();
 await page.getByRole('tab',{name:'安装履历',exact:true}).click();await page.getByRole('button',{name:'记录安装或补录历史',exact:true}).click();await page.getByLabel('位置说明 *',{exact:true}).fill('机柜第三层，独立安装位置');await page.getByLabel('原因（共享设备必填）',{exact:true}).fill('补记机柜安装');await page.getByRole('button',{name:'预览安装变更',exact:true}).click();await confirm(page);await expect(page.getByRole('dialog')).toHaveCount(0);await expect(page.getByText('机柜第三层，独立安装位置',{exact:true})).toBeVisible();
 await page.getByRole('button',{name:'设备的操作',exact:true}).click();await page.getByRole('menuitem',{name:'停用设备',exact:true}).click();await page.getByLabel('操作原因 *',{exact:true}).fill('停用后清理原来源');await page.getByRole('button',{name:'预览变更',exact:true}).click();await confirm(page);await page.getByRole('button',{name:'设备的操作',exact:true}).click();await expect(page.getByRole('menuitem',{name:'启用设备',exact:true})).toBeVisible();await page.keyboard.press('Escape');
 await page.goto(`${consoleURL}/t/${f.tenant}/resources/facilities/${f.facility.id}?tab=channels&point=${f.point.id}&channel=${f.channel.id}`);await expect(page.getByText('当前来源对应的设备或输出已停用。保留关系供查询，可由具备权限的人解除。',{exact:true})).toBeVisible();await page.getByRole('button',{name:'解除来源',exact:true}).click();await page.getByLabel('变更原因 *',{exact:true}).fill('停用设备解除来源');await page.getByRole('button',{name:'预览来源变更',exact:true}).click();await confirm(page);await expect(page.getByText('未配置来源',{exact:true})).toBeVisible();
 const detail=await(await page.request.get(`${api}/tenants/${f.tenant}/devices/${device.id}`)).json();expect(detail.status).toBe('disabled');expect(detail.current_installation.location_description).toBe('机柜第三层，独立安装位置');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:info.outputPath('device-source-cleanup.png'),fullPage:true});expect(errors).toEqual([]);
});

test('public manufacturer private product draft and immutable publication',async({page,request},info)=>{
 test.setTimeout(90_000);const f=await seed(request);await login(page,f.user);await page.goto(`${consoleURL}/t/${f.tenant}/device-products?source=public&manufacturer=${f.manufacturer.id}`);await page.getByRole('button',{name:'新增本租户产品',exact:true}).click();await page.getByLabel('产品编号 *',{exact:true}).fill(`private-${f.suffix}`);await page.getByLabel('产品名称 *',{exact:true}).fill('本租户独立产品');await page.getByLabel('型号 *',{exact:true}).fill('PRIVATE');await page.getByRole('button',{name:'添加输出槽',exact:true}).click();await page.getByLabel('槽位 key *',{exact:true}).fill('custom');await page.getByLabel('输出槽名称 *',{exact:true}).fill('本地可配置输出');await page.getByRole('button',{name:'创建产品草稿',exact:true}).click();await expect(page.getByRole('heading',{name:'本租户独立产品',exact:true})).toBeVisible();await page.getByRole('button',{name:'校验并发布',exact:true}).click();await expect(page.getByText('草稿校验通过。',{exact:true})).toBeVisible();await page.getByLabel('操作原因 *',{exact:true}).fill('发布私有产品');await page.getByRole('button',{name:'确认发布',exact:true}).click();await expect(page.getByRole('button',{name:'从所选版本创建草稿',exact:true})).toBeVisible();await expect(page.getByText('尚未发布版本。通过校验并发布后，设备才能采用此产品。',{exact:true})).toHaveCount(0);await page.screenshot({path:info.outputPath('private-product.png'),fullPage:true});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
});

test('explicit output grant, restricted parent, revoke and request receipt',async({page,request},info)=>{
 test.setTimeout(120_000);const f=await seed(request);const device=await f.post(`/tenants/${f.tenant}/devices`,{...meta(),code:'GRANT-DEV',name:'只授权一个输出的设备',serial_number:'hidden-serial-number',description:'hidden-private-description',product_version_id:f.product.version_id});const slots=await(await request.get(`${api}/tenants/${f.tenant}/devices/${device.id}/outputs`)).json();const output=slots.items.find((s:{current:unknown})=>s.current).current;await login(page,f.user);
 await page.goto(`${consoleURL}/t/${f.tenant}/projects/${f.project}/device-access`);await page.getByRole('tab',{name:'新增对象',exact:true}).click();await select(page,'授权对象种类','具体输出');await page.getByLabel('选择 固定状态',{exact:true}).check();await page.getByLabel('允许使用',{exact:true}).check();await page.getByLabel('授权变更原因 *',{exact:true}).fill('仅授予这个输出');await page.getByRole('button',{name:'预览授权变更',exact:true}).click();await confirm(page);await expect(page.getByLabel('可作为来源使用',{exact:true})).toBeChecked();
 await page.goto(`${consoleURL}/t/${f.tenant}/p/${f.project}/devices?tab=outputs`);await page.getByRole('link',{name:'固定状态',exact:true}).click();await expect(page.getByRole('heading',{name:'固定状态',exact:true})).toBeVisible();await expect(page.getByText('hidden-serial-number',{exact:true})).toHaveCount(0);await expect(page.getByText('hidden-private-description',{exact:true})).toHaveCount(0);expect((await page.request.get(`${api}/tenants/${f.tenant}/projects/${f.project}/devices/${device.id}`)).status()).toBe(404);await expect(page.getByRole('tab',{name:'观测含义',exact:true})).toBeVisible();await page.screenshot({path:info.outputPath('project-output.png'),fullPage:true});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 const grants=await(await request.get(`${api}/tenants/${f.tenant}/projects/${f.project}/device-access`)).json();const input={...meta(),expected_device_access_revision:grants.device_access_revision,expected_project_revision:grants.project_revision,add:[],update:[],end:[{grant_id:grants.items[0].id,expected_revision:grants.items[0].revision}]};const preview=await f.post(`/tenants/${f.tenant}/projects/${f.project}/device-access/preview`,input);await f.post(`/tenants/${f.tenant}/projects/${f.project}/device-access/changes`,{...input,preview_hash:preview.preview_hash});await page.reload();await expect(page.getByRole('heading',{name:'固定状态',exact:true})).toHaveCount(0);expect((await page.request.get(`${api}/tenants/${f.tenant}/projects/${f.project}/device-outputs/${output.id}`)).status()).toBe(404);expect((await request.get(`${api}/tenants/${f.tenant}/device-requests/${input.request_id}`)).status()).toBe(200);
});

test('lost write response preserves input, receipt resolves once, keyboard device list and dark layout',async({page,request},info)=>{
 test.setTimeout(120_000);const f=await seed(request);await login(page,f.user);await page.goto(`${consoleURL}/t/${f.tenant}/devices`);await page.getByRole('button',{name:'登记设备',exact:true}).click();const dialog=page.getByRole('dialog');await dialog.getByLabel('设备编号 *',{exact:true}).fill('UNCERTAIN');await dialog.getByLabel('搜索产品名称、型号或编号',{exact:true}).fill(f.suffix);await dialog.getByRole('button',{name:`状态采集仪_${f.suffix} · D-01 · v1`,exact:true}).click();const name='设备名称较长时仍应换行且可通过键盘定位设备资料';await dialog.getByLabel('设备名称 *',{exact:true}).fill(name);
 let writes=0;await page.route(`**/api/v1/tenants/${f.tenant}/devices`,async route=>{if(route.request().method()!=='POST'){await route.continue();return;}writes++;const response=await route.fetch();expect(response.status()).toBe(200);await route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({code:'unavailable',message:'模拟提交后响应中断'})});});
 await dialog.getByRole('button',{name:'登记设备',exact:true}).click();await expect(dialog.getByText('提交结果尚未确认，输入已保留。请先查验本次结果。',{exact:true})).toBeVisible();await expect(dialog.getByLabel('设备名称 *',{exact:true})).toHaveValue(name);await expect(dialog.getByRole('button',{name:'登记设备',exact:true})).toBeDisabled();await dialog.getByRole('button',{name:'查验本次提交结果',exact:true}).click();await expect(page.getByRole('heading',{name,exact:true})).toBeVisible();expect(writes).toBe(1);await page.unroute(`**/api/v1/tenants/${f.tenant}/devices`);
 await page.goto(`${consoleURL}/t/${f.tenant}/devices?q=UNCERTAIN`);const list=page.getByRole('navigation',{name:'选择设备',exact:true});const item=list.getByRole('link',{name,exact:true});await item.focus();await page.keyboard.press('Home');await expect(item).toBeFocused();await page.keyboard.press('Enter');await expect(page.getByRole('heading',{name,exact:true})).toBeVisible();

 await page.evaluate(()=>document.documentElement.classList.add('dark'));await page.screenshot({path:info.outputPath('device-dark-keyboard.png'),fullPage:true});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
});

test('device edit conflict reloads the latest record while retaining the draft',async({page,request})=>{
 const markupErrors:string[]=[];page.on('console',m=>{if(m.type()==='error'&&/cannot contain|cannot be a descendant|hydration/i.test(m.text()))markupErrors.push(m.text());});
 test.setTimeout(90_000);const f=await seed(request);const device=await f.post(`/tenants/${f.tenant}/devices`,{...meta(),code:'CONFLICT',name:'最初设备',product_version_id:f.product.version_id});await login(page,f.user);await page.goto(`${consoleURL}/t/${f.tenant}/devices/${device.id}`);await page.getByRole('button',{name:'编辑档案',exact:true}).click();await page.getByLabel('设备名称 *',{exact:true}).fill('我的保留输入');
 const r=await request.patch(`${api}/tenants/${f.tenant}/devices/${device.id}`,{headers: { ...f.headers, 'X-Command-ID': randomUUID() },data:{...meta(),expected_revision:device.revision,name:'另一窗口已保存'}});expect(r.status(),await r.text()).toBe(200);await page.getByRole('button',{name:'保存资料',exact:true}).click();await expect(page.getByRole('button',{name:'读取最新记录并保留输入',exact:true})).toBeVisible();await expect(page.getByLabel('设备名称 *',{exact:true})).toHaveValue('我的保留输入');await page.getByRole('button',{name:'读取最新记录并保留输入',exact:true}).click();await expect(page.getByText('最新已保存资料：另一窗口已保存 · 无序列号',{exact:true})).toBeVisible();await expect(page.getByLabel('设备名称 *',{exact:true})).toHaveValue('我的保留输入');await page.getByRole('button',{name:'保存资料',exact:true}).click();await expect(page.getByRole('heading',{name:'我的保留输入',exact:true})).toBeVisible();expect(markupErrors).toEqual([]);
});

test('late device write cannot reopen details after changing tenant workspace',async({page,request})=>{
 test.setTimeout(120_000);const f=await seed(request);const device=await f.post(`/tenants/${f.tenant}/devices`,{...meta(),code:'LATE',name:'原租户私有设备',product_version_id:f.product.version_id});await f.post('/auth/logout',undefined,204);await f.post('/auth/login',{username:'e2e_owner',password});const account=(await(await request.get(`${api}/access/accounts?q=${f.user.username}`)).json()).items[0];const second=await f.post('/platform/tenants',{name:`设备隔离租户_${f.suffix}`,code:`dev-other-${f.suffix}`,administrator_account_id:account.id,client_request_id:randomUUID(),reason:'迟到设备响应验证'},201);
 await login(page,f.user);await page.goto(`${consoleURL}/t/${f.tenant}/devices/${device.id}`);await page.getByRole('button',{name:'编辑档案',exact:true}).click();await page.getByLabel('设备名称 *',{exact:true}).fill('原租户迟到设备内容');let release!:()=>void;const held=new Promise<void>(resolve=>{release=resolve;});let fetched!:()=>void;const ready=new Promise<void>(resolve=>{fetched=resolve;});const path=`${api}/tenants/${f.tenant}/devices/${device.id}`;await page.route(path,async route=>{if(route.request().method()!=='PATCH'){await route.continue();return;}const r=await route.fetch();expect(r.status()).toBe(200);fetched();await held;try{await route.fulfill({response:r});}catch{/* Completed write can be canceled by scope exit. */}});
 await page.getByRole('button',{name:'保存资料',exact:true}).click();await ready;await page.getByRole('button',{name:'关闭面板',exact:true}).click();const picker=await openWorkspacePicker(page);await browseWorkspaceTenant(page,`设备隔离租户_${f.suffix}`);await picker.getByRole('button',{name:new RegExp(`^设备隔离租户_${f.suffix} 租户`)}).click();await expect(page).toHaveURL(`${consoleURL}/t/${second.tenant_id}/profile`);release();await page.unroute(path);await expect(page.getByRole('dialog',{name:'切换工作空间',exact:true,includeHidden:true})).toHaveCount(0);await expect(page.getByRole('dialog',{name:'主导航',exact:true,includeHidden:true})).toHaveCount(0);await navigateTo(page,'设备台账');await expect(page).toHaveURL(`${consoleURL}/t/${second.tenant_id}/devices`);await expect(page.getByRole('dialog')).toHaveCount(0);await expect(page.getByText('原租户迟到设备内容',{exact:true})).toHaveCount(0);expect((await(await page.request.get(path)).json()).name).toBe('原租户迟到设备内容');
});

test('manufacturer catalog Drawer and product version workspace keep real writes and fixed versions', async ({ page, request, isMobile }, info) => {
  test.setTimeout(120_000);
  const f = await seed(request);
  await f.post('/auth/logout', undefined, 204);
  await f.post('/auth/login', { username: 'e2e_owner', password });
  const root = `/platform/device-definitions/products/${f.product.id}`;
  const identity = await (await request.get(`${api}${root}`)).json();
  const draft = await f.post(`${root}/drafts`, { ...meta(), expected_revision: identity.revision, version_id: f.product.version_id });
  const save = await request.put(`${api}${root}/draft`, { headers: { ...f.headers, 'X-Command-ID': randomUUID() }, data: { ...meta(), expected_draft_revision: draft.revision, content: { ...identity.versions[0].content, description: '已发布 v2 的固定产品说明' } } });
  expect(save.status(), await save.text()).toBe(200);
  await f.post(`${root}/publish`, { ...meta(), expected_draft_revision: (await save.json()).revision, classification: { mode: 'unclassified' } });
  await login(page, { username: 'e2e_owner', password });
  await page.goto(`${consoleURL}/platform/device-products?manufacturer=${f.manufacturer.id}`);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  if (isMobile) await page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true }).click();
  const navigation = page.getByRole('navigation', { name: '主导航', exact: true });
  const monitoring = navigation.getByRole('group', { name: '监测业务', exact: true });
  await expect(monitoring.getByRole('link', { name: '公共设备产品', exact: true })).toBeVisible();
  await expect(monitoring.getByRole('button', { name: '设备管理', exact: true })).toHaveCount(0);
  await monitoring.getByRole('button', { name: '工程配置', exact: true }).click();
  await expect(monitoring.getByRole('link', { name: '指标目录', exact: true })).toBeVisible();
  if (isMobile) await page.getByRole('button', { name: '关闭导航', exact: true }).click();
  const product = page.getByRole('link', { name: `状态采集仪_${f.suffix}`, exact: true });
  await expect(product).toBeVisible();
  await expect(product).toContainText('v2');
  await page.screenshot({ path: info.outputPath('product-catalog-light.png'), fullPage: true });

  const manage = page.getByRole('button', { name: '管理厂家', exact: true });
  await manage.click();
  const drawer = page.getByRole('dialog', { name: '管理厂家', exact: true });
  await expect(drawer).toBeVisible();
  await expect(page.locator('[data-slot="drawer-popup"]')).toHaveAttribute('data-swipe-direction', isMobile ? 'down' : 'right');
  await expect(page.locator('[data-slot="drawer-overlay"]')).toHaveCSS('backdrop-filter', /blur\([1-9]/);
  await drawer.getByLabel('搜索厂家名称或编号', { exact: true }).fill(f.suffix);
  await expect(drawer.getByRole('table', { name: '厂家管理', exact: true })).toContainText(`设备厂家_${f.suffix}`);
  await drawer.getByRole('button', { name: '新建厂家', exact: true }).click();
  await page.getByLabel('厂家编号 *', { exact: true }).fill(`drawer_${f.suffix}`);
  await page.getByLabel('厂家名称 *', { exact: true }).fill(`抽屉厂家_${f.suffix}`);
  await page.getByRole('button', { name: '创建厂家', exact: true }).click();
  await expect(page.getByRole('dialog', { name: '管理厂家', exact: true })).toBeVisible();
  await page.getByLabel('搜索厂家名称或编号', { exact: true }).fill(`抽屉厂家_${f.suffix}`);
  await expect(page.getByRole('table', { name: '厂家管理', exact: true })).toContainText(`抽屉厂家_${f.suffix}`);
  await page.getByRole('button', { name: '关闭面板', exact: true }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(manage).toBeFocused();
  await expect(page).toHaveURL(new RegExp(`manufacturer=${f.manufacturer.id}`));

  await product.click();
  await expect(page.getByRole('heading', { name: `状态采集仪_${f.suffix}`, exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '查看发布版本 v2', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('table', { name: '产品输出槽', exact: true })).toContainText('固定状态');
  await expect(page.getByRole('table', { name: '产品输出槽', exact: true })).toContainText('可配置');
  await page.getByRole('tab', { name: '版本比较', exact: true }).click();
  await expect(page.getByRole('table', { name: '产品版本差异', exact: true })).toContainText('已发布 v2 的固定产品说明');
  await page.getByRole('button', { name: '查看发布版本 v1', exact: true }).click();
  await page.getByRole('button', { name: '从所选版本创建草稿', exact: true }).click();
  await page.getByLabel('操作原因 *', { exact: true }).fill('从明确的历史版本创建草稿');
  const creation = page.waitForRequest((r) => r.url().endsWith(`${root}/drafts`) && r.method() === 'POST');
  await page.getByRole('button', { name: '创建草稿', exact: true }).click();
  expect((await creation).postDataJSON().version_id).toBe(f.product.version_id);
  await expect(page.getByRole('button', { name: '查看当前草稿', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await page.getByRole('button', { name: '编辑草稿', exact: true }).click();
  await page.getByLabel('产品说明', { exact: true }).fill('草稿里单独修改的说明');
  await page.getByRole('button', { name: '保存草稿', exact: true }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.getByRole('tab', { name: '产品信息', exact: true }).click();
  await expect(page.getByText('草稿里单独修改的说明', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '查看发布版本 v2', exact: true }).click();
  await expect(page.getByText('已发布 v2 的固定产品说明', { exact: true })).toBeVisible();
  await expect(page.getByText('草稿里单独修改的说明', { exact: true })).toHaveCount(0);
  await page.getByRole('tab', { name: /^输出槽/ }).click();
  await page.screenshot({ path: info.outputPath('product-version-light.png'), fullPage: true });
  await page.getByRole('button', { name: '切换主题', exact: true }).click();
  await page.getByRole('menuitemradio', { name: '深色', exact: true }).click();
  await page.screenshot({ path: info.outputPath('product-version-dark.png'), fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.getByRole('link', { name: '返回设备产品目录', exact: true }).click();
  await manage.click();
  await expect(page.getByRole('dialog', { name: '管理厂家', exact: true })).toBeVisible();
  await expect(page.getByRole('table', { name: '厂家管理', exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath('manufacturer-drawer-dark.png') });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect(errors).toEqual([]);
});

test('tenant and project device workspaces share presentation and retain separate permissions', async ({ page, request, isMobile }, info) => {
  test.setTimeout(120_000);
  const f = await seed(request);
  const device = await f.post(`/tenants/${f.tenant}/devices`, { ...meta(), code: 'SCOPED-DEVICE', name: '分层展示的现场设备', serial_number: 'SERIAL-FULL-GRANT', product_version_id: f.product.version_id });
  const slots = await (await request.get(`${api}/tenants/${f.tenant}/devices/${device.id}/outputs`)).json();
  const output = slots.items.find((slot: { current: unknown }) => slot.current).current;
  const grantPath = `/tenants/${f.tenant}/projects/${f.project}/device-access`;
  const grants = await (await request.get(`${api}${grantPath}`)).json();
  const input = { ...meta(), expected_device_access_revision: grants.device_access_revision, expected_project_revision: grants.project_revision, add: [{ kind: 'device', target_id: device.id, use_source: false }, { kind: 'output', target_id: output.id, use_source: false }], update: [], end: [] };
  const preview = await f.post(`${grantPath}/preview`, input);
  await f.post(`${grantPath}/changes`, { ...input, preview_hash: preview.preview_hash });
  await login(page, f.user);
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));

  await page.goto(`${consoleURL}/t/${f.tenant}/device-products?source=public&manufacturer=${f.manufacturer.id}`);
  await expect(page.getByRole('button', { name: '管理厂家', exact: true })).toHaveCount(0);
  await page.getByRole('link', { name: `状态采集仪_${f.suffix}`, exact: true }).click();
  await expect(page.getByRole('button', { name: '查看发布版本 v1', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('button', { name: '编辑草稿', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: '从所选版本创建草稿', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: '复制为本租户新产品', exact: true })).toBeVisible();
  await page.goto(`${consoleURL}/t/${f.tenant}/device-products?source=private`);
  const manage = page.getByRole('button', { name: '管理厂家', exact: true });
  await manage.click();
  await page.getByRole('button', { name: '新建厂家', exact: true }).click();
  await page.getByLabel('厂家编号 *', { exact: true }).fill(`tenant_${f.suffix}`);
  await page.getByLabel('厂家名称 *', { exact: true }).fill('租户独立维护的厂家');
  const write = page.waitForRequest(request => request.method() === 'POST' && request.url().endsWith(`/tenants/${f.tenant}/device-definitions/manufacturers`));
  await page.getByRole('button', { name: '创建厂家', exact: true }).click();
  expect((await write).postDataJSON().name).toBe('租户独立维护的厂家');
  await expect(page.getByRole('table', { name: '厂家管理', exact: true })).toContainText('租户独立维护的厂家');
  await page.getByRole('button', { name: '关闭面板', exact: true }).click();
  await expect(manage).toBeFocused();

  await page.goto(`${consoleURL}/t/${f.tenant}/devices?q=SCOPED-DEVICE`);
  await expect(page.getByRole('complementary', { name: '设备列表', exact: true })).toContainText('分层展示的现场设备');
  await page.screenshot({ path: info.outputPath('tenant-device-list.png'), fullPage: true });
  await page.getByRole('link', { name: '分层展示的现场设备', exact: true }).click();
  await expect(page.getByRole('button', { name: '编辑档案', exact: true })).toBeVisible();
  await expect(page.getByText('SERIAL-FULL-GRANT', { exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath('tenant-device-detail.png'), fullPage: true });
  await page.getByRole('tab', { name: '输出槽', exact: true }).click();
  const historyTrigger = page.getByRole('button', { name: '固定状态的操作', exact: true });
  await historyTrigger.click();
  await page.getByRole('menuitem', { name: '代次历史', exact: true }).click();
  await expect(page.getByRole('dialog', { name: '输出代次历史', exact: true })).toBeVisible();
  await page.getByRole('button', { name: '关闭面板', exact: true }).click();
  await expect(historyTrigger).toBeFocused();

  await page.goto(`${consoleURL}/t/${f.tenant}/p/${f.project}/devices`);
  await expect(page.getByRole('tab', { name: '设备档案', exact: true })).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByRole('table', { name: '设备列表', exact: true })).toContainText('分层展示的现场设备');
  if (isMobile) await page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true }).click();
  const group = page.getByRole('navigation', { name: '主导航', exact: true }).getByRole('group', { name: '监测业务', exact: true });
  await expect(group.getByRole('link', { name: '项目设备', exact: true })).toBeVisible();
  await expect(group.getByRole('button', { name: '项目设备', exact: true })).toHaveCount(0);
  if (isMobile) await page.getByRole('button', { name: '关闭导航', exact: true }).click();
  await page.getByRole('link', { name: '分层展示的现场设备', exact: true }).click();
  await expect(page.getByText('SERIAL-FULL-GRANT', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '编辑档案', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: '设备的操作', exact: true })).toHaveCount(0);
  await page.getByRole('tab', { name: '获准输出', exact: true }).click();
  await page.getByRole('link', { name: '固定状态', exact: true }).click();
  await expect(page.getByRole('heading', { name: '固定状态', exact: true })).toBeVisible();
  await page.getByRole('link', { name: '返回获准设备与输出', exact: true }).click();
  await expect(page.getByRole('tab', { name: '获准输出', exact: true })).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByRole('tabpanel', { name: '获准输出', exact: true }).getByRole('table', { name: '获准设备输出', exact: true })).toContainText('固定状态');
  await page.getByRole('button', { name: '切换主题', exact: true }).click();
  await page.getByRole('menuitemradio', { name: '深色', exact: true }).click();
  const outputLink = page.getByRole('link', { name: '固定状态', exact: true });
  await expect(outputLink).toBeVisible();
  const foreground = await page.locator('body').evaluate(element => getComputedStyle(element).color);
  await expect.poll(() => outputLink.evaluate(element => getComputedStyle(element).color)).toBe(foreground);
  await page.screenshot({ path: info.outputPath('project-output-list-dark.png'), fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect(errors).toEqual([]);
});
