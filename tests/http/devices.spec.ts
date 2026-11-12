import {randomBytes,randomUUID} from 'node:crypto';
import {test,expect} from '@playwright/test';
const origin=`http://${process.env.INFRAPRO_HTTP_TEST_HOST}:18088`;const base=`${origin}/console`;
test('device catalog and registration under ordinary HTTP, base path and strict CSP',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await page.addInitScript(()=>{const violations:string[]=[];Object.assign(window,{deviceCSP:violations});document.addEventListener('securitypolicyviolation',e=>violations.push(e.effectiveDirective));});
 await page.goto(`${base}/login`);await page.getByLabel('用户名',{exact:true}).fill('e2e_owner');await page.getByLabel('密码',{exact:true}).fill('Real browser 河流 secret 592');await page.getByRole('button',{name:'登录',exact:true}).click();await expect(page).not.toHaveURL(/\/login/);
 const token=(await(await page.request.get(`${origin}/api/v1/auth/csrf`)).json()).token;const headers={Origin:origin,'X-CSRF-Token':token};const meta=()=>({request_id:randomUUID(),reason:'设备部署验证'});const post=async(path:string,data:unknown,status=200)=>{const r=await page.request.post(`${origin}/api/v1${path}`,{headers,data});expect(r.status(),await r.text()).toBe(status);return r.json();};const suffix=randomBytes(5).toString('hex');const self=await(await page.request.get(`${origin}/api/v1/auth/me`)).json();
 const m=await post('/platform/device-definitions/manufacturers',{...meta(),code:`http_${suffix}`,name:'HTTP 设备厂家'});const draft=await post('/platform/device-definitions/products',{...meta(),manufacturer_id:m.id,code:`http_${suffix}`,name:`HTTP 产品_${suffix}`,model:'ZERO',output_slots:[]});const product=await post(`/platform/device-definitions/products/${draft.id}/publish`,{...meta(),expected_draft_revision:draft.revision});const tenant=await post('/platform/tenants',{name:'HTTP 设备租户',code:`device-${suffix}`,administrator_account_id:self.account.id,client_request_id:randomUUID(),reason:'部署路径验证'},201);
 await page.goto(`${base}/t/${tenant.tenant_id}/devices`);await page.getByRole('button',{name:'登记设备',exact:true}).click();await page.getByLabel('设备编号 *',{exact:true}).fill('HTTP-DEV');await page.getByLabel('搜索产品名称、型号或编号',{exact:true}).fill(suffix);await page.getByRole('button',{name:`HTTP 产品_${suffix} · ZERO · v1`,exact:true}).click();await page.getByLabel('设备名称 *',{exact:true}).fill('普通 HTTP 设备');await page.getByRole('dialog').getByRole('button',{name:'登记设备',exact:true}).click();await expect(page.getByRole('heading',{name:'普通 HTTP 设备',exact:true})).toBeVisible();await page.reload();await expect(page.getByRole('heading',{name:'普通 HTTP 设备',exact:true})).toBeVisible();
 const id=page.url().split('/').at(-1)!;await post(`/tenants/${tenant.tenant_id}/devices/${id}/installations`,{...meta(),expected_installation_revision:'0',target:{kind:'location'},location_description:'无效空时间',installed_at:''},400);await post(`/tenants/${tenant.tenant_id}/devices/${id}/installations`,{...meta(),expected_installation_revision:0,target:{kind:'location'},location_description:'数字修订'},400);await post(`/tenants/${tenant.tenant_id}/devices`,{...meta(),code:'UNKNOWN',name:'未知字段',product_version_id:product.version_id,online:true},400);
 expect(await page.evaluate(()=>window.isSecureContext)).toBe(false);expect(await page.evaluate(()=>Reflect.get(window,'deviceCSP'))).toEqual([]);expect(errors).toEqual([]);
});

test('proxy accepts large product drafts and preserves handler and proxy body limits', async ({ request }) => {
 const csrf = await (await request.get(`${origin}/api/v1/auth/csrf`)).json();
 const headers = { Origin: origin, 'X-CSRF-Token': csrf.token };
 const post = async (path: string, data: unknown, status = 200) => {
  const response = await request.post(`${origin}/api/v1${path}`, { headers, data });
  expect(response.status(), await response.text()).toBe(status);
  return response.json();
 };
 await post('/auth/login', { username: 'e2e_owner', password: 'Real browser 河流 secret 592' });
 const self = await (await request.get(`${origin}/api/v1/auth/me`)).json();
 const suffix = randomBytes(5).toString('hex');
 const meta = () => ({ request_id: randomUUID(), reason: '代理请求大小回归' });
 const tenant = await post('/platform/tenants', { name: '大请求回归租户', code: `large-${suffix}`, administrator_account_id: self.account.id, client_request_id: randomUUID(), reason: '代理请求验证' }, 201);
 const output_slots = Array.from({ length: 80 }, (_, i) => ({ key: `slot_${i}`, name: `输出 ${i}`, mode: 'configurable', description: 'x'.repeat(160), order: i, allowed_shapes: ['decimal'] }));
 for (const scope of ['/platform', `/tenants/${tenant.tenant_id}`]) {
  const manufacturer = await post(`${scope}/device-definitions/manufacturers`, { ...meta(), code: `large_${suffix}`, name: '大请求厂家' });
  const payload = { ...meta(), manufacturer_id: manufacturer.id, code: `large_${suffix}`, name: '大槽位产品', model: 'LARGE', output_slots };
  expect(Buffer.byteLength(JSON.stringify(payload))).toBeGreaterThan(16 * 1024);
  const draft = await post(`${scope}/device-definitions/products`, payload);
  const saved = await (await request.get(`${origin}/api/v1${scope}/device-definitions/products/${draft.id}`)).json();
  expect(saved.draft.content.output_slots).toHaveLength(80);
  // This exceeds the device handler's 256 KiB ceiling, but fits the proxy's 272 KiB ceiling.
  const largePayload = { ...payload, description: 'x'.repeat(240 * 1024) };
  expect(Buffer.byteLength(JSON.stringify(largePayload))).toBeGreaterThan(256 * 1024);
  expect(Buffer.byteLength(JSON.stringify(largePayload))).toBeLessThan(272 * 1024);
  const tooLarge = await post(`${scope}/device-definitions/products`, largePayload, 413);
  expect(tooLarge.code).toBe('input_too_large');
 }
 const oversized = await request.post(`${origin}/api/v1/platform/device-definitions/products`, { headers, data: { padding: 'x'.repeat(273 * 1024) } });
 expect(oversized.status()).toBe(413);
 // Business payloads must not relax the identity API's 16 KiB proxy limit.
 const auth = await request.post(`${origin}/api/v1/auth/login`, { headers, data: { username: 'e2e_owner', password: 'x'.repeat(17 * 1024) } });
 expect(auth.status()).toBe(413);
});
